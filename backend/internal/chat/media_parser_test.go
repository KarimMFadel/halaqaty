package chat

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// readTestdata loads one committed binary fixture (see testdata/).
func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return data
}

// requireParser skips a parser-dependent test when the ADR-022 tool is not
// installed on this machine; the production runtime image installs both.
func requireParser(t *testing.T, tool string) {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		t.Skipf("%s not installed; parser-dependent check skipped", tool)
	}
}

// TestValidateMediaPayload_RejectsTruncatedContainers pins the fail-closed
// rule: a magic header without a parseable body never reaches storage.
func TestValidateMediaPayload_RejectsTruncatedContainers(t *testing.T) {
	for _, tc := range []struct {
		name string
		kind MessageType
		mime string
		data []byte
	}{
		{"audio", MessageTypeVoice, "audio/ogg", []byte("OggS")},
		{"jpeg", MessageTypeImage, "image/jpeg", []byte{0xff, 0xd8, 0xff, 0xe0}},
		{"pdf", MessageTypeFile, "application/pdf", []byte("%PDF-1.7\n")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := validateMediaPayload(context.Background(), tc.kind, tc.mime, tc.data)
			if !errors.Is(err, ErrUnsupportedMIME) {
				t.Fatalf("validateMediaPayload() error = %v, want unsupported media", err)
			}
		})
	}
}

// TestValidateImageAcceptsDecodableImages proves genuinely decodable JPEG
// and PNG payloads pass the full-pixel-decode validation (ADR-022).
func TestValidateImageAcceptsDecodableImages(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 7, 5))
	var pngBuf, jpegBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegBuf, img, nil); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"png": pngBuf.Bytes(), "jpeg": jpegBuf.Bytes()} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateImage(data); err != nil {
				t.Fatalf("valid %s rejected: %v", name, err)
			}
		})
	}
}

// TestValidateImageRejectsTruncatedPixelData proves a truncated body behind
// a valid header is rejected: header-only decoding is not acceptance.
func TestValidateImageRejectsTruncatedPixelData(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 7, 5))
	var pngBuf, jpegBuf bytes.Buffer
	if err := png.Encode(&pngBuf, img); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegBuf, img, nil); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"png": pngBuf.Bytes(), "jpeg": jpegBuf.Bytes()} {
		t.Run(name, func(t *testing.T) {
			truncated := data[:len(data)/2]
			if _, err := validateImage(truncated); !errors.Is(err, ErrUnsupportedMIME) {
				t.Fatalf("truncated %s accepted (len %d of %d)", name, len(truncated), len(data))
			}
		})
	}
}

// craftedPNGHeader builds a PNG carrying only a signature and a valid-CRC
// IHDR with the given dimensions.
func craftedPNGHeader(width, height uint32) []byte {
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8], ihdr[9], ihdr[10], ihdr[11], ihdr[12] = 8, 2, 0, 0, 0
	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})
	var chunkLen [4]byte
	binary.BigEndian.PutUint32(chunkLen[:], uint32(len(ihdr)))
	out.Write(chunkLen[:])
	chunk := append([]byte("IHDR"), ihdr...)
	out.Write(chunk)
	binary.BigEndian.PutUint32(chunkLen[:], crc32.ChecksumIEEE(chunk))
	out.Write(chunkLen[:])
	return out.Bytes()
}

// TestValidateImageRejectsOversizedDimensionsBeforeDecode proves the pixel
// ceiling rejects via the header alone: completing this test at all shows
// no oversized pixel buffer was ever allocated for a full decode.
func TestValidateImageRejectsOversizedDimensionsBeforeDecode(t *testing.T) {
	header := craftedPNGHeader(100_001, 100_001)
	if _, err := validateImage(header); !errors.Is(err, ErrUnsupportedMIME) {
		t.Fatal("image exceeding the pixel ceiling accepted")
	}
}

// TestProbeAudioDurationParsesRealContainer pins that the served duration is
// parsed from real container metadata, never client-declared. Fixture:
// testdata/voice_2s.ogg — a mono 440 Hz FLAC-in-OGG tone generated with
// `ffmpeg -f lavfi -i sine=frequency=440:duration=2 -ac 1 -c:a flac`;
// ffprobe reports duration 2.000000 (verified with ffprobe 5.1 and 6.1), so
// the validated duration is 2 seconds.
func TestProbeAudioDurationParsesRealContainer(t *testing.T) {
	requireParser(t, "ffprobe")
	seconds, err := probeAudioDuration(context.Background(), readTestdata(t, "voice_2s.ogg"))
	if err != nil {
		t.Fatalf("valid audio container rejected: %v", err)
	}
	if seconds != 2 {
		t.Fatalf("parsed duration = %d, want 2", seconds)
	}
}

// TestProbeAudioDurationRejectsVideoContainer proves a container carrying
// any non-audio stream is rejected even when it also has audio. Fixture:
// testdata/video_with_audio.mp4 — one H.264 video and one AAC audio stream
// generated with `ffmpeg -f lavfi -i testsrc=size=64x64:rate=10:duration=1
// -f lavfi -i sine=frequency=440:duration=1 -c:v libx264 -c:a aac`.
func TestProbeAudioDurationRejectsVideoContainer(t *testing.T) {
	requireParser(t, "ffprobe")
	if _, err := probeAudioDuration(context.Background(), readTestdata(t, "video_with_audio.mp4")); !errors.Is(err, ErrUnsupportedMIME) {
		t.Fatalf("video container accepted as voice: %v", err)
	}
}

// TestValidatePDFAcceptsWellFormedDocument proves a structurally valid PDF
// passes qpdf --check. Fixture: testdata/valid.pdf — a minimal one-page
// document with a correct xref table; `qpdf --check` reports no syntax or
// stream encoding errors (verified with qpdf 11.3).
func TestValidatePDFAcceptsWellFormedDocument(t *testing.T) {
	requireParser(t, "qpdf")
	if err := validatePDF(context.Background(), readTestdata(t, "valid.pdf")); err != nil {
		t.Fatalf("well-formed PDF rejected: %v", err)
	}
}

// TestAllowedAudioFormatsMatchesSniffedContainers pins the demuxer
// allowlist: exactly one ffprobe format_name per container family
// detectMIME admits (OGG, MPEG, MP4/M4A, WebM — see the derivation on
// allowedAudioFormats).
func TestAllowedAudioFormatsMatchesSniffedContainers(t *testing.T) {
	want := []string{"ogg", "mp3", "mov,mp4,m4a,3gp,3g2,mj2", "matroska,webm"}
	if len(allowedAudioFormats) != len(want) {
		t.Fatalf("allowlist has %d entries, want %d", len(allowedAudioFormats), len(want))
	}
	for _, name := range want {
		if !allowedAudioFormats[name] {
			t.Fatalf("container family %q missing from allowlist", name)
		}
	}
}
