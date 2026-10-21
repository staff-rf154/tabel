import 'dart:async';
import 'dart:convert';
import 'dart:typed_data';

import 'package:http/http.dart' as http;
import 'package:uuid/uuid.dart';

import 'models.dart';
import 'storage.dart';

class ApiException implements Exception {
  ApiException(this.status, this.message);

  final int status;
  final String message;

  @override
  String toString() => message;
}

class ApiClient {
  ApiClient({required this.storage, String? baseUrl})
      : _baseUrl = normalizeBaseUrl(baseUrl ?? defaultBaseUrl);

  static const defaultBaseUrl = 'https://srv.staff-rf.ru/api';

  static String normalizeBaseUrl(String value) {
    var normalized = value.trim().replaceAll(RegExp(r'/+$'), '');
    if (normalized.isEmpty) return defaultBaseUrl;
    final uri = Uri.tryParse(normalized);
    if (uri == null || !uri.hasScheme || uri.host.isEmpty) return normalized;
    final localHosts = {'localhost', '127.0.0.1', '10.0.2.2', '::1'};
    final defaultPort = !uri.hasPort || uri.port == 80 || uri.port == 443;
    if ((uri.path.isEmpty || uri.path == '/') &&
        defaultPort &&
        !localHosts.contains(uri.host)) {
      normalized = '$normalized/api';
    }
    return normalized;
  }

  final AppStorage storage;
  final http.Client _client = http.Client();
  String _baseUrl;

  String get baseUrl => _baseUrl;

  Future<void> loadBaseUrl() async {
    final saved = await storage.readBaseUrl();
    if (saved != null && saved.isNotEmpty) {
      _baseUrl = normalizeBaseUrl(saved);
      if (_baseUrl != saved) {
        await storage.saveBaseUrl(_baseUrl);
      }
    }
  }

  Future<void> setBaseUrl(String value) async {
    _baseUrl = normalizeBaseUrl(value);
    await storage.saveBaseUrl(_baseUrl);
  }

  Future<void> login(String login, String password) async {
    final response = await _send(
      'POST',
      '/auth/login',
      body: {'login': login, 'password': password},
      authenticated: false,
      retry: false,
    );
    final body = _asMap(response);
    final access = body['access_token'] as String?;
    final refresh = body['refresh_token'] as String?;
    if (access == null || refresh == null) {
      throw ApiException(500, 'сервер не вернул токены');
    }
    await storage.saveTokens(access, refresh);
  }

  Future<void> logout() async {
    try {
      await _send('POST', '/auth/logout', body: const {}, retry: false);
    } on ApiException {
      // ignore: локальный выход выполняется в любом случае
    } finally {
      await storage.clearSession();
    }
  }

  Future<Me> me() async {
    final body = await _get('/me');
    return Me.fromJson(_asMap(body));
  }

  Future<MobileBootstrap> bootstrap() async {
    final body = await _get('/mobile/bootstrap');
    return MobileBootstrap.fromJson(_asMap(body));
  }

  Future<MonthlyStats> monthlyStats() async {
    final body = await _get('/me/monthly-stats');
    return MonthlyStats.fromJson(_asMap(body));
  }

  Future<List<AdminUser>> adminUsers(
      {String query = '', int limit = 200}) async {
    final body = await _get('/admin/users', query: {
      'q': query,
      'limit': '$limit',
    });
    return _asList(body)
        .map((item) => AdminUser.fromJson(_asMap(item)))
        .toList();
  }

  Future<List<ReportSummaryItem>> reportSummary(
      {required DateTime from, required DateTime to}) async {
    final body = await _get('/reports/summary', query: {
      'from': from.toUtc().toIso8601String(),
      'to': to.toUtc().toIso8601String(),
    });
    return _asList(body)
        .map((item) => ReportSummaryItem.fromJson(_asMap(item)))
        .toList();
  }

  Future<List<MarkItem>> marks(
      {required DateTime from, required DateTime to}) async {
    final body = await _get('/marks', query: {
      'from': from.toUtc().toIso8601String(),
      'to': to.toUtc().toIso8601String(),
      'limit': '200',
    });
    final list = _asList(body);
    return list.map((item) => MarkItem.fromJson(_asMap(item))).toList();
  }

  Future<MyQR> myQR() async {
    final body = await _get('/me/qr');
    return MyQR.fromJson(_asMap(body));
  }

  Future<MyQR> rotateMyQR() async {
    final body = await _send('POST', '/me/qr/rotate', body: const {});
    return MyQR.fromJson(_asMap(body));
  }

  Future<Uint8List> myQRImage() async {
    final access = await storage.readAccess();
    final uri = Uri.parse('$_baseUrl/me/qr.png');
    final response = await _client.get(uri, headers: {
      if (access != null && access.isNotEmpty)
        'Authorization': 'Bearer $access',
    });
    if (response.statusCode == 401 && await _refresh()) {
      return myQRImage();
    }
    if (response.statusCode != 200) {
      throw ApiException(response.statusCode, 'не удалось загрузить QR-код');
    }
    return response.bodyBytes;
  }

  Future<QRVerification> verifyQr(String token) async {
    final body = await _get('/qr/${Uri.encodeComponent(token)}');
    return QRVerification.fromJson(_asMap(body));
  }

  Future<MarkItem> createMark({
    required String qrToken,
    required String clientType,
    String? eventType,
    double? lat,
    double? lon,
    double? accuracy,
    int? locationAgeSec,
    String? deviceOs,
    String? deviceOsVersion,
    String? deviceModel,
    String? appVersion,
    bool mockLocation = false,
    DateTime? timestampClient,
    String? idempotencyKey,
  }) async {
    final body = await _send(
      'POST',
      '/mark',
      headers: {'Idempotency-Key': idempotencyKey ?? const Uuid().v4()},
      body: {
        'qr_token': qrToken,
        'client_type': clientType,
        if (eventType != null) 'event_type': eventType,
        if (lat != null) 'lat': lat,
        if (lon != null) 'lon': lon,
        if (accuracy != null) 'accuracy_m': accuracy,
        if (locationAgeSec != null) 'location_age_sec': locationAgeSec,
        if (timestampClient != null)
          'timestamp_client': timestampClient.toUtc().toIso8601String(),
        if (deviceOs != null) 'device_os': deviceOs,
        if (deviceOsVersion != null) 'device_os_version': deviceOsVersion,
        if (deviceModel != null) 'device_model': deviceModel,
        if (appVersion != null) 'app_version': appVersion,
        'mock_location': mockLocation,
      },
    );
    return MarkItem.fromJson(_asMap(body));
  }

  Future<List<DelegationItem>> delegations() async {
    final body = await _get('/delegations');
    return _asList(body)
        .map((item) => DelegationItem.fromJson(_asMap(item)))
        .toList();
  }

  Future<void> createDelegation(
      {required String toLogin, required DateTime validUntil}) async {
    await _send('POST', '/delegations', body: {
      'to_login': toLogin,
      'valid_until': validUntil.toUtc().toIso8601String(),
    });
  }

  Future<dynamic> _get(String path, {Map<String, String>? query}) {
    return _send('GET', path, query: query);
  }

  Future<dynamic> _send(
    String method,
    String path, {
    Map<String, String>? query,
    Object? body,
    Map<String, String>? headers,
    bool authenticated = true,
    bool retry = true,
  }) async {
    final uri = Uri.parse('$_baseUrl$path').replace(queryParameters: query);
    final request = http.Request(method, uri);
    request.headers['Accept'] = 'application/json';
    request.headers.addAll(headers ?? const {});
    if (body != null) {
      request.headers['Content-Type'] = 'application/json';
      request.body = jsonEncode(body);
    }
    if (authenticated) {
      final access = await storage.readAccess();
      if (access != null && access.isNotEmpty) {
        request.headers['Authorization'] = 'Bearer $access';
      }
    }

    late http.Response response;
    try {
      final streamed =
          await _client.send(request).timeout(const Duration(seconds: 25));
      response = await http.Response.fromStream(streamed);
    } on TimeoutException {
      throw ApiException(0, 'превышено время ожидания ответа сервера');
    } catch (_) {
      throw ApiException(0, 'нет связи с сервером');
    }

    if (response.statusCode == 401 && authenticated && retry) {
      final refreshed = await _refresh();
      if (refreshed) {
        return _send(method, path,
            query: query,
            body: body,
            headers: headers,
            authenticated: authenticated,
            retry: false);
      }
    }

    final decoded = _decode(response.body);
    if (response.statusCode >= 200 && response.statusCode < 300) {
      return decoded;
    }
    final message =
        decoded is Map<String, dynamic> ? decoded['error'] as String? : null;
    throw ApiException(response.statusCode,
        message ?? 'ошибка сервера (HTTP ${response.statusCode})');
  }

  Future<bool> _refresh() async {
    final refresh = await storage.readRefresh();
    if (refresh == null || refresh.isEmpty) {
      return false;
    }
    try {
      final body = await _send(
        'POST',
        '/auth/refresh',
        body: {'refresh_token': refresh},
        authenticated: false,
        retry: false,
      );
      final map = _asMap(body);
      final access = map['access_token'] as String?;
      final nextRefresh = map['refresh_token'] as String?;
      if (access == null || nextRefresh == null) {
        return false;
      }
      await storage.saveTokens(access, nextRefresh);
      return true;
    } on ApiException {
      await storage.clearSession();
      return false;
    }
  }

  dynamic _decode(String raw) {
    if (raw.isEmpty) {
      return null;
    }
    try {
      return jsonDecode(raw);
    } catch (_) {
      return raw;
    }
  }

  Map<String, dynamic> _asMap(dynamic value) {
    if (value is Map<String, dynamic>) {
      return value;
    }
    if (value is Map) {
      return Map<String, dynamic>.from(value);
    }
    throw ApiException(500, 'неожиданный ответ сервера');
  }

  List<dynamic> _asList(dynamic value) {
    if (value is List) {
      return value;
    }
    throw ApiException(500, 'неожиданный ответ сервера');
  }
}
