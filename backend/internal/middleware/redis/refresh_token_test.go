package rdb

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestNewRefreshTokenUses256Bits(t *testing.T) {
	token, err := newRefreshToken()
	if err != nil {
		t.Fatalf("new refresh token: %v", err)
	}
	randomBytes, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(randomBytes) != 32 {
		t.Fatalf("decoded bytes = %d, err = %v", len(randomBytes), err)
	}
}

func TestRefreshTokenKeyDoesNotExposeRawToken(t *testing.T) {
	token := "raw-refresh-token"
	key := refreshTokenKey(token)
	if !strings.HasPrefix(key, refreshTokenPrefix) || strings.Contains(key, token) {
		t.Fatalf("unsafe refresh token key %q", key)
	}
}

func TestCreateRotateAndDeleteRefreshToken(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	defer client.Close()
	ctx := context.Background()

	oldToken, err := CreateRefreshToken(client, ctx, 42, time.Hour)
	if err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	newToken, userID, err := RotateRefreshToken(client, ctx, oldToken, time.Hour)
	if err != nil || userID != 42 || newToken == oldToken {
		t.Fatalf("rotate result token=%q user=%d err=%v", newToken, userID, err)
	}
	if _, _, err := RotateRefreshToken(client, ctx, oldToken, time.Hour); !errors.Is(err, ErrRefreshTokenNotFound) {
		t.Fatalf("reused token error = %v", err)
	}
	if err := DeleteRefreshToken(client, ctx, newToken); err != nil {
		t.Fatalf("delete refresh token: %v", err)
	}
	if _, _, err := RotateRefreshToken(client, ctx, newToken, time.Hour); !errors.Is(err, ErrRefreshTokenNotFound) {
		t.Fatalf("deleted token error = %v", err)
	}
}
