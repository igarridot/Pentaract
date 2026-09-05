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

func (t *deleteTracker) finish(err error) {
	t.mu.Lock()
	t.done = true
	t.err = err
	t.mu.Unlock()
}

func (t *deleteTracker) status() (done bool, err error, total int64, deleted int64) {
	t.mu.RLock()
	done, err = t.done, t.err
	t.mu.RUnlock()
	return done, err, t.progress.TotalChunks, t.progress.DeletedChunks.Load()
}

// DeleteTrackers holds in-flight delete operations (files, folders and whole
// storages) so their progress can be streamed over SSE. One instance is shared
// by every handler that deletes.
type DeleteTrackers struct {
	mu        sync.RWMutex
	m         map[string]*deleteTracker
	afterFunc func(time.Duration, func()) *time.Timer
}

func NewDeleteTrackers() *DeleteTrackers {
	return &DeleteTrackers{m: make(map[string]*deleteTracker), afterFunc: time.AfterFunc}
}

// track registers a tracker for deleteID and returns the progress to feed the
// service plus a finish callback to call with the outcome. With an empty
// deleteID (client did not ask for progress) both are no-ops.
func (d *DeleteTrackers) track(deleteID string, storageID uuid.UUID) (*service.DeleteProgress, func(err error)) {
	if deleteID == "" {
		return nil, func(error) {}
	}
	tracker := d.start(deleteID, storageID)
	return tracker.progress, func(err error) {
		tracker.finish(err)
		d.scheduleCleanup(deleteID)
	}
}

func (d *DeleteTrackers) start(deleteID string, storageID uuid.UUID) *deleteTracker {
	tracker := &deleteTracker{progress: &service.DeleteProgress{}, storageID: storageID}
	d.mu.Lock()
	d.m[deleteID] = tracker
	d.mu.Unlock()
	return tracker
}

func (d *DeleteTrackers) get(deleteID string) (*deleteTracker, bool) {
	d.mu.RLock()
	tracker, ok := d.m[deleteID]
	d.mu.RUnlock()
	return tracker, ok
}

func (d *DeleteTrackers) scheduleCleanup(deleteID string) {
	d.afterFunc(service.TrackerCleanupDelay, func() {
		d.mu.Lock()
		delete(d.m, deleteID)
		d.mu.Unlock()
	})
}
