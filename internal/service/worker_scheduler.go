package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/repository"
)

type WorkerScheduler struct {
	workersRepo schedulerWorkersRepo
	rateLimit   int
	mu          sync.RWMutex
	waiting     map[uuid.UUID]int
	// S2: buffered tokens to reduce DB round-trips
	tokenBufMu sync.Mutex
	tokenBuf   map[uuid.UUID][]*repository.WorkerToken
}

type schedulerWorkersRepo interface {
	GetToken(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error)
	GetTokenBatch(ctx context.Context, storageID uuid.UUID, rateLimit, count int) ([]repository.WorkerToken, error)
	NextAvailableIn(ctx context.Context, storageID uuid.UUID, rateLimit int) (time.Duration, error)
}

func NewWorkerScheduler(workersRepo schedulerWorkersRepo, rateLimit int) *WorkerScheduler {
	return &WorkerScheduler{
		workersRepo: workersRepo,
		rateLimit:   rateLimit,
		waiting:     make(map[uuid.UUID]int),
		tokenBuf:    make(map[uuid.UUID][]*repository.WorkerToken),
	}
}

// RateLimit returns the configured rate limit per worker per minute.
func (s *WorkerScheduler) RateLimit() int {
	return s.rateLimit
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

// popBufferedToken returns a pre-fetched token from the buffer, or nil.
func (s *WorkerScheduler) popBufferedToken(storageID uuid.UUID) *repository.WorkerToken {
	s.tokenBufMu.Lock()
	defer s.tokenBufMu.Unlock()
	buf := s.tokenBuf[storageID]
	if len(buf) == 0 {
		return nil
	}
	token := buf[0]
	s.tokenBuf[storageID] = buf[1:]
	return token
}

// GetToken blocks until a worker token is available for the given storage.
// S2: tries to fetch tokens in batches and buffer extras to reduce DB queries.
func (s *WorkerScheduler) GetToken(ctx context.Context, storageID uuid.UUID) (*repository.WorkerToken, error) {
	// Try buffer first
	if t := s.popBufferedToken(storageID); t != nil {
		return t, nil
	}

	logged := false
	markedWaiting := false
	defer func() {
		if markedWaiting {
			s.setWaiting(storageID, -1)
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Try batch fetch
		tokens, err := s.workersRepo.GetTokenBatch(ctx, storageID, s.rateLimit, TokenBatchSize)
		if err != nil {
			return nil, fmt.Errorf("getting worker tokens: %w", err)
		}
		if len(tokens) > 0 {
			// Buffer extras
			if len(tokens) > 1 {
				s.tokenBufMu.Lock()
				for i := 1; i < len(tokens); i++ {
					t := tokens[i]
					s.tokenBuf[storageID] = append(s.tokenBuf[storageID], &t)
				}
				s.tokenBufMu.Unlock()
			}
			return &tokens[0], nil
		}

		// All workers are at rate limit. Query when the next slot opens.
		waitDur, err := s.workersRepo.NextAvailableIn(ctx, storageID, s.rateLimit)
		if err != nil || waitDur <= 0 {
			waitDur = 2 * time.Second
		}

		if !logged {
			slog.Info("all workers at rate limit, waiting", "rate_limit", s.rateLimit, "storage_id", storageID, "wait", waitDur.Round(time.Millisecond))
			logged = true
			if !markedWaiting {
				s.setWaiting(storageID, 1)
				markedWaiting = true
			}
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(waitDur):
		}
	}
}
