package repo

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
)

type Session struct {
	ID        string
	UserID    string
	ExpiresAt time.Time
	RevokedAt *time.Time
}

type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

const authUserColumns = `
	u.id::text, u.employee_id::text, u.role, u.login, u.password_hash,
	u.failed_attempts, u.is_locked, u.auth_blocked, u.is_main_admin,
	u.registration_reset_allowed`

func scanAuthUser(row pgx.Row, u *AuthUser) error {
	return row.Scan(
		&u.ID, &u.EmployeeID, &u.Role, &u.Login, &u.PasswordHash,
		&u.FailedAttempts, &u.IsLocked, &u.AuthBlocked, &u.IsMainAdmin,
		&u.RegistrationResetAllowed,
	)
}

func (r *Repo) FindAuthUserByLogin(ctx context.Context, login string) (AuthUser, error) {
	var u AuthUser
	err := scanAuthUser(r.Pool.QueryRow(ctx, `SELECT `+authUserColumns+` FROM users u WHERE u.login = $1`, login), &u)
	if isNoRows(err) {
		return u, ErrNotFound
	}
	return u, err
}

func (r *Repo) FindAuthUserByID(ctx context.Context, id string) (AuthUser, error) {
	var u AuthUser
	err := scanAuthUser(r.Pool.QueryRow(ctx, `SELECT `+authUserColumns+` FROM users u WHERE u.id = $1::uuid`, id), &u)
	if isNoRows(err) {
		return u, ErrNotFound
	}
	return u, err
}

func (r *Repo) ProfileByID(ctx context.Context, id string) (Profile, error) {
	return profileByID(ctx, r.Pool, id)
}

func profileByID(ctx context.Context, q querier, id string) (Profile, error) {
	var p Profile
	err := q.QueryRow(ctx, `
SELECT u.id::text, u.role, u.login, u.employee_id::text,
       e.full_name, e.position, e.employee_number
FROM users u
LEFT JOIN employees e ON e.id = u.employee_id
WHERE u.id = $1::uuid`, id).Scan(
		&p.ID, &p.Role, &p.Login, &p.EmployeeID,
		&p.FullName, &p.Position, &p.EmployeeNumber,
	)
	if isNoRows(err) {
		return p, ErrNotFound
	}
	return p, err
}

func (r *Repo) RecordLoginFailure(ctx context.Context, userID string) (int, bool, error) {
	var attempts int
	var locked bool
	err := r.Pool.QueryRow(ctx, `
UPDATE users
SET failed_attempts = failed_attempts + 1,
    is_locked = (failed_attempts + 1 >= 5),
    updated_at = now()
WHERE id = $1::uuid
RETURNING failed_attempts, is_locked`, userID).Scan(&attempts, &locked)
	return attempts, locked, err
}

func (r *Repo) RecordLoginSuccess(ctx context.Context, userID string) error {
	_, err := r.Pool.Exec(ctx, `
UPDATE users
SET failed_attempts = 0, is_locked = false, last_login_at = now(), updated_at = now()
WHERE id = $1::uuid`, userID)
	return err
}

func (r *Repo) CreateSession(ctx context.Context, sessionID, userID, tokenHash, userAgent string, ip *string, expiresAt time.Time) (Session, error) {
	session := Session{ID: sessionID, UserID: userID, ExpiresAt: expiresAt}
	var ua *string
	if userAgent != "" {
		ua = &userAgent
	}
	_, err := r.Pool.Exec(ctx, `
INSERT INTO sessions (id, user_id, token_hash, user_agent, ip, expires_at)
VALUES ($1::uuid, $2::uuid, $3, $4, $5::inet, $6)`,
		session.ID, userID, tokenHash, ua, ip, expiresAt)
	if err != nil {
		return Session{}, err
	}
	return session, nil
}

func (r *Repo) SessionByTokenHash(ctx context.Context, tokenHash string) (Session, AuthUser, error) {
	var s Session
	var u AuthUser
	err := r.Pool.QueryRow(ctx, `
SELECT s.id::text, s.user_id::text, s.expires_at, s.revoked_at,
       `+authUserColumns+`
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1`, tokenHash).Scan(
		&s.ID, &s.UserID, &s.ExpiresAt, &s.RevokedAt,
		&u.ID, &u.EmployeeID, &u.Role, &u.Login, &u.PasswordHash,
		&u.FailedAttempts, &u.IsLocked, &u.AuthBlocked, &u.IsMainAdmin,
		&u.RegistrationResetAllowed,
	)
	if isNoRows(err) {
		return s, u, ErrNotFound
	}
	return s, u, err
}

func (r *Repo) RevokeSession(ctx context.Context, sessionID string) error {
	_, err := r.Pool.Exec(ctx, `
UPDATE sessions SET revoked_at = now()
WHERE id = $1::uuid AND revoked_at IS NULL`, sessionID)
	return err
}

func (r *Repo) RevokeUserSessions(ctx context.Context, userID string) error {
	_, err := r.Pool.Exec(ctx, `
UPDATE sessions SET revoked_at = now()
WHERE user_id = $1::uuid AND revoked_at IS NULL`, userID)
	return err
}

func (r *Repo) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	tag, err := r.Pool.Exec(ctx, `
UPDATE users
SET password_hash = $2, password_algo = 'argon2id', updated_at = now()
WHERE id = $1::uuid`, userID, passwordHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// RegisterEmployee создаёт (или пересоздаёт после сброса администратором)
// учётную запись сотрудника по данным QR-токена.
func (r *Repo) RegisterEmployee(ctx context.Context, employeeID, login, passwordHash, fullName, phone string) (Profile, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Profile{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var lockedID string
	err = tx.QueryRow(ctx, `SELECT id::text FROM employees WHERE id = $1::uuid AND is_active FOR UPDATE`, employeeID).Scan(&lockedID)
	if isNoRows(err) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}

	var existingID string
	var resetAllowed bool
	err = tx.QueryRow(ctx, `
SELECT id::text, registration_reset_allowed FROM users
WHERE employee_id = $1::uuid FOR UPDATE`, employeeID).Scan(&existingID, &resetAllowed)
	switch {
	case err == nil && !resetAllowed:
		return Profile{}, ErrAlreadyRegistered
	case err == nil:
		_, err = tx.Exec(ctx, `
UPDATE users
SET login = $2, password_hash = $3, password_algo = 'argon2id',
    failed_attempts = 0, is_locked = false, auth_blocked = false,
    registration_reset_allowed = false, registered_at = now(), updated_at = now()
WHERE id = $1::uuid`, existingID, login, passwordHash)
		if err != nil {
			if isUniqueViolation(err) {
				return Profile{}, ErrLoginTaken
			}
			return Profile{}, err
		}
	case !isNoRows(err):
		return Profile{}, err
	default:
		var userID string
		err = tx.QueryRow(ctx, `
INSERT INTO users (employee_id, role, login, password_hash, password_algo, registered_at)
VALUES ($1::uuid, 'employee', $2, $3, 'argon2id', now())
RETURNING id::text`, employeeID, login, passwordHash).Scan(&userID)
		if err != nil {
			if isUniqueViolation(err) {
				return Profile{}, ErrLoginTaken
			}
			return Profile{}, err
		}
		existingID = userID
	}

	if _, err := tx.Exec(ctx, `
UPDATE employees
SET full_name = $2,
    phone = COALESCE(phone, NULLIF($3, '')),
    updated_at = now()
WHERE id = $1::uuid`, employeeID, fullName, phone); err != nil {
		return Profile{}, err
	}

	profile, err := profileByID(ctx, tx, existingID)
	if err != nil {
		return Profile{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

// BootstrapMainAdmin создаёт главного администратора при завершении установки.
// Если главный администратор уже существует, возвращается его идентификатор.
func (r *Repo) BootstrapMainAdmin(ctx context.Context, login, passwordHash string) (string, error) {
	var id string
	err := r.Pool.QueryRow(ctx, `
SELECT id::text FROM users WHERE is_main_admin ORDER BY created_at LIMIT 1`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !isNoRows(err) {
		return "", err
	}

	err = r.Pool.QueryRow(ctx, `
INSERT INTO users (role, login, password_hash, password_algo, is_main_admin, registered_at)
VALUES ('admin', $1, $2, 'argon2id', true, now())
RETURNING id::text`, login, passwordHash).Scan(&id)
	if isUniqueViolation(err) {
		return "", ErrLoginTaken
	}
	if err != nil {
		return "", fmt.Errorf("создание главного администратора: %w", err)
	}
	return id, nil
}

func loginFragment(value string) string {
	var b strings.Builder
	for _, ch := range value {
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '-' || ch == '_' {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func capitalizeFirst(value string) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}

// employeeLoginCandidates строит варианты логина «ФамилияИ», «ФамилияИв»,
// «ФамилияИва» и так далее. Первый вариант — базовый «ФамилияИ».
func employeeLoginCandidates(fullName string) (string, []string) {
	fields := strings.Fields(fullName)
	surname := ""
	name := ""
	if len(fields) > 0 {
		surname = loginFragment(fields[0])
	}
	if len(fields) > 1 {
		name = loginFragment(fields[1])
	}
	if surname == "" && name == "" {
		surname = "Сотрудник"
	}
	if surname == "" {
		surname = name
		name = ""
	}
	surname = capitalizeFirst(surname)
	nameRunes := []rune(name)
	base := surname
	if len(nameRunes) > 0 {
		base += string(unicode.ToUpper(nameRunes[0]))
	}
	candidates := []string{base}
	for i := 2; i <= len(nameRunes); i++ {
		candidates = append(candidates, base+string(nameRunes[1:i]))
	}
	return base, candidates
}

// GenerateEmployeeLogin строит логин «ФамилияИ» и при занятости добавляет
// по одной букве имени, пока логин не станет уникальным.
func (r *Repo) GenerateEmployeeLogin(ctx context.Context, fullName string) (string, error) {
	base, candidates := employeeLoginCandidates(fullName)
	for _, candidate := range candidates {
		var exists bool
		if err := r.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE lower(login) = lower($1))`, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	for suffix := 2; suffix < 10000; suffix++ {
		candidate := base + strconv.Itoa(suffix)
		var exists bool
		if err := r.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE lower(login) = lower($1))`, candidate).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return candidate, nil
		}
	}
	return "", ErrLoginTaken
}

func (r *Repo) ListUsers(ctx context.Context, role, search string, limit, offset int) ([]User, error) {
	limit = normalizedLimit(limit, 50, 500)
	offset = normalizedOffset(offset)
	rows, err := r.Pool.Query(ctx, `
SELECT u.id::text, u.role, u.login, u.employee_id::text,
       u.is_locked, u.auth_blocked, u.is_main_admin,
       u.registered_at, u.last_login_at,
       e.full_name, e.employee_number
FROM users u
LEFT JOIN employees e ON e.id = u.employee_id
WHERE ($1 = '' OR u.role = $1)
  AND ($2 = ''
       OR u.login ILIKE '%' || $2 || '%'
       OR e.full_name ILIKE '%' || $2 || '%'
       OR e.employee_number ILIKE '%' || $2 || '%')
ORDER BY u.created_at DESC
LIMIT $3 OFFSET $4`, role, search, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(
			&u.ID, &u.Role, &u.Login, &u.EmployeeID,
			&u.IsLocked, &u.AuthBlocked, &u.IsMainAdmin,
			&u.RegisteredAt, &u.LastLoginAt,
			&u.FullName, &u.EmployeeNumber,
		); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (r *Repo) SetAuthBlocked(ctx context.Context, userID string, blocked bool) error {
	tag, err := r.Pool.Exec(ctx, `
UPDATE users
SET auth_blocked = $2,
    is_locked = CASE WHEN $2 THEN is_locked ELSE false END,
    failed_attempts = CASE WHEN $2 THEN failed_attempts ELSE 0 END,
    updated_at = now()
WHERE id = $1::uuid`, userID, blocked)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if blocked {
		return r.RevokeUserSessions(ctx, userID)
	}
	return nil
}

func (r *Repo) SetRegistrationReset(ctx context.Context, userID string, allowed bool) error {
	tag, err := r.Pool.Exec(ctx, `
UPDATE users
SET registration_reset_allowed = $2, updated_at = now()
WHERE id = $1::uuid AND role = 'employee'`, userID, allowed)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	if allowed {
		return r.RevokeUserSessions(ctx, userID)
	}
	return nil
}

// OperatorNumber возвращает табельный номер пользователя (если он привязан к сотруднику).
func (r *Repo) OperatorNumber(ctx context.Context, userID string) *string {
	var number string
	err := r.Pool.QueryRow(ctx, `
SELECT e.employee_number FROM users u
JOIN employees e ON e.id = u.employee_id
WHERE u.id = $1::uuid`, userID).Scan(&number)
	if err != nil {
		return nil
	}
	return &number
}

var userColumns = `
	id::text, role, login, employee_id::text, is_locked, auth_blocked, is_main_admin,
	registered_at, last_login_at`

func scanUser(row pgx.Row, u *User) error {
	return row.Scan(&u.ID, &u.Role, &u.Login, &u.EmployeeID, &u.IsLocked, &u.AuthBlocked,
		&u.IsMainAdmin, &u.RegisteredAt, &u.LastLoginAt)
}

func (r *Repo) CreateUser(ctx context.Context, login, passwordHash, role string, employeeID *string) (User, error) {
	var u User
	err := scanUser(r.Pool.QueryRow(ctx, `
INSERT INTO users (role, login, password_hash, password_algo, employee_id, registered_at)
VALUES ($1, $2, $3, 'argon2id', $4::uuid, now())
RETURNING `+userColumns, role, login, passwordHash, employeeID), &u)
	if isUniqueViolation(err) {
		return u, ErrLoginTaken
	}
	return u, err
}

func (r *Repo) SetUserRole(ctx context.Context, id, role string) error {
	tag, err := r.Pool.Exec(ctx, `UPDATE users SET role = $2, updated_at = now() WHERE id = $1::uuid`, id, role)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) UpdateUserRole(ctx context.Context, userID, role string) (User, error) {
	var u User
	err := r.Pool.QueryRow(ctx, `
UPDATE users SET role = $2, updated_at = now()
WHERE id = $1::uuid
RETURNING id::text, role, login, employee_id::text,
          is_locked, auth_blocked, is_main_admin,
          registered_at, last_login_at,
          (SELECT full_name FROM employees WHERE employees.id = users.employee_id),
          (SELECT employee_number FROM employees WHERE employees.id = users.employee_id)`,
		userID, role).Scan(
		&u.ID, &u.Role, &u.Login, &u.EmployeeID,
		&u.IsLocked, &u.AuthBlocked, &u.IsMainAdmin,
		&u.RegisteredAt, &u.LastLoginAt,
		&u.FullName, &u.EmployeeNumber,
	)
	if isNoRows(err) {
		return u, ErrNotFound
	}
	return u, err
}

func (r *Repo) FindEmployeeUserByLogin(ctx context.Context, login string) (AuthUser, error) {
	var u AuthUser
	err := scanAuthUser(r.Pool.QueryRow(ctx, `SELECT `+authUserColumns+` FROM users u WHERE u.login = $1 AND u.role = 'employee'`, login), &u)
	if isNoRows(err) {
		return u, ErrNotFound
	}
	return u, err
}

func (r *Repo) DeleteUser(ctx context.Context, userID string) error {
	tag, err := r.Pool.Exec(ctx, `DELETE FROM users WHERE id = $1::uuid AND NOT is_main_admin`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
