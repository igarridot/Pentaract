package service

import (
	"context"
	"strings"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
	"github.com/Dominux/Pentaract/internal/password"
	"github.com/google/uuid"
)

type UsersService struct {
	usersRepo         usersRepository
	superuserEmail    string
	allowRegistration bool
}

type usersRepository interface {
	Create(ctx context.Context, email, passwordHash string) (*domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	ListNonAdmin(ctx context.Context, adminEmail string) ([]domain.User, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error
	DeleteManaged(ctx context.Context, id uuid.UUID) error
}

// NewUsersService builds the users service. allowRegistration opens
// Register to anyone; otherwise only the superuser adds accounts.
func NewUsersService(usersRepo usersRepository, superuserEmail string, allowRegistration bool) *UsersService {
	return &UsersService{
		usersRepo:         usersRepo,
		superuserEmail:    superuserEmail,
		allowRegistration: allowRegistration,
	}
}

// RegistrationEnabled tells whether anyone may create an account.
func (s *UsersService) RegistrationEnabled() bool {
	return s.allowRegistration
}

// Register is self sign-up from the /register page, refused unless
// registration is open.
func (s *UsersService) Register(ctx context.Context, email, pass string) (*domain.User, error) {
	if !s.allowRegistration {
		return nil, domain.ErrRegistrationDisabled()
	}
	return s.create(ctx, email, pass)
}

// CreateManaged is the superuser adding an account from User Management,
// whether or not registration is open.
func (s *UsersService) CreateManaged(ctx context.Context, caller *appjwt.AuthUser, email, pass string) (*domain.User, error) {
	if err := s.requireAdmin(caller); err != nil {
		return nil, err
	}
	return s.create(ctx, strings.TrimSpace(email), pass)
}

func (s *UsersService) create(ctx context.Context, email, pass string) (*domain.User, error) {
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

func (s *UsersService) requireAdmin(caller *appjwt.AuthUser) error {
	if !s.IsAdmin(caller) {
		return domain.ErrForbidden()
	}
	return nil
}

func (s *UsersService) requireManagedTarget(ctx context.Context, targetUserID uuid.UUID) error {
	target, err := s.usersRepo.GetByID(ctx, targetUserID)
	if err != nil {
		return err
	}
	if strings.EqualFold(target.Email, s.superuserEmail) {
		return domain.ErrForbidden()
	}
	return nil
}

func (s *UsersService) ListManaged(ctx context.Context, caller *appjwt.AuthUser) ([]domain.User, error) {
	if err := s.requireAdmin(caller); err != nil {
		return nil, err
	}
	return s.usersRepo.ListNonAdmin(ctx, s.superuserEmail)
}

func (s *UsersService) UpdatePassword(ctx context.Context, caller *appjwt.AuthUser, targetUserID uuid.UUID, newPassword string) error {
	if err := s.requireAdmin(caller); err != nil {
		return err
	}
	if newPassword == "" {
		return domain.ErrBadRequest("password is required")
	}

	if err := s.requireManagedTarget(ctx, targetUserID); err != nil {
		return err
	}

	hash, err := password.Hash(newPassword)
	if err != nil {
		return domain.ErrInternal("failed to hash password")
	}
	return s.usersRepo.UpdatePassword(ctx, targetUserID, hash)
}

func (s *UsersService) DeleteManaged(ctx context.Context, caller *appjwt.AuthUser, targetUserID uuid.UUID) error {
	if err := s.requireAdmin(caller); err != nil {
		return err
	}
	if err := s.requireManagedTarget(ctx, targetUserID); err != nil {
		return err
	}
	return s.usersRepo.DeleteManaged(ctx, targetUserID)
}
