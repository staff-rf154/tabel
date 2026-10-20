package repo

import (
	"context"
)

const officeColumns = `id::text, name, lat, lon, radius_m, is_active`

func scanOffice(row interface{ Scan(...any) error }, o *Office) error {
	return row.Scan(&o.ID, &o.Name, &o.Lat, &o.Lon, &o.RadiusM, &o.IsActive)
}

func (r *Repo) ListOffices(ctx context.Context, activeOnly bool) ([]Office, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT `+officeColumns+`
FROM offices
WHERE (NOT $1 OR is_active)
ORDER BY name`, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var offices []Office
	for rows.Next() {
		var o Office
		if err := scanOffice(rows, &o); err != nil {
			return nil, err
		}
		offices = append(offices, o)
	}
	return offices, rows.Err()
}

func (r *Repo) CreateOffice(ctx context.Context, in OfficeInput) (Office, error) {
	radius := 500
	if in.RadiusM != nil {
		radius = *in.RadiusM
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	var o Office
	err := scanOffice(r.Pool.QueryRow(ctx, `
INSERT INTO offices (name, lat, lon, radius_m, is_active)
VALUES ($1, $2, $3, $4, $5)
RETURNING `+officeColumns, in.Name, in.Lat, in.Lon, radius, active), &o)
	return o, err
}

func (r *Repo) UpdateOffice(ctx context.Context, id string, in OfficeInput) (Office, error) {
	radius := 500
	if in.RadiusM != nil {
		radius = *in.RadiusM
	}
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	var o Office
	err := scanOffice(r.Pool.QueryRow(ctx, `
UPDATE offices
SET name = $2, lat = $3, lon = $4, radius_m = $5, is_active = $6, updated_at = now()
WHERE id = $1::uuid
RETURNING `+officeColumns, id, in.Name, in.Lat, in.Lon, radius, active), &o)
	if isNoRows(err) {
		return Office{}, ErrNotFound
	}
	return o, err
}

func (r *Repo) DeleteOffice(ctx context.Context, id string) error {
	tag, err := r.Pool.Exec(ctx, `DELETE FROM offices WHERE id = $1::uuid`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
