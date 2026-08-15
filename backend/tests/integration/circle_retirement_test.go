//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func TestCircleRetirement_RetainsHistoryAndBlocksMutations(t *testing.T) {
	env := setupCircleRoleEnv(t)
	ctx := context.Background()
	audit := &recordedCircleAudit{}
	service := rbac.NewService(env.repo, audit)
	circle := env.createCircle(t, "creator", `{"name":"Retirement History"}`)
	if err := service.AddStudentMember(ctx, circle.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("add student: %v", err)
	}

	before, err := env.repo.ListMembers(ctx, circle.ID)
	if err != nil {
		t.Fatalf("list members before archive: %v", err)
	}
	if err := service.ArchiveCircle(ctx, env.userIDs["creator"], circle.ID); err != nil {
		t.Fatalf("archive circle: %v", err)
	}

	archived, err := env.repo.FindCircleByID(ctx, circle.ID)
	if err != nil || !archived.IsArchived {
		t.Fatalf("retained archived circle: circle=%+v err=%v", archived, err)
	}
	after, err := env.repo.ListMembers(ctx, circle.ID)
	if err != nil {
		t.Fatalf("list members after archive: %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("retained memberships: before=%d after=%d", len(before), len(after))
	}

	assertArchivedMutation := func(name string, err error) {
		t.Helper()
		if !errors.Is(err, rbac.ErrCircleArchived) {
			t.Errorf("%s: got %v want ErrCircleArchived", name, err)
		}
	}
	_, err = service.JoinPublicCircle(ctx, env.userIDs["outsider"], circle.ID)
	assertArchivedMutation("join", err)
	_, err = service.UpdateCircle(ctx, env.userIDs["creator"], circle.ID, rbac.UpdateCircleRequest{Name: stringPointer("Renamed")})
	assertArchivedMutation("settings", err)
	_, err = service.AssignRole(ctx, env.userIDs["creator"], circle.ID, env.userIDs["student"], rbac.RoleSupervisor)
	assertArchivedMutation("role change", err)
	err = service.RemoveMember(ctx, env.userIDs["creator"], circle.ID, env.userIDs["student"])
	assertArchivedMutation("member removal", err)
	if got := audit.count(logging.ActionCircleArchive); got != 1 {
		t.Fatalf("archive audit events: got %d want 1", got)
	}
}

func TestCircleMutations_RejectMalformedIDs(t *testing.T) {
	env := setupCircleRoleEnv(t)
	ctx := context.Background()
	service := rbac.NewService(env.repo, &recordedCircleAudit{})
	actor := env.userIDs["creator"]

	if _, err := service.UpdateCircle(ctx, actor, "not-a-uuid", rbac.UpdateCircleRequest{}); !errors.Is(err, rbac.ErrCircleNotFound) {
		t.Fatalf("update malformed circle: got %v want ErrCircleNotFound", err)
	}
	if _, err := service.RefreshInviteCode(ctx, actor, "not-a-uuid"); !errors.Is(err, rbac.ErrCircleNotFound) {
		t.Fatalf("refresh malformed circle: got %v want ErrCircleNotFound", err)
	}
	if err := service.ArchiveCircle(ctx, actor, "not-a-uuid"); !errors.Is(err, rbac.ErrCircleNotFound) {
		t.Fatalf("archive malformed circle: got %v want ErrCircleNotFound", err)
	}
	if err := service.RemoveMember(ctx, actor, "not-a-uuid", env.userIDs["student"]); !errors.Is(err, rbac.ErrCircleNotFound) {
		t.Fatalf("remove from malformed circle: got %v want ErrCircleNotFound", err)
	}
	circle := env.createCircle(t, "creator", `{"name":"Malformed Target"}`)
	if err := service.RemoveMember(ctx, actor, circle.ID, "not-a-uuid"); !errors.Is(err, rbac.ErrMemberNotFound) {
		t.Fatalf("remove malformed member: got %v want ErrMemberNotFound", err)
	}
}

func stringPointer(value string) *string { return &value }
