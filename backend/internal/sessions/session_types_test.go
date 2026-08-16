package sessions

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestMediaCredentialNeverFormatted guards FR-005: formatting a
// MediaConnection or its MediaCredential must never expose the raw
// credential value. (The credential is a string-kind type, so JSON encoding
// is unaffected by the String method.)
func TestMediaCredentialNeverFormatted(t *testing.T) {
	const raw = "raw-media-credential-do-not-print"
	conn := MediaConnection{
		Endpoint:   "wss://media.example.com",
		Credential: MediaCredential(raw),
		ExpiresAt:  time.Now(),
	}

	for _, rendered := range []string{
		fmt.Sprint(conn),
		fmt.Sprintf("%v", conn),
		fmt.Sprintf("%+v", conn),
		fmt.Sprint(conn.Credential),
		fmt.Sprintf("%v", conn.Credential),
		fmt.Sprintf("%+v", conn.Credential),
		fmt.Sprintf("%#v", conn.Credential),
		conn.Credential.String(),
		conn.Credential.GoString(),
	} {
		if strings.Contains(rendered, raw) {
			t.Fatalf("formatted media connection leaks credential: %q", rendered)
		}
	}
}
