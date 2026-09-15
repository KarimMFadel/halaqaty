package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	if err := s.repo.hydrateSenderReadReceipts(ctx, viewerID, msgs); err != nil {
		return nil, fmt.Errorf("load chat history read receipts: %w", err)
	}
	if err := s.repo.hydrateReplyPreviews(ctx, viewerID, msgs); err != nil {
		return nil, fmt.Errorf("load chat history reply previews: %w", err)
	}
	outcome := metrics.ChatOutcomeAccepted
	if len(msgs) == 0 {
		outcome = metrics.ChatOutcomeNoResults
	}
	s.metrics.RecordLatencyOutcome(metrics.ChatOperationHistory, outcome, time.Since(start))
	return msgs, nil
}

// Search returns one retained, non-deleted group-message search page for a
// current member, including retained history from an archived circle.
func (s *GroupService) Search(ctx context.Context, viewerID, circleID uuid.UUID, query string, before *uuid.UUID, limit int) ([]Message, error) {
	start := time.Now()
	trimmed, err := ValidateSearchQuery(query)
	if err != nil {
		return nil, err
	}
	if err := authorizeRetainedCircleMember(ctx, s.membership, viewerID, circleID, func(reason metrics.ChatDenial) {
		s.recordDenial(ctx, viewerID, circleID, reason)
	}); err != nil {
		return nil, err
	}
	messages, err := s.repo.GroupSearchPage(ctx, circleID, viewerID, trimmed, before, clampHistoryLimit(limit))
	if err != nil {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationSearch, metrics.ChatOutcomeFailure, time.Since(start))
		s.metrics.RecordSearch(metrics.ChatSearchFailure, time.Since(start))
		return nil, fmt.Errorf("search chat history: %w", err)
	}
	if err := s.repo.hydrateSenderReadReceipts(ctx, viewerID, messages); err != nil {
		s.metrics.RecordSearch(metrics.ChatSearchFailure, time.Since(start))
		return nil, fmt.Errorf("load chat search read receipts: %w", err)
	}
	if err := s.repo.hydrateReplyPreviews(ctx, viewerID, messages); err != nil {
		s.metrics.RecordSearch(metrics.ChatSearchFailure, time.Since(start))
		return nil, fmt.Errorf("load chat search reply previews: %w", err)
	}
	outcome := metrics.ChatOutcomeAccepted
	if len(messages) == 0 {
		outcome = metrics.ChatOutcomeNoResults
	}
	s.metrics.RecordLatencyOutcome(metrics.ChatOperationSearch, outcome, time.Since(start))
	searchOutcome := metrics.ChatSearchResults
	if len(messages) == 0 {
		searchOutcome = metrics.ChatSearchNoResults
	}
	s.metrics.RecordSearch(searchOutcome, time.Since(start))
	return messages, nil
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

// ReplyText sends a group text reply only after locking an active target in
// the sender's current membership period.
func (s *GroupService) ReplyText(ctx context.Context, senderID, circleID, replyToID uuid.UUID, content, idempotencyKey string) (Message, error) {
	trimmed, err := ValidateText(content)
	if err != nil {
		return Message{}, err
	}
	if err := ValidateIdempotencyKey(idempotencyKey); err != nil {
		return Message{}, err
	}
	if err := s.authorizeActiveMember(ctx, senderID, circleID); err != nil {
		return Message{}, err
	}
	var reply Message
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockActiveCircleMember(ctx, circleID, senderID); err != nil {
			return err
		}
		input := MessageInput{SenderID: senderID, CircleID: &circleID, Type: MessageTypeText, Content: trimmed, ReplyToID: &replyToID, IdempotencyKey: idempotencyKey}
		existing, found, err := tx.FindMessageByIdempotency(ctx, senderID, idempotencyKey)
		if err != nil {
			return err
		}
		if found {
			if !matchesMessageInput(existing, input) {
				return ErrIdempotencyConflict
			}
			reply = existing
			return nil
		}
		target, err := tx.LockVisibleGroupReplyTarget(ctx, circleID, senderID, replyToID)
		if err != nil {
			return err
		}
		message, inserted, err := tx.InsertMessage(ctx, input)
		if err != nil {
			return err
		}
		reply = message
		reply.ReplyPreview = safeReplyPreview(target)
		if !inserted {
			return nil
		}
		return tx.InsertOutboxEvent(ctx, message.ID, realtime.EventChatMessage, nil)
	})
	if err != nil {
		return Message{}, fmt.Errorf("reply to group message: %w", err)
	}
	messages := []Message{reply}
	if err := s.repo.hydrateReplyPreviews(ctx, senderID, messages); err != nil {
		return Message{}, fmt.Errorf("hydrate group reply preview: %w", err)
	}
	reply = messages[0]
	return reply, nil
}

// ListPinned returns the current member's visible pinned group messages.
func (s *GroupService) ListPinned(ctx context.Context, viewerID, circleID uuid.UUID) ([]Message, error) {
	if err := authorizeRetainedCircleMember(ctx, s.membership, viewerID, circleID, func(reason metrics.ChatDenial) {
		s.recordDenial(ctx, viewerID, circleID, reason)
	}); err != nil {
		return nil, err
	}
	messages, err := s.repo.ListPinnedMessages(ctx, circleID, viewerID)
	if err != nil {
		return nil, fmt.Errorf("list pinned chat messages: %w", err)
	}
	if err := s.repo.hydrateReplyPreviews(ctx, viewerID, messages); err != nil {
		return nil, fmt.Errorf("load pinned chat reply previews: %w", err)
	}
	return messages, nil
}

// Pin pins an eligible group message. The transaction's circle lock prevents
// concurrent callers from exceeding the five-message limit.
func (s *GroupService) Pin(ctx context.Context, actorID, circleID, messageID uuid.UUID) (bool, error) {
	return s.setPinned(ctx, actorID, circleID, messageID, true)
}

// Unpin removes an eligible group message pin under the same circle lock.
func (s *GroupService) Unpin(ctx context.Context, actorID, circleID, messageID uuid.UUID) (bool, error) {
	return s.setPinned(ctx, actorID, circleID, messageID, false)
}

func (s *GroupService) setPinned(ctx context.Context, actorID, circleID, messageID uuid.UUID, pin bool) (bool, error) {
	if err := s.authorizeActiveMember(ctx, actorID, circleID); err != nil {
		return false, err
	}
	changed := false
	err := s.repo.WithTx(ctx, func(tx *Tx) error {
		role, err := tx.LockCirclePinActor(ctx, circleID, actorID)
		if err != nil {
			return err
		}
		if role != rbac.RoleTeacher && role != rbac.RoleSupervisor {
			return rbac.ErrForbidden
		}
		if pin {
			alreadyPinned, err := tx.LockVisibleGroupMessagePinState(ctx, circleID, actorID, messageID)
			if err != nil {
				return err
			}
			if alreadyPinned {
				return nil
			}
			count, err := tx.CountPinnedMessages(ctx, circleID)
			if err != nil {
				return err
			}
			if count >= 5 {
				return ErrPinLimit
			}
			changed, err = tx.PinVisibleGroupMessage(ctx, circleID, actorID, messageID)
			return err
		}
		pinned, err := tx.LockVisibleGroupMessagePinState(ctx, circleID, actorID, messageID)
		if err != nil {
			return err
		}
		if !pinned {
			return nil
		}
		changed, err = tx.UnpinVisibleGroupMessage(ctx, circleID, actorID, messageID)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("set group message pin: %w", err)
	}
	return changed, nil
}

func safeReplyPreview(target Message) *ReplyPreview {
	preview := target.Content
	if preview == "" {
		preview = "Attachment"
	}
	runes := []rune(strings.TrimSpace(preview))
	if len(runes) > 160 {
		runes = runes[:160]
	}
	return &ReplyPreview{ID: target.ID, Preview: string(runes)}
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
