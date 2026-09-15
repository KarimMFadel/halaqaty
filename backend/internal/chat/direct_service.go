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
	if err := s.repo.hydrateReplyPreviews(ctx, viewerID, messages); err != nil {
		return nil, fmt.Errorf("load direct chat reply previews: %w", err)
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

// ReplyText sends a direct reply only when its target belongs to the same
// currently eligible unordered pair.
func (s *DirectService) ReplyText(ctx context.Context, senderID, peerID, replyToID uuid.UUID, content, idempotencyKey string) (Message, error) {
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
	var reply Message
	err = s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockQualifyingDMCircle(ctx, senderID, peerID); err != nil {
			return err
		}
		input := MessageInput{SenderID: senderID, DMRecipientID: &peerID, Type: MessageTypeText, Content: trimmed, ReplyToID: &replyToID, IdempotencyKey: idempotencyKey}
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
		target, err := tx.LockVisibleDirectReplyTarget(ctx, senderID, peerID, replyToID)
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
		if err := tx.InsertOutboxEvent(ctx, message.ID, realtime.EventChatMessage, &peerID); err != nil {
			return err
		}
		return tx.InsertOutboxEvent(ctx, message.ID, realtime.EventChatMessage, &senderID)
	})
	if err != nil {
		return Message{}, fmt.Errorf("reply to direct message: %w", err)
	}
	messages := []Message{reply}
	if err := s.repo.hydrateReplyPreviews(ctx, senderID, messages); err != nil {
		return Message{}, fmt.Errorf("hydrate direct reply preview: %w", err)
	}
	reply = messages[0]
	return reply, nil
}

// DeleteOwnMessage soft-deletes one recent direct message after rechecking
// current pair eligibility and emits a targeted deletion projection.
func (s *DirectService) DeleteOwnMessage(ctx context.Context, senderID, peerID, messageID uuid.UUID) error {
	if err := s.authorize(ctx, senderID, peerID); err != nil {
		return err
	}
	var store *MediaStore
	if s.media != nil {
		store = s.media.store
	}
	return NewModerationService(s.repo, store).Delete(ctx, senderID, uuid.Nil, messageID)
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
