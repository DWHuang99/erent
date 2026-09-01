package user

import (
	"context"
	"errors"

	casbinrbac "github.com/DWHuang99/erent/internal/middleware/casbin"
	"github.com/casbin/casbin/v3"
)

var (
	ErrUserNotExists = errors.New("user does not exist")
	ErrUserDisabled  = errors.New("user is disabled")
)

type Service struct {
	repository *Repository
	enforcer   *casbin.SyncedEnforcer
}

func NewService(repository *Repository, enforcer *casbin.SyncedEnforcer) *Service {
	return &Service{repository: repository, enforcer: enforcer}
}

func (s *Service) GetUserByID(ctx context.Context, userID uint64) (*CurrentUser, error) {
	currentUser, err := s.repository.GetUserByID(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrUserNotExists
	}
	if err != nil {
		return nil, err
	}
	if !currentUser.IsActive {
		return nil, ErrUserDisabled
	}
	roles, permissions, err := casbinrbac.AuthorizationForUser(s.enforcer, currentUser.ID, currentUser.RoleCode)
	if err != nil {
		return nil, err
	}
	currentUser.Roles = roles
	currentUser.Permissions = permissions
	return currentUser, nil
}
