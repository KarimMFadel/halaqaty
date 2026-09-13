//go:build integration

package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

// TestPresenceService_MarkGroupMessageReadIsIdempotent proves an active member
// can mark another member's visible group message read repeatedly while the
// durable read fact remains unique.
func TestPresenceService_MarkGroupMessageReadIsIdempotent(t *testing.T) {
	repo := newChatRepo(t)
	service := NewPresenceService(repo, rbac.NewRepository(repo.pool))
	ctx := context.Background()
	senderID := seedUser(t, repo, "presence-active-sender")
	readerID := seedUser(t, repo, "presence-active-reader")
	circleID := seedCircle(t, repo, "Presence Active Circle", senderID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, senderID, "teacher", joinedAt)
	seedMember(t, repo, circleID, readerID, "student", joinedAt)
	messageID := seedMessage(t, repo, senderID, &circleID, nil, joinedAt.Add(time.Minute), "visible history")

	for attempt := 0; attempt < 2; attempt++ {
		if err := service.MarkGroupMessageRead(ctx, readerID, circleID, messageID); err != nil {
			t.Fatalf("MarkGroupMessageRead() attempt %d: %v", attempt+1, err)
		}
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM message_reads WHERE message_id = $1 AND user_id = $2`, messageID, readerID); n != 1 {
		t.Fatalf("repeated mark-read must leave one read fact: got %d", n)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox WHERE message_id = $1 AND event_type = $2 AND recipient_id = $3`, messageID, realtime.EventChatMessageRead, senderID); n != 1 {
		t.Fatalf("repeated mark-read must leave one sender-targeted event: got %d", n)
	}
}

// TestPresenceService_MarkGroupMessageReadRejectsArchivedCircle proves retained
// archived-circle history stays read-only: marking a visible message read is a
// mutation and must fail before any persistence is attempted.
func TestPresenceService_MarkGroupMessageReadRejectsArchivedCircle(t *testing.T) {
	repo := newChatRepo(t)
	service := NewPresenceService(repo, rbac.NewRepository(repo.pool))
	ctx := context.Background()
	senderID := seedUser(t, repo, "presence-archived-sender")
	readerID := seedUser(t, repo, "presence-archived-reader")
	circleID := seedCircle(t, repo, "Presence Archived Circle", senderID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, senderID, "teacher", joinedAt)
	seedMember(t, repo, circleID, readerID, "student", joinedAt)
	messageID := seedMessage(t, repo, senderID, &circleID, nil, joinedAt.Add(time.Minute), "retained history")
	if _, err := repo.pool.Exec(ctx, `UPDATE circles SET is_archived = TRUE WHERE id = $1`, circleID); err != nil {
		t.Fatalf("archive circle: %v", err)
	}

	err := service.MarkGroupMessageRead(ctx, readerID, circleID, messageID)
	if !errors.Is(err, ErrCircleArchived) {
		t.Fatalf("MarkGroupMessageRead() error = %v, want ErrCircleArchived", err)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM message_reads`); n != 0 {
		t.Fatalf("archived mark-read must persist nothing: got %d read facts", n)
	}
}

// TestPresenceService_MarkGroupMessageReadRejectsOwnMessage proves read facts
// are recipient-only, even when the sender is an active member.
func TestPresenceService_MarkGroupMessageReadRejectsOwnMessage(t *testing.T) {
	repo := newChatRepo(t)
	service := NewPresenceService(repo, rbac.NewRepository(repo.pool))
	ctx := context.Background()
	senderID := seedUser(t, repo, "presence-own-sender")
	circleID := seedCircle(t, repo, "Presence Own Circle", senderID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, senderID, "teacher", joinedAt)
	messageID := seedMessage(t, repo, senderID, &circleID, nil, joinedAt.Add(time.Minute), "own message")

	err := service.MarkGroupMessageRead(ctx, senderID, circleID, messageID)
	if !errors.Is(err, ErrMessageNotVisible) {
		t.Fatalf("MarkGroupMessageRead() error = %v, want ErrMessageNotVisible", err)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM message_reads`); n != 0 {
		t.Fatalf("sender read must persist nothing: got %d read facts", n)
	}
}

// TestPresenceService_MarkDirectMessageReadIsIdempotent proves the same
// durable/read-event invariant for an eligible teacher-student DM pair.
func TestPresenceService_MarkDirectMessageReadIsIdempotent(t *testing.T) {
	repo := newChatRepo(t)
	service := NewPresenceService(repo, rbac.NewRepository(repo.pool))
	ctx := context.Background()
	teacherID := seedUser(t, repo, "presence-dm-teacher")
	studentID := seedUser(t, repo, "presence-dm-student")
	circleID := seedCircle(t, repo, "Presence DM Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	seedMember(t, repo, circleID, studentID, "student", joinedAt)
	messageID := seedMessage(t, repo, teacherID, nil, &studentID, joinedAt.Add(time.Minute), "dm")

	for attempt := 0; attempt < 2; attempt++ {
		if err := service.MarkDirectMessageRead(ctx, studentID, messageID); err != nil {
			t.Fatalf("MarkDirectMessageRead() attempt %d: %v", attempt+1, err)
		}
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM message_reads WHERE message_id = $1 AND user_id = $2`, messageID, studentID); n != 1 {
		t.Fatalf("repeated DM mark-read must leave one fact: got %d", n)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox WHERE message_id = $1 AND event_type = $2 AND recipient_id = $3`, messageID, realtime.EventChatMessageRead, teacherID); n != 1 {
		t.Fatalf("repeated DM mark-read must leave one targeted event: got %d", n)
	}
}
