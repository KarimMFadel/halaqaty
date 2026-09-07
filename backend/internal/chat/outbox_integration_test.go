//go:build integration

package chat

import (
	"context"
	"errors"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

// newOutboxIntegration builds the full outbox stack over a freshly migrated
// schema: service, PostgreSQL store, projector, hub, and dispatcher.
func newOutboxIntegration(t *testing.T) (*Repository, *GroupService, *rbac.Repository, *metrics.ChatMetrics) {
	t.Helper()
	repo := newChatRepo(t)
	membership := rbac.NewRepository(repo.pool)
	chatMetrics := &metrics.ChatMetrics{}
	service := NewGroupService(repo, membership, chatMetrics, logging.NewAuditLogger(nil))
	return repo, service, membership, chatMetrics
}

// rewindDueOutbox makes every pending event immediately claimable again so
// bounded-retry tests do not wait for real backoff clocks.
func rewindDueOutbox(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		UPDATE chat_event_outbox
		SET available_at = NOW() - INTERVAL '1 second'
		WHERE delivered_at IS NULL AND parked_at IS NULL
	`); err != nil {
		t.Fatalf("rewind outbox availability: %v", err)
	}
}

func outboxRowState(t *testing.T, repo *Repository, eventID string) (delivered, parked bool, attempts int) {
	t.Helper()
	var deliveredAt, parkedAt *time.Time
	if err := repo.pool.QueryRow(context.Background(),
		`SELECT delivered_at, parked_at, attempt_count FROM chat_event_outbox WHERE event_id::text = $1`, eventID,
	).Scan(&deliveredAt, &parkedAt, &attempts); err != nil {
		t.Fatalf("load outbox row state: %v", err)
	}
	return deliveredAt != nil, parkedAt != nil, attempts
}

// TestOutboxIntegration_SendCommitsMessageAndOutboxAtomically proves the
// transactional-outbox invariant at the service boundary: every accepted send
// commits exactly one message row and one identifier-only event row together.
func TestOutboxIntegration_SendCommitsMessageAndOutboxAtomically(t *testing.T) {
	repo, service, _, _ := newOutboxIntegration(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "ob-atomic-sender")
	circle := seedCircle(t, repo, "Outbox Atomic Circle", sender)
	seedMember(t, repo, circle, sender, "teacher", time.Now().UTC().Add(-time.Hour))

	sent, err := service.SendText(ctx, sender, circle, "atomic send", "ob-atomic-key-1")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM messages WHERE id = $1`, sent.ID); n != 1 {
		t.Fatalf("message rows: got %d want 1", n)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox WHERE message_id = $1`, sent.ID); n != 1 {
		t.Fatalf("outbox rows: got %d want 1", n)
	}
	var eventType string
	if err := repo.pool.QueryRow(ctx, `SELECT event_type FROM chat_event_outbox WHERE message_id = $1`, sent.ID).Scan(&eventType); err != nil {
		t.Fatalf("load outbox event type: %v", err)
	}
	if eventType != realtime.EventChatMessage {
		t.Fatalf("outbox event type = %q, want %q", eventType, realtime.EventChatMessage)
	}
}

// TestOutboxIntegration_DispatchDueProjectsToAuthorizedSubscriber proves the
// end-to-end US1-AC3 flow: a durable send is projected once to the currently
// authorized subscriber and the event row is marked delivered.
func TestOutboxIntegration_DispatchDueProjectsToAuthorizedSubscriber(t *testing.T) {
	repo, service, membership, _ := newOutboxIntegration(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "ob-live-teacher")
	circle := seedCircle(t, repo, "Outbox Live Circle", teacher)
	seedMember(t, repo, circle, teacher, "teacher", time.Now().UTC().Add(-time.Hour))

	tickets := realtime.NewTicketService(membership)
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()
	ticket, err := tickets.Issue(ctx, teacher.String())
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	conn := dialProjectorClient(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()
	subscribeProjectorCircle(t, conn, circle.String())

	sent, err := service.SendText(ctx, teacher, circle, "live projection", "ob-live-key-1")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	projector := NewRealtimeProjector(membership, hub, tickets)
	dispatcher := NewOutboxDispatcher(NewPGOutboxStore(repo), projector, nil, nil, nil, nil)
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatalf("dispatch due: %v", err)
	}

	envelope := readProjectorJSON(t, conn)
	if envelope["type"] != realtime.EventChatMessage {
		t.Fatalf("subscriber event = %v", envelope)
	}
	payload, _ := envelope["payload"].(map[string]any)
	if payload["id"] != sent.ID.String() {
		t.Fatalf("projected message id = %v, want %s", payload["id"], sent.ID)
	}
	var eventID string
	if err := repo.pool.QueryRow(ctx, `SELECT event_id::text FROM chat_event_outbox WHERE message_id = $1`, sent.ID).Scan(&eventID); err != nil {
		t.Fatalf("load event id: %v", err)
	}
	if envelope["event_id"] != eventID {
		t.Fatalf("projected event id = %v, want %s", envelope["event_id"], eventID)
	}
	delivered, _, _ := outboxRowState(t, repo, eventID)
	if !delivered {
		t.Fatal("delivered event must be marked delivered")
	}
	rewindDueOutbox(t, repo)
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatalf("second dispatch: %v", err)
	}
	assertNoProjectorEvent(t, conn)
}

// TestOutboxIntegration_BoundedRetriesParkEventAfterFiveAttempts drives five
// failing deliveries through real claim SQL and proves the event parks exactly
// at the five-attempt ceiling instead of retrying forever.
func TestOutboxIntegration_BoundedRetriesParkEventAfterFiveAttempts(t *testing.T) {
	repo, service, _, chatMetrics := newOutboxIntegration(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "ob-retry-sender")
	circle := seedCircle(t, repo, "Outbox Retry Circle", sender)
	seedMember(t, repo, circle, sender, "teacher", time.Now().UTC().Add(-time.Hour))

	sent, err := service.SendText(ctx, sender, circle, "doomed projection", "ob-retry-key-1")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	var eventID string
	if err := repo.pool.QueryRow(ctx, `SELECT event_id::text FROM chat_event_outbox WHERE message_id = $1`, sent.ID).Scan(&eventID); err != nil {
		t.Fatalf("load event id: %v", err)
	}

	failing := &fakeChatProjector{err: errors.New("hub unavailable")}
	dispatcher := NewOutboxDispatcher(NewPGOutboxStore(repo), failing, chatMetrics, nil, nil, identityJitter)
	for attempt := 1; attempt <= 5; attempt++ {
		if err := dispatcher.DispatchDue(ctx, 10); err != nil {
			t.Fatalf("dispatch attempt %d: %v", attempt, err)
		}
		_, parked, attempts := outboxRowState(t, repo, eventID)
		if attempt < 5 {
			if parked || attempts != attempt {
				t.Fatalf("attempt %d state: parked=%v attempts=%d, want false/%d", attempt, parked, attempts, attempt)
			}
			rewindDueOutbox(t, repo)
			continue
		}
		if !parked || attempts != 5 {
			t.Fatalf("fifth attempt state: parked=%v attempts=%d, want true/5", parked, attempts)
		}
	}
	if got := chatMetrics.Summary().Outbox[metrics.ChatOutboxParked]; got != 1 {
		t.Fatalf("parked metric = %d, want 1", got)
	}
	rewindDueOutbox(t, repo)
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatalf("post-park dispatch: %v", err)
	}
	if _, _, attempts := outboxRowState(t, repo, eventID); attempts != 5 {
		t.Fatalf("parked events must never be re-claimed, attempts = %d", attempts)
	}
}

// TestOutboxIntegration_StartupReplayIncludesParkedEvents proves parked rows
// are re-armed with a fresh attempt budget and delivered by startup replay,
// and that the recovery audit fires against real Postgres claim semantics
// (RETURNING yields post-update values, so the pre-claim parked state must
// come from the claimed CTE's was_parked flag).
func TestOutboxIntegration_StartupReplayIncludesParkedEvents(t *testing.T) {
	repo, service, _, _ := newOutboxIntegration(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "ob-replay-sender")
	circle := seedCircle(t, repo, "Outbox Replay Circle", sender)
	seedMember(t, repo, circle, sender, "teacher", time.Now().UTC().Add(-time.Hour))

	sent, err := service.SendText(ctx, sender, circle, "parked then replayed", "ob-replay-key-1")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	var eventID string
	if err := repo.pool.QueryRow(ctx, `SELECT event_id::text FROM chat_event_outbox WHERE message_id = $1`, sent.ID).Scan(&eventID); err != nil {
		t.Fatalf("load event id: %v", err)
	}

	failing := NewOutboxDispatcher(NewPGOutboxStore(repo), &fakeChatProjector{err: errors.New("hub unavailable")}, nil, nil, nil, identityJitter)
	for attempt := 1; attempt <= 5; attempt++ {
		if err := failing.DispatchDue(ctx, 10); err != nil {
			t.Fatalf("dispatch attempt %d: %v", attempt, err)
		}
		rewindDueOutbox(t, repo)
	}
	if _, parked, _ := outboxRowState(t, repo, eventID); !parked {
		t.Fatal("fixture must park the event before replay")
	}

	if _, err := repo.pool.Exec(ctx, `UPDATE chat_event_outbox SET available_at = NOW() + INTERVAL '1 hour' WHERE event_id::text = $1`, eventID); err != nil {
		t.Fatalf("push availability forward: %v", err)
	}
	audit := &recordingAuditHandler{}
	working := NewOutboxDispatcher(NewPGOutboxStore(repo), &fakeChatProjector{}, nil, logging.NewAuditLogger(slog.New(audit)), nil, nil)
	if err := working.Replay(ctx, 10); err != nil {
		t.Fatalf("startup replay: %v", err)
	}
	delivered, parked, attempts := outboxRowState(t, repo, eventID)
	if !delivered || parked {
		t.Fatalf("replayed parked event state: delivered=%v parked=%v, want true/false", delivered, parked)
	}
	if attempts < 1 || attempts > 5 {
		t.Fatalf("replayed attempt count = %d, want within the fresh five-attempt budget", attempts)
	}
	if !strings.Contains(audit.rendered(), string(logging.ChatOutcomeRecovered)) {
		t.Fatalf("replayed parked event must audit recovery: %s", audit.rendered())
	}
}

// TestOutboxIntegration_DeletedMessageDeliverySkipped proves payload reload
// from current database truth: an event whose message was soft-deleted between
// send and dispatch completes without projection.
func TestOutboxIntegration_DeletedMessageDeliverySkipped(t *testing.T) {
	repo, service, _, _ := newOutboxIntegration(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "ob-deleted-sender")
	circle := seedCircle(t, repo, "Outbox Deleted Circle", sender)
	seedMember(t, repo, circle, sender, "teacher", time.Now().UTC().Add(-time.Hour))

	sent, err := service.SendText(ctx, sender, circle, "deleted before dispatch", "ob-deleted-key-1")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `UPDATE messages SET deleted_at = NOW() WHERE id = $1`, sent.ID); err != nil {
		t.Fatalf("soft-delete fixture: %v", err)
	}

	projector := &fakeChatProjector{}
	dispatcher := NewOutboxDispatcher(NewPGOutboxStore(repo), projector, nil, nil, nil, nil)
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatalf("dispatch due: %v", err)
	}
	if projector.count() != 0 {
		t.Fatalf("deleted message must not be projected, got %d calls", projector.count())
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox WHERE delivered_at IS NULL`); n != 0 {
		t.Fatalf("skipped events must be marked delivered, %d undelivered", n)
	}
}
