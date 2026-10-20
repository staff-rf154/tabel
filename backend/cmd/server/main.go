package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"tabel/internal/app"
	"tabel/internal/config"
	"tabel/internal/httpapi"
	"tabel/internal/jobs"
	"tabel/internal/settings"
)

func main() {
	cfg := config.Load()

	key, err := settings.ParseKey(cfg.EncryptionKey)
	if err != nil {
		log.Fatalf("настройки: %v", err)
	}

	store, err := settings.Open(cfg.SettingsDBPath, key)
	if err != nil {
		log.Fatalf("хранилище настроек: %v", err)
	}
	defer func() { _ = store.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var pool *pgxpool.Pool
	var rdb *redis.Client

	databaseURL := resolveString(store, settings.KeyDatabaseURL, cfg.DefaultDatabaseURL)
	redisAddr := resolveString(store, settings.KeyRedisAddr, cfg.DefaultRedisAddr)
	redisPass := resolveString(store, settings.KeyRedisPassword, cfg.DefaultRedisPass)

	if databaseURL != "" && redisAddr != "" {
		pool, rdb, err = app.Connect(ctx, databaseURL, redisAddr, redisPass)
		if err != nil {
			log.Printf("подключение к БД/Redis не установлено: %v", err)
			pool, rdb = nil, nil
		} else {
			log.Printf("подключение к БД и Redis установлено, миграции применены")
		}
	} else {
		log.Printf("система не настроена — требуется первичная установка через веб-интерфейс")
	}

	if pool != nil {
		defer pool.Close()
	}
	if rdb != nil {
		defer func() { _ = rdb.Close() }()
	}

	api := httpapi.NewServer(cfg, store, pool, rdb)
	if err := api.EnsureQRKey(); err != nil {
		log.Fatalf("ключ подписи QR: %v", err)
	}

	go jobs.Run(ctx, pool)

	srv := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           api.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Printf("backend запущен на :%s (env=%s)", cfg.HTTPPort, cfg.AppEnv)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http-сервер: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("остановка backend...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("ошибка остановки: %v", err)
	}
}

func resolveString(store *settings.Store, key, fallback string) string {
	if v, err := store.GetString(key); err == nil && v != "" {
		return v
	}
	return fallback
}
