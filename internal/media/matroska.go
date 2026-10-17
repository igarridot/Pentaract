// Package media reads just enough of a media container to describe it: which
// codecs it carries and how long it lasts. Only Matroska (and its WebM
// subset) is understood, which is what the web UI needs to decide how to
// play an MKV without shipping the whole file through ffprobe.
package media

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
)

// Element IDs from the Matroska specification, with the EBML marker bits kept
// so they compare directly against what readID returns.
const (
	idEBML              = 0x1A45DFA3
	idDocType           = 0x4282
	idSegment           = 0x18538067
	idInfo              = 0x1549A966
	idTimecodeScale     = 0x2AD7B1
	idDuration          = 0x4489
	idTracks            = 0x1654AE6B
	idTrackEntry        = 0xAE
	idTrackNumber       = 0xD7
	idTrackType         = 0x83
	idCodecID           = 0x86
	idTrackName         = 0x536E
	idLanguage          = 0x22B59C
	idLanguageIETF      = 0x22B59D
	idFlagDefault       = 0x88
	idVideo             = 0xE0
	idPixelWidth        = 0xB0
	idPixelHeight       = 0xBA
	idAudio             = 0xE1
	idSamplingFrequency = 0xB5
	idChannels          = 0x9F
	idCluster           = 0x1F43B675
)

// Track type values of the Matroska TrackType element.
const (
	trackTypeVideo    = 1
	trackTypeAudio    = 2
	trackTypeSubtitle = 17
)

// Track type names as reported in Info.Tracks.
const (
	TrackVideo    = "video"
	TrackAudio    = "audio"
	TrackSubtitle = "subtitle"
	TrackOther    = "other"
)

const defaultTimecodeScale = 1_000_000 // nanoseconds per Matroska tick

// ErrNotMatroska reports that the data does not start with an EBML header.
var ErrNotMatroska = errors.New("not a Matroska/WebM file")

// ErrHeaderIncomplete reports that the Segment Info and Tracks elements were
// not found before the data ran out (or before the first Cluster).
var ErrHeaderIncomplete = errors.New("matroska header incomplete: info or tracks not found")

// Track describes one Matroska track.
type Track struct {
	Number     uint64  `json:"number"`
	Type       string  `json:"type"`
	CodecID    string  `json:"codec_id"`
	Codec      string  `json:"codec"`
	Name       string  `json:"name,omitempty"`
	Language   string  `json:"language,omitempty"`
	Default    bool    `json:"default"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	Channels   int     `json:"channels,omitempty"`
	SampleRate float64 `json:"sample_rate,omitempty"`
}

// Info is the parsed description of a Matroska file.
type Info struct {
	Container       string  `json:"container"`
	DurationSeconds float64 `json:"duration_seconds"`
	Tracks          []Track `json:"tracks"`
}

// VideoTracks returns the video tracks in file order.
func (i *Info) VideoTracks() []Track { return i.tracksOfType(TrackVideo) }

// AudioTracks returns the audio tracks in file order.
func (i *Info) AudioTracks() []Track { return i.tracksOfType(TrackAudio) }

func (i *Info) tracksOfType(kind string) []Track {
	var out []Track
	for _, t := range i.Tracks {
		if t.Type == kind {
			out = append(out, t)
		}
	}
	return out
}

// ParseMatroska reads the EBML header, Segment Info and Tracks from r. It
// stops at the first Cluster, so only the head of the file is consumed. The
// caller bounds r (io.LimitReader) to cap how much is read when the header is
// unexpectedly large.
func ParseMatroska(r io.Reader) (*Info, error) {
	br := bufio.NewReaderSize(r, 64*1024)

	id, size, _, err := readElementHeader(br)
	if err != nil || id != idEBML {
		return nil, ErrNotMatroska
	}
	docType, err := parseEBMLHeader(io.LimitReader(br, int64(size)))
	if err != nil {
		return nil, ErrNotMatroska
	}

	id, _, _, err = readElementHeader(br)
	if err != nil {
		return nil, wrapIncomplete(err)
	}
	if id != idSegment {
		return nil, fmt.Errorf("expected Segment element, got 0x%X", id)
	}

	info := &Info{Container: containerName(docType)}
	haveInfo, haveTracks := false, false
	for !(haveInfo && haveTracks) {
		id, size, unknown, err := readElementHeader(br)
		if err != nil {
			return nil, wrapIncomplete(err)
		}
		switch id {
		case idInfo:
			if err := parseInfo(io.LimitReader(br, int64(size)), info); err != nil {
				return nil, err
			}
			haveInfo = true
		case idTracks:
			if err := parseTracks(io.LimitReader(br, int64(size)), info); err != nil {
				return nil, err
			}
			haveTracks = true
		case idCluster:
			return nil, ErrHeaderIncomplete
		default:
			if unknown {
				return nil, fmt.Errorf("unexpected unknown-size element 0x%X before tracks", id)
			}
			if err := skip(br, size); err != nil {
				return nil, wrapIncomplete(err)
			}
		}
	}
	return info, nil
}

func wrapIncomplete(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return ErrHeaderIncomplete
	}
	return err
}

func containerName(docType string) string {
	switch strings.ToLower(docType) {
	case "webm":
		return "webm"
	default:
		return "matroska"
	}
}

func parseEBMLHeader(r io.Reader) (string, error) {
	docType := "matroska"
	err := forEachChild(r, func(id uint32, data io.Reader, size uint64) error {
		if id == idDocType {
			s, err := readString(data, size)
			if err != nil {
				return err
			}
			docType = s
			return nil
		}
		return skip(data, size)
	})
	return docType, err
}

func parseInfo(r io.Reader, info *Info) error {
	scale := uint64(defaultTimecodeScale)
	duration := 0.0
	err := forEachChild(r, func(id uint32, data io.Reader, size uint64) error {
		switch id {
		case idTimecodeScale:
			v, err := readUint(data, size)
			if err != nil {
				return err
			}
			scale = v
		case idDuration:
			v, err := readFloat(data, size)
			if err != nil {
				return err
			}
			duration = v
		default:
			return skip(data, size)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if duration > 0 && scale > 0 {
		info.DurationSeconds = duration * float64(scale) / 1e9
	}
	return nil
}

func parseTracks(r io.Reader, info *Info) error {
	return forEachChild(r, func(id uint32, data io.Reader, size uint64) error {
		if id != idTrackEntry {
			return skip(data, size)
		}
		track, err := parseTrackEntry(data)
		if err != nil {
			return err
		}
		info.Tracks = append(info.Tracks, track)
		return nil
	})
}

func parseTrackEntry(r io.Reader) (Track, error) {
	track := Track{Type: TrackOther, Default: true}
	languageIETF := ""
	err := forEachChild(r, func(id uint32, data io.Reader, size uint64) error {
		switch id {
		case idTrackNumber:
			v, err := readUint(data, size)
			track.Number = v
			return err
		case idTrackType:
			v, err := readUint(data, size)
			track.Type = trackTypeName(v)
			return err
		case idCodecID:
			s, err := readString(data, size)
			track.CodecID = s
			return err
		case idTrackName:
			s, err := readString(data, size)
			track.Name = s
			return err
		case idLanguage:
			s, err := readString(data, size)
			track.Language = s
			return err
		case idLanguageIETF:
			s, err := readString(data, size)
			languageIETF = s
			return err
		case idFlagDefault:
			v, err := readUint(data, size)
			track.Default = v != 0
			return err
		case idVideo:
			return forEachChild(data, func(id uint32, data io.Reader, size uint64) error {
				switch id {
				case idPixelWidth:
					v, err := readUint(data, size)
					track.Width = int(v)
					return err
				case idPixelHeight:
					v, err := readUint(data, size)
					track.Height = int(v)
					return err
				}
				return skip(data, size)
			})
		case idAudio:
			return forEachChild(data, func(id uint32, data io.Reader, size uint64) error {
				switch id {
				case idChannels:
					v, err := readUint(data, size)
					track.Channels = int(v)
					return err
				case idSamplingFrequency:
					v, err := readFloat(data, size)
					track.SampleRate = v
					return err
				}
				return skip(data, size)
			})
		}
		return skip(data, size)
	})
	if err != nil {
		return track, err
	}
	if languageIETF != "" {
		track.Language = languageIETF
	}
	if track.Language == "" {
		track.Language = "und"
	}
	track.Codec = CodecName(track.CodecID)
	return track, nil
}

func trackTypeName(v uint64) string {
	switch v {
	case trackTypeVideo:
		return TrackVideo
	case trackTypeAudio:
		return TrackAudio
	case trackTypeSubtitle:
		return TrackSubtitle
	}
	return TrackOther
}

// forEachChild walks the children of a master element whose payload is r
// (already bounded to the element size) and hands each child's payload to fn,
// which must consume exactly size bytes.
func forEachChild(r io.Reader, fn func(id uint32, data io.Reader, size uint64) error) error {
	br := bufio.NewReader(r)
	for {
		id, size, unknown, err := readElementHeader(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if unknown {
			return fmt.Errorf("unknown-size child element 0x%X", id)
		}
		if err := fn(id, io.LimitReader(br, int64(size)), size); err != nil {
			return err
		}
	}
}

// readElementHeader reads an element ID and its data size. unknown reports
// the all-ones "unknown size" marker that live-written Segments and Clusters
// use.
func readElementHeader(r *bufio.Reader) (id uint32, size uint64, unknown bool, err error) {
	id, err = readID(r)
	if err != nil {
		return 0, 0, false, err
	}
	size, unknown, err = readSize(r)
	if err != nil {
		return 0, 0, false, noEOF(err)
	}
	return id, size, unknown, nil
}

// noEOF turns an EOF in the middle of a header into ErrUnexpectedEOF so the
// caller cannot mistake it for a clean end of the parent element.
func noEOF(err error) error {
	if errors.Is(err, io.EOF) {
		return io.ErrUnexpectedEOF
	}
	return err
}

func readID(r *bufio.Reader) (uint32, error) {
	first, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	length := vintLength(first)
	if length == 0 || length > 4 {
		return 0, fmt.Errorf("invalid element id byte 0x%02X", first)
	}
	id := uint32(first)
	for i := 1; i < length; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, noEOF(err)
		}
		id = id<<8 | uint32(b)
	}
	return id, nil
}

func readSize(r *bufio.Reader) (uint64, bool, error) {
	first, err := r.ReadByte()
	if err != nil {
		return 0, false, err
	}
	length := vintLength(first)
	if length == 0 {
		return 0, false, fmt.Errorf("invalid size byte 0x%02X", first)
	}
	mask := byte(0xFF >> length)
	value := uint64(first & mask)
	allOnes := first&mask == mask
	for i := 1; i < length; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, false, noEOF(err)
		}
		if b != 0xFF {
			allOnes = false
		}
		value = value<<8 | uint64(b)
	}
	return value, allOnes, nil
}

// vintLength returns how many bytes a variable-length integer occupies given
// its first byte, or 0 when the byte is not a valid VINT start.
func vintLength(first byte) int {
	for i := range 8 {
		if first&(0x80>>i) != 0 {
			return i + 1
		}
	}
	return 0
}

func readUint(r io.Reader, size uint64) (uint64, error) {
	if size > 8 {
		return 0, fmt.Errorf("integer element too large: %d bytes", size)
	}
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, noEOF(err)
	}
	var v uint64
	for _, b := range buf {
		v = v<<8 | uint64(b)
	}
	return v, nil
}

func readFloat(r io.Reader, size uint64) (float64, error) {
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, noEOF(err)
	}
	switch size {
	case 0:
		return 0, nil
	case 4:
		return float64(math.Float32frombits(binary.BigEndian.Uint32(buf))), nil
	case 8:
		return math.Float64frombits(binary.BigEndian.Uint64(buf)), nil
	}
	return 0, fmt.Errorf("invalid float element size %d", size)
}

func readString(r io.Reader, size uint64) (string, error) {
	buf := make([]byte, size)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", noEOF(err)
	}
	return strings.TrimRight(string(buf), "\x00"), nil
}

func skip(r io.Reader, size uint64) error {
	if size == 0 {
		return nil
	}
	n, err := io.CopyN(io.Discard, r, int64(size))
	if err != nil {
		return noEOF(err)
	}
	if uint64(n) != size {
		return io.ErrUnexpectedEOF
	}
	return nil
}
