package jwtservice

import (
	"errors"
	"testing"
	"time"

	"erent/internal/config"
)

func newTestJWTManager(secret string, accessTTL time.Duration) *JWTManager {
	return NewJWTManager(config.JWTConfig{
		Secret:     secret,
		Issuer:     "issuer",
		Audience:   "audience",
		AccessTTL:  accessTTL,
		RefreshTTL: time.Hour,
	})
}

func TestGenerateAndParseToken(t *testing.T) {
	manager := newTestJWTManager("test-secret-with-at-least-32-characters", time.Minute)
	token, err := manager.GenerateTokenWithPermissions(42, "alice", []string{"admin"}, []string{"dashboard:view"})
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := manager.ParseToken(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.Subject != "42" || claims.Username != "alice" || len(claims.Role) != 1 || claims.Role[0] != "admin" || len(claims.Permissions) != 1 || claims.Permissions[0] != "dashboard:view" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestParseTokenRejectsExpiredAndWrongSecret(t *testing.T) {
	expiredManager := newTestJWTManager("test-secret-with-at-least-32-characters", -time.Second)
	expired, _ := expiredManager.GenerateToken(1, "alice", "user")
	if _, err := expiredManager.ParseToken(expired); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token error = %v", err)
	}

	issuer := newTestJWTManager("issuer-secret-with-at-least-32-characters", time.Minute)
	token, _ := issuer.GenerateToken(1, "alice", "user")
	other := newTestJWTManager("another-secret-with-at-least-32-characters", time.Minute)
	if _, err := other.ParseToken(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong-secret error = %v", err)
	}
}
