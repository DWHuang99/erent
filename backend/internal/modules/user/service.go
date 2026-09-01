package user

import (
	"context"
	"errors"
)

var (
	ErrUserNotExists = errors.New("user does not exist")
	ErrUserDisabled  = errors.New("user is disabled")
)

type UserReader interface {
	GetUserByID(context.Context, uint64) (*CurrentUser, error)
}

type Service struct {
	repository UserReader
}

func NewService(repository UserReader) *Service {
	return &Service{repository: repository}
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
	return currentUser, nil
}
