package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
)

// TestHLSWithRealFFmpeg drives the pipeline end to end with the ffmpeg on
// PATH: the Matroska fixture is served over HTTP with Range support (as the
// loopback source route does), ffmpeg remuxes it to fragmented MP4 and the
// segments come back through Resource. Skipped when ffmpeg is missing.
func TestHLSWithRealFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	data, err := os.ReadFile("testdata/sample20s.mkv")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, HLSSourcePathPrefix) {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "sample.mkv", time.Time{}, bytes.NewReader(data))
	}))
	defer source.Close()

	s := NewHLSService(HLSConfig{
		WorkDir:        filepath.Join(t.TempDir(), "hls"),
		SegmentSeconds: 4,
		IdleTimeout:    time.Minute,
		SourceBaseURL:  source.URL,
	}, &fakeHLSFiles{data: data})
	t.Cleanup(s.Close)

	file := &domain.File{ID: uuid.New(), Path: "sample20s.mkv", Size: int64(len(data))}
	user := uuid.New()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	info, err := s.MediaInfo(ctx, file)
	if err != nil {
		t.Fatalf("media info: %v", err)
	}
	if info.DurationSeconds < 19.9 || info.DurationSeconds > 20.1 {
		t.Fatalf("duration = %v", info.DurationSeconds)
	}

	playlist, err := s.Playlist(ctx, file, 0, "")
	if err != nil {
		t.Fatalf("playlist: %v", err)
	}
	if !strings.Contains(string(playlist), "seg00004.m4s") || strings.Contains(string(playlist), "seg00005") {
		t.Fatalf("20 s / 4 s must give 5 segments:\n%s", playlist)
	}

	initPath, err := s.Resource(ctx, user, file, 0, "init.mp4")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	initData, _ := os.ReadFile(initPath)
	if len(initData) < 8 || string(initData[4:8]) != "ftyp" {
		t.Fatalf("init.mp4 is not an MP4 header: %q", initData[:min(len(initData), 16)])
	}

	for i := 0; i < 3; i++ {
		segPath, err := s.Resource(ctx, user, file, 0, hlsSegmentName(i))
		if err != nil {
			t.Fatalf("segment %d: %v", i, err)
		}
		seg, _ := os.ReadFile(segPath)
		if !bytes.Contains(seg, []byte("moof")) {
			t.Fatalf("segment %d is not a fragmented MP4 segment", i)
		}
	}

	// A fresh session starting at the last segment exercises the seek path
	// (-ss 16 with -copyts and -start_number 4) against the real binary.
	s.Stop(user, file.ID)
	segPath, err := s.Resource(ctx, user, file, 0, hlsSegmentName(4))
	if err != nil {
		t.Fatalf("seek segment: %v", err)
	}
	if filepath.Dir(segPath) == filepath.Dir(initPath) {
		t.Fatalf("stop must have ended the first session")
	}
	seg, _ := os.ReadFile(segPath)
	if !bytes.Contains(seg, []byte("moof")) {
		t.Fatalf("seek segment is not a fragmented MP4 segment")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(segPath), hlsSegmentName(3))); !os.IsNotExist(err) {
		t.Fatalf("a session started at segment 4 must not produce segment 3")
	}
	s.Stop(user, file.ID)
}
