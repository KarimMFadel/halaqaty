package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTimeoutResponseWriter_WriteImplicitlySetsOKStatus(t *testing.T) {
	w := &timeoutResponseWriter{header: make(http.Header)}
	if _, err := w.Write([]byte("hello ")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if _, err := w.Write([]byte("world")); err != nil {
		t.Fatalf("second write: %v", err)
	}
	if w.status != http.StatusOK {
		t.Fatalf("status = %d, want implicit %d", w.status, http.StatusOK)
	}
	if got := w.body.String(); got != "hello world" {
		t.Fatalf("body = %q, want buffered writes in order", got)
	}
}

func TestTimeoutResponseWriter_FirstWriteHeaderWins(t *testing.T) {
	w := &timeoutResponseWriter{header: make(http.Header)}
	w.WriteHeader(http.StatusTeapot)
	w.WriteHeader(http.StatusInternalServerError)
	if w.status != http.StatusTeapot {
		t.Fatalf("status = %d, want first status %d", w.status, http.StatusTeapot)
	}
}

func TestTimeoutResponseWriter_WriteTo_CopiesHeadersStatusAndBody(t *testing.T) {
	w := &timeoutResponseWriter{header: make(http.Header)}
	w.Header().Set("X-Cache", "no-store")
	w.WriteHeader(http.StatusCreated)
	if _, err := w.Write([]byte(`{"ok":true}`)); err != nil {
		t.Fatalf("write body: %v", err)
	}

	rec := httptest.NewRecorder()
	w.writeTo(rec)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
	if got := rec.Header().Get("X-Cache"); got != "no-store" {
		t.Fatalf("header X-Cache = %q, want copied value", got)
	}
	if rec.Body.String() != `{"ok":true}` {
		t.Fatalf("body = %q, want buffered body", rec.Body.String())
	}
	// Headers must be copied, not shared, so later mutation of the target
	// cannot corrupt the buffered writer.
	rec.Header().Set("X-Cache", "mutated")
	if got := w.Header().Get("X-Cache"); got != "no-store" {
		t.Fatalf("writeTo shared the header map: X-Cache = %q", got)
	}
}

func TestTimeoutResponseWriter_WriteTo_DefaultsToOKWithoutStatus(t *testing.T) {
	w := &timeoutResponseWriter{header: make(http.Header)}
	rec := httptest.NewRecorder()
	w.writeTo(rec)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want default %d", rec.Code, http.StatusOK)
	}
}
