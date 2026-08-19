//go:build contract

package contract

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestLiveKitSDKImportsAreConfinedToTheAdapter protects ADR-015's dependency
// direction. The provider SDK may be imported only by the backend adapter;
// session policy and the rest of the backend must compile against neutral
// session-media types.
func TestLiveKitSDKImportsAreConfinedToTheAdapter(t *testing.T) {
	root := repositoryRoot(t)
	internal := filepath.Join(root, "backend", "internal")
	allowed := filepath.Join("sessions", "livekit")

	err := filepath.Walk(internal, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		for _, spec := range file.Imports {
			importPath := strings.Trim(spec.Path.Value, `"`)
			if !strings.Contains(importPath, "github.com/livekit/") {
				continue
			}
			rel, relErr := filepath.Rel(internal, filepath.Dir(path))
			if relErr != nil {
				return relErr
			}
			if rel != allowed {
				t.Errorf("%s imports provider SDK %q outside backend/internal/sessions/livekit", filepath.ToSlash(filepath.Join("backend", "internal", rel, filepath.Base(path))), importPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk backend/internal: %v", err)
	}
}

// TestLiveKitFieldsDoNotCrossTheBoundary prevents provider-shaped public or
// durable fields (for example livekit_room or livekit_token) from becoming a
// contract. Provider state is represented by neutral MediaConnection and
// media_room_ref values outside the adapter.
func TestLiveKitFieldsDoNotCrossTheBoundary(t *testing.T) {
	root := repositoryRoot(t)
	for _, base := range []string{
		filepath.Join(root, "backend", "internal"),
		filepath.Join(root, "mobile", "lib"),
	} {
		err := filepath.Walk(base, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || (!strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, ".dart")) {
				return nil
			}
			contents, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, line := range strings.Split(string(contents), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.Contains(trimmed, "json:\"livekit_") || strings.Contains(trimmed, "@JsonKey(name: 'livekit_") {
					t.Errorf("%s exposes provider-specific field: %s", path, trimmed)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", base, err)
		}
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(1)
	if !ok {
		t.Fatal("resolve contract test location")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}
