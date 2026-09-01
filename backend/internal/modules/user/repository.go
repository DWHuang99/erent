package user

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

var ErrNotFound = errors.New("user not found")

type Repository struct {
	database *gorm.DB
}

func NewRepository(database *gorm.DB) *Repository {
	return &Repository{database: database}
}

func (r *Repository) AddUser(ctx context.Context, username, passwordHash, roleCode string) (*User, error) {
	model := User{
		Username:     strings.TrimSpace(username),
		PasswordHash: passwordHash,
		RoleCode:     roleCode,
		IsActive:     true,
	}
	if err := r.database.WithContext(ctx).Create(&model).Error; err != nil {
		return nil, err
	}
	return &model, nil
}

func (r *Repository) GetUserAuthByUsername(ctx context.Context, username string) (*UserAuth, error) {
	var model User
	err := r.database.WithContext(ctx).Where("username = ?", strings.TrimSpace(username)).First(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &UserAuth{
		ID:           model.ID,
		Username:     model.Username,
		PasswordHash: model.PasswordHash,
		RoleCode:     model.RoleCode,
		IsActive:     model.IsActive,
	}, nil
}

func (r *Repository) GetUserAuthByID(ctx context.Context, userID uint64) (*UserAuth, error) {
	var model User
	err := r.database.WithContext(ctx).First(&model, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &UserAuth{
		ID:           model.ID,
		Username:     model.Username,
		PasswordHash: model.PasswordHash,
		RoleCode:     model.RoleCode,
		IsActive:     model.IsActive,
	}, nil
}

func (r *Repository) GetUserByID(ctx context.Context, userID uint64) (*CurrentUser, error) {
	var model User
	err := r.database.WithContext(ctx).First(&model, userID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &CurrentUser{
		ID:          model.ID,
		Username:    model.Username,
		RoleCode:    model.RoleCode,
		RoleName:    model.RoleCode,
		Roles:       []string{model.RoleCode},
		Permissions: []string{},
		IsActive:    model.IsActive,
		CreatedAt:   model.CreatedAt,
		UpdatedAt:   model.UpdatedAt,
	}, nil
}

// CreateBootstrapUser only creates a missing local administrator and never overwrites an existing account.
func (r *Repository) CreateBootstrapUser(ctx context.Context, username, passwordHash, roleCode string) error {
	if username == "" {
		return nil
	}
	model := User{
		Username:     username,
		PasswordHash: passwordHash,
		RoleCode:     roleCode,
		IsActive:     true,
	}
	if err := r.database.WithContext(ctx).Where("username = ?", username).FirstOrCreate(&model).Error; err != nil {
		return fmt.Errorf("create bootstrap user: %w", err)
	}
	return nil
}
