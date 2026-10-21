package app

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"tabel/internal/cache"
	"tabel/internal/db"
)

// TestDatabase проверяет подключение и приводит структуру БД в актуальное
// состояние. Если базы ещё нет, она будет создана.
func TestDatabase(ctx context.Context, databaseURL string) (bool, error) {
	pool, created, err := connectPostgres(ctx, databaseURL)
	if err != nil {
		return created, err
	}
	pool.Close()
	return created, nil
}

func Connect(ctx context.Context, databaseURL, redisAddr, redisPassword string) (*pgxpool.Pool, *redis.Client, error) {
	pool, _, err := connectPostgres(ctx, databaseURL)
	if err != nil {
		return nil, nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	rdb := cache.New(redisAddr, redisPassword)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		pool.Close()
		return nil, nil, fmt.Errorf("проверка Redis: %w", err)
	}

	return pool, rdb, nil
}

func connectPostgres(ctx context.Context, databaseURL string) (*pgxpool.Pool, bool, error) {
	created, err := db.EnsureDatabase(ctx, databaseURL)
	if err != nil {
		return nil, false, fmt.Errorf("подготовка PostgreSQL: %w", err)
	}

	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		return nil, created, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, created, fmt.Errorf("проверка PostgreSQL: %w", err)
	}

	if err := db.Migrate(ctx, databaseURL); err != nil {
		pool.Close()
		return nil, created, fmt.Errorf("миграции: %w", err)
	}
	return pool, created, nil
}
