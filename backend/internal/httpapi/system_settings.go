package httpapi

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"tabel/internal/app"
	"tabel/internal/cache"
	tabeldb "tabel/internal/db"
	"tabel/internal/settings"
)

type servicePorts struct {
	Backend   int `json:"backend"`
	Admin     int `json:"admin"`
	Operator  int `json:"operator"`
	Reports   int `json:"reports"`
	Postgres  int `json:"postgres"`
	PgBouncer int `json:"pgbouncer"`
	Redis     int `json:"redis"`
	S3API     int `json:"s3_api"`
	S3Admin   int `json:"s3_admin"`
	HTTPS     int `json:"https"`
}

type pgBouncerSettings struct {
	Host           string `json:"host"`
	Port           int    `json:"port"`
	PoolMode       string `json:"pool_mode"`
	MaxClientConn  int    `json:"max_client_conn"`
	DefaultPoolSiz int    `json:"default_pool_size"`
	AuthType       string `json:"auth_type"`
}

type s3SettingsResponse struct {
	Endpoint     string `json:"endpoint"`
	Region       string `json:"region"`
	Bucket       string `json:"bucket"`
	AccessKeySet bool   `json:"access_key_set"`
	SecretKeySet bool   `json:"secret_key_set"`
	UseSSL       bool   `json:"use_ssl"`
	PathStyle    bool   `json:"path_style"`
}

type sslStatus struct {
	CertPath   string   `json:"cert_path"`
	KeyPath    string   `json:"key_path"`
	Ready      bool     `json:"ready"`
	Subject    string   `json:"subject"`
	Issuer     string   `json:"issuer"`
	NotAfter   string   `json:"not_after"`
	DaysLeft   int      `json:"days_left"`
	DNSNames   []string `json:"dns_names"`
	Error      string   `json:"error,omitempty"`
	ExpiringOK bool     `json:"expiring_soon"`
}

type demoStatus struct {
	Employees int64  `json:"employees"`
	Marks     int64  `json:"marks"`
	QRTokens  int64  `json:"qr_tokens"`
	LastReset string `json:"last_reset"`
}

type systemSettingsResponse struct {
	DatabaseURL          string              `json:"database_url"`
	DatabaseURLSet       bool                `json:"database_url_set"`
	DatabaseDirectURL    string              `json:"database_direct_url"`
	DatabaseDirectURLSet bool                `json:"database_direct_url_set"`
	RedisAddr            string              `json:"redis_addr"`
	RedisPasswordSet     bool                `json:"redis_password_set"`
	QRPrivateKeySet      bool                `json:"qr_private_key_set"`
	QRPublicKey          string              `json:"qr_public_key"`
	SettingsDBPath       string              `json:"settings_db_path"`
	SettingsDBSize       int64               `json:"settings_db_size"`
	AppEnv               string              `json:"app_env"`
	ServicePorts         servicePorts        `json:"service_ports"`
	PgBouncer            pgBouncerSettings   `json:"pgbouncer"`
	S3                   s3SettingsResponse  `json:"s3"`
	SSL                  sslStatus           `json:"ssl"`
	Demo                 demoStatus          `json:"demo"`
	PGRecommendations    []tabeldb.PGSetting `json:"postgres_recommendations"`
}

type servicePortsInput struct {
	Backend   *int `json:"backend"`
	Admin     *int `json:"admin"`
	Operator  *int `json:"operator"`
	Reports   *int `json:"reports"`
	Postgres  *int `json:"postgres"`
	PgBouncer *int `json:"pgbouncer"`
	Redis     *int `json:"redis"`
	S3API     *int `json:"s3_api"`
	S3Admin   *int `json:"s3_admin"`
	HTTPS     *int `json:"https"`
}

type pgBouncerInput struct {
	Host           string `json:"host"`
	Port           *int   `json:"port"`
	PoolMode       string `json:"pool_mode"`
	MaxClientConn  *int   `json:"max_client_conn"`
	DefaultPoolSiz *int   `json:"default_pool_size"`
	AuthType       string `json:"auth_type"`
}

type s3Input struct {
	Endpoint  string `json:"endpoint"`
	Region    string `json:"region"`
	Bucket    string `json:"bucket"`
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	UseSSL    *bool  `json:"use_ssl"`
	PathStyle *bool  `json:"path_style"`
}

type sslInput struct {
	CertPath string `json:"cert_path"`
	KeyPath  string `json:"key_path"`
}

type systemConnectionsInput struct {
	DatabaseURL       string             `json:"database_url"`
	DatabaseDirectURL string             `json:"database_direct_url"`
	RedisAddr         string             `json:"redis_addr"`
	RedisPassword     string             `json:"redis_password"`
	ServicePorts      *servicePortsInput `json:"service_ports"`
	PgBouncer         *pgBouncerInput    `json:"pgbouncer"`
	S3                *s3Input           `json:"s3"`
	SSL               *sslInput          `json:"ssl"`
}

type systemConnectionsResponse struct {
	systemSettingsResponse
	RestartRequired bool `json:"restart_required"`
}

func (s *Server) handleGetSystemSettings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.systemSettings(r.Context()))
}

func (s *Server) handleTestSystemConnections(w http.ResponseWriter, r *http.Request) {
	var req systemConnectionsInput
	if !decodeJSON(w, r, &req) {
		return
	}
	databaseURL, directURL, redisAddr, redisPassword := s.candidateConnections(req)
	if databaseURL == "" {
		writeError(w, http.StatusBadRequest, "не задана строка подключения PostgreSQL")
		return
	}
	if redisAddr == "" {
		writeError(w, http.StatusBadRequest, "не задан адрес Redis")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result := map[string]any{"status": "ok"}
	created, err := app.TestDatabase(ctx, databaseURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "PostgreSQL (рабочее подключение): "+err.Error())
		return
	}
	result["database"] = "ok"
	result["database_created"] = created

	if directURL != "" && directURL != databaseURL {
		directCreated, err := app.TestDatabase(ctx, directURL)
		if err != nil {
			writeError(w, http.StatusBadRequest, "PostgreSQL (прямое подключение): "+err.Error())
			return
		}
		result["database_direct"] = "ok"
		result["database_direct_created"] = directCreated
	}

	rdb := cache.New(redisAddr, redisPassword)
	defer func() { _ = rdb.Close() }()
	if err := rdb.Ping(ctx).Err(); err != nil {
		writeError(w, http.StatusBadRequest, "Redis: "+err.Error())
		return
	}
	result["redis"] = "ok"

	certPath, keyPath := s.resolveSSLPaths(req.SSL)
	status := inspectSSL(certPath, keyPath)
	result["ssl"] = status
	if certPath != "" || keyPath != "" {
		if !status.Ready {
			writeError(w, http.StatusBadRequest, "SSL: "+status.Error)
			return
		}
	}

	s3 := s.resolveS3(req.S3)
	if s3.Endpoint != "" {
		code, err := testS3Endpoint(ctx, s3.Endpoint, s3.UseSSL)
		if err != nil {
			writeError(w, http.StatusBadRequest, "S3: "+err.Error())
			return
		}
		result["s3"] = map[string]any{"status": "ok", "endpoint": s3.Endpoint, "http_status": code}
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleUpdateSystemSettings(w http.ResponseWriter, r *http.Request) {
	var req systemConnectionsInput
	if !decodeJSON(w, r, &req) {
		return
	}
	databaseURL, directURL, redisAddr, redisPassword := s.candidateConnections(req)
	if databaseURL == "" {
		writeError(w, http.StatusBadRequest, "не задана строка подключения PostgreSQL")
		return
	}
	if redisAddr == "" {
		writeError(w, http.StatusBadRequest, "не задан адрес Redis")
		return
	}
	if directURL != "" {
		if _, err := pgx.ParseConfig(directURL); err != nil {
			writeError(w, http.StatusBadRequest, "прямое подключение PostgreSQL: "+err.Error())
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if _, err := app.TestDatabase(ctx, databaseURL); err != nil {
		writeError(w, http.StatusBadRequest, "PostgreSQL: "+err.Error())
		return
	}
	rdb := cache.New(redisAddr, redisPassword)
	defer func() { _ = rdb.Close() }()
	if err := rdb.Ping(ctx).Err(); err != nil {
		writeError(w, http.StatusBadRequest, "Redis: "+err.Error())
		return
	}

	currentDatabaseURL := s.resolveDatabaseURL()
	currentDirectURL := s.resolveDatabaseDirectURL()
	currentRedisAddr, currentRedisPassword := s.resolveRedis()
	changed := make([]string, 0, 8)

	if value := strings.TrimSpace(req.DatabaseURL); value != "" && value != currentDatabaseURL {
		if err := s.store.SetString(settings.KeyDatabaseURL, value, true); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сохранить подключение PostgreSQL")
			return
		}
		changed = append(changed, settings.KeyDatabaseURL)
	}
	if value := strings.TrimSpace(req.DatabaseDirectURL); value != "" && value != currentDirectURL {
		if err := s.store.SetString(settings.KeyDatabaseDirectURL, value, true); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сохранить прямое подключение PostgreSQL")
			return
		}
		changed = append(changed, settings.KeyDatabaseDirectURL)
	}
	redisAddrChanged := false
	if value := strings.TrimSpace(req.RedisAddr); value != "" && value != currentRedisAddr {
		if err := s.store.SetString(settings.KeyRedisAddr, value, false); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сохранить адрес Redis")
			return
		}
		changed = append(changed, settings.KeyRedisAddr)
		redisAddrChanged = true
	}
	if value := strings.TrimSpace(req.RedisPassword); value != "" {
		if value != currentRedisPassword {
			if err := s.store.SetString(settings.KeyRedisPassword, value, true); err != nil {
				writeError(w, http.StatusInternalServerError, "не удалось сохранить пароль Redis")
				return
			}
			changed = append(changed, settings.KeyRedisPassword)
		}
	} else if redisAddrChanged && currentRedisPassword != "" {
		if err := s.store.SetString(settings.KeyRedisPassword, "", true); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось очистить пароль Redis")
			return
		}
		changed = append(changed, settings.KeyRedisPassword)
	}

	if req.ServicePorts != nil {
		ports := []struct {
			key   string
			value *int
			name  string
		}{
			{settings.KeyHTTPPort, req.ServicePorts.Backend, "backend"},
			{settings.KeyAdminPort, req.ServicePorts.Admin, "admin"},
			{settings.KeyOperatorPort, req.ServicePorts.Operator, "operator"},
			{settings.KeyReportsPort, req.ServicePorts.Reports, "reports"},
			{settings.KeyPostgresPort, req.ServicePorts.Postgres, "postgres"},
			{settings.KeyPgBouncerPublicPort, req.ServicePorts.PgBouncer, "pgbouncer"},
			{settings.KeyRedisPublicPort, req.ServicePorts.Redis, "redis"},
			{settings.KeyS3APIPort, req.ServicePorts.S3API, "s3_api"},
			{settings.KeyS3AdminPort, req.ServicePorts.S3Admin, "s3_admin"},
			{settings.KeyHTTPSPort, req.ServicePorts.HTTPS, "https"},
		}
		for _, item := range ports {
			if item.value == nil {
				continue
			}
			if err := savePort(s.store, item.key, *item.value); err != nil {
				writeError(w, http.StatusBadRequest, "порт "+item.name+": "+err.Error())
				return
			}
			changed = append(changed, item.key)
		}
	}

	if req.PgBouncer != nil {
		if v := strings.TrimSpace(req.PgBouncer.Host); v != "" && v != s.resolveString(settings.KeyPgBouncerHost, s.cfg.DefaultPgBouncerHost) {
			if err := s.store.SetString(settings.KeyPgBouncerHost, v, false); err != nil {
				writeError(w, http.StatusInternalServerError, "не удалось сохранить хост PgBouncer")
				return
			}
			changed = append(changed, settings.KeyPgBouncerHost)
		}
		if req.PgBouncer.Port != nil {
			if err := savePort(s.store, settings.KeyPgBouncerPort, *req.PgBouncer.Port); err != nil {
				writeError(w, http.StatusBadRequest, "порт PgBouncer: "+err.Error())
				return
			}
			changed = append(changed, settings.KeyPgBouncerPort)
		}
		if v := strings.TrimSpace(req.PgBouncer.PoolMode); v != "" {
			if v != "transaction" && v != "session" && v != "statement" {
				writeError(w, http.StatusBadRequest, "режим пула PgBouncer: допустимо transaction, session или statement")
				return
			}
			if err := s.store.SetString(settings.KeyPgBouncerPoolMode, v, false); err != nil {
				writeError(w, http.StatusInternalServerError, "не удалось сохранить режим PgBouncer")
				return
			}
			changed = append(changed, settings.KeyPgBouncerPoolMode)
		}
		if req.PgBouncer.MaxClientConn != nil {
			if err := savePositive(s.store, settings.KeyPgBouncerMaxClients, *req.PgBouncer.MaxClientConn); err != nil {
				writeError(w, http.StatusBadRequest, "max_client_conn: "+err.Error())
				return
			}
			changed = append(changed, settings.KeyPgBouncerMaxClients)
		}
		if req.PgBouncer.DefaultPoolSiz != nil {
			if err := savePositive(s.store, settings.KeyPgBouncerPoolSize, *req.PgBouncer.DefaultPoolSiz); err != nil {
				writeError(w, http.StatusBadRequest, "default_pool_size: "+err.Error())
				return
			}
			changed = append(changed, settings.KeyPgBouncerPoolSize)
		}
		if v := strings.TrimSpace(req.PgBouncer.AuthType); v != "" {
			if err := s.store.SetString(settings.KeyPgBouncerAuthType, v, false); err != nil {
				writeError(w, http.StatusInternalServerError, "не удалось сохранить auth_type PgBouncer")
				return
			}
			changed = append(changed, settings.KeyPgBouncerAuthType)
		}
	}

	if req.S3 != nil {
		s3changed, err := s.saveS3(req.S3)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		changed = append(changed, s3changed...)
	}
	if req.SSL != nil {
		sslchanged, err := s.saveSSL(req.SSL)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		changed = append(changed, sslchanged...)
	}

	if len(changed) > 0 {
		if rp, ok := s.repo(); ok {
			principal, _ := principalFrom(r.Context())
			s.audit(r, rp, &principal.UserID, "admin.system_settings_update", "settings", "", map[string]any{
				"changed": changed,
			})
		}
	}
	writeJSON(w, http.StatusOK, systemConnectionsResponse{
		systemSettingsResponse: s.systemSettings(r.Context()),
		RestartRequired:        len(changed) > 0,
	})
}

func (s *Server) systemSettings(ctx context.Context) systemSettingsResponse {
	databaseURL := s.resolveDatabaseURL()
	directURL := s.resolveDatabaseDirectURL()
	redisAddr, redisPassword := s.resolveRedis()
	_, qrKeyErr := s.store.GetString(settings.KeyQRPrivateKey)
	publicKey, err := s.qrMgr.PublicKeyPEM()
	if err != nil {
		publicKey = ""
	}
	if qrKeyErr != nil && !errors.Is(qrKeyErr, settings.ErrNotFound) {
		publicKey = ""
	}
	size := int64(0)
	if info, err := os.Stat(s.cfg.SettingsDBPath); err == nil {
		size = info.Size()
	}
	pb := s.resolvePgBouncer()
	s3 := s.resolveS3(nil)
	return systemSettingsResponse{
		DatabaseURL:          maskDatabaseURL(databaseURL),
		DatabaseURLSet:       databaseURL != "",
		DatabaseDirectURL:    maskDatabaseURL(directURL),
		DatabaseDirectURLSet: directURL != "",
		RedisAddr:            redisAddr,
		RedisPasswordSet:     redisPassword != "",
		QRPrivateKeySet:      qrKeyErr == nil,
		QRPublicKey:          publicKey,
		SettingsDBPath:       s.cfg.SettingsDBPath,
		SettingsDBSize:       size,
		AppEnv:               s.cfg.AppEnv,
		ServicePorts:         s.resolvePorts(),
		PgBouncer:            pb,
		S3: s3SettingsResponse{
			Endpoint:     s3.Endpoint,
			Region:       s3.Region,
			Bucket:       s3.Bucket,
			AccessKeySet: s3.AccessKey != "",
			SecretKeySet: s3.SecretKey != "",
			UseSSL:       s3.UseSSL,
			PathStyle:    s3.PathStyle,
		},
		SSL:               inspectSSL(s.resolveSSLPaths(nil)),
		Demo:              s.demoStatus(ctx),
		PGRecommendations: tabeldb.PostgresRecommendations(),
	}
}

func (s *Server) resolveDatabaseDirectURL() string {
	if v, err := s.store.GetString(settings.KeyDatabaseDirectURL); err == nil && v != "" {
		return v
	}
	return s.cfg.DefaultDatabaseDirectURL
}

func (s *Server) resolveString(key, fallback string) string {
	if v, err := s.store.GetString(key); err == nil && v != "" {
		return v
	}
	return fallback
}

func (s *Server) resolveInt(key string, fallback int) int {
	if v, err := s.store.GetString(key); err == nil && v != "" {
		if n, convErr := strconv.Atoi(v); convErr == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func (s *Server) resolveBool(key string, fallback bool) bool {
	if v, err := s.store.GetString(key); err == nil && v != "" {
		if b, convErr := strconv.ParseBool(v); convErr == nil {
			return b
		}
	}
	return fallback
}

func (s *Server) resolvePorts() servicePorts {
	d := s.cfg.ServicePorts
	return servicePorts{
		Backend:   s.resolveInt(settings.KeyHTTPPort, d.Backend),
		Admin:     s.resolveInt(settings.KeyAdminPort, d.Admin),
		Operator:  s.resolveInt(settings.KeyOperatorPort, d.Operator),
		Reports:   s.resolveInt(settings.KeyReportsPort, d.Reports),
		Postgres:  s.resolveInt(settings.KeyPostgresPort, d.Postgres),
		PgBouncer: s.resolveInt(settings.KeyPgBouncerPublicPort, d.PgBouncer),
		Redis:     s.resolveInt(settings.KeyRedisPublicPort, d.Redis),
		S3API:     s.resolveInt(settings.KeyS3APIPort, d.S3API),
		S3Admin:   s.resolveInt(settings.KeyS3AdminPort, d.S3Admin),
		HTTPS:     s.resolveInt(settings.KeyHTTPSPort, d.HTTPS),
	}
}

func (s *Server) resolvePgBouncer() pgBouncerSettings {
	return pgBouncerSettings{
		Host:           s.resolveString(settings.KeyPgBouncerHost, s.cfg.DefaultPgBouncerHost),
		Port:           s.resolveInt(settings.KeyPgBouncerPort, s.cfg.DefaultPgBouncerPort),
		PoolMode:       s.resolveString(settings.KeyPgBouncerPoolMode, s.cfg.DefaultPgBouncerPoolMode),
		MaxClientConn:  s.resolveInt(settings.KeyPgBouncerMaxClients, s.cfg.DefaultPgBouncerMaxClients),
		DefaultPoolSiz: s.resolveInt(settings.KeyPgBouncerPoolSize, s.cfg.DefaultPgBouncerPoolSize),
		AuthType:       s.resolveString(settings.KeyPgBouncerAuthType, s.cfg.DefaultPgBouncerAuthType),
	}
}

type s3Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	UseSSL    bool
	PathStyle bool
}

func (s *Server) resolveS3(input *s3Input) s3Config {
	out := s3Config{
		Endpoint:  s.resolveString(settings.KeyS3Endpoint, s.cfg.DefaultS3Endpoint),
		Region:    s.resolveString(settings.KeyS3Region, s.cfg.DefaultS3Region),
		Bucket:    s.resolveString(settings.KeyS3Bucket, s.cfg.DefaultS3Bucket),
		AccessKey: s.resolveString(settings.KeyS3AccessKey, s.cfg.DefaultS3AccessKey),
		SecretKey: s.resolveString(settings.KeyS3SecretKey, s.cfg.DefaultS3SecretKey),
		UseSSL:    s.resolveBool(settings.KeyS3UseSSL, s.cfg.DefaultS3UseSSL),
		PathStyle: s.resolveBool(settings.KeyS3PathStyle, s.cfg.DefaultS3PathStyle),
	}
	if input == nil {
		return out
	}
	if v := strings.TrimSpace(input.Endpoint); v != "" {
		out.Endpoint = v
	}
	if v := strings.TrimSpace(input.Region); v != "" {
		out.Region = v
	}
	if v := strings.TrimSpace(input.Bucket); v != "" {
		out.Bucket = v
	}
	if v := strings.TrimSpace(input.AccessKey); v != "" {
		out.AccessKey = v
	}
	if v := strings.TrimSpace(input.SecretKey); v != "" {
		out.SecretKey = v
	}
	if input.UseSSL != nil {
		out.UseSSL = *input.UseSSL
	}
	if input.PathStyle != nil {
		out.PathStyle = *input.PathStyle
	}
	return out
}

func (s *Server) resolveSSLPaths(input *sslInput) (string, string) {
	cert := s.resolveString(settings.KeySSLCertPath, s.cfg.DefaultSSLCertPath)
	key := s.resolveString(settings.KeySSLKeyPath, s.cfg.DefaultSSLKeyPath)
	if input != nil {
		if v := strings.TrimSpace(input.CertPath); v != "" {
			cert = v
		}
		if v := strings.TrimSpace(input.KeyPath); v != "" {
			key = v
		}
	}
	return cert, key
}

func (s *Server) candidateConnections(req systemConnectionsInput) (string, string, string, string) {
	databaseURL := strings.TrimSpace(req.DatabaseURL)
	if databaseURL == "" {
		databaseURL = s.resolveDatabaseURL()
	}
	directURL := strings.TrimSpace(req.DatabaseDirectURL)
	if directURL == "" {
		directURL = s.resolveDatabaseDirectURL()
	}
	currentAddr, currentPassword := s.resolveRedis()
	redisAddr := strings.TrimSpace(req.RedisAddr)
	if redisAddr == "" {
		redisAddr = currentAddr
	}
	if redisAddr == "" {
		redisAddr = s.cfg.DefaultRedisAddr
	}
	redisPassword := req.RedisPassword
	if redisPassword == "" && redisAddr == currentAddr {
		redisPassword = currentPassword
	}
	return databaseURL, directURL, redisAddr, redisPassword
}

func (s *Server) saveS3(input *s3Input) ([]string, error) {
	changed := make([]string, 0, 8)
	values := []struct {
		key    string
		value  string
		secret bool
	}{
		{settings.KeyS3Endpoint, strings.TrimSpace(input.Endpoint), false},
		{settings.KeyS3Region, strings.TrimSpace(input.Region), false},
		{settings.KeyS3Bucket, strings.TrimSpace(input.Bucket), false},
		{settings.KeyS3AccessKey, strings.TrimSpace(input.AccessKey), false},
		{settings.KeyS3SecretKey, strings.TrimSpace(input.SecretKey), true},
	}
	for _, item := range values {
		if item.value == "" {
			continue
		}
		if err := s.store.SetString(item.key, item.value, item.secret); err != nil {
			return changed, err
		}
		changed = append(changed, item.key)
	}
	if input.UseSSL != nil {
		if err := s.store.SetString(settings.KeyS3UseSSL, strconv.FormatBool(*input.UseSSL), false); err != nil {
			return changed, err
		}
		changed = append(changed, settings.KeyS3UseSSL)
	}
	if input.PathStyle != nil {
		if err := s.store.SetString(settings.KeyS3PathStyle, strconv.FormatBool(*input.PathStyle), false); err != nil {
			return changed, err
		}
		changed = append(changed, settings.KeyS3PathStyle)
	}
	return changed, nil
}

func (s *Server) saveSSL(input *sslInput) ([]string, error) {
	changed := make([]string, 0, 2)
	if v := strings.TrimSpace(input.CertPath); v != "" {
		if err := s.store.SetString(settings.KeySSLCertPath, v, false); err != nil {
			return changed, err
		}
		changed = append(changed, settings.KeySSLCertPath)
	}
	if v := strings.TrimSpace(input.KeyPath); v != "" {
		if err := s.store.SetString(settings.KeySSLKeyPath, v, false); err != nil {
			return changed, err
		}
		changed = append(changed, settings.KeySSLKeyPath)
	}
	return changed, nil
}

func savePort(store *settings.Store, key string, value int) error {
	if value < 1 || value > 65535 {
		return fmt.Errorf("значение должно быть от 1 до 65535")
	}
	return store.SetString(key, strconv.Itoa(value), false)
}

func savePositive(store *settings.Store, key string, value int) error {
	if value < 1 {
		return fmt.Errorf("значение должно быть положительным")
	}
	return store.SetString(key, strconv.Itoa(value), false)
}

func inspectSSL(certPath, keyPath string) sslStatus {
	out := sslStatus{CertPath: certPath, KeyPath: keyPath}
	if certPath == "" || keyPath == "" {
		out.Error = "не заданы пути к сертификату и ключу"
		return out
	}
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		out.Error = "разбор сертификата: " + err.Error()
		return out
	}
	now := time.Now()
	out.Ready = true
	out.Subject = leaf.Subject.String()
	out.Issuer = leaf.Issuer.String()
	out.NotAfter = leaf.NotAfter.Format(time.RFC3339)
	out.DaysLeft = int(leaf.NotAfter.Sub(now).Hours() / 24)
	out.DNSNames = append(out.DNSNames, leaf.DNSNames...)
	out.ExpiringOK = out.DaysLeft < 30
	if out.DaysLeft <= 0 {
		out.Error = "срок действия сертификата истёк"
		out.Ready = false
	}
	return out
}

func testS3Endpoint(ctx context.Context, endpoint string, useSSL bool) (int, error) {
	raw := strings.TrimSpace(endpoint)
	if !strings.Contains(raw, "://") {
		scheme := "http"
		if useSSL {
			scheme = "https"
		}
		raw = scheme + "://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return 0, fmt.Errorf("некорректный endpoint")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return 0, err
	}
	client := &http.Client{
		Timeout: 6 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	if resp.StatusCode >= 500 {
		return resp.StatusCode, fmt.Errorf("сервер вернул HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func (s *Server) demoStatus(ctx context.Context) demoStatus {
	var out demoStatus
	pool, _ := s.connections()
	if pool == nil {
		return out
	}
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_ = pool.QueryRow(queryCtx, `
SELECT (SELECT count(*) FROM employees),
       (SELECT count(*) FROM marks),
       (SELECT count(*) FROM qr_tokens),
       COALESCE((SELECT to_char(created_at, 'YYYY-MM-DD HH24:MI') FROM audit_log
                 WHERE action = 'demo.reset' ORDER BY created_at DESC LIMIT 1), '')
`).Scan(&out.Employees, &out.Marks, &out.QRTokens, &out.LastReset)
	return out
}

func (s *Server) handleListDBScripts(w http.ResponseWriter, _ *http.Request) {
	scripts, err := tabeldb.SetupScripts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать скрипты: "+err.Error())
		return
	}
	type item struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Size        int    `json:"size"`
	}
	out := make([]item, 0, len(scripts))
	for _, script := range scripts {
		out = append(out, item{
			Name:        script.Name,
			Title:       script.Title,
			Description: script.Description,
			Size:        len(script.Content),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDownloadDBScript(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	scripts, err := tabeldb.SetupScripts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать скрипты")
		return
	}
	for _, script := range scripts {
		if script.Name != name {
			continue
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename=\""+script.Name+"\"")
		_, _ = io.WriteString(w, script.Content)
		return
	}
	writeError(w, http.StatusNotFound, "скрипт не найден")
}

func (s *Server) handleDownloadDBScriptsZip(w http.ResponseWriter, _ *http.Request) {
	scripts, err := tabeldb.SetupScripts()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать скрипты")
		return
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, script := range scripts {
		f, err := zw.Create(script.Name)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сформировать архив")
			return
		}
		if _, err := io.WriteString(f, script.Content); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось сформировать архив")
			return
		}
	}
	if err := zw.Close(); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сформировать архив")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\"tabel-postgres-scripts.zip\"")
	_, _ = w.Write(buf.Bytes())
}

func (s *Server) handleDownloadSettingsDB(w http.ResponseWriter, r *http.Request) {
	tmp, err := os.CreateTemp("", "tabel-settings-*.db")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать временный файл")
		return
	}
	path := tmp.Name()
	_ = tmp.Close()
	_ = os.Remove(path)
	defer func() { _ = os.Remove(path) }()
	if err := s.store.Backup(path); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать копию базы настроек: "+err.Error())
		return
	}
	f, err := os.Open(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось открыть копию базы настроек")
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось прочитать копию базы настроек")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", "attachment; filename=\"settings.db\"")
	http.ServeContent(w, r, "settings.db", info.ModTime(), f)
}

func (s *Server) handleMigratePostgreSQL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TargetURL        string `json:"target_url"`
		TargetRuntimeURL string `json:"target_runtime_url"`
		SwitchConnection bool   `json:"switch_connection"`
		Confirm          string `json:"confirm"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(req.Confirm), "MIGRATE") {
		writeError(w, http.StatusBadRequest, "для переноса введите подтверждение MIGRATE")
		return
	}
	target := strings.TrimSpace(req.TargetURL)
	if target == "" {
		writeError(w, http.StatusBadRequest, "не задана строка подключения целевого PostgreSQL")
		return
	}
	source := s.resolveDatabaseDirectURL()
	if source == "" {
		writeError(w, http.StatusBadRequest, "сначала задайте прямое подключение к PostgreSQL (database_direct_url)")
		return
	}
	if source == target {
		writeError(w, http.StatusBadRequest, "целевая база совпадает с текущей")
		return
	}
	if _, err := exec.LookPath("pg_dump"); err != nil {
		writeError(w, http.StatusInternalServerError, "в образе backend отсутствует pg_dump")
		return
	}
	if _, err := exec.LookPath("pg_restore"); err != nil {
		writeError(w, http.StatusInternalServerError, "в образе backend отсутствует pg_restore")
		return
	}
	if !s.migrationMu.TryLock() {
		writeError(w, http.StatusConflict, "другой перенос уже выполняется")
		return
	}
	defer s.migrationMu.Unlock()

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	if _, err := tabeldb.EnsureDatabase(ctx, target); err != nil {
		writeError(w, http.StatusBadRequest, "целевой PostgreSQL: "+err.Error())
		return
	}

	tmp, err := os.CreateTemp("", "tabel-migrate-*.dump")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать временный файл")
		return
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer func() { _ = os.Remove(path) }()

	dump := exec.CommandContext(ctx, "pg_dump",
		"--dbname", source,
		"--format=custom",
		"--no-owner",
		"--no-privileges",
		"--file", path,
	)
	if out, err := dump.CombinedOutput(); err != nil {
		writeError(w, http.StatusBadGateway, "pg_dump: "+err.Error()+": "+trimOutput(out))
		return
	}
	restore := exec.CommandContext(ctx, "pg_restore",
		"--dbname", target,
		"--clean", "--if-exists",
		"--no-owner", "--no-privileges",
		"--single-transaction", "--exit-on-error",
		path,
	)
	if out, err := restore.CombinedOutput(); err != nil {
		writeError(w, http.StatusBadGateway, "pg_restore: "+err.Error()+": "+trimOutput(out))
		return
	}

	targetPool, err := tabeldb.Connect(ctx, target)
	if err != nil {
		writeError(w, http.StatusBadGateway, "проверка целевой базы: "+err.Error())
		return
	}
	var employees, marks int64
	if err := targetPool.QueryRow(ctx, `SELECT (SELECT count(*) FROM employees), (SELECT count(*) FROM marks)`).Scan(&employees, &marks); err != nil {
		targetPool.Close()
		writeError(w, http.StatusBadGateway, "проверка данных в целевой базе: "+err.Error())
		return
	}
	targetPool.Close()

	switched := false
	if req.SwitchConnection {
		runtimeURL := strings.TrimSpace(req.TargetRuntimeURL)
		if runtimeURL == "" {
			runtimeURL = target
		}
		redisAddr, redisPassword := s.resolveRedis()
		newPool, newRDB, err := app.Connect(ctx, runtimeURL, redisAddr, redisPassword)
		if err != nil {
			writeError(w, http.StatusBadGateway, "подключение к целевой базе: "+err.Error())
			return
		}
		if err := s.store.SetString(settings.KeyDatabaseURL, runtimeURL, true); err != nil {
			newPool.Close()
			_ = newRDB.Close()
			writeError(w, http.StatusInternalServerError, "не удалось сохранить рабочее подключение")
			return
		}
		if err := s.store.SetString(settings.KeyDatabaseDirectURL, target, true); err != nil {
			newPool.Close()
			_ = newRDB.Close()
			writeError(w, http.StatusInternalServerError, "не удалось сохранить прямое подключение")
			return
		}
		oldPool, oldRDB := s.connections()
		s.SetConnections(newPool, newRDB)
		if oldPool != nil {
			oldPool.Close()
		}
		if oldRDB != nil {
			_ = oldRDB.Close()
		}
		switched = true
	}

	principal, _ := principalFrom(r.Context())
	if rp, ok := s.repo(); ok {
		s.audit(r, rp, &principal.UserID, "admin.postgres_migrate", "system", "", map[string]any{
			"target_url": maskDatabaseURL(target),
			"switched":   switched,
			"employees":  employees,
			"marks":      marks,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           "migrated",
		"switched":         switched,
		"employees":        employees,
		"marks":            marks,
		"target_url":       maskDatabaseURL(target),
		"restart_required": !switched,
	})
}

func trimOutput(raw []byte) string {
	text := strings.TrimSpace(string(raw))
	if len(text) > 1500 {
		text = text[:1500] + "…"
	}
	return text
}

func maskDatabaseURL(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "задано"
	}
	if parsed.User != nil {
		user := parsed.User.Username()
		if _, hasPassword := parsed.User.Password(); hasPassword {
			parsed.User = url.UserPassword(user, "***")
		}
	}
	return strings.ReplaceAll(parsed.String(), "%2A%2A%2A", "***")
}

func (s *Server) handleTestSSL(w http.ResponseWriter, r *http.Request) {
	var req sslInput
	if !decodeJSON(w, r, &req) {
		return
	}
	certPath, keyPath := s.resolveSSLPaths(&req)
	status := inspectSSL(certPath, keyPath)
	if !status.Ready {
		writeError(w, http.StatusBadRequest, "SSL: "+status.Error)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *Server) handleTestS3(w http.ResponseWriter, r *http.Request) {
	var req s3Input
	if !decodeJSON(w, r, &req) {
		return
	}
	cfg := s.resolveS3(&req)
	if cfg.Endpoint == "" {
		writeError(w, http.StatusBadRequest, "не задан endpoint S3")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	code, err := testS3Endpoint(ctx, cfg.Endpoint, cfg.UseSSL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "S3: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "endpoint": cfg.Endpoint, "http_status": code})
}
