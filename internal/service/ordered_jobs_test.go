package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunOrderedJobsHandlesResultsInInputOrder(t *testing.T) {
	jobs := []int{0, 1, 2, 3, 4, 5, 6, 7}
	var handled []int
	err := runOrderedJobs(context.Background(), 4, jobs,
		func(ctx context.Context, job int) (int, error) {
			// Later jobs finish first so the ordering must come from the runner.
			time.Sleep(time.Duration(len(jobs)-job) * 2 * time.Millisecond)
			return job * 10, nil
		},
		func(job, result int) error {
			if result != job*10 {
				t.Fatalf("job %d got result %d", job, result)
			}
			handled = append(handled, job)
			return nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for i, job := range handled {
		if job != i {
			t.Fatalf("results handled out of order: %v", handled)
		}
	}
	if len(handled) != len(jobs) {
		t.Fatalf("handled %d of %d jobs", len(handled), len(jobs))
	}
}

func TestRunOrderedJobsSingleJobAndNoJobs(t *testing.T) {
	if err := runOrderedJobs[int, int](context.Background(), 3, nil, nil, nil); err != nil {
		t.Fatalf("no jobs must be a no-op, got %v", err)
	}

	var handled int
	err := runOrderedJobs(context.Background(), 0, []int{7},
		func(context.Context, int) (int, error) { return 1, nil },
		func(int, int) error { handled++; return nil },
	)
	if err != nil || handled != 1 {
		t.Fatalf("single job: err=%v handled=%d", err, handled)
	}
}

func TestRunOrderedJobsHandleErrorStopsRemainingWork(t *testing.T) {
	boom := errors.New("boom")
	var handled int
	var loaded atomic.Int32
	err := runOrderedJobs(context.Background(), 2, []int{0, 1, 2, 3, 4, 5},
		func(ctx context.Context, job int) (int, error) {
			loaded.Add(1)
			return job, nil
		},
		func(job, _ int) error {
			handled++
			if job == 1 {
				return boom
			}
			return nil
		},
	)
	if !errors.Is(err, boom) {
		t.Fatalf("expected handle error, got %v", err)
	}
	if handled != 2 {
		t.Fatalf("handle must stop after the failing job, ran %d times", handled)
	}
}

func TestRunOrderedJobsLoadErrorPropagates(t *testing.T) {
	// A load failure is returned as is, including a cancellation that did not
	// come from the caller's context: silently ending with a partial result
	// would hand the client a truncated file.
	for _, loadErr := range []error{errors.New("telegram down"), context.Canceled} {
		err := runOrderedJobs(context.Background(), 2, []int{0, 1, 2},
			func(ctx context.Context, job int) (int, error) {
				if job == 1 {
					return 0, loadErr
				}
				return job, nil
			},
			func(int, int) error { return nil },
		)
		if !errors.Is(err, loadErr) {
			t.Fatalf("expected %v, got %v", loadErr, err)
		}
	}
}

func TestRunOrderedJobsReturnsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := runOrderedJobs(ctx, 2, []int{0, 1, 2, 3},
		func(ctx context.Context, job int) (int, error) {
			if job == 0 {
				cancel()
			}
			<-ctx.Done()
			return 0, ctx.Err()
		},
		func(int, int) error { return nil },
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
