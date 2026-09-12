package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const (
	// maxMediaPixels bounds the decoded pixel count before image.Decode is
	// allowed to allocate (ADR-022).
	maxMediaPixels = 100_000_000
	// maxParserOutputBytes caps how many ffprobe/qpdf stdout and stderr
	// bytes are retained per pipe, so a hostile file cannot OOM the
	// API through unbounded parser diagnostics (ADR-022 bounded output).
	maxParserOutputBytes = 8 << 20
	// parserTimeout bounds each external parser invocation (ADR-022).
	parserTimeout = 10 * time.Second
)

// allowedAudioFormats is the ffprobe demuxer-name (format_name) allowlist
// for voice notes. Derivation: detectMIME admits exactly four audio
// container families — OggS, ID3/MPEG frame sync, an ftyp box, and an EBML
// header (upload_service.go) — and specs/004-real-time-chat/plan.md permits
// "OGG, MPEG, MP4, and WebM" while the spec stays codec-neutral (FR-020:
// no product-mandated voice codec). Each admitted family maps to one ffprobe
// demuxer name: "ogg", "mp3", the ISO-BMFF demuxer ("mov,mp4,m4a,3gp,3g2,
// mj2", which covers M4A), and the Matroska demuxer ("matroska,webm"). Any
// other format_name means the payload is not one of the sniffed container
// families and is rejected (ADR-022 allowlisted containers); codec choice
// inside a family stays neutral. The all-audio stream rule in
// allStreamsAudio rejects non-audio streams (video/subtitle).
var allowedAudioFormats = map[string]bool{
	"ogg":                     true,
	"mp3":                     true,
	"mov,mp4,m4a,3gp,3g2,mj2": true,
	"matroska,webm":           true,
}

// validateMediaPayload parses an upload before it reaches object storage.
func validateMediaPayload(ctx context.Context, kind MessageType, mime string, data []byte) (int, error) {
	if mediaMessageType(mime) != kind {
		return 0, ErrUnsupportedMIME
	}
	switch kind {
	case MessageTypeVoice:
		return probeAudioDuration(ctx, data)
	case MessageTypeImage:
		return validateImage(data)
	case MessageTypeFile:
		return 0, validatePDF(ctx, data)
	default:
		return 0, ErrUnsupportedMIME
	}
}

// validateImage decodes the full pixel data so a truncated or corrupt body
// behind a valid header is rejected (FR-022). DecodeConfig enforces the
// pixel ceiling first, so image.Decode can never allocate beyond it.
func validateImage(data []byte) (int, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width < 1 || config.Height < 1 || int64(config.Width)*int64(config.Height) > maxMediaPixels {
		return 0, ErrMalformedMedia
	}
	if _, _, err := image.Decode(bytes.NewReader(data)); err != nil {
		return 0, ErrMalformedMedia
	}
	return 0, nil
}

// probedStream is one ffprobe stream entry used by audio validation.
type probedStream struct {
	CodecType string `json:"codec_type"`
}

type probedPacket struct {
	PTS      string `json:"pts_time"`
	Duration string `json:"duration_time"`
}

// probeResult is the ffprobe JSON payload audio validation consumes.
type probeResult struct {
	Packets []probedPacket `json:"packets"`
	Streams []probedStream `json:"streams"`
	Format  struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
	} `json:"format"`
}

// probeAudioDuration parses an audio payload with ffprobe and returns the
// duration in whole seconds derived from packets and metadata, never from a
// client-declared value (FR-022). Malformed containers, non-allowlisted
// demuxers, non-audio streams, and unreadable durations all reject.
func probeAudioDuration(ctx context.Context, data []byte) (int, error) {
	path, cleanup, err := writeMediaTempFile(data)
	if err != nil {
		return 0, fmt.Errorf("prepare audio validation: %w", err)
	}
	defer cleanup()
	parserCtx, cancel := context.WithTimeout(ctx, parserTimeout)
	defer cancel()
	// Restrictive invocation per ADR-022: the private temporary file is the
	// sole input argument, and -protocol_whitelist file disables every
	// network and protocol handler so the probe can only open local files.
	// Stdin isolation comes from runBoundedCommand leaving cmd.Stdin nil,
	// which os/exec wires to the null device (ffprobe has no -nostdin
	// option; that flag exists only in ffmpeg).
	output, err := runBoundedCommand(parserCtx, "ffprobe",
		"-v", "error", "-protocol_whitelist", "file",
		"-show_packets", "-show_entries", "stream=codec_type:format=format_name,duration:packet=pts_time,duration_time",
		"-of", "json", path)
	if err != nil {
		return 0, ErrMalformedMedia
	}
	var result probeResult
	if json.Unmarshal(output, &result) != nil {
		return 0, ErrMalformedMedia
	}
	if !allStreamsAudio(result.Streams) || !allowedAudioFormats[result.Format.FormatName] {
		return 0, ErrMalformedMedia
	}
	return parsedAudioDuration(result)
}

// parsedAudioDuration uses the larger of container duration and the complete
// packet timeline, so understated container metadata cannot bypass the limit.
func parsedAudioDuration(result probeResult) (int, error) {
	duration, err := strconv.ParseFloat(result.Format.Duration, 64)
	if result.Format.Duration == "" || result.Format.Duration == "N/A" {
		duration = 0
		err = nil
	}
	if err != nil || duration < 0 || math.IsInf(duration, 0) || math.IsNaN(duration) {
		return 0, ErrMalformedMedia
	}
	if len(result.Packets) == 0 {
		return 0, ErrMalformedMedia
	}
	first, last := math.Inf(1), math.Inf(-1)
	for _, packet := range result.Packets {
		pts, ptsErr := strconv.ParseFloat(packet.PTS, 64)
		length, lengthErr := strconv.ParseFloat(packet.Duration, 64)
		if ptsErr != nil || lengthErr != nil || length < 0 || math.IsNaN(pts) || math.IsInf(pts, 0) || math.IsNaN(length) || math.IsInf(length, 0) {
			return 0, ErrMalformedMedia
		}
		first = math.Min(first, pts)
		last = math.Max(last, pts+length)
	}
	duration = math.Max(duration, last-first)
	if math.IsInf(duration, 0) || duration <= 0 {
		return 0, ErrMalformedMedia
	}
	if duration > MaxVoiceDurationSeconds {
		return MaxVoiceDurationSeconds + 1, nil
	}
	return int(math.Ceil(duration)), nil
}

// allStreamsAudio reports whether streams carries at least one audio stream
// and no video, subtitle, or other non-audio stream (ADR-022).
func allStreamsAudio(streams []probedStream) bool {
	if len(streams) == 0 {
		return false
	}
	for _, stream := range streams {
		if stream.CodecType != "audio" {
			return false
		}
	}
	return true
}

// validatePDF checks structural PDF validity with qpdf and rejects any
// malformed document before staging (ADR-022).
func validatePDF(ctx context.Context, data []byte) error {
	path, cleanup, err := writeMediaTempFile(data)
	if err != nil {
		return fmt.Errorf("prepare PDF validation: %w", err)
	}
	defer cleanup()
	parserCtx, cancel := context.WithTimeout(ctx, parserTimeout)
	defer cancel()
	if _, err := runBoundedCommand(parserCtx, "qpdf", "--check", path); err != nil {
		return ErrMalformedMedia
	}
	return nil
}

// parserOutputBuffer retains bounded output while accepting every byte, allowing
// os/exec to drain both pipes even after the retention budget is exhausted.
type parserOutputBuffer struct {
	buffer   bytes.Buffer
	exceeded bool
}

func (b *parserOutputBuffer) Len() int { return b.buffer.Len() }

func (b *parserOutputBuffer) Write(data []byte) (int, error) {
	count := len(data)
	remaining := maxParserOutputBytes - b.Len()
	if count > remaining {
		b.exceeded = true
		data = data[:remaining]
	}
	_, _ = b.buffer.Write(data)
	return count, nil
}

// runBoundedCommand drains both output pipes through bounded writers.
func runBoundedCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr parserOutputBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if stdout.exceeded || stderr.exceeded {
		return nil, fmt.Errorf("%s output exceeded limit", name)
	}
	if err != nil {
		return nil, fmt.Errorf("run %s: %w", name, err)
	}
	if stderr.Len() > 0 {
		return nil, fmt.Errorf("%s reported parser diagnostics", name)
	}
	return stdout.buffer.Bytes(), nil
}

// writeMediaTempFile writes one private temporary file and returns its path
// with a cleanup closure; parser diagnostics and paths are never logged.
func writeMediaTempFile(data []byte) (string, func(), error) {
	file, err := os.CreateTemp("", "halaqaty-chat-media-*")
	if err != nil {
		return "", nil, err
	}
	path := file.Name()
	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(path)
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", nil, err
	}
	return path, func() { _ = os.Remove(path) }, nil
}
