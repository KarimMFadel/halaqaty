//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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

	details, err := env.svc.GetCircle(ctx, env.userIDs["student"], circle.ID)
	if err != nil {
		t.Fatalf("student read circle details: %v", err)
	}
	if details.IsArchived {
		t.Fatalf("circle should be active before archive")
	}
	members, err := env.svc.ListMembers(ctx, env.userIDs["student"], circle.ID)
	if err != nil {
		t.Fatalf("student list members: %v", err)
	}
	if len(members) < 2 {
		t.Fatalf("expected retained members, got %d", len(members))
	}

	_, err = env.svc.GetCircle(ctx, env.userIDs["outsider"], circle.ID)
	if !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("outsider read details: got %v want %v", err, rbac.ErrForbidden)
	}
	_, err = env.svc.ListMembers(ctx, env.userIDs["outsider"], circle.ID)
	if !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("outsider list members: got %v want %v", err, rbac.ErrForbidden)
	}

	if err := env.svc.ArchiveCircle(ctx, env.userIDs["creator"], circle.ID); err != nil {
		t.Fatalf("archive circle: %v", err)
	}

	archivedDetails, err := env.svc.GetCircle(ctx, env.userIDs["student"], circle.ID)
	if err != nil {
		t.Fatalf("student read archived details: %v", err)
	}
	if !archivedDetails.IsArchived {
		t.Fatalf("expected archived circle details to report is_archived=true")
	}
	archivedMembers, err := env.svc.ListMembers(ctx, env.userIDs["student"], circle.ID)
	if err != nil {
		t.Fatalf("student list archived members: %v", err)
	}
	if len(archivedMembers) != len(members) {
		t.Fatalf("member retention mismatch after archive: before=%d after=%d", len(members), len(archivedMembers))
	}

	if err := env.svc.AddStudentMember(ctx, circle.ID, env.userIDs["outsider"]); !errors.Is(err, rbac.ErrCircleArchived) {
		t.Fatalf("mutation after archive should fail with ErrCircleArchived, got %v", err)
	}
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
