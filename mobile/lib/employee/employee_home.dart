import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:intl/intl.dart';

import '../api.dart';
import '../version_label.dart';
import '../models.dart';

class EmployeeHome extends StatefulWidget {
  const EmployeeHome({
    super.key,
    required this.api,
    required this.me,
    required this.bootstrap,
    required this.onLogout,
  });

  final ApiClient api;
  final Me me;
  final MobileBootstrap bootstrap;
  final Future<void> Function() onLogout;

  @override
  State<EmployeeHome> createState() => _EmployeeHomeState();
}

class _EmployeeHomeState extends State<EmployeeHome> {
  bool _loading = true;
  String? _error;
  MonthlyStats? _stats;
  MyQR? _qr;
  Uint8List? _qrPng;
  bool _qrLoading = true;
  bool _qrRotating = false;
  String? _qrError;
  final _format = DateFormat('dd.MM.yyyy HH:mm', 'ru');
  final _monthFormat = DateFormat('LLLL yyyy', 'ru');

  @override
  void initState() {
    super.initState();
    _load();
    if (widget.bootstrap.feature('my_qr')) {
      _loadQR();
    } else {
      _qrLoading = false;
    }
  }

  Future<void> _refreshAll() async {
    await Future.wait([
      _load(),
      if (widget.bootstrap.feature('my_qr')) _loadQR(),
    ]);
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
        content: const Text(
            'Старый QR-код сразу перестанет действовать. Показывайте оператору только новый код.'),
        actions: [
          TextButton(
              onPressed: () => Navigator.pop(context, false),
              child: const Text('Отмена')),
          FilledButton(
              onPressed: () => Navigator.pop(context, true),
              child: const Text('Сменить')),
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
        const SnackBar(
            content: Text('QR-код изменён. Старый код больше не действует.')),
      );
    } on ApiException catch (error) {
      if (!mounted) return;
      setState(() => _qrRotating = false);
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text(error.message)));
    }
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    try {
      final stats = await widget.api.monthlyStats();
      if (!mounted) return;
      setState(() {
        _stats = stats;
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
    final showQR = widget.bootstrap.feature('my_qr');
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
            if (showQR) ...[
              _qrCard(context),
              const SizedBox(height: 16),
            ],
            _profileCard(context),
            if (widget.bootstrap.feature('monthly_stats')) ...[
              const SizedBox(height: 16),
              _statsCard(context),
              const SizedBox(height: 16),
              _recentCard(context),
            ],
          ],
        ),
      ),
    );
  }

  Widget _statsCard(BuildContext context) {
    final stats = _stats;
    final month = stats == null
        ? ''
        : _capitalize(_monthFormat.format(DateTime.parse('${stats.month}-01')));
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    month.isEmpty
                        ? 'Отработано за месяц'
                        : 'Отработано за $month',
                    style: Theme.of(context).textTheme.titleLarge,
                  ),
                ),
                IconButton(
                  tooltip: 'Обновить',
                  onPressed: _loading ? null : _load,
                  icon: const Icon(Icons.refresh),
                ),
              ],
            ),
            if (_loading)
              const Center(
                child: Padding(
                    padding: EdgeInsets.all(24),
                    child: CircularProgressIndicator()),
              )
            else if (_error != null) ...[
              Text(_error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error)),
              TextButton(onPressed: _load, child: const Text('Повторить')),
            ] else if (stats != null) ...[
              Row(
                children: [
                  Expanded(
                      child: _StatTile(
                          label: 'Дней', value: '${stats.daysWorked}')),
                  Expanded(
                      child: _StatTile(
                          label: 'Часов', value: _hours(stats.workedMinutes))),
                  Expanded(
                      child: _StatTile(
                          label: 'Смен', value: '${stats.shiftsTotal}')),
                ],
              ),
              const SizedBox(height: 8),
              Text('Всего: ${_duration(stats.workedMinutes)}'),
              if (stats.overtimeShifts > 0)
                Text('Переработок: ${stats.overtimeShifts}'),
              if (stats.shiftsTotal > stats.shiftsClosed)
                const Text('Текущая смена ещё не закрыта'),
            ],
          ],
        ),
      ),
    );
  }

  Widget _recentCard(BuildContext context) {
    final stats = _stats;
    final marks = stats?.lastCheckIns ?? const <MarkItem>[];
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Последние входы за месяц',
                style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 8),
            if (!_loading && _error == null && marks.isEmpty)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 16),
                child: Text('Входов за календарный месяц нет'),
              ),
            ...marks.map(
              (mark) => ListTile(
                contentPadding: EdgeInsets.zero,
                leading: const Icon(Icons.login, color: Colors.green),
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
              const Center(
                  child: Padding(
                      padding: EdgeInsets.all(24),
                      child: CircularProgressIndicator()))
            else if (_qrError != null) ...[
              Text(_qrError!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error)),
              TextButton(onPressed: _loadQR, child: const Text('Повторить')),
            ] else if (qr != null) ...[
              Center(
                child: _qrPng == null
                    ? const Text('Изображение недоступно')
                    : Image.memory(_qrPng!,
                        width: 260, height: 260, gaplessPlayback: true),
              ),
              const SizedBox(height: 8),
              Text(qr.validUntil == null
                  ? 'Бессрочный QR-код'
                  : 'Действует до ${qr.validUntil}'),
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
            Text(me.fullName ?? me.login,
                style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 8),
            Text(
                'Роль: ${widget.bootstrap.roleTitle.isEmpty ? me.roleTitle : widget.bootstrap.roleTitle}'),
            if (me.position != null) Text('Должность: ${me.position}'),
            if (me.employeeNumber != null)
              Text('Табельный номер: ${me.employeeNumber}'),
            const SizedBox(height: 12),
            const AppVersionLabel(align: TextAlign.left),
          ],
        ),
      ),
    );
  }

  String _capitalize(String value) {
    if (value.isEmpty) return value;
    return '${value[0].toUpperCase()}${value.substring(1)}';
  }

  String _hours(int minutes) {
    final hours = minutes / 60;
    if (hours == 0) return '0';
    return hours.toStringAsFixed(hours >= 10 ? 0 : 1);
  }

  String _duration(int minutes) {
    final hours = minutes ~/ 60;
    final rest = minutes % 60;
    return '$hours ч $rest мин';
  }
}

class _StatTile extends StatelessWidget {
  const _StatTile({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        Text(value, style: Theme.of(context).textTheme.headlineSmall),
        Text(label, style: Theme.of(context).textTheme.bodySmall),
      ],
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
