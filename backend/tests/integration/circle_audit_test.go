//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func TestCircleAudit_RecordsCompleteMutationLifecycle(t *testing.T) {
	env := setupCircleRoleEnv(t)
	audit := &recordedCircleAudit{}
	service := rbac.NewService(env.repo, audit)
	ctx := context.Background()

	circle, err := service.CreateCircle(ctx, env.userIDs["creator"], rbac.CreateCircleRequest{Name: "Audit Lifecycle"})
	if err != nil {
		t.Fatalf("create circle: %v", err)
	}
	if _, err := service.JoinPublicCircle(ctx, env.userIDs["student"], circle.ID); err != nil {
		t.Fatalf("join circle: %v", err)
	}
	if _, err := service.AssignRole(ctx, env.userIDs["creator"], circle.ID, env.userIDs["student"], rbac.RoleSupervisor); err != nil {
		t.Fatalf("assign role: %v", err)
	}
	if err := service.RemoveMember(ctx, env.userIDs["creator"], circle.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if _, err := service.RefreshInviteCode(ctx, env.userIDs["creator"], circle.ID); err != nil {
		t.Fatalf("refresh invite: %v", err)
	}
	if err := service.ArchiveCircle(ctx, env.userIDs["creator"], circle.ID); err != nil {
		t.Fatalf("archive circle: %v", err)
	}

	for _, action := range []string{
		logging.ActionCircleCreate,
		logging.ActionCircleJoin,
		logging.ActionRoleChange,
		logging.ActionMemberRemoval,
		logging.ActionInviteRefresh,
		logging.ActionCircleArchive,
	} {
		if got := audit.count(action); got != 1 {
			t.Errorf("%s audit events: got %d want 1", action, got)
		}
	}
}
