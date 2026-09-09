package main

import (
	"context"
	"strings"
	"testing"

	"erent/internal/config"
	"erent/internal/modules/user"
	"erent/internal/security"
	"erent/internal/testdatabase"
)

func TestCreateBootstrapUserSkipsEmptyUsername(t *testing.T) {
	if err := createBootstrapUser(context.Background(), config.Config{}, nil); err != nil {
		t.Fatalf("skip bootstrap user: %v", err)
	}
}

func TestCreateBootstrapUserRejectsUnsupportedRole(t *testing.T) {
	configuration := config.Config{
		BootstrapUsername: "bootstrap-admin",
		BootstrapPassword: "password123",
		BootstrapRole:     "owner",
	}
	err := createBootstrapUser(context.Background(), configuration, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid bootstrap role") {
		t.Fatalf("expected invalid bootstrap role error, got %v", err)
	}
}

func TestCreateBootstrapUserCreatesMissingUserWithoutOverwriting(t *testing.T) {
	database := testdatabase.Open(t)
	if err := database.AutoMigrate(&user.User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	repository := user.NewRepository(database)
	configuration := config.Config{
		BootstrapUsername: "bootstrap-admin",
		BootstrapPassword: "password123",
		BootstrapRole:     "admin",
	}

	if err := createBootstrapUser(context.Background(), configuration, repository); err != nil {
		t.Fatalf("create bootstrap user: %v", err)
	}
	configuration.BootstrapPassword = "different-password"
	if err := createBootstrapUser(context.Background(), configuration, repository); err != nil {
		t.Fatalf("repeat bootstrap user creation: %v", err)
	}

	auth, err := repository.GetUserAuthByUsername(context.Background(), configuration.BootstrapUsername)
	if err != nil {
		t.Fatalf("load bootstrap user: %v", err)
	}
	if auth.RoleCode != "admin" || !auth.IsActive {
		t.Fatalf("unexpected bootstrap user: %+v", auth)
	}
	if !security.VerifyPassword("password123", auth.PasswordHash) {
		t.Fatal("bootstrap user password was overwritten")
	}
}
