package livekit

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/livekit/protocol/auth"
)

const (
	whKey    = "test-key"
	whSecret = "test-secret-value-not-a-real-credential"
)

// sha256Sum returns the base64 body digest the webhook token must carry.
func sha256Sum(body string) string {
	sum := sha256.Sum256([]byte(body))
	return base64.StdEncoding.EncodeToString(sum[:])
}

// newSignedAPIToken builds the HMAC JWT LiveKit sends in the Authorization
// header of a webhook delivery, bound to the body digest.
func newSignedAPIToken(apiKey, apiSecret, digest string) (string, error) {
	return auth.NewAccessToken(apiKey, apiSecret).SetSha256(digest).ToJWT()
}

// signedWebhookRequest builds a provider webhook delivery for body.
func signedWebhookRequest(t *testing.T, apiKey, apiSecret string, body string, withAuth bool) *http.Request {
	t.Helper()
	token, err := newSignedAPIToken(apiKey, apiSecret, sha256Sum(body))
	if err != nil {
		t.Fatalf("sign test webhook: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "/api/v1/webhooks/livekit", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if withAuth {
		req.Header.Set(http.CanonicalHeaderKey("Authorization"), token)
	}
	return req
}

const joinedBody = `{
	"event": "participant_joined",
	"room": {"name": "room-ref-abc"},
	"participant": {"identity": "student-1"},
	"created_at": 1765000000
}`

func TestVerifyWebhookTranslatesNeutralEvent(t *testing.T) {
	verifier := NewWebhookVerifier(whKey, whSecret)
	event, err := verifier.Verify(signedWebhookRequest(t, whKey, whSecret, joinedBody, true))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if event.Type != EventParticipantJoined {
		t.Fatalf("type = %q, want %q", event.Type, EventParticipantJoined)
	}
	if event.RoomRef != "room-ref-abc" {
		t.Fatalf("room ref = %q, want room-ref-abc", event.RoomRef)
	}
	if event.Identity != "student-1" {
		t.Fatalf("identity = %q, want student-1", event.Identity)
	}
	if want := time.Unix(1765000000, 0).UTC(); !event.Timestamp.Equal(want) {
		t.Fatalf("timestamp = %v, want %v", event.Timestamp, want)
	}
	if event.ID == "" {
		t.Fatal("event must carry a stable dedup identifier")
	}
}

func TestVerifyWebhookRejectsBadSignature(t *testing.T) {
	verifier := NewWebhookVerifier(whKey, whSecret)
	if _, err := verifier.Verify(signedWebhookRequest(t, whKey, "wrong-secret", joinedBody, true)); err == nil {
		t.Fatal("webhook signed with the wrong secret must be rejected")
	}
}

func TestVerifyWebhookRejectsMissingAuthorization(t *testing.T) {
	verifier := NewWebhookVerifier(whKey, whSecret)
	if _, err := verifier.Verify(signedWebhookRequest(t, whKey, whSecret, joinedBody, false)); err == nil {
		t.Fatal("webhook without the Authorization header must be rejected")
	}
}

func TestVerifyWebhookRejectsTamperedBody(t *testing.T) {
	verifier := NewWebhookVerifier(whKey, whSecret)
	// Sign the original body, then deliver a tampered variant.
	token, err := newSignedAPIToken(whKey, whSecret, sha256Sum(joinedBody))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	tampered := strings.Replace(joinedBody, "student-1", "attacker-1", 1)
	req, err := http.NewRequest(http.MethodPost, "/webhooks/livekit", strings.NewReader(tampered))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set(http.CanonicalHeaderKey("Authorization"), token)
	if _, err := verifier.Verify(req); err == nil {
		t.Fatal("tampered body must fail the digest check")
	}
}

func TestVerifyWebhookDuplicateDeliveryIsStable(t *testing.T) {
	verifier := NewWebhookVerifier(whKey, whSecret)
	first, err := verifier.Verify(signedWebhookRequest(t, whKey, whSecret, joinedBody, true))
	if err != nil {
		t.Fatalf("first delivery: %v", err)
	}
	second, err := verifier.Verify(signedWebhookRequest(t, whKey, whSecret, joinedBody, true))
	if err != nil {
		t.Fatalf("duplicate delivery: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("duplicate delivery must keep the event id stable: %q vs %q", first.ID, second.ID)
	}
	// A different event must never collide.
	other := strings.Replace(joinedBody, "student-1", "student-2", 1)
	third, err := verifier.Verify(signedWebhookRequest(t, whKey, whSecret, other, true))
	if err != nil {
		t.Fatalf("other event: %v", err)
	}
	if third.ID == first.ID {
		t.Fatal("distinct events must have distinct ids")
	}
}

func TestVerifyWebhookRejectsUnsupportedEvents(t *testing.T) {
	verifier := NewWebhookVerifier(whKey, whSecret)
	body := `{"event": "track_published", "room": {"name": "r"}, "participant": {"identity": "u"}, "created_at": 1}`
	if _, err := verifier.Verify(signedWebhookRequest(t, whKey, whSecret, body, true)); err == nil {
		t.Fatal("events outside the F-005 neutral set must be rejected, not silently dropped")
	}
}

func TestVerifyWebhookRejectsMalformedJSON(t *testing.T) {
	verifier := NewWebhookVerifier(whKey, whSecret)
	if _, err := verifier.Verify(signedWebhookRequest(t, whKey, whSecret, "{not json", true)); err == nil {
		t.Fatal("malformed body must be rejected")
	}
}

func TestVerifyWebhookRoomFinishedCarriesNoIdentity(t *testing.T) {
	verifier := NewWebhookVerifier(whKey, whSecret)
	body := `{"event": "room_finished", "room": {"name": "room-ref-xyz"}, "created_at": 1765000100}`
	event, err := verifier.Verify(signedWebhookRequest(t, whKey, whSecret, body, true))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if event.Type != EventRoomFinished {
		t.Fatalf("type = %q, want %q", event.Type, EventRoomFinished)
	}
	if event.Identity != "" {
		t.Fatalf("room events carry no identity, got %q", event.Identity)
	}
	if event.RoomRef == "" {
		t.Fatal("room events must carry the room reference")
	}
}

func TestVerifyWebhookErrorMessagesNeverLeakSecret(t *testing.T) {
	verifier := NewWebhookVerifier(whKey, whSecret)
	_, err := verifier.Verify(signedWebhookRequest(t, whKey, "wrong-secret", joinedBody, true))
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), whSecret) || strings.Contains(err.Error(), "wrong-secret") {
		t.Fatalf("error message leaks a secret: %v", err)
	}
	_ = fmt.Sprint(verifier) // formatting the verifier must not panic
}
