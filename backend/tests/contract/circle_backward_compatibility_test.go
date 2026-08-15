//go:build contract

package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func TestCircleBackwardCompatibility_ExistingOperationsRemainDocumented(t *testing.T) {
	contents, err := os.ReadFile("../../../docs/contracts/openapi.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI contract: %v", err)
	}
	contract := string(contents)
	for _, operation := range []string{
		"operationId: createCircle",
		"operationId: discoverPublicCircles",
		"operationId: getCircle",
		"operationId: listCircleMembers",
		"operationId: joinPublicCircle",
		"operationId: joinCircle",
	} {
		if !strings.Contains(contract, operation) {
			t.Errorf("backward-compatible operation missing: %s", operation)
		}
	}
}

func TestCircleBackwardCompatibility_StandardErrorEnvelopes(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusConflict,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusServiceUnavailable,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			recorder := httptest.NewRecorder()
			phttp.WriteError(recorder, "ERR_CIRCLE_TEST", "safe message", status)

			if recorder.Code != status {
				t.Fatalf("status: got %d want %d", recorder.Code, status)
			}
			if recorder.Header().Get(httpconst.HeaderContentType) != httpconst.ContentTypeApplicationJSON {
				t.Fatalf("content type: %q", recorder.Header().Get(httpconst.HeaderContentType))
			}
			var envelope phttp.ErrorEnvelope
			if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
				t.Fatalf("decode envelope: %v", err)
			}
			if envelope.Error.Code != "ERR_CIRCLE_TEST" || envelope.Error.Message != "safe message" {
				t.Fatalf("error envelope: %+v", envelope)
			}
		})
	}
}

func TestCircleBackwardCompatibility_ValidationEnvelopeRetainsFieldErrors(t *testing.T) {
	recorder := httptest.NewRecorder()
	phttp.WriteValidationError(recorder, "validation failed", map[string]string{"circle_id": "must be a UUID"})

	var envelope phttp.ErrorEnvelope
	if err := json.NewDecoder(recorder.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode validation envelope: %v", err)
	}
	if recorder.Code != http.StatusBadRequest || envelope.Error.Fields["circle_id"] == "" {
		t.Fatalf("validation envelope: status=%d body=%+v", recorder.Code, envelope)
	}
}
