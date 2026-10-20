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

func TestDatabase(ctx context.Context, databaseURL string) error {
	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return pool.Ping(pingCtx)
}

func Connect(ctx context.Context, databaseURL, redisAddr, redisPassword string) (*pgxpool.Pool, *redis.Client, error) {
	pool, err := db.Connect(ctx, databaseURL)
	if err != nil {
		return nil, nil, err
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("проверка PostgreSQL: %w", err)
	}

	if err := db.Migrate(ctx, databaseURL); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("миграции: %w", err)
	}

	rdb := cache.New(redisAddr, redisPassword)
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		_ = rdb.Close()
		pool.Close()
		return nil, nil, fmt.Errorf("проверка Redis: %w", err)
	}

	return pool, rdb, nil
}
