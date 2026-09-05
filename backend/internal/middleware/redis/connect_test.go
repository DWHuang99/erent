package rdb

import (
	"context"
	"testing"

	"erent/internal/config"

	"github.com/alicebob/miniredis/v2"
)

func TestConnectUsesRedisConfig(t *testing.T) {
	server := miniredis.RunT(t)
	client, err := Connect(context.Background(), config.RedisConfig{
		Address: server.Addr(),
		DB:      2,
	})
	if err != nil {
		t.Fatalf("connect redis: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if client.Options().Addr != server.Addr() || client.Options().DB != 2 {
		t.Fatalf("unexpected redis options: %+v", client.Options())
	}
}
