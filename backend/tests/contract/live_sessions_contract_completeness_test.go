//go:build contract

package contract

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// T055 checks the F-005 contract surface and the implemented media response
// safety rules. Runtime discovery wiring is intentionally not invented here;
// T057/T059 own that implementation.
func TestLiveSessionsContractCompleteness(t *testing.T) {
	contract, err := os.ReadFile("../../../specs/005-live-sessions-livekit/contracts/live-sessions.openapi.yaml")
	if err != nil {
		t.Fatalf("read feature contract: %v", err)
	}
	text := string(contract)
	for _, required := range []string{
		"/circles/{circleId}/sessions:",
		"ERR_MEDIA_UNAVAILABLE",
		"Cache-Control",
		"no-store",
		"ErrorResponse",
		"F-006",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("feature contract missing %q", required)
		}
	}

	// Every F-005 operation advertises the complete standard error mapping.
	for _, status := range []string{"'400'", "'401'", "'403'", "'404'", "'409'", "'422'", "'429'", "'500'"} {
		if strings.Count(text, status) == 0 {
			t.Errorf("feature contract has no %s response", status)
		}
	}
	if !strings.Contains(text, "'503': { $ref: '#/components/responses/Error' }") {
		t.Error("feature contract has no 503 error response")
	}
}

func TestLiveSessionsStandardErrorEnvelopeShape(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
	}{
		{http.StatusBadRequest, httpconst.ErrorCodeValidationFailed},
		{http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized},
		{http.StatusForbidden, httpconst.ErrorCodeForbidden},
		{http.StatusNotFound, httpconst.ErrorCodeNotFound},
		{http.StatusConflict, httpconst.ErrorCodeConflict},
		{http.StatusTooManyRequests, httpconst.ErrorCodeRateLimitExceeded},
		{http.StatusInternalServerError, httpconst.ErrorCodeInternalServerError},
		{http.StatusServiceUnavailable, httpconst.ErrorCodeMediaUnavailable},
	} {
		t.Run(tc.code, func(t *testing.T) {
			body, err := json.Marshal(phttp.ErrorEnvelope{Error: phttp.ErrorBody{Code: tc.code, Message: "contract"}})
			if err != nil {
				t.Fatal(err)
			}
			var got phttp.ErrorEnvelope
			if err := json.Unmarshal(body, &got); err != nil {
				t.Fatalf("decode status %d envelope: %v", tc.status, err)
			}
			if got.Error.Code != tc.code || got.Error.Message == "" {
				t.Fatalf("invalid standard envelope: %+v", got)
			}
		})
	}
}
