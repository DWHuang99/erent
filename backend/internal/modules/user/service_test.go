package user

import (
	"context"
	"errors"
	"testing"
)

type fakeUserReader struct {
	currentUser *CurrentUser
	err         error
}

func (f fakeUserReader) GetUserByID(context.Context, uint64) (*CurrentUser, error) {
	return f.currentUser, f.err
}

func TestGetUserByID(t *testing.T) {
	service := NewService(fakeUserReader{currentUser: &CurrentUser{ID: 1, Username: "alice", IsActive: true}})
	currentUser, err := service.GetUserByID(context.Background(), 1)
	if err != nil || currentUser.Username != "alice" {
		t.Fatalf("user = %+v, err = %v", currentUser, err)
	}
}

func TestGetUserByIDMapsMissingAndDisabled(t *testing.T) {
	missing := NewService(fakeUserReader{err: ErrNotFound})
	if _, err := missing.GetUserByID(context.Background(), 1); !errors.Is(err, ErrUserNotExists) {
		t.Fatalf("missing error = %v", err)
	}
	disabled := NewService(fakeUserReader{currentUser: &CurrentUser{ID: 1}})
	if _, err := disabled.GetUserByID(context.Background(), 1); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("disabled error = %v", err)
	}
}
