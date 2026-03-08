package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/repository"
)

type fakeSchedulerRepo struct {
	getTokenFn        func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error)
	nextAvailableInFn func(ctx context.Context, storageID uuid.UUID, rateLimit int) (time.Duration, error)
}

func (f *fakeSchedulerRepo) GetToken(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
	return f.getTokenFn(ctx, storageID, rateLimit)
}
func (f *fakeSchedulerRepo) NextAvailableIn(ctx context.Context, storageID uuid.UUID, rateLimit int) (time.Duration, error) {
	return f.nextAvailableInFn(ctx, storageID, rateLimit)
}

func TestWorkerSchedulerGetTokenImmediate(t *testing.T) {
	sid := uuid.New()
	repo := &fakeSchedulerRepo{
		getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
			return &repository.WorkerToken{Token: "t", Name: "w1"}, nil
		},
		nextAvailableInFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (time.Duration, error) {
			return 0, nil
		},
	}
	s := NewWorkerSchedulerWithRepo(repo, 10)
	wt, err := s.GetToken(context.Background(), sid)
	if err != nil || wt == nil || wt.Token != "t" {
		t.Fatalf("unexpected result: wt=%v err=%v", wt, err)
	}
	if s.IsWaiting(sid) {
		t.Fatalf("scheduler should not be waiting")
	}
}

func TestWorkerSchedulerGetTokenWaitAndCancel(t *testing.T) {
	sid := uuid.New()
	calls := 0
	repo := &fakeSchedulerRepo{
		getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
			calls++
			return nil, nil
		},
		nextAvailableInFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (time.Duration, error) {
			return 10 * time.Millisecond, nil
		},
	}
	s := NewWorkerSchedulerWithRepo(repo, 10)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := s.GetToken(ctx, sid)
	if err == nil {
		t.Fatalf("expected cancellation error")
	}
	if calls == 0 {
		t.Fatalf("expected getToken to be called at least once")
	}
}

func TestWorkerSchedulerRepoError(t *testing.T) {
	repo := &fakeSchedulerRepo{
		getTokenFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (*repository.WorkerToken, error) {
			return nil, errors.New("db error")
		},
		nextAvailableInFn: func(ctx context.Context, storageID uuid.UUID, rateLimit int) (time.Duration, error) {
			return 0, nil
		},
	}
	s := NewWorkerSchedulerWithRepo(repo, 10)
	_, err := s.GetToken(context.Background(), uuid.New())
	if err == nil {
		t.Fatalf("expected repository error")
	}
}
