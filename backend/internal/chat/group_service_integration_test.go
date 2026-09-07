//go:build integration

package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// newGroupService builds the service over a freshly migrated schema with the
// real F-002 membership reader, mirroring production wiring.
func newGroupService(t *testing.T) (*Repository, *GroupService) {
	t.Helper()
	repo := newChatRepo(t)
	membership := rbac.NewRepository(repo.pool)
	service := NewGroupService(repo, membership, &metrics.ChatMetrics{}, logging.NewAuditLogger(nil))
	return repo, service
}

// TestGroupService_History_MembershipPeriodAuthorization proves US1-AC1 at the
// service boundary: members see only their current membership period, and both
// non-members and unknown-circle callers get the identical non-enumerating
// denial error.
func TestGroupService_History_MembershipPeriodAuthorization(t *testing.T) {
	repo, service := newGroupService(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "svc-hist-teacher")
	student := seedUser(t, repo, "svc-hist-student")
	outsider := seedUser(t, repo, "svc-hist-outsider")
	circle := seedCircle(t, repo, "Service History Circle", teacher)

	joinedAt := time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Microsecond)
	seedMember(t, repo, circle, teacher, "teacher", joinedAt.Add(-time.Hour))
	seedMember(t, repo, circle, student, "student", joinedAt)
	seedMessage(t, repo, teacher, &circle, nil, joinedAt.Add(-10*time.Minute), "before-join")
	seedMessage(t, repo, teacher, &circle, nil, joinedAt, "at-join")
	seedMessage(t, repo, teacher, &circle, nil, joinedAt.Add(10*time.Minute), "after-join")

	studentPage, err := service.History(ctx, student, circle, nil, 50)
	if err != nil {
		t.Fatalf("student history: %v", err)
	}
	if got := messageContents(studentPage); len(got) != 2 || got[0] != "after-join" || got[1] != "at-join" {
		t.Fatalf("student must see only the current membership period: got %v", got)
	}

	teacherPage, err := service.History(ctx, teacher, circle, nil, 50)
	if err != nil {
		t.Fatalf("teacher history: %v", err)
	}
	if got := messageContents(teacherPage); len(got) != 3 {
		t.Fatalf("teacher joined before all messages and must see them: got %v", got)
	}

	nonMember, err := service.History(ctx, outsider, circle, nil, 50)
	if !errors.Is(err, ErrCircleNotVisible) {
		t.Fatalf("non-member history error = %v, want ErrCircleNotVisible", err)
	}
	if nonMember != nil {
		t.Fatalf("non-member page must be nil, got %v", nonMember)
	}
	unknownCircle, err := service.History(ctx, student, uuid.New(), nil, 50)
	if !errors.Is(err, ErrCircleNotVisible) {
		t.Fatalf("unknown-circle history error = %v, want identical ErrCircleNotVisible", err)
	}
	if unknownCircle != nil {
		t.Fatalf("unknown-circle page must be nil, got %v", unknownCircle)
	}
}

// TestGroupService_History_KeysetPaginationIsDeterministic pages equal-timestamp
// messages through the service and proves strict (sent_at, id) DESC ordering
// with full, duplicate-free coverage.
func TestGroupService_History_KeysetPaginationIsDeterministic(t *testing.T) {
	repo, service := newGroupService(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "svc-page-teacher")
	circle := seedCircle(t, repo, "Service Page Circle", teacher)
	seedMember(t, repo, circle, teacher, "teacher", time.Now().UTC().Add(-time.Hour))

	sentAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	for i := 0; i < 5; i++ {
		seedMessage(t, repo, teacher, &circle, nil, sentAt, "svc-page-message")
	}

	var flattened []Message
	before := (*uuid.UUID)(nil)
	for page := 0; page < 3; page++ {
		msgs, err := service.History(ctx, teacher, circle, before, 2)
		if err != nil {
			t.Fatalf("service page %d: %v", page, err)
		}
		wantLen := 2
		if page == 2 {
			wantLen = 1
		}
		if len(msgs) != wantLen {
			t.Fatalf("service page %d length: got %d want %d", page, len(msgs), wantLen)
		}
		flattened = append(flattened, msgs...)
		before = &msgs[len(msgs)-1].ID
	}

	assertStrictlyDescending(t, flattened)
	if len(flattened) != 5 {
		t.Fatalf("pagination must cover every message exactly once: got %d", len(flattened))
	}
	terminal, err := service.History(ctx, teacher, circle, before, 2)
	if err != nil {
		t.Fatalf("terminal page: %v", err)
	}
	if len(terminal) != 0 {
		t.Fatalf("terminal page must be empty, got %d messages", len(terminal))
	}
}

// TestGroupService_SendText_IdempotentSingleRowAndOutboxEvent proves US1-AC2
// and US2-AC3: a replayed (sender, idempotency key) returns the committed
// original and creates neither a second message row nor a second outbox event.
func TestGroupService_SendText_IdempotentSingleRowAndOutboxEvent(t *testing.T) {
	repo, service := newGroupService(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "svc-send-sender")
	circle := seedCircle(t, repo, "Service Send Circle", sender)
	seedMember(t, repo, circle, sender, "teacher", time.Now().UTC().Add(-time.Hour))

	first, err := service.SendText(ctx, sender, circle, "  durable once  ", "svc-send-key-1")
	if err != nil {
		t.Fatalf("first send: %v", err)
	}
	if first.ID == uuid.Nil || first.State != MessageStateActive || first.Content != "durable once" {
		t.Fatalf("first send projection incomplete: %+v", first)
	}

	second, err := service.SendText(ctx, sender, circle, "durable once", "svc-send-key-1")
	if err != nil {
		t.Fatalf("replayed send: %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("replayed send must return the same message: got %s want %s", second.ID, first.ID)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM messages WHERE sender_id = $1`, sender); n != 1 {
		t.Fatalf("durable message rows: got %d want 1", n)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox WHERE message_id = $1`, first.ID); n != 1 {
		t.Fatalf("outbox events for the message: got %d want 1", n)
	}
}

// TestGroupService_SendText_NonMemberDeniedWithoutEnumeration proves the send
// authorization gate: non-members and unknown circles receive the identical
// denial and nothing is persisted.
func TestGroupService_SendText_NonMemberDeniedWithoutEnumeration(t *testing.T) {
	repo, service := newGroupService(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "svc-deny-teacher")
	outsider := seedUser(t, repo, "svc-deny-outsider")
	circle := seedCircle(t, repo, "Service Deny Circle", teacher)
	seedMember(t, repo, circle, teacher, "teacher", time.Now().UTC().Add(-time.Hour))

	if _, err := service.SendText(ctx, outsider, circle, "unauthorized", "svc-deny-key-1"); !errors.Is(err, ErrCircleNotVisible) {
		t.Fatalf("non-member send error = %v, want ErrCircleNotVisible", err)
	}
	if _, err := service.SendText(ctx, teacher, uuid.New(), "unknown circle", "svc-deny-key-2"); !errors.Is(err, ErrCircleNotVisible) {
		t.Fatalf("unknown-circle send error = %v, want identical ErrCircleNotVisible", err)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM messages`); n != 0 {
		t.Fatalf("denied sends must persist nothing: got %d messages", n)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox`); n != 0 {
		t.Fatalf("denied sends must persist no outbox events: got %d", n)
	}
}

// TestGroupService_SendText_ArchivedCircleMutationsDenied proves FR-032's
// mutation gate for US1 text sends while reads stay available.
func TestGroupService_SendText_ArchivedCircleMutationsDenied(t *testing.T) {
	repo, service := newGroupService(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "svc-arch-teacher")
	circle := seedCircle(t, repo, "Service Archive Circle", teacher)
	seedMember(t, repo, circle, teacher, "teacher", time.Now().UTC().Add(-time.Hour))
	if _, err := repo.pool.Exec(ctx, `UPDATE circles SET is_archived = TRUE WHERE id = $1`, circle); err != nil {
		t.Fatalf("archive fixture: %v", err)
	}

	if _, err := service.SendText(ctx, teacher, circle, "into the archive", "svc-arch-key-1"); !errors.Is(err, ErrCircleArchived) {
		t.Fatalf("archived send error = %v, want ErrCircleArchived", err)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM messages`); n != 0 {
		t.Fatalf("archived send must persist nothing: got %d messages", n)
	}
	seedMessage(t, repo, teacher, &circle, nil, time.Now().UTC().Add(-time.Minute), "retained history")
	history, err := service.History(ctx, teacher, circle, nil, 50)
	if err != nil {
		t.Fatalf("archived retained history: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("archived retained history must stay readable: got %d messages", len(history))
	}
}

// TestGroupService_NoLiveSessionIndependence proves US1-AC4 backend-side:
// history and sends succeed with zero rows in the live-session tables.
func TestGroupService_NoLiveSessionIndependence(t *testing.T) {
	repo, service := newGroupService(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "svc-nosession-teacher")
	circle := seedCircle(t, repo, "Service NoSession Circle", teacher)
	seedMember(t, repo, circle, teacher, "teacher", time.Now().UTC().Add(-time.Hour))

	if _, err := service.SendText(ctx, teacher, circle, "no session needed", "svc-nosession-key-1"); err != nil {
		t.Fatalf("send without live session: %v", err)
	}
	if _, err := service.History(ctx, teacher, circle, nil, 50); err != nil {
		t.Fatalf("history without live session: %v", err)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM sessions`); n != 0 {
		t.Fatalf("chat must not create live sessions: got %d rows", n)
	}
}
