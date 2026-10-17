package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/singleflight"

	"github.com/Dominux/Pentaract/internal/domain"
	"github.com/Dominux/Pentaract/internal/media"
)

// HLSConfig configures the ffmpeg-backed HLS pipeline the web UI uses to play
// Matroska files.
type HLSConfig struct {
	// FFmpegPath is the ffmpeg binary (name or path). Empty means "ffmpeg".
	FFmpegPath string
	// WorkDir holds one directory per playback session.
	WorkDir string
	// SegmentSeconds is the nominal length of each HLS segment.
	SegmentSeconds int
	// IdleTimeout kills a session nobody has asked a segment from.
	IdleTimeout time.Duration
	// SourceBaseURL is this server's own loopback address, which ffmpeg reads
	// the Matroska file from through the ranged inline download.
	SourceBaseURL string
}

const (
	// HLSHeaderReadLimit caps how much of a file is read to parse its
	// Matroska header. It stays below one chunk so the read never fans out
	// into several Telegram downloads.
	HLSHeaderReadLimit = 8 << 20
	// hlsMaxWaitAhead is how many segments past the last produced one a
	// request may wait for before the session is restarted at its position.
	hlsMaxWaitAhead = 3
	// hlsSegmentWaitTimeout bounds how long a segment request waits for
	// ffmpeg. Telegram can take minutes to hand over a chunk.
	hlsSegmentWaitTimeout  = 4 * time.Minute
	hlsSegmentPollInterval = 100 * time.Millisecond
	hlsReaperInterval      = 15 * time.Second
	hlsStderrLimit         = 8 << 10
	hlsInitSegmentName     = "init.mp4"
	hlsFFmpegPlaylist      = "ffmpeg.m3u8"
	// HLSSourcePathPrefix is the loopback-only route ffmpeg reads from.
	HLSSourcePathPrefix = "/api/internal/hls_source/"
)

var hlsSegmentNameRe = regexp.MustCompile(`^seg(\d{5})\.m4s$`)

// ErrHLSUnavailable reports that the server cannot run ffmpeg.
var ErrHLSUnavailable = errors.New("hls playback unavailable")

// ErrHLSUnknownDuration reports a file whose header carries no duration, so
// no VOD playlist can be built for it.
var ErrHLSUnknownDuration = errors.New("file duration unknown")

// hlsFileReader is what the pipeline needs from the files service.
type hlsFileReader interface {
	ExactFileSize(ctx context.Context, file *domain.File) (int64, error)
	DownloadFileRangeToWriter(ctx context.Context, file *domain.File, w io.Writer, start, end, totalSize int64, progress *DownloadProgress) error
}

type hlsSessionKey struct {
	userID uuid.UUID
	fileID uuid.UUID
	audio  uint64
}

// hlsSession is one running ffmpeg process writing segments from startIndex
// onwards into dir.
type hlsSession struct {
	key        hlsSessionKey
	dir        string
	token      string
	startIndex int
	produced   int // highest segment index known to exist
	lastSeen   time.Time
	cancel     context.CancelFunc
	done       chan struct{}
	exitErr    error
	stderr     *tailBuffer
}

// HLSService turns Matroska files into HLS on demand. One ffmpeg process per
// (user, file, audio track) writes fragmented MP4 segments into a scratch
// directory; a segment request either finds its file, waits for it when the
// process is about to produce it, or restarts ffmpeg at that position (a
// seek). Video is copied when it is H.264/HEVC and audio when it is AAC;
// anything else is transcoded.
type HLSService struct {
	cfg         HLSConfig
	files       hlsFileReader
	unavailable string

	mu       sync.Mutex
	sessions map[hlsSessionKey]*hlsSession
	sources  map[string]*domain.File

	infoMu    sync.Mutex
	infos     map[uuid.UUID]*media.Info
	infoLoads singleflight.Group

	now        func() time.Time
	stopReaper chan struct{}
	reaperDone chan struct{}
}

// NewHLSService builds the pipeline. A missing ffmpeg does not fail startup:
// the service reports itself unavailable and playback requests say why.
func NewHLSService(cfg HLSConfig, files hlsFileReader) *HLSService {
	if cfg.FFmpegPath == "" {
		cfg.FFmpegPath = "ffmpeg"
	}
	if cfg.SegmentSeconds <= 0 {
		cfg.SegmentSeconds = 6
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = 2 * time.Minute
	}
	if cfg.WorkDir == "" {
		cfg.WorkDir = filepath.Join(os.TempDir(), "pentaract-hls")
	}
	s := &HLSService{
		cfg:        cfg,
		files:      files,
		sessions:   make(map[hlsSessionKey]*hlsSession),
		sources:    make(map[string]*domain.File),
		infos:      make(map[uuid.UUID]*media.Info),
		now:        time.Now,
		stopReaper: make(chan struct{}),
		reaperDone: make(chan struct{}),
	}
	if _, err := exec.LookPath(cfg.FFmpegPath); err != nil {
		s.unavailable = fmt.Sprintf("ffmpeg not found (%s); install it or set FFMPEG_PATH", cfg.FFmpegPath)
		slog.Warn("hls playback disabled", "reason", s.unavailable)
	}
	if err := os.MkdirAll(cfg.WorkDir, 0o755); err != nil {
		s.unavailable = fmt.Sprintf("cannot create HLS work dir %s: %v", cfg.WorkDir, err)
		slog.Warn("hls playback disabled", "reason", s.unavailable)
	} else {
		removeStaleSessionDirs(cfg.WorkDir)
	}
	go s.reap()
	return s
}

// removeStaleSessionDirs drops session directories a previous process left
// behind. Only our own "session-*" entries are touched.
func removeStaleSessionDirs(workDir string) {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() && len(entry.Name()) > 8 && entry.Name()[:8] == "session-" {
			_ = os.RemoveAll(filepath.Join(workDir, entry.Name()))
		}
	}
}

// Available reports whether playback can run and, if not, why.
func (s *HLSService) Available() (bool, string) {
	return s.unavailable == "", s.unavailable
}

// SegmentSeconds returns the nominal segment length.
func (s *HLSService) SegmentSeconds() int { return s.cfg.SegmentSeconds }

// Close stops every session and the reaper.
func (s *HLSService) Close() {
	close(s.stopReaper)
	<-s.reaperDone
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, sess := range s.sessions {
		s.stopSessionLocked(sess)
	}
}

// MediaInfo parses the Matroska header of file, reading at most
// HLSHeaderReadLimit bytes through the ranged download. Results are cached
// per file ID: a stored file never changes.
func (s *HLSService) MediaInfo(ctx context.Context, file *domain.File) (*media.Info, error) {
	s.infoMu.Lock()
	info, ok := s.infos[file.ID]
	s.infoMu.Unlock()
	if ok {
		return info, nil
	}

	value, err, _ := s.infoLoads.Do(file.ID.String(), func() (any, error) {
		// The header read runs on its own clock so a cancelled UI request
		// does not fail the other callers sharing this load.
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Minute)
		defer cancel()
		info, err := s.readInfo(loadCtx, file)
		if err != nil {
			return nil, err
		}
		s.infoMu.Lock()
		if len(s.infos) >= 1024 {
			s.infos = make(map[uuid.UUID]*media.Info)
		}
		s.infos[file.ID] = info
		s.infoMu.Unlock()
		return info, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*media.Info), nil
}

var errHLSHeaderParsed = errors.New("header parsed")

func (s *HLSService) readInfo(ctx context.Context, file *domain.File) (*media.Info, error) {
	total := file.Size
	if total <= 0 {
		var err error
		if total, err = s.files.ExactFileSize(ctx, file); err != nil {
			return nil, err
		}
	}
	end := min(total, HLSHeaderReadLimit) - 1
	if end < 0 {
		return nil, media.ErrNotMatroska
	}

	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	pr, pw := io.Pipe()
	writerDone := make(chan error, 1)
	go func() {
		err := s.files.DownloadFileRangeToWriter(readCtx, file, pw, 0, end, total, nil)
		pw.CloseWithError(err)
		writerDone <- err
	}()

	info, parseErr := media.ParseMatroska(io.LimitReader(pr, HLSHeaderReadLimit))
	pr.CloseWithError(errHLSHeaderParsed)
	cancel()
	writeErr := <-writerDone

	if parseErr != nil {
		// A transport failure explains an incomplete header better than
		// the parser's own message does.
		if writeErr != nil && !errors.Is(writeErr, errHLSHeaderParsed) && !errors.Is(writeErr, io.ErrClosedPipe) && errors.Is(parseErr, media.ErrHeaderIncomplete) {
			return nil, fmt.Errorf("reading media header: %w", writeErr)
		}
		return nil, parseErr
	}
	return info, nil
}

// Playlist returns the VOD playlist for file. query is appended verbatim to
// every segment URI so the player's credentials (access_token) and audio
// track choice travel with each request.
func (s *HLSService) Playlist(ctx context.Context, file *domain.File, audio uint64, query string) ([]byte, error) {
	if ok, reason := s.Available(); !ok {
		return nil, fmt.Errorf("%w: %s", ErrHLSUnavailable, reason)
	}
	info, err := s.MediaInfo(ctx, file)
	if err != nil {
		return nil, err
	}
	if _, err := media.BuildPlan(info, audio); err != nil {
		return nil, err
	}
	playlist, err := BuildHLSPlaylist(info.DurationSeconds, s.cfg.SegmentSeconds, query)
	if err != nil {
		return nil, err
	}
	return []byte(playlist), nil
}

// Resource returns the local path of an init segment or media segment, once
// ffmpeg has written it. name is "init.mp4" or "segNNNNN.m4s".
func (s *HLSService) Resource(ctx context.Context, userID uuid.UUID, file *domain.File, audio uint64, name string) (string, error) {
	if ok, reason := s.Available(); !ok {
		return "", fmt.Errorf("%w: %s", ErrHLSUnavailable, reason)
	}
	index, isInit, err := parseHLSResourceName(name)
	if err != nil {
		return "", err
	}
	info, err := s.MediaInfo(ctx, file)
	if err != nil {
		return "", err
	}
	plan, err := media.BuildPlan(info, audio)
	if err != nil {
		return "", err
	}
	segments, err := hlsSegmentCount(info.DurationSeconds, s.cfg.SegmentSeconds)
	if err != nil {
		return "", err
	}

	key := hlsSessionKey{userID: userID, fileID: file.ID, audio: audio}
	s.mu.Lock()
	sess := s.sessions[key]
	if isInit {
		if sess != nil {
			index = sess.startIndex
		} else {
			index = 0
		}
	}
	if index >= segments {
		s.mu.Unlock()
		return "", domain.ErrNotFound("segment out of range")
	}
	if sess == nil || !sess.covers(index) {
		if sess != nil {
			s.stopSessionLocked(sess)
		}
		sess, err = s.startSessionLocked(key, file, plan, index)
		if err != nil {
			s.mu.Unlock()
			return "", err
		}
	}
	sess.lastSeen = s.now()
	s.mu.Unlock()

	return sess.waitFor(ctx, name, hlsSegmentWaitTimeout)
}

// Stop ends every session of userID on fileID, whatever audio track.
func (s *HLSService) Stop(userID, fileID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, sess := range s.sessions {
		if key.userID == userID && key.fileID == fileID {
			s.stopSessionLocked(sess)
		}
	}
}

// LookupSource resolves the token of a loopback source URL to its file.
func (s *HLSService) LookupSource(token string) (*domain.File, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, ok := s.sources[token]
	return file, ok
}

func (s *HLSService) startSessionLocked(key hlsSessionKey, file *domain.File, plan media.Plan, startIndex int) (*hlsSession, error) {
	dir, err := os.MkdirTemp(s.cfg.WorkDir, "session-")
	if err != nil {
		return nil, fmt.Errorf("creating hls session dir: %w", err)
	}
	token, err := randomToken()
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	sourceURL := s.cfg.SourceBaseURL + HLSSourcePathPrefix + token
	startSeconds := float64(startIndex) * float64(s.cfg.SegmentSeconds)
	args := hlsFFmpegArgs(s.cfg, sourceURL, plan, startSeconds, startIndex)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, s.cfg.FFmpegPath, args...)
	cmd.Dir = dir
	stderr := &tailBuffer{limit: hlsStderrLimit}
	cmd.Stderr = stderr
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Start(); err != nil {
		cancel()
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("starting ffmpeg: %w", err)
	}
	slog.Info("hls session started", "file", file.Path, "start_index", startIndex, "video_copy", plan.VideoCopy, "audio_copy", plan.AudioCopy)

	sess := &hlsSession{
		key:        key,
		dir:        dir,
		token:      token,
		startIndex: startIndex,
		produced:   startIndex - 1,
		lastSeen:   s.now(),
		cancel:     cancel,
		done:       make(chan struct{}),
		stderr:     stderr,
	}
	go func() {
		sess.exitErr = cmd.Wait()
		close(sess.done)
		if sess.exitErr != nil && ctx.Err() == nil {
			slog.Warn("ffmpeg exited with error", "file", file.Path, "err", sess.exitErr, "stderr", stderr.String())
		}
	}()
	s.sessions[key] = sess
	s.sources[token] = file
	return sess, nil
}

func (s *HLSService) stopSessionLocked(sess *hlsSession) {
	if current, ok := s.sessions[sess.key]; ok && current == sess {
		delete(s.sessions, sess.key)
	}
	delete(s.sources, sess.token)
	sess.cancel()
	go func() {
		<-sess.done
		_ = os.RemoveAll(sess.dir)
	}()
}

func (s *HLSService) reap() {
	defer close(s.reaperDone)
	ticker := time.NewTicker(hlsReaperInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopReaper:
			return
		case <-ticker.C:
			s.reapIdle()
		}
	}
}

func (s *HLSService) reapIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := s.now().Add(-s.cfg.IdleTimeout)
	for _, sess := range s.sessions {
		if sess.lastSeen.Before(cutoff) {
			slog.Info("hls session idle, stopping", "dir", sess.dir)
			s.stopSessionLocked(sess)
		}
	}
}

// covers reports whether the session already has, or is about to produce,
// segment index. Must be called with HLSService.mu held.
func (sess *hlsSession) covers(index int) bool {
	if index < sess.startIndex {
		return false
	}
	if sess.exists(index) {
		return true
	}
	if sess.finished() {
		return false
	}
	return index-sess.lastProduced() <= hlsMaxWaitAhead
}

func (sess *hlsSession) finished() bool {
	select {
	case <-sess.done:
		return true
	default:
		return false
	}
}

func (sess *hlsSession) exists(index int) bool {
	_, err := os.Stat(filepath.Join(sess.dir, hlsSegmentName(index)))
	return err == nil
}

func (sess *hlsSession) lastProduced() int {
	for sess.exists(sess.produced + 1) {
		sess.produced++
	}
	return sess.produced
}

// waitFor blocks until name exists in the session directory, ffmpeg exits
// without producing it, ctx ends or timeout passes.
func (sess *hlsSession) waitFor(ctx context.Context, name string, timeout time.Duration) (string, error) {
	path := filepath.Join(sess.dir, name)
	ticker := time.NewTicker(hlsSegmentPollInterval)
	defer ticker.Stop()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
		select {
		case <-sess.done:
			if _, err := os.Stat(path); err == nil {
				return path, nil
			}
			return "", fmt.Errorf("ffmpeg ended before producing %s: %w (%s)", name, sess.exitError(), sess.stderr.String())
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline.C:
			return "", fmt.Errorf("timed out waiting for %s", name)
		case <-ticker.C:
		}
	}
}

func (sess *hlsSession) exitError() error {
	if sess.exitErr == nil {
		return errors.New("exit status 0")
	}
	return sess.exitErr
}

// hlsFFmpegArgs builds the ffmpeg command line for a session starting at
// startSeconds and numbering its segments from startIndex. Input seeking
// (-ss before -i) uses the Matroska cues, and -copyts keeps the original
// timestamps so every session's segments line up on the file's timeline no
// matter where it started.
func hlsFFmpegArgs(cfg HLSConfig, sourceURL string, plan media.Plan, startSeconds float64, startIndex int) []string {
	segment := strconv.Itoa(cfg.SegmentSeconds)
	args := []string{
		"-hide_banner", "-loglevel", "warning", "-nostdin",
		// Telegram can take minutes to hand over a chunk: never time out
		// a read early, and reconnect if the loopback connection drops.
		"-rw_timeout", "180000000",
		"-seekable", "1",
		"-reconnect", "1", "-reconnect_on_network_error", "1", "-reconnect_delay_max", "15",
	}
	if startSeconds > 0 {
		args = append(args, "-ss", formatSeconds(startSeconds))
	}
	args = append(args, "-i", sourceURL, "-map", fmt.Sprintf("0:v:%d", plan.VideoIndex))
	if plan.Audio != nil {
		args = append(args, "-map", fmt.Sprintf("0:a:%d", plan.AudioIndex))
	}

	if plan.VideoCopy {
		args = append(args, "-c:v", "copy")
		if plan.Video.Codec == media.CodecHEVC {
			// Safari only plays HEVC tagged hvc1.
			args = append(args, "-tag:v", "hvc1")
		}
	} else {
		args = append(args,
			"-c:v", "libx264", "-preset", "veryfast", "-crf", "23", "-pix_fmt", "yuv420p",
			"-force_key_frames", fmt.Sprintf("expr:gte(t,%s+n_forced*%s)", formatSeconds(startSeconds), segment),
		)
	}
	if plan.Audio != nil {
		if plan.AudioCopy {
			args = append(args, "-c:a", "copy")
		} else {
			args = append(args, "-c:a", "aac", "-b:a", "192k", "-ac", "2")
		}
	}

	args = append(args,
		"-sn", "-dn",
		"-copyts", "-avoid_negative_ts", "disabled",
		"-f", "hls",
		"-hls_time", segment,
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", hlsInitSegmentName,
		"-hls_flags", "temp_file+independent_segments",
		"-hls_list_size", "0",
		"-hls_playlist_type", "vod",
		"-start_number", strconv.Itoa(startIndex),
		"-hls_segment_filename", "seg%05d.m4s",
		hlsFFmpegPlaylist,
	)
	return args
}

func formatSeconds(seconds float64) string {
	return strconv.FormatFloat(seconds, 'f', 3, 64)
}

func hlsSegmentName(index int) string {
	return fmt.Sprintf("seg%05d.m4s", index)
}

// parseHLSResourceName accepts "init.mp4" (isInit) or "segNNNNN.m4s".
func parseHLSResourceName(name string) (index int, isInit bool, err error) {
	if name == hlsInitSegmentName {
		return 0, true, nil
	}
	match := hlsSegmentNameRe.FindStringSubmatch(name)
	if match == nil {
		return 0, false, domain.ErrNotFound("unknown hls resource")
	}
	index, err = strconv.Atoi(match[1])
	if err != nil {
		return 0, false, domain.ErrNotFound("unknown hls resource")
	}
	return index, false, nil
}

func hlsSegmentCount(durationSeconds float64, segmentSeconds int) (int, error) {
	if durationSeconds <= 0 || segmentSeconds <= 0 {
		return 0, ErrHLSUnknownDuration
	}
	count := int(durationSeconds / float64(segmentSeconds))
	if float64(count)*float64(segmentSeconds) < durationSeconds {
		count++
	}
	return count, nil
}

// BuildHLSPlaylist writes a VOD playlist of uniform segments. Real segments
// cut at keyframes and so run a little longer than announced; players
// realign on the timestamps inside each fragment (-copyts keeps them
// absolute), so the announced durations only need to be close.
func BuildHLSPlaylist(durationSeconds float64, segmentSeconds int, query string) (string, error) {
	count, err := hlsSegmentCount(durationSeconds, segmentSeconds)
	if err != nil {
		return "", err
	}
	suffix := ""
	if query != "" {
		suffix = "?" + query
	}
	var b []byte
	b = fmt.Appendf(b, "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-INDEPENDENT-SEGMENTS\n", segmentSeconds)
	b = fmt.Appendf(b, "#EXT-X-MAP:URI=\"%s%s\"\n", hlsInitSegmentName, suffix)
	remaining := durationSeconds
	for i := range count {
		length := min(float64(segmentSeconds), remaining)
		remaining -= length
		b = fmt.Appendf(b, "#EXTINF:%s,\n%s%s\n", formatSeconds(length), hlsSegmentName(i), suffix)
	}
	b = append(b, "#EXT-X-ENDLIST\n"...)
	return string(b), nil
}

func randomToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generating token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// tailBuffer keeps the last limit bytes written to it.
type tailBuffer struct {
	mu    sync.Mutex
	limit int
	data  []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > b.limit {
		b.data = b.data[len(b.data)-b.limit:]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
