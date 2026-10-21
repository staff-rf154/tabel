import 'package:flutter/material.dart';
import 'package:package_info_plus/package_info_plus.dart';

class AppVersionLabel extends StatelessWidget {
  const AppVersionLabel({super.key, this.align = TextAlign.center});

  final TextAlign align;

  static Future<PackageInfo>? _info;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    return FutureBuilder<PackageInfo>(
      future: _info ??= PackageInfo.fromPlatform(),
      builder: (context, snapshot) {
        final info = snapshot.data;
        if (info == null) return const SizedBox.shrink();
        final build = info.buildNumber.isEmpty ? '' : ' (${info.buildNumber})';
        return Text(
          'Версия ${info.version}$build',
          textAlign: align,
          style: theme.textTheme.bodySmall
              ?.copyWith(color: theme.colorScheme.onSurfaceVariant),
        );
      },
    );
  }
}
