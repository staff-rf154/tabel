package repo

import (
	"context"
)

type ExpiringPaperQR struct {
	ID             string
	Token          string
	EmployeeID     string
	EmployeeNumber string
	FullName       string
	ValidUntil     string
}

func (r *Repo) ExpirePaperTokens(ctx context.Context) (int, error) {
	tag, err := r.Pool.Exec(ctx, `
UPDATE qr_tokens SET status = 'expired', updated_at = now()
WHERE type = 'paper' AND status = 'active' AND valid_until IS NOT NULL AND valid_until < current_date`)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func (r *Repo) ExpiringPaperTokens(ctx context.Context, withinDays int) ([]ExpiringPaperQR, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT q.id::text, q.token::text, q.employee_id::text, e.employee_number, e.full_name, q.valid_until::text
FROM qr_tokens q
JOIN employees e ON e.id = q.employee_id
WHERE q.type = 'paper' AND q.status = 'active'
  AND q.valid_until IS NOT NULL
  AND q.valid_until >= current_date
  AND q.valid_until <= current_date + $1::int
ORDER BY q.valid_until`, withinDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ExpiringPaperQR
	for rows.Next() {
		var item ExpiringPaperQR
		if err := rows.Scan(&item.ID, &item.Token, &item.EmployeeID, &item.EmployeeNumber, &item.FullName, &item.ValidUntil); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) OAuthClientsDueForRotation(ctx context.Context, withinDays int) ([]OAuthClient, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT `+oauthClientColumns+`
FROM oauth_clients
WHERE is_active AND next_rotation_at <= now() + ($1::int * interval '1 day')
ORDER BY next_rotation_at`, withinDays)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var clients []OAuthClient
	for rows.Next() {
		var c OAuthClient
		if err := scanOAuthClient(rows, &c); err != nil {
			return nil, err
		}
		clients = append(clients, c)
	}
	return clients, rows.Err()
}

func (r *Repo) NotificationExistsForPayload(ctx context.Context, ntype, key, value string) (bool, error) {
	var exists bool
	err := r.Pool.QueryRow(ctx, `
SELECT EXISTS (
	SELECT 1 FROM notifications WHERE type = $1 AND payload->>$2 = $3
)`, ntype, key, value).Scan(&exists)
	return exists, err
}

func (r *Repo) EnsureMarksPartition(ctx context.Context, date string) error {
	_, err := r.Pool.Exec(ctx, `SELECT create_marks_partition($1::date)`, date)
	return err
}
