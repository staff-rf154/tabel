import 'dart:convert';

import 'package:flutter/foundation.dart';
import 'package:uuid/uuid.dart';

import '../storage.dart';

class QueueItem {
  QueueItem({
    required this.id,
    required this.qrToken,
    required this.idempotencyKey,
    required this.timestampClient,
    this.eventType,
    this.lat,
    this.lon,
    this.accuracy,
    this.locationAgeSec,
  });

  final String id;
  final String qrToken;
  final String idempotencyKey;
  final DateTime timestampClient;
  final String? eventType;
  final double? lat;
  final double? lon;
  final double? accuracy;
  final int? locationAgeSec;

  factory QueueItem.create({
    required String qrToken,
    String? eventType,
    double? lat,
    double? lon,
    double? accuracy,
    int? locationAgeSec,
  }) {
    return QueueItem(
      id: const Uuid().v4(),
      qrToken: qrToken,
      idempotencyKey: const Uuid().v4(),
      timestampClient: DateTime.now(),
      eventType: eventType,
      lat: lat,
      lon: lon,
      accuracy: accuracy,
      locationAgeSec: locationAgeSec,
    );
  }

  factory QueueItem.fromJson(Map<String, dynamic> json) => QueueItem(
        id: json['id'] as String,
        qrToken: json['qr_token'] as String,
        idempotencyKey: json['idempotency_key'] as String,
        timestampClient: DateTime.parse(json['timestamp_client'] as String),
        eventType: json['event_type'] as String?,
        lat: (json['lat'] as num?)?.toDouble(),
        lon: (json['lon'] as num?)?.toDouble(),
        accuracy: (json['accuracy'] as num?)?.toDouble(),
        locationAgeSec: (json['location_age_sec'] as num?)?.toInt(),
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        'qr_token': qrToken,
        'idempotency_key': idempotencyKey,
        'timestamp_client': timestampClient.toIso8601String(),
        if (eventType != null) 'event_type': eventType,
        if (lat != null) 'lat': lat,
        if (lon != null) 'lon': lon,
        if (accuracy != null) 'accuracy': accuracy,
        if (locationAgeSec != null) 'location_age_sec': locationAgeSec,
      };
}

class OfflineQueue {
  OfflineQueue(this._storage);

  final AppStorage _storage;
  final ValueNotifier<int> count = ValueNotifier<int>(0);

  Future<List<QueueItem>> all() async {
    final raw = await _storage.readQueue();
    final items = raw.map(QueueItem.fromJson).toList();
    count.value = items.length;
    return items;
  }

  Future<void> add(QueueItem item) async {
    final raw = await _storage.readQueue();
    raw.add(item.toJson());
    await _storage.writeQueue(raw);
    count.value = raw.length;
  }

  Future<void> remove(String id) async {
    final raw = await _storage.readQueue();
    raw.removeWhere((item) => item['id'] == id);
    await _storage.writeQueue(raw);
    count.value = raw.length;
  }

  Future<void> replace(List<QueueItem> items) async {
    await _storage.writeQueue(items.map((item) => item.toJson()).toList());
    count.value = items.length;
  }

  String exportCsv(List<QueueItem> items) {
    final buffer = StringBuffer('id,qr_token,event_type,timestamp_client,idempotency_key\r\n');
    for (final item in items) {
      buffer.writeln([
        item.id,
        item.qrToken,
        item.eventType ?? '',
        item.timestampClient.toUtc().toIso8601String(),
        item.idempotencyKey,
      ].map(_escape).join(','));
    }
    return buffer.toString();
  }

  String _escape(String value) {
    if (value.contains(',') || value.contains('"')) {
      return '"${value.replaceAll('"', '""')}"';
    }
    return value;
  }
}

Map<String, dynamic> decodeQueueJson(String raw) => jsonDecode(raw) as Map<String, dynamic>;
