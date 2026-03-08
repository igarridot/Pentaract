package service

import (
	"context"
	"strings"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
	"github.com/Dominux/Pentaract/internal/password"
	"github.com/Dominux/Pentaract/internal/repository"
	"github.com/google/uuid"
)

type UsersService struct {
	usersRepo      usersRepository
	superuserEmail string
}

type usersRepository interface {
	Create(ctx context.Context, email, passwordHash string) (*domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	ListNonAdmin(ctx context.Context, adminEmail string) ([]domain.User, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error
	DeleteManaged(ctx context.Context, id uuid.UUID) error
}

func NewUsersService(usersRepo *repository.UsersRepo, superuserEmail string) *UsersService {
	return NewUsersServiceWithRepo(usersRepo, superuserEmail)
}

func NewUsersServiceWithRepo(usersRepo usersRepository, superuserEmail string) *UsersService {
	return &UsersService{
		usersRepo:      usersRepo,
		superuserEmail: superuserEmail,
	}
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

func (s *UsersService) IsAdmin(user *appjwt.AuthUser) bool {
	if user == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(s.superuserEmail))
}

func (s *UsersService) AdminStatus(user *appjwt.AuthUser) bool {
	return s.IsAdmin(user)
}

func (s *UsersService) ListManaged(ctx context.Context, caller *appjwt.AuthUser) ([]domain.User, error) {
	if !s.IsAdmin(caller) {
		return nil, domain.ErrForbidden()
	}
	return s.usersRepo.ListNonAdmin(ctx, s.superuserEmail)
}

func (s *UsersService) UpdatePassword(ctx context.Context, caller *appjwt.AuthUser, targetUserID uuid.UUID, newPassword string) error {
	if !s.IsAdmin(caller) {
		return domain.ErrForbidden()
	}
	if newPassword == "" {
		return domain.ErrBadRequest("password is required")
	}

	target, err := s.usersRepo.GetByID(ctx, targetUserID)
	if err != nil {
		return err
	}
	if strings.EqualFold(target.Email, s.superuserEmail) {
		return domain.ErrForbidden()
	}

	hash, err := password.Hash(newPassword)
	if err != nil {
		return domain.ErrInternal("failed to hash password")
	}
	return s.usersRepo.UpdatePassword(ctx, targetUserID, hash)
}

func (s *UsersService) DeleteManaged(ctx context.Context, caller *appjwt.AuthUser, targetUserID uuid.UUID) error {
	if !s.IsAdmin(caller) {
		return domain.ErrForbidden()
	}
	target, err := s.usersRepo.GetByID(ctx, targetUserID)
	if err != nil {
		return err
	}
	if strings.EqualFold(target.Email, s.superuserEmail) {
		return domain.ErrForbidden()
	}
	return s.usersRepo.DeleteManaged(ctx, targetUserID)
}
