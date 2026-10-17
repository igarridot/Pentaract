package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/Dominux/Pentaract/internal/domain"
	appjwt "github.com/Dominux/Pentaract/internal/jwt"
	"github.com/Dominux/Pentaract/internal/media"
	"github.com/Dominux/Pentaract/internal/service"
)

type fakeHLS struct {
	available   bool
	reason      string
	infoFn      func(ctx context.Context, file *domain.File) (*media.Info, error)
	playlistFn  func(ctx context.Context, file *domain.File, variant service.HLSVariant, query string) ([]byte, error)
	resourceFn  func(ctx context.Context, userID uuid.UUID, file *domain.File, variant service.HLSVariant, name string) (string, error)
	stopped     []uuid.UUID
	stoppedBy   uuid.UUID
	sourceToken string
	sourceFile  *domain.File
}

func (f *fakeHLS) Available() (bool, string) { return f.available, f.reason }
func (f *fakeHLS) SegmentSeconds() int       { return 6 }
func (f *fakeHLS) MediaInfo(ctx context.Context, file *domain.File) (*media.Info, error) {
	return f.infoFn(ctx, file)
}
func (f *fakeHLS) Playlist(ctx context.Context, file *domain.File, variant service.HLSVariant, query string) ([]byte, error) {
	return f.playlistFn(ctx, file, variant, query)
}
func (f *fakeHLS) Resource(ctx context.Context, userID uuid.UUID, file *domain.File, variant service.HLSVariant, name string) (string, error) {
	return f.resourceFn(ctx, userID, file, variant, name)
}
func (f *fakeHLS) Stop(userID, fileID uuid.UUID) {
	f.stoppedBy = userID
	f.stopped = append(f.stopped, fileID)
}
func (f *fakeHLS) LookupSource(token string) (*domain.File, bool) {
	if token == f.sourceToken && f.sourceFile != nil {
		return f.sourceFile, true
	}
	return nil, false
}

func mkvInfo() *media.Info {
	return &media.Info{Container: "matroska", DurationSeconds: 100, Tracks: []media.Track{
		{Number: 1, Type: media.TrackVideo, Codec: media.CodecH264, CodecID: "V_MPEG4/ISO/AVC", Default: true},
		{Number: 2, Type: media.TrackAudio, Codec: "ac3", CodecID: "A_AC3", Channels: 6, Default: true},
	}}
}

// mediaTestRouter mounts the media handler on the real route shapes so the
// wildcard and token parameters are parsed as in production.
func mediaTestRouter(svc filesService, hls *fakeHLS) (http.Handler, *MediaHandler) {
	files := newTestFilesHandler(svc)
	h := NewMediaHandler(svc, hls, files.DownloadHandler)
	r := chi.NewRouter()
	r.Get("/internal/hls_source/{token}", h.HLSSource)
	r.Group(func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				ctx := context.WithValue(req.Context(), authUserKey, &appjwt.AuthUser{ID: testMediaUser, Email: "u@example.com"})
				next.ServeHTTP(w, req.WithContext(ctx))
			})
		})
		r.Get("/storages/{storageID}/files/media_info/*", h.MediaInfo)
		r.Get("/storages/{storageID}/files/hls/*", h.HLS)
		r.Post("/storages/{storageID}/files/hls_stop/*", h.HLSStop)
	})
	return r, h
}

var testMediaUser = uuid.New()

func mediaFileService(file *domain.File) *mockFilesService {
	return &mockFilesService{
		getFileForDownloadFn: func(_ context.Context, userID, _ uuid.UUID, path string) (*domain.File, error) {
			if userID != testMediaUser {
				return nil, domain.ErrForbidden()
			}
			if path != file.Path {
				return nil, domain.ErrNotFound("file")
			}
			return file, nil
		},
	}
}

func TestMediaInfoDescribesMatroskaFile(t *testing.T) {
	file := &domain.File{ID: uuid.New(), Path: "shows/ep 1.mkv", Size: 10}
	hls := &fakeHLS{available: true, infoFn: func(context.Context, *domain.File) (*media.Info, error) { return mkvInfo(), nil }}
	router, _ := mediaTestRouter(mediaFileService(file), hls)

	req := httptest.NewRequest(http.MethodGet, "/storages/"+uuid.NewString()+"/files/media_info/shows/ep%201.mkv", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", w.Code, w.Body.String())
	}
	var resp mediaInfoResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Container != "matroska" || resp.DurationSeconds != 100 || len(resp.Tracks) != 2 {
		t.Fatalf("unexpected response %+v", resp)
	}
	if !resp.HLS.Available || !resp.HLS.VideoCopy || resp.HLS.AudioCopy || resp.HLS.SegmentSeconds != 6 {
		t.Fatalf("h264+ac3 must copy video and transcode audio: %+v", resp.HLS)
	}
}

func TestMediaInfoExplainsWhyPlaybackIsUnavailable(t *testing.T) {
	file := &domain.File{ID: uuid.New(), Path: "a.mkv", Size: 10}
	cases := map[string]struct {
		hls  *fakeHLS
		want func(t *testing.T, resp mediaInfoResponse)
	}{
		"no ffmpeg": {
			hls: &fakeHLS{available: false, reason: "ffmpeg not found", infoFn: func(context.Context, *domain.File) (*media.Info, error) { return mkvInfo(), nil }},
			want: func(t *testing.T, resp mediaInfoResponse) {
				if resp.HLS.Available || resp.HLS.Reason != "ffmpeg not found" || resp.Container != "matroska" {
					t.Fatalf("unexpected %+v", resp)
				}
			},
		},
		"not matroska": {
			hls: &fakeHLS{available: true, infoFn: func(context.Context, *domain.File) (*media.Info, error) { return nil, media.ErrNotMatroska }},
			want: func(t *testing.T, resp mediaInfoResponse) {
				if resp.Container != "unsupported" || resp.HLS.Available || resp.HLS.Reason == "" {
					t.Fatalf("unexpected %+v", resp)
				}
			},
		},
		"header read failed": {
			hls: &fakeHLS{available: true, infoFn: func(context.Context, *domain.File) (*media.Info, error) { return nil, errors.New("telegram down") }},
			want: func(t *testing.T, resp mediaInfoResponse) {
				if resp.Container != "unknown" || resp.Error != "telegram down" || resp.HLS.Available {
					t.Fatalf("unexpected %+v", resp)
				}
			},
		},
		"no duration": {
			hls: &fakeHLS{available: true, infoFn: func(context.Context, *domain.File) (*media.Info, error) {
				info := mkvInfo()
				info.DurationSeconds = 0
				return info, nil
			}},
			want: func(t *testing.T, resp mediaInfoResponse) {
				if resp.HLS.Available || !strings.Contains(resp.HLS.Reason, "duration") {
					t.Fatalf("unexpected %+v", resp)
				}
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			router, _ := mediaTestRouter(mediaFileService(file), tc.hls)
			req := httptest.NewRequest(http.MethodGet, "/storages/"+uuid.NewString()+"/files/media_info/a.mkv", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d body %s", w.Code, w.Body.String())
			}
			var resp mediaInfoResponse
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			tc.want(t, resp)
		})
	}
}

func TestMediaInfoRequiresExistingFile(t *testing.T) {
	file := &domain.File{ID: uuid.New(), Path: "a.mkv", Size: 10}
	router, _ := mediaTestRouter(mediaFileService(file), &fakeHLS{available: true})
	req := httptest.NewRequest(http.MethodGet, "/storages/"+uuid.NewString()+"/files/media_info/missing.mkv", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestHLSPlaylistEchoesQueryAndAudioTrack(t *testing.T) {
	file := &domain.File{ID: uuid.New(), Path: "dir/movie.mkv", Size: 10}
	var gotVariant service.HLSVariant
	var gotQuery string
	hls := &fakeHLS{available: true, playlistFn: func(_ context.Context, _ *domain.File, variant service.HLSVariant, query string) ([]byte, error) {
		gotVariant, gotQuery = variant, query
		return []byte("#EXTM3U\n"), nil
	}}
	router, _ := mediaTestRouter(mediaFileService(file), hls)

	req := httptest.NewRequest(http.MethodGet, "/storages/"+uuid.NewString()+"/files/hls/dir/movie.mkv/index.m3u8?audio=2&access_token=tok", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/vnd.apple.mpegurl" {
		t.Fatalf("content type = %q", ct)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("playlists must not be cached")
	}
	if gotVariant != (service.HLSVariant{Audio: 2}) || gotQuery != "audio=2&access_token=tok" || w.Body.String() != "#EXTM3U\n" {
		t.Fatalf("variant=%+v query=%q body=%q", gotVariant, gotQuery, w.Body.String())
	}

	// The fallback the UI asks for when the browser cannot decode the copy:
	// transcode=1 selects the variant and stays in every segment URI.
	req = httptest.NewRequest(http.MethodGet, "/storages/"+uuid.NewString()+"/files/hls/dir/movie.mkv/index.m3u8?audio=2&transcode=1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || gotVariant != (service.HLSVariant{Audio: 2, TranscodeVideo: true}) || gotQuery != "audio=2&transcode=1" {
		t.Fatalf("transcode: status=%d variant=%+v query=%q", w.Code, gotVariant, gotQuery)
	}
}

func TestHLSServesSegmentsFromThePipeline(t *testing.T) {
	file := &domain.File{ID: uuid.New(), Path: "movie.mkv", Size: 10}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "seg00003.m4s"), []byte("segment-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "init.mp4"), []byte("init-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	var gotUser uuid.UUID
	var gotName string
	hls := &fakeHLS{available: true, resourceFn: func(_ context.Context, userID uuid.UUID, _ *domain.File, _ service.HLSVariant, name string) (string, error) {
		gotUser, gotName = userID, name
		return filepath.Join(dir, name), nil
	}}
	router, _ := mediaTestRouter(mediaFileService(file), hls)
	storage := uuid.NewString()

	req := httptest.NewRequest(http.MethodGet, "/storages/"+storage+"/files/hls/movie.mkv/seg00003.m4s", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "segment-bytes" {
		t.Fatalf("status = %d body %q", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Type") != "video/iso.segment" || gotUser != testMediaUser || gotName != "seg00003.m4s" {
		t.Fatalf("content type %q user %s name %q", w.Header().Get("Content-Type"), gotUser, gotName)
	}

	req = httptest.NewRequest(http.MethodGet, "/storages/"+storage+"/files/hls/movie.mkv/init.mp4", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "video/mp4" || w.Body.String() != "init-bytes" {
		t.Fatalf("init: status = %d type %q body %q", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
}

func TestHLSMapsPipelineErrors(t *testing.T) {
	file := &domain.File{ID: uuid.New(), Path: "movie.mkv", Size: 10}
	cases := map[string]struct {
		err  error
		want int
	}{
		"unavailable":    {err: service.ErrHLSUnavailable, want: http.StatusServiceUnavailable},
		"bad audio":      {err: media.ErrAudioTrackNotFound, want: http.StatusBadRequest},
		"no duration":    {err: service.ErrHLSUnknownDuration, want: http.StatusBadRequest},
		"not found":      {err: domain.ErrNotFound("segment out of range"), want: http.StatusNotFound},
		"ffmpeg crashed": {err: errors.New("ffmpeg ended before producing seg00001.m4s"), want: http.StatusInternalServerError},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			hls := &fakeHLS{available: true, resourceFn: func(context.Context, uuid.UUID, *domain.File, service.HLSVariant, string) (string, error) {
				return "", tc.err
			}}
			router, _ := mediaTestRouter(mediaFileService(file), hls)
			req := httptest.NewRequest(http.MethodGet, "/storages/"+uuid.NewString()+"/files/hls/movie.mkv/seg00001.m4s", nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (%s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}

func TestHLSRejectsBadRequests(t *testing.T) {
	file := &domain.File{ID: uuid.New(), Path: "movie.mkv", Size: 10}
	router, _ := mediaTestRouter(mediaFileService(file), &fakeHLS{available: true})
	storage := uuid.NewString()

	for _, target := range []string{
		"/storages/" + storage + "/files/hls/movie.mkv/index.m3u8?audio=two",
		"/storages/" + storage + "/files/hls/movie.mkv",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d", target, w.Code)
		}
	}
}

func TestHLSStopEndsTheUsersSession(t *testing.T) {
	file := &domain.File{ID: uuid.New(), Path: "movie.mkv", Size: 10}
	hls := &fakeHLS{available: true}
	router, _ := mediaTestRouter(mediaFileService(file), hls)

	req := httptest.NewRequest(http.MethodPost, "/storages/"+uuid.NewString()+"/files/hls_stop/movie.mkv", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
	if len(hls.stopped) != 1 || hls.stopped[0] != file.ID || hls.stoppedBy != testMediaUser {
		t.Fatalf("stop not forwarded: %+v", hls)
	}
}

func TestHLSSourceServesRangesToLoopbackOnly(t *testing.T) {
	file := &domain.File{ID: uuid.New(), Path: "movie.mkv", Size: 100}
	svc := mediaFileService(file)
	svc.exactFileSizeFn = func(context.Context, *domain.File) (int64, error) { return 100, nil }
	svc.downloadFileRangeToWriter = func(_ context.Context, _ *domain.File, w io.Writer, start, end, _ int64, _ *service.DownloadProgress) error {
		_, err := w.Write([]byte(strings.Repeat("x", int(end-start+1))))
		return err
	}
	hls := &fakeHLS{available: true, sourceToken: "tok123", sourceFile: file}
	router, _ := mediaTestRouter(svc, hls)

	req := httptest.NewRequest(http.MethodGet, "/internal/hls_source/tok123", nil)
	req.RemoteAddr = "127.0.0.1:34567"
	req.Header.Set("Range", "bytes=10-19")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusPartialContent || w.Body.String() != strings.Repeat("x", 10) {
		t.Fatalf("status = %d body %q", w.Code, w.Body.String())
	}
	if w.Header().Get("Content-Range") != "bytes 10-19/100" || w.Header().Get("Content-Type") != "video/x-matroska" {
		t.Fatalf("headers: %v", w.Header())
	}

	req = httptest.NewRequest(http.MethodGet, "/internal/hls_source/tok123", nil)
	req.RemoteAddr = "192.168.1.20:34567"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("non-loopback status = %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/internal/hls_source/other", nil)
	req.RemoteAddr = "[::1]:34567"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown token status = %d", w.Code)
	}
}

func TestSplitHLSPath(t *testing.T) {
	cases := map[string][2]string{
		"movie.mkv/index.m3u8":          {"movie.mkv", "index.m3u8"},
		"a/b/c.mkv/seg00001.m4s":        {"a/b/c.mkv", "seg00001.m4s"},
		"only.mkv":                      {"", "only.mkv"},
		"dir with space/x.mkv/init.mp4": {"dir with space/x.mkv", "init.mp4"},
	}
	for input, want := range cases {
		filePath, resource := splitHLSPath(input)
		if filePath != want[0] || resource != want[1] {
			t.Errorf("splitHLSPath(%q) = %q, %q; want %q, %q", input, filePath, resource, want[0], want[1])
		}
	}
}
