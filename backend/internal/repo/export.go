package repo

import (
	"context"
	"time"
)

func (r *Repo) ExportMarks(ctx context.Context, since *time.Time, cursorTS *time.Time, cursorID string, limit int) ([]ExportMark, error) {
	limit = normalizedLimit(limit, 1000, 1000)
	rows, err := r.Pool.Query(ctx, `
SELECT m.id::text, e.employee_number, e.full_name, m.event_type,
       m.timestamp_server, m.timestamp_client, m.operator_number,
       m.qr_source, m.client_type
FROM marks m
JOIN employees e ON e.id = m.employee_id
WHERE ($1::timestamptz IS NULL OR m.timestamp_server >= $1)
  AND ($2::timestamptz IS NULL OR (m.timestamp_server, m.id) > ($2, $3::uuid))
ORDER BY m.timestamp_server, m.id
LIMIT $4`, since, cursorTS, nullUUID(cursorID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var items []ExportMark
	for rows.Next() {
		var item ExportMark
		if err := rows.Scan(
			&item.MarkID, &item.EmployeeNumber, &item.EmployeeFullName, &item.EventType,
			&item.TimestampServer, &item.TimestampClient, &item.OperatorNumber,
			&item.QRSource, &item.ClientType,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repo) AckExports(ctx context.Context, clientID string, markIDs []string) (int, error) {
	acknowledged := 0
	for _, markID := range markIDs {
		tag, err := r.Pool.Exec(ctx, `
INSERT INTO export_acks (mark_id, client_id)
VALUES ($1::uuid, $2)
ON CONFLICT (mark_id) DO NOTHING`, markID, clientID)
		if err != nil {
			return acknowledged, err
		}
		acknowledged += int(tag.RowsAffected())
	}
	return acknowledged, nil
}

func (r *Repo) RecordOfflineImport(ctx context.Context, uploadedBy *string, filename string, total, ok, failed int, status string, report any) error {
	_, err := r.Pool.Exec(ctx, `
INSERT INTO offline_imports (uploaded_by, filename, rows_total, rows_ok, rows_error, status, report)
VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::jsonb)`,
		uploadedBy, filename, total, ok, failed, status, marshalJSON(report))
	return err
}
