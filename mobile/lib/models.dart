class Me {
  Me({
    required this.id,
    required this.login,
    required this.role,
    this.fullName,
    this.employeeId,
    this.employeeNumber,
    this.position,
  });

  final String id;
  final String login;
  final String role;
  final String? fullName;
  final String? employeeId;
  final String? employeeNumber;
  final String? position;

  factory Me.fromJson(Map<String, dynamic> json) => Me(
        id: json['id'] as String? ?? '',
        login: json['login'] as String? ?? '',
        role: json['role'] as String? ?? 'employee',
        fullName: json['full_name'] as String?,
        employeeId: json['employee_id'] as String?,
        employeeNumber: json['employee_number'] as String?,
        position: json['position'] as String?,
      );

  String get roleTitle => switch (role) {
        'operator' => 'Оператор',
        'employee' => 'Сотрудник',
        'admin' => 'Администратор',
        'shift_lead' => 'Руководитель смены',
        'director' => 'Директор',
        'hr' => 'Отдел кадров',
        _ => role,
      };
}

class MobileNavItem {
  const MobileNavItem(
      {required this.key, required this.title, required this.icon});

  final String key;
  final String title;
  final String icon;

  factory MobileNavItem.fromJson(Map<String, dynamic> json) => MobileNavItem(
        key: json['key'] as String? ?? '',
        title: json['title'] as String? ?? '',
        icon: json['icon'] as String? ?? '',
      );
}

class MobileBootstrap {
  MobileBootstrap({
    required this.version,
    required this.configVersion,
    required this.role,
    required this.roleTitle,
    required this.shell,
    required this.features,
    required this.navigation,
    required this.cacheUntilLogout,
    required this.refreshAfterSec,
  });

  final int version;
  final int configVersion;
  final String role;
  final String roleTitle;
  final String shell;
  final Map<String, bool> features;
  final List<MobileNavItem> navigation;
  final bool cacheUntilLogout;
  final int refreshAfterSec;

  bool feature(String key) => features[key] == true;

  factory MobileBootstrap.fromJson(Map<String, dynamic> json) {
    final rawFeatures = json['features'];
    final features = <String, bool>{};
    if (rawFeatures is Map) {
      for (final entry in rawFeatures.entries) {
        if (entry.value == true) features[entry.key.toString()] = true;
      }
    }
    final rawNavigation = json['navigation'];
    final navigation = rawNavigation is List
        ? rawNavigation
            .whereType<Map>()
            .map((item) =>
                MobileNavItem.fromJson(Map<String, dynamic>.from(item)))
            .where((item) => item.key.isNotEmpty)
            .toList()
        : <MobileNavItem>[];
    final rawPolicy = json['policy'];
    final policy = rawPolicy is Map
        ? Map<String, dynamic>.from(rawPolicy)
        : <String, dynamic>{};
    return MobileBootstrap(
      version: json['version'] as int? ?? 1,
      configVersion: json['config_version'] as int? ?? 1,
      role: json['role'] as String? ?? '',
      roleTitle: json['role_title'] as String? ?? '',
      shell: json['shell'] as String? ?? 'unsupported',
      features: features,
      navigation: navigation,
      cacheUntilLogout: policy['cache_until_logout'] as bool? ?? true,
      refreshAfterSec: policy['refresh_after_sec'] as int? ?? 86400,
    );
  }
}

class MarkItem {
  MarkItem({
    required this.id,
    required this.eventType,
    required this.timestampServer,
    required this.overtime,
    required this.outsideOffice,
  });

  final String id;
  final String eventType;
  final DateTime timestampServer;
  final bool overtime;
  final bool outsideOffice;

  factory MarkItem.fromJson(Map<String, dynamic> json) => MarkItem(
        id: json['id'] as String? ?? '',
        eventType: json['event_type'] as String? ?? 'check_in',
        timestampServer:
            DateTime.parse(json['timestamp_server'] as String).toLocal(),
        overtime: json['overtime'] as bool? ?? false,
        outsideOffice: json['outside_office'] as bool? ?? false,
      );

  String get eventTitle => eventTypeTitle(eventType);
}

class MonthlyStats {
  MonthlyStats({
    required this.month,
    required this.daysWorked,
    required this.shiftsTotal,
    required this.shiftsClosed,
    required this.workedMinutes,
    required this.overtimeShifts,
    required this.lastCheckIns,
  });

  final String month;
  final int daysWorked;
  final int shiftsTotal;
  final int shiftsClosed;
  final int workedMinutes;
  final int overtimeShifts;
  final List<MarkItem> lastCheckIns;

  factory MonthlyStats.fromJson(Map<String, dynamic> json) {
    final rawMarks = json['last_check_ins'];
    return MonthlyStats(
      month: json['month'] as String? ?? '',
      daysWorked: json['days_worked'] as int? ?? 0,
      shiftsTotal: json['shifts_total'] as int? ?? 0,
      shiftsClosed: json['shifts_closed'] as int? ?? 0,
      workedMinutes: json['worked_minutes'] as int? ?? 0,
      overtimeShifts: json['overtime_shifts'] as int? ?? 0,
      lastCheckIns: rawMarks is List
          ? rawMarks
              .whereType<Map>()
              .map((item) => MarkItem.fromJson(Map<String, dynamic>.from(item)))
              .toList()
          : <MarkItem>[],
    );
  }
}

class AdminUser {
  AdminUser({
    required this.id,
    required this.login,
    required this.role,
    required this.isLocked,
    required this.authBlocked,
    required this.isMainAdmin,
    this.fullName,
    this.employeeNumber,
  });

  final String id;
  final String login;
  final String role;
  final bool isLocked;
  final bool authBlocked;
  final bool isMainAdmin;
  final String? fullName;
  final String? employeeNumber;

  String get roleTitle => switch (role) {
        'operator' => 'Оператор',
        'employee' => 'Сотрудник',
        'admin' => 'Администратор',
        'shift_lead' => 'Руководитель смены',
        'director' => 'Директор',
        'hr' => 'Отдел кадров',
        _ => role,
      };

  String get statusTitle {
    if (authBlocked) return 'Заблокирован';
    if (isLocked) return 'Временно заблокирован';
    return 'Активен';
  }

  factory AdminUser.fromJson(Map<String, dynamic> json) => AdminUser(
        id: json['id'] as String? ?? '',
        login: json['login'] as String? ?? '',
        role: json['role'] as String? ?? '',
        isLocked: json['is_locked'] as bool? ?? false,
        authBlocked: json['auth_blocked'] as bool? ?? false,
        isMainAdmin: json['is_main_admin'] as bool? ?? false,
        fullName: json['full_name'] as String?,
        employeeNumber: json['employee_number'] as String?,
      );
}

class ReportSummaryItem {
  ReportSummaryItem({
    required this.employeeId,
    required this.employeeNumber,
    required this.fullName,
    required this.position,
    required this.shiftsTotal,
    required this.shiftsClosed,
    required this.workedMinutes,
    required this.overtimeShifts,
    required this.marksTotal,
  });

  final String employeeId;
  final String employeeNumber;
  final String fullName;
  final String? position;
  final int shiftsTotal;
  final int shiftsClosed;
  final int workedMinutes;
  final int overtimeShifts;
  final int marksTotal;

  factory ReportSummaryItem.fromJson(Map<String, dynamic> json) =>
      ReportSummaryItem(
        employeeId: json['employee_id'] as String? ?? '',
        employeeNumber: json['employee_number'] as String? ?? '',
        fullName: json['full_name'] as String? ?? '',
        position: json['position'] as String?,
        shiftsTotal: json['shifts_total'] as int? ?? 0,
        shiftsClosed: json['shifts_closed'] as int? ?? 0,
        workedMinutes: json['worked_minutes'] as int? ?? 0,
        overtimeShifts: json['overtime_shifts'] as int? ?? 0,
        marksTotal: json['marks_total'] as int? ?? 0,
      );
}

String eventTypeTitle(String? value) => switch (value) {
      'check_in' => 'Приход',
      'check_out' => 'Уход',
      null => 'Авто',
      'auto' => 'Авто',
      _ => value,
    };

class MyQR {
  MyQR({
    required this.id,
    required this.token,
    required this.qrContent,
    required this.status,
    this.validUntil,
    this.revokedReason,
  });

  final String id;
  final String token;
  final String qrContent;
  final String status;
  final String? validUntil;
  final String? revokedReason;

  bool get valid => status == 'active';

  factory MyQR.fromJson(Map<String, dynamic> json) => MyQR(
        id: json['id'] as String? ?? '',
        token: json['token'] as String? ?? '',
        qrContent: json['qr_content'] as String? ?? '',
        status: json['status'] as String? ?? 'active',
        validUntil: json['valid_until'] as String?,
        revokedReason: json['revoked_reason'] as String?,
      );
}

class QRVerification {
  QRVerification({
    required this.valid,
    required this.tokenStatus,
    required this.signatureValid,
    required this.qrDataMismatch,
    this.employeeId,
    this.employeeNumber,
    this.fullName,
    this.reason,
  });

  final bool valid;
  final String tokenStatus;
  final bool signatureValid;
  final bool qrDataMismatch;
  final String? employeeId;
  final String? employeeNumber;
  final String? fullName;
  final String? reason;

  factory QRVerification.fromJson(Map<String, dynamic> json) => QRVerification(
        valid: json['valid'] as bool? ?? false,
        tokenStatus: json['token_status'] as String? ?? 'not_found',
        signatureValid: json['signature_valid'] as bool? ?? false,
        qrDataMismatch: json['qr_data_mismatch'] as bool? ?? false,
        employeeId: json['employee_id'] as String?,
        employeeNumber: json['employee_number'] as String?,
        fullName: json['full_name'] as String?,
        reason: json['reason'] as String?,
      );
}

class DelegationItem {
  DelegationItem(
      {required this.id,
      required this.status,
      required this.validUntil,
      this.toLogin});

  final String id;
  final String status;
  final DateTime validUntil;
  final String? toLogin;

  factory DelegationItem.fromJson(Map<String, dynamic> json) => DelegationItem(
        id: json['id'] as String? ?? '',
        status: json['status'] as String? ?? '',
        validUntil: DateTime.parse(json['valid_until'] as String).toLocal(),
        toLogin: json['to_login'] as String?,
      );

  String get statusTitle => switch (status) {
        'pending' => 'Ожидает решения',
        'approved' => 'Подтверждено',
        'rejected' => 'Отклонено',
        'revoked' => 'Отозвано',
        'expired' => 'Истекло',
        _ => status,
      };
}
