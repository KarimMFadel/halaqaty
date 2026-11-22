//go:build integration

package sessions

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
)

func TestSessionLifecycleNotificationsUseTransactionalEligibleRecipients(t *testing.T) {
	ctx := context.Background()
	repo := newSessionRepository(t)
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test")
	}
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "000021_notification_system.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply F-008 migration: %v", err)
	}
	noticeRepo := notifications.NewRepository(repo.pool)
	repo.SetNotificationRepository(noticeRepo)
	teacher := seedRepoUser(t, repo, "notice-teacher")
	student := seedRepoUser(t, repo, "notice-student")
	circle := seedRepoCircle(t, repo, teacher)
	for _, userID := range []string{teacher, student} {
		if _, err := repo.pool.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1::uuid, $2::uuid, $3)`, circle, userID, map[bool]string{true: "teacher", false: "student"}[userID == teacher]); err != nil {
			t.Fatal(err)
		}
	}
	created, err := repo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatal(err)
	}
	started, err := repo.StartSession(ctx, created.ID, MediaRoomRef("notification-room"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.JoinSession(ctx, started.ID, teacher); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.JoinSession(ctx, started.ID, student); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.EndSession(ctx, started.ID, EndReasonManual); err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{teacher, student} {
		page, err := noticeRepo.List(ctx, userID, nil, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 2 || page.UnreadCount != 2 {
			t.Fatalf("user %s notifications = %+v; want one start and one end", userID, page.Items)
		}
		found := map[notifications.EventType]bool{}
		for _, item := range page.Items {
			found[item.Type] = item.SourceRef.ID.String() == started.ID
		}
		if !found[notifications.EventSessionStarted] || !found[notifications.EventSessionEnded] {
			t.Fatalf("user %s lifecycle notifications = %+v", userID, found)
		}
	}
	// Repeated state-transition attempts cannot create duplicate source notices.
	if _, err := repo.EndSession(ctx, started.ID, EndReasonManual); err == nil {
		t.Fatal("duplicate session end unexpectedly succeeded")
	}
	for _, userID := range []string{teacher, student} {
		page, err := noticeRepo.List(ctx, userID, nil, 10)
		if err != nil || len(page.Items) != 2 {
			t.Fatalf("user %s notices after duplicate end = %d, error=%v", userID, len(page.Items), err)
		}
	}
}

// Recipient gates from the matrix rows: session.started reaches active circle
// members (including one who never joins), session.ended reaches only joined
// participants — and soft-deleted former members receive neither.
func TestSessionLifecycleNoticesSeparateMembersFromParticipants(t *testing.T) {
	ctx := context.Background()
	repo := newSessionRepository(t)
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test")
	}
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "000021_notification_system.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply F-008 migration: %v", err)
	}
	noticeRepo := notifications.NewRepository(repo.pool)
	repo.SetNotificationRepository(noticeRepo)
	teacher := seedRepoUser(t, repo, "scope-teacher")
	joiner := seedRepoUser(t, repo, "scope-joiner")
	bystander := seedRepoUser(t, repo, "scope-bystander")
	former := seedRepoUser(t, repo, "scope-former")
	circle := seedRepoCircle(t, repo, teacher)
	for _, member := range []struct {
		id   string
		role string
	}{{teacher, "teacher"}, {joiner, "student"}, {bystander, "student"}, {former, "student"}} {
		if _, err := repo.pool.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1::uuid, $2::uuid, $3)`, circle, member.id, member.role); err != nil {
			t.Fatal(err)
		}
	}
	// The former member is soft-deleted before the lifecycle transitions.
	if _, err := repo.pool.Exec(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1::uuid`, former); err != nil {
		t.Fatal(err)
	}
	created, err := repo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatal(err)
	}
	started, err := repo.StartSession(ctx, created.ID, MediaRoomRef("scope-room"))
	if err != nil {
		t.Fatal(err)
	}
	for _, userID := range []string{teacher, joiner} {
		if _, err := repo.JoinSession(ctx, started.ID, userID); err != nil {
			t.Fatalf("join session %s: %v", userID, err)
		}
	}
	if _, err := repo.EndSession(ctx, started.ID, EndReasonManual); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		userID      string
		startedWant int
		endedWant   int
	}{
		{teacher, 1, 1},
		{joiner, 1, 1},
		// The bystander stayed a member but never joined the session.
		{bystander, 1, 0},
		// The durable intent list never projects to soft-deleted users.
		{former, 0, 0},
	} {
		page, err := noticeRepo.List(ctx, tc.userID, nil, 10)
		if err != nil {
			t.Fatal(err)
		}
		var startedCount, endedCount int
		for _, item := range page.Items {
			switch item.Type {
			case notifications.EventSessionStarted:
				startedCount++
			case notifications.EventSessionEnded:
				endedCount++
			}
			if item.SourceRef.ID.String() != started.ID {
				t.Errorf("user %s notice %s references session %s, want %s", tc.userID, item.Type, item.SourceRef.ID, started.ID)
			}
		}
		if startedCount != tc.startedWant || endedCount != tc.endedWant {
			t.Errorf("user %s notices = (%d started, %d ended), want (%d, %d): page=%+v",
				tc.userID, startedCount, endedCount, tc.startedWant, tc.endedWant, page.Items)
		}
	}
}
