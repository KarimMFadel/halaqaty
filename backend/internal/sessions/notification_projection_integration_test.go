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
