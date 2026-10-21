import 'package:flutter/material.dart';
import 'package:intl/intl.dart';

import '../api.dart';
import '../models.dart';
import '../version_label.dart';
import 'admin_users_screen.dart';

class ManagementHome extends StatefulWidget {
  const ManagementHome({
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
  State<ManagementHome> createState() => _ManagementHomeState();
}

class _ManagementHomeState extends State<ManagementHome> {
  bool _loading = true;
  String? _error;
  List<ReportSummaryItem> _items = const [];
  final _monthFormat = DateFormat('LLLL yyyy', 'ru');

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    final now = DateTime.now();
    final from = DateTime(now.year, now.month, 1);
    try {
      final items = await widget.api.reportSummary(from: from, to: now);
      if (!mounted) return;
      setState(() {
        _items = items;
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
    final now = DateTime.now();
    final month = _capitalize(_monthFormat.format(now));
    return Scaffold(
      appBar: AppBar(
        title: const Text('Сводка'),
        actions: [
          IconButton(
            tooltip: 'Выйти',
            onPressed: widget.onLogout,
            icon: const Icon(Icons.logout),
          ),
        ],
      ),
      body: RefreshIndicator(
        onRefresh: _load,
        child: ListView(
          padding: const EdgeInsets.all(16),
          children: [
            _profileCard(context),
            if (widget.bootstrap.feature('reports_summary')) ...[
              const SizedBox(height: 16),
              _summaryCard(context, month),
              if (widget.bootstrap.feature('users_directory')) ...[
                const SizedBox(height: 16),
                Card(
                  child: ListTile(
                    leading: const Icon(Icons.group_outlined),
                    title: const Text('Пользователи'),
                    subtitle:
                        const Text('Поиск по логину, ФИО и табельному номеру'),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: _openUsers,
                  ),
                ),
              ],
              const SizedBox(height: 16),
              _topCard(context),
            ] else
              const Card(
                child: Padding(
                  padding: EdgeInsets.all(16),
                  child: Text('Раздел отчётов недоступен для этой роли.'),
                ),
              ),
          ],
        ),
      ),
    );
  }

  Future<void> _openUsers() async {
    await Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (context) => AdminUsersScreen(api: widget.api),
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
            const SizedBox(height: 12),
            const AppVersionLabel(align: TextAlign.left),
          ],
        ),
      ),
    );
  }

  Widget _summaryCard(BuildContext context, String month) {
    final active = _items
        .where((item) => item.shiftsTotal > 0 || item.marksTotal > 0)
        .length;
    final shifts = _items.fold<int>(0, (sum, item) => sum + item.shiftsTotal);
    final minutes =
        _items.fold<int>(0, (sum, item) => sum + item.workedMinutes);
    final marks = _items.fold<int>(0, (sum, item) => sum + item.marksTotal);

    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                    child: Text('Итоги за $month',
                        style: Theme.of(context).textTheme.titleLarge)),
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
            ] else ...[
              Row(
                children: [
                  Expanded(
                      child: _Metric(label: 'Сотрудников', value: '$active')),
                  Expanded(child: _Metric(label: 'Смен', value: '$shifts')),
                  Expanded(
                      child: _Metric(label: 'Часов', value: _hours(minutes))),
                ],
              ),
              const SizedBox(height: 12),
              Text('Отметок за месяц: $marks'),
              Text('Отработано всего: ${_duration(minutes)}'),
            ],
          ],
        ),
      ),
    );
  }

  Widget _topCard(BuildContext context) {
    final top = _items.where((item) => item.workedMinutes > 0).toList()
      ..sort((a, b) => b.workedMinutes.compareTo(a.workedMinutes));
    return Card(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Сотрудники по отработанному времени',
                style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 8),
            if (!_loading && _error == null && top.isEmpty)
              const Padding(
                padding: EdgeInsets.symmetric(vertical: 16),
                child: Text('За текущий месяц данных пока нет'),
              ),
            ...top.take(10).map(
                  (item) => ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: const Icon(Icons.person_outline),
                    title: Text(item.fullName),
                    subtitle: Text([
                      item.employeeNumber,
                      if (item.position != null) item.position!,
                    ].where((part) => part.isNotEmpty).join(' · ')),
                    trailing: Text(_duration(item.workedMinutes)),
                  ),
                ),
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

class _Metric extends StatelessWidget {
  const _Metric({required this.label, required this.value});

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
