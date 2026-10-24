package queue

import (
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/google/uuid"
)

func TestQueueTurnNoticeUsesStableEventAndSessionReference(t *testing.T) {
	recipientID := uuid.New()
	sessionID := uuid.New()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	input := queueTurnNoticeInput("queue-event-7", "queue.your_turn", recipientID, sessionID, now)

	if input.Type != notifications.EventQueueYourTurn || input.RecipientID != recipientID ||
		input.SourceEventID != "queue-event-7" || input.SourceRef.Type != notifications.SourceSession || input.SourceRef.ID != sessionID {
		t.Fatalf("queue notice identity = %+v", input)
	}
	if !input.PushExpiresAt.Equal(now.Add(24*time.Hour)) || input.Title == "" || input.Body == "" {
		t.Fatalf("queue notice display/expiry = %+v", input)
	}
}

func TestQueueNextSoonNoticeUsesStableEventAndSessionReference(t *testing.T) {
	recipientID := uuid.New()
	sessionID := uuid.New()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	input := queueTurnNoticeInput("queue-event-8", "queue.next_soon", recipientID, sessionID, now)

	if input.Type != notifications.EventQueueNextSoon || input.RecipientID != recipientID ||
		input.SourceEventID != "queue-event-8" || input.SourceRef.Type != notifications.SourceSession || input.SourceRef.ID != sessionID {
		t.Fatalf("queue next-soon notice identity = %+v", input)
	}
	if !input.PushExpiresAt.Equal(now.Add(24*time.Hour)) || input.Title != "You're next" || input.Body == "" {
		t.Fatalf("queue next-soon notice display/expiry = %+v", input)
	}
}

func TestGradeNoticeRequiresGradeCompletionAndReferencesRecitationLog(t *testing.T) {
	studentID := uuid.New()
	progressID := uuid.New()
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	input := gradePostedNoticeInput("grade-event-3", studentID, progressID, now)

	if input.Type != notifications.EventGradePosted || input.RecipientID != studentID ||
		input.SourceEventID != "grade-event-3" || input.SourceRef.Type != notifications.SourceRecitation || input.SourceRef.ID != progressID {
		t.Fatalf("grade notice identity = %+v", input)
	}
	if input.Title == "" || input.Body == "" || !input.PushExpiresAt.Equal(now.Add(24*time.Hour)) {
		t.Fatalf("grade notice display/expiry = %+v", input)
	}
}
