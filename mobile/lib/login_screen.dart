import 'package:flutter/material.dart';

import 'api.dart';
import 'version_label.dart';

class LoginScreen extends StatefulWidget {
  const LoginScreen({super.key, required this.api, required this.onLoggedIn});

  final ApiClient api;
  final Future<void> Function() onLoggedIn;

  @override
  State<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends State<LoginScreen> {
  final _loginController = TextEditingController();
  final _passwordController = TextEditingController();
  final _baseUrlController = TextEditingController();

  bool _busy = false;
  bool _showSettings = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _baseUrlController.text = widget.api.baseUrl;
  }

  @override
  void dispose() {
    _loginController.dispose();
    _passwordController.dispose();
    _baseUrlController.dispose();
    super.dispose();
  }

  Future<void> _submit() async {
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await widget.api.setBaseUrl(_baseUrlController.text);
      await widget.api
          .login(_loginController.text.trim(), _passwordController.text);
      await widget.onLoggedIn();
    } on ApiException catch (error) {
      final hint = error.status == 405
          ? ' Проверьте адрес API, например https://srv.staff-rf.ru/api'
          : '';
      setState(() => _error = '${error.message}$hint');
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Табель — вход')),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            TextField(
              controller: _loginController,
              autofillHints: const [AutofillHints.username],
              decoration: const InputDecoration(
                  labelText: 'Логин', border: OutlineInputBorder()),
            ),
            const SizedBox(height: 16),
            TextField(
              controller: _passwordController,
              obscureText: true,
              autofillHints: const [AutofillHints.password],
              onSubmitted: (_) => _busy ? null : _submit(),
              decoration: const InputDecoration(
                  labelText: 'Пароль', border: OutlineInputBorder()),
            ),
            if (_error != null) ...[
              const SizedBox(height: 12),
              Text(_error!,
                  style: TextStyle(color: Theme.of(context).colorScheme.error)),
            ],
            const SizedBox(height: 20),
            FilledButton(
              onPressed: _busy ? null : _submit,
              child: Text(_busy ? 'Вход…' : 'Войти'),
            ),
            const SizedBox(height: 24),
            TextButton(
              onPressed: () => setState(() => _showSettings = !_showSettings),
              child: const Text('Настройки подключения'),
            ),
            if (_showSettings)
              TextField(
                controller: _baseUrlController,
                keyboardType: TextInputType.url,
                decoration: const InputDecoration(
                  labelText: 'Адрес сервера',
                  helperText: 'Например, https://srv.staff-rf.ru/api',
                  border: OutlineInputBorder(),
                ),
              ),
            const SizedBox(height: 28),
            const AppVersionLabel(),
          ],
        ),
      ),
    );
  }
}
