package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/password"
)

type fakeAuthUsersRepo struct {
	getByEmailFn func(ctx context.Context, email string) (*domain.User, error)
}

func (f *fakeAuthUsersRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return f.getByEmailFn(ctx, email)
}

func TestAuthServiceLogin(t *testing.T) {
	userID := uuid.New()
	repo := &fakeAuthUsersRepo{
		getByEmailFn: func(ctx context.Context, email string) (*domain.User, error) {
			if email == "missing@example.com" {
				return nil, domain.ErrNotFound("user")
			}
			hash, _ := password.Hash("secret")
			return &domain.User{ID: userID, Email: "u@example.com", PasswordHash: hash}, nil
		},
	}
	svc := NewAuthServiceWithRepo(repo, "secret-key", 3600)

	if _, err := svc.Login(context.Background(), "missing@example.com", "secret"); err == nil {
		t.Fatalf("expected unauthorized when user is missing")
	}
	if _, err := svc.Login(context.Background(), "u@example.com", "wrong"); err == nil {
		t.Fatalf("expected unauthorized on wrong password")
	}

	resp, err := svc.Login(context.Background(), "u@example.com", "secret")
	if err != nil {
		t.Fatalf("unexpected login error: %v", err)
	}
	if resp.AccessToken == "" || resp.TokenType != "bearer" || resp.ExpiresIn != 3600 {
		t.Fatalf("unexpected login response: %+v", resp)
	}
}
