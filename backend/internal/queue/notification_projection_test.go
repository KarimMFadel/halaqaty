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
	// Lock-screen copy must stay generic: no queue, student, or session
	// identifiers may leak into the notice text.
	titles := map[string]bool{"Your turn": true, "You're next": true}
	if !titles[input.Title] || input.SourceEventID == input.Title || input.SourceEventID == input.Body {
		t.Fatalf("queue notice copy = (%q, %q), want the approved generic template", input.Title, input.Body)
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
	// FR-019/DeepLink boundary: the grade projection construction takes no
	// grade value in — title and body are the approved generic copy, and the
	// only source reference points at the recitation log (loaded after
	// authorization in F-007).
	if input.Title != "New grade" || input.Body != "A recitation grade is ready to view." {
		t.Fatalf("grade notice copy = (%q, %q), want the approved generic template", input.Title, input.Body)
	}
	if input.SourceEventID == input.Title || input.SourceEventID == input.Body {
		t.Fatalf("grade notice copy = (%q, %q), want no identifiers in text", input.Title, input.Body)
	}
}
