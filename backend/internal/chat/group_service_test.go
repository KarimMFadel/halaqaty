package chat

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestGroupService_SendTextValidatesBeforePersistence proves input validation
// (reused from validation.go) precedes every persistence and authorization
// dependency: the service is constructed without a repository or membership
// reader, so any ordering violation would panic instead of returning the
// stable models.go error.
func TestGroupService_SendTextValidatesBeforePersistence(t *testing.T) {
	svc := NewGroupService(nil, nil, nil, nil)
	circle := uuid.New()
	sender := uuid.New()

	tests := []struct {
		name    string
		content string
		key     string
		want    error
	}{
		{name: "rejects empty text", content: "   ", key: "key-1", want: ErrInvalidText},
		{name: "rejects overlong text", content: strings.Repeat("م", MaxTextRunes+1), key: "key-2", want: ErrInvalidText},
		{name: "rejects missing idempotency key", content: "hello", key: "", want: ErrInvalidIdempotencyKey},
		{name: "rejects oversized idempotency key", content: "hello", key: strings.Repeat("k", MaxIdempotencyKeyLength+1), want: ErrInvalidIdempotencyKey},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.SendText(context.Background(), sender, circle, tt.content, tt.key)
			if !errors.Is(err, tt.want) {
				t.Fatalf("SendText() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestGroupService_ClampHistoryLimit pins the page-size contract: unset or
// non-positive limits fall back to the default and no limit exceeds the
// documented maximum.
func TestGroupService_ClampHistoryLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "zero falls back to default", limit: 0, want: DefaultHistoryPageSize},
		{name: "negative falls back to default", limit: -5, want: DefaultHistoryPageSize},
		{name: "one stays one", limit: 1, want: 1},
		{name: "maximum stays maximum", limit: MaxHistoryPageSize, want: MaxHistoryPageSize},
		{name: "oversized clamps to maximum", limit: MaxHistoryPageSize + 1, want: MaxHistoryPageSize},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampHistoryLimit(tt.limit); got != tt.want {
				t.Fatalf("clampHistoryLimit(%d) = %d, want %d", tt.limit, got, tt.want)
			}
		})
	}
}

// TestGroupService_PackageStaysSessionIndependent is a supplemental
// source-policy guard for the US1-AC4 requirement that group chat works
// without the live-session domain. Behavioral proof lives in
// TestGroupService_NoLiveSessionIndependence (operations succeed with zero
// session rows) and the F-004 contract gate; this bounded scan covers only
// the non-test .go files of this package and catches a future import of the
// sessions domain, a media-provider SDK, or a messaging SDK. It is not
// project-wide proof.
func TestGroupService_PackageStaysSessionIndependent(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	entries, err := os.ReadDir(filepath.Dir(thisFile))
	if err != nil {
		t.Fatalf("read chat package: %v", err)
	}
	forbidden := []string{"/internal/sessions", "github.com/livekit/", "firebase.google.com/go"}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, marker := range forbidden {
			if strings.Contains(string(contents), marker) {
				t.Errorf("%s must not depend on %q", name, marker)
			}
		}
	}
}
