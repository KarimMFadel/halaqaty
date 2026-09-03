package rbac

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// newHandlerRequest builds a request with an optional JSON body.
func newHandlerRequest(method, target, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, target, nil)
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	return req
}

// withUserPrincipal attaches an authenticated local user to the request.
func withUserPrincipal(req *http.Request, userID string) *http.Request {
	return req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: userID}))
}

func decodeErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder) phttp.ErrorEnvelope {
	t.Helper()
	var env phttp.ErrorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	return env
}

func TestRBACHandlers_MissingPrincipal_Return401(t *testing.T) {
	t.Parallel()

	handler := NewHandler(NewService(newStubStore(), nil))
	cases := []struct {
		name   string
		method string
		target string
		body   string
		run    func(http.ResponseWriter, *http.Request)
	}{
		{"create circle", http.MethodPost, "/circles", `{}`, handler.CreateCircle},
		{"update circle", http.MethodPut, "/circles/{circleId}", `{}`, handler.UpdateCircle},
		{"join circle by invite", http.MethodPost, "/circles/join", `{}`, handler.JoinCircle},
		{"join public circle", http.MethodPost, "/circles/{circleId}/join", "", handler.JoinPublicCircle},
		{"discover public circles", http.MethodGet, "/circles/discover", "", handler.DiscoverPublicCircles},
		{"assign role", http.MethodPut, "/circles/{circleId}/members/{userId}/role", `{}`, handler.AssignRole},
		{"get circle", http.MethodGet, "/circles/{circleId}", "", handler.GetCircle},
		{"list members", http.MethodGet, "/circles/{circleId}/members", "", handler.ListMembers},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := newHandlerRequest(tc.method, tc.target, tc.body)
			rec := httptest.NewRecorder()
			tc.run(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			if env := decodeErrorEnvelope(t, rec); env.Error.Code != httpconst.ErrorCodeUnauthorized {
				t.Fatalf("error code: got %q want %q", env.Error.Code, httpconst.ErrorCodeUnauthorized)
			}
		})
	}
}

func TestRBACHandlers_ZeroService_Return500(t *testing.T) {
	t.Parallel()

	var unconfigured *Handler
	cases := []struct {
		name   string
		method string
		target string
		run    func(http.ResponseWriter, *http.Request)
	}{
		{"create circle", http.MethodPost, "/circles", unconfigured.CreateCircle},
		{"join circle by invite", http.MethodPost, "/circles/join", unconfigured.JoinCircle},
		{"join public circle", http.MethodPost, "/circles/{circleId}/join", unconfigured.JoinPublicCircle},
		{"discover public circles", http.MethodGet, "/circles/discover", unconfigured.DiscoverPublicCircles},
		{"assign role", http.MethodPut, "/circles/{circleId}/members/{userId}/role", unconfigured.AssignRole},
		{"get circle", http.MethodGet, "/circles/{circleId}", unconfigured.GetCircle},
		{"list members", http.MethodGet, "/circles/{circleId}/members", unconfigured.ListMembers},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := newHandlerRequest(tc.method, tc.target, "")
			rec := httptest.NewRecorder()
			tc.run(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusInternalServerError, rec.Body.String())
			}
			if env := decodeErrorEnvelope(t, rec); env.Error.Code != httpconst.ErrorCodeInternalServerError {
				t.Fatalf("error code: got %q want %q", env.Error.Code, httpconst.ErrorCodeInternalServerError)
			}
		})
	}
}

func TestUpdateCircleHandler_ZeroService_FoldsInto401(t *testing.T) {
	t.Parallel()

	var unconfigured *Handler
	req := newHandlerRequest(http.MethodPut, "/circles/{circleId}", `{}`)
	rec := httptest.NewRecorder()
	unconfigured.UpdateCircle(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d — body: %s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func newUpdateCircleHandler() (*Handler, *stubStore) {
	store := newStubStore()
	store.circles[unitCircleID] = Circle{
		ID: unitCircleID, Name: "Old Name", MaxCapacity: 50,
		GenderRestriction: "unspecified", Language: "ar", GradingPolicy: "required",
	}
	store.members[unitCircleID] = map[string]string{unitCreatorID: RoleTeacher}
	return NewHandler(NewService(store, nil)), store
}

func TestUpdateCircleHandler_ValidRequest_ReturnsUpdatedCircle(t *testing.T) {
	t.Parallel()

	handler, _ := newUpdateCircleHandler()
	req := withUserPrincipal(newHandlerRequest(http.MethodPut, "/circles/{circleId}", `{"name":"New Name","max_capacity":80}`), unitCreatorID)
	req.SetPathValue("circleId", unitCircleID)
	rec := httptest.NewRecorder()
	handler.UpdateCircle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 — body: %s", rec.Code, rec.Body.String())
	}
	var circle CircleResponse
	if err := json.NewDecoder(rec.Body).Decode(&circle); err != nil {
		t.Fatalf("decode circle response: %v", err)
	}
	if circle.ID != unitCircleID || circle.Name != "New Name" || circle.MaxCapacity != 80 {
		t.Fatalf("unexpected circle projection: %+v", circle)
	}
}

func TestUpdateCircleHandler_ServiceFailures_MapToStatus(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		circleID    string
		body        string
		principalID string
		wantCode    int
		wantErrCode string
		wantField   string
	}{
		{
			name:        "unknown circle returns 404",
			circleID:    "99999999-9999-9999-9999-999999999999",
			body:        `{"name":"New Name"}`,
			principalID: unitCreatorID,
			wantCode:    http.StatusNotFound,
			wantErrCode: httpconst.ErrorCodeNotFound,
		},
		{
			name:        "malformed circle id returns 404",
			circleID:    "not-a-uuid",
			body:        `{"name":"New Name"}`,
			principalID: unitCreatorID,
			wantCode:    http.StatusNotFound,
			wantErrCode: httpconst.ErrorCodeNotFound,
		},
		{
			name:        "non-teacher member returns 403",
			circleID:    unitCircleID,
			body:        `{"name":"New Name"}`,
			principalID: unitStudentID,
			wantCode:    http.StatusForbidden,
			wantErrCode: httpconst.ErrorCodeForbidden,
		},
		{
			name:        "unknown body field returns 400",
			circleID:    unitCircleID,
			body:        `{"bogus":1}`,
			principalID: unitCreatorID,
			wantCode:    http.StatusBadRequest,
			wantErrCode: httpconst.ErrorCodeValidationFailed,
		},
		{
			name:        "blank name returns 400 with field error",
			circleID:    unitCircleID,
			body:        `{"name":"   "}`,
			principalID: unitCreatorID,
			wantCode:    http.StatusBadRequest,
			wantErrCode: httpconst.ErrorCodeValidationFailed,
			wantField:   httpconst.FieldName,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			handler, _ := newUpdateCircleHandler()
			req := withUserPrincipal(newHandlerRequest(http.MethodPut, "/circles/{circleId}", tc.body), tc.principalID)
			req.SetPathValue("circleId", tc.circleID)
			rec := httptest.NewRecorder()
			handler.UpdateCircle(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status: got %d want %d — body: %s", rec.Code, tc.wantCode, rec.Body.String())
			}
			env := decodeErrorEnvelope(t, rec)
			if env.Error.Code != tc.wantErrCode {
				t.Fatalf("error code: got %q want %q", env.Error.Code, tc.wantErrCode)
			}
			if tc.wantField != "" {
				if _, ok := env.Error.Fields[tc.wantField]; !ok {
					t.Fatalf("expected %q field error, got %v", tc.wantField, env.Error.Fields)
				}
			}
		})
	}
}
