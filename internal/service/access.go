package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

type AccessService struct {
	accessRepo accessRepository
	usersRepo  accessUsersRepository
}

type accessRepository interface {
	HasAccess(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error)
	CreateOrUpdate(ctx context.Context, userID, storageID uuid.UUID, accessType domain.AccessType) error
	List(ctx context.Context, storageID uuid.UUID) ([]domain.UserWithAccess, error)
	Delete(ctx context.Context, userID, storageID uuid.UUID) error
}

type accessUsersRepository interface {
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	ListGrantCandidates(ctx context.Context, storageID, callerID uuid.UUID) ([]domain.User, error)
}

func NewAccessService(accessRepo accessRepository, usersRepo accessUsersRepository) *AccessService {
	return &AccessService{
		accessRepo: accessRepo,
		usersRepo:  usersRepo,
	}
}

func (s *AccessService) Grant(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, email string, accessType domain.AccessType) error {
	// Check caller has admin access
	if err := requireStorageAccess(ctx, s.accessRepo, callerID, storageID, domain.AccessAdmin); err != nil {
		return err
	}

	// Find target user
	user, err := s.usersRepo.GetByEmail(ctx, email)
	if err != nil {
		return err
	}

	// Cannot change own access
	if user.ID == callerID {
		return domain.ErrSelfAccess()
	}

	return s.accessRepo.CreateOrUpdate(ctx, user.ID, storageID, accessType)
}

func (s *AccessService) List(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID) ([]domain.UserWithAccess, error) {
	if err := requireStorageAccess(ctx, s.accessRepo, callerID, storageID, domain.AccessAdmin); err != nil {
		return nil, err
	}

	return s.accessRepo.List(ctx, storageID)
}

func (s *AccessService) Revoke(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, targetUserID uuid.UUID) error {
	if err := requireStorageAccess(ctx, s.accessRepo, callerID, storageID, domain.AccessAdmin); err != nil {
		return err
	}

	if targetUserID == callerID {
		return domain.ErrSelfAccess()
	}

	return s.accessRepo.Delete(ctx, targetUserID, storageID)
}

func (s *AccessService) ListGrantCandidates(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID) ([]domain.User, error) {
	if err := requireStorageAccess(ctx, s.accessRepo, callerID, storageID, domain.AccessAdmin); err != nil {
		return nil, err
	}

	return s.usersRepo.ListGrantCandidates(ctx, storageID, callerID)
}

type storageAccessChecker interface {
	HasAccess(ctx context.Context, userID, storageID uuid.UUID, requiredLevel domain.AccessType) (bool, error)
}

func requireStorageAccess(ctx context.Context, checker storageAccessChecker, userID, storageID uuid.UUID, requiredLevel domain.AccessType) error {
	ok, err := checker.HasAccess(ctx, userID, storageID, requiredLevel)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrForbidden()
	}
	return nil
}
