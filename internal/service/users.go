package service

import (
	"context"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/password"
	"github.com/Dominux/Pentaract/internal/repository"
)

type UsersService struct {
	usersRepo *repository.UsersRepo
}

func NewUsersService(usersRepo *repository.UsersRepo) *UsersService {
	return &UsersService{usersRepo: usersRepo}
}

func (s *UsersService) Register(ctx context.Context, email, pass string) (*domain.User, error) {
	if email == "" || pass == "" {
		return nil, domain.ErrBadRequest("email and password are required")
	}

	hash, err := password.Hash(pass)
	if err != nil {
		return nil, domain.ErrInternal("failed to hash password")
	}

	return s.usersRepo.Create(ctx, email, hash)
}
