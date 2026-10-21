import 'package:flutter_test/flutter_test.dart';
import 'package:tabel_mobile/api.dart';
import 'package:tabel_mobile/models.dart';

void main() {
  test('нормализует адрес API из корня сайта', () {
    expect(ApiClient.normalizeBaseUrl('https://srv.staff-rf.ru/'),
        'https://srv.staff-rf.ru/api');
    expect(ApiClient.normalizeBaseUrl('https://srv.staff-rf.ru/api'),
        'https://srv.staff-rf.ru/api');
    expect(ApiClient.normalizeBaseUrl('http://10.0.2.2:8080'),
        'http://10.0.2.2:8080');
  });

  test('разбирает профиль пользователя', () {
    final me = Me.fromJson({
      'id': 'u1',
      'login': 'ivanov',
      'role': 'employee',
      'full_name': 'Иванов Иван',
      'employee_number': 'E-0001',
      'position': 'Оператор',
    });
    expect(me.roleTitle, 'Сотрудник');
    expect(me.employeeNumber, 'E-0001');
    expect(me.position, 'Оператор');
  });

  test('разбирает отметку и тип события', () {
    final mark = MarkItem.fromJson({
      'id': 'm1',
      'event_type': 'check_out',
      'timestamp_server': '2026-10-19T09:00:00Z',
      'overtime': true,
      'outside_office': false,
    });
    expect(mark.eventTitle, 'Уход');
    expect(mark.overtime, isTrue);
  });

  test('разбирает конфигурацию интерфейса с сервера', () {
    final config = MobileBootstrap.fromJson({
      'version': 1,
      'config_version': 3,
      'role': 'employee',
      'role_title': 'Сотрудник',
      'shell': 'employee',
      'features': {'my_qr': true, 'monthly_stats': true},
      'navigation': [
        {'key': 'home', 'title': 'Мои отметки', 'icon': 'badge'},
      ],
      'policy': {'cache_until_logout': true, 'refresh_after_sec': 86400},
    });
    expect(config.shell, 'employee');
    expect(config.feature('monthly_stats'), isTrue);
    expect(config.navigation.single.title, 'Мои отметки');
    expect(config.cacheUntilLogout, isTrue);
  });

  test('разбирает статистику сотрудника за месяц', () {
    final stats = MonthlyStats.fromJson({
      'month': '2026-10',
      'days_worked': 12,
      'shifts_total': 13,
      'shifts_closed': 12,
      'worked_minutes': 7320,
      'overtime_shifts': 1,
      'last_check_ins': [
        {
          'id': 'm1',
          'event_type': 'check_in',
          'timestamp_server': '2026-10-20T08:00:00Z'
        },
      ],
    });
    expect(stats.month, '2026-10');
    expect(stats.workedMinutes, 7320);
    expect(stats.lastCheckIns.single.eventTitle, 'Приход');
  });

  test('разбирает итоги отчёта для руководителя', () {
    final item = ReportSummaryItem.fromJson({
      'employee_id': 'e1',
      'employee_number': 'E-0001',
      'full_name': 'Иванов Иван',
      'position': 'Оператор',
      'shifts_total': 10,
      'shifts_closed': 9,
      'worked_minutes': 4800,
      'overtime_shifts': 1,
      'marks_total': 20,
    });
    expect(item.fullName, 'Иванов Иван');
    expect(item.workedMinutes, 4800);
  });

  test('разбирает пользователя для админского списка', () {
    final user = AdminUser.fromJson({
      'id': 'u1',
      'login': 'ПетроваМ',
      'role': 'shift_lead',
      'full_name': 'Петрова Мария',
      'employee_number': 'Н-00000002',
      'is_locked': false,
      'auth_blocked': false,
      'is_main_admin': false,
    });
    expect(user.roleTitle, 'Руководитель смены');
    expect(user.statusTitle, 'Активен');
  });

  test('разбирает результат проверки QR', () {
    final qr = QRVerification.fromJson({
      'valid': true,
      'token_status': 'active',
      'signature_valid': true,
      'employee_id': 'e1',
      'employee_number': 'E-0002',
      'full_name': 'Петрова Мария',
      'qr_data_mismatch': false,
    });
    expect(qr.valid, isTrue);
    expect(qr.fullName, 'Петрова Мария');
  });
}
