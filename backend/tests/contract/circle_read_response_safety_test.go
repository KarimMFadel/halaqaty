//go:build contract

package contract

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func TestCircleReadResponseSafety_MemberListDoesNotExposeInviteSecrets(t *testing.T) {
	store := newCircleStoreStub()
	store.circles[contractCircleID] = rbac.Circle{
		ID:         contractCircleID,
		Name:       "Safety Circle",
		InviteCode: "HLQ-7X2K",
		CreatedAt:  time.Now().UTC(),
	}
	store.members[contractCircleID] = map[string]string{testLocalUserID: rbac.RoleTeacher}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/circles/"+contractCircleID+"/members", nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()

	buildGetCircleRoute(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	for _, forbidden := range []string{"invite_code", "invite_link", "teacher_id"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), forbidden) {
			t.Fatalf("member list leaked %q: %s", forbidden, rec.Body.String())
		}
	}
}

func TestCircleReadResponseSafety_NonMemberCannotReadPrivateMemberData(t *testing.T) {
	store := newCircleStoreStub()
	store.circles[contractCircleID] = rbac.Circle{
		ID:        contractCircleID,
		Name:      "Private Circle",
		IsPrivate: true,
		CreatedAt: time.Now().UTC(),
	}
	store.members[contractCircleID] = map[string]string{contractTeacherAID: rbac.RoleTeacher}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/circles/"+contractCircleID+"/members", nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()

	buildGetCircleRoute(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
	}
}

func TestCircleReadResponseSafety_ArchivedHistoryRemainsVisibleToMembers(t *testing.T) {
	store := newCircleStoreStub()
	store.circles[contractCircleID] = rbac.Circle{
		ID:         contractCircleID,
		Name:       "Archived Circle",
		InviteCode: "HLQ-7X2K",
		IsArchived: true,
		CreatedAt:  time.Now().UTC(),
	}
	store.members[contractCircleID] = map[string]string{
		testLocalUserID:    rbac.RoleStudent,
		contractTeacherAID: rbac.RoleTeacher,
	}

	for _, path := range []string{
		"/api/v1/circles/" + contractCircleID,
		"/api/v1/circles/" + contractCircleID + "/members",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
		req.Header.Set(httpconst.HeaderSessionID, testSessionID)
		rec := httptest.NewRecorder()

		buildGetCircleRoute(store).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("path %s status: got %d want %d body=%s", path, rec.Code, http.StatusOK, rec.Body.String())
		}
	}
}
