//go:build integration

package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

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
	if n := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox WHERE message_id = $1 AND event_type = $2 AND recipient_id = $3`, messageID, realtime.EventChatMessageRead, readerID); n != 1 {
		t.Fatalf("repeated mark-read must leave one reader-identified event: got %d", n)
	}
	receipts, err := repo.LoadSenderReadReceipts(ctx, Message{ID: messageID, CircleID: &circleID, SenderID: senderID})
	if err != nil {
		t.Fatalf("load sender read receipts: %v", err)
	}
	if len(receipts) != 1 || receipts[0].UserID != readerID || receipts[0].ReadAt.IsZero() {
		t.Fatalf("sender read receipts = %#v", receipts)
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
		if err := service.MarkDirectMessageRead(ctx, studentID, teacherID, messageID); err != nil {
			t.Fatalf("MarkDirectMessageRead() attempt %d: %v", attempt+1, err)
		}
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM message_reads WHERE message_id = $1 AND user_id = $2`, messageID, studentID); n != 1 {
		t.Fatalf("repeated DM mark-read must leave one fact: got %d", n)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox WHERE message_id = $1 AND event_type = $2 AND recipient_id = $3`, messageID, realtime.EventChatMessageRead, studentID); n != 1 {
		t.Fatalf("repeated DM mark-read must leave one reader-identified event: got %d", n)
	}
}

// TestPresenceService_LoadSenderReadReceiptsHidesPriorMembershipPeriod proves
// a rejoined reader cannot expose a fact for a message sent before rejoining.
func TestPresenceService_LoadSenderReadReceiptsHidesPriorMembershipPeriod(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	senderID := seedUser(t, repo, "presence-period-sender")
	readerID := seedUser(t, repo, "presence-period-reader")
	circleID := seedCircle(t, repo, "Presence Period Circle", senderID)
	oldJoin := time.Now().UTC().Add(-3 * time.Hour)
	seedMember(t, repo, circleID, senderID, "teacher", oldJoin)
	seedMember(t, repo, circleID, readerID, "student", oldJoin)
	messageID := seedMessage(t, repo, senderID, &circleID, nil, oldJoin.Add(time.Hour), "before rejoin")
	if _, err := repo.pool.Exec(ctx, `UPDATE circle_members SET joined_at = $1 WHERE circle_id = $2 AND user_id = $3`, time.Now().UTC(), circleID, readerID); err != nil {
		t.Fatalf("simulate rejoin: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `INSERT INTO message_reads (message_id, user_id) VALUES ($1, $2)`, messageID, readerID); err != nil {
		t.Fatalf("seed historical read fact: %v", err)
	}
	receipts, err := repo.LoadSenderReadReceipts(ctx, Message{ID: messageID, CircleID: &circleID, SenderID: senderID})
	if err != nil {
		t.Fatalf("load sender read receipts: %v", err)
	}
	if len(receipts) != 0 {
		t.Fatalf("prior-period receipts = %#v, want none", receipts)
	}
}

// TestPresenceService_MarkGroupMessageReadRejectsPostRemoval proves that a
// removed member cannot create a new read fact for retained group history.
func TestPresenceService_MarkGroupMessageReadRejectsPostRemoval(t *testing.T) {
	repo := newChatRepo(t)
	service := NewPresenceService(repo, rbac.NewRepository(repo.pool))
	ctx := context.Background()
	senderID := seedUser(t, repo, "presence-removed-sender")
	readerID := seedUser(t, repo, "presence-removed-reader")
	circleID := seedCircle(t, repo, "Presence Removed Circle", senderID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, senderID, "teacher", joinedAt)
	seedMember(t, repo, circleID, readerID, "student", joinedAt)
	messageID := seedMessage(t, repo, senderID, &circleID, nil, joinedAt.Add(time.Minute), "before removal")
	if err := rbac.NewRepository(repo.pool).RemoveMember(ctx, circleID.String(), readerID.String()); err != nil {
		t.Fatalf("remove reader: %v", err)
	}

	err := service.MarkGroupMessageRead(ctx, readerID, circleID, messageID)
	if !errors.Is(err, ErrCircleNotVisible) {
		t.Fatalf("MarkGroupMessageRead() error = %v, want ErrCircleNotVisible", err)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM message_reads WHERE message_id = $1`, messageID); n != 0 {
		t.Fatalf("post-removal mark-read must persist nothing: got %d read facts", n)
	}
}

// TestPresenceService_LoadSenderReadReceiptsFiltersRemovedReaders proves that
// sender-visible group receipt details track the reader's current membership.
func TestPresenceService_LoadSenderReadReceiptsFiltersRemovedReaders(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	senderID := seedUser(t, repo, "presence-filter-sender")
	readerID := seedUser(t, repo, "presence-filter-reader")
	circleID := seedCircle(t, repo, "Presence Receipt Filter Circle", senderID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, senderID, "teacher", joinedAt)
	seedMember(t, repo, circleID, readerID, "student", joinedAt)
	messageID := seedMessage(t, repo, senderID, &circleID, nil, joinedAt.Add(time.Minute), "receipt")
	if _, err := repo.pool.Exec(ctx, `INSERT INTO message_reads (message_id, user_id) VALUES ($1, $2)`, messageID, readerID); err != nil {
		t.Fatalf("seed read fact: %v", err)
	}
	if err := rbac.NewRepository(repo.pool).RemoveMember(ctx, circleID.String(), readerID.String()); err != nil {
		t.Fatalf("remove reader: %v", err)
	}

	receipts, err := repo.LoadSenderReadReceipts(ctx, Message{ID: messageID, CircleID: &circleID, SenderID: senderID})
	if err != nil {
		t.Fatalf("load sender read receipts: %v", err)
	}
	if len(receipts) != 0 {
		t.Fatalf("removed reader receipts = %#v, want none", receipts)
	}
}

// TestPresenceService_RestoredDMEligibilityPreservesReadFacts proves that a
// pair's existing conversation and durable receipt reappear after eligibility
// is restored through a new qualifying membership.
func TestPresenceService_RestoredDMEligibilityPreservesReadFacts(t *testing.T) {
	repo := newChatRepo(t)
	service := NewPresenceService(repo, rbac.NewRepository(repo.pool))
	ctx := context.Background()
	teacherID := seedUser(t, repo, "presence-restored-dm-teacher")
	studentID := seedUser(t, repo, "presence-restored-dm-student")
	circleID := seedCircle(t, repo, "Presence Restored DM Circle", teacherID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, teacherID, "teacher", joinedAt)
	seedMember(t, repo, circleID, studentID, "student", joinedAt)
	messageID := seedMessage(t, repo, teacherID, nil, &studentID, joinedAt.Add(time.Minute), "retained dm")
	if err := service.MarkDirectMessageRead(ctx, studentID, teacherID, messageID); err != nil {
		t.Fatalf("record initial DM read: %v", err)
	}
	if err := rbac.NewRepository(repo.pool).RemoveMember(ctx, circleID.String(), studentID.String()); err != nil {
		t.Fatalf("remove student: %v", err)
	}
	if err := service.MarkDirectMessageRead(ctx, studentID, teacherID, messageID); !errors.Is(err, ErrDMNotEligible) {
		t.Fatalf("removed DM reader error = %v, want ErrDMNotEligible", err)
	}
	seedMember(t, repo, circleID, studentID, "student", time.Now().UTC())

	receipts, err := repo.LoadSenderReadReceipts(ctx, Message{ID: messageID, DMRecipientID: &studentID, SenderID: teacherID})
	if err != nil {
		t.Fatalf("load restored DM receipts: %v", err)
	}
	if len(receipts) != 1 || receipts[0].UserID != studentID {
		t.Fatalf("restored DM receipts = %#v, want retained student fact", receipts)
	}
}

// TestPresenceService_TypingDoesNotPersistChatFacts proves an accepted typing
// command is ephemeral: it creates neither message history nor outbox rows.
func TestPresenceService_TypingDoesNotPersistChatFacts(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	userID := seedUser(t, repo, "presence-typing-user")
	circleID := seedCircle(t, repo, "Presence Typing Circle", userID)
	seedMember(t, repo, circleID, userID, "teacher", time.Now().UTC().Add(-time.Hour))
	membership := rbac.NewRepository(repo.pool)
	tickets := realtime.NewTicketService(membership)
	ticket, err := tickets.IssueForSession(ctx, userID.String(), "typing-session")
	if err != nil {
		t.Fatalf("issue realtime ticket: %v", err)
	}
	projector := NewRealtimeProjector(membership, realtime.NewHub(tickets, nil), tickets, func(context.Context, string, string) (bool, error) { return true, nil })
	handler := NewTypingCommandHandler(projector)
	beforeMessages := countRows(t, repo, `SELECT COUNT(*) FROM messages`)
	beforeOutbox := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox`)

	err = handler(ctx, realtime.ChatCommand{
		Connection: realtime.ConnectionIdentity{UserID: userID.String(), RealtimeTicket: ticket.Token},
		RequestID:  uuid.NewString(),
		Payload:    map[string]any{"circle_id": circleID.String(), "is_typing": true},
	})
	if err != nil {
		t.Fatalf("accepted typing command: %v", err)
	}
	if got := countRows(t, repo, `SELECT COUNT(*) FROM messages`); got != beforeMessages {
		t.Fatalf("typing changed message history: got %d, want %d", got, beforeMessages)
	}
	if got := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox`); got != beforeOutbox {
		t.Fatalf("typing changed durable outbox: got %d, want %d", got, beforeOutbox)
	}
}
