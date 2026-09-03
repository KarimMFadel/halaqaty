package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

const handlerTestUserID = "11111111-1111-1111-1111-111111111112"

// newHandlerTestService returns a service over a store that can fail on demand.
func newHandlerTestService() (*Handler, *handlerStore) {
	displayName := "Handler User"
	fullName := "Handler Full Name"
	country := "EG"
	completedAt := time.Now().UTC().Add(-time.Hour)
	store := &handlerStore{
		record: Record{
			Profile: auth.UserProfile{
				ID:                handlerTestUserID,
				FirebaseUID:       "firebase-handler-test",
				FullName:          &fullName,
				DisplayName:       &displayName,
				Country:           &country,
				PreferredLanguage: "ar",
				CreatedAt:         time.Now().UTC(),
			},
			CompletedAt: &completedAt,
		},
	}
	return NewHandler(NewService(store)), store
}

func requestWithPrincipal(method, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, "/auth/me", nil)
	} else {
		req = httptest.NewRequest(method, "/auth/me", strings.NewReader(body))
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	}
	return req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: handlerTestUserID}))
}

func decodeHandlerError(t *testing.T, rec *httptest.ResponseRecorder) phttp.ErrorEnvelope {
	t.Helper()
	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v body=%s", err, rec.Body.String())
	}
	return envelope
}

func TestHandler_GetMe_UnauthenticatedOrMisconfiguredReturnsTypedErrors(t *testing.T) {
	handler, _ := newHandlerTestService()

	cases := []struct {
		name        string
		handler     *Handler
		principal   bool
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "missing handler service reports internal error",
			handler:     &Handler{},
			principal:   true,
			wantStatus:  http.StatusInternalServerError,
			wantCode:    httpconst.ErrorCodeInternalServerError,
			wantMessage: httpconst.ErrorMessageProfileHandlerNotConfigured,
		},
		{
			name:        "missing principal is rejected as unauthorized",
			handler:     handler,
			wantStatus:  http.StatusUnauthorized,
			wantCode:    httpconst.ErrorCodeUnauthorized,
			wantMessage: httpconst.ErrorMessageUnauthorized,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
			if tc.principal {
				req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: handlerTestUserID}))
			}
			rec := httptest.NewRecorder()
			tc.handler.GetMe(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			envelope := decodeHandlerError(t, rec)
			if envelope.Error.Code != tc.wantCode {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, tc.wantCode)
			}
			if envelope.Error.Message != tc.wantMessage {
				t.Fatalf("error message: got %q, want %q", envelope.Error.Message, tc.wantMessage)
			}
		})
	}
}

func TestHandler_GetMe_EmptyUserIDPrincipalIsRejected(t *testing.T) {
	handler, _ := newHandlerTestService()

	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: ""}))
	rec := httptest.NewRecorder()
	handler.GetMe(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if envelope := decodeHandlerError(t, rec); envelope.Error.Code != httpconst.ErrorCodeUnauthorized {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeUnauthorized)
	}
}

func TestHandler_GetMe_StoreFailureReturnsInternalServerError(t *testing.T) {
	handler, store := newHandlerTestService()
	store.failGet = true

	rec := httptest.NewRecorder()
	handler.GetMe(rec, requestWithPrincipal(http.MethodGet, ""))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	envelope := decodeHandlerError(t, rec)
	if envelope.Error.Code != httpconst.ErrorCodeInternalServerError {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeInternalServerError)
	}
	if envelope.Error.Message != httpconst.ErrorMessageInternalServerError {
		t.Fatalf("error message: got %q, want %q", envelope.Error.Message, httpconst.ErrorMessageInternalServerError)
	}
}

func TestHandler_GetMe_ReturnsStoredProfile(t *testing.T) {
	handler, _ := newHandlerTestService()

	rec := httptest.NewRecorder()
	handler.GetMe(rec, requestWithPrincipal(http.MethodGet, ""))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got auth.UserProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode profile response: %v", err)
	}
	if got.ID != handlerTestUserID {
		t.Fatalf("profile id: got %q, want %q", got.ID, handlerTestUserID)
	}
	if got.FullName == nil || *got.FullName != "Handler Full Name" {
		t.Fatalf("full name: got %+v, want %q", got.FullName, "Handler Full Name")
	}
}

func TestHandler_UpdateMe_UnauthenticatedOrMisconfiguredReturnsTypedErrors(t *testing.T) {
	handler, _ := newHandlerTestService()

	cases := []struct {
		name        string
		handler     *Handler
		principal   bool
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "missing handler service reports internal error",
			handler:     &Handler{},
			principal:   true,
			wantStatus:  http.StatusInternalServerError,
			wantCode:    httpconst.ErrorCodeInternalServerError,
			wantMessage: httpconst.ErrorMessageProfileHandlerNotConfigured,
		},
		{
			name:        "missing principal is rejected as unauthorized",
			handler:     handler,
			wantStatus:  http.StatusUnauthorized,
			wantCode:    httpconst.ErrorCodeUnauthorized,
			wantMessage: httpconst.ErrorMessageUnauthorized,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPut, "/auth/me", strings.NewReader(`{"bio":"x"}`))
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			if tc.principal {
				req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: handlerTestUserID}))
			}
			rec := httptest.NewRecorder()
			tc.handler.UpdateMe(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			envelope := decodeHandlerError(t, rec)
			if envelope.Error.Code != tc.wantCode {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, tc.wantCode)
			}
			if envelope.Error.Message != tc.wantMessage {
				t.Fatalf("error message: got %q, want %q", envelope.Error.Message, tc.wantMessage)
			}
		})
	}
}

func TestHandler_UpdateMe_MalformedBodyReturnsFieldValidationErrors(t *testing.T) {
	handler, _ := newHandlerTestService()

	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{
			name:    "invalid json is reported on the body field",
			body:    `{"bio":`,
			wantMsg: httpconst.ErrorMessageRequestBodyInvalid,
		},
		{
			name:    "unknown fields are rejected",
			body:    `{"nickname":"Ali"}`,
			wantMsg: httpconst.ErrorMessageRequestBodyUnknownFields,
		},
		{
			name:    "trailing content after the json object is rejected",
			body:    `{"bio":"x"}{"bio":"y"}`,
			wantMsg: httpconst.ErrorMessageRequestBodyInvalid,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.UpdateMe(rec, requestWithPrincipal(http.MethodPut, tc.body))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
			}
			envelope := decodeHandlerError(t, rec)
			if envelope.Error.Code != httpconst.ErrorCodeValidationFailed {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeValidationFailed)
			}
			if got := envelope.Error.Fields[httpconst.FieldBody]; got != tc.wantMsg {
				t.Fatalf("body field message: got %q, want %q fields=%v", got, tc.wantMsg, envelope.Error.Fields)
			}
		})
	}
}

func TestHandler_UpdateMe_ServiceValidationErrorsSurfaceFieldMap(t *testing.T) {
	handler, _ := newHandlerTestService()

	rec := httptest.NewRecorder()
	handler.UpdateMe(rec, requestWithPrincipal(http.MethodPut, `{"country":"1A"}`))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	envelope := decodeHandlerError(t, rec)
	if envelope.Error.Code != httpconst.ErrorCodeValidationFailed {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeValidationFailed)
	}
	if envelope.Error.Message != httpconst.ErrorMessageValidationFailed {
		t.Fatalf("error message: got %q, want %q", envelope.Error.Message, httpconst.ErrorMessageValidationFailed)
	}
	if envelope.Error.Fields[httpconst.FieldCountry] != httpconst.ErrorMessageCountryInvalid {
		t.Fatalf("country field: got %q, want %q fields=%v", envelope.Error.Fields[httpconst.FieldCountry], httpconst.ErrorMessageCountryInvalid, envelope.Error.Fields)
	}
}

func TestHandler_UpdateMe_StoreFailureReturnsInternalServerError(t *testing.T) {
	handler, store := newHandlerTestService()
	store.failUpdate = true

	rec := httptest.NewRecorder()
	handler.UpdateMe(rec, requestWithPrincipal(http.MethodPut, `{"bio":"Student"}`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	envelope := decodeHandlerError(t, rec)
	if envelope.Error.Code != httpconst.ErrorCodeInternalServerError {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeInternalServerError)
	}
}

func TestHandler_UpdateMe_LoadFailureReturnsInternalServerError(t *testing.T) {
	handler, store := newHandlerTestService()
	store.failGet = true

	rec := httptest.NewRecorder()
	handler.UpdateMe(rec, requestWithPrincipal(http.MethodPut, `{"bio":"Student"}`))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
	}
	envelope := decodeHandlerError(t, rec)
	if envelope.Error.Code != httpconst.ErrorCodeInternalServerError {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeInternalServerError)
	}
}

func TestHandler_UpdateMe_UpdatesAndReturnsProfile(t *testing.T) {
	handler, _ := newHandlerTestService()

	rec := httptest.NewRecorder()
	handler.UpdateMe(rec, requestWithPrincipal(http.MethodPut, `{"bio":"Updated bio","country":"eg"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var got auth.UserProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode profile response: %v", err)
	}
	if got.Bio == nil || *got.Bio != "Updated bio" {
		t.Fatalf("bio: got %+v, want %q", got.Bio, "Updated bio")
	}
	if got.Country == nil || *got.Country != "EG" {
		t.Fatalf("country: got %+v, want %q", got.Country, "EG")
	}
}

type handlerStore struct {
	record     Record
	failGet    bool
	failUpdate bool
}

func (s *handlerStore) GetByUserID(context.Context, string) (Record, error) {
	if s.failGet {
		return Record{}, context.DeadlineExceeded
	}
	return s.record, nil
}

func (s *handlerStore) UpdateByUserID(_ context.Context, in UpdateInput) error {
	if s.failUpdate {
		return context.DeadlineExceeded
	}
	if in.Bio != nil {
		s.record.Profile.Bio = strPtr(*in.Bio)
	}
	if in.Country != nil {
		s.record.Profile.Country = strPtr(*in.Country)
	}
	s.record.CompletedAt = in.CompletedAt
	return nil
}
