package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func TestRoleMiddleware_RejectsMalformedCircleID(t *testing.T) {
	repo := &roleTestRepo{role: "teacher"}
	handler := NewRoleMiddleware(repo).RequireAny("teacher")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := requestWithPrincipal(http.MethodPut, "not-a-uuid")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assertErrorCode(t, rec, http.StatusBadRequest, httpconst.ErrorCodeValidationFailed)
	if repo.calls != 0 {
		t.Fatalf("membership lookup calls: got %d, want 0", repo.calls)
	}
}

func TestRoleMiddleware_ReturnsInternalErrorForRepositoryFailure(t *testing.T) {
	handler := NewRoleMiddleware(&roleTestRepo{err: errors.New("database unavailable")}).RequireAny("teacher")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, requestWithPrincipal(http.MethodPut, "11111111-1111-1111-1111-111111111111"))

	assertErrorCode(t, rec, http.StatusInternalServerError, httpconst.ErrorCodeInternalServerError)
}

func TestRoleMiddleware_ReturnsForbiddenForMissingMembership(t *testing.T) {
	handler := NewRoleMiddleware(&roleTestRepo{err: auth.ErrCircleMembershipNotFound}).RequireAny("teacher")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, requestWithPrincipal(http.MethodPut, "11111111-1111-1111-1111-111111111111"))

	assertErrorCode(t, rec, http.StatusForbidden, httpconst.ErrorCodeForbidden)
}

func requestWithPrincipal(method, circleID string) *http.Request {
	req := httptest.NewRequest(method, "/circles/"+circleID+"/members/user/role", nil)
	req.SetPathValue("circleId", circleID)
	return req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: "22222222-2222-2222-2222-222222222222"}))
}

func assertErrorCode(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status: got %d, want %d; body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	var envelope phttp.ErrorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != wantCode {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, wantCode)
	}
}

type roleTestRepo struct {
	role  string
	err   error
	calls int
}

func (r *roleTestRepo) RoleForUserInCircle(context.Context, string, string) (string, error) {
	r.calls++
	return r.role, r.err
}
