//go:build integration

package chat

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/google/uuid"
)

// newDirectNotificationFixture wires the production chat notification stack —
// DB-backed DM eligibility, notification repository, and realtime hub — over a
// freshly migrated schema, mirroring the cmd/api wiring for direct messages.
func newDirectNotificationFixture(t *testing.T) (*Repository, *notifications.Repository, *RealtimeProjector) {
	t.Helper()
	repo := newChatRepo(t)
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate integration test")
	}
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "000021_notification_system.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.pool.Exec(context.Background(), string(migration)); err != nil {
		t.Fatalf("apply F-008 migration: %v", err)
	}
	noticeRepo := notifications.NewRepository(repo.pool)
	membership := rbac.NewRepository(repo.pool)
	tickets := realtime.NewTicketService(membership)
	hub := realtime.NewHub(tickets, nil)
	projector := NewRealtimeProjector(membership, hub, tickets, validProjectorSession)
	projector.SetDMEligibilityChecker(func(ctx context.Context, userA, userB uuid.UUID) (bool, error) {
		_, eligible, err := repo.FindQualifyingDMCircle(ctx, userA, userB)
		return eligible, err
	})
	projector.SetNotificationProjection(noticeRepo, repo.ListNotificationRecipients)
	return repo, noticeRepo, projector
}

// chatMessageRows counts durable message rows for one message id.
func chatMessageRows(t *testing.T, repo *Repository, ctx context.Context, messageID uuid.UUID) int64 {
	t.Helper()
	return countRows(t, repo, `SELECT COUNT(*) FROM messages WHERE id = $1`, messageID)
}

// deliverDirectOutboxEvents runs the committed direct-message outbox events
// through the production dispatcher once, then replays them and returns.
func deliverDirectOutboxEvents(t *testing.T, repo *Repository, ctx context.Context, projector *RealtimeProjector, messageID uuid.UUID) {
	t.Helper()
	dispatcher := NewOutboxDispatcher(NewPGOutboxStore(repo), projector, nil, nil, nil, nil)
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatalf("dispatch direct outbox events: %v", err)
	}
	var undelivered int
	if err := repo.pool.QueryRow(ctx, `SELECT COUNT(*) FROM chat_event_outbox WHERE message_id = $1 AND delivered_at IS NULL`, messageID).Scan(&undelivered); err != nil {
		t.Fatal(err)
	}
	if undelivered != 0 {
		t.Fatalf("undelivered direct outbox events = %d, want all targeted events delivered", undelivered)
	}
	if _, err := repo.pool.Exec(ctx, `
UPDATE chat_event_outbox
SET delivered_at = NULL, available_at = NOW() - INTERVAL '1 second'
WHERE message_id = $1`, messageID); err != nil {
		t.Fatalf("re-arm direct outbox events for replay: %v", err)
	}
	if err := dispatcher.Replay(ctx, 10); err != nil {
		t.Fatalf("replay direct outbox events: %v", err)
	}
}

// F-004/FR-009 non-duplication and FR-020 gates for the private-message row:
// the committed DM produces one push intent for the recipient only, the
// sender-directed targeted event is excluded, the notice carries no message
// content, replay stays idempotent, and no second message row is created.
func TestDirectMessageNotificationTargetsRecipientWithoutDuplicatingSource(t *testing.T) {
	repo, noticeRepo, projector := newDirectNotificationFixture(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "dm-notice-teacher")
	student := seedUser(t, repo, "dm-notice-student")
	circle := seedCircle(t, repo, "Direct Notice Circle", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, teacher, "teacher", joined)
	seedMember(t, repo, circle, student, "student", joined)

	content := "private content must not be copied"
	service := NewDirectService(repo, nil)
	sent, err := service.SendText(ctx, teacher, student, content, "dm-notice-key-1")
	if err != nil {
		t.Fatalf("send direct message: %v", err)
	}
	if rows := chatMessageRows(t, repo, ctx, sent.ID); rows != 1 {
		t.Fatalf("message rows=%d, want exactly one durable row", rows)
	}

	deliverDirectOutboxEvents(t, repo, ctx, projector, sent.ID)

	studentPage, err := noticeRepo.List(ctx, student.String(), nil, 10)
	if err != nil || len(studentPage.Items) != 1 {
		t.Fatalf("student notices = %+v, error=%v; want exactly one private-message notice", studentPage.Items, err)
	}
	notice := studentPage.Items[0]
	if notice.Type != notifications.EventPrivateMessage {
		t.Fatalf("notice type=%q, want private_message", notice.Type)
	}
	if notice.SourceRef.Type != notifications.SourceMessage || notice.SourceRef.ID != sent.ID {
		t.Fatalf("notice source=%+v, want the committed message id %s", notice.SourceRef, sent.ID)
	}
	if notice.Body == content || notice.Title == content {
		t.Fatalf("notice exposed message content: title=%q body=%q", notice.Title, notice.Body)
	}
	if studentPage.UnreadCount != 1 {
		t.Fatalf("student unread=%d, want 1", studentPage.UnreadCount)
	}
	var pushState string
	if err := repo.pool.QueryRow(ctx, `SELECT delivery_state FROM notifications WHERE id = $1`, notice.ID).Scan(&pushState); err != nil {
		t.Fatal(err)
	}
	if pushState != "pending_no_device" {
		t.Fatalf("push intent state=%q, want pending_no_device", pushState)
	}
	teacherPage, err := noticeRepo.List(ctx, teacher.String(), nil, 10)
	if err != nil || len(teacherPage.Items) != 0 {
		t.Fatalf("sender notices = %+v, error=%v; want none", teacherPage.Items, err)
	}

	if rows := chatMessageRows(t, repo, ctx, sent.ID); rows != 1 {
		t.Fatalf("message rows after replay=%d, want one (F-008 must not duplicate chat storage)", rows)
	}
}

// Edge case "eligibility changes before projection": a pair that becomes
// DM-ineligible between commit and dispatch produces no notice for anyone.
func TestDirectMessageNotificationRechecksEligibilityBeforePersisting(t *testing.T) {
	repo, noticeRepo, projector := newDirectNotificationFixture(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "dm-recheck-teacher")
	student := seedUser(t, repo, "dm-recheck-student")
	circle := seedCircle(t, repo, "Direct Recheck Circle", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, teacher, "teacher", joined)
	seedMember(t, repo, circle, student, "student", joined)

	service := NewDirectService(repo, nil)
	sent, err := service.SendText(ctx, teacher, student, "sent while eligible", "dm-recheck-key-1")
	if err != nil {
		t.Fatalf("send direct message: %v", err)
	}
	// The last qualifying DM relationship disappears before the projection runs.
	for _, memberID := range []uuid.UUID{teacher, student} {
		if _, err := repo.pool.Exec(ctx, `DELETE FROM circle_members WHERE circle_id = $1 AND user_id = $2`, circle, memberID); err != nil {
			t.Fatalf("remove qualifying membership: %v", err)
		}
	}

	dispatcher := NewOutboxDispatcher(NewPGOutboxStore(repo), projector, nil, nil, nil, nil)
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatalf("dispatch ineligible direct events: %v", err)
	}
	for _, memberID := range []uuid.UUID{teacher, student} {
		page, err := noticeRepo.List(ctx, memberID.String(), nil, 10)
		if err != nil || len(page.Items) != 0 {
			t.Fatalf("member %s notices = %+v, error=%v; want none after eligibility recheck", memberID, page.Items, err)
		}
	}
	if rows := chatMessageRows(t, repo, ctx, sent.ID); rows != 1 {
		t.Fatalf("message rows=%d, want the durable F-004 row untouched", rows)
	}
}

func TestGroupMessageNotificationTargetsOnlyEligibleCurrentMembers(t *testing.T) {
	ctx := context.Background()
	repo := newChatRepo(t)
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
	sender := seedUser(t, repo, "notice-sender")
	member := seedUser(t, repo, "notice-member")
	lateJoiner := seedUser(t, repo, "notice-late")
	circle := seedCircle(t, repo, "Notification Circle", sender)
	sentAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, sender, "teacher", sentAt.Add(-time.Hour))
	seedMember(t, repo, circle, member, "student", sentAt.Add(-time.Minute))
	seedMember(t, repo, circle, lateJoiner, "student", sentAt.Add(time.Minute))

	creator := notifications.NewRepository(repo.pool)
	projector := &RealtimeProjector{notifications: creator, listRecipients: repo.ListNotificationRecipients}
	message := Message{ID: uuid.New(), CircleID: &circle, SenderID: sender, SentAt: sentAt, Type: MessageTypeText, Content: "private content must not be copied"}
	eventID := uuid.New()
	event := OutboxEvent{EventID: eventID, MessageID: message.ID, EventType: "chat.message"}
	if err := projector.createGroupMessageNotifications(ctx, event, message); err != nil {
		t.Fatalf("project group message notices: %v", err)
	}
	if err := projector.createGroupMessageNotifications(ctx, event, message); err != nil {
		t.Fatalf("replay group message notices: %v", err)
	}
	page, err := creator.List(ctx, member.String(), nil, 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].Type != notifications.EventGroupMessage {
		t.Fatalf("eligible member notices = %+v, error=%v", page.Items, err)
	}
	if page.Items[0].Body == message.Content || page.Items[0].SourceRef.ID != message.ID {
		t.Fatalf("notice exposed message content or wrong source: %+v", page.Items[0])
	}
	for _, id := range []uuid.UUID{sender, lateJoiner} {
		other, err := creator.List(ctx, id.String(), nil, 10)
		if err != nil || len(other.Items) != 0 {
			t.Fatalf("ineligible user %s notices = %+v, error=%v", id, other.Items, err)
		}
	}
}
