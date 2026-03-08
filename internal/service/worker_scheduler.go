package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/repository"
)

type WorkerScheduler struct {
	workersRepo *repository.StorageWorkersRepo
	rateLimit   int
	mu          sync.RWMutex
	waiting     map[uuid.UUID]int
}

func NewWorkerScheduler(workersRepo *repository.StorageWorkersRepo, rateLimit int) *WorkerScheduler {
	return &WorkerScheduler{
		workersRepo: workersRepo,
		rateLimit:   rateLimit,
		waiting:     make(map[uuid.UUID]int),
	}
}

func (s *WorkerScheduler) setWaiting(storageID uuid.UUID, delta int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.waiting[storageID] += delta
	if s.waiting[storageID] <= 0 {
		delete(s.waiting, storageID)
	}
}

func (s *WorkerScheduler) IsWaiting(storageID uuid.UUID) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.waiting[storageID] > 0
}

// GetToken blocks until a worker token is available for the given storage.
// It queries the DB for the next available slot and waits accordingly
// instead of polling every second.
func (s *WorkerScheduler) GetToken(ctx context.Context, storageID uuid.UUID) (*repository.WorkerToken, error) {
	logged := false
	markedWaiting := false
	for {
		select {
		case <-ctx.Done():
			if markedWaiting {
				s.setWaiting(storageID, -1)
			}
			return nil, ctx.Err()
		default:
		}

		wt, err := s.workersRepo.GetToken(ctx, storageID, s.rateLimit)
		if err != nil {
			return nil, fmt.Errorf("getting worker token: %w", err)
		}
		if wt != nil {
			if markedWaiting {
				s.setWaiting(storageID, -1)
			}
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
			if !markedWaiting {
				s.setWaiting(storageID, 1)
				markedWaiting = true
			}
		}

		select {
		case <-ctx.Done():
			if markedWaiting {
				s.setWaiting(storageID, -1)
			}
			return nil, ctx.Err()
		case <-time.After(waitDur):
		}
	}
}
