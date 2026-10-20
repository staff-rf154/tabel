package repo

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidDelegation  = errors.New("некорректное делегирование")
	ErrDelegationState    = errors.New("недопустимое состояние делегирования")
	ErrDelegationConflict = errors.New("уже есть активное делегирование")
)

const delegationColumns = `
	id::text, from_user_id::text, to_user_id::text, status, requested_at,
	valid_until, decided_by::text, decided_at`

func scanDelegation(row interface{ Scan(...any) error }, d *Delegation) error {
	return row.Scan(&d.ID, &d.FromUserID, &d.ToUserID, &d.Status, &d.RequestedAt,
		&d.ValidUntil, &d.DecidedBy, &d.DecidedAt)
}

func (r *Repo) CreateDelegation(ctx context.Context, fromUserID, toUserID string, validUntil time.Time) (Delegation, error) {
	if fromUserID == toUserID {
		return Delegation{}, ErrInvalidDelegation
	}
	if !validUntil.After(time.Now()) {
		return Delegation{}, ErrInvalidDelegation
	}
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Delegation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var fromRole string
	if err := tx.QueryRow(ctx, `SELECT role FROM users WHERE id = $1::uuid FOR UPDATE`, fromUserID).Scan(&fromRole); err != nil {
		if isNoRows(err) {
			return Delegation{}, ErrNotFound
		}
		return Delegation{}, err
	}
	var toRole string
	if err := tx.QueryRow(ctx, `SELECT role FROM users WHERE id = $1::uuid FOR UPDATE`, toUserID).Scan(&toRole); err != nil {
		if isNoRows(err) {
			return Delegation{}, ErrNotFound
		}
		return Delegation{}, err
	}
	if fromRole != "operator" || toRole != "employee" {
		return Delegation{}, ErrInvalidDelegation
	}
	var active int
	if err := tx.QueryRow(ctx, `
SELECT count(*) FROM delegations
WHERE (from_user_id = $1::uuid OR to_user_id = $1::uuid)
  AND status IN ('pending', 'approved')`, fromUserID).Scan(&active); err != nil {
		return Delegation{}, err
	}
	if active > 0 {
		return Delegation{}, ErrDelegationConflict
	}
	if err := tx.QueryRow(ctx, `
SELECT count(*) FROM delegations
WHERE to_user_id = $1::uuid AND status IN ('pending', 'approved')`, toUserID).Scan(&active); err != nil {
		return Delegation{}, err
	}
	if active > 0 {
		return Delegation{}, ErrDelegationConflict
	}

	var delegation Delegation
	if err := scanDelegation(tx.QueryRow(ctx, `
INSERT INTO delegations (from_user_id, to_user_id, status, valid_until)
VALUES ($1::uuid, $2::uuid, 'pending', $3)
RETURNING `+delegationColumns, fromUserID, toUserID, validUntil), &delegation); err != nil {
		return Delegation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Delegation{}, err
	}
	body := "Запрошена передача прав оператора"
	if err := r.NotifyAdmins(ctx, "delegation.requested", body, &body, map[string]any{
		"delegation_id": delegation.ID,
		"from_user_id":  fromUserID,
		"to_user_id":    toUserID,
		"valid_until":   validUntil.UTC().Format(time.RFC3339),
	}); err != nil {
		return Delegation{}, err
	}
	target := toUserID
	_, _ = r.CreateNotification(ctx, "delegation.requested", body, &body, map[string]any{"delegation_id": delegation.ID}, &target)
	return delegation, nil
}

func (r *Repo) ListDelegations(ctx context.Context, userID string, all bool) ([]Delegation, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT `+delegationColumns+`,
	COALESCE((SELECT login FROM users WHERE users.id = delegations.from_user_id), ''),
	COALESCE((SELECT login FROM users WHERE users.id = delegations.to_user_id), '')
FROM delegations
WHERE $2 OR from_user_id = $1::uuid OR to_user_id = $1::uuid
ORDER BY requested_at DESC
LIMIT 500`, userID, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var delegations []Delegation
	for rows.Next() {
		var d Delegation
		if err := rows.Scan(&d.ID, &d.FromUserID, &d.ToUserID, &d.Status, &d.RequestedAt,
			&d.ValidUntil, &d.DecidedBy, &d.DecidedAt, &d.FromLogin, &d.ToLogin); err != nil {
			return nil, err
		}
		delegations = append(delegations, d)
	}
	return delegations, rows.Err()
}

func (r *Repo) GetDelegation(ctx context.Context, id string) (Delegation, error) {
	var d Delegation
	err := scanDelegation(r.Pool.QueryRow(ctx, `SELECT `+delegationColumns+` FROM delegations WHERE id = $1::uuid`, id), &d)
	if isNoRows(err) {
		return d, ErrNotFound
	}
	return d, err
}

func (r *Repo) DecideDelegation(ctx context.Context, id, adminID string, approve bool) (Delegation, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Delegation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var d Delegation
	if err := scanDelegation(tx.QueryRow(ctx, `SELECT `+delegationColumns+` FROM delegations WHERE id = $1::uuid FOR UPDATE`, id), &d); err != nil {
		if isNoRows(err) {
			return Delegation{}, ErrNotFound
		}
		return Delegation{}, err
	}
	if d.Status != "pending" {
		return Delegation{}, ErrDelegationState
	}
	status := "rejected"
	if approve {
		status = "approved"
		if d.ValidUntil.Before(time.Now()) {
			return Delegation{}, ErrDelegationState
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET role = 'employee', updated_at = now() WHERE id = $1::uuid AND role = 'operator'`, d.FromUserID); err != nil {
			return Delegation{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET role = 'operator', updated_at = now() WHERE id = $1::uuid AND role = 'employee'`, d.ToUserID); err != nil {
			return Delegation{}, err
		}
	}
	if err := scanDelegation(tx.QueryRow(ctx, `
UPDATE delegations SET status = $2, decided_by = $3::uuid, decided_at = now(), updated_at = now()
WHERE id = $1::uuid
RETURNING `+delegationColumns, id, status, adminID), &d); err != nil {
		return Delegation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Delegation{}, err
	}

	title := "Делегирование отклонено"
	if approve {
		title = "Делегирование подтверждено"
	}
	body := title
	payload := map[string]any{"delegation_id": d.ID}
	for _, targetID := range []string{d.FromUserID, d.ToUserID} {
		target := targetID
		if _, err := r.CreateNotification(ctx, "delegation."+status, title, &body, payload, &target); err != nil {
			return Delegation{}, err
		}
	}
	return d, nil
}

func (r *Repo) RevokeDelegation(ctx context.Context, id, actorID string, isAdmin bool) (Delegation, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Delegation{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var d Delegation
	if err := scanDelegation(tx.QueryRow(ctx, `SELECT `+delegationColumns+` FROM delegations WHERE id = $1::uuid FOR UPDATE`, id), &d); err != nil {
		if isNoRows(err) {
			return Delegation{}, ErrNotFound
		}
		return Delegation{}, err
	}
	if !isAdmin && actorID != d.FromUserID && actorID != d.ToUserID {
		return Delegation{}, ErrNotFound
	}
	if d.Status != "pending" && d.Status != "approved" {
		return Delegation{}, ErrDelegationState
	}
	wasApproved := d.Status == "approved"
	if err := scanDelegation(tx.QueryRow(ctx, `
UPDATE delegations SET status = 'revoked', decided_by = $2::uuid, decided_at = now(), updated_at = now()
WHERE id = $1::uuid
RETURNING `+delegationColumns, id, actorID), &d); err != nil {
		return Delegation{}, err
	}
	if wasApproved {
		if err := restoreDelegationRoles(ctx, tx, d.FromUserID, d.ToUserID, d.ID); err != nil {
			return Delegation{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Delegation{}, err
	}
	title := "Делегирование отозвано"
	body := title
	payload := map[string]any{"delegation_id": d.ID}
	for _, targetID := range []string{d.FromUserID, d.ToUserID} {
		target := targetID
		if _, err := r.CreateNotification(ctx, "delegation.revoked", title, &body, payload, &target); err != nil {
			return Delegation{}, err
		}
	}
	return d, nil
}

func (r *Repo) ExpireDelegations(ctx context.Context) (int, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT id::text FROM delegations
WHERE status = 'approved' AND valid_until <= now()`)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	expired := 0
	for _, id := range ids {
		tx, err := r.Pool.Begin(ctx)
		if err != nil {
			return expired, err
		}
		var d Delegation
		if err := scanDelegation(tx.QueryRow(ctx, `SELECT `+delegationColumns+` FROM delegations WHERE id = $1::uuid FOR UPDATE`, id), &d); err != nil {
			_ = tx.Rollback(ctx)
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return expired, err
		}
		if d.Status != "approved" || d.ValidUntil.After(time.Now()) {
			_ = tx.Rollback(ctx)
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE delegations SET status = 'expired', updated_at = now() WHERE id = $1::uuid`, d.ID); err != nil {
			_ = tx.Rollback(ctx)
			return expired, err
		}
		if err := restoreDelegationRoles(ctx, tx, d.FromUserID, d.ToUserID, d.ID); err != nil {
			_ = tx.Rollback(ctx)
			return expired, err
		}
		if err := tx.Commit(ctx); err != nil {
			return expired, err
		}
		expired++
	}
	return expired, nil
}

func restoreDelegationRoles(ctx context.Context, tx pgx.Tx, fromUserID, toUserID, delegationID string) error {
	if _, err := tx.Exec(ctx, `UPDATE users SET role = 'operator', updated_at = now() WHERE id = $1::uuid AND role = 'employee'`, fromUserID); err != nil {
		return err
	}
	var other int
	if err := tx.QueryRow(ctx, `
SELECT count(*) FROM delegations
WHERE to_user_id = $1::uuid AND status = 'approved' AND valid_until > now() AND id <> $2::uuid`,
		toUserID, delegationID).Scan(&other); err != nil {
		return err
	}
	if other == 0 {
		if _, err := tx.Exec(ctx, `UPDATE users SET role = 'employee', updated_at = now() WHERE id = $1::uuid AND role = 'operator'`, toUserID); err != nil {
			return err
		}
	}
	return nil
}
