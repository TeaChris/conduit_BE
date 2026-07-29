package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/conduit-platform/conduit/backend/internal/config"
)

// NewClient creates a new Redis client and verifies connectivity.
func NewClient(ctx context.Context, cfg config.RedisConfig, logger zerolog.Logger) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr(),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("pinging redis: %w", err)
	}

	logger.Info().
		Str("addr", cfg.Addr()).
		Int("db", cfg.DB).
		Int("pool_size", cfg.PoolSize).
		Msg("redis connection established")

	return client, nil
}
