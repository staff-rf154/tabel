import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:intl/intl.dart';

import '../api.dart';
import '../version_label.dart';
import '../models.dart';

class EmployeeHome extends StatefulWidget {
  const EmployeeHome({super.key, required this.api, required this.me, required this.onLogout});

  final ApiClient api;
  final Me me;
  final Future<void> Function() onLogout;

  @override
  State<EmployeeHome> createState() => _EmployeeHomeState();
}

class _EmployeeHomeState extends State<EmployeeHome> {
  bool _week = false;
  bool _loading = true;
  String? _error;
  List<MarkItem> _marks = const [];
  MyQR? _qr;
  Uint8List? _qrPng;
  bool _qrLoading = true;
  bool _qrRotating = false;
  String? _qrError;
  final _format = DateFormat('dd.MM.yyyy HH:mm', 'ru');

  @override
  void initState() {
    super.initState();
    _load();
    _loadQR();
  }

  Future<void> _refreshAll() async {
    await Future.wait([_load(), _loadQR()]);
  }

  Future<void> _loadQR() async {
    setState(() {
      _qrLoading = true;
      _qrError = null;
    });
    try {
      final qr = await widget.api.myQR();
      final image = await widget.api.myQRImage();
      if (!mounted) return;
      setState(() {
        _qr = qr;
        _qrPng = image;
        _qrLoading = false;
      });
    } on ApiException catch (error) {
      if (!mounted) return;
      setState(() {
        _qrError = error.message;
        _qrLoading = false;
      });
    }
  }

  Future<void> _rotateQR() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Сменить QR-код?'),
        content: const Text('Старый QR-код сразу перестанет действовать. Показывайте оператору только новый код.'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(context, false), child: const Text('Отмена')),
          FilledButton(onPressed: () => Navigator.pop(context, true), child: const Text('Сменить')),
        ],
      ),
    );
    if (confirmed != true || !mounted) return;
    setState(() => _qrRotating = true);
    try {
      final qr = await widget.api.rotateMyQR();
      final image = await widget.api.myQRImage();
      if (!mounted) return;
      setState(() {
        _qr = qr;
        _qrPng = image;
        _qrRotating = false;
      });
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('QR-код изменён. Старый код больше не действует.')),
      );
    } on ApiException catch (error) {
      if (!mounted) return;
      setState(() => _qrRotating = false);
      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(error.message)));
    }
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    final now = DateTime.now();
    final start = _week
        ? DateTime(now.year, now.month, now.day).subtract(Duration(days: now.weekday - 1))
        : DateTime(now.year, now.month, now.day);
    try {
      final marks = await widget.api.marks(from: start, to: now);
      if (!mounted) return;
      setState(() {
        _marks = marks;
        _loading = false;
      });
    } on ApiException catch (error) {
      if (!mounted) return;
      setState(() {
        _error = error.message;
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Мои отметки'),
        actions: [
          IconButton(
            tooltip: 'Выйти',
            onPressed: widget.onLogout,
            icon: const Icon(Icons.logout),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _refreshAll,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            _qrCard(context),
            const SizedBox(height: 16),
            _profileCard(context),
            const SizedBox(height: 16),
            SegmentedButton<bool>(
              segments: const [
                ButtonSegment(value: false, label: Text('Сегодня')),
                ButtonSegment(value: true, label: Text('Неделя')),
              ],
              selected: {_week},
              onSelectionChanged: (value) {
                setState(() => _week = value.first);
                _load();
              },
            ),
            const SizedBox(height: 16),
            if (_loading) const Center(child: Padding(padding: EdgeInsets.all(24), child: CircularProgressIndicator())),
            if (_error != null)
              Padding(
                padding: const EdgeInsets.symmetric(vertical: 12),
                child: Text(_error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
              ),
            if (!_loading && _error == null && _marks.isEmpty)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 24),
                child: Text('Отметок за выбранный период нет'),
              ),
            ..._marks.map(
              (mark) => Card(
                child: ListTile(
                  leading: Icon(
                    mark.eventType == 'check_in' ? Icons.login : Icons.logout,
                    color: mark.eventType == 'check_in' ? Colors.green : Colors.orange,
                  ),
                  title: Text(mark.eventTitle),
                  subtitle: Text(_format.format(mark.timestampServer)),
                  trailing: Row(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      if (mark.overtime) const _Badge(text: 'Переработка'),
                      if (mark.outsideOffice) const _Badge(text: 'Вне офиса'),
                    ],
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _qrCard(BuildContext context) {
    final qr = _qr;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Text('Мой QR', style: Theme.of(context).textTheme.titleLarge),
                const Spacer(),
                IconButton(
                  tooltip: 'Обновить',
                  onPressed: _qrLoading ? null : _loadQR,
                  icon: const Icon(Icons.refresh),
                ),
              ],
            ),
            if (_qrLoading)
              const Center(child: Padding(padding: EdgeInsets.all(24), child: CircularProgressIndicator()))
            else if (_qrError != null) ...[
              Text(_qrError!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
              TextButton(onPressed: _loadQR, child: const Text('Повторить')),
            ] else if (qr != null) ...[
              Center(
                child: _qrPng == null
                    ? const Text('Изображение недоступно')
                    : Image.memory(_qrPng!, width: 260, height: 260, gaplessPlayback: true),
              ),
              const SizedBox(height: 8),
              Text(qr.validUntil == null ? 'Бессрочный QR-код' : 'Действует до ${qr.validUntil}'),
              const SizedBox(height: 4),
              const Text('Покажите этот QR-код оператору для отметки.'),
              const SizedBox(height: 8),
              Align(
                alignment: Alignment.centerRight,
                child: OutlinedButton.icon(
                  onPressed: _qrRotating ? null : _rotateQR,
                  icon: const Icon(Icons.autorenew),
                  label: Text(_qrRotating ? 'Смена…' : 'Сменить QR'),
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }

  Widget _profileCard(BuildContext context) {
    final me = widget.me;
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(me.fullName ?? me.login, style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 8),
            Text('Роль: ${me.roleTitle}'),
            if (me.position != null) Text('Должность: ${me.position}'),
            if (me.employeeNumber != null) Text('Табельный номер: ${me.employeeNumber}'),
            const SizedBox(height: 12),
            const AppVersionLabel(align: TextAlign.left),
          ],
        ),
      ),
    );
  }
}

class _Badge extends StatelessWidget {
  const _Badge({required this.text});

  final String text;

  @override
  Widget build(BuildContext context) {
    return Container(
      margin: const EdgeInsets.only(left: 6),
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: Theme.of(context).colorScheme.secondaryContainer,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Text(text, style: const TextStyle(fontSize: 11)),
    );
  }
}
