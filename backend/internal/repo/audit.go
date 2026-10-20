package repo

import (
	"context"
	"encoding/json"
)

type AuditInput struct {
	ActorUserID *string
	Action      string
	EntityType  string
	EntityID    string
	Details     any
	IP          *string
	UserAgent   string
}

func (r *Repo) WriteAudit(ctx context.Context, in AuditInput) error {
	var details *string
	if in.Details != nil {
		raw, err := json.Marshal(in.Details)
		if err != nil {
			return err
		}
		value := string(raw)
		details = &value
	}
	var entityType, entityID, userAgent *string
	if in.EntityType != "" {
		entityType = &in.EntityType
	}
	if in.EntityID != "" {
		entityID = &in.EntityID
	}
	if in.UserAgent != "" {
		userAgent = &in.UserAgent
	}
	_, err := r.Pool.Exec(ctx, `
INSERT INTO audit_log (actor_user_id, action, entity_type, entity_id, details, ip, user_agent)
VALUES ($1::uuid, $2, $3, $4, $5::jsonb, $6::inet, $7)`,
		in.ActorUserID, in.Action, entityType, entityID, details, in.IP, userAgent)
	return err
}

func (r *Repo) ListAudit(ctx context.Context, f AuditFilter) ([]AuditEntry, error) {
	f.Limit = normalizedLimit(f.Limit, 100, 1000)
	f.Offset = normalizedOffset(f.Offset)
	rows, err := r.Pool.Query(ctx, `
SELECT id, actor_user_id::text, action, entity_type, entity_id, details,
       COALESCE(host(ip), ''), user_agent, created_at
FROM audit_log
WHERE ($1 = '' OR action = $1)
  AND ($2 = '' OR entity_type = $2)
  AND ($3 = '' OR entity_id = $3)
  AND ($4::timestamptz IS NULL OR created_at >= $4)
  AND ($5::timestamptz IS NULL OR created_at <= $5)
ORDER BY created_at DESC
LIMIT $6 OFFSET $7`,
		f.Action, f.EntityType, f.EntityID, f.From, f.To, f.Limit, f.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var (
			e      AuditEntry
			raw    []byte
			ip     string
			ua     *string
			action string
			entity *string
			entID  *string
		)
		if err := rows.Scan(
			&e.ID, &e.ActorUserID, &action, &entity, &entID, &raw,
			&ip, &ua, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		e.Action = action
		e.EntityType = entity
		e.EntityID = entID
		if len(raw) > 0 {
			e.Details = json.RawMessage(raw)
		} else {
			e.Details = json.RawMessage("null")
		}
		if ip != "" {
			e.IP = &ip
		}
		e.UserAgent = ua
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
