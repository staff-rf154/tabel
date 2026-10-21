import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/date_symbol_data_local.dart';

import 'api.dart';
import 'employee/employee_home.dart';
import 'login_screen.dart';
import 'management/management_home.dart';
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
  MobileBootstrap? _config;

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
      final config = await _resolveConfig(me);
      if (!mounted) return;
      setState(() {
        _me = me;
        _config = config;
        _stage = _Stage.home;
      });
    } on ApiException catch (error) {
      if (error.status == 401 || error.status == 403) {
        await widget.storage.clearSession();
      }
      if (mounted) setState(() => _stage = _Stage.login);
    }
  }

  Future<MobileBootstrap?> _readCachedConfig(String role) async {
    final raw = await widget.storage.readBootstrap();
    if (raw == null || raw.isEmpty) return null;
    try {
      final decoded = jsonDecode(raw);
      if (decoded is! Map) return null;
      final config =
          MobileBootstrap.fromJson(Map<String, dynamic>.from(decoded));
      if (config.role != role || config.shell.isEmpty) return null;
      return config;
    } catch (_) {
      return null;
    }
  }

  Future<MobileBootstrap> _resolveConfig(Me me, {bool refresh = true}) async {
    final cached = await _readCachedConfig(me.role);
    if (!refresh && cached != null) return cached;
    try {
      final fresh = await widget.api.bootstrap();
      await widget.storage.saveBootstrap(jsonEncode({
        'version': fresh.version,
        'config_version': fresh.configVersion,
        'role': fresh.role,
        'role_title': fresh.roleTitle,
        'shell': fresh.shell,
        'features': fresh.features,
        'navigation': fresh.navigation
            .map((item) =>
                {'key': item.key, 'title': item.title, 'icon': item.icon})
            .toList(),
        'cache_until_logout': fresh.cacheUntilLogout,
        'policy': {
          'cache_until_logout': fresh.cacheUntilLogout,
          'refresh_after_sec': fresh.refreshAfterSec,
        },
      }));
      return fresh;
    } on ApiException {
      if (cached != null) return cached;
      rethrow;
    }
  }

  Future<void> _acceptConsent() async {
    await widget.storage.setConsent(true);
    await _trySession();
  }

  Future<void> _loggedIn() async {
    try {
      final me = await widget.api.me();
      final config = await _resolveConfig(me);
      if (!mounted) return;
      setState(() {
        _me = me;
        _config = config;
        _stage = _Stage.home;
      });
    } catch (_) {
      await widget.api.logout();
      rethrow;
    }
  }

  Future<void> _logout() async {
    await widget.api.logout();
    await widget.storage.clearSession();
    if (!mounted) return;
    setState(() {
      _me = null;
      _config = null;
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
        final config = _config!;
        switch (config.shell) {
          case 'employee':
            return EmployeeHome(
                api: widget.api, me: me, bootstrap: config, onLogout: _logout);
          case 'operator':
            return OperatorHome(
              api: widget.api,
              me: me,
              storage: widget.storage,
              bootstrap: config,
              onLogout: _logout,
            );
          case 'management':
            return ManagementHome(
                api: widget.api, me: me, bootstrap: config, onLogout: _logout);
          default:
            return _UnsupportedRoleScreen(
                me: me, bootstrap: config, onLogout: _logout);
        }
    }
  }
}

class _UnsupportedRoleScreen extends StatelessWidget {
  const _UnsupportedRoleScreen(
      {required this.me, required this.bootstrap, required this.onLogout});

  final Me me;
  final MobileBootstrap bootstrap;
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
            Text(
                bootstrap.roleTitle.isEmpty
                    ? me.roleTitle
                    : bootstrap.roleTitle,
                style: Theme.of(context).textTheme.titleLarge),
            const SizedBox(height: 12),
            const Text(
                'Для этой роли мобильный интерфейс пока не настроен администратором.'),
            const Spacer(),
            const AppVersionLabel(),
            const SizedBox(height: 12),
            SizedBox(
              width: double.infinity,
              child:
                  FilledButton(onPressed: onLogout, child: const Text('Выйти')),
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
            Text('Перед началом работы',
                style: Theme.of(context).textTheme.headlineSmall),
            const SizedBox(height: 16),
            const Text(
                'Для учёта рабочего времени приложение обрабатывает персональные данные и, при отметке, геолокацию.'),
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
