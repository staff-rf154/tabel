import 'package:flutter/material.dart';
import 'package:intl/intl.dart';

import '../api.dart';
import '../models.dart';

class DelegateScreen extends StatefulWidget {
  const DelegateScreen({super.key, required this.api});

  final ApiClient api;

  @override
  State<DelegateScreen> createState() => _DelegateScreenState();
}

class _DelegateScreenState extends State<DelegateScreen> {
  final _userController = TextEditingController();
  final _format = DateFormat('dd.MM.yyyy HH:mm', 'ru');
  List<DelegationItem> _items = const [];
  DateTime _validUntil = DateTime.now().add(const Duration(days: 1));
  bool _busy = false;
  String? _message;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void dispose() {
    _userController.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    try {
      final items = await widget.api.delegations();
      if (!mounted) return;
      setState(() => _items = items);
    } on ApiException catch (error) {
      if (!mounted) return;
      setState(() => _message = error.message);
    }
  }

  Future<void> _pickDate() async {
    final date = await showDatePicker(
      context: context,
      initialDate: _validUntil,
      firstDate: DateTime.now(),
      lastDate: DateTime.now().add(const Duration(days: 90)),
    );
    if (date == null) return;
    if (!mounted) return;
    final time = await showTimePicker(
      context: context,
      initialTime: TimeOfDay.fromDateTime(_validUntil),
    );
    if (!mounted) return;
    setState(() {
      _validUntil = DateTime(date.year, date.month, date.day, time?.hour ?? 23,
          time?.minute ?? 59);
    });
  }

  Future<void> _submit() async {
    setState(() {
      _busy = true;
      _message = null;
    });
    try {
      await widget.api.createDelegation(
        toLogin: _userController.text.trim(),
        validUntil: _validUntil,
      );
      _userController.clear();
      await _load();
      if (!mounted) return;
      setState(() => _message = 'Запрос на передачу прав отправлен');
    } on ApiException catch (error) {
      if (!mounted) return;
      setState(() => _message = error.message);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        const Text(
            'Передача прав оператора сотруднику. Укажите логин сотрудника; запрос подтверждает администратор.'),
        const SizedBox(height: 16),
        TextField(
          controller: _userController,
          decoration: const InputDecoration(
            labelText: 'Логин сотрудника',
            border: OutlineInputBorder(),
          ),
        ),
        const SizedBox(height: 12),
        OutlinedButton.icon(
          onPressed: _pickDate,
          icon: const Icon(Icons.event),
          label: Text('Действует до: ${_format.format(_validUntil)}'),
        ),
        const SizedBox(height: 12),
        FilledButton(
          onPressed: _busy ? null : _submit,
          child: Text(_busy ? 'Отправка…' : 'Передать права'),
        ),
        if (_message != null)
          Padding(
            padding: const EdgeInsets.symmetric(vertical: 12),
            child: Text(_message!),
          ),
        const Divider(height: 32),
        Text('Мои делегирования',
            style: Theme.of(context).textTheme.titleMedium),
        if (_items.isEmpty)
          const Padding(
            padding: EdgeInsets.symmetric(vertical: 12),
            child: Text('Записей нет'),
          ),
        ..._items.map(
          (item) => Card(
            child: ListTile(
              title: Text('Статус: ${item.statusTitle}'),
              subtitle: Text([
                if (item.toLogin != null) 'Логин: ${item.toLogin}',
                'До ${_format.format(item.validUntil)}'
              ].join(' · ')),
            ),
          ),
        ),
      ],
    );
  }
}
