//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func TestCircleInvitationHTTP_ReplacementAcceptanceAndRevocation(t *testing.T) {
	env := setupCircleRoleEnv(t)
	env.repo.SetNotificationRepository(notifications.NewRepository(env.pool))
	circle := env.createCircle(t, "creator", `{"name":"Invitations","backup_supervisor_user_id":"`+env.userIDs["backup_sup"]+`"}`)
	first := sendCircleInvitation(t, env, "creator", circle.ID, env.userIDs["teacher_a"], rbac.RoleTeacher, http.StatusCreated)
	second := sendCircleInvitation(t, env, "creator", circle.ID, env.userIDs["teacher_a"], rbac.RoleTeacher, http.StatusCreated)
	if first.ID == second.ID {
		t.Fatal("replacement invitation must have a new ID")
	}
	assertInvitationState(t, env, first.ID, "superseded")
	assertInvitationState(t, env, second.ID, "pending")
	assertInvitationNotificationCount(t, env, env.userIDs["teacher_a"], 2)

	wrongRecipient := doJSONRequest(t, env.mux, http.MethodPost, "/circle-invitations/"+second.ID+"/accept", "", circleAuthHeaders(env, "student"))
	if wrongRecipient.Code != http.StatusNotFound {
		t.Fatalf("non-recipient acceptance status: got %d want 404 body=%s", wrongRecipient.Code, wrongRecipient.Body.String())
	}
	acceptedResponse := doJSONRequest(t, env.mux, http.MethodPost, "/circle-invitations/"+second.ID+"/accept", "", circleAuthHeaders(env, "teacher_a"))
	if acceptedResponse.Code != http.StatusOK {
		t.Fatalf("recipient acceptance status: got %d want %d body=%s", acceptedResponse.Code, http.StatusOK, acceptedResponse.Body.String())
	}
	if role, err := env.repo.RoleInCircle(context.Background(), circle.ID, env.userIDs["teacher_a"]); err != nil || role != rbac.RoleTeacher {
		t.Fatalf("accepted role: got %q err=%v want teacher", role, err)
	}
	assertInvitationState(t, env, second.ID, "accepted")
	replayed := doJSONRequest(t, env.mux, http.MethodPost, "/circle-invitations/"+second.ID+"/accept", "", circleAuthHeaders(env, "teacher_a"))
	if replayed.Code != http.StatusConflict {
		t.Fatalf("replay status: got %d want %d body=%s", replayed.Code, http.StatusConflict, replayed.Body.String())
	}

	revoked := sendCircleInvitation(t, env, "creator", circle.ID, env.userIDs["outsider"], rbac.RoleStudent, http.StatusCreated)
	response := doJSONRequest(t, env.mux, http.MethodDelete, "/circles/"+circle.ID+"/invitations/"+revoked.ID, "", circleAuthHeaders(env, "backup_sup"))
	if response.Code != http.StatusNoContent {
		t.Fatalf("manager revoke status: got %d want %d body=%s", response.Code, http.StatusNoContent, response.Body.String())
	}
	assertInvitationState(t, env, revoked.ID, "revoked")
}

func TestCircleInvitationHTTP_StudentAuthorityAndRecipientIsolation(t *testing.T) {
	env := setupCircleRoleEnv(t)
	circle := env.createCircle(t, "creator", `{"name":"Student Invitations"}`)
	if err := env.svc.AddStudentMember(context.Background(), circle.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("add student: %v", err)
	}
	teacherInvite := doJSONRequest(t, env.mux, http.MethodPost, "/circles/"+circle.ID+"/invitations",
		`{"recipient_user_id":"`+env.userIDs["outsider"]+`","role":"teacher"}`, circleAuthHeaders(env, "student"))
	if teacherInvite.Code != http.StatusForbidden {
		t.Fatalf("student teacher-invite status: got %d want 403 body=%s", teacherInvite.Code, teacherInvite.Body.String())
	}
	studentInvite := sendCircleInvitation(t, env, "student", circle.ID, env.userIDs["outsider"], rbac.RoleStudent, http.StatusCreated)
	managerList := doJSONRequest(t, env.mux, http.MethodGet, "/circles/"+circle.ID+"/invitations", "", circleAuthHeaders(env, "creator"))
	studentList := doJSONRequest(t, env.mux, http.MethodGet, "/circles/"+circle.ID+"/invitations", "", circleAuthHeaders(env, "student"))
	if managerList.Code != http.StatusOK || studentList.Code != http.StatusOK {
		t.Fatalf("invitation list statuses: manager=%d student=%d", managerList.Code, studentList.Code)
	}
	if got := invitationIDs(t, managerList.Body.Bytes()); len(got) != 1 || got[0] != studentInvite.ID {
		t.Fatalf("manager invitation list: %v", got)
	}
	if got := invitationIDs(t, studentList.Body.Bytes()); len(got) != 1 || got[0] != studentInvite.ID {
		t.Fatalf("student should see only own invitation: %v", got)
	}
	myList := doJSONRequest(t, env.mux, http.MethodGet, "/users/me/circle-invitations", "", circleAuthHeaders(env, "outsider"))
	if got := invitationIDs(t, myList.Body.Bytes()); len(got) != 1 || got[0] != studentInvite.ID {
		t.Fatalf("recipient inbox did not isolate addressed invitation: %v", got)
	}
}

func TestCircleInvitationService_ConcurrentAcceptanceIsSingleUse(t *testing.T) {
	env := setupCircleRoleEnv(t)
	circle := env.createCircle(t, "creator", `{"name":"Concurrent Acceptance"}`)
	invitation := sendCircleInvitation(t, env, "creator", circle.ID, env.userIDs["teacher_a"], rbac.RoleTeacher, http.StatusCreated)
	results := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := env.svc.AcceptCircleInvitation(context.Background(), env.userIDs["teacher_a"], invitation.ID)
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, rbac.ErrInvitationConflict):
			conflicts++
		default:
			t.Fatalf("concurrent acceptance: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("acceptance outcomes: successes=%d conflicts=%d, want one each", successes, conflicts)
	}
}

func sendCircleInvitation(t *testing.T, env *circleRoleEnv, actor, circleID, recipientID, role string, status int) rbac.CircleInvitation {
	t.Helper()
	response := doJSONRequest(t, env.mux, http.MethodPost, "/circles/"+circleID+"/invitations",
		`{"recipient_user_id":"`+recipientID+`","role":"`+role+`"}`, circleAuthHeaders(env, actor))
	if response.Code != status {
		t.Fatalf("send invitation status: got %d want %d body=%s", response.Code, status, response.Body.String())
	}
	var invitation rbac.CircleInvitation
	if err := json.Unmarshal(response.Body.Bytes(), &invitation); err != nil {
		t.Fatalf("decode circle invitation: %v", err)
	}
	return invitation
}

func circleAuthHeaders(env *circleRoleEnv, actor string) map[string]string {
	return map[string]string{
		httpconst.HeaderAuthorization: env.tokens[actor],
		httpconst.HeaderSessionID:     env.sessions[actor],
		httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
	}
}

func assertInvitationState(t *testing.T, env *circleRoleEnv, invitationID, want string) {
	t.Helper()
	var state string
	if err := env.pool.QueryRow(context.Background(), `SELECT state FROM circle_invitations WHERE id = $1::uuid`, invitationID).Scan(&state); err != nil {
		t.Fatalf("read invitation state: %v", err)
	}
	if state != want {
		t.Fatalf("invitation state: got %q want %q", state, want)
	}
}

func assertInvitationNotificationCount(t *testing.T, env *circleRoleEnv, recipientID string, want int) {
	t.Helper()
	var count int
	if err := env.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM notifications WHERE user_id = $1::uuid AND type = 'circle_invitation'
	`, recipientID).Scan(&count); err != nil {
		t.Fatalf("count invitation notifications: %v", err)
	}
	if count != want {
		t.Fatalf("invitation notifications: got %d want %d", count, want)
	}
}

func invitationIDs(t *testing.T, payload []byte) []string {
	t.Helper()
	var list struct {
		Data []rbac.CircleInvitation `json:"data"`
	}
	if err := json.Unmarshal(payload, &list); err != nil {
		t.Fatalf("decode invitation list: %v", err)
	}
	ids := make([]string, 0, len(list.Data))
	for _, invitation := range list.Data {
		ids = append(ids, invitation.ID)
	}
	return ids
}
