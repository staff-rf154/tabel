package repo

import "context"

const qrTokenColumns = `
	q.id::text, q.employee_id::text, q.type, q.token::text, q.payload, q.signature,
	to_char(q.valid_from, 'YYYY-MM-DD'), to_char(q.valid_until, 'YYYY-MM-DD'), q.status, q.revoked_reason,
	e.is_active AND NOT EXISTS (
		SELECT 1 FROM users u WHERE u.employee_id = e.id AND u.auth_blocked
	), e.employee_number, e.full_name, e.position, e.phone, e.email`

func scanQRToken(row interface{ Scan(...any) error }, t *QRToken) error {
	return row.Scan(
		&t.ID, &t.EmployeeID, &t.Type, &t.Token, &t.Payload, &t.Signature,
		&t.ValidFrom, &t.ValidUntil, &t.Status, &t.RevokedReason,
		&t.EmployeeOK, &t.EmployeeNum, &t.FullName, &t.Position, &t.Phone, &t.Email,
	)
}

func getQRToken(ctx context.Context, q querier, where string, arg any) (QRToken, error) {
	var t QRToken
	err := scanQRToken(q.QueryRow(ctx, `
SELECT `+qrTokenColumns+`
FROM qr_tokens q
JOIN employees e ON e.id = q.employee_id
WHERE `+where, arg), &t)
	if isNoRows(err) {
		return t, ErrNotFound
	}
	return t, err
}

func (r *Repo) FindQRToken(ctx context.Context, token string) (QRToken, error) {
	return getQRToken(ctx, r.Pool, "q.token = $1::uuid", token)
}

func (r *Repo) GetActiveStaticQR(ctx context.Context, employeeID string) (QRToken, error) {
	return getQRToken(ctx, r.Pool, "q.employee_id = $1::uuid AND q.type = 'static' AND q.status = 'active'", employeeID)
}

func (r *Repo) GetQRTokenByID(ctx context.Context, id string) (QRToken, error) {
	return getQRToken(ctx, r.Pool, "q.id = $1::uuid", id)
}

func (r *Repo) FindEmployeeByNumber(ctx context.Context, number string) (Employee, error) {
	var e Employee
	err := scanEmployee(r.Pool.QueryRow(ctx, `SELECT `+employeeColumns+` FROM employees WHERE employee_number = $1`, number), &e)
	if isNoRows(err) {
		return e, ErrNotFound
	}
	return e, err
}

// CreateStaticQR создаёт статический QR-токен. Если у сотрудника уже есть
// активный статический токен, он отзывается (ротация).
func (r *Repo) CreateStaticQR(ctx context.Context, employeeID, token, payload, signature, issuedBy, revokeReason string) (QRToken, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return QRToken{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var active bool
	if err := tx.QueryRow(ctx, `SELECT is_active FROM employees WHERE id = $1::uuid FOR UPDATE`, employeeID).Scan(&active); err != nil {
		if isNoRows(err) {
			return QRToken{}, ErrNotFound
		}
		return QRToken{}, err
	}
	if !active {
		return QRToken{}, ErrEmployeeInactive
	}

	if _, err := tx.Exec(ctx, `
UPDATE qr_tokens
SET status = 'revoked', revoked_at = now(), revoked_by = $2::uuid,
    revoked_reason = NULLIF($3, ''), updated_at = now()
WHERE employee_id = $1::uuid AND type = 'static' AND status = 'active'`, employeeID, issuedBy, revokeReason); err != nil {
		return QRToken{}, err
	}

	var id string
	err = tx.QueryRow(ctx, `
INSERT INTO qr_tokens (employee_id, type, token, payload, signature, status, issued_by)
VALUES ($1::uuid, 'static', $2::uuid, $3::jsonb, $4, 'active', $5::uuid)
RETURNING id::text`, employeeID, token, payload, signature, issuedBy).Scan(&id)
	if isUniqueViolation(err) {
		return QRToken{}, ErrQRTokenExists
	}
	if err != nil {
		return QRToken{}, err
	}

	created, err := getQRToken(ctx, tx, "q.id = $1::uuid", id)
	if err != nil {
		return QRToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return QRToken{}, err
	}
	return created, nil
}

// IssuePaperQR выдаёт бумажный QR-токен с ограниченным сроком действия.
func (r *Repo) IssuePaperQR(ctx context.Context, employeeID, token, payload, signature, validFrom, validUntil, issuedBy string) (QRToken, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return QRToken{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var active bool
	if err := tx.QueryRow(ctx, `SELECT is_active FROM employees WHERE id = $1::uuid FOR UPDATE`, employeeID).Scan(&active); err != nil {
		if isNoRows(err) {
			return QRToken{}, ErrNotFound
		}
		return QRToken{}, err
	}
	if !active {
		return QRToken{}, ErrEmployeeInactive
	}

	if _, err := tx.Exec(ctx, `
UPDATE qr_tokens
SET status = 'expired', updated_at = now()
WHERE employee_id = $1::uuid AND type = 'paper' AND status = 'active'
  AND valid_until < $2::date`, employeeID, validFrom); err != nil {
		return QRToken{}, err
	}

	var id string
	err = tx.QueryRow(ctx, `
INSERT INTO qr_tokens (employee_id, type, token, payload, signature, valid_from, valid_until, status, issued_by)
VALUES ($1::uuid, 'paper', $2::uuid, $3::jsonb, $4, $5::date, $6::date, 'active', $7::uuid)
RETURNING id::text`, employeeID, token, payload, signature, validFrom, validUntil, issuedBy).Scan(&id)
	if isUniqueViolation(err) {
		return QRToken{}, ErrActivePaperQRExists
	}
	if err != nil {
		return QRToken{}, err
	}

	created, err := getQRToken(ctx, tx, "q.id = $1::uuid", id)
	if err != nil {
		return QRToken{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return QRToken{}, err
	}
	return created, nil
}

// RevokePaperQR отзывает бумажный токен. Повторный отзыв идемпотентен.
func (r *Repo) RevokePaperQR(ctx context.Context, id, revokedBy string) error {
	tag, err := r.Pool.Exec(ctx, `
UPDATE qr_tokens
SET status = 'revoked', revoked_at = now(), revoked_by = $2::uuid,
    revoked_reason = 'отозван администратором', updated_at = now()
WHERE id = $1::uuid AND type = 'paper' AND status = 'active'`, id, revokedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	var exists bool
	err = r.Pool.QueryRow(ctx, `SELECT true FROM qr_tokens WHERE id = $1::uuid AND type = 'paper'`, id).Scan(&exists)
	if isNoRows(err) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return nil
}
