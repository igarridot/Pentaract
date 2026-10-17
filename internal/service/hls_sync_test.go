package service

import (
	"bytes"
	"encoding/binary"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/Dominux/Pentaract/internal/media"
)

// TestHLSAudioStaysInSyncInTheBrowser checks A/V sync the way Chrome plays
// the HLS output: video frames at their timestamps, audio frames decoded back
// to back from the first audio timestamp (MSE ignores edit lists and audio
// timestamp gaps). The source is a Matroska file whose audio starts 1.03 s
// after the video, like the Blu-ray remuxes that drifted: black video with a
// white flash and silent audio with a beep, both every 5 s. Whatever the
// session (from the start or after a seek, video copied or re-encoded), the
// beep must stay where the source puts it relative to its flash.
func TestHLSAudioStaysInSyncInTheBrowser(t *testing.T) {
	for _, bin := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not installed", bin)
		}
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "src.mkv")
	plain := filepath.Join(dir, "plain.mkv")
	runTool(t, "ffmpeg", "-v", "error", "-y",
		"-f", "lavfi", "-i", "color=black:size=320x180:rate=24000/1001,drawbox=c=white:t=fill:enable='lt(mod(t,5),0.04)*gt(t,1)'",
		"-f", "lavfi", "-i", "aevalsrc='if(lt(mod(t,5),0.1)*gt(t,1),0.8*sin(2*PI*1000*t),0)':s=48000",
		"-t", "30", "-map", "0:v", "-map", "1:a",
		"-c:v", "libx264", "-preset", "ultrafast", "-bf", "3", "-g", "24", "-c:a", "ac3", plain)
	runTool(t, "ffmpeg", "-v", "error", "-y", "-i", plain, "-itsoffset", "1.03", "-i", plain,
		"-map", "0:v", "-map", "1:a", "-c", "copy", src)

	// A player honouring timestamps hears each beep this long after its flash.
	srcFlash := flashTime(t, src, false)
	want := beepTime(t, src, false, srcFlash) - srcFlash

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, src)
	}))
	defer source.Close()

	copyPlan := media.Plan{
		Video: media.Track{Codec: media.CodecH264}, VideoCopy: true,
		Audio: &media.Track{Codec: "ac3"},
	}
	cases := []struct {
		name         string
		plan         media.Plan
		startSeconds float64
		startIndex   int
	}{
		{"copy from the start", copyPlan, 0, 0},
		{"copy after a seek", copyPlan, 12, 2},
		{"transcode from the start", copyPlan.WithVideoTranscode(), 0, 0},
		{"transcode after a seek", copyPlan.WithVideoTranscode(), 12, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := t.TempDir()
			cmd := exec.Command("ffmpeg", hlsFFmpegArgs(HLSConfig{SegmentSeconds: 6}, source.URL, tc.plan, tc.startSeconds, tc.startIndex)...)
			cmd.Dir = out
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("ffmpeg: %v\n%s", err, output)
			}
			joined := joinHLSOutput(t, out)
			flash := flashTime(t, joined, true)
			got := beepTime(t, joined, true, flash) - flash
			if drift := got - want; math.Abs(drift) > 0.04 {
				t.Fatalf("audio is %+.0f ms off the video (beep-flash %.3f s, source %.3f s)", drift*1000, got, want)
			}
		})
	}
}

func runTool(t *testing.T, name string, args ...string) []byte {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, stderr.String())
	}
	return stdout.Bytes()
}

// joinHLSOutput concatenates init.mp4 and the segments into one MP4.
func joinHLSOutput(t *testing.T, dir string) string {
	t.Helper()
	segments, _ := filepath.Glob(filepath.Join(dir, "seg*.m4s"))
	sort.Strings(segments)
	if len(segments) == 0 {
		t.Fatalf("ffmpeg produced no segments")
	}
	var joined []byte
	for _, name := range append([]string{filepath.Join(dir, hlsInitSegmentName)}, segments...) {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		joined = append(joined, data...)
	}
	path := filepath.Join(dir, "joined.mp4")
	if err := os.WriteFile(path, joined, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func editListArgs(ignore bool) []string {
	if ignore {
		return []string{"-ignore_editlist", "1"}
	}
	return nil
}

// flashTime is the timestamp of the first white video frame.
func flashTime(t *testing.T, file string, ignoreEditList bool) float64 {
	t.Helper()
	args := append(append([]string{"-v", "error"}, editListArgs(ignoreEditList)...),
		"-i", file, "-map", "0:v", "-vf", "signalstats,metadata=print:key=lavfi.signalstats.YAVG:file=-", "-f", "null", "-")
	var pts float64
	for _, line := range strings.Split(string(runTool(t, "ffmpeg", args...)), "\n") {
		if _, after, ok := strings.Cut(line, "pts_time:"); ok {
			pts, _ = strconv.ParseFloat(strings.TrimSpace(after), 64)
		}
		if _, after, ok := strings.Cut(line, "YAVG="); ok {
			if luma, _ := strconv.ParseFloat(strings.TrimSpace(after), 64); luma > 100 {
				return pts
			}
		}
	}
	t.Fatalf("no flash found in %s", file)
	return 0
}

// beepTime is when the first beep after notBefore is heard if the audio
// frames are played back to back from the first audio timestamp, as Chrome's
// MSE does. After a seek the audio can hold the beep of a marker whose flash
// came before the first video frame, hence notBefore.
func beepTime(t *testing.T, file string, ignoreEditList bool, notBefore float64) float64 {
	t.Helper()
	probe := append(append([]string{"-v", "error"}, editListArgs(ignoreEditList)...),
		"-select_streams", "a", "-show_entries", "packet=pts_time", "-of", "csv=p=0", file)
	first := math.Inf(1)
	for _, line := range strings.Fields(string(runTool(t, "ffprobe", probe...))) {
		if pts, err := strconv.ParseFloat(strings.TrimRight(line, ","), 64); err == nil && pts < first {
			first = pts
		}
	}
	decode := append(append([]string{"-v", "error"}, editListArgs(ignoreEditList)...),
		"-i", file, "-map", "0:a", "-ac", "1", "-ar", "48000", "-f", "s16le", "-acodec", "pcm_s16le", "-")
	pcm := runTool(t, "ffmpeg", decode...)
	for i := 0; i+1 < len(pcm); i += 2 {
		at := first + float64(i/2)/48000
		if sample := int16(binary.LittleEndian.Uint16(pcm[i:])); at >= notBefore && (sample > 8000 || sample < -8000) {
			return at
		}
	}
	t.Fatalf("no beep found in %s", file)
	return 0
}
