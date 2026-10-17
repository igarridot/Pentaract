package media

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"testing"
)

// --- synthetic EBML builder -------------------------------------------------

func idBytes(id uint32) []byte {
	var out []byte
	for shift := 24; shift >= 0; shift -= 8 {
		b := byte(id >> shift)
		if len(out) == 0 && b == 0 {
			continue
		}
		out = append(out, b)
	}
	return out
}

func sizeBytes(size int) []byte {
	if size < 127 {
		return []byte{0x80 | byte(size)}
	}
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(size))
	buf[0] = 0x01
	return buf
}

func el(id uint32, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	return bytes.Join([][]byte{idBytes(id), sizeBytes(len(body)), body}, nil)
}

func elUnknownSize(id uint32, payload ...[]byte) []byte {
	body := bytes.Join(payload, nil)
	return bytes.Join([][]byte{idBytes(id), {0x01, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, body}, nil)
}

func uintEl(id uint32, v uint64) []byte {
	var body []byte
	for shift := 56; shift >= 0; shift -= 8 {
		b := byte(v >> shift)
		if len(body) == 0 && b == 0 && shift > 0 {
			continue
		}
		body = append(body, b)
	}
	return el(id, body)
}

func floatEl(id uint32, v float64) []byte {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, math.Float64bits(v))
	return el(id, buf)
}

func stringEl(id uint32, s string) []byte { return el(id, []byte(s)) }

func ebmlHeader(docType string) []byte {
	return el(idEBML, stringEl(idDocType, docType))
}

func videoTrack(number uint64, codec string, w, h int) []byte {
	return el(idTrackEntry,
		uintEl(idTrackNumber, number),
		uintEl(idTrackType, trackTypeVideo),
		stringEl(idCodecID, codec),
		el(idVideo, uintEl(idPixelWidth, uint64(w)), uintEl(idPixelHeight, uint64(h))),
	)
}

func audioTrack(number uint64, codec, lang string, channels int, def bool) []byte {
	flag := uint64(0)
	if def {
		flag = 1
	}
	return el(idTrackEntry,
		uintEl(idTrackNumber, number),
		uintEl(idTrackType, trackTypeAudio),
		stringEl(idCodecID, codec),
		stringEl(idLanguage, lang),
		uintEl(idFlagDefault, flag),
		el(idAudio, uintEl(idChannels, uint64(channels)), floatEl(idSamplingFrequency, 48000)),
	)
}

func segmentInfo(durationTicks float64, scale uint64) []byte {
	parts := [][]byte{floatEl(idDuration, durationTicks)}
	if scale != 0 {
		parts = append(parts, uintEl(idTimecodeScale, scale))
	}
	return el(idInfo, parts...)
}

func cluster() []byte { return el(idCluster, uintEl(0xE7, 0)) }

// --- tests ------------------------------------------------------------------

func TestParseMatroskaSyntheticFile(t *testing.T) {
	file := bytes.Join([][]byte{
		ebmlHeader("matroska"),
		el(idSegment,
			el(0x114D9B74, []byte{1, 2, 3}), // SeekHead, skipped
			segmentInfo(5400000, 1000000),   // 5400 s
			el(idTracks,
				videoTrack(1, "V_MPEGH/ISO/HEVC", 3840, 2160),
				audioTrack(2, "A_EAC3", "eng", 6, false),
				audioTrack(3, "A_AAC/MPEG4/LC", "spa", 2, true),
				el(idTrackEntry, uintEl(idTrackNumber, 4), uintEl(idTrackType, trackTypeSubtitle), stringEl(idCodecID, "S_HDMV/PGS")),
			),
			cluster(),
		),
	}, nil)

	info, err := ParseMatroska(bytes.NewReader(file))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.Container != "matroska" {
		t.Fatalf("container = %q", info.Container)
	}
	if info.DurationSeconds != 5400 {
		t.Fatalf("duration = %v, want 5400", info.DurationSeconds)
	}
	if len(info.Tracks) != 4 {
		t.Fatalf("tracks = %d, want 4", len(info.Tracks))
	}
	video := info.Tracks[0]
	if video.Type != TrackVideo || video.Codec != CodecHEVC || video.Width != 3840 || video.Height != 2160 || !video.Default {
		t.Fatalf("unexpected video track %+v", video)
	}
	eac3 := info.Tracks[1]
	if eac3.Type != TrackAudio || eac3.Codec != "eac3" || eac3.Language != "eng" || eac3.Channels != 6 || eac3.SampleRate != 48000 || eac3.Default {
		t.Fatalf("unexpected eac3 track %+v", eac3)
	}
	aac := info.Tracks[2]
	if aac.Codec != CodecAAC || !aac.Default || aac.Language != "spa" {
		t.Fatalf("unexpected aac track %+v", aac)
	}
	if sub := info.Tracks[3]; sub.Type != TrackSubtitle || sub.Codec != "pgs" || sub.Language != "und" {
		t.Fatalf("unexpected subtitle track %+v", sub)
	}
}

func TestParseMatroskaUnknownSegmentSizeAndDefaultScale(t *testing.T) {
	file := bytes.Join([][]byte{
		ebmlHeader("webm"),
		elUnknownSize(idSegment,
			el(idTracks, videoTrack(1, "V_VP9", 1280, 720), audioTrack(2, "A_OPUS", "und", 2, true)),
			segmentInfo(90000, 0), // Info after Tracks, default scale
			cluster(),
		),
	}, nil)

	info, err := ParseMatroska(bytes.NewReader(file))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.Container != "webm" || info.DurationSeconds != 90 {
		t.Fatalf("unexpected info %+v", info)
	}
	if info.Tracks[0].Codec != "vp9" || info.Tracks[1].Codec != "opus" {
		t.Fatalf("unexpected codecs %+v", info.Tracks)
	}
}

func TestParseMatroskaStopsReadingOnceHeaderIsComplete(t *testing.T) {
	head := bytes.Join([][]byte{
		ebmlHeader("matroska"),
		elUnknownSize(idSegment, segmentInfo(1000, 1000000), el(idTracks, videoTrack(1, "V_MPEG4/ISO/AVC", 1920, 1080))),
	}, nil)
	// Whatever follows must never be touched: a reader that fails on read.
	r := io.MultiReader(bytes.NewReader(head), failingReader{})

	info, err := ParseMatroska(r)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.Tracks[0].Codec != CodecH264 {
		t.Fatalf("unexpected track %+v", info.Tracks[0])
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("read past header") }

func TestParseMatroskaErrors(t *testing.T) {
	cases := map[string]struct {
		data []byte
		want error
	}{
		"not ebml":      {data: []byte("\x00\x00\x00\x1cftypisom"), want: ErrNotMatroska},
		"empty":         {data: nil, want: ErrNotMatroska},
		"cluster first": {data: bytes.Join([][]byte{ebmlHeader("matroska"), elUnknownSize(idSegment, cluster())}, nil), want: ErrHeaderIncomplete},
		"truncated":     {data: bytes.Join([][]byte{ebmlHeader("matroska"), elUnknownSize(idSegment, segmentInfo(1, 0))}, nil), want: ErrHeaderIncomplete},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseMatroska(bytes.NewReader(tc.data))
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestParseMatroskaRealFixture(t *testing.T) {
	f, err := os.Open("testdata/sample.mkv")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer f.Close()

	info, err := ParseMatroska(io.LimitReader(f, 64*1024))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.Container != "matroska" {
		t.Fatalf("container = %q", info.Container)
	}
	if info.DurationSeconds < 0.3 || info.DurationSeconds > 0.4 {
		t.Fatalf("duration = %v, want about 0.32", info.DurationSeconds)
	}
	videos, audios := info.VideoTracks(), info.AudioTracks()
	if len(videos) != 1 || len(audios) != 2 {
		t.Fatalf("tracks: %d video, %d audio: %+v", len(videos), len(audios), info.Tracks)
	}
	if videos[0].Codec != CodecH264 || videos[0].Width != 32 || videos[0].Height != 16 {
		t.Fatalf("unexpected video %+v", videos[0])
	}
	if audios[0].Codec != "ac3" || audios[0].Language != "spa" || audios[0].Name != "Castellano" || audios[0].Channels != 2 {
		t.Fatalf("unexpected first audio %+v", audios[0])
	}
	if audios[1].Codec != CodecAAC || audios[1].Language != "eng" || audios[1].Channels != 1 {
		t.Fatalf("unexpected second audio %+v", audios[1])
	}
}

func TestCodecName(t *testing.T) {
	cases := map[string]string{
		"V_MPEG4/ISO/AVC":  "h264",
		"V_MPEGH/ISO/HEVC": "hevc",
		"V_MPEG4/ISO/ASP":  "mpeg4",
		"A_AAC":            "aac",
		"A_AAC/MPEG4/LC":   "aac",
		"A_AC3":            "ac3",
		"A_EAC3":           "eac3",
		"A_DTS/LOSSLESS":   "dts",
		"A_TRUEHD":         "truehd",
		"S_TEXT/UTF8":      "subrip",
		"X_WEIRD":          "x_weird",
	}
	for id, want := range cases {
		if got := CodecName(id); got != want {
			t.Errorf("CodecName(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestBuildPlan(t *testing.T) {
	info := &Info{Tracks: []Track{
		{Number: 1, Type: TrackVideo, Codec: CodecH264},
		{Number: 2, Type: TrackAudio, Codec: "ac3"},
		{Number: 3, Type: TrackAudio, Codec: CodecAAC, Default: true},
		{Number: 4, Type: TrackSubtitle, Codec: "pgs"},
	}}

	plan, err := BuildPlan(info, 0)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if !plan.VideoCopy || plan.VideoIndex != 0 {
		t.Fatalf("h264 must be copied: %+v", plan)
	}
	if plan.Audio == nil || plan.Audio.Number != 3 || plan.AudioIndex != 1 || !plan.AudioCopy {
		t.Fatalf("default audio must be the aac track, copied: %+v", plan)
	}

	plan, err = BuildPlan(info, 2)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.Audio.Number != 2 || plan.AudioIndex != 0 || plan.AudioCopy {
		t.Fatalf("ac3 must be transcoded: %+v", plan)
	}

	if _, err := BuildPlan(info, 4); !errors.Is(err, ErrAudioTrackNotFound) {
		t.Fatalf("subtitle track as audio: err = %v", err)
	}

	vp9 := &Info{Tracks: []Track{{Number: 1, Type: TrackVideo, Codec: "vp9"}}}
	plan, err = BuildPlan(vp9, 0)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if plan.VideoCopy || plan.Audio != nil {
		t.Fatalf("vp9 without audio: %+v", plan)
	}

	if _, err := BuildPlan(&Info{Tracks: []Track{{Number: 1, Type: TrackAudio, Codec: "mp3"}}}, 0); !errors.Is(err, ErrNoVideoTrack) {
		t.Fatalf("audio-only: err = %v", err)
	}
}
