import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:intl/intl.dart';

import '../api.dart';
import '../models.dart';
import 'offline_queue.dart';

class QueueScreen extends StatefulWidget {
  const QueueScreen({super.key, required this.api, required this.queue});

  final ApiClient api;
  final OfflineQueue queue;

  @override
  State<QueueScreen> createState() => _QueueScreenState();
}

class _QueueScreenState extends State<QueueScreen> {
  List<QueueItem> _items = const [];
  bool _busy = false;
  String? _message;
  final _format = DateFormat('dd.MM.yyyy HH:mm', 'ru');

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final items = await widget.queue.all();
    if (!mounted) return;
    setState(() => _items = items);
    if (items.isNotEmpty) {
      await _sendAll(silent: true);
    }
  }

  Future<void> _sendAll({bool silent = false}) async {
    if (_busy) return;
    setState(() {
      _busy = true;
      if (!silent) _message = null;
    });
    var sent = 0;
    String? failure;
    for (final item in List<QueueItem>.from(await widget.queue.all())) {
      try {
        await widget.api.createMark(
          qrToken: item.qrToken,
          clientType: 'mobile',
          eventType: item.eventType,
          lat: item.lat,
          lon: item.lon,
          accuracy: item.accuracy,
          locationAgeSec: item.locationAgeSec,
          timestampClient: item.timestampClient,
          idempotencyKey: item.idempotencyKey,
        );
        await widget.queue.remove(item.id);
        sent++;
      } on ApiException catch (error) {
        if (error.status == 0) {
          failure = 'Нет связи. Отправка остановлена';
          break;
        }
        if (error.status == 409 || error.status == 201) {
          await widget.queue.remove(item.id);
          sent++;
          continue;
        }
        failure = 'Строка ${item.id.substring(0, 8)}: ${error.message}';
      }
    }
    final remaining = await widget.queue.all();
    if (!mounted) return;
    setState(() {
      _items = remaining;
      _busy = false;
      _message = failure ?? (sent > 0 ? 'Отправлено: $sent' : null);
    });
  }

  Future<void> _remove(QueueItem item) async {
    await widget.queue.remove(item.id);
    await _load();
  }

  Future<void> _export() async {
    final csv = widget.queue.exportCsv(_items);
    await Clipboard.setData(ClipboardData(text: csv));
    if (!mounted) return;
    setState(() => _message = 'CSV скопирован в буфер обмена');
  }

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(12),
      children: [
        Row(
          children: [
            Expanded(
              child: FilledButton.icon(
                onPressed: _busy || _items.isEmpty ? null : () => _sendAll(),
                icon: const Icon(Icons.cloud_upload),
                label: Text(_busy ? 'Отправка…' : 'Отправить все'),
              ),
            ),
            const SizedBox(width: 8),
            OutlinedButton.icon(
              onPressed: _items.isEmpty ? null : _export,
              icon: const Icon(Icons.copy),
              label: const Text('CSV'),
            ),
          ],
        ),
        if (_message != null)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 12),
            child: Text(_message!),
          ),
        if (_items.isEmpty)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 32),
            child: Center(child: Text('Очередь пуста')),
          ),
        ..._items.map(
          (item) => Card(
            child: ListTile(
              leading: const Icon(Icons.schedule),
              title: Text(_format.format(item.timestampClient)),
              subtitle: Text('${eventTypeTitle(item.eventType)} · ${item.qrToken.substring(0, 8)}…'),
              trailing: IconButton(
                tooltip: 'Удалить',
                onPressed: () => _remove(item),
                icon: const Icon(Icons.delete_outline),
              ),
            ),
          ),
        ),
      ],
    );
  }
}
