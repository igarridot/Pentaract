package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

type fakeAccessRepo struct {
	hasAccessFn      func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error)
	createOrUpdateFn func(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error
	listFn           func(ctx context.Context, storageID uuid.UUID) ([]domain.UserWithAccess, error)
	deleteFn         func(ctx context.Context, userID, storageID uuid.UUID) error
}

func (f *fakeAccessRepo) HasAccess(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
	return f.hasAccessFn(ctx, userID, storageID, requiredLevel)
}
func (f *fakeAccessRepo) CreateOrUpdate(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error {
	return f.createOrUpdateFn(ctx, userID, storageID, accessType)
}
func (f *fakeAccessRepo) List(ctx context.Context, storageID uuid.UUID) ([]domain.UserWithAccess, error) {
	return f.listFn(ctx, storageID)
}
func (f *fakeAccessRepo) Delete(ctx context.Context, userID, storageID uuid.UUID) error {
	return f.deleteFn(ctx, userID, storageID)
}

type fakeAccessUsersRepo struct {
	getByEmailFn          func(ctx context.Context, email string) (*domain.User, error)
	listGrantCandidatesFn func(ctx context.Context, storageID, callerID uuid.UUID) ([]domain.User, error)
}

func (f *fakeAccessUsersRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return f.getByEmailFn(ctx, email)
}
func (f *fakeAccessUsersRepo) ListGrantCandidates(ctx context.Context, storageID, callerID uuid.UUID) ([]domain.User, error) {
	return f.listGrantCandidatesFn(ctx, storageID, callerID)
}

func TestAccessServiceFlows(t *testing.T) {
	caller := uuid.New()
	storage := uuid.New()
	target := uuid.New()
	accessRepo := &fakeAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return true, nil
		},
		createOrUpdateFn: func(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error {
			return nil
		},
		listFn: func(ctx context.Context, storageID uuid.UUID) ([]domain.UserWithAccess, error) {
			return []domain.UserWithAccess{{Email: "u@example.com", AccessType: domain.AccessWrite}}, nil
		},
		deleteFn: func(ctx context.Context, userID, storageID uuid.UUID) error { return nil },
	}
	usersRepo := &fakeAccessUsersRepo{
		getByEmailFn: func(ctx context.Context, email string) (*domain.User, error) {
			return &domain.User{ID: target, Email: email}, nil
		},
		listGrantCandidatesFn: func(ctx context.Context, storageID, callerID uuid.UUID) ([]domain.User, error) {
			return []domain.User{{Email: "candidate@example.com"}}, nil
		},
	}
	svc := NewAccessServiceWithRepos(accessRepo, usersRepo)

	if err := svc.Grant(context.Background(), caller, storage, "target@example.com", domain.AccessRead); err != nil {
		t.Fatalf("grant failed: %v", err)
	}
	list, err := svc.List(context.Background(), caller, storage)
	if err != nil || len(list) != 1 {
		t.Fatalf("list failed: %v %v", err, list)
	}
	if err := svc.Revoke(context.Background(), caller, storage, target); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	candidates, err := svc.ListGrantCandidates(context.Background(), caller, storage)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates failed: %v %v", err, candidates)
	}
}

func TestAccessServiceForbiddenAndSelf(t *testing.T) {
	id := uuid.New()
	accessRepo := &fakeAccessRepo{
		hasAccessFn: func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
			return false, nil
		},
		createOrUpdateFn: func(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error { return nil },
		listFn:           func(ctx context.Context, storageID uuid.UUID) ([]domain.UserWithAccess, error) { return nil, nil },
		deleteFn:         func(ctx context.Context, userID, storageID uuid.UUID) error { return nil },
	}
	usersRepo := &fakeAccessUsersRepo{
		getByEmailFn: func(ctx context.Context, email string) (*domain.User, error) {
			return &domain.User{ID: id, Email: email}, nil
		},
		listGrantCandidatesFn: func(ctx context.Context, storageID, callerID uuid.UUID) ([]domain.User, error) { return nil, nil },
	}
	svc := NewAccessServiceWithRepos(accessRepo, usersRepo)
	if err := svc.Grant(context.Background(), id, uuid.New(), "x@example.com", domain.AccessRead); err == nil {
		t.Fatalf("expected forbidden grant")
	}

	accessRepo.hasAccessFn = func(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error) {
		return true, nil
	}
	if err := svc.Grant(context.Background(), id, uuid.New(), "x@example.com", domain.AccessRead); err == nil {
		t.Fatalf("expected self-access error")
	}
	if err := svc.Revoke(context.Background(), id, uuid.New(), id); err == nil {
		t.Fatalf("expected self-access on revoke")
	}
}
