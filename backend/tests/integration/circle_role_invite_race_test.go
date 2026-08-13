//go:build integration

package integration

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

type recordedCircleAudit struct {
	mu     sync.Mutex
	events []logging.AuditEvent
}

func (a *recordedCircleAudit) Log(_ context.Context, event logging.AuditEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, event)
}

func (a *recordedCircleAudit) count(action string) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	count := 0
	for _, event := range a.events {
		if event.Action == action {
			count++
		}
	}
	return count
}

func TestCircleRoleInviteRace_ConcurrentRoleUpdatesPreserveTeacherAndEmitOneChangeEvent(t *testing.T) {
	env := setupCircleRoleEnv(t)
	audit := &recordedCircleAudit{}
	svc := rbac.NewService(env.repo, audit)
	circle := env.createCircle(t, "creator", `{"name":"Concurrent Role Updates","teacher_user_ids":["`+env.userIDs["teacher_a"]+`","`+env.userIDs["teacher_b"]+`"]}`)

	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, targetID := range []string{env.userIDs["teacher_a"], env.userIDs["teacher_b"]} {
		wg.Add(1)
		go func(targetID string) {
			defer wg.Done()
			_, err := svc.AssignRole(context.Background(), env.userIDs["creator"], circle.ID, targetID, rbac.RoleStudent)
			results <- err
		}(targetID)
	}
	wg.Wait()
	close(results)

	successes := 0
	finalTeacherRejections := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, rbac.ErrFinalTeacher):
			finalTeacherRejections++
		default:
			t.Fatalf("concurrent role update: %v", err)
		}
	}
	if successes != 1 || finalTeacherRejections != 1 {
		t.Fatalf("concurrent role updates: successes=%d final_teacher_rejections=%d, want one each", successes, finalTeacherRejections)
	}
	if got := audit.count(logging.ActionRoleChange); got != 1 {
		t.Fatalf("role-change audit events: got %d want 1", got)
	}
}

func TestCircleRoleInviteRace_ConcurrentInviteRefreshesLeaveOnlyFinalCodeActive(t *testing.T) {
	env := setupCircleRoleEnv(t)
	audit := &recordedCircleAudit{}
	svc := rbac.NewService(env.repo, audit)
	circle := env.createCircle(t, "creator", `{"name":"Concurrent Invite Refreshes"}`)

	codes := make(chan string, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, err := svc.RefreshInviteCode(context.Background(), env.userIDs["creator"], circle.ID)
			if err != nil {
				errs <- err
				return
			}
			codes <- code
		}()
	}
	wg.Wait()
	close(codes)
	close(errs)

	for err := range errs {
		t.Fatalf("refresh invite: %v", err)
	}
	refreshed := make(map[string]struct{}, 2)
	for code := range codes {
		refreshed[code] = struct{}{}
	}
	if len(refreshed) != 2 {
		t.Fatalf("refresh codes: got %d unique codes want 2", len(refreshed))
	}
	current, err := env.repo.FindCircleByID(context.Background(), circle.ID)
	if err != nil {
		t.Fatalf("read refreshed circle: %v", err)
	}
	if _, ok := refreshed[current.InviteCode]; !ok {
		t.Fatalf("current invite code %q was not returned by a completed refresh", current.InviteCode)
	}
	if current.InviteCode == circle.InviteCode {
		t.Fatalf("old invite code %q remained active", circle.InviteCode)
	}
	if got := audit.count(logging.ActionInviteRefresh); got != 2 {
		t.Fatalf("invite-refresh audit events: got %d want 2", got)
	}
}

func TestCircleRoleInviteRace_ConcurrentArchivesEmitOneAuditEvent(t *testing.T) {
	env := setupCircleRoleEnv(t)
	audit := &recordedCircleAudit{}
	svc := rbac.NewService(env.repo, audit)
	circle := env.createCircle(t, "creator", `{"name":"Concurrent Archive"}`)

	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- svc.ArchiveCircle(context.Background(), env.userIDs["creator"], circle.ID)
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("archive circle: %v", err)
		}
	}
	if got := audit.count(logging.ActionCircleArchive); got != 1 {
		t.Fatalf("archive audit events: got %d want 1", got)
	}
}

func TestCircleRoleInviteRace_ArchiveAgainstRefreshAndRemovalCompletesSafely(t *testing.T) {
	for _, mutation := range []struct {
		name string
		run  func(context.Context, *rbac.Service, *circleRoleEnv, rbac.CircleResponse) error
	}{
		{
			name: "invite refresh",
			run: func(ctx context.Context, service *rbac.Service, env *circleRoleEnv, circle rbac.CircleResponse) error {
				_, err := service.RefreshInviteCode(ctx, env.userIDs["creator"], circle.ID)
				return err
			},
		},
		{
			name: "member removal",
			run: func(ctx context.Context, service *rbac.Service, env *circleRoleEnv, circle rbac.CircleResponse) error {
				return service.RemoveMember(ctx, env.userIDs["creator"], circle.ID, env.userIDs["student"])
			},
		},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			env := setupCircleRoleEnv(t)
			audit := &recordedCircleAudit{}
			service := rbac.NewService(env.repo, audit)
			circle := env.createCircle(t, "creator", `{"name":"Archive Mutation Race"}`)
			if err := service.AddStudentMember(context.Background(), circle.ID, env.userIDs["student"]); err != nil {
				t.Fatalf("add student: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			start := make(chan struct{})
			errs := make(chan error, 2)
			go func() {
				<-start
				errs <- service.ArchiveCircle(ctx, env.userIDs["creator"], circle.ID)
			}()
			go func() {
				<-start
				errs <- mutation.run(ctx, service, env, circle)
			}()
			close(start)

			for range 2 {
				err := <-errs
				if err != nil && !errors.Is(err, rbac.ErrCircleArchived) {
					t.Fatalf("concurrent archive mutation: %v", err)
				}
			}
			archived, err := env.repo.FindCircleByID(ctx, circle.ID)
			if err != nil || !archived.IsArchived {
				t.Fatalf("circle not archived: circle=%+v err=%v", archived, err)
			}
		})
	}
}
