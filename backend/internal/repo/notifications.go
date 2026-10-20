package repo

import (
	"context"
	"encoding/json"
	"time"
)

func (r *Repo) CreateNotification(ctx context.Context, ntype, title string, body *string, payload any, targetUserID *string) (Notification, error) {
	var raw any
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return Notification{}, err
		}
		raw = string(encoded)
	}
	var notification Notification
	var payloadBytes []byte
	err := r.Pool.QueryRow(ctx, `
INSERT INTO notifications (type, title, body, payload, target_user_id)
VALUES ($1, $2, $3, $4::jsonb, $5::uuid)
RETURNING id::text, type, title, body, payload, target_user_id::text, is_read, created_at`,
		ntype, title, body, raw, targetUserID).Scan(
		&notification.ID, &notification.Type, &notification.Title, &notification.Body,
		&payloadBytes, &notification.TargetUserID, &notification.IsRead, &notification.CreatedAt,
	)
	if err != nil {
		return Notification{}, err
	}
	notification.Payload = json.RawMessage(payloadBytes)
	return notification, nil
}

func (r *Repo) NotifyAdmins(ctx context.Context, ntype, title string, body *string, payload any) error {
	rows, err := r.Pool.Query(ctx, `
SELECT id::text FROM users
WHERE role = 'admin' OR is_main_admin
ORDER BY registered_at NULLS LAST`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		target := id
		if _, err := r.CreateNotification(ctx, ntype, title, body, payload, &target); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repo) ListNotifications(ctx context.Context, userID string, unreadOnly bool, limit int) ([]Notification, error) {
	limit = normalizedLimit(limit, 50, 200)
	rows, err := r.Pool.Query(ctx, `
SELECT id::text, type, title, body, payload, target_user_id::text, is_read, created_at
FROM notifications
WHERE target_user_id = $1::uuid
  AND (NOT $2 OR NOT is_read)
ORDER BY created_at DESC
LIMIT $3`, userID, unreadOnly, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notifications []Notification
	for rows.Next() {
		var notification Notification
		var payload []byte
		if err := rows.Scan(
			&notification.ID, &notification.Type, &notification.Title, &notification.Body,
			&payload, &notification.TargetUserID, &notification.IsRead, &notification.CreatedAt,
		); err != nil {
			return nil, err
		}
		notification.Payload = json.RawMessage(payload)
		notifications = append(notifications, notification)
	}
	return notifications, rows.Err()
}

func (r *Repo) MarkNotificationRead(ctx context.Context, userID, notificationID string) error {
	tag, err := r.Pool.Exec(ctx, `
UPDATE notifications SET is_read = true
WHERE id = $1::uuid AND target_user_id = $2::uuid`, notificationID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repo) NotificationsCreatedSince(ctx context.Context, ntype string, since time.Time) (int, error) {
	var count int
	err := r.Pool.QueryRow(ctx, `
SELECT count(*) FROM notifications WHERE type = $1 AND created_at >= $2`, ntype, since).Scan(&count)
	return count, err
}
