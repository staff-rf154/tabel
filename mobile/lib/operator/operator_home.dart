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
    required this.onLogout,
  });

  final ApiClient api;
  final Me me;
  final AppStorage storage;
  final Future<void> Function() onLogout;

  @override
  State<OperatorHome> createState() => _OperatorHomeState();
}

class _OperatorHomeState extends State<OperatorHome> {
  late final OfflineQueue _queue = OfflineQueue(widget.storage);
  int _index = 0;

  static const _titles = ['Сканирование', 'Очередь', 'Передать права', 'Профиль'];

  @override
  void initState() {
    super.initState();
    _queue.all();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: Text(_titles[_index])),
      body: IndexedStack(
        index: _index,
        children: [
          ScanScreen(api: widget.api, queue: _queue),
          QueueScreen(api: widget.api, queue: _queue),
          DelegateScreen(api: widget.api),
          _ProfileTab(me: widget.me, onLogout: widget.onLogout),
        ],
      ),
      bottomNavigationBar: NavigationBar(
        selectedIndex: _index,
        onDestinationSelected: (value) => setState(() => _index = value),
        destinations: [
          const NavigationDestination(icon: Icon(Icons.qr_code_scanner), label: 'Скан'),
          NavigationDestination(
            icon: ValueListenableBuilder<int>(
              valueListenable: _queue.count,
              builder: (context, count, _) => Badge(
                isLabelVisible: count > 0,
                label: Text('$count'),
                child: const Icon(Icons.cloud_queue),
              ),
            ),
            label: 'Очередь',
          ),
          const NavigationDestination(icon: Icon(Icons.swap_horiz), label: 'Права'),
          const NavigationDestination(icon: Icon(Icons.person), label: 'Профиль'),
        ],
      ),
    );
  }
}

class _ProfileTab extends StatelessWidget {
  const _ProfileTab({required this.me, required this.onLogout});

  final Me me;
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
            subtitle: Text(me.roleTitle),
          ),
        ),
        if (me.employeeNumber != null)
          ListTile(title: const Text('Табельный номер'), subtitle: Text(me.employeeNumber!)),
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
