import 'package:flutter/material.dart';

import '../api.dart';
import '../models.dart';
import '../storage.dart';
import '../version_label.dart';
import 'delegate_screen.dart';
import 'offline_queue.dart';
import 'queue_screen.dart';
import 'scan_screen.dart';

class OperatorHome extends StatefulWidget {
  const OperatorHome({
    super.key,
    required this.api,
    required this.me,
    required this.storage,
    required this.bootstrap,
    required this.onLogout,
  });

  final ApiClient api;
  final Me me;
  final AppStorage storage;
  final MobileBootstrap bootstrap;
  final Future<void> Function() onLogout;

  @override
  State<OperatorHome> createState() => _OperatorHomeState();
}

class _OperatorHomeState extends State<OperatorHome> {
  late final OfflineQueue _queue = OfflineQueue(widget.storage);
  late final List<MobileNavItem> _tabs = _buildTabs();
  int _index = 0;

  static const _knownTabs = {'scan', 'queue', 'delegations', 'profile'};

  List<MobileNavItem> _buildTabs() {
    final items = widget.bootstrap.navigation
        .where((item) => _knownTabs.contains(item.key))
        .toList();
    if (items.isNotEmpty) return items;
    return const [
      MobileNavItem(key: 'scan', title: 'Скан', icon: 'qr_code_scanner'),
      MobileNavItem(key: 'queue', title: 'Очередь', icon: 'cloud_queue'),
      MobileNavItem(key: 'delegations', title: 'Права', icon: 'swap_horiz'),
      MobileNavItem(key: 'profile', title: 'Профиль', icon: 'person'),
    ];
  }

  @override
  void initState() {
    super.initState();
    _queue.all();
  }

  @override
  Widget build(BuildContext context) {
    if (_tabs.isEmpty) {
      return Scaffold(
        appBar: AppBar(title: const Text('Табель')),
        body: const Center(child: Text('Интерфейс оператора не настроен')),
      );
    }
    final current = _tabs[_index < _tabs.length ? _index : _tabs.length - 1];
    return Scaffold(
      appBar: AppBar(title: Text(current.title)),
      body: IndexedStack(
        index: _index,
        children: _tabs.map(_buildScreen).toList(),
      ),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: (value) => setState(() => _index = value),
        destinations: _tabs.map((item) {
          return NavigationDestination(
            icon: item.key == 'queue'
                ? ValueListenableBuilder<int>(
                    valueListenable: _queue.count,
                    builder: (context, count, _) => Badge(
                      isLabelVisible: count > 0,
                      label: Text('$count'),
                      child: Icon(_icon(item.key)),
                    ),
                  )
                : Icon(_icon(item.key)),
            label: item.title,
          );
        }).toList(),
      ),
    );
  }

  Widget _buildScreen(MobileNavItem item) {
    switch (item.key) {
      case 'scan':
        return ScanScreen(api: widget.api, queue: _queue);
      case 'queue':
        return QueueScreen(api: widget.api, queue: _queue);
      case 'delegations':
        return DelegateScreen(api: widget.api);
      case 'profile':
        return _ProfileTab(
            me: widget.me,
            bootstrap: widget.bootstrap,
            onLogout: widget.onLogout);
      default:
        return Center(child: Text('Раздел «${item.title}» недоступен'));
    }
  }

  IconData _icon(String key) => switch (key) {
        'scan' => Icons.qr_code_scanner,
        'queue' => Icons.cloud_queue,
        'delegations' => Icons.swap_horiz,
        'profile' => Icons.person,
        _ => Icons.apps,
      };
}

class _ProfileTab extends StatelessWidget {
  const _ProfileTab(
      {required this.me, required this.bootstrap, required this.onLogout});

  final Me me;
  final MobileBootstrap bootstrap;
  final Future<void> Function() onLogout;

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Card(
          child: ListTile(
            leading: const Icon(Icons.person),
            title: Text(me.fullName ?? me.login),
            subtitle: Text(bootstrap.roleTitle.isEmpty
                ? me.roleTitle
                : bootstrap.roleTitle),
          ),
        ),
        if (me.employeeNumber != null)
          ListTile(
              title: const Text('Табельный номер'),
              subtitle: Text(me.employeeNumber!)),
        const SizedBox(height: 8),
        const Padding(
          padding: EdgeInsets.symmetric(horizontal: 16),
          child: AppVersionLabel(align: TextAlign.left),
        ),
        const SizedBox(height: 24),
        FilledButton.icon(
          onPressed: onLogout,
          icon: const Icon(Icons.logout),
          label: const Text('Выйти'),
        ),
      ],
    );
  }
}
