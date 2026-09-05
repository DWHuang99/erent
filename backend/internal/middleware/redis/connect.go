package rdb

import (
	"context"
	"fmt"

	"erent/internal/config"

	"github.com/redis/go-redis/v9"
)

func Connect(ctx context.Context, configuration config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     configuration.Address,
		Password: configuration.Password,
		DB:       configuration.DB,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("ping redis: %w", err)
	}
	return client, nil
}
