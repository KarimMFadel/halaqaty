//go:build contract

package contract

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecitationQueueMediaBoundary(t *testing.T) {
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
		text := string(contents)
		for _, forbidden := range []string{"livekit", "ReciterAudioControl", "GrantReciterAudio", "RevokeReciterAudio"} {
			if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
				t.Errorf("queue/%s contains forbidden media-boundary symbol %q", entry.Name(), forbidden)
			}
		}
	}
}
