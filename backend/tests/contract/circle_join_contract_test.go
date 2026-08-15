//go:build contract

package contract

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func buildPublicJoinCircleRoute(store *circleStoreStub) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, nil))
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), repo)
	mux := http.NewServeMux()
	mux.Handle("POST /circles/{circleId}/join", authMW.Require(http.HandlerFunc(handler.JoinPublicCircle)))
	return mux
}

func TestCircleJoinContract_PublicAndInviteJoins(t *testing.T) {
	for _, tc := range []struct {
		name       string
		method     string
		path       string
		body       string
		seed       func(*circleStoreStub)
		route      func(*circleStoreStub) http.Handler
		wantStatus int
	}{
		{
			name:   "joins active public circle",
			method: http.MethodPost,
			path:   "/circles/" + contractCircleID + "/join",
			seed: func(store *circleStoreStub) {
				store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Public Circle", CreatedAt: time.Now().UTC()}
			},
			route:      buildPublicJoinCircleRoute,
			wantStatus: http.StatusCreated,
		},
		{
			name:   "joins by invite code",
			method: http.MethodPost,
			path:   "/circles/join",
			body:   `{"invite_code":"HLQ-7X2K"}`,
			seed: func(store *circleStoreStub) {
				store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Public Circle", InviteCode: "HLQ-7X2K", CreatedAt: time.Now().UTC()}
			},
			route:      buildJoinCircleRoute,
			wantStatus: http.StatusOK,
		},
		{
			name:       "rejects missing invite code",
			method:     http.MethodPost,
			path:       "/circles/join",
			body:       `{}`,
			route:      buildJoinCircleRoute,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "returns not found for unknown public circle",
			method:     http.MethodPost,
			path:       "/circles/99999999-9999-9999-9999-999999999999/join",
			route:      buildPublicJoinCircleRoute,
			wantStatus: http.StatusNotFound,
		},
		{
			name:   "returns conflict for existing membership",
			method: http.MethodPost,
			path:   "/circles/join",
			body:   `{"invite_code":"HLQ-7X2K"}`,
			seed: func(store *circleStoreStub) {
				store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Public Circle", InviteCode: "HLQ-7X2K", CreatedAt: time.Now().UTC()}
				store.members[contractCircleID] = map[string]string{testLocalUserID: rbac.RoleStudent}
			},
			route:      buildJoinCircleRoute,
			wantStatus: http.StatusConflict,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newCircleStoreStub()
			if tc.seed != nil {
				tc.seed(store)
			}
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
			req.Header.Set(httpconst.HeaderSessionID, testSessionID)
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			rec := httptest.NewRecorder()

			tc.route(store).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
