package chat

import (
	"context"
	"errors"

	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/google/uuid"
)

// ModerationService is the transport seam for message deletion.
type ModerationService interface {
	Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
}

// ModerationServiceImpl applies the marker-before-commit deletion protocol.
type ModerationServiceImpl struct {
	repo  *Repository
	media *MediaStore
}

// NewModerationService constructs the chat moderation service.
func NewModerationService(repo *Repository, media *MediaStore) *ModerationServiceImpl {
	return &ModerationServiceImpl{repo: repo, media: media}
}

// Delete soft-deletes a group message as its sender within ten server minutes
// or as the current circle teacher. A nil circleID identifies a direct message;
// direct deletion is restricted to the sender and an eligible pair.
func (s *ModerationServiceImpl) Delete(ctx context.Context, actorID, circleID, messageID uuid.UUID) error {
	if s == nil || s.repo == nil {
		return errors.New("delete chat message: repository is not configured")
	}
	return s.repo.WithTx(ctx, func(tx *Tx) error {
		message, err := tx.LockMessageForModeration(ctx, messageID)
		if err != nil {
			return err
		}
		teacher := false
		if message.CircleID != nil {
			if circleID != *message.CircleID {
				return ErrMessageNotVisible
			}
			role, err := tx.LockCirclePinActor(ctx, circleID, actorID)
			if err != nil {
				return err
			}
			teacher = role == "teacher"
			if actorID != message.SenderID && !teacher {
				return ErrMessageNotVisible
			}
		} else {
			if message.DMRecipientID == nil || actorID != message.SenderID {
				return ErrMessageNotVisible
			}
			if err := tx.LockQualifyingDMCircle(ctx, message.SenderID, *message.DMRecipientID); err != nil {
				return err
			}
		}
		if message.State == MessageStateDeleted {
			return nil
		}
		serverNow, allowed, err := tx.moderationDeleteWindow(ctx, messageID, actorID, teacher)
		if err != nil {
			return err
		}
		if !allowed {
			return ErrDirectDeleteConflict
		}
		if message.UploadID != nil {
			if s.media == nil {
				return errors.New("delete chat message: media store is not configured")
			}
			upload, err := tx.LoadMessageUploadForDelete(ctx, messageID)
			if err != nil {
				return err
			}
			if err := s.media.ApplyDeleteMarker(ctx, upload.ObjectKey); err != nil {
				return err
			}
		}
		changed, err := tx.SoftDeleteMessage(ctx, messageID, actorID, teacher, serverNow)
		if err != nil {
			return err
		}
		if !changed {
			return nil
		}
		if teacher && message.CircleID != nil {
			if _, err := tx.InsertModerationAudit(ctx, ModerationAudit{MessageID: messageID, CircleID: *message.CircleID, ActorID: actorID, Action: ModerationActionTeacherDelete}); err != nil {
				return err
			}
		}
		return tx.InsertOutboxEvent(ctx, messageID, realtime.EventChatMessageDeleted, message.DMRecipientID)
	})
}
