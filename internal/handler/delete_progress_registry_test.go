package handler

import (
	"testing"

	"github.com/google/uuid"
)

func TestDeleteTrackerRegistry(t *testing.T) {
	id := "del-" + uuid.NewString()
	storageID := uuid.New()
	tracker := startDeleteTracker(id, storageID)
	if tracker == nil {
		t.Fatalf("expected tracker")
	}
	got, ok := getDeleteTracker(id)
	if !ok || got != tracker {
		t.Fatalf("expected tracker in registry")
	}

	tracker.progress.TotalChunks = 10
	tracker.progress.DeletedChunks.Store(3)
	markDeleteTrackerDone(tracker, nil)
	done, err, total, deleted := getDeleteTrackerStatus(tracker)
	if !done || err != nil || total != 10 || deleted != 3 {
		t.Fatalf("unexpected tracker status: done=%v err=%v total=%d deleted=%d", done, err, total, deleted)
	}
}
