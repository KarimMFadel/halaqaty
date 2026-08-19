//go:build contract

package contract

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions/livekit"
	"github.com/livekit/protocol/auth"
)

const (
	webhookKey    = "contract-key"
	webhookSecret = "contract-secret"
)

const webhookBody = `{"event":"participant_joined","room":{"name":"opaque-room"},"participant":{"identity":"user-1"},"created_at":1765000000}`

func signedWebhook(t *testing.T, body, secret string, authorization bool) *http.Request {
	t.Helper()
	sum := sha256.Sum256([]byte(body))
	token, err := auth.NewAccessToken(webhookKey, secret).SetSha256(base64.StdEncoding.EncodeToString(sum[:])).ToJWT()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/livekit", strings.NewReader(body))
	if authorization {
		req.Header.Set("Authorization", token)
	}
	return req
}

func TestLiveKitWebhookContractVerifiesRequiredSignatureAndJSON(t *testing.T) {
	contract, err := os.ReadFile("../../../specs/005-live-sessions-livekit/contracts/live-sessions.openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"/webhooks/livekit:", "name: Authorization", "application/json", "signature"} {
		if !strings.Contains(string(contract), required) {
			t.Errorf("webhook contract missing %q", required)
		}
	}
	verifier := livekit.NewWebhookVerifier(webhookKey, webhookSecret)
	if _, err := verifier.Verify(signedWebhook(t, webhookBody, webhookSecret, true)); err != nil {
		t.Fatalf("valid signed webhook rejected: %v", err)
	}
	for name, req := range map[string]*http.Request{
		"missing authorization": signedWebhook(t, webhookBody, webhookSecret, false),
		"invalid signature":     signedWebhook(t, webhookBody, "wrong-secret", true),
		"invalid json":          signedWebhook(t, "{not-json", webhookSecret, true),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := verifier.Verify(req); err == nil {
				t.Fatal("invalid webhook was accepted")
			}
		})
	}
}

func TestLiveKitWebhookContractDuplicateAndAuditSafety(t *testing.T) {
	verifier := livekit.NewWebhookVerifier(webhookKey, webhookSecret)
	first, err := verifier.Verify(signedWebhook(t, webhookBody, webhookSecret, true))
	if err != nil {
		t.Fatal(err)
	}
	second, err := verifier.Verify(signedWebhook(t, webhookBody, webhookSecret, true))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("duplicate delivery IDs = %q and %q", first.ID, second.ID)
	}
	if strings.Contains(string(first.RoomRef), webhookSecret) || strings.Contains(first.Identity, webhookSecret) {
		t.Fatal("provider secret appeared in neutral audit fields")
	}
	if _, err := verifier.Verify(signedWebhook(t, webhookBody, "wrong-secret", true)); err == nil || strings.Contains(err.Error(), webhookSecret) || strings.Contains(err.Error(), "wrong-secret") {
		t.Fatalf("signature error leaked credential: %v", err)
	}
	var _ sessions.MediaWebhookEventType
}

func TestLiveKitWebhookHTTPContractMapsValidationAndSignatureErrors(t *testing.T) {
	handler := sessions.NewHandler(nil)
	handler.SetWebhookVerifier(livekit.NewHandlerVerifier(webhookKey, webhookSecret))
	for name, tc := range map[string]struct {
		req  *http.Request
		want int
	}{
		"valid":     {signedWebhook(t, webhookBody, webhookSecret, true), http.StatusNoContent},
		"bad sign":  {signedWebhook(t, webhookBody, "wrong-secret", true), http.StatusUnauthorized},
		"bad json":  {signedWebhook(t, "{not-json", webhookSecret, true), http.StatusBadRequest},
		"no header": {signedWebhook(t, webhookBody, webhookSecret, false), http.StatusUnauthorized},
	} {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, tc.req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.want, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), webhookSecret) || strings.Contains(rec.Body.String(), "opaque-room") {
				t.Fatal("webhook response exposed provider credential or room reference")
			}
			if tc.want != http.StatusNoContent {
				var envelope phttp.ErrorEnvelope
				if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
					t.Fatalf("error response is not standard JSON: %v", err)
				}
			}
		})
	}
}
