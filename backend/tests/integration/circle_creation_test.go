//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func TestCircleCreation_AtomicInitialMembershipPersistence(t *testing.T) {
	env := setupCircleRoleEnv(t)
	circle := env.createCircle(t, "creator", `{"name":"Atomic Circle","teacher_user_ids":["`+env.userIDs["teacher_a"]+`","`+env.userIDs["teacher_b"]+`"],"backup_supervisor_user_id":"`+env.userIDs["backup_sup"]+`"}`)

	members, err := env.repo.ListMembers(context.Background(), circle.ID)
	if err != nil {
		t.Fatalf("list persisted members: %v", err)
	}
	roles := make(map[string]string, len(members))
	for _, member := range members {
		roles[member.UserID] = member.Role
	}
	want := map[string]string{
		env.userIDs["creator"]:    rbac.RoleSupervisor,
		env.userIDs["teacher_a"]:  rbac.RoleTeacher,
		env.userIDs["teacher_b"]:  rbac.RoleTeacher,
		env.userIDs["backup_sup"]: rbac.RoleSupervisor,
	}
	if len(roles) != len(want) {
		t.Fatalf("persisted member count: got %d want %d (%v)", len(roles), len(want), roles)
	}
	for userID, role := range want {
		if roles[userID] != role {
			t.Fatalf("member %s role: got %q want %q", userID, roles[userID], role)
		}
	}
}

func TestCircleCreation_RollsBackWhenInitialMembershipInsertFails(t *testing.T) {
	env := setupCircleRoleEnv(t)
	ctx := context.Background()
	_, err := env.pool.Exec(ctx, `
CREATE FUNCTION fail_circle_teacher_insert() RETURNS trigger AS $$
BEGIN
  IF NEW.role = 'teacher' THEN RAISE EXCEPTION 'forced teacher insert failure'; END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER fail_circle_teacher_insert
BEFORE INSERT ON circle_members
FOR EACH ROW EXECUTE FUNCTION fail_circle_teacher_insert();`)
	if err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	resp := doJSONRequest(t, env.mux, http.MethodPost, "/circles",
		`{"name":"Rollback Circle","teacher_user_ids":["`+env.userIDs["teacher_a"]+`"]}`,
		map[string]string{
			httpconst.HeaderAuthorization: env.tokens["creator"],
			httpconst.HeaderSessionID:     env.sessions["creator"],
			httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
		},
	)
	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d want %d body=%s", resp.Code, http.StatusInternalServerError, resp.Body.String())
	}

	var count int
	if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM circles WHERE name = 'Rollback Circle'`).Scan(&count); err != nil {
		t.Fatalf("count rolled back circles: %v", err)
	}
	if count != 0 {
		t.Fatalf("transaction left %d circle rows after membership failure", count)
	}
}
