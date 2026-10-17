package service

import (
	"context"

	"golang.org/x/sync/errgroup"
)

type orderedJob[Job any] struct {
	index int
	job   Job
}

type orderedJobResult[Job any, Result any] struct {
	index  int
	job    Job
	result Result
}

// runOrderedJobs loads jobs with up to parallelism workers and hands every
// result to handle strictly in input order. The first load or handle error
// stops the remaining work and is returned.
func runOrderedJobs[Job any, Result any](
	ctx context.Context,
	parallelism int,
	jobs []Job,
	load func(context.Context, Job) (Result, error),
	handle func(Job, Result) error,
) error {
	if len(jobs) == 0 {
		return nil
	}
	parallelism = max(min(parallelism, len(jobs)), 1)

	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	g, gctx := errgroup.WithContext(workCtx)
	jobCh := make(chan orderedJob[Job])
	resultCh := make(chan orderedJobResult[Job, Result], parallelism)

	for range parallelism {
		g.Go(func() error {
			for job := range jobCh {
				result, err := load(gctx, job.job)
				if err != nil {
					return err
				}

				select {
				case resultCh <- orderedJobResult[Job, Result]{index: job.index, job: job.job, result: result}:
				case <-gctx.Done():
					return gctx.Err()
				}
			}
			return nil
		})
	}

	g.Go(func() error {
		defer close(jobCh)
		for index, job := range jobs {
			select {
			case jobCh <- orderedJob[Job]{index: index, job: job}:
			case <-gctx.Done():
				return gctx.Err()
			}
		}
		return nil
	})

	errCh := make(chan error, 1)
	go func() {
		err := g.Wait()
		close(resultCh)
		errCh <- err
	}()

	// Results arrive out of order; park them until their turn comes.
	pending := make(map[int]orderedJobResult[Job, Result], parallelism)
	nextIndex := 0
	var handleErr error

	for result := range resultCh {
		pending[result.index] = result

		for next, ok := pending[nextIndex]; ok; next, ok = pending[nextIndex] {
			delete(pending, nextIndex)
			nextIndex++
			if handleErr != nil {
				continue
			}
			if handleErr = handle(next.job, next.result); handleErr != nil {
				cancel()
			}
		}
	}

	workerErr := <-errCh
	switch {
	case handleErr != nil:
		return handleErr
	case workerErr != nil:
		return workerErr
	default:
		return ctx.Err()
	}
}
