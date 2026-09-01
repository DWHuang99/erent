package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DWHuang99/erent/internal/dto/request"
	jwtservice "github.com/DWHuang99/erent/internal/middleware/jwt"
	"github.com/DWHuang99/erent/internal/modules/user"
	"github.com/DWHuang99/erent/internal/security"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

type fakeUserAuthRepository struct {
	userAuth *user.UserAuth
	err      error
}

func (f fakeUserAuthRepository) GetUserAuthByUsername(context.Context, string) (*user.UserAuth, error) {
	return f.userAuth, f.err
}

func (f fakeUserAuthRepository) GetUserAuthByID(context.Context, uint64) (*user.UserAuth, error) {
	return f.userAuth, f.err
}

func testRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestLoginIssuesTokenForValidUser(t *testing.T) {
	hash, _ := security.HashPassword("correct-password")
	manager := jwtservice.NewJWTManager("test-secret-with-at-least-32-characters", "issuer", "audience", time.Minute, time.Hour)
	service := NewService(fakeUserAuthRepository{userAuth: &user.UserAuth{
		ID: 7, Username: "alice", PasswordHash: hash, RoleCode: "admin", IsActive: true,
	}}, manager, testRedisClient(t))
	accessToken, refreshToken, exists, err := service.Login(context.Background(), request.LoginRequest{Username: "alice", Password: "correct-password"})
	if err != nil || !exists || accessToken == "" || refreshToken == "" {
		t.Fatalf("access = %q, refresh = %q, exists = %v, err = %v", accessToken, refreshToken, exists, err)
	}
	newAccessToken, newRefreshToken, err := service.Refresh(context.Background(), refreshToken)
	if err != nil || newAccessToken == "" || newRefreshToken == "" || newRefreshToken == refreshToken {
		t.Fatalf("refresh access = %q, refresh = %q, err = %v", newAccessToken, newRefreshToken, err)
	}
	if _, _, err := service.Refresh(context.Background(), refreshToken); !errors.Is(err, ErrInvalidRefreshToken) {
		t.Fatalf("reused refresh token error = %v", err)
	}
	if err := service.Logout(context.Background(), newRefreshToken); err != nil {
		t.Fatalf("logout: %v", err)
	}
}

func TestLoginRejectsMissingWrongPasswordAndDisabledUser(t *testing.T) {
	manager := jwtservice.NewJWTManager("test-secret-with-at-least-32-characters", "issuer", "audience", time.Minute, time.Hour)
	missing := NewService(fakeUserAuthRepository{err: user.ErrNotFound}, manager, testRedisClient(t))
	if access, refresh, exists, err := missing.Login(context.Background(), request.LoginRequest{Username: "missing", Password: "password"}); err != nil || exists || access != "" || refresh != "" {
		t.Fatalf("missing user result = %q, %q, %v, %v", access, refresh, exists, err)
	}

	hash, _ := security.HashPassword("correct-password")
	wrong := NewService(fakeUserAuthRepository{userAuth: &user.UserAuth{ID: 1, PasswordHash: hash, IsActive: true}}, manager, testRedisClient(t))
	if _, _, exists, err := wrong.Login(context.Background(), request.LoginRequest{Password: "wrong-password"}); err != nil || exists {
		t.Fatalf("wrong password exists = %v, err = %v", exists, err)
	}

	disabled := NewService(fakeUserAuthRepository{userAuth: &user.UserAuth{ID: 1, PasswordHash: hash}}, manager, testRedisClient(t))
	if _, _, exists, err := disabled.Login(context.Background(), request.LoginRequest{Password: "correct-password"}); !exists || !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("disabled exists = %v, err = %v", exists, err)
	}
}
