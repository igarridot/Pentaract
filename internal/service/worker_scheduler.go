package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/repository"
)

type WorkerScheduler struct {
	workersRepo *repository.StorageWorkersRepo
	rateLimit   int
}

func NewWorkerScheduler(workersRepo *repository.StorageWorkersRepo, rateLimit int) *WorkerScheduler {
	return &WorkerScheduler{
		workersRepo: workersRepo,
		rateLimit:   rateLimit,
	}
}

// GetToken blocks until a worker token is available for the given storage.
func (s *WorkerScheduler) GetToken(ctx context.Context, storageID uuid.UUID) (string, error) {
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		token, err := s.workersRepo.GetToken(ctx, storageID, s.rateLimit)
		if err != nil {
			return "", fmt.Errorf("getting worker token: %w", err)
		}
		if token != "" {
			return token, nil
		}

		log.Printf("No workers available for storage %s, retrying in 1s...", storageID)
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}
}
