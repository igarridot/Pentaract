package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/media"
)

// fakeFFmpeg mimics ffmpeg for the session manager: it records its arguments,
// writes init.mp4 and then one segment every few milliseconds from
// -start_number onwards, using the same .tmp-then-rename dance as the real
// hls muxer. FAKE_FFMPEG_FAIL=1 makes it exit with an error instead.
const fakeFFmpeg = `#!/bin/sh
echo "$@" > args.txt
if [ "$FAKE_FFMPEG_FAIL" = "1" ]; then
  echo "boom: cannot open input" >&2
  exit 1
fi
start=0
while [ $# -gt 0 ]; do
  case "$1" in -start_number) start=$2; shift;; esac
  shift
done
echo init > init.mp4
i=$start
while [ $i -lt $((start+200)) ]; do
  name=$(printf 'seg%05d.m4s' $i)
  echo "segment $i" > "$name.tmp"
  mv "$name.tmp" "$name"
  i=$((i+1))
  sleep 0.02
done
`

func writeFakeFFmpeg(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(path, []byte(fakeFFmpeg), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	return path
}

type fakeHLSFiles struct {
	mu     sync.Mutex
	data   []byte
	calls  int
	ranges [][2]int64
	err    error
}

func (f *fakeHLSFiles) ExactFileSize(context.Context, *domain.File) (int64, error) {
	return int64(len(f.data)), nil
}

func (f *fakeHLSFiles) DownloadFileRangeToWriter(_ context.Context, _ *domain.File, w io.Writer, start, end, _ int64, _ *DownloadProgress) error {
	f.mu.Lock()
	f.calls++
	f.ranges = append(f.ranges, [2]int64{start, end})
	f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	end = min(end, int64(len(f.data))-1)
	_, err := w.Write(f.data[start : end+1])
	return err
}

func newTestHLSService(t *testing.T, files hlsFileReader) *HLSService {
	t.Helper()
	s := NewHLSService(HLSConfig{
		FFmpegPath:     writeFakeFFmpeg(t),
		WorkDir:        filepath.Join(t.TempDir(), "hls"),
		SegmentSeconds: 6,
		IdleTimeout:    time.Minute,
		SourceBaseURL:  "http://127.0.0.1:8000",
	}, files)
	t.Cleanup(s.Close)
	return s
}

func testMKVInfo() *media.Info {
	return &media.Info{Container: "matroska", DurationSeconds: 100, Tracks: []media.Track{
		{Number: 1, Type: media.TrackVideo, Codec: media.CodecH264},
		{Number: 2, Type: media.TrackAudio, Codec: "ac3", Default: true},
		{Number: 3, Type: media.TrackAudio, Codec: media.CodecAAC},
	}}
}

func primeInfo(s *HLSService, file *domain.File, info *media.Info) {
	s.infoMu.Lock()
	s.infos[file.ID] = info
	s.infoMu.Unlock()
}

func TestBuildHLSPlaylist(t *testing.T) {
	playlist, err := BuildHLSPlaylist(20, 6, "audio=2&access_token=tok")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:6\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n" +
		"#EXT-X-MAP:URI=\"init.mp4?audio=2&access_token=tok\"\n" +
		"#EXTINF:6.000,\nseg00000.m4s?audio=2&access_token=tok\n" +
		"#EXTINF:6.000,\nseg00001.m4s?audio=2&access_token=tok\n" +
		"#EXTINF:6.000,\nseg00002.m4s?audio=2&access_token=tok\n" +
		"#EXTINF:2.000,\nseg00003.m4s?audio=2&access_token=tok\n" +
		"#EXT-X-ENDLIST\n"
	if playlist != want {
		t.Fatalf("playlist mismatch:\n%s\nwant:\n%s", playlist, want)
	}

	plain, err := BuildHLSPlaylist(6, 6, "")
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(plain, "\nseg00000.m4s\n") || strings.Contains(plain, "seg00001") {
		t.Fatalf("exact multiple must give one segment without query:\n%s", plain)
	}

	if _, err := BuildHLSPlaylist(0, 6, ""); !errors.Is(err, ErrHLSUnknownDuration) {
		t.Fatalf("unknown duration: err = %v", err)
	}
}

func TestHLSFFmpegArgs(t *testing.T) {
	cfg := HLSConfig{SegmentSeconds: 6}
	info := testMKVInfo()
	plan, _ := media.BuildPlan(info, 0)
	args := strings.Join(hlsFFmpegArgs(cfg, "http://127.0.0.1:8000/src", plan, 12, 2), " ")

	for _, want := range []string{
		"-ss 12.000 -i http://127.0.0.1:8000/src",
		"-map 0:v:0 -map 0:a:0",
		"-c:v copy",
		"-c:a aac -b:a 192k -ac 2",
		"-copyts -avoid_negative_ts disabled",
		"-hls_segment_type fmp4",
		"-hls_flags temp_file+independent_segments",
		"-start_number 2",
		"-hls_segment_filename seg%05d.m4s ffmpeg.m3u8",
	} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q:\n%s", want, args)
		}
	}
	if strings.Contains(args, "libx264") || strings.Contains(args, "hvc1") {
		t.Errorf("h264 copy must not transcode or retag:\n%s", args)
	}

	plan, _ = media.BuildPlan(info, 3)
	args = strings.Join(hlsFFmpegArgs(cfg, "u", plan, 0, 0), " ")
	if strings.Contains(args, "-ss ") || !strings.Contains(args, "-map 0:a:1") || !strings.Contains(args, "-c:a copy") {
		t.Errorf("aac track from the start: no -ss, second audio stream copied:\n%s", args)
	}

	hevc := &media.Info{DurationSeconds: 10, Tracks: []media.Track{{Number: 1, Type: media.TrackVideo, Codec: media.CodecHEVC}}}
	plan, _ = media.BuildPlan(hevc, 0)
	args = strings.Join(hlsFFmpegArgs(cfg, "u", plan, 0, 0), " ")
	if !strings.Contains(args, "-c:v copy -tag:v hvc1") || strings.Contains(args, "-map 0:a") || strings.Contains(args, "-c:a") {
		t.Errorf("hevc without audio: copy with hvc1 tag and no audio mapping:\n%s", args)
	}

	vp9 := &media.Info{DurationSeconds: 10, Tracks: []media.Track{{Number: 1, Type: media.TrackVideo, Codec: "vp9"}}}
	plan, _ = media.BuildPlan(vp9, 0)
	args = strings.Join(hlsFFmpegArgs(cfg, "u", plan, 30, 5), " ")
	if !strings.Contains(args, "-c:v libx264") || !strings.Contains(args, "-force_key_frames expr:gte(t,30.000+n_forced*6)") {
		t.Errorf("vp9 must be transcoded with keyframes aligned to segments:\n%s", args)
	}
}

func TestParseHLSResourceName(t *testing.T) {
	if index, isInit, err := parseHLSResourceName("init.mp4"); err != nil || !isInit || index != 0 {
		t.Fatalf("init: %d %v %v", index, isInit, err)
	}
	if index, isInit, err := parseHLSResourceName("seg00042.m4s"); err != nil || isInit || index != 42 {
		t.Fatalf("segment: %d %v %v", index, isInit, err)
	}
	for _, bad := range []string{"seg42.m4s", "seg00042.ts", "../etc/passwd", "ffmpeg.m3u8", ""} {
		if _, _, err := parseHLSResourceName(bad); err == nil {
			t.Errorf("%q must be rejected", bad)
		}
	}
}

func TestHLSMediaInfoReadsHeaderThroughRangeDownload(t *testing.T) {
	data, err := os.ReadFile("../media/testdata/sample.mkv")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	files := &fakeHLSFiles{data: data}
	s := newTestHLSService(t, files)
	file := &domain.File{ID: uuid.New(), Path: "sample.mkv", Size: int64(len(data))}

	info, err := s.MediaInfo(context.Background(), file)
	if err != nil {
		t.Fatalf("media info: %v", err)
	}
	if info.Container != "matroska" || len(info.VideoTracks()) != 1 || len(info.AudioTracks()) != 2 {
		t.Fatalf("unexpected info %+v", info)
	}
	if files.ranges[0] != [2]int64{0, int64(len(data)) - 1} {
		t.Fatalf("header read must cover the file head: %v", files.ranges)
	}

	if _, err := s.MediaInfo(context.Background(), file); err != nil {
		t.Fatalf("second media info: %v", err)
	}
	if files.calls != 1 {
		t.Fatalf("info must be cached per file, got %d reads", files.calls)
	}
}

func TestHLSMediaInfoCapsHeaderReadToOneChunk(t *testing.T) {
	data, _ := os.ReadFile("../media/testdata/sample.mkv")
	files := &fakeHLSFiles{data: data}
	s := newTestHLSService(t, files)
	file := &domain.File{ID: uuid.New(), Path: "big.mkv", Size: 40 << 20}

	if _, err := s.MediaInfo(context.Background(), file); err != nil {
		t.Fatalf("media info: %v", err)
	}
	if files.ranges[0] != [2]int64{0, HLSHeaderReadLimit - 1} {
		t.Fatalf("header read must stop at the limit: %v", files.ranges)
	}
}

func TestHLSMediaInfoErrors(t *testing.T) {
	s := newTestHLSService(t, &fakeHLSFiles{data: []byte("\x00\x00\x00\x1cftypisom")})
	if _, err := s.MediaInfo(context.Background(), &domain.File{ID: uuid.New(), Path: "a.mp4", Size: 12}); !errors.Is(err, media.ErrNotMatroska) {
		t.Fatalf("mp4: err = %v", err)
	}

	transport := errors.New("telegram down")
	s = newTestHLSService(t, &fakeHLSFiles{data: []byte("x"), err: transport})
	if _, err := s.MediaInfo(context.Background(), &domain.File{ID: uuid.New(), Path: "a.mkv", Size: 100}); !errors.Is(err, transport) && !errors.Is(err, media.ErrNotMatroska) {
		t.Fatalf("transport failure: err = %v", err)
	}
}

func TestHLSResourceSessionsFollowPlayback(t *testing.T) {
	s := newTestHLSService(t, &fakeHLSFiles{})
	file := &domain.File{ID: uuid.New(), Path: "movie.mkv", Size: 1}
	primeInfo(s, file, testMKVInfo())
	user := uuid.New()
	ctx := context.Background()

	initPath, err := s.Resource(ctx, user, file, 0, "init.mp4")
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	firstDir := filepath.Dir(initPath)
	args := readArgs(t, firstDir)
	if strings.Contains(args, "-ss ") || !strings.Contains(args, "-start_number 0") {
		t.Fatalf("first session must start at 0: %s", args)
	}
	if !strings.Contains(args, HLSSourcePathPrefix) {
		t.Fatalf("ffmpeg must read from the loopback source route: %s", args)
	}

	// Sequential segments are served from the same process.
	for i := 0; i < 3; i++ {
		path, err := s.Resource(ctx, user, file, 0, hlsSegmentName(i))
		if err != nil {
			t.Fatalf("segment %d: %v", i, err)
		}
		if filepath.Dir(path) != firstDir {
			t.Fatalf("segment %d must come from the first session", i)
		}
	}

	// A far seek restarts ffmpeg at that position.
	path, err := s.Resource(ctx, user, file, 0, hlsSegmentName(12))
	if err != nil {
		t.Fatalf("seek: %v", err)
	}
	secondDir := filepath.Dir(path)
	if secondDir == firstDir {
		t.Fatalf("seek must start a new session")
	}
	args = readArgs(t, secondDir)
	if !strings.Contains(args, "-ss 72.000") || !strings.Contains(args, "-start_number 12") {
		t.Fatalf("seek session must start at segment 12 (72 s): %s", args)
	}
	waitForGone(t, firstDir)

	// Seeking back before the session start restarts as well.
	path, err = s.Resource(ctx, user, file, 0, hlsSegmentName(11))
	if err != nil {
		t.Fatalf("seek back: %v", err)
	}
	if filepath.Dir(path) == secondDir {
		t.Fatalf("backward seek must start a new session")
	}

	// The init segment follows the active session.
	initPath, err = s.Resource(ctx, user, file, 0, "init.mp4")
	if err != nil {
		t.Fatalf("init again: %v", err)
	}
	if filepath.Dir(initPath) != filepath.Dir(path) {
		t.Fatalf("init must come from the active session")
	}

	if _, err := s.Resource(ctx, user, file, 0, hlsSegmentName(17)); err == nil {
		t.Fatalf("segment past the end must fail (100 s / 6 s = 17 segments)")
	}

	// Another audio track is its own session.
	other, err := s.Resource(ctx, user, file, 3, hlsSegmentName(0))
	if err != nil {
		t.Fatalf("other audio: %v", err)
	}
	if !strings.Contains(readArgs(t, filepath.Dir(other)), "-map 0:a:1") {
		t.Fatalf("audio track 3 must map the second audio stream")
	}

	s.Stop(user, file.ID)
	waitForGone(t, filepath.Dir(path))
	waitForGone(t, filepath.Dir(other))
	s.mu.Lock()
	remaining := len(s.sessions) + len(s.sources)
	s.mu.Unlock()
	if remaining != 0 {
		t.Fatalf("stop must drop sessions and source tokens, %d left", remaining)
	}
}

func TestHLSResourceReportsFFmpegFailure(t *testing.T) {
	t.Setenv("FAKE_FFMPEG_FAIL", "1")
	s := newTestHLSService(t, &fakeHLSFiles{})
	file := &domain.File{ID: uuid.New(), Path: "movie.mkv", Size: 1}
	primeInfo(s, file, testMKVInfo())

	_, err := s.Resource(context.Background(), uuid.New(), file, 0, hlsSegmentName(0))
	if err == nil || !strings.Contains(err.Error(), "cannot open input") {
		t.Fatalf("ffmpeg failure must surface its stderr, got %v", err)
	}
}

func TestHLSResourceRejectsBadAudioTrackAndUnknownResource(t *testing.T) {
	s := newTestHLSService(t, &fakeHLSFiles{})
	file := &domain.File{ID: uuid.New(), Path: "movie.mkv", Size: 1}
	primeInfo(s, file, testMKVInfo())

	if _, err := s.Resource(context.Background(), uuid.New(), file, 9, hlsSegmentName(0)); !errors.Is(err, media.ErrAudioTrackNotFound) {
		t.Fatalf("bad audio: err = %v", err)
	}
	if _, err := s.Resource(context.Background(), uuid.New(), file, 0, "../../etc/passwd"); err == nil {
		t.Fatalf("path traversal must be rejected")
	}
	if _, err := s.Playlist(context.Background(), file, 9, ""); !errors.Is(err, media.ErrAudioTrackNotFound) {
		t.Fatalf("playlist bad audio: err = %v", err)
	}
}

func TestHLSSourceTokenResolvesWhileSessionRuns(t *testing.T) {
	s := newTestHLSService(t, &fakeHLSFiles{})
	file := &domain.File{ID: uuid.New(), Path: "movie.mkv", Size: 1}
	primeInfo(s, file, testMKVInfo())
	user := uuid.New()

	if _, err := s.Resource(context.Background(), user, file, 0, hlsSegmentName(0)); err != nil {
		t.Fatalf("segment: %v", err)
	}
	s.mu.Lock()
	var token string
	for _, sess := range s.sessions {
		token = sess.token
	}
	s.mu.Unlock()
	if got, ok := s.LookupSource(token); !ok || got != file {
		t.Fatalf("token must resolve to the file while the session runs")
	}
	if _, ok := s.LookupSource("nope"); ok {
		t.Fatalf("unknown token must not resolve")
	}
	s.Stop(user, file.ID)
	if _, ok := s.LookupSource(token); ok {
		t.Fatalf("token must die with the session")
	}
}

func TestHLSReapIdleSessions(t *testing.T) {
	s := newTestHLSService(t, &fakeHLSFiles{})
	now := time.Now()
	s.now = func() time.Time { return now }
	file := &domain.File{ID: uuid.New(), Path: "movie.mkv", Size: 1}
	primeInfo(s, file, testMKVInfo())

	path, err := s.Resource(context.Background(), uuid.New(), file, 0, hlsSegmentName(0))
	if err != nil {
		t.Fatalf("segment: %v", err)
	}
	s.reapIdle()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fresh session must survive the reaper")
	}
	now = now.Add(2 * time.Minute)
	s.reapIdle()
	waitForGone(t, filepath.Dir(path))
}

func TestHLSUnavailableWithoutFFmpeg(t *testing.T) {
	s := NewHLSService(HLSConfig{FFmpegPath: filepath.Join(t.TempDir(), "missing-ffmpeg"), WorkDir: t.TempDir()}, &fakeHLSFiles{})
	t.Cleanup(s.Close)
	if ok, reason := s.Available(); ok || !strings.Contains(reason, "ffmpeg not found") {
		t.Fatalf("expected unavailable, got %v %q", ok, reason)
	}
	file := &domain.File{ID: uuid.New(), Path: "movie.mkv", Size: 1}
	primeInfo(s, file, testMKVInfo())
	if _, err := s.Playlist(context.Background(), file, 0, ""); !errors.Is(err, ErrHLSUnavailable) {
		t.Fatalf("playlist: err = %v", err)
	}
	if _, err := s.Resource(context.Background(), uuid.New(), file, 0, "init.mp4"); !errors.Is(err, ErrHLSUnavailable) {
		t.Fatalf("resource: err = %v", err)
	}
}

func TestNewHLSServiceCleansStaleSessionDirs(t *testing.T) {
	workDir := t.TempDir()
	stale := filepath.Join(workDir, "session-old")
	keep := filepath.Join(workDir, "unrelated")
	for _, dir := range []string{stale, keep} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s := NewHLSService(HLSConfig{FFmpegPath: writeFakeFFmpeg(t), WorkDir: workDir}, &fakeHLSFiles{})
	t.Cleanup(s.Close)
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("stale session dir must be removed")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("other directories must be left alone")
	}
}

func readArgs(t *testing.T, dir string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(filepath.Join(dir, "args.txt"))
		if err == nil && bytes.Contains(data, []byte("\n")) {
			return string(data)
		}
		if time.Now().After(deadline) {
			t.Fatalf("args.txt not written in %s: %v", dir, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func waitForGone(t *testing.T, dir string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s still exists", dir)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
