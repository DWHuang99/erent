package jwtservice

import (
	"errors"
	"testing"
	"time"
)

func TestGenerateAndParseToken(t *testing.T) {
	manager := NewJWTManager("test-secret-with-at-least-32-characters", "issuer", "audience", time.Minute, time.Hour)
	token, err := manager.GenerateToken(42, "alice", "admin")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	claims, err := manager.ParseToken(token)
	if err != nil {
		t.Fatalf("parse token: %v", err)
	}
	if claims.Subject != "42" || claims.Username != "alice" || len(claims.Role) != 1 || claims.Role[0] != "admin" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestParseTokenRejectsExpiredAndWrongSecret(t *testing.T) {
	expiredManager := NewJWTManager("test-secret-with-at-least-32-characters", "issuer", "audience", -time.Second, time.Hour)
	expired, _ := expiredManager.GenerateToken(1, "alice", "user")
	if _, err := expiredManager.ParseToken(expired); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token error = %v", err)
	}

	issuer := NewJWTManager("issuer-secret-with-at-least-32-characters", "issuer", "audience", time.Minute, time.Hour)
	token, _ := issuer.GenerateToken(1, "alice", "user")
	other := NewJWTManager("another-secret-with-at-least-32-characters", "issuer", "audience", time.Minute, time.Hour)
	if _, err := other.ParseToken(token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong-secret error = %v", err)
	}
}
