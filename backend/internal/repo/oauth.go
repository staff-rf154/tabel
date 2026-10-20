package repo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net"
	"time"
)

const oauthSecretTTL = 33 * 24 * time.Hour

const oauthClientColumns = `
	id::text, client_id, name, is_active, secret_rotated_at, next_rotation_at`

func scanOAuthClient(row interface{ Scan(...any) error }, c *OAuthClient) error {
	return row.Scan(&c.ID, &c.ClientID, &c.Name, &c.IsActive, &c.SecretRotatedAt, &c.NextRotationAt)
}

func HashOAuthSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func RandomURLToken(bytesCount int) (string, error) {
	buf := make([]byte, bytesCount)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func (r *Repo) CreateOAuthClient(ctx context.Context, name string) (OAuthClientCreated, error) {
	clientIDBytes := make([]byte, 8)
	if _, err := rand.Read(clientIDBytes); err != nil {
		return OAuthClientCreated{}, err
	}
	clientID := "tabel_" + hex.EncodeToString(clientIDBytes)
	secret, err := RandomURLToken(32)
	if err != nil {
		return OAuthClientCreated{}, err
	}
	var client OAuthClient
	err = scanOAuthClient(r.Pool.QueryRow(ctx, `
INSERT INTO oauth_clients (client_id, client_secret_hash, name, secret_rotated_at, next_rotation_at)
VALUES ($1, $2, NULLIF($3, ''), now(), now() + interval '33 days')
RETURNING `+oauthClientColumns, clientID, HashOAuthSecret(secret), name), &client)
	if err != nil {
		return OAuthClientCreated{}, err
	}
	return OAuthClientCreated{OAuthClient: client, ClientSecret: secret}, nil
}

func (r *Repo) ListOAuthClients(ctx context.Context) ([]OAuthClient, error) {
	rows, err := r.Pool.Query(ctx, `SELECT `+oauthClientColumns+` FROM oauth_clients ORDER BY created_at DESC`)
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

func (r *Repo) RotateOAuthSecret(ctx context.Context, id string) (OAuthClientCreated, error) {
	secret, err := RandomURLToken(32)
	if err != nil {
		return OAuthClientCreated{}, err
	}
	var client OAuthClient
	err = scanOAuthClient(r.Pool.QueryRow(ctx, `
UPDATE oauth_clients
SET client_secret_hash = $2, secret_rotated_at = now(),
    next_rotation_at = now() + interval '33 days', updated_at = now()
WHERE id = $1::uuid
RETURNING `+oauthClientColumns, id, HashOAuthSecret(secret)), &client)
	if isNoRows(err) {
		return OAuthClientCreated{}, ErrNotFound
	}
	if err != nil {
		return OAuthClientCreated{}, err
	}
	if _, err := r.Pool.Exec(ctx, `DELETE FROM oauth_access_tokens WHERE client_id = $1`, client.ClientID); err != nil {
		return OAuthClientCreated{}, err
	}
	return OAuthClientCreated{OAuthClient: client, ClientSecret: secret}, nil
}

func (r *Repo) OAuthClientByUUID(ctx context.Context, id string) (OAuthClient, error) {
	var client OAuthClient
	err := scanOAuthClient(r.Pool.QueryRow(ctx, `SELECT `+oauthClientColumns+` FROM oauth_clients WHERE id = $1::uuid`, id), &client)
	if isNoRows(err) {
		return client, ErrNotFound
	}
	return client, err
}

func (r *Repo) AuthenticateOAuthClient(ctx context.Context, clientID, secret string) (OAuthClient, error) {
	var client OAuthClient
	var hash string
	err := r.Pool.QueryRow(ctx, `
SELECT `+oauthClientColumns+`, client_secret_hash
FROM oauth_clients
WHERE client_id = $1 AND is_active`, clientID).Scan(
		&client.ID, &client.ClientID, &client.Name, &client.IsActive,
		&client.SecretRotatedAt, &client.NextRotationAt, &hash,
	)
	if isNoRows(err) {
		return client, ErrNotFound
	}
	if err != nil {
		return client, err
	}
	if HashOAuthSecret(secret) != hash {
		return client, ErrNotFound
	}
	return client, nil
}

func (r *Repo) CreateOAuthAccessToken(ctx context.Context, clientID, tokenHash string, expiresAt time.Time) error {
	_, err := r.Pool.Exec(ctx, `
INSERT INTO oauth_access_tokens (client_id, token_hash, expires_at)
VALUES ($1, $2, $3)`, clientID, tokenHash, expiresAt)
	return err
}

func (r *Repo) FindOAuthAccessToken(ctx context.Context, tokenHash string) (OAuthAccess, error) {
	var access OAuthAccess
	err := r.Pool.QueryRow(ctx, `
SELECT c.id::text, t.client_id, t.expires_at
FROM oauth_access_tokens t
JOIN oauth_clients c ON c.client_id = t.client_id
WHERE t.token_hash = $1 AND t.expires_at > now() AND c.is_active`, tokenHash).Scan(
		&access.ClientUUID, &access.ClientID, &access.ExpiresAt,
	)
	if isNoRows(err) {
		return access, ErrNotFound
	}
	return access, err
}

func (r *Repo) ListIPWhitelist(ctx context.Context, clientUUID string) ([]IpWhitelistEntry, error) {
	rows, err := r.Pool.Query(ctx, `
SELECT id::text, oauth_client_id::text, cidr::text, description
FROM ip_whitelist
WHERE oauth_client_id = $1::uuid
ORDER BY created_at`, clientUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []IpWhitelistEntry
	for rows.Next() {
		var entry IpWhitelistEntry
		if err := rows.Scan(&entry.ID, &entry.OAuthClientID, &entry.CIDR, &entry.Description); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (r *Repo) AddIPWhitelist(ctx context.Context, clientUUID, cidr string, description *string) (IpWhitelistEntry, error) {
	var entry IpWhitelistEntry
	err := r.Pool.QueryRow(ctx, `
INSERT INTO ip_whitelist (oauth_client_id, cidr, description)
VALUES ($1::uuid, $2::cidr, $3)
RETURNING id::text, oauth_client_id::text, cidr::text, description`,
		clientUUID, cidr, description).Scan(
		&entry.ID, &entry.OAuthClientID, &entry.CIDR, &entry.Description,
	)
	return entry, err
}

func (r *Repo) IPAllowed(ctx context.Context, clientUUID string, ip net.IP) (bool, error) {
	entries, err := r.ListIPWhitelist(ctx, clientUUID)
	if err != nil {
		return false, err
	}
	if len(entries) == 0 {
		return true, nil
	}
	for _, entry := range entries {
		_, network, err := net.ParseCIDR(entry.CIDR)
		if err != nil {
			continue
		}
		if network.Contains(ip) {
			return true, nil
		}
	}
	return false, nil
}
