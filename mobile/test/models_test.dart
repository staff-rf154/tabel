import 'package:flutter_test/flutter_test.dart';
import 'package:tabel_mobile/models.dart';

void main() {
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
