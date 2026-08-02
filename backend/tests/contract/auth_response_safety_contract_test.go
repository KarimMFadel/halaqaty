//go:build contract

package contract

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"halaqaty/backend/internal/platform/httpconst"
)

// TestResponseSafety asserts that auth endpoints never leak sensitive fields
// such as "password" in their responses.
func TestResponseSafety(t *testing.T) {
	store := &registerStubStore{}
	registerH := buildRegisterRoute(store)
	createSessionH := buildCreateSessionRoute()

	t.Run("register response never contains password field", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(`{"display_name":"Ali"}`))
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer any")

		rec := httptest.NewRecorder()
		registerH.ServeHTTP(rec, req)

		body := rec.Body.String()
		if strings.Contains(strings.ToLower(body), `"password"`) {
			t.Fatalf("register response must not contain 'password' field, got: %s", body)
		}
	})

	t.Run("create session response never contains password field", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/auth/sessions", bytes.NewBufferString(`{}`))
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer valid-token")

		rec := httptest.NewRecorder()
		createSessionH.ServeHTTP(rec, req)

		body := rec.Body.String()
		if strings.Contains(strings.ToLower(body), `"password"`) {
			t.Fatalf("create session response must not contain 'password' field, got: %s", body)
		}
	})
}
