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
// It queries the DB for the next available slot and waits accordingly
// instead of polling every second.
func (s *WorkerScheduler) GetToken(ctx context.Context, storageID uuid.UUID) (*repository.WorkerToken, error) {
	logged := false
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		wt, err := s.workersRepo.GetToken(ctx, storageID, s.rateLimit)
		if err != nil {
			return nil, fmt.Errorf("getting worker token: %w", err)
		}
		if wt != nil {
			return wt, nil
		}

		// All workers are at rate limit. Query when the next slot opens.
		waitDur, err := s.workersRepo.NextAvailableIn(ctx, storageID, s.rateLimit)
		if err != nil || waitDur <= 0 {
			waitDur = 2 * time.Second
		}

		if !logged {
			log.Printf("[scheduler] all workers at rate limit (%d/min) for storage %s, waiting %s", s.rateLimit, storageID, waitDur.Round(time.Millisecond))
			logged = true
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(waitDur):
		}
	}
}
