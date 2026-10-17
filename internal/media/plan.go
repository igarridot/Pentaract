package media

import (
	"errors"
	"strings"
)

// Codec names reported in Track.Codec. They are the ffmpeg names so the
// frontend and the HLS planner speak the same language.
const (
	CodecH264 = "h264"
	CodecHEVC = "hevc"
	CodecAAC  = "aac"
)

// codecByPrefix maps Matroska CodecID prefixes to codec names. Longer
// prefixes are listed first where one is a prefix of another.
var codecByPrefix = []struct {
	prefix string
	name   string
}{
	{"V_MPEG4/ISO/AVC", CodecH264},
	{"V_MPEGH/ISO/HEVC", CodecHEVC},
	{"V_MPEG4/ISO/ASP", "mpeg4"},
	{"V_MPEG4/ISO/SP", "mpeg4"},
	{"V_MPEG4/ISO/AP", "mpeg4"},
	{"V_MPEG4/MS/V3", "msmpeg4"},
	{"V_MPEG2", "mpeg2video"},
	{"V_MPEG1", "mpeg1video"},
	{"V_VP9", "vp9"},
	{"V_VP8", "vp8"},
	{"V_AV1", "av1"},
	{"V_THEORA", "theora"},
	{"V_PRORES", "prores"},
	{"V_MS/VFW/FOURCC", "vfw"},
	{"A_AAC", CodecAAC},
	{"A_EAC3", "eac3"},
	{"A_AC3", "ac3"},
	{"A_DTS", "dts"},
	{"A_TRUEHD", "truehd"},
	{"A_MLP", "mlp"},
	{"A_FLAC", "flac"},
	{"A_OPUS", "opus"},
	{"A_VORBIS", "vorbis"},
	{"A_MPEG/L3", "mp3"},
	{"A_MPEG/L2", "mp2"},
	{"A_MPEG/L1", "mp1"},
	{"A_PCM", "pcm"},
	{"A_ALAC", "alac"},
	{"A_MS/ACM", "acm"},
	{"S_TEXT/UTF8", "subrip"},
	{"S_TEXT/ASCII", "subrip"},
	{"S_TEXT/ASS", "ass"},
	{"S_TEXT/SSA", "ssa"},
	{"S_TEXT/WEBVTT", "webvtt"},
	{"S_HDMV/PGS", "pgs"},
	{"S_VOBSUB", "vobsub"},
	{"S_DVBSUB", "dvbsub"},
}

// CodecName maps a Matroska CodecID to a short codec name. Unknown IDs fall
// back to the ID itself in lower case.
func CodecName(codecID string) string {
	upper := strings.ToUpper(codecID)
	for _, c := range codecByPrefix {
		if strings.HasPrefix(upper, c.prefix) {
			return c.name
		}
	}
	return strings.ToLower(codecID)
}

// Plan says how the HLS pipeline turns a Matroska file into something a
// browser plays: which tracks to take and whether each one can be copied or
// has to be transcoded. Browsers decode H.264 and HEVC video and AAC audio
// in fragmented MP4; everything else is converted.
type Plan struct {
	Video      Track
	Audio      *Track
	VideoCopy  bool
	AudioCopy  bool
	VideoIndex int // position among the file's video tracks (ffmpeg's 0:v:N)
	AudioIndex int // position among the file's audio tracks (ffmpeg's 0:a:N)
}

// ErrNoVideoTrack reports a file without a video track.
var ErrNoVideoTrack = errors.New("no video track")

// ErrAudioTrackNotFound reports that the requested audio track number does
// not name an audio track of the file.
var ErrAudioTrackNotFound = errors.New("audio track not found")

// BuildPlan picks the first video track and the requested audio track
// (0 means the file's default audio track) and decides copy vs transcode.
func BuildPlan(info *Info, audioTrackNumber uint64) (Plan, error) {
	videos := info.VideoTracks()
	if len(videos) == 0 {
		return Plan{}, ErrNoVideoTrack
	}
	plan := Plan{Video: videos[0], VideoIndex: 0}
	plan.VideoCopy = plan.Video.Codec == CodecH264 || plan.Video.Codec == CodecHEVC

	audios := info.AudioTracks()
	if len(audios) == 0 {
		return plan, nil
	}
	index := -1
	if audioTrackNumber == 0 {
		index = DefaultAudioIndex(audios)
	} else {
		for i, t := range audios {
			if t.Number == audioTrackNumber {
				index = i
				break
			}
		}
	}
	if index < 0 {
		return Plan{}, ErrAudioTrackNotFound
	}
	audio := audios[index]
	plan.Audio = &audio
	plan.AudioIndex = index
	plan.AudioCopy = audio.Codec == CodecAAC
	return plan, nil
}

// DefaultAudioIndex returns the position of the first audio track flagged as
// default, or 0 when none is.
func DefaultAudioIndex(audios []Track) int {
	for i, t := range audios {
		if t.Default {
			return i
		}
	}
	return 0
}
