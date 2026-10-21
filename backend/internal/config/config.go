package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type ServicePorts struct {
	Backend   int
	Admin     int
	Operator  int
	Reports   int
	Postgres  int
	PgBouncer int
	Redis     int
	S3API     int
	S3Admin   int
	HTTPS     int
}

type Config struct {
	AppEnv             string
	HTTPPort           string
	SettingsDBPath     string
	EncryptionKey      string
	CORSAllowedOrigins []string

	JWTSecret     string
	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration

	DefaultDatabaseURL       string
	DefaultDatabaseDirectURL string
	DefaultRedisAddr         string
	DefaultRedisPass         string

	DefaultPgBouncerHost       string
	DefaultPgBouncerPort       int
	DefaultPgBouncerPoolMode   string
	DefaultPgBouncerMaxClients int
	DefaultPgBouncerPoolSize   int
	DefaultPgBouncerAuthType   string

	DefaultS3Endpoint  string
	DefaultS3Region    string
	DefaultS3Bucket    string
	DefaultS3AccessKey string
	DefaultS3SecretKey string
	DefaultS3UseSSL    bool
	DefaultS3PathStyle bool

	DefaultSSLCertPath string
	DefaultSSLKeyPath  string
	ServicePorts       ServicePorts
}

func Load() Config {
	return Config{
		AppEnv:                   getenv("APP_ENV", "development"),
		HTTPPort:                 getenv("HTTP_PORT", "8080"),
		SettingsDBPath:           getenv("SETTINGS_DB_PATH", "/data/settings.db"),
		EncryptionKey:            getenv("SETTINGS_ENCRYPTION_KEY", ""),
		CORSAllowedOrigins:       splitList(getenv("CORS_ALLOWED_ORIGINS", "*")),
		JWTSecret:                getenv("JWT_SECRET", ""),
		JWTAccessTTL:             getduration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL:            getduration("JWT_REFRESH_TTL", 30*24*time.Hour),
		DefaultDatabaseURL:       getenv("DATABASE_URL", ""),
		DefaultDatabaseDirectURL: getenv("DATABASE_DIRECT_URL", ""),
		DefaultRedisAddr:         getenv("REDIS_ADDR", ""),
		DefaultRedisPass:         getenv("REDIS_PASSWORD", ""),

		DefaultPgBouncerHost:       getenv("PGBOUNCER_HOST", "pgbouncer"),
		DefaultPgBouncerPort:       getint("PGBOUNCER_INTERNAL_PORT", 5432),
		DefaultPgBouncerPoolMode:   getenv("PGBOUNCER_POOL_MODE", "transaction"),
		DefaultPgBouncerMaxClients: getint("PGBOUNCER_MAX_CLIENT_CONN", 200),
		DefaultPgBouncerPoolSize:   getint("PGBOUNCER_POOL_SIZE", 40),
		DefaultPgBouncerAuthType:   getenv("PGBOUNCER_AUTH_TYPE", "scram-sha-256"),

		DefaultS3Endpoint:  getenv("S3_ENDPOINT", ""),
		DefaultS3Region:    getenv("S3_REGION", "garage"),
		DefaultS3Bucket:    getenv("S3_BUCKET", "tabel"),
		DefaultS3AccessKey: getenv("S3_ACCESS_KEY", ""),
		DefaultS3SecretKey: getenv("S3_SECRET_KEY", ""),
		DefaultS3UseSSL:    getbool("S3_USE_SSL", false),
		DefaultS3PathStyle: getbool("S3_PATH_STYLE", true),

		DefaultSSLCertPath: getenv("SSL_CERT_PATH", ""),
		DefaultSSLKeyPath:  getenv("SSL_KEY_PATH", ""),
		ServicePorts: ServicePorts{
			Backend:   getint("BACKEND_PORT", 8080),
			Admin:     getint("ADMIN_PORT", 8081),
			Operator:  getint("OPERATOR_PORT", 8082),
			Reports:   getint("REPORTS_PORT", 8083),
			Postgres:  getint("POSTGRES_PORT", 5432),
			PgBouncer: getint("PGBOUNCER_PORT", 6432),
			Redis:     getint("REDIS_PORT", 6379),
			S3API:     getint("S3_API_PORT", 9000),
			S3Admin:   getint("GARAGE_ADMIN_PORT", 9001),
			HTTPS:     getint("HTTPS_PORT", 443),
		},
	}
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}

func getint(key string, fallback int) int {
	raw := getenv(key, "")
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func getbool(key string, fallback bool) bool {
	raw := strings.TrimSpace(getenv(key, ""))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func getduration(key string, fallback time.Duration) time.Duration {
	raw := getenv(key, "")
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

func splitList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
