//go:build contract

package contract

import (
	"bytes"
	"context"
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

func TestProfileMeContract_RequiresBearerAndSession(t *testing.T) {
	t.Parallel()

	store := newProfileStoreStub()
	profileHandler := profile.NewHandler(profile.NewService(store))

	verifier := &alwaysOKVerifier{}
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	sessionService := auth.NewSessionService(30 * 24 * time.Hour)
	authMW := middleware.NewAuthMiddleware(verifier, sessionService, repo)

	getRoute := authMW.Require(http.HandlerFunc(profileHandler.GetMe))
	putRoute := authMW.Require(http.HandlerFunc(profileHandler.UpdateMe))

	cases := []struct {
		name       string
		handler    http.Handler
		method     string
		body       string
		authHeader string
		sessionID  string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "GET missing bearer returns 401",
			handler:    getRoute,
			method:     http.MethodGet,
			authHeader: "",
			sessionID:  testSessionID,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeUnauthorized,
		},
		{
			name:       "GET missing session header returns 401",
			handler:    getRoute,
			method:     http.MethodGet,
			authHeader: bearerValid,
			sessionID:  "",
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionMissing,
		},
		{
			name:       "PUT missing bearer returns 401",
			handler:    putRoute,
			method:     http.MethodPut,
			body:       `{"full_name":"Ali Mahmoud","country":"EG"}`,
			authHeader: "",
			sessionID:  testSessionID,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeUnauthorized,
		},
		{
			name:       "PUT missing session header returns 401",
			handler:    putRoute,
			method:     http.MethodPut,
			body:       `{"full_name":"Ali Mahmoud","country":"EG"}`,
			authHeader: bearerValid,
			sessionID:  "",
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionMissing,
		},
		{
			name:       "GET with bearer and session returns profile",
			handler:    getRoute,
			method:     http.MethodGet,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusOK,
		},
		{
			name:       "PUT with bearer and session updates profile",
			handler:    putRoute,
			method:     http.MethodPut,
			body:       `{"full_name":"Ali Mahmoud","country":"eg"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/auth/me", bytes.NewBufferString(tc.body))
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			if tc.authHeader != "" {
				req.Header.Set(httpconst.HeaderAuthorization, tc.authHeader)
			}
			if tc.sessionID != "" {
				req.Header.Set(httpconst.HeaderSessionID, tc.sessionID)
			}

			rec := httptest.NewRecorder()
			tc.handler.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}

			if tc.wantCode != "" {
				var env phttp.ErrorEnvelope
				if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
					t.Fatalf("decode error envelope: %v", err)
				}
				if env.Error.Code != tc.wantCode {
					t.Fatalf("error code: got %q want %q", env.Error.Code, tc.wantCode)
				}
				return
			}

			var got auth.UserProfile
			if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
				t.Fatalf("decode profile response: %v", err)
			}
			if got.ID != testLocalUserID {
				t.Fatalf("expected profile id %q got %q", testLocalUserID, got.ID)
			}
		})
	}
}

type profileStoreStub struct {
	record profile.Record
}

func newProfileStoreStub() *profileStoreStub {
	displayName := "Ali"
	return &profileStoreStub{
		record: profile.Record{
			Profile: auth.UserProfile{
				ID:                testLocalUserID,
				FirebaseUID:       testFirebaseUID,
				DisplayName:       &displayName,
				PreferredLanguage: "ar",
				CreatedAt:         time.Now().UTC(),
			},
		},
	}
}

func (s *profileStoreStub) GetByUserID(_ context.Context, _ string) (profile.Record, error) {
	return s.record, nil
}

func (s *profileStoreStub) UpdateByUserID(_ context.Context, in profile.UpdateInput) error {
	if in.FullName != nil {
		s.record.Profile.FullName = strPtrContract(*in.FullName)
	}
	if in.DisplayName != nil {
		s.record.Profile.DisplayName = strPtrContract(*in.DisplayName)
	}
	if in.Bio != nil {
		s.record.Profile.Bio = strPtrContract(*in.Bio)
	}
	if in.Country != nil {
		s.record.Profile.Country = strPtrContract(*in.Country)
	}
	if in.AvatarURL != nil {
		s.record.Profile.AvatarURL = strPtrContract(*in.AvatarURL)
	}
	s.record.CompletedAt = in.CompletedAt
	return nil
}

func strPtrContract(value string) *string {
	return &value
}
