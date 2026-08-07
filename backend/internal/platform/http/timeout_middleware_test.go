package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func TestTimeoutMiddleware_ReturnsJSONErrorEnvelope(t *testing.T) {
	handler := TimeoutMiddleware(time.Millisecond, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		time.Sleep(10 * time.Millisecond)
	}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
	if got := rec.Header().Get(httpconst.HeaderContentType); got != httpconst.ContentTypeApplicationJSON {
		t.Fatalf("content type: got %q, want %q", got, httpconst.ContentTypeApplicationJSON)
	}
	var envelope ErrorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != httpconst.ErrorCodeRequestTimeout {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeRequestTimeout)
	}
}
