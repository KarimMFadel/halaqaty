//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func (e *circleRoleEnv) joinCircle(t *testing.T, actor, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSONRequest(t, e.mux, http.MethodPost, path, body, map[string]string{
		httpconst.HeaderAuthorization: e.tokens[actor],
		httpconst.HeaderSessionID:     e.sessions[actor],
		httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
	})
}

func TestCircleJoin_PublicPrivateAndInvite(t *testing.T) {
	env := setupCircleRoleEnv(t)

	public := env.createCircle(t, "creator", `{"name":"Public Join Circle"}`)
	if resp := env.joinCircle(t, "student", "/circles/"+public.ID+"/join", ""); resp.Code != http.StatusCreated {
		t.Fatalf("public join: got %d want %d body=%s", resp.Code, http.StatusCreated, resp.Body.String())
	}

	private := env.createCircle(t, "creator2", `{"name":"Private Join Circle","is_private":true}`)
	if resp := env.joinCircle(t, "outsider", "/circles/"+private.ID+"/join", ""); resp.Code != http.StatusConflict {
		t.Fatalf("private join: got %d want %d body=%s", resp.Code, http.StatusConflict, resp.Body.String())
	}

	invite := env.createCircle(t, "creator2", `{"name":"Invite Join Circle"}`)
	if resp := env.joinCircle(t, "outsider", "/circles/join", fmt.Sprintf(`{"invite_code":%q}`, invite.InviteCode)); resp.Code != http.StatusOK {
		t.Fatalf("invite join: got %d want %d body=%s", resp.Code, http.StatusOK, resp.Body.String())
	}
}

func TestCircleJoin_RejectsDuplicateFullArchivedAndSixthMembership(t *testing.T) {
	env := setupCircleRoleEnv(t)

	circle := env.createCircle(t, "creator", `{"name":"Duplicate Join Circle"}`)
	if err := env.svc.AddStudentMember(context.Background(), circle.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("seed duplicate membership: %v", err)
	}
	if resp := env.joinCircle(t, "student", "/circles/"+circle.ID+"/join", ""); resp.Code != http.StatusConflict {
		t.Fatalf("duplicate join: got %d want %d body=%s", resp.Code, http.StatusConflict, resp.Body.String())
	}

	full := env.createCircle(t, "creator2", `{"name":"Full Join Circle","max_capacity":2}`)
	for _, user := range []string{"student", "backup_sup"} {
		if err := env.svc.AddStudentMember(context.Background(), full.ID, env.userIDs[user]); err != nil {
			t.Fatalf("seed full circle with %s: %v", user, err)
		}
	}
	if resp := env.joinCircle(t, "outsider", "/circles/"+full.ID+"/join", ""); resp.Code != http.StatusConflict {
		t.Fatalf("full join: got %d want %d body=%s", resp.Code, http.StatusConflict, resp.Body.String())
	}

	archived := env.createCircle(t, "creator2", `{"name":"Archived Join Circle"}`)
	if _, err := env.pool.Exec(context.Background(), `UPDATE circles SET is_archived = true WHERE id = $1`, archived.ID); err != nil {
		t.Fatalf("archive circle: %v", err)
	}
	if resp := env.joinCircle(t, "outsider", "/circles/"+archived.ID+"/join", ""); resp.Code != http.StatusConflict {
		t.Fatalf("archived join: got %d want %d body=%s", resp.Code, http.StatusConflict, resp.Body.String())
	}

	for i := 0; i < 5; i++ {
		memberCircle := env.createCircle(t, "creator", fmt.Sprintf(`{"name":"Membership Limit %d"}`, i))
		if err := env.svc.AddStudentMember(context.Background(), memberCircle.ID, env.userIDs["outsider"]); err != nil {
			t.Fatalf("seed active membership %d: %v", i+1, err)
		}
	}
	sixth := env.createCircle(t, "creator2", `{"name":"Sixth Membership"}`)
	if resp := env.joinCircle(t, "outsider", "/circles/"+sixth.ID+"/join", ""); resp.Code != http.StatusConflict {
		t.Fatalf("sixth membership: got %d want %d body=%s", resp.Code, http.StatusConflict, resp.Body.String())
	}
}

func TestCircleJoin_ConcurrentCapacityIsAtomic(t *testing.T) {
	env := setupCircleRoleEnv(t)
	circle := env.createCircle(t, "creator", `{"name":"Atomic Capacity","max_capacity":2}`)
	if err := env.svc.AddStudentMember(context.Background(), circle.ID, env.userIDs["backup_sup"]); err != nil {
		t.Fatalf("seed occupied student slot: %v", err)
	}

	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, actor := range []string{"student", "outsider"} {
		wg.Add(1)
		go func(actor string) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/circles/"+circle.ID+"/join", bytes.NewBuffer(nil))
			req.Header.Set(httpconst.HeaderAuthorization, env.tokens[actor])
			req.Header.Set(httpconst.HeaderSessionID, env.sessions[actor])
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			rec := httptest.NewRecorder()
			env.mux.ServeHTTP(rec, req)
			statuses <- rec.Code
		}(actor)
	}
	wg.Wait()
	close(statuses)

	got := make([]int, 0, 2)
	for status := range statuses {
		got = append(got, status)
	}
	sort.Ints(got)
	want := []int{http.StatusCreated, http.StatusConflict}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("concurrent joins: got %v want %v", got, want)
	}

	members, err := env.repo.ListMembers(context.Background(), circle.ID)
	if err != nil {
		t.Fatalf("list members after concurrent joins: %v", err)
	}
	if len(members) != 3 {
		t.Fatalf("member count after concurrent joins: got %d want 3", len(members))
	}
}
