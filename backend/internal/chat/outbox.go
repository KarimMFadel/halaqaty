package chat

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
)

const (
	// chatOutboxAttemptLimit is the delivery-attempt ceiling before parking.
	chatOutboxAttemptLimit = 5
	// chatOutboxMaxRetryDelay caps any single jittered retry delay.
	chatOutboxMaxRetryDelay = 30 * time.Second
	// DefaultOutboxDispatchInterval is the periodic dispatch cadence used when
	// Run receives a non-positive interval.
	DefaultOutboxDispatchInterval = 5 * time.Second
)

// ErrMessageNotProjectable indicates the message referenced by an outbox event
// no longer has a projectable durable state (for example it was soft-deleted);
// the event completes without projection.
var ErrMessageNotProjectable = errors.New("chat: message not projectable")

// OutboxStore is the persistence seam required by asynchronous chat-event
// delivery; PGOutboxStore implements it on PostgreSQL.
type OutboxStore interface {
	// ClaimDueEvents leases due, undelivered, unparked events for dispatch.
	ClaimDueEvents(ctx context.Context, limit int) ([]OutboxEvent, error)
	// ClaimReplayEvents leases due pending and parked events for replay.
	ClaimReplayEvents(ctx context.Context, limit int) ([]OutboxEvent, error)
	// MarkDelivered records successful delivery.
	MarkDelivered(ctx context.Context, eventID uuid.UUID) error
	// ScheduleRetry reschedules one failed delivery attempt.
	ScheduleRetry(ctx context.Context, eventID uuid.UUID, availableAt time.Time) error
	// Park records retry exhaustion for explicit replay.
	Park(ctx context.Context, eventID uuid.UUID) error
	// LoadMessage reloads the durable message backing an event.
	LoadMessage(ctx context.Context, messageID uuid.UUID) (Message, error)
}

// ChatEventProjector projects one reloaded chat event to realtime clients;
// *RealtimeProjector implements it in production.
type ChatEventProjector interface {
	// ProjectMessage delivers the event for the reloaded message.
	ProjectMessage(ctx context.Context, event OutboxEvent, msg Message) error
}

// OutboxDispatcher performs at-least-once projection of committed chat events
// without changing durable message truth when realtime delivery fails: five
// bounded attempts with jittered exponential backoff, then parking.
type OutboxDispatcher struct {
	store     OutboxStore
	projector ChatEventProjector
	metrics   *metrics.ChatMetrics
	audit     *logging.AuditLogger
	now       func() time.Time
	jitter    func(time.Duration) time.Duration
}

// NewOutboxDispatcher constructs a dispatcher. now and jitter are injected to
// keep retry behavior deterministic in tests; nil selects the wall clock and
// the bounded ±10% default jitter.
func NewOutboxDispatcher(store OutboxStore, projector ChatEventProjector, chatMetrics *metrics.ChatMetrics, audit *logging.AuditLogger, now func() time.Time, jitter func(time.Duration) time.Duration) *OutboxDispatcher {
	if now == nil {
		now = time.Now
	}
	if jitter == nil {
		jitter = defaultChatOutboxJitter
	}
	return &OutboxDispatcher{store: store, projector: projector, metrics: chatMetrics, audit: audit, now: now, jitter: jitter}
}

// DispatchDue delivers the currently due events.
func (d *OutboxDispatcher) DispatchDue(ctx context.Context, limit int) error {
	events, err := d.store.ClaimDueEvents(ctx, limit)
	if err != nil {
		return fmt.Errorf("claim due chat outbox events: %w", err)
	}
	return d.dispatchAll(ctx, events)
}

// dispatchAll delivers claimed events in order; the first infrastructure
// failure aborts the batch and is retried by the next dispatch cycle.
func (d *OutboxDispatcher) dispatchAll(ctx context.Context, events []OutboxEvent) error {
	for _, event := range events {
		if err := d.Dispatch(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// Replay delivers due pending and parked events after startup recovery or on
// operator request; a successfully replayed parked event is audited as
// recovered.
func (d *OutboxDispatcher) Replay(ctx context.Context, limit int) error {
	events, err := d.store.ClaimReplayEvents(ctx, limit)
	if err != nil {
		return fmt.Errorf("claim chat outbox replay: %w", err)
	}
	for _, event := range events {
		parked := event.WasParked
		if err := d.Dispatch(ctx, event); err != nil {
			return err
		}
		if parked && d.audit != nil {
			d.audit.LogChat(ctx, logging.ChatOutboxAuditEvent("", event.EventID.String(), logging.ChatOutcomeRecovered))
		}
	}
	return nil
}

// Dispatch delivers one claimed event or schedules its retry/parking. The
// claim already counted this attempt, so event.AttemptCount is the number of
// attempts made including this one.
func (d *OutboxDispatcher) Dispatch(ctx context.Context, event OutboxEvent) error {
	if d.store == nil || d.projector == nil {
		return fmt.Errorf("chat outbox dispatcher requires store and projector")
	}
	msg, err := d.store.LoadMessage(ctx, event.MessageID)
	if errors.Is(err, ErrMessageNotProjectable) {
		if err := d.store.MarkDelivered(ctx, event.EventID); err != nil {
			return fmt.Errorf("mark skipped chat outbox event delivered: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("reload chat outbox payload: %w", err)
	}
	start := d.now()
	// The projection error is deliberately not propagated or logged: bounded
	// retry owns recovery and the parked metric plus audit surface exhaustion,
	// so committed message truth never depends on realtime availability.
	projectErr := d.projector.ProjectMessage(ctx, event, msg)
	if projectErr == nil {
		if err := d.store.MarkDelivered(ctx, event.EventID); err != nil {
			return fmt.Errorf("mark chat outbox event delivered: %w", err)
		}
		d.metrics.RecordOutbox(metrics.ChatOutboxDelivered)
		d.metrics.RecordLatencyOutcome(metrics.ChatOperationOutbox, metrics.ChatOutcomeAccepted, d.now().Sub(start))
		return nil
	}
	if event.AttemptCount >= chatOutboxAttemptLimit {
		if parkErr := d.store.Park(ctx, event.EventID); parkErr != nil {
			return fmt.Errorf("park exhausted chat outbox event: %w", parkErr)
		}
		d.metrics.RecordOutbox(metrics.ChatOutboxParked)
		if d.audit != nil {
			d.audit.LogChat(ctx, logging.ChatOutboxAuditEvent("", event.EventID.String(), logging.ChatOutcomeParked))
		}
		return nil
	}
	if err := d.store.ScheduleRetry(ctx, event.EventID, d.now().UTC().Add(chatOutboxRetryDelay(event.AttemptCount, d.jitter))); err != nil {
		return fmt.Errorf("schedule chat outbox retry: %w", err)
	}
	d.metrics.RecordOutbox(metrics.ChatOutboxRetried)
	return nil
}

// Run drives dispatch until ctx is canceled. Startup replay runs once before
// the periodic loop so parked backlog and due events recover immediately. A
// startup replay failure is non-fatal, mirroring the periodic loop's tolerance
// of DispatchDue failures: parked backlog then waits for the next restart
// replay while pending events keep flowing.
func (d *OutboxDispatcher) Run(ctx context.Context, limit int, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultOutboxDispatchInterval
	}
	start := d.now()
	if err := d.Replay(ctx, limit); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
		d.metrics.RecordLatencyOutcome(metrics.ChatOperationOutbox, metrics.ChatOutcomeFailure, d.now().Sub(start))
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		_ = d.DispatchDue(ctx, limit) // the next cycle retries transient store/projector failures
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// chatOutboxRetryDelay computes the jittered exponential backoff after the
// failed attempt n (1-based): 1/2/4/8 seconds, capped at 30 seconds.
func chatOutboxRetryDelay(attempt int, jitter func(time.Duration) time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(1) << (attempt - 1) * time.Second
	delay = jitter(delay)
	if delay < 0 {
		delay = 0
	}
	if delay > chatOutboxMaxRetryDelay {
		delay = chatOutboxMaxRetryDelay
	}
	return delay
}

// defaultChatOutboxJitter applies bounded ±10% jitter to avoid synchronized
// retry bursts while preserving the configured exponential schedule.
func defaultChatOutboxJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	delta := delay / 10
	if delta == 0 {
		return delay
	}
	return delay - delta + time.Duration(rand.Int63n(int64(2*delta)+1))
}

// PGOutboxStore is the PostgreSQL implementation of OutboxStore. Event claims
// reuse the repository's FOR UPDATE SKIP LOCKED statements; lifecycle updates
// run as single guarded statements.
type PGOutboxStore struct {
	repo *Repository
}

// NewPGOutboxStore constructs the PostgreSQL chat outbox store.
func NewPGOutboxStore(repo *Repository) *PGOutboxStore { return &PGOutboxStore{repo: repo} }

// ClaimDueEvents implements OutboxStore using the repository claim.
func (s *PGOutboxStore) ClaimDueEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	return s.repo.ClaimOutboxEvents(ctx, limit)
}

// ClaimReplayEvents implements OutboxStore, leasing pending and parked rows.
func (s *PGOutboxStore) ClaimReplayEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	if limit < 1 {
		return []OutboxEvent{}, nil
	}
	var events []OutboxEvent
	err := s.repo.WithTx(ctx, func(tx *Tx) error {
		rows, err := tx.tx.Query(ctx, claimReplayChatOutboxEventsQuery, limit)
		if err != nil {
			return fmt.Errorf("claim chat outbox replay: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			event, err := scanReplayOutboxEvent(rows)
			if err != nil {
				return fmt.Errorf("scan replayed chat outbox event: %w", err)
			}
			events = append(events, event)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate replayed chat outbox events: %w", err)
		}
		return nil
	})
	return events, err
}

// MarkDelivered implements OutboxStore.
func (s *PGOutboxStore) MarkDelivered(ctx context.Context, eventID uuid.UUID) error {
	if _, err := s.repo.pool.Exec(ctx, markChatOutboxEventDeliveredQuery, eventID); err != nil {
		return fmt.Errorf("mark chat outbox event delivered: %w", err)
	}
	return nil
}

// ScheduleRetry implements OutboxStore; the claim statements already counted
// the attempt.
func (s *PGOutboxStore) ScheduleRetry(ctx context.Context, eventID uuid.UUID, availableAt time.Time) error {
	if _, err := s.repo.pool.Exec(ctx, retryChatOutboxEventQuery, eventID, availableAt); err != nil {
		return fmt.Errorf("schedule chat outbox retry: %w", err)
	}
	return nil
}

// Park implements OutboxStore; the schema enforces parking only at the
// five-attempt ceiling.
func (s *PGOutboxStore) Park(ctx context.Context, eventID uuid.UUID) error {
	if _, err := s.repo.pool.Exec(ctx, parkChatOutboxEventQuery, eventID); err != nil {
		return fmt.Errorf("park chat outbox event: %w", err)
	}
	return nil
}

// LoadMessage implements OutboxStore by reloading the durable message from
// current PostgreSQL state; soft-deleted messages report
// ErrMessageNotProjectable.
func (s *PGOutboxStore) LoadMessage(ctx context.Context, messageID uuid.UUID) (Message, error) {
	msg, err := scanMessage(s.repo.pool.QueryRow(ctx, findMessageForProjectionQuery, messageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrMessageNotProjectable
	}
	if err != nil {
		return Message{}, fmt.Errorf("load chat message for projection: %w", err)
	}
	return msg, nil
}
