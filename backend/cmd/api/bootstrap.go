package main

import (
	"context"
	"fmt"

	"erent/internal/config"
	casbinrbac "erent/internal/middleware/casbin"
	"erent/internal/modules/user"
	"erent/internal/security"
)

func createBootstrapUser(ctx context.Context, configuration config.Config, repository *user.Repository) error {
	if configuration.BootstrapUsername == "" {
		return nil
	}
	if !casbinrbac.IsSupportedRole(configuration.BootstrapRole) {
		return fmt.Errorf("invalid bootstrap role %q", configuration.BootstrapRole)
	}
	passwordHash, err := security.HashPassword(configuration.BootstrapPassword)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}
	if err := repository.CreateBootstrapUser(
		ctx,
		configuration.BootstrapUsername,
		passwordHash,
		configuration.BootstrapRole,
	); err != nil {
		return err
	}
	return nil
}
