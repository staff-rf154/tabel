import 'dart:async';
import 'dart:convert';

import 'package:audioplayers/audioplayers.dart';
import 'package:flutter/material.dart';
import 'package:mobile_scanner/mobile_scanner.dart';

import '../api.dart';
import 'device_context.dart';
import 'offline_queue.dart';

class ScanScreen extends StatefulWidget {
  const ScanScreen({super.key, required this.api, required this.queue});

  final ApiClient api;
  final OfflineQueue queue;

  @override
  State<ScanScreen> createState() => _ScanScreenState();
}

class _ScanFeedback {
  const _ScanFeedback(
      {required this.ok, required this.title, required this.detail});

  final bool ok;
  final String title;
  final String detail;
}

class _SessionScan {
  const _SessionScan(
      {required this.at,
      required this.ok,
      required this.title,
      required this.detail});

  final DateTime at;
  final bool ok;
  final String title;
  final String detail;
}

class _ScanScreenState extends State<ScanScreen> {
  final _scannerController = MobileScannerController();
  final _player = AudioPlayer();

  _ScanFeedback? _feedback;
  final List<_SessionScan> _history = [];
  bool _busy = false;
  DateTime? _lastScan;
  String? _lastRaw;
  Timer? _resetTimer;

  @override
  void initState() {
    super.initState();
    _player.setReleaseMode(ReleaseMode.stop);
  }

  @override
  void dispose() {
    _resetTimer?.cancel();
    _scannerController.dispose();
    _player.dispose();
    super.dispose();
  }

  String? _extractToken(String raw) {
    final trimmed = raw.trim();
    final uuidPattern = RegExp(
        r'[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}');
    if (RegExp(r'^[0-9a-fA-F-]{36}$').hasMatch(trimmed)) {
      return trimmed.toLowerCase();
    }
    try {
      final decoded = jsonDecode(trimmed);
      if (decoded is Map) {
        for (final key in ['token', 'qr_token', 'id']) {
          final value = decoded[key];
          if (value is String && uuidPattern.hasMatch(value)) {
            return value;
          }
        }
      }
    } catch (_) {
      // не JSON — ищем UUID внутри строки
    }
    final match = uuidPattern.firstMatch(trimmed);
    return match?.group(0)?.toLowerCase();
  }

  Future<void> _playFeedback(bool ok) async {
    try {
      await _player.stop();
      await _player.play(
        AssetSource(ok ? 'sounds/ok.wav' : 'sounds/error.wav'),
        mode: PlayerMode.lowLatency,
      );
    } catch (_) {
      // звук не должен блокировать отметку
    }
  }

  void _showFeedback(
      {required bool ok, required String title, required String detail}) {
    _resetTimer?.cancel();
    setState(() {
      _feedback = _ScanFeedback(ok: ok, title: title, detail: detail);
      _history.insert(
          0,
          _SessionScan(
              at: DateTime.now(), ok: ok, title: title, detail: detail));
      if (_history.length > 20) _history.removeRange(20, _history.length);
      _busy = false;
    });
    unawaited(_playFeedback(ok));
    _resetTimer = Timer(Duration(seconds: ok ? 3 : 4), () {
      if (!mounted) return;
      setState(() => _feedback = null);
    });
  }

  Future<void> _handleDetect(BarcodeCapture capture) async {
    if (_busy || _feedback != null) return;
    final raw =
        capture.barcodes.isEmpty ? null : capture.barcodes.first.rawValue;
    if (raw == null || raw.isEmpty) return;
    final now = DateTime.now();
    if (_lastRaw == raw &&
        _lastScan != null &&
        now.difference(_lastScan!) < const Duration(seconds: 8)) {
      return;
    }
    _lastRaw = raw;
    _lastScan = now;
    final token = _extractToken(raw);
    if (token == null) {
      _showFeedback(
          ok: false,
          title: 'ОШИБКА',
          detail: 'QR-код не содержит корректный токен');
      return;
    }
    await _process(token);
  }

  Future<void> _process(String token) async {
    if (_busy) return;
    final pending = await widget.queue.all();
    if (pending.isNotEmpty) {
      _showFeedback(
          ok: false,
          title: 'ОШИБКА',
          detail: 'Сначала отправьте офлайн-очередь ($pending)');
      return;
    }

    setState(() {
      _busy = true;
      _feedback = null;
    });
    _resetTimer?.cancel();

    try {
      final verification = await widget.api.verifyQr(token);
      if (!verification.valid) {
        final reason =
            verification.reason == null ? '' : ' (${verification.reason})';
        _showFeedback(
          ok: false,
          title: 'ОШИБКА',
          detail: 'QR недействителен: ${verification.tokenStatus}$reason',
        );
        return;
      }

      final device = await DeviceContext.collect();
      final mark = await widget.api.createMark(
        qrToken: token,
        clientType: 'mobile',
        lat: device.lat,
        lon: device.lon,
        accuracy: device.accuracy,
        locationAgeSec: device.locationAgeSec,
        deviceOs: device.os,
        deviceOsVersion: device.osVersion,
        deviceModel: device.model,
        appVersion: device.appVersion,
        mockLocation: device.mockLocation,
      );
      if (!mounted) return;
      final employee =
          verification.fullName ?? verification.employeeNumber ?? 'Сотрудник';
      _showFeedback(
        ok: true,
        title: 'OK · ${mark.eventTitle}',
        detail: '$employee · ${mark.timestampServer}',
      );
    } on ApiException catch (error) {
      if (!mounted) return;
      if (error.status == 0) {
        try {
          final device = await DeviceContext.collect();
          await widget.queue.add(QueueItem.create(
            qrToken: token,
            eventType: null,
            lat: device.lat,
            lon: device.lon,
            accuracy: device.accuracy,
            locationAgeSec: device.locationAgeSec,
          ));
          _showFeedback(
              ok: false,
              title: 'ОШИБКА',
              detail: 'Нет связи. Отметка сохранена в офлайн-очередь');
        } catch (_) {
          _showFeedback(ok: false, title: 'ОШИБКА', detail: error.message);
        }
      } else {
        _showFeedback(ok: false, title: 'ОШИБКА', detail: error.message);
      }
    } catch (_) {
      if (mounted) {
        _showFeedback(
            ok: false, title: 'ОШИБКА', detail: 'Не удалось отправить отметку');
      }
    } finally {
      if (mounted && _feedback == null) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return ValueListenableBuilder<int>(
      valueListenable: widget.queue.count,
      builder: (context, pending, _) {
        final feedback = _feedback;
        return ListView(
          padding: const EdgeInsets.all(12),
          children: [
            if (pending > 0)
              Card(
                color: Theme.of(context).colorScheme.errorContainer,
                child: ListTile(
                  leading: const Icon(Icons.cloud_off),
                  title: Text('В офлайн-очереди $pending отметок'),
                  subtitle: const Text(
                      'Отправьте очередь, чтобы продолжить сканирование'),
                ),
              ),
            Container(
              height: 300,
              decoration: BoxDecoration(
                color: Colors.black,
                borderRadius: BorderRadius.circular(12),
                border: feedback == null
                    ? null
                    : Border.all(
                        color:
                            feedback.ok ? Colors.greenAccent : Colors.redAccent,
                        width: 4,
                      ),
              ),
              clipBehavior: Clip.antiAlias,
              child: Stack(
                fit: StackFit.expand,
                children: [
                  MobileScanner(
                    controller: _scannerController,
                    onDetect: _handleDetect,
                  ),
                  if (_busy && feedback == null)
                    const ColoredBox(
                      color: Colors.black54,
                      child: Center(
                          child:
                              CircularProgressIndicator(color: Colors.white)),
                    ),
                  if (feedback != null)
                    ColoredBox(
                      color: feedback.ok
                          ? Colors.green.withOpacity(0.88)
                          : Colors.red.withOpacity(0.88),
                      child: Center(
                        child: Padding(
                          padding: const EdgeInsets.all(20),
                          child: Column(
                            mainAxisSize: MainAxisSize.min,
                            children: [
                              Icon(
                                feedback.ok ? Icons.check_circle : Icons.error,
                                size: 64,
                                color: Colors.white,
                              ),
                              const SizedBox(height: 8),
                              Text(
                                feedback.title,
                                textAlign: TextAlign.center,
                                style: const TextStyle(
                                    color: Colors.white,
                                    fontSize: 26,
                                    fontWeight: FontWeight.bold),
                              ),
                              const SizedBox(height: 6),
                              Text(
                                feedback.detail,
                                textAlign: TextAlign.center,
                                style: const TextStyle(
                                    color: Colors.white, fontSize: 15),
                              ),
                            ],
                          ),
                        ),
                      ),
                    ),
                ],
              ),
            ),
            const SizedBox(height: 8),
            const Text(
              'Наведите QR-код на камеру. Приход или уход определяется автоматически, отметка отправляется сразу.',
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 16),
            Text('Последние сканирования',
                style: Theme.of(context).textTheme.titleMedium),
            const SizedBox(height: 8),
            if (_history.isEmpty)
              const Card(
                child: ListTile(
                  leading: Icon(Icons.history),
                  title: Text('Сканирований в текущей сессии пока нет'),
                ),
              ),
            ..._history.map((item) {
              final time = '${item.at.hour.toString().padLeft(2, '0')}:'
                  '${item.at.minute.toString().padLeft(2, '0')}:'
                  '${item.at.second.toString().padLeft(2, '0')}';
              return Card(
                child: ListTile(
                  leading: Icon(
                    item.ok ? Icons.check_circle : Icons.error,
                    color: item.ok ? Colors.green : Colors.red,
                  ),
                  title: Text(item.title),
                  subtitle: Text('$time · ${item.detail}'),
                ),
              );
            }),
          ],
        );
      },
    );
  }
}
