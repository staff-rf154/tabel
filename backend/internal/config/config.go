package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	AppEnv             string
	HTTPPort           string
	SettingsDBPath     string
	EncryptionKey      string
	CORSAllowedOrigins []string

	JWTSecret     string
	JWTAccessTTL  time.Duration
	JWTRefreshTTL time.Duration

	DefaultDatabaseURL string
	DefaultRedisAddr   string
	DefaultRedisPass   string
}

func Load() Config {
	return Config{
		AppEnv:             getenv("APP_ENV", "development"),
		HTTPPort:           getenv("HTTP_PORT", "8080"),
		SettingsDBPath:     getenv("SETTINGS_DB_PATH", "/data/settings.db"),
		EncryptionKey:      getenv("SETTINGS_ENCRYPTION_KEY", ""),
		CORSAllowedOrigins: splitList(getenv("CORS_ALLOWED_ORIGINS", "*")),
		JWTSecret:          getenv("JWT_SECRET", ""),
		JWTAccessTTL:       getduration("JWT_ACCESS_TTL", 15*time.Minute),
		JWTRefreshTTL:      getduration("JWT_REFRESH_TTL", 30*24*time.Hour),
		DefaultDatabaseURL: getenv("DATABASE_URL", ""),
		DefaultRedisAddr:   getenv("REDIS_ADDR", ""),
		DefaultRedisPass:   getenv("REDIS_PASSWORD", ""),
	}
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
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
