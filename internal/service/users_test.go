package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
)

type fakeUsersRepo struct {
	createFn         func(ctx context.Context, email, passwordHash string) (*domain.User, error)
	getByIDFn        func(ctx context.Context, id uuid.UUID) (*domain.User, error)
	listNonAdminFn   func(ctx context.Context, adminEmail string) ([]domain.User, error)
	updatePasswordFn func(ctx context.Context, id uuid.UUID, passwordHash string) error
	deleteManagedFn  func(ctx context.Context, id uuid.UUID) error
}

func (f *fakeUsersRepo) Create(ctx context.Context, email, passwordHash string) (*domain.User, error) {
	return f.createFn(ctx, email, passwordHash)
}
func (f *fakeUsersRepo) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return f.getByIDFn(ctx, id)
}
func (f *fakeUsersRepo) ListNonAdmin(ctx context.Context, adminEmail string) ([]domain.User, error) {
	return f.listNonAdminFn(ctx, adminEmail)
}
func (f *fakeUsersRepo) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	return f.updatePasswordFn(ctx, id, passwordHash)
}
func (f *fakeUsersRepo) DeleteManaged(ctx context.Context, id uuid.UUID) error {
	return f.deleteManagedFn(ctx, id)
}

func TestUsersServiceIsAdmin(t *testing.T) {
	svc := NewUsersService(&fakeUsersRepo{}, "admin@example.com", true)
	if !svc.IsAdmin(&appjwt.AuthUser{Email: "ADMIN@example.com"}) {
		t.Fatalf("expected admin to be recognized case-insensitively")
	}
	if svc.IsAdmin(nil) {
		t.Fatalf("nil user must not be admin")
	}
	if svc.IsAdmin(&appjwt.AuthUser{Email: "user@example.com"}) {
		t.Fatalf("unexpected admin status for regular user")
	}
}

func TestUsersServiceRegister(t *testing.T) {
	called := false
	repo := &fakeUsersRepo{
		createFn: func(ctx context.Context, email, passwordHash string) (*domain.User, error) {
			called = true
			if email != "u@example.com" {
				t.Fatalf("unexpected email: %s", email)
			}
			if passwordHash == "" || strings.Contains(passwordHash, "secret") {
				t.Fatalf("password must be hashed")
			}
			return &domain.User{Email: email}, nil
		},
	}
	svc := NewUsersService(repo, "admin@example.com", true)

	if _, err := svc.Register(context.Background(), "", "x"); err == nil {
		t.Fatalf("expected bad request for empty email")
	}

	u, err := svc.Register(context.Background(), "u@example.com", "secret")
	if err != nil {
		t.Fatalf("register unexpected error: %v", err)
	}
	if !called || u.Email != "u@example.com" {
		t.Fatalf("register was not executed as expected")
	}
}

func TestUsersServiceListManaged(t *testing.T) {
	repo := &fakeUsersRepo{
		listNonAdminFn: func(ctx context.Context, adminEmail string) ([]domain.User, error) {
			return []domain.User{{Email: "u@example.com"}}, nil
		},
	}
	svc := NewUsersService(repo, "admin@example.com", true)

	if _, err := svc.ListManaged(context.Background(), &appjwt.AuthUser{Email: "u@example.com"}); err == nil {
		t.Fatalf("expected forbidden for non-admin")
	}
	users, err := svc.ListManaged(context.Background(), &appjwt.AuthUser{Email: "admin@example.com"})
	if err != nil || len(users) != 1 {
		t.Fatalf("expected managed users, err=%v users=%v", err, users)
	}
}

func TestUsersServiceUpdatePassword(t *testing.T) {
	targetID := uuid.New()
	var updatedHash string
	repo := &fakeUsersRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return &domain.User{ID: id, Email: "user@example.com"}, nil
		},
		updatePasswordFn: func(ctx context.Context, id uuid.UUID, passwordHash string) error {
			updatedHash = passwordHash
			return nil
		},
	}
	svc := NewUsersService(repo, "admin@example.com", true)

	if err := svc.UpdatePassword(context.Background(), &appjwt.AuthUser{Email: "u@example.com"}, targetID, "x"); err == nil {
		t.Fatalf("expected forbidden for non-admin")
	}
	if err := svc.UpdatePassword(context.Background(), &appjwt.AuthUser{Email: "admin@example.com"}, targetID, ""); err == nil {
		t.Fatalf("expected bad request for empty password")
	}
	if err := svc.UpdatePassword(context.Background(), &appjwt.AuthUser{Email: "admin@example.com"}, targetID, "new-secret"); err != nil {
		t.Fatalf("unexpected update error: %v", err)
	}
	if updatedHash == "" || strings.Contains(updatedHash, "new-secret") {
		t.Fatalf("password must be hashed before update")
	}
}

func TestUsersServiceDeleteManaged(t *testing.T) {
	targetID := uuid.New()
	deleted := false
	repo := &fakeUsersRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return &domain.User{ID: id, Email: "user@example.com"}, nil
		},
		deleteManagedFn: func(ctx context.Context, id uuid.UUID) error {
			deleted = true
			return nil
		},
	}
	svc := NewUsersService(repo, "admin@example.com", true)

	if err := svc.DeleteManaged(context.Background(), &appjwt.AuthUser{Email: "u@example.com"}, targetID); err == nil {
		t.Fatalf("expected forbidden for non-admin")
	}
	if err := svc.DeleteManaged(context.Background(), &appjwt.AuthUser{Email: "admin@example.com"}, targetID); err != nil {
		t.Fatalf("unexpected delete error: %v", err)
	}
	if !deleted {
		t.Fatalf("expected delete to be called")
	}
}

func TestUsersServiceAdminTargetForbiddenAndRepoErrors(t *testing.T) {
	targetID := uuid.New()
	repo := &fakeUsersRepo{
		getByIDFn: func(ctx context.Context, id uuid.UUID) (*domain.User, error) {
			return &domain.User{ID: id, Email: "admin@example.com"}, nil
		},
		updatePasswordFn: func(ctx context.Context, id uuid.UUID, passwordHash string) error { return nil },
		deleteManagedFn:  func(ctx context.Context, id uuid.UUID) error { return nil },
		createFn: func(ctx context.Context, email, passwordHash string) (*domain.User, error) {
			return nil, errors.New("create fail")
		},
		listNonAdminFn: func(ctx context.Context, adminEmail string) ([]domain.User, error) {
			return nil, errors.New("list fail")
		},
	}
	svc := NewUsersService(repo, "admin@example.com", true)

	if _, err := svc.Register(context.Background(), "u@example.com", "x"); err == nil {
		t.Fatalf("expected register repo error")
	}
	if _, err := svc.ListManaged(context.Background(), &appjwt.AuthUser{Email: "admin@example.com"}); err == nil {
		t.Fatalf("expected list managed repo error")
	}
	if err := svc.UpdatePassword(context.Background(), &appjwt.AuthUser{Email: "admin@example.com"}, targetID, "x"); err == nil {
		t.Fatalf("expected forbidden updating superuser")
	}
	if err := svc.DeleteManaged(context.Background(), &appjwt.AuthUser{Email: "admin@example.com"}, targetID); err == nil {
		t.Fatalf("expected forbidden deleting superuser")
	}
}

func TestUsersServiceRegisterRefusedWhenClosed(t *testing.T) {
	repo := &fakeUsersRepo{
		createFn: func(ctx context.Context, email, passwordHash string) (*domain.User, error) {
			t.Fatalf("a closed registration must not create the user")
			return nil, nil
		},
	}
	svc := NewUsersService(repo, "admin@example.com", false)

	if svc.RegistrationEnabled() {
		t.Fatalf("expected registration closed")
	}
	_, err := svc.Register(context.Background(), "u@example.com", "secret")
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.Code != 403 {
		t.Fatalf("expected 403, got %v", err)
	}
}

func TestUsersServiceCreateManaged(t *testing.T) {
	var created []string
	repo := &fakeUsersRepo{
		createFn: func(ctx context.Context, email, passwordHash string) (*domain.User, error) {
			if passwordHash == "" || strings.Contains(passwordHash, "secret") {
				t.Fatalf("password must be hashed")
			}
			created = append(created, email)
			return &domain.User{Email: email}, nil
		},
	}
	// Closed registration does not stop the superuser.
	svc := NewUsersService(repo, "admin@example.com", false)
	admin := &appjwt.AuthUser{Email: "admin@example.com"}

	u, err := svc.CreateManaged(context.Background(), admin, "  new@example.com ", "secret")
	if err != nil || u.Email != "new@example.com" {
		t.Fatalf("create managed: user=%+v err=%v", u, err)
	}

	if _, err := svc.CreateManaged(context.Background(), &appjwt.AuthUser{Email: "u@example.com"}, "x@example.com", "secret"); err == nil {
		t.Fatalf("expected forbidden for a non-admin caller")
	}
	if _, err := svc.CreateManaged(context.Background(), admin, "   ", "secret"); err == nil {
		t.Fatalf("expected bad request for an empty email")
	}
	if _, err := svc.CreateManaged(context.Background(), admin, "y@example.com", ""); err == nil {
		t.Fatalf("expected bad request for an empty password")
	}
	if len(created) != 1 {
		t.Fatalf("only the valid admin request may create a user, got %v", created)
	}

	repo.createFn = func(ctx context.Context, email, passwordHash string) (*domain.User, error) {
		return nil, domain.ErrAlreadyExists("user")
	}
	if _, err := svc.CreateManaged(context.Background(), admin, "new@example.com", "secret"); err == nil {
		t.Fatalf("expected the duplicate error")
	}
}
