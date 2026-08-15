//go:build contract

package contract

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// TestCircleRoleManagementContract_ArchivedCircleRejectsRoleChange protects the
// active-member contract: archived circles retain history but accept no role mutations.
func TestCircleRoleManagementContract_ArchivedCircleRejectsRoleChange(t *testing.T) {
	store := newCircleStoreStub()
	store.actorRole = rbac.RoleTeacher
	store.circles[contractCircleID] = rbac.Circle{
		ID:         contractCircleID,
		Name:       "Archived Circle",
		IsArchived: true,
		CreatedAt:  time.Now().UTC(),
	}
	store.members[contractCircleID] = map[string]string{
		testLocalUserID:   rbac.RoleTeacher,
		contractStudentID: rbac.RoleStudent,
	}

	req := httptest.NewRequest(
		http.MethodPut,
		"/circles/"+contractCircleID+"/members/"+contractStudentID+"/role",
		bytes.NewBufferString(`{"role":"supervisor"}`),
	)
	req.SetPathValue("circleId", contractCircleID)
	req.SetPathValue("userId", contractStudentID)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)

	rec := httptest.NewRecorder()
	buildAssignRoleRoute(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if got := store.members[contractCircleID][contractStudentID]; got != rbac.RoleStudent {
		t.Fatalf("role: got %q, want %q", got, rbac.RoleStudent)
	}
}
