package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const markColumns = `
	id::text, employee_id::text, operator_id::text, operator_number, event_type,
	timestamp_client, timestamp_server, lat, lon, accuracy_m, location_age_sec,
	device_os, device_os_version, device_model, app_version, mock_location,
	outside_office, overtime, auto_closed_previous, qr_source, qr_token_id::text, qr_data_mismatch,
	client_type, shift_id::text`

const markColumnsWithEmployee = `
	m.id::text, m.employee_id::text, m.operator_id::text, m.operator_number, m.event_type,
	m.timestamp_client, m.timestamp_server, m.lat, m.lon, m.accuracy_m, m.location_age_sec,
	m.device_os, m.device_os_version, m.device_model, m.app_version, m.mock_location,
	m.outside_office, m.overtime, m.auto_closed_previous, m.qr_source, m.qr_token_id::text, m.qr_data_mismatch,
	m.client_type, m.shift_id::text, e.employee_number, e.full_name,
	CASE
		WHEN NOT e.is_active THEN 'сотрудник деактивирован'
		WHEN EXISTS (SELECT 1 FROM users u WHERE u.employee_id = e.id AND u.auth_blocked) THEN 'пользователь заблокирован'
		WHEN qt.status = 'revoked' THEN 'QR-токен отозван'
		WHEN qt.status IS NOT NULL AND qt.status <> 'active' THEN 'QR-токен неактивен'
		WHEN m.qr_data_mismatch THEN 'расхождение данных QR'
		ELSE NULL
	END`

func markDest(m *Mark) []any {
	return []any{
		&m.ID, &m.EmployeeID, &m.OperatorID, &m.OperatorNumber, &m.EventType,
		&m.TimestampClient, &m.TimestampServer, &m.Lat, &m.Lon, &m.AccuracyM, &m.LocationAgeSec,
		&m.DeviceOS, &m.DeviceOSVersion, &m.DeviceModel, &m.AppVersion, &m.MockLocation,
		&m.OutsideOffice, &m.Overtime, &m.AutoClosedPrevious, &m.QRSource, &m.QRTokenID, &m.QRDataMismatch,
		&m.ClientType, &m.ShiftID,
	}
}

func scanMark(row interface{ Scan(...any) error }, m *Mark) error {
	return row.Scan(markDest(m)...)
}

func scanMarkWithEmployee(row interface{ Scan(...any) error }, m *Mark) error {
	return row.Scan(append(markDest(m), &m.EmployeeNumber, &m.FullName, &m.InvalidReason)...)
}

func getMarkByID(ctx context.Context, q querier, id string) (Mark, error) {
	var m Mark
	err := scanMark(q.QueryRow(ctx, `SELECT `+markColumns+` FROM marks WHERE id = $1::uuid ORDER BY timestamp_server DESC LIMIT 1`, id), &m)
	if isNoRows(err) {
		return m, ErrNotFound
	}
	return m, err
}

// CreateMark создаёт отметку с защитой от повторов по Idempotency-Key,
// дедупликацией одинаковых событий в пределах 2 минут, ведением смен и
// автоматическим закрытием забытого выхода при новой отметке прихода.
func (r *Repo) CreateMark(
	ctx context.Context,
	employeeID string,
	operatorID, operatorNumber *string,
	qrSource string,
	in MarkInput,
	idempotencyKey string,
	autoClose *AutoCloseInput,
) (Mark, bool, *AutoClosure, error) {
	tx, err := r.Pool.Begin(ctx)
	if err != nil {
		return Mark{}, false, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var existingMarkID *string
	err = tx.QueryRow(ctx, `
SELECT mark_id::text FROM mark_idempotency WHERE idempotency_key = $1::uuid`, idempotencyKey).Scan(&existingMarkID)
	switch {
	case err == nil && existingMarkID != nil:
		mark, err := getMarkByID(ctx, tx, *existingMarkID)
		if err != nil {
			return Mark{}, false, nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Mark{}, false, nil, err
		}
		return mark, false, nil, nil
	case err == nil:
		return Mark{}, false, nil, ErrIdempotencyConflict
	case err != nil && !isNoRows(err):
		return Mark{}, false, nil, err
	}

	var duplicate Mark
	err = scanMark(tx.QueryRow(ctx, `
SELECT `+markColumns+`
FROM marks
WHERE employee_id = $1::uuid
  AND auto_closed_previous = false
ORDER BY timestamp_server DESC
LIMIT 1`, employeeID), &duplicate)
	switch {
	case err == nil && duplicate.EventType == in.EventType && duplicate.TimestampServer.After(time.Now().UTC().Add(-2*time.Minute)):
		if _, err := tx.Exec(ctx, `
INSERT INTO mark_idempotency (idempotency_key, mark_id, employee_id)
VALUES ($1::uuid, $2::uuid, $3::uuid)
ON CONFLICT (idempotency_key) DO NOTHING`, idempotencyKey, duplicate.ID, employeeID); err != nil {
			return Mark{}, false, nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Mark{}, false, nil, err
		}
		return duplicate, false, nil, nil
	case err != nil && !isNoRows(err):
		return Mark{}, false, nil, err
	}

	var reserved string
	err = tx.QueryRow(ctx, `
INSERT INTO mark_idempotency (idempotency_key, employee_id)
VALUES ($1::uuid, $2::uuid)
ON CONFLICT (idempotency_key) DO NOTHING
RETURNING idempotency_key::text`, idempotencyKey, employeeID).Scan(&reserved)
	if isNoRows(err) {
		return Mark{}, false, nil, ErrIdempotencyConflict
	}
	if err != nil {
		return Mark{}, false, nil, err
	}
	var employeeActive bool
	err = tx.QueryRow(ctx, `SELECT is_active FROM employees WHERE id = $1::uuid FOR UPDATE`, employeeID).Scan(&employeeActive)
	if isNoRows(err) {
		return Mark{}, false, nil, ErrNotFound
	}
	if err != nil {
		return Mark{}, false, nil, err
	}
	if !employeeActive {
		return Mark{}, false, nil, ErrEmployeeInactive
	}
	openShift, err := getOpenShiftTx(ctx, tx, employeeID)
	if err != nil {
		return Mark{}, false, nil, err
	}

	if in.EventType == "check_in" && openShift != nil && autoClose == nil {
		return Mark{}, false, nil, ErrOpenShiftExists
	}
	if in.EventType == "check_out" && openShift == nil {
		return Mark{}, false, nil, ErrInvalidTransition
	}

	var closure *AutoClosure
	if in.EventType == "check_in" && openShift != nil {
		synthetic, err := insertMark(ctx, tx, markInsert{
			EmployeeID:         employeeID,
			EventType:          "check_out",
			TimestampServer:    autoClose.At,
			Overtime:           autoClose.Overtime,
			AutoClosedPrevious: true,
			ShiftID:            &openShift.ID,
			IdempotencyKey:     uuid.NewString(),
		})
		if err != nil {
			return Mark{}, false, nil, err
		}
		if _, err := tx.Exec(ctx, `
UPDATE shifts
SET ended_at = $2, status = 'auto_closed', overtime = overtime OR $3,
    check_out_mark_id = $4::uuid, updated_at = now()
WHERE id = $1::uuid`, openShift.ID, autoClose.At, autoClose.Overtime, synthetic.ID); err != nil {
			return Mark{}, false, nil, err
		}
		if err := insertAutoCloseNotification(ctx, tx, employeeID, openShift.ID, synthetic.ID, autoClose.At); err != nil {
			return Mark{}, false, nil, err
		}
		closure = &AutoClosure{ShiftID: openShift.ID, MarkID: synthetic.ID, ClosedAt: autoClose.At}
	}

	mark, err := insertMark(ctx, tx, markInsert{
		EmployeeID:      employeeID,
		OperatorID:      operatorID,
		OperatorNumber:  operatorNumber,
		EventType:       in.EventType,
		TimestampServer: in.TimestampServer,
		TimestampClient: in.TimestampClient,
		Lat:             in.Lat,
		Lon:             in.Lon,
		AccuracyM:       in.AccuracyM,
		LocationAgeSec:  in.LocationAgeSec,
		DeviceOS:        in.DeviceOS,
		DeviceOSVersion: in.DeviceOSVersion,
		DeviceModel:     in.DeviceModel,
		AppVersion:      in.AppVersion,
		MockLocation:    in.MockLocation,
		OutsideOffice:   in.OutsideOffice,
		Overtime:        in.Overtime,
		QRSource:        ptrOrNil(qrSource),
		QRTokenID:       in.QRTokenID,
		QRDataMismatch:  in.QRDataMismatch,
		ClientType:      &in.ClientType,
		IdempotencyKey:  idempotencyKey,
	})
	if err != nil {
		return Mark{}, false, nil, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE mark_idempotency SET mark_id = $2::uuid
WHERE idempotency_key = $1::uuid`, idempotencyKey, mark.ID); err != nil {
		return Mark{}, false, nil, err
	}

	if in.EventType == "check_in" {
		var shiftID string
		err = tx.QueryRow(ctx, `
INSERT INTO shifts (employee_id, schedule_id, check_in_mark_id, started_at, status, overtime)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, 'open', $5)
RETURNING id::text`, employeeID, in.ScheduleID, mark.ID, in.TimestampServer, in.Overtime).Scan(&shiftID)
		if err != nil {
			return Mark{}, false, nil, err
		}
		if err := setMarkShift(ctx, tx, mark.ID, in.TimestampServer, shiftID); err != nil {
			return Mark{}, false, nil, err
		}
		mark.ShiftID = &shiftID
	} else {
		if _, err := tx.Exec(ctx, `
UPDATE shifts
SET ended_at = $2, status = 'closed', overtime = overtime OR $3,
    check_out_mark_id = $4::uuid, updated_at = now()
WHERE id = $1::uuid`, openShift.ID, in.TimestampServer, in.Overtime, mark.ID); err != nil {
			return Mark{}, false, nil, err
		}
		if err := setMarkShift(ctx, tx, mark.ID, in.TimestampServer, openShift.ID); err != nil {
			return Mark{}, false, nil, err
		}
		mark.ShiftID = &openShift.ID
	}

	if err := tx.Commit(ctx); err != nil {
		return Mark{}, false, nil, err
	}
	return mark, true, closure, nil
}

type markInsert struct {
	EmployeeID         string
	OperatorID         *string
	OperatorNumber     *string
	EventType          string
	TimestampServer    time.Time
	TimestampClient    *time.Time
	Lat                *float64
	Lon                *float64
	AccuracyM          *float64
	LocationAgeSec     *int
	DeviceOS           *string
	DeviceOSVersion    *string
	DeviceModel        *string
	AppVersion         *string
	MockLocation       bool
	OutsideOffice      bool
	Overtime           bool
	AutoClosedPrevious bool
	QRSource           *string
	QRTokenID          *string
	QRDataMismatch     bool
	ClientType         *string
	ShiftID            *string
	IdempotencyKey     string
}

func insertMark(ctx context.Context, tx pgx.Tx, in markInsert) (Mark, error) {
	if err := ensureMarksPartition(ctx, tx, in.TimestampServer); err != nil {
		return Mark{}, err
	}
	var mark Mark
	err := scanMark(tx.QueryRow(ctx, `
INSERT INTO marks (
    employee_id, operator_id, operator_number, event_type, timestamp_server,
    timestamp_client, lat, lon, accuracy_m, location_age_sec, device_os,
    device_os_version, device_model, app_version, mock_location, outside_office,
    overtime, auto_closed_previous, qr_source, qr_token_id, qr_data_mismatch, client_type,
    shift_id, idempotency_key
) VALUES (
    $1::uuid, $2::uuid, $3, $4, $5,
    $6, $7, $8, $9, $10, $11,
    $12, $13, $14, $15, $16,
    $17, $18, $19, $20::uuid, $21, $22,
    $23::uuid, $24::uuid
)
RETURNING `+markColumns,
		in.EmployeeID, in.OperatorID, in.OperatorNumber, in.EventType, in.TimestampServer,
		in.TimestampClient, in.Lat, in.Lon, in.AccuracyM, in.LocationAgeSec, in.DeviceOS,
		in.DeviceOSVersion, in.DeviceModel, in.AppVersion, in.MockLocation, in.OutsideOffice,
		in.Overtime, in.AutoClosedPrevious, in.QRSource, in.QRTokenID, in.QRDataMismatch, in.ClientType,
		in.ShiftID, in.IdempotencyKey), &mark)
	return mark, err
}

func ensureMarksPartition(ctx context.Context, tx pgx.Tx, ts time.Time) error {
	_, err := tx.Exec(ctx, `SELECT create_marks_partition($1::date)`, ts.UTC().Format("2006-01-02"))
	return err
}

func setMarkShift(ctx context.Context, tx pgx.Tx, markID string, timestamp time.Time, shiftID string) error {
	_, err := tx.Exec(ctx, `
UPDATE marks SET shift_id = $2::uuid
WHERE id = $1::uuid AND timestamp_server = $3`, markID, shiftID, timestamp)
	return err
}

func getOpenShiftTx(ctx context.Context, tx pgx.Tx, employeeID string) (*Shift, error) {
	var s Shift
	err := scanShift(tx.QueryRow(ctx, `
SELECT `+shiftColumns+`
FROM shifts
WHERE employee_id = $1::uuid AND status = 'open'
ORDER BY started_at DESC
LIMIT 1
FOR UPDATE`, employeeID), &s)
	if isNoRows(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func insertAutoCloseNotification(ctx context.Context, tx pgx.Tx, employeeID, shiftID, markID string, closedAt time.Time) error {
	var targetUserID *string
	err := tx.QueryRow(ctx, `
SELECT id::text FROM users
WHERE employee_id = $1::uuid
ORDER BY registered_at NULLS LAST
LIMIT 1`, employeeID).Scan(&targetUserID)
	if err != nil && !isNoRows(err) {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"employee_id": employeeID,
		"shift_id":    shiftID,
		"mark_id":     markID,
		"closed_at":   closedAt.UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	body := fmt.Sprintf("Смена закрыта автоматически %s: не было отметки ухода.",
		closedAt.In(time.FixedZone("NOV", 7*60*60)).Format("02.01.2006 15:04"))
	_, err = tx.Exec(ctx, `
INSERT INTO notifications (type, title, body, payload, target_user_id)
VALUES ('shift.auto_closed', 'Смена закрыта автоматически', $1, $2::jsonb, $3::uuid)`,
		body, string(payload), targetUserID)
	return err
}

func (r *Repo) ListMarks(ctx context.Context, filter MarkFilter) ([]Mark, error) {
	limit := normalizedLimit(filter.Limit, 100, 1000)
	offset := normalizedOffset(filter.Offset)
	rows, err := r.Pool.Query(ctx, `
SELECT `+markColumnsWithEmployee+`
FROM marks m
JOIN employees e ON e.id = m.employee_id
LEFT JOIN qr_tokens qt ON qt.id = m.qr_token_id
WHERE (NULLIF($1, '') IS NULL OR m.employee_id = NULLIF($1, '')::uuid)
  AND (NULLIF($2, '') IS NULL OR m.operator_id = NULLIF($2, '')::uuid)
  AND ($3::timestamptz IS NULL OR m.timestamp_server >= $3)
  AND ($4::timestamptz IS NULL OR m.timestamp_server <= $4)
  AND ($5::boolean IS NULL OR m.outside_office = $5)
  AND ($6::boolean IS NULL OR m.mock_location = $6)
  AND ($7::boolean IS NULL OR m.auto_closed_previous = $7)
  AND ($8::boolean IS NULL OR m.overtime = $8)
  AND ($9 = '' OR m.qr_source = $9)
  AND ($10 = '' OR m.client_type = $10)
  AND ($11::boolean IS NULL OR m.qr_data_mismatch = $11)
  AND ($12 = '' OR e.employee_number ILIKE '%' || $12 || '%' OR e.full_name ILIKE '%' || $12 || '%')
  AND (NULLIF($13, '')::uuid IS NULL OR e.department_id = NULLIF($13, '')::uuid)
  AND (NULLIF($14, '')::uuid IS NULL OR e.position_id = NULLIF($14, '')::uuid)
ORDER BY m.timestamp_server DESC
LIMIT $15 OFFSET $16`,
		filter.EmployeeID, filter.OperatorID, filter.From, filter.To,
		filter.OutsideOffice, filter.MockLocation, filter.AutoClosedPrevious, filter.Overtime,
		filter.QRSource, filter.ClientType, filter.QRDataMismatch, filter.Query,
		filter.DepartmentID, filter.PositionID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var marks []Mark
	for rows.Next() {
		var m Mark
		if err := scanMarkWithEmployee(rows, &m); err != nil {
			return nil, err
		}
		marks = append(marks, m)
	}
	return marks, rows.Err()
}

func (r *Repo) ListMarksByEmployee(ctx context.Context, employeeID string, from, to *time.Time, limit int) ([]Mark, error) {
	return r.ListMarks(ctx, MarkFilter{EmployeeID: employeeID, From: from, To: to, Limit: limit})
}
