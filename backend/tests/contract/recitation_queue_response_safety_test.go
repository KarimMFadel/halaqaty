//go:build contract

package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecitationQueueResponseSafety(t *testing.T) {
	root := filepath.Join("..", "..", "internal", "queue")
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read queue package: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", entry.Name(), err)
		}
		text := strings.ToLower(string(contents))
		for _, forbidden := range []string{"media_credential", "media_room_ref", "stack trace", "provider_secret"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("queue/%s contains forbidden client-visible detail %q", entry.Name(), forbidden)
			}
		}
	}
}
