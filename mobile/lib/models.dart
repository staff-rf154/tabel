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
        timestampServer: DateTime.parse(json['timestamp_server'] as String).toLocal(),
        overtime: json['overtime'] as bool? ?? false,
        outsideOffice: json['outside_office'] as bool? ?? false,
      );

  String get eventTitle => eventTypeTitle(eventType);
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
  DelegationItem({required this.id, required this.status, required this.validUntil, this.toLogin});

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
