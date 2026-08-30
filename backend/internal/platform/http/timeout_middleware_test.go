package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"

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

func TestWebSocketUpgradeThroughSharedMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(testWriter{t}, nil))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Upgrade(w, r, nil, 0, 0)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()
	})
	wrapped := TimeoutMiddleware(time.Second,
		RecoveryMiddleware(logger,
			RequestIDMiddleware(LoggerMiddleware(logger, handler)),
		),
	)
	server := httptest.NewServer(wrapped)
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):], nil)
	if err != nil {
		t.Fatalf("dial websocket through shared middleware: %v", err)
	}
	defer conn.Close()
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}
