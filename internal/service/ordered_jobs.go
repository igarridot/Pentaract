package service

import (
	"context"
	"errors"

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

func runOrderedJobs[Job any, Result any](
	ctx context.Context,
	parallelism int,
	jobs []Job,
	load func(context.Context, Job) (Result, error),
	handle func(Job, Result) error,
	cleanup func(Job, Result),
) error {
	if len(jobs) == 0 {
		return nil
	}
	if parallelism > len(jobs) {
		parallelism = len(jobs)
	}
	if parallelism <= 1 {
		for _, job := range jobs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			result, err := load(ctx, job)
			if err != nil {
				return err
			}
			if err := handle(job, result); err != nil {
				if cleanup != nil {
					cleanup(job, result)
				}
				return err
			}
		}
		return nil
	}

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
				case resultCh <- orderedJobResult[Job, Result]{
					index:  job.index,
					job:    job.job,
					result: result,
				}:
				case <-gctx.Done():
					if cleanup != nil {
						cleanup(job.job, result)
					}
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

	pending := make(map[int]orderedJobResult[Job, Result], parallelism)
	nextIndex := 0
	var handleErr error

	for result := range resultCh {
		pending[result.index] = result

		for {
			next, ok := pending[nextIndex]
			if !ok {
				break
			}
			delete(pending, nextIndex)

			if handleErr == nil {
				if err := handle(next.job, next.result); err != nil {
					handleErr = err
					cancel()
				}
			} else if cleanup != nil {
				cleanup(next.job, next.result)
			}
			nextIndex++
		}
	}

	workerErr := <-errCh
	if handleErr != nil {
		return handleErr
	}
	if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
		return workerErr
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}
