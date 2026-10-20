package repo

import (
	"context"
	"time"
)

type MobileBuild struct {
	ID         string    `json:"id"`
	Platform   string    `json:"platform"`
	Version    string    `json:"version"`
	Filename   string    `json:"filename"`
	StoredPath string    `json:"-"`
	SizeBytes  int64     `json:"size_bytes"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
}

const mobileBuildColumns = `id::text, platform, version, filename, stored_path, size_bytes, is_active, created_at`

func scanMobileBuild(row interface{ Scan(...any) error }, b *MobileBuild) error {
	return row.Scan(&b.ID, &b.Platform, &b.Version, &b.Filename, &b.StoredPath, &b.SizeBytes, &b.IsActive, &b.CreatedAt)
}

func (r *Repo) CreateMobileBuild(ctx context.Context, platform, version, filename, storedPath string, size int64, uploadedBy *string) (MobileBuild, error) {
	var build MobileBuild
	err := scanMobileBuild(r.Pool.QueryRow(ctx, `
INSERT INTO mobile_builds (platform, version, filename, stored_path, size_bytes, uploaded_by)
VALUES ($1, $2, $3, $4, $5, $6::uuid)
RETURNING `+mobileBuildColumns, platform, version, filename, storedPath, size, uploadedBy), &build)
	return build, err
}

func (r *Repo) ListMobileBuilds(ctx context.Context) ([]MobileBuild, error) {
	rows, err := r.Pool.Query(ctx, `SELECT `+mobileBuildColumns+` FROM mobile_builds ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var builds []MobileBuild
	for rows.Next() {
		var build MobileBuild
		if err := scanMobileBuild(rows, &build); err != nil {
			return nil, err
		}
		builds = append(builds, build)
	}
	return builds, rows.Err()
}

func (r *Repo) GetMobileBuild(ctx context.Context, id string) (MobileBuild, error) {
	var build MobileBuild
	err := scanMobileBuild(r.Pool.QueryRow(ctx, `SELECT `+mobileBuildColumns+` FROM mobile_builds WHERE id = $1::uuid`, id), &build)
	if isNoRows(err) {
		return build, ErrNotFound
	}
	return build, err
}

func (r *Repo) DeleteMobileBuild(ctx context.Context, id string) error {
	tag, err := r.Pool.Exec(ctx, `DELETE FROM mobile_builds WHERE id = $1::uuid`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
