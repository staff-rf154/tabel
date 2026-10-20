import 'package:device_info_plus/device_info_plus.dart';
import 'package:geolocator/geolocator.dart';
import 'package:package_info_plus/package_info_plus.dart';

class DeviceContext {
  const DeviceContext({
    this.lat,
    this.lon,
    this.accuracy,
    this.locationAgeSec,
    this.os,
    this.osVersion,
    this.model,
    this.appVersion,
    this.mockLocation = false,
  });

  final double? lat;
  final double? lon;
  final double? accuracy;
  final int? locationAgeSec;
  final String? os;
  final String? osVersion;
  final String? model;
  final String? appVersion;
  final bool mockLocation;

  static Future<DeviceContext> collect() async {
    Position? position;
    try {
      final service = await Geolocator.isLocationServiceEnabled();
      if (service) {
        var permission = await Geolocator.checkPermission();
        if (permission == LocationPermission.denied) {
          permission = await Geolocator.requestPermission();
        }
        if (permission == LocationPermission.whileInUse || permission == LocationPermission.always) {
          position = await Geolocator.getCurrentPosition(
            desiredAccuracy: LocationAccuracy.high,
            timeLimit: const Duration(seconds: 8),
          );
        }
      }
    } catch (_) {
      position = null;
    }

    String? os;
    String? osVersion;
    String? model;
    final info = DeviceInfoPlugin();
    try {
      final android = await info.androidInfo;
      os = 'android';
      osVersion = android.version.release;
      model = '${android.manufacturer} ${android.model}';
    } catch (_) {
      try {
        final ios = await info.iosInfo;
        os = 'ios';
        osVersion = ios.systemVersion;
        model = ios.utsname.machine;
      } catch (_) {
        os = null;
      }
    }

    String? appVersion;
    try {
      final package = await PackageInfo.fromPlatform();
      appVersion = '${package.version}+${package.buildNumber}';
    } catch (_) {
      appVersion = null;
    }

    final age = position == null ? null : DateTime.now().difference(position.timestamp).inSeconds;
    return DeviceContext(
      lat: position?.latitude,
      lon: position?.longitude,
      accuracy: position?.accuracy,
      locationAgeSec: age,
      os: os,
      osVersion: osVersion,
      model: model,
      appVersion: appVersion,
      mockLocation: position?.isMocked ?? false,
    );
  }
}
