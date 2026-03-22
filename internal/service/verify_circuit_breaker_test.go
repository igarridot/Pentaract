package service

import (
	"context"
	"testing"
	"time"
)

func TestCircuitBreakerStartsClosed(t *testing.T) {
	cb := newVerifyCircuitBreaker()
	if cb.IsTripped() {
		t.Fatal("new circuit breaker should not be tripped")
	}
	if cb.ConsecutiveFailures() != 0 {
		t.Fatal("new circuit breaker should have 0 failures")
	}
}

func TestCircuitBreakerTripsAfterThreshold(t *testing.T) {
	cb := newVerifyCircuitBreaker()

	// Record failures up to threshold-1 — should not trip
	for i := 0; i < VerifyCBFailureThreshold-1; i++ {
		cb.RecordFailure()
		if cb.IsTripped() {
			t.Fatalf("tripped after %d failures, threshold is %d", i+1, VerifyCBFailureThreshold)
		}
	}

	// One more failure should trip it
	cb.RecordFailure()
	if !cb.IsTripped() {
		t.Fatal("should be tripped after reaching threshold")
	}
	if cb.ConsecutiveFailures() != VerifyCBFailureThreshold {
		t.Fatalf("consecutive failures = %d, want %d", cb.ConsecutiveFailures(), VerifyCBFailureThreshold)
	}
}

func TestCircuitBreakerSuccessResetsFailures(t *testing.T) {
	cb := newVerifyCircuitBreaker()

	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordSuccess()

	if cb.ConsecutiveFailures() != 0 {
		t.Fatalf("consecutive failures should be 0 after success, got %d", cb.ConsecutiveFailures())
	}
	if cb.IsTripped() {
		t.Fatal("should not be tripped after success")
	}
}

func TestCircuitBreakerRecoveryAfterTrip(t *testing.T) {
	cb := newVerifyCircuitBreaker()

	for i := 0; i < VerifyCBFailureThreshold; i++ {
		cb.RecordFailure()
	}
	if !cb.IsTripped() {
		t.Fatal("should be tripped")
	}

	cb.RecordSuccess()
	if cb.IsTripped() {
		t.Fatal("should recover after success")
	}
}

func TestCircuitBreakerWaitIfTrippedReturnsImmediatelyWhenClosed(t *testing.T) {
	cb := newVerifyCircuitBreaker()

	start := time.Now()
	err := cb.WaitIfTripped(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatal("WaitIfTripped should return immediately when not tripped")
	}
}

func TestCircuitBreakerWaitIfTrippedBlocksDuringCooldown(t *testing.T) {
	cb := &verifyCircuitBreaker{
		cooldown:         100 * time.Millisecond,
		failureThreshold: 1,
	}

	cb.RecordFailure()
	if !cb.IsTripped() {
		t.Fatal("should be tripped")
	}

	start := time.Now()
	err := cb.WaitIfTripped(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Fatalf("should have waited for cooldown, elapsed = %v", elapsed)
	}
}

func TestCircuitBreakerWaitIfTrippedRespectsContextCancellation(t *testing.T) {
	cb := &verifyCircuitBreaker{
		cooldown:         10 * time.Second,
		failureThreshold: 1,
	}

	cb.RecordFailure()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := cb.WaitIfTripped(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestCircuitBreakerWaitIfTrippedReturnsAfterCooldownExpires(t *testing.T) {
	cb := &verifyCircuitBreaker{
		cooldown:         50 * time.Millisecond,
		failureThreshold: 1,
	}

	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond) // wait past cooldown

	start := time.Now()
	err := cb.WaitIfTripped(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if time.Since(start) > 20*time.Millisecond {
		t.Fatal("should return immediately when cooldown already expired")
	}
}

func TestIsHashMismatch(t *testing.T) {
	mismatch := &chunkHashMismatchError{position: 5}
	if !isHashMismatch(mismatch) {
		t.Fatal("isHashMismatch should return true for chunkHashMismatchError")
	}

	other := context.DeadlineExceeded
	if isHashMismatch(other) {
		t.Fatal("isHashMismatch should return false for non-mismatch errors")
	}

	if isHashMismatch(nil) {
		t.Fatal("isHashMismatch should return false for nil")
	}
}

func TestChunkHashMismatchErrorMessage(t *testing.T) {
	err := &chunkHashMismatchError{position: 42}
	msg := err.Error()
	if msg != "chunk 42: content mismatch after Telegram round-trip" {
		t.Fatalf("unexpected message: %s", msg)
	}
}
