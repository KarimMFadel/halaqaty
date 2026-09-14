package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

// DirectService implements the unordered-pair direct conversation contract.
// Every operation asks PostgreSQL whether the current pair still has a
// qualifying active-circle relationship before reading or mutating data.
type DirectService struct {
	repo  *Repository
	media *UploadService
}

var ErrDirectDeleteConflict = errors.New("chat: direct message cannot be deleted")

// NewDirectService constructs the direct-message service.
func NewDirectService(repo *Repository, media *UploadService) *DirectService {
	return &DirectService{repo: repo, media: media}
}

// SetMediaService wires the configured object-store service after startup
// configuration has been loaded.
func (s *DirectService) SetMediaService(media *UploadService) { s.media = media }

// History returns the current eligible pair's newest-first message page.
func (s *DirectService) History(ctx context.Context, viewerID, peerID uuid.UUID, before *uuid.UUID, limit int) ([]Message, error) {
	if err := s.authorize(ctx, viewerID, peerID); err != nil {
		return nil, err
	}
	messages, err := s.repo.DMHistoryPage(ctx, viewerID, peerID, before, clampHistoryLimit(limit))
	if err != nil {
		return nil, err
	}
	if err := s.repo.hydrateSenderReadReceipts(ctx, viewerID, messages); err != nil {
		return nil, fmt.Errorf("load direct chat read receipts: %w", err)
	}
	return messages, nil
}

// SendText durably accepts one idempotent direct text message and creates
// targeted outbox projections for both authenticated pair members.
func (s *DirectService) SendText(ctx context.Context, senderID, peerID uuid.UUID, content, idempotencyKey string) (Message, error) {
	if senderID == peerID {
		return Message{}, ErrInvalidContext
	}
	trimmed, err := ValidateText(content)
	if err != nil {
		return Message{}, err
	}
	if err := ValidateIdempotencyKey(idempotencyKey); err != nil {
		return Message{}, err
	}
	if err := s.authorize(ctx, senderID, peerID); err != nil {
		return Message{}, err
	}

	var sent Message
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockQualifyingDMCircle(ctx, senderID, peerID); err != nil {
			return err
		}
		msg, inserted, err := tx.InsertMessage(ctx, MessageInput{
			SenderID:       senderID,
			DMRecipientID:  &peerID,
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
		if err := tx.InsertOutboxEvent(ctx, msg.ID, realtime.EventChatMessage, &peerID); err != nil {
			return err
		}
		return tx.InsertOutboxEvent(ctx, msg.ID, realtime.EventChatMessage, &senderID)
	})
	if err != nil {
		return Message{}, fmt.Errorf("send direct message: %w", err)
	}
	return sent, nil
}

// DeleteOwnMessage soft-deletes one recent direct message after rechecking
// current pair eligibility and emits a targeted deletion projection.
func (s *DirectService) DeleteOwnMessage(ctx context.Context, senderID, peerID, messageID uuid.UUID) error {
	if err := s.authorize(ctx, senderID, peerID); err != nil {
		return err
	}
	return s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockQualifyingDMCircle(ctx, senderID, peerID); err != nil {
			return err
		}
		if s.media != nil {
			if err := s.media.RevokeMessageMediaInTx(ctx, tx, messageID, senderID, peerID); err != nil {
				return err
			}
		}
		deleted, err := tx.DeleteOwnDirectMessage(ctx, messageID, senderID, peerID)
		if err != nil {
			return err
		}
		if !deleted {
			return nil
		}
		return tx.InsertOutboxEvent(ctx, messageID, realtime.EventChatMessageDeleted, &peerID)
	})
}

func (s *DirectService) authorize(ctx context.Context, viewerID, peerID uuid.UUID) error {
	if viewerID == peerID {
		return ErrDMNotEligible
	}
	if s.repo == nil {
		return fmt.Errorf("authorize direct message: chat repository is not configured")
	}
	_, eligible, err := s.repo.FindQualifyingDMCircle(ctx, viewerID, peerID)
	if err != nil {
		return fmt.Errorf("authorize direct message: %w", err)
	}
	if !eligible {
		return ErrDMNotEligible
	}
	return nil
}
