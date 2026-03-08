package service

import (
	"context"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

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
