package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/repository"
)

type AccessService struct {
	accessRepo *repository.AccessRepo
	usersRepo  *repository.UsersRepo
}

func NewAccessService(accessRepo *repository.AccessRepo, usersRepo *repository.UsersRepo) *AccessService {
	return &AccessService{
		accessRepo: accessRepo,
		usersRepo:  usersRepo,
	}
}

func (s *AccessService) Grant(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, email string, accessType domain.AccessType) error {
	// Check caller has admin access
	ok, err := s.accessRepo.HasAccess(ctx, callerID, storageID, domain.AccessAdmin)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrForbidden()
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
	ok, err := s.accessRepo.HasAccess(ctx, callerID, storageID, domain.AccessAdmin)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrForbidden()
	}

	return s.accessRepo.List(ctx, storageID)
}

func (s *AccessService) Revoke(ctx context.Context, callerID uuid.UUID, storageID uuid.UUID, targetUserID uuid.UUID) error {
	ok, err := s.accessRepo.HasAccess(ctx, callerID, storageID, domain.AccessAdmin)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrForbidden()
	}

	if targetUserID == callerID {
		return domain.ErrSelfAccess()
	}

	return s.accessRepo.Delete(ctx, targetUserID, storageID)
}
