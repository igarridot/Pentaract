package handler

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDeleteTrackersTrackReportsProgressAndOutcome(t *testing.T) {
	d := NewDeleteTrackers()
	d.afterFunc = func(time.Duration, func()) *time.Timer { return &time.Timer{} } // never clean up

	progress, finish := d.track("del-1", uuid.New())
	if progress == nil {
		t.Fatalf("expected progress for a tracked delete")
	}
	tracker, ok := d.get("del-1")
	if !ok || tracker.progress != progress {
		t.Fatalf("expected tracker registered under its id")
	}

	progress.TotalChunks = 10
	progress.DeletedChunks.Store(3)
	boom := errors.New("boom")
	finish(boom)

	done, err, total, deleted := tracker.status()
	if !done || !errors.Is(err, boom) || total != 10 || deleted != 3 {
		t.Fatalf("unexpected status: done=%v err=%v total=%d deleted=%d", done, err, total, deleted)
	}
}

func TestDeleteTrackersTrackWithoutIDIsNoop(t *testing.T) {
	d := NewDeleteTrackers()
	progress, finish := d.track("", uuid.New())
	if progress != nil {
		t.Fatalf("expected nil progress without a delete id")
	}
	finish(nil) // must not panic
	if len(d.m) != 0 {
		t.Fatalf("nothing should be registered without a delete id")
	}
}

func TestDeleteTrackersFinishSchedulesCleanup(t *testing.T) {
	d := NewDeleteTrackers()
	d.afterFunc = func(_ time.Duration, fn func()) *time.Timer {
		fn()
		return &time.Timer{}
	}

	_, finish := d.track("del-2", uuid.New())
	finish(nil)
	if _, ok := d.get("del-2"); ok {
		t.Fatalf("expected tracker to be removed once cleanup runs")
	}
}
