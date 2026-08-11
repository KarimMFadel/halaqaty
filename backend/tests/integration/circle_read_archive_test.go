//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func TestCircleReadArchive(t *testing.T) {
	env := setupCircleRoleEnv(t)
	ctx := context.Background()

	circle := env.createCircle(t, "creator", `{"name":"Read Archive Circle","is_private":true}`)
	if err := env.svc.AddStudentMember(ctx, circle.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("add student membership: %v", err)
	}

	detailsResponse := env.getCircle(t, "student", circle.ID)
	var details rbac.CircleResponse
	decodeJSONResponse(t, detailsResponse, &details)
	if details.IsArchived {
		t.Fatalf("circle should be active before archive")
	}
	membersResponse := env.getCircleMembers(t, "student", circle.ID)
	var membersPayload rbac.MemberListResponse
	decodeJSONResponse(t, membersResponse, &membersPayload)
	members := membersPayload.Data
	if len(members) < 2 {
		t.Fatalf("expected retained members, got %d", len(members))
	}
	assertMemberProjection(t, members, env.userIDs["student"], "student", rbac.RoleStudent)
	assertMemberProjection(t, members, env.userIDs["creator"], "creator", rbac.RoleTeacher)

	if response := env.getCircle(t, "outsider", circle.ID); response.Code != http.StatusForbidden {
		t.Fatalf("outsider read details: got %d want %d body=%s", response.Code, http.StatusForbidden, response.Body.String())
	}
	if response := env.getCircleMembers(t, "outsider", circle.ID); response.Code != http.StatusForbidden {
		t.Fatalf("outsider list members: got %d want %d body=%s", response.Code, http.StatusForbidden, response.Body.String())
	}

	if err := env.svc.ArchiveCircle(ctx, env.userIDs["creator"], circle.ID); err != nil {
		t.Fatalf("archive circle: %v", err)
	}

	archivedDetailsResponse := env.getCircle(t, "student", circle.ID)
	var archivedDetails rbac.CircleResponse
	decodeJSONResponse(t, archivedDetailsResponse, &archivedDetails)
	if !archivedDetails.IsArchived {
		t.Fatalf("expected archived circle details to report is_archived=true")
	}
	archivedMembersResponse := env.getCircleMembers(t, "student", circle.ID)
	var archivedMembersPayload rbac.MemberListResponse
	decodeJSONResponse(t, archivedMembersResponse, &archivedMembersPayload)
	archivedMembers := archivedMembersPayload.Data
	if len(archivedMembers) != len(members) {
		t.Fatalf("member retention mismatch after archive: before=%d after=%d", len(members), len(archivedMembers))
	}

	if err := env.svc.AddStudentMember(ctx, circle.ID, env.userIDs["outsider"]); !errors.Is(err, rbac.ErrCircleArchived) {
		t.Fatalf("mutation after archive should fail with ErrCircleArchived, got %v", err)
	}
}

func (e *circleRoleEnv) getCircle(t *testing.T, actor, circleID string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSONRequest(t, e.mux, http.MethodGet, "/circles/"+circleID, "", map[string]string{
		httpconst.HeaderAuthorization: e.tokens[actor],
		httpconst.HeaderSessionID:     e.sessions[actor],
	})
}

func (e *circleRoleEnv) getCircleMembers(t *testing.T, actor, circleID string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSONRequest(t, e.mux, http.MethodGet, "/circles/"+circleID+"/members", "", map[string]string{
		httpconst.HeaderAuthorization: e.tokens[actor],
		httpconst.HeaderSessionID:     e.sessions[actor],
	})
}

func decodeJSONResponse(t *testing.T, response *httptest.ResponseRecorder, destination any) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", response.Code, http.StatusOK, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), destination); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func assertMemberProjection(t *testing.T, members []rbac.CircleMember, userID, displayName, role string) {
	t.Helper()
	for _, member := range members {
		if member.UserID == userID {
			if member.DisplayName != displayName || member.Role != role || member.JoinedAt.IsZero() {
				t.Fatalf("member projection: got %+v want display_name=%q role=%q and non-zero joined_at", member, displayName, role)
			}
			return
		}
	}
	t.Fatalf("member %s not found in %+v", userID, members)
}

func TestCircleReadArchive_PublicDiscoveryStaysRedacted(t *testing.T) {
	env := setupCircleRoleEnv(t)

	circle := env.createCircle(t, "creator", `{"name":"Public Summary Circle","is_private":false}`)
	resp := doJSONRequest(t, env.mux, http.MethodGet, "/circles/discover", "", map[string]string{
		httpconst.HeaderAuthorization: env.tokens["creator"],
		httpconst.HeaderSessionID:     env.sessions["creator"],
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("discover status: got %d want %d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}

	var payload struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode discovery payload: %v", err)
	}
	if len(payload.Data) == 0 {
		t.Fatalf("expected at least one public circle summary")
	}
	found := false
	for _, item := range payload.Data {
		text := strings.ToLower(string(item))
		if strings.Contains(text, circle.ID) {
			found = true
		}
		for _, forbidden := range []string{"invite_code", "invite_link", "user_id", "role", "is_private", "is_archived", "member_count"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("public summary leaked %q: %s", forbidden, item)
			}
		}
	}
	if !found {
		t.Fatalf("expected created public circle in discover response")
	}
}
