//go:build integration

package queue

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/google/uuid"
)

func TestQueueOutboxNotificationProjectionsAreRecipientScopedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := newQueueRepository(t)
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test")
	}
	migrationsDir := filepath.Join(filepath.Dir(file), "..", "..", "migrations")
	for _, name := range []string{"000018_real_time_chat.up.sql", "000019_account_deletion_tombstone.up.sql", "000021_notification_system.up.sql"} {
		migration, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := repo.pool.Exec(ctx, string(migration)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}

	teacherID := qSeedUser(t, repo, "notification-teacher")
	studentID := qSeedUser(t, repo, "notification-student")
	circleID := qSeedCircle(t, repo, teacherID)
	qSeedMember(t, repo, circleID, teacherID, "teacher", time.Now().UTC())
	qSeedMember(t, repo, circleID, studentID, "student", time.Now().UTC())
	sessionID := qInsertSession(t, repo, circleID, teacherID, "managers_and_student")
	round := qCreateRound(t, repo, sessionID, teacherID, "active", []string{studentID})
	noticeRepo := notifications.NewRepository(repo.pool)
	projector := NewRealtimeOutboxProjector(repo, nil, noticeRepo)

	queueEventID := uuid.NewString()
	queueEvent := OutboxEvent{EventID: queueEventID, SessionID: sessionID}
	if err := projector.createQueueTurnNotification(ctx, queueEvent, "queue.your_turn", studentID); err != nil {
		t.Fatalf("create queue turn notification: %v", err)
	}
	if err := projector.createQueueTurnNotification(ctx, queueEvent, "queue.your_turn", studentID); err != nil {
		t.Fatalf("replay queue turn notification: %v", err)
	}
	nextSoonEvent := OutboxEvent{EventID: uuid.NewString(), SessionID: sessionID}
	if err := projector.createQueueTurnNotification(ctx, nextSoonEvent, "queue.next_soon", studentID); err != nil {
		t.Fatalf("create queue next-soon notification: %v", err)
	}
	if err := projector.createQueueTurnNotification(ctx, nextSoonEvent, "queue.next_soon", studentID); err != nil {
		t.Fatalf("replay queue next-soon notification: %v", err)
	}

	turns := NewTurnService(repo)
	advanced, err := turns.Advance(ctx, round.ID, round.Version)
	if err != nil {
		t.Fatalf("advance queue: %v", err)
	}
	if _, err := turns.Start(ctx, *advanced.SelectedEntryID, 1); err != nil {
		t.Fatalf("start queue entry: %v", err)
	}
	grade := GradeExcellent
	if _, err := turns.Complete(ctx, *advanced.SelectedEntryID, 2, teacherID, &grade, nil); err != nil {
		t.Fatalf("complete graded entry: %v", err)
	}
	var progressID uuid.UUID
	if err := repo.pool.QueryRow(ctx, `SELECT id FROM memorization_progress WHERE queue_entry_id = $1::uuid`, *advanced.SelectedEntryID).Scan(&progressID); err != nil {
		t.Fatalf("load recitation log id: %v", err)
	}
	metadata, _ := json.Marshal(map[string]string{"graded": "true", "student_id": studentID})
	gradeEvent := OutboxEvent{EventID: uuid.NewString(), EventType: "queue.grade_submitted", ResourceID: advanced.SelectedEntryID, EventMetadata: metadata}
	if err := projector.projectGradeNotification(ctx, gradeEvent, map[string]any{"graded": "true", "student_id": studentID}); err != nil {
		t.Fatalf("project grade notification: %v", err)
	}
	if err := projector.projectGradeNotification(ctx, gradeEvent, map[string]any{"graded": "true", "student_id": studentID}); err != nil {
		t.Fatalf("replay grade notification: %v", err)
	}

	page, err := noticeRepo.List(ctx, studentID, nil, 10)
	if err != nil {
		t.Fatalf("list projected notices: %v", err)
	}
	if len(page.Items) != 3 || page.UnreadCount != 3 {
		t.Fatalf("student notices = %d, unread = %d; want three idempotent notices", len(page.Items), page.UnreadCount)
	}
	got := map[notifications.EventType]notifications.Item{}
	for _, item := range page.Items {
		got[item.Type] = item
	}
	if item := got[notifications.EventQueueYourTurn]; item.SourceRef.Type != notifications.SourceSession || item.SourceRef.ID.String() != sessionID {
		t.Fatalf("queue notice source = %+v", item.SourceRef)
	}
	if item := got[notifications.EventQueueNextSoon]; item.SourceRef.Type != notifications.SourceSession || item.SourceRef.ID.String() != sessionID {
		t.Fatalf("queue next-soon notice source = %+v", item.SourceRef)
	}
	if item := got[notifications.EventGradePosted]; item.SourceRef.Type != notifications.SourceRecitation || item.SourceRef.ID != progressID {
		t.Fatalf("grade notice source = %+v, want recitation %s", item.SourceRef, progressID)
	}
	teacherPage, err := noticeRepo.List(ctx, teacherID, nil, 10)
	if err != nil || len(teacherPage.Items) != 0 {
		t.Fatalf("teacher notices = %d, error = %v; want none", len(teacherPage.Items), err)
	}
}
