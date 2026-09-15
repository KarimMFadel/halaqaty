package chat

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

// PresenceService records durable chat presence facts.
type PresenceService struct {
	repo       *Repository
	membership MembershipReader
}

// NewPresenceService constructs the chat presence service.
func NewPresenceService(repo *Repository, membership MembershipReader) *PresenceService {
	return &PresenceService{repo: repo, membership: membership}
}

// MarkGroupMessageRead records one recipient read fact idempotently. The
// circle membership, archive state, and message visibility are rechecked and
// locked in the same transaction immediately before persistence.
func (s *PresenceService) MarkGroupMessageRead(ctx context.Context, readerID, circleID, messageID uuid.UUID) error {
	if err := authorizeActiveCircleMember(ctx, s.membership, readerID, circleID, func(metrics.ChatDenial) {}); err != nil {
		return err
	}
	err := s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockActiveCircleMember(ctx, circleID, readerID); err != nil {
			return err
		}
		_, err := tx.LockVisibleGroupMessageForRead(ctx, circleID, readerID, messageID)
		if err != nil {
			return err
		}
		inserted, err := tx.InsertMessageRead(ctx, messageID, readerID)
		if err != nil || !inserted {
			return err
		}
		return tx.InsertOutboxEvent(ctx, messageID, realtime.EventChatMessageRead, &readerID)
	})
	if err != nil {
		return fmt.Errorf("mark group message read: %w", err)
	}
	return nil
}

// MarkDirectMessageRead records one eligible recipient read fact idempotently.
// The pair relationship and target message are rechecked in the transaction
// immediately before persistence.
func (s *PresenceService) MarkDirectMessageRead(ctx context.Context, readerID, peerID, messageID uuid.UUID) error {
	err := s.repo.WithTx(ctx, func(tx *Tx) error {
		_, err := tx.LockEligibleDirectMessageForRead(ctx, readerID, peerID, messageID)
		if err != nil {
			return err
		}
		inserted, err := tx.InsertMessageRead(ctx, messageID, readerID)
		if err != nil || !inserted {
			return err
		}
		return tx.InsertOutboxEvent(ctx, messageID, realtime.EventChatMessageRead, &readerID)
	})
	if err != nil {
		return fmt.Errorf("mark direct message read: %w", err)
	}
	return nil
}
