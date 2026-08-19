//go:build integration

package integration

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions/livekit"
	"github.com/livekit/protocol/auth"
)

func TestLiveKitWebhookIntegration_RateLimitAndSignedDelivery(t *testing.T) {
	const key, secret = "integration-key", "integration-secret"
	body := `{"event":"room_finished","room":{"name":"opaque-room"},"created_at":1765000000}`
	verifier := livekit.NewHandlerVerifier(key, secret)
	endpoint := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.Verify(r); err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := middleware.NewRateLimitMiddleware(1, 1).LimitByIP(endpoint)

	req := signedIntegrationWebhook(t, key, secret, body)
	req.RemoteAddr = "198.51.100.10:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("signed delivery status = %d, want 204", rec.Code)
	}
	req = signedIntegrationWebhook(t, key, secret, body)
	req.RemoteAddr = "198.51.100.10:1234"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second delivery status = %d, want 429", rec.Code)
	}
	if strings.Contains(rec.Body.String(), secret) || strings.Contains(rec.Body.String(), "opaque-room") {
		t.Fatal("rate-limit response exposed provider data")
	}
}

func signedIntegrationWebhook(t *testing.T, key, secret, body string) *http.Request {
	t.Helper()
	sum := sha256.Sum256([]byte(body))
	token, err := auth.NewAccessToken(key, secret).SetSha256(base64.StdEncoding.EncodeToString(sum[:])).ToJWT()
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/livekit", strings.NewReader(body))
	req.Header.Set("Authorization", token)
	return req
}

var _ sessions.MediaWebhookEventType
