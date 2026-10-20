import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/date_symbol_data_local.dart';

import 'api.dart';
import 'employee/employee_home.dart';
import 'login_screen.dart';
import 'models.dart';
import 'operator/operator_home.dart';
import 'storage.dart';
import 'version_label.dart';

Future<void> main() async {
  WidgetsFlutterBinding.ensureInitialized();
  await initializeDateFormatting('ru');
  final storage = AppStorage();
  final api = ApiClient(storage: storage);
  runApp(TabelApp(storage: storage, api: api));
}

class TabelApp extends StatelessWidget {
  const TabelApp({super.key, required this.storage, required this.api});

  final AppStorage storage;
  final ApiClient api;

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'Табель',
      debugShowCheckedModeBanner: false,
      locale: const Locale('ru'),
      supportedLocales: const [Locale('ru')],
      localizationsDelegates: const [
        GlobalMaterialLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
      ],
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(seedColor: const Color(0xFF2563EB)),
        useMaterial3: true,
      ),
      home: RootGate(storage: storage, api: api),
    );
  }
}

enum _Stage { loading, consent, login, home }

class RootGate extends StatefulWidget {
  const RootGate({super.key, required this.storage, required this.api});

  final AppStorage storage;
  final ApiClient api;

  @override
  State<RootGate> createState() => _RootGateState();
}

class _RootGateState extends State<RootGate> {
  _Stage _stage = _Stage.loading;
  Me? _me;

  @override
  void initState() {
    super.initState();
    _bootstrap();
  }

  Future<void> _bootstrap() async {
    await widget.api.loadBaseUrl();
    final consent = await widget.storage.hasConsent();
    if (!consent) {
      if (mounted) setState(() => _stage = _Stage.consent);
      return;
    }
    await _trySession();
  }

  Future<void> _trySession() async {
    final access = await widget.storage.readAccess();
    if (access == null || access.isEmpty) {
      if (mounted) setState(() => _stage = _Stage.login);
      return;
    }
    try {
      final me = await widget.api.me();
      if (!mounted) return;
      setState(() {
        _me = me;
        _stage = _Stage.home;
      });
    } on ApiException {
      await widget.storage.clearTokens();
      if (mounted) setState(() => _stage = _Stage.login);
    }
  }

  Future<void> _acceptConsent() async {
    await widget.storage.setConsent(true);
    await _trySession();
  }

  Future<void> _loggedIn() async {
    final me = await widget.api.me();
    if (!mounted) return;
    setState(() {
      _me = me;
      _stage = _Stage.home;
    });
  }

  Future<void> _logout() async {
    await widget.api.logout();
    if (!mounted) return;
    setState(() {
      _me = null;
      _stage = _Stage.login;
    });
  }

  @override
  Widget build(BuildContext context) {
    switch (_stage) {
      case _Stage.loading:
        return const Scaffold(body: Center(child: CircularProgressIndicator()));
      case _Stage.consent:
        return ConsentScreen(onAccept: _acceptConsent);
      case _Stage.login:
        return LoginScreen(api: widget.api, onLoggedIn: _loggedIn);
      case _Stage.home:
        final me = _me!;
        if (me.role == 'employee') {
          return EmployeeHome(api: widget.api, me: me, onLogout: _logout);
        }
        if (me.role == 'operator') {
          return OperatorHome(api: widget.api, me: me, storage: widget.storage, onLogout: _logout);
        }
        return _WebOnlyScreen(me: me, onLogout: _logout);
    }
  }
}

class _WebOnlyScreen extends StatelessWidget {
  const _WebOnlyScreen({required this.me, required this.onLogout});

  final Me me;
  final Future<void> Function() onLogout;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(title: const Text('Табель')),
      body: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Роль: ${me.roleTitle}', style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 12),
            const Text('Для этой роли работа с отчётами доступна в веб-приложении отчётов.'),
            const Spacer(),
            const AppVersionLabel(),
            const SizedBox(height: 12),
            SizedBox(
              width: double.infinity,
              child: FilledButton(onPressed: onLogout, child: const Text('Выйти')),
            ),
          ],
        ),
      ),
    );
  }
}

class ConsentScreen extends StatefulWidget {
  const ConsentScreen({super.key, required this.onAccept});

  final Future<void> Function() onAccept;

  @override
  State<ConsentScreen> createState() => _ConsentScreenState();
}

class _ConsentScreenState extends State<ConsentScreen> {
  bool _pdn = false;
  bool _geo = false;

  @override
  Widget build(BuildContext context) {
    final ready = _pdn && _geo;
    return Scaffold(
      appBar: AppBar(title: const Text('Согласие')),
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.all(24),
          children: [
            Text('Перед началом работы', style: Theme.of(context).textTheme.headlineSmall),
            const SizedBox(height: 16),
            const Text('Для учёта рабочего времени приложение обрабатывает персональные данные и, при отметке, геолокацию.'),
            const SizedBox(height: 16),
            CheckboxListTile(
              value: _pdn,
              onChanged: (value) => setState(() => _pdn = value ?? false),
              title: const Text('Согласен на обработку персональных данных'),
              controlAffinity: ListTileControlAffinity.leading,
            ),
            CheckboxListTile(
              value: _geo,
              onChanged: (value) => setState(() => _geo = value ?? false),
              title: const Text('Согласен на обработку геолокации при отметке'),
              controlAffinity: ListTileControlAffinity.leading,
            ),
            const SizedBox(height: 16),
            FilledButton(
              onPressed: ready ? () => widget.onAccept() : null,
              child: const Text('Принять и продолжить'),
            ),
            const SizedBox(height: 20),
            const AppVersionLabel(),
          ],
        ),
      ),
    );
  }
}
