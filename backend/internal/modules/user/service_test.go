package user

import (
	"context"
	"errors"
	"testing"

	casbinrbac "github.com/DWHuang99/erent/internal/middleware/casbin"
	"github.com/DWHuang99/erent/internal/security"
	"github.com/DWHuang99/erent/internal/testdatabase"
	"gorm.io/gorm"
)

func testRepository(t *testing.T) (*gorm.DB, *Repository) {
	t.Helper()
	database := testdatabase.Open(t)
	repository := NewRepository(database)
	if err := database.AutoMigrate(&User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	return database, repository
}

func createTestUser(t *testing.T, database *gorm.DB, repository *Repository, username string, active bool) uint64 {
	t.Helper()
	hash, err := security.HashPassword("password123")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	model := User{Username: username, PasswordHash: hash, RoleCode: "admin", IsActive: true}
	if err := database.Create(&model).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if !active {
		if err := database.Model(&model).Update("is_active", false).Error; err != nil {
			t.Fatalf("disable user: %v", err)
		}
	}
	userAuth, err := repository.GetUserAuthByUsername(context.Background(), username)
	if err != nil {
		t.Fatalf("load seeded user: %v", err)
	}
	return userAuth.ID
}

func TestGetUserByID(t *testing.T) {
	database, repository := testRepository(t)
	userID := createTestUser(t, database, repository, "alice", true)
	enforcer, err := casbinrbac.NewMemoryEnforcer()
	if err != nil {
		t.Fatalf("create test enforcer: %v", err)
	}
	service := NewService(repository, enforcer)

	currentUser, err := service.GetUserByID(context.Background(), userID)
	if err != nil || currentUser.Username != "alice" || len(currentUser.Roles) != 1 || currentUser.Roles[0] != casbinrbac.RoleAdmin || len(currentUser.Permissions) != 1 || currentUser.Permissions[0] != casbinrbac.PermissionDashboardView {
		t.Fatalf("user = %+v, err = %v", currentUser, err)
	}
}

func TestGetUserByIDMapsMissingAndDisabled(t *testing.T) {
	database, repository := testRepository(t)
	enforcer, err := casbinrbac.NewMemoryEnforcer()
	if err != nil {
		t.Fatalf("create test enforcer: %v", err)
	}
	service := NewService(repository, enforcer)
	if _, err := service.GetUserByID(context.Background(), 999); !errors.Is(err, ErrUserNotExists) {
		t.Fatalf("missing error = %v", err)
	}

	disabledID := createTestUser(t, database, repository, "disabled", false)
	if _, err := service.GetUserByID(context.Background(), disabledID); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("disabled error = %v", err)
	}
}
