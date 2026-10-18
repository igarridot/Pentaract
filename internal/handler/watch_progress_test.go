package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

type mockWatchProgressService struct {
	saveFn     func(ctx context.Context, userID, storageID uuid.UUID, path string, position, duration float64) (*domain.WatchProgress, error)
	setWatchedFn func(ctx context.Context, userID, storageID uuid.UUID, path string, watched bool) (*domain.WatchProgress, error)
	annotateFn   func(ctx context.Context, userID, storageID uuid.UUID, elements []domain.FSElement) error
}

func (m *mockWatchProgressService) SetWatched(ctx context.Context, userID, storageID uuid.UUID, path string, watched bool) (*domain.WatchProgress, error) {
	if m.setWatchedFn == nil {
		return &domain.WatchProgress{Watched: watched}, nil
	}
	return m.setWatchedFn(ctx, userID, storageID, path, watched)
}

func (m *mockWatchProgressService) Save(ctx context.Context, userID, storageID uuid.UUID, path string, position, duration float64) (*domain.WatchProgress, error) {
	if m.saveFn == nil {
		return &domain.WatchProgress{PositionSeconds: position, DurationSeconds: duration}, nil
	}
	return m.saveFn(ctx, userID, storageID, path, position, duration)
}
func (m *mockWatchProgressService) Annotate(ctx context.Context, userID, storageID uuid.UUID, elements []domain.FSElement) error {
	if m.annotateFn == nil {
		return nil
	}
	return m.annotateFn(ctx, userID, storageID, elements)
}

func TestSaveWatchProgressPassesPositionAndAnswersStored(t *testing.T) {
	var gotPath string
	var gotPosition, gotDuration float64
	watch := &mockWatchProgressService{
		saveFn: func(ctx context.Context, userID, storageID uuid.UUID, path string, position, duration float64) (*domain.WatchProgress, error) {
			gotPath, gotPosition, gotDuration = path, position, duration
			return &domain.WatchProgress{PositionSeconds: 0, DurationSeconds: duration, Watched: true}, nil
		},
	}
	h := NewFilesHandler(&mockFilesService{}, watch, NewDeleteTrackers())

	w := httptest.NewRecorder()
	body := `{"position_seconds": 5400.5, "duration_seconds": 5600}`
	h.SaveWatchProgress(w, makeFilesReq(http.MethodPut, "/", body, uuid.New().String(), "movies/a%20b.mkv"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if gotPath != "movies/a b.mkv" || gotPosition != 5400.5 || gotDuration != 5600 {
		t.Fatalf("unexpected call: path=%q position=%v duration=%v", gotPath, gotPosition, gotDuration)
	}
	var resp domain.WatchProgress
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || !resp.Watched {
		t.Fatalf("expected stored progress in body, got %s (err %v)", w.Body.String(), err)
	}
}

func TestSaveWatchProgressRejectsBadRequests(t *testing.T) {
	h := newTestFilesHandler(&mockFilesService{})
	storageID := uuid.New().String()

	for name, tc := range map[string]struct{ body, path string }{
		"invalid body": {body: "{", path: "a.mkv"},
		"no path":      {body: `{"position_seconds": 1, "duration_seconds": 2}`, path: ""},
	} {
		w := httptest.NewRecorder()
		h.SaveWatchProgress(w, makeFilesReq(http.MethodPut, "/", tc.body, storageID, tc.path))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", name, w.Code)
		}
	}
}

func TestSaveWatchProgressReturnsServiceErrors(t *testing.T) {
	watch := &mockWatchProgressService{
		saveFn: func(ctx context.Context, userID, storageID uuid.UUID, path string, position, duration float64) (*domain.WatchProgress, error) {
			return nil, domain.ErrNotFound("file")
		},
	}
	h := NewFilesHandler(&mockFilesService{}, watch, NewDeleteTrackers())

	w := httptest.NewRecorder()
	h.SaveWatchProgress(w, makeFilesReq(http.MethodPut, "/", `{"position_seconds": 1, "duration_seconds": 2}`, uuid.New().String(), "gone.mkv"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestTreeAndSearchIncludeWatchProgress(t *testing.T) {
	listing := func() []domain.FSElement {
		return []domain.FSElement{{Path: "a.mkv", Name: "a.mkv", IsFile: true}, {Path: "dir/", Name: "dir"}}
	}
	svc := &mockFilesService{
		listDirFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) ([]domain.FSElement, error) {
			return listing(), nil
		},
		searchFn: func(ctx context.Context, userID, storageID uuid.UUID, basePath, searchPath string) ([]domain.FSElement, error) {
			return listing(), nil
		},
	}
	watch := &mockWatchProgressService{
		annotateFn: func(ctx context.Context, userID, storageID uuid.UUID, elements []domain.FSElement) error {
			elements[0].Watch = &domain.WatchProgress{PositionSeconds: 42, DurationSeconds: 100}
			return nil
		},
	}
	h := NewFilesHandler(svc, watch, NewDeleteTrackers())
	storageID := uuid.New().String()

	for name, serve := range map[string]http.HandlerFunc{"tree": h.Tree, "search": h.Search} {
		w := httptest.NewRecorder()
		serve(w, makeFilesReq(http.MethodGet, "/?search_path=a", "", storageID, ""))
		var got []domain.FSElement
		if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if got[0].Watch == nil || got[0].Watch.PositionSeconds != 42 || got[1].Watch != nil {
			t.Fatalf("%s: unexpected watch progress: %s", name, w.Body.String())
		}
	}
}

func TestTreeStillListsWhenWatchProgressFails(t *testing.T) {
	svc := &mockFilesService{
		listDirFn: func(ctx context.Context, userID, storageID uuid.UUID, path string) ([]domain.FSElement, error) {
			return []domain.FSElement{{Path: "a.mkv", Name: "a.mkv", IsFile: true}}, nil
		},
	}
	watch := &mockWatchProgressService{
		annotateFn: func(ctx context.Context, userID, storageID uuid.UUID, elements []domain.FSElement) error {
			return errors.New("db down")
		},
	}
	h := NewFilesHandler(svc, watch, NewDeleteTrackers())

	w := httptest.NewRecorder()
	h.Tree(w, makeFilesReq(http.MethodGet, "/", "", uuid.New().String(), ""))
	if w.Code != http.StatusOK {
		t.Fatalf("expected the listing anyway, got %d", w.Code)
	}
}

func TestSetWatchedPassesTheMark(t *testing.T) {
	var gotPath string
	var gotWatched *bool
	watch := &mockWatchProgressService{
		setWatchedFn: func(ctx context.Context, userID, storageID uuid.UUID, path string, watched bool) (*domain.WatchProgress, error) {
			gotPath, gotWatched = path, &watched
			return &domain.WatchProgress{Watched: watched, DurationSeconds: 100}, nil
		},
	}
	h := NewFilesHandler(&mockFilesService{}, watch, NewDeleteTrackers())
	storageID := uuid.New().String()

	for _, watched := range []bool{true, false} {
		body := `{"watched": false}`
		if watched {
			body = `{"watched": true}`
		}
		w := httptest.NewRecorder()
		h.SetWatched(w, makeFilesReq(http.MethodPut, "/", body, storageID, "tv/ep%201.mkv"))
		if w.Code != http.StatusOK || gotPath != "tv/ep 1.mkv" || gotWatched == nil || *gotWatched != watched {
			t.Fatalf("watched=%v: code %d path %q body %s", watched, w.Code, gotPath, w.Body.String())
		}
		var resp domain.WatchProgress
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil || resp.Watched != watched {
			t.Fatalf("watched=%v: unexpected body %s", watched, w.Body.String())
		}
	}
}

func TestSetWatchedRejectsBadRequests(t *testing.T) {
	called := false
	watch := &mockWatchProgressService{
		setWatchedFn: func(ctx context.Context, userID, storageID uuid.UUID, path string, watched bool) (*domain.WatchProgress, error) {
			called = true
			return nil, nil
		},
	}
	h := NewFilesHandler(&mockFilesService{}, watch, NewDeleteTrackers())
	storageID := uuid.New().String()

	// A missing "watched" must not be read as false: that would silently
	// mark the video pending.
	for name, tc := range map[string]struct{ body, path string }{
		"invalid body":    {body: "{", path: "a.mkv"},
		"missing watched": {body: `{}`, path: "a.mkv"},
		"no path":         {body: `{"watched": true}`, path: ""},
	} {
		w := httptest.NewRecorder()
		h.SetWatched(w, makeFilesReq(http.MethodPut, "/", tc.body, storageID, tc.path))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: expected 400, got %d", name, w.Code)
		}
	}
	if called {
		t.Fatalf("a bad request must not reach the service")
	}

	watch.setWatchedFn = func(ctx context.Context, userID, storageID uuid.UUID, path string, watched bool) (*domain.WatchProgress, error) {
		return nil, domain.ErrForbidden()
	}
	w := httptest.NewRecorder()
	h.SetWatched(w, makeFilesReq(http.MethodPut, "/", `{"watched": true}`, storageID, "a.mkv"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected the service error, got %d", w.Code)
	}
}
