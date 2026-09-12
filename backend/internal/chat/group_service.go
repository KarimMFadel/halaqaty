package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

// DefaultHistoryPageSize is the group-history page size used when the caller
// omits or invalidates the limit.
const DefaultHistoryPageSize = 50

// MaxHistoryPageSize is the largest group-history page a client may receive.
const MaxHistoryPageSize = 100

var (
	// ErrCircleNotVisible denies group-chat access without distinguishing a
	// nonexistent circle from a circle the caller does not belong to, so
	// unauthorized callers cannot enumerate circles (FR-004).
	ErrCircleNotVisible = errors.New("chat: circle not visible")
	// ErrCircleArchived denies chat mutations on an archived circle while its
	// retained history stays readable (FR-032).
	ErrCircleArchived = errors.New("chat: circle is archived")
)

// MembershipReader answers current circle membership and lifecycle questions
// from F-002-owned state. Authorization is re-read from PostgreSQL at
// operation time (SR-002); *rbac.Repository satisfies it in production.
type MembershipReader interface {
	// IsMember reports whether userID currently belongs to circleID.
	IsMember(ctx context.Context, circleID, userID string) (bool, error)
	// FindCircleByID loads one circle including its archived state.
	FindCircleByID(ctx context.Context, circleID string) (rbac.Circle, error)
}

type membershipPeriodReader interface {
	MembershipStartedAt(context.Context, string, string) (time.Time, error)
}

// GroupService implements US1 group-chat authorization and business behavior
// over durable chat persistence. It is session-independent by construction:
// identity and backend sessions are verified by HTTP middleware, and every
// protected operation rechecks current circle membership from PostgreSQL.
type GroupService struct {
	repo       *Repository
	membership MembershipReader
	metrics    *metrics.ChatMetrics
	audit      *logging.AuditLogger
}

// NewGroupService constructs the group chat service. chatMetrics and audit are
// optional; a limiter (30 sends/minute per user and circle) can wrap SendText
// at the transport layer without service changes.
func NewGroupService(repo *Repository, membership MembershipReader, chatMetrics *metrics.ChatMetrics, audit *logging.AuditLogger) *GroupService {
	return &GroupService{repo: repo, membership: membership, metrics: chatMetrics, audit: audit}
}

// History returns one circle history page for a current member in (sent_at,
// id) DESC order, restricted to the viewer's current membership period. Both
// non-members and unknown circles return ErrCircleNotVisible identically.
func (s *GroupService) History(ctx context.Context, viewerID, circleID uuid.UUID, before *uuid.UUID, limit int) ([]Message, error) {
	start := time.Now()
	if err := authorizeRetainedCircleMember(ctx, s.membership, viewerID, circleID, func(reason metrics.ChatDenial) {
		s.recordDenial(ctx, viewerID, circleID, reason)
	}); err != nil {
		return nil, err
	}
	msgs, err := s.repo.GroupHistoryPage(ctx, circleID, viewerID, before, clampHistoryLimit(limit))
	if err != nil {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationHistory, metrics.ChatOutcomeFailure, time.Since(start))
		return nil, fmt.Errorf("load chat history: %w", err)
	}
	outcome := metrics.ChatOutcomeAccepted
	if len(msgs) == 0 {
		outcome = metrics.ChatOutcomeNoResults
	}
	s.metrics.RecordLatencyOutcome(metrics.ChatOperationHistory, outcome, time.Since(start))
	return msgs, nil
}

// SendText durably accepts one group text message for a current member of an
// active circle. The message and its identifier-only chat.message outbox event
// commit atomically; a replayed (sender, idempotency key) returns the committed
// original without a second row or event (FR-007).
func (s *GroupService) SendText(ctx context.Context, senderID, circleID uuid.UUID, content, idempotencyKey string) (Message, error) {
	start := time.Now()
	trimmed, err := ValidateText(content)
	if err != nil {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationSend, metrics.ChatOutcomeRejected, time.Since(start))
		return Message{}, err
	}
	if err := ValidateIdempotencyKey(idempotencyKey); err != nil {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationSend, metrics.ChatOutcomeRejected, time.Since(start))
		return Message{}, err
	}
	if err := s.authorizeActiveMember(ctx, senderID, circleID); err != nil {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationSend, metrics.ChatOutcomeDenied, time.Since(start))
		return Message{}, err
	}

	var sent Message
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockActiveCircleMember(ctx, circleID, senderID); err != nil {
			return err
		}
		msg, inserted, err := tx.InsertMessage(ctx, MessageInput{
			SenderID:       senderID,
			CircleID:       &circleID,
			Type:           MessageTypeText,
			Content:        trimmed,
			IdempotencyKey: idempotencyKey,
		})
		if err != nil {
			return err
		}
		sent = msg
		if !inserted {
			return nil
		}
		return tx.InsertOutboxEvent(ctx, msg.ID, realtime.EventChatMessage, nil)
	})
	if err != nil {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationSend, metrics.ChatOutcomeFailure, time.Since(start))
		return Message{}, fmt.Errorf("send chat message: %w", err)
	}
	s.metrics.RecordLatencyOutcome(metrics.ChatOperationSend, metrics.ChatOutcomeAccepted, time.Since(start))
	if s.audit != nil {
		s.audit.LogChat(ctx, logging.ChatMessageAuditEvent(senderID.String(), circleID.String(), sent.ID.String(), logging.ChatOutcomeAccepted))
	}
	return sent, nil
}

// authorizeActiveMember rechecks current circle state from PostgreSQL for a
// mutating operation: the circle must exist, be unarchived, and currently
// contain the sender (FR-001, FR-032, SR-002). Unknown circles and
// non-members are denied identically so callers cannot enumerate circles.
func (s *GroupService) authorizeActiveMember(ctx context.Context, actorID, circleID uuid.UUID) error {
	return authorizeActiveCircleMember(ctx, s.membership, actorID, circleID, func(reason metrics.ChatDenial) {
		s.recordDenial(ctx, actorID, circleID, reason)
	})
}

// denialRecorder records one bounded denial reason for an actor and circle.
type denialRecorder func(reason metrics.ChatDenial)

// authorizeActiveCircleMember rechecks current circle state from PostgreSQL
// for a mutating chat operation: the circle must exist, be unarchived, and
// currently contain the actor (FR-001, FR-032, SR-002). Unknown circles and
// non-members are denied identically so callers cannot enumerate circles.
func authorizeActiveCircleMember(ctx context.Context, membership MembershipReader, actorID, circleID uuid.UUID, deny denialRecorder) error {
	circle, err := membership.FindCircleByID(ctx, circleID.String())
	if errors.Is(err, rbac.ErrCircleNotFound) {
		deny(metrics.ChatDenialIneligible)
		return ErrCircleNotVisible
	}
	if err != nil {
		return fmt.Errorf("load chat circle: %w", err)
	}
	member, err := membership.IsMember(ctx, circleID.String(), actorID.String())
	if err != nil {
		return fmt.Errorf("authorize chat membership: %w", err)
	}
	if !member {
		deny(metrics.ChatDenialIneligible)
		return ErrCircleNotVisible
	}
	if circle.IsArchived {
		deny(metrics.ChatDenialArchived)
		return ErrCircleArchived
	}
	return nil
}

// recordDenial records one bounded authorization denial in metrics and a
// redacted audit event; denial records never carry message content (SR-006).
func (s *GroupService) recordDenial(ctx context.Context, actor, circle uuid.UUID, reason metrics.ChatDenial) {
	recordChatDenial(ctx, s.metrics, s.audit, actor, circle, reason)
}

// recordChatDenial records one bounded authorization denial in metrics and a
// redacted audit event; denial records never carry message content (SR-006).
func recordChatDenial(ctx context.Context, chatMetrics *metrics.ChatMetrics, audit *logging.AuditLogger, actor, circle uuid.UUID, reason metrics.ChatDenial) {
	chatMetrics.RecordDenial(reason)
	if audit != nil {
		audit.LogChat(ctx, logging.ChatDenialAuditEvent(actor.String(), circle.String(), circle.String(), logging.ChatOutcomeDenied))
	}
}

// clampHistoryLimit bounds a requested page size to the documented default and
// maximum.
func clampHistoryLimit(limit int) int {
	if limit < 1 {
		return DefaultHistoryPageSize
	}
	if limit > MaxHistoryPageSize {
		return MaxHistoryPageSize
	}
	return limit
}
