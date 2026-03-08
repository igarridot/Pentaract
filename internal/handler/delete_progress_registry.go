package handler

import (
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/service"
)

type deleteTracker struct {
	mu        sync.RWMutex
	storageID uuid.UUID
	progress  *service.DeleteProgress
	err       error
	done      bool
}

var deleteRegistry = struct {
	mu sync.RWMutex
	m  map[string]*deleteTracker
}{
	m: make(map[string]*deleteTracker),
}

func startDeleteTracker(deleteID string, storageID uuid.UUID) *deleteTracker {
	tracker := &deleteTracker{progress: &service.DeleteProgress{}, storageID: storageID}
	deleteRegistry.mu.Lock()
	deleteRegistry.m[deleteID] = tracker
	deleteRegistry.mu.Unlock()
	return tracker
}

func getDeleteTracker(deleteID string) (*deleteTracker, bool) {
	deleteRegistry.mu.RLock()
	tracker, ok := deleteRegistry.m[deleteID]
	deleteRegistry.mu.RUnlock()
	return tracker, ok
}

func scheduleDeleteTrackerCleanup(deleteID string) {
	time.AfterFunc(5*time.Minute, func() {
		deleteRegistry.mu.Lock()
		delete(deleteRegistry.m, deleteID)
		deleteRegistry.mu.Unlock()
	})
}

func markDeleteTrackerDone(tracker *deleteTracker, err error) {
	tracker.mu.Lock()
	tracker.done = true
	tracker.err = err
	tracker.mu.Unlock()
}

func getDeleteTrackerStatus(tracker *deleteTracker) (done bool, err error, total int64, deleted int64) {
	tracker.mu.RLock()
	done = tracker.done
	err = tracker.err
	tracker.mu.RUnlock()

	total = tracker.progress.TotalChunks
	deleted = tracker.progress.DeletedChunks.Load()
	return
}
