package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// chunkHashMismatchError is returned when a verified chunk's content does not
// match the original plaintext hash. This is a permanent failure — retrying
// will not fix it.
type chunkHashMismatchError struct {
	position int16
}

func (e *chunkHashMismatchError) Error() string {
	return fmt.Sprintf("chunk %d: content mismatch after Telegram round-trip", e.position)
}

func isHashMismatch(err error) bool {
	var mismatch *chunkHashMismatchError
	return errors.As(err, &mismatch)
}

// verifyCircuitBreaker tracks consecutive transient verification failures
// during an upload and pauses new verifications when a threshold is reached,
// giving Telegram time to recover from throttling. One instance per upload.
type verifyCircuitBreaker struct {
	mu                   sync.Mutex
	consecutiveFailures  int
	consecutiveSuccesses int
	tripped              bool
	lastTrippedAt        time.Time
	cooldown             time.Duration
	failureThreshold     int
}

func newVerifyCircuitBreaker() *verifyCircuitBreaker {
	return &verifyCircuitBreaker{
		cooldown:         VerifyCBCooldownDuration,
		failureThreshold: VerifyCBFailureThreshold,
	}
}

// RecordSuccess resets the failure counter and increments success counter.
func (cb *verifyCircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFailures = 0
	cb.consecutiveSuccesses++
	if cb.tripped {
		cb.tripped = false
		slog.Info("verification circuit breaker recovered")
	}
}

// RecordFailure increments the failure counter. If the threshold is reached,
// the breaker trips and new verifications should wait for the cooldown.
func (cb *verifyCircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFailures++
	cb.consecutiveSuccesses = 0
	if cb.consecutiveFailures >= cb.failureThreshold && !cb.tripped {
		cb.tripped = true
		cb.lastTrippedAt = time.Now()
		slog.Warn("verification circuit breaker tripped",
			"consecutive_failures", cb.consecutiveFailures,
			"cooldown", cb.cooldown)
	}
}

// WaitIfTripped blocks until the cooldown has elapsed if the breaker is
// currently tripped. Returns ctx.Err() if the context is cancelled during
// the wait, or nil otherwise.
func (cb *verifyCircuitBreaker) WaitIfTripped(ctx context.Context) error {
	cb.mu.Lock()
	if !cb.tripped {
		cb.mu.Unlock()
		return nil
	}
	remaining := cb.cooldown - time.Since(cb.lastTrippedAt)
	cb.mu.Unlock()

	if remaining <= 0 {
		return nil
	}

	slog.Info("verification circuit breaker waiting", "remaining", remaining.Round(time.Millisecond))
	timer := time.NewTimer(remaining)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// IsTripped returns whether the breaker is currently in a tripped state.
func (cb *verifyCircuitBreaker) IsTripped() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.tripped
}

// ConsecutiveFailures returns the current consecutive failure count.
func (cb *verifyCircuitBreaker) ConsecutiveFailures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.consecutiveFailures
}
