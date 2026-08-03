//go:build contract

package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/profile"
)

func TestProfileValidationContract_ErrorEnvelopeAndFieldMap(t *testing.T) {
	t.Parallel()

	store := newProfileStoreStub()
	profileHandler := profile.NewHandler(profile.NewService(store))
	verifier := &alwaysOKVerifier{}
	repo := &stubSessionRepo{
		sessionID: testSessionID,
		userID:    testLocalUserID,
		expiresAt: time.Now().UTC().Add(24 * time.Hour),
	}
	sessionService := auth.NewSessionService(30 * 24 * time.Hour)
	authMW := middleware.NewAuthMiddleware(verifier, sessionService, repo)
	route := authMW.Require(http.HandlerFunc(profileHandler.UpdateMe))

	cases := []struct {
		name       string
		body       string
		wantFields []string
	}{
		{
			name:       "unknown field maps to body validation field",
			body:       `{"full_name":"Ali Mahmoud","country":"EG","unexpected":1}`,
			wantFields: []string{httpconst.FieldBody},
		},
		{
			name:       "malformed JSON maps to body validation field",
			body:       `{"full_name":"Ali","country":"EG"`,
			wantFields: []string{httpconst.FieldBody},
		},
		{
			name:       "first completion missing required full_name and country",
			body:       `{"bio":"Student"}`,
			wantFields: []string{httpconst.FieldFullName, httpconst.FieldCountry},
		},
		{
			name:       "invalid country code maps to country field",
			body:       `{"full_name":"Ali Mahmoud","country":"E1"}`,
			wantFields: []string{httpconst.FieldCountry},
		},
		{
			name:       "empty display_name maps to display_name field",
			body:       `{"full_name":"Ali Mahmoud","country":"EG","display_name":""}`,
			wantFields: []string{httpconst.FieldDisplayName},
		},
		{
			name:       "single-character display_name maps to display_name field",
			body:       `{"full_name":"Ali Mahmoud","country":"EG","display_name":"A"}`,
			wantFields: []string{httpconst.FieldDisplayName},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Ensure this case is treated as first completion by default.
			store.record.CompletedAt = nil
			store.record.Profile.FullName = nil
			store.record.Profile.Country = nil

			req := httptest.NewRequest(http.MethodPut, "/auth/me", bytes.NewBufferString(tc.body))
			req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
			req.Header.Set(httpconst.HeaderSessionID, testSessionID)
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)

			rec := httptest.NewRecorder()
			route.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}

			var env phttp.ErrorEnvelope
			if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if env.Error.Code != httpconst.ErrorCodeValidationFailed {
				t.Fatalf("error code: got %q want %q", env.Error.Code, httpconst.ErrorCodeValidationFailed)
			}
			if env.Error.Message != httpconst.ErrorMessageValidationFailed {
				t.Fatalf("error message: got %q want %q", env.Error.Message, httpconst.ErrorMessageValidationFailed)
			}

			for _, field := range tc.wantFields {
				if env.Error.Fields[field] == "" {
					t.Fatalf("expected validation field %q in fields map=%v", field, env.Error.Fields)
				}
			}
		})
	}
}
