package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DWHuang99/erent/internal/dto/request"
	casbinrbac "github.com/DWHuang99/erent/internal/middleware/casbin"
	jwtservice "github.com/DWHuang99/erent/internal/middleware/jwt"
	"github.com/DWHuang99/erent/internal/modules/user"
	"github.com/DWHuang99/erent/internal/security"
	"github.com/DWHuang99/erent/internal/testdatabase"
	"github.com/alicebob/miniredis/v2"
	"github.com/casbin/casbin/v3"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

func testRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func testEnforcer(t *testing.T) *casbin.SyncedEnforcer {
	t.Helper()
	enforcer, err := casbinrbac.NewMemoryEnforcer()
	if err != nil {
		t.Fatalf("create test enforcer: %v", err)
	}
	return enforcer
}

func testUserRepository(t *testing.T) (*gorm.DB, *user.Repository) {
	t.Helper()
	database := testdatabase.Open(t)
	repository := user.NewRepository(database)
	if err := database.AutoMigrate(&user.User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	return database, repository
}

func seedUser(t *testing.T, database *gorm.DB, username, password string, active bool) {
	t.Helper()
	hash, err := security.HashPassword(password)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	model := user.User{Username: username, PasswordHash: hash, RoleCode: "admin", IsActive: true}
	if err := database.Create(&model).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if !active {
		if err := database.Model(&model).Update("is_active", false).Error; err != nil {
			t.Fatalf("disable user: %v", err)
		}
	}
}

func TestLoginIssuesTokenForValidUser(t *testing.T) {
	database, repository := testUserRepository(t)
	seedUser(t, database, "alice", "correct-password", true)
	manager := jwtservice.NewJWTManager("test-secret-with-at-least-32-characters", "issuer", "audience", time.Minute, time.Hour)
	service := NewService(repository, manager, testRedisClient(t), testEnforcer(t))

	accessToken, refreshToken, exists, err := service.Login(context.Background(), request.LoginRequest{Username: "alice", Password: "correct-password"})
	if err != nil || !exists || accessToken == "" || refreshToken == "" {
		t.Fatalf("access = %q, refresh = %q, exists = %v, err = %v", accessToken, refreshToken, exists, err)
	}
	claims, err := manager.ParseToken(accessToken)
	if err != nil || len(claims.Role) != 1 || claims.Role[0] != casbinrbac.RoleAdmin || len(claims.Permissions) != 1 || claims.Permissions[0] != casbinrbac.PermissionDashboardView {
		t.Fatalf("access claims = %+v, err = %v", claims, err)
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

	_, missingRepository := testUserRepository(t)
	missing := NewService(missingRepository, manager, testRedisClient(t), testEnforcer(t))
	if access, refresh, exists, err := missing.Login(context.Background(), request.LoginRequest{Username: "missing", Password: "password"}); err != nil || exists || access != "" || refresh != "" {
		t.Fatalf("missing user result = %q, %q, %v, %v", access, refresh, exists, err)
	}

	wrongDatabase, wrongRepository := testUserRepository(t)
	seedUser(t, wrongDatabase, "wrong-user", "correct-password", true)
	wrong := NewService(wrongRepository, manager, testRedisClient(t), testEnforcer(t))
	if _, _, exists, err := wrong.Login(context.Background(), request.LoginRequest{Username: "wrong-user", Password: "wrong-password"}); err != nil || exists {
		t.Fatalf("wrong password exists = %v, err = %v", exists, err)
	}

	disabledDatabase, disabledRepository := testUserRepository(t)
	seedUser(t, disabledDatabase, "disabled-user", "correct-password", false)
	disabled := NewService(disabledRepository, manager, testRedisClient(t), testEnforcer(t))
	if _, _, exists, err := disabled.Login(context.Background(), request.LoginRequest{Username: "disabled-user", Password: "correct-password"}); !exists || !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("disabled exists = %v, err = %v", exists, err)
	}
}

func TestRegisterCreatesDefaultUserAndCasbinBinding(t *testing.T) {
	_, repository := testUserRepository(t)
	enforcer := testEnforcer(t)
	service := NewService(repository, nil, nil, enforcer)
	registerRequest := request.RegisterRequest{
		Username:      "new-user",
		Password:      "password123",
		CheckPassword: "password123",
		Code:          "123456",
		IAgree:        true,
	}

	registeredUser, err := service.Register(context.Background(), registerRequest)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if registeredUser.Username != "new-user" || registeredUser.RoleCode != casbinrbac.RoleUser || !registeredUser.IsActive {
		t.Fatalf("registered user = %+v", registeredUser)
	}
	if !security.VerifyPassword("password123", registeredUser.PasswordHash) {
		t.Fatal("registered password was not hashed correctly")
	}
	allowed, err := enforcer.Enforce(casbinrbac.UserSubject(registeredUser.ID), casbinrbac.PermissionDashboardView)
	if err != nil || !allowed {
		t.Fatalf("dashboard permission: allowed=%v err=%v", allowed, err)
	}
	if _, err := service.Register(context.Background(), registerRequest); !errors.Is(err, ErrUserExists) {
		t.Fatalf("duplicate register error = %v", err)
	}
}

func TestRegisterRejectsInvalidRequest(t *testing.T) {
	_, repository := testUserRepository(t)
	service := NewService(repository, nil, nil, testEnforcer(t))
	valid := request.RegisterRequest{
		Username:      "new-user",
		Password:      "password123",
		CheckPassword: "password123",
		Code:          "123456",
		IAgree:        true,
	}
	tests := map[string]request.RegisterRequest{
		"short username": func() request.RegisterRequest { value := valid; value.Username = "ab"; return value }(),
		"short password": func() request.RegisterRequest {
			value := valid
			value.Password = "short"
			value.CheckPassword = "short"
			return value
		}(),
		"password mismatch":   func() request.RegisterRequest { value := valid; value.CheckPassword = "different"; return value }(),
		"missing code":        func() request.RegisterRequest { value := valid; value.Code = ""; return value }(),
		"agreement not given": func() request.RegisterRequest { value := valid; value.IAgree = false; return value }(),
	}
	for name, registerRequest := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := service.Register(context.Background(), registerRequest); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("register error = %v", err)
			}
		})
	}
}
