//go:build contract

package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func buildCircleRetirementRoute(store *circleStoreStub) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, nil))
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), repo)
	mux := http.NewServeMux()
	mux.Handle("DELETE /circles/{circleId}", authMW.Require(http.HandlerFunc(handler.ArchiveCircle)))
	mux.Handle("GET /circles/{circleId}", authMW.Require(http.HandlerFunc(handler.GetCircle)))
	return mux
}

func TestCircleRetirementContract_ArchiveOnlyDelete(t *testing.T) {
	store := newCircleStoreStub()
	store.circles[contractCircleID] = rbac.Circle{
		ID:        contractCircleID,
		Name:      "Retired Circle",
		CreatedAt: time.Now().UTC(),
	}
	store.members[contractCircleID] = map[string]string{testLocalUserID: rbac.RoleTeacher}
	route := buildCircleRetirementRoute(store)

	for _, name := range []string{"teacher archives the circle", "archive is idempotent"} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodDelete, "/circles/"+contractCircleID, nil)
			req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
			req.Header.Set(httpconst.HeaderSessionID, testSessionID)
			rec := httptest.NewRecorder()

			route.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
			}
			circle, exists := store.circles[contractCircleID]
			if !exists || !circle.IsArchived {
				t.Fatal("DELETE must retain the circle and mark it archived")
			}
			if role := store.members[contractCircleID][testLocalUserID]; role != rbac.RoleTeacher {
				t.Fatalf("archive must retain membership history: got role %q", role)
			}
		})
	}

	readReq := httptest.NewRequest(http.MethodGet, "/circles/"+contractCircleID, nil)
	readReq.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	readReq.Header.Set(httpconst.HeaderSessionID, testSessionID)
	readRec := httptest.NewRecorder()
	route.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("archived read status: got %d, want %d body=%s", readRec.Code, http.StatusOK, readRec.Body.String())
	}
}

func TestCircleRetirementContract_NonTeacherCannotArchive(t *testing.T) {
	store := newCircleStoreStub()
	store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Active Circle", CreatedAt: time.Now().UTC()}
	store.members[contractCircleID] = map[string]string{testLocalUserID: rbac.RoleSupervisor}

	req := httptest.NewRequest(http.MethodDelete, "/circles/"+contractCircleID, nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()
	buildCircleRetirementRoute(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
	var env phttp.ErrorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != httpconst.ErrorCodeForbidden {
		t.Fatalf("error code: got %q, want %q", env.Error.Code, httpconst.ErrorCodeForbidden)
	}
	if store.circles[contractCircleID].IsArchived {
		t.Fatal("non-teacher archive attempt must not retire the circle")
	}
}
