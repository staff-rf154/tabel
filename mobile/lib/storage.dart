import 'dart:convert';

import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class AppStorage {
  AppStorage({FlutterSecureStorage? storage})
      : _storage = storage ?? const FlutterSecureStorage();

  static const _accessKey = 'tabel_access_token';
  static const _refreshKey = 'tabel_refresh_token';
  static const _consentKey = 'tabel_pdn_consent';
  static const _baseUrlKey = 'tabel_base_url';
  static const _queueKey = 'tabel_offline_queue';
  static const _bootstrapKey = 'tabel_mobile_bootstrap';

  final FlutterSecureStorage _storage;

  Future<String?> readAccess() => _storage.read(key: _accessKey);
  Future<String?> readRefresh() => _storage.read(key: _refreshKey);

  Future<void> saveTokens(String access, String refresh) async {
    await _storage.write(key: _accessKey, value: access);
    await _storage.write(key: _refreshKey, value: refresh);
  }

  Future<void> clearTokens() async {
    await _storage.delete(key: _accessKey);
    await _storage.delete(key: _refreshKey);
  }

  Future<void> clearSession() async {
    await clearTokens();
    await _storage.delete(key: _bootstrapKey);
  }

  Future<String?> readBootstrap() => _storage.read(key: _bootstrapKey);

  Future<void> saveBootstrap(String value) async {
    await _storage.write(key: _bootstrapKey, value: value);
  }

  Future<void> clearBootstrap() async {
    await _storage.delete(key: _bootstrapKey);
  }

  Future<bool> hasConsent() async =>
      (await _storage.read(key: _consentKey)) == 'true';

  Future<void> setConsent(bool value) async {
    await _storage.write(key: _consentKey, value: value ? 'true' : 'false');
  }

  Future<String?> readBaseUrl() => _storage.read(key: _baseUrlKey);

  Future<void> saveBaseUrl(String value) async {
    await _storage.write(key: _baseUrlKey, value: value);
  }

  Future<List<Map<String, dynamic>>> readQueue() async {
    final raw = await _storage.read(key: _queueKey);
    if (raw == null || raw.isEmpty) {
      return <Map<String, dynamic>>[];
    }
    final decoded = jsonDecode(raw);
    if (decoded is! List) {
      return <Map<String, dynamic>>[];
    }
    return decoded
        .whereType<Map>()
        .map((item) => Map<String, dynamic>.from(item))
        .toList();
  }

  Future<void> writeQueue(List<Map<String, dynamic>> queue) async {
    await _storage.write(key: _queueKey, value: jsonEncode(queue));
  }
}
