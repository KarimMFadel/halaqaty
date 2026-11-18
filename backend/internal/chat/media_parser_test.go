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
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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
			if !errors.Is(err, ErrMalformedMedia) {
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
			if _, err := validateImage(truncated); !errors.Is(err, ErrMalformedMedia) {
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
	if _, err := validateImage(header); !errors.Is(err, ErrMalformedMedia) {
		t.Fatal("image exceeding the pixel ceiling accepted")
	}
}

// stubDecodeFullImage replaces the full-pixel decode seam for the duration of
// one test so decode counts and concurrency stay observable and cheap.
func stubDecodeFullImage(t *testing.T, fake func(io.Reader) (image.Image, string, error)) {
	t.Helper()
	original := decodeFullImage
	decodeFullImage = fake
	t.Cleanup(func() { decodeFullImage = original })
}

// requireCraftedHeaderParses proves the header-only fixture actually parses at
// the DecodeConfig stage with the intended dimensions; without this guard a
// malformed fixture would make the ordering tests pass vacuously.
func requireCraftedHeaderParses(t *testing.T, header []byte, width, height uint32) {
	t.Helper()
	config, _, err := image.DecodeConfig(bytes.NewReader(header))
	if err != nil || config.Width != int(width) || config.Height != int(height) {
		t.Fatalf("crafted header does not parse to %dx%d: got %dx%d err=%v", width, height, config.Width, config.Height, err)
	}
}

// TestValidateImageRejectsOverCeilingDimensionsWithoutFullDecode proves an
// image whose header dimensions exceed the pixel ceiling is rejected at the
// DecodeConfig stage without ever running the full-pixel decode, so a
// hostile header can never trigger a decoded allocation (ADR-022).
func TestValidateImageRejectsOverCeilingDimensionsWithoutFullDecode(t *testing.T) {
	decodes := 0
	stubDecodeFullImage(t, func(io.Reader) (image.Image, string, error) {
		decodes++
		return nil, "", errors.New("full decode must not run for an over-ceiling image")
	})
	// 4001×5001 = 20,005,001 px: one pixel row over the ceiling.
	header := craftedPNGHeader(4001, 5001)
	requireCraftedHeaderParses(t, header, 4001, 5001)
	if _, err := validateImage(header); !errors.Is(err, ErrMalformedMedia) {
		t.Fatalf("validateImage() error = %v, want ErrMalformedMedia", err)
	}
	if decodes != 0 {
		t.Fatalf("full decode ran %d times for an over-ceiling image", decodes)
	}
}

// TestValidateImageDecodesAtCeilingDimensions pins the ceiling boundary:
// exactly-ceiling dimensions still reach the full decode, and a single pixel
// row beyond it is rejected by the header check alone.
func TestValidateImageDecodesAtCeilingDimensions(t *testing.T) {
	for _, tc := range []struct {
		name        string
		width       uint32
		height      uint32
		wantErr     error
		wantDecodes int
	}{
		{name: "exactly at ceiling decodes", width: 4000, height: 5000, wantDecodes: 1},
		{name: "one pixel over ceiling rejects", width: 4000, height: 5001, wantErr: ErrMalformedMedia, wantDecodes: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			header := craftedPNGHeader(tc.width, tc.height)
			requireCraftedHeaderParses(t, header, tc.width, tc.height)
			decodes := 0
			stubDecodeFullImage(t, func(io.Reader) (image.Image, string, error) {
				decodes++
				return nil, "png", nil
			})
			_, err := validateImage(header)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("validateImage() error = %v, want %v", err, tc.wantErr)
			}
			if decodes != tc.wantDecodes {
				t.Fatalf("full decode count = %d, want %d", decodes, tc.wantDecodes)
			}
		})
	}
}

// TestValidateImageBoundsConcurrentDecodes proves the package-level decode
// semaphore caps simultaneous full decodes even when many uploads race: with
// 3×limit concurrent validate calls, no more than limit may be inside the
// decode at once, every waiter eventually decodes after slots free, and no
// slot is leaked.
func TestValidateImageBoundsConcurrentDecodes(t *testing.T) {
	const limit = maxConcurrentImageDecodes
	var inFlight, peak int32
	release := make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	defer free()
	stubDecodeFullImage(t, func(io.Reader) (image.Image, string, error) {
		current := atomic.AddInt32(&inFlight, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if current <= p || atomic.CompareAndSwapInt32(&peak, p, current) {
				break
			}
		}
		defer atomic.AddInt32(&inFlight, -1)
		<-release
		return nil, "png", nil
	})

	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	var payload bytes.Buffer
	if err := png.Encode(&payload, img); err != nil {
		t.Fatal(err)
	}

	const callers = 3 * limit
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := validateImage(payload.Bytes())
			errs <- err
		}()
	}

	// Wait until exactly limit decoders are inside and the peak has stayed at
	// limit for a settle window: with the semaphore this is reached and held;
	// without it the unbounded ramp pushes the peak past limit and the loop
	// times out instead.
	const settleChecks = 10
	stable := 0
	for deadline := time.Now().Add(2 * time.Second); ; {
		if time.Now().After(deadline) {
			t.Fatalf("peak concurrent decodes never settled at %d (peak=%d, inFlight=%d)", limit, atomic.LoadInt32(&peak), atomic.LoadInt32(&inFlight))
		}
		if atomic.LoadInt32(&peak) == limit {
			stable++
		} else {
			stable = 0
		}
		if stable >= settleChecks {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := atomic.LoadInt32(&peak); got != limit {
		t.Fatalf("peak concurrent decodes = %d, want exactly %d", got, limit)
	}

	free()
	wg.Wait()
	for i := 0; i < callers; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("blocked waiter failed after slots freed: %v", err)
		}
	}
	if got := len(decodeSlots); got != 0 {
		t.Fatalf("decode slots leaked: %d held after all decodes returned", got)
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
	if _, err := probeAudioDuration(context.Background(), readTestdata(t, "video_with_audio.mp4")); !errors.Is(err, ErrMalformedMedia) {
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
