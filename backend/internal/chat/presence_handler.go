package chat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

// PresenceServiceAPI is the transport seam for durable chat presence facts.
type PresenceServiceAPI interface {
	MarkGroupMessageRead(ctx context.Context, readerID, circleID, messageID uuid.UUID) error
	MarkDirectMessageRead(ctx context.Context, readerID, messageID uuid.UUID) error
}

// NewTypingCommandHandler creates the non-persistent group typing command
// handler. Authorization is checked before broadcast and again for each
// subscribed connection; the sender never receives their own indicator.
func NewTypingCommandHandler(membership MembershipReader, hub *realtime.Hub) realtime.ChatCommandHandler {
	return func(ctx context.Context, command realtime.ChatCommand) error {
		circleRaw, ok := command.Payload[httpconst.FieldCircleID].(string)
		if !ok || command.Payload["dm_peer_id"] != nil {
			return ErrInvalidContext
		}
		circleID, err := uuid.Parse(circleRaw)
		if err != nil || membership == nil || hub == nil {
			return ErrInvalidContext
		}
		circle, err := membership.FindCircleByID(ctx, circleID.String())
		if err != nil || circle.IsArchived {
			return ErrCircleNotVisible
		}
		member, err := membership.IsMember(ctx, circleID.String(), command.Connection.UserID)
		if err != nil || !member {
			return ErrCircleNotVisible
		}
		isTyping, ok := command.Payload["is_typing"].(bool)
		if !ok {
			return ErrInvalidContext
		}
		topic, err := realtime.NewCircleTopic(circleID.String())
		if err != nil {
			return fmt.Errorf("build typing topic: %w", err)
		}
		eventID := command.RequestID
		if eventID == "" {
			eventID = uuid.NewString()
		}
		expiresAt := time.Now().UTC().Add(5 * time.Second)
		payload := map[string]any{
			"type":        realtime.EventChatTyping,
			"event_id":    eventID,
			"occurred_at": time.Now().UTC().Format(time.RFC3339Nano),
			"payload": map[string]any{
				"user_id":    command.Connection.UserID,
				"circle_id":  circleID.String(),
				"is_typing":  isTyping,
				"expires_at": expiresAt.Format(time.RFC3339Nano),
			},
		}
		return hub.BroadcastAuthorized(ctx, topic, realtime.AuthorizedDelivery{
			EventID: eventID,
			Payload: payload,
			Authorize: func(ctx context.Context, connection realtime.ConnectionIdentity) (bool, error) {
				if connection.UserID == command.Connection.UserID {
					return false, nil
				}
				return membership.IsMember(ctx, circleID.String(), connection.UserID)
			},
		})
	}
}

// MarkDirectMessageRead records the caller's read fact for an eligible DM.
func (h *PresenceHandler) MarkDirectMessageRead(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	if err := ValidateIdempotencyKey(r.Header.Get(httpconst.HeaderIdempotencyKey)); err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldIdempotencyKey: httpconst.ErrorMessageChatIdempotencyKeyInvalid})
		return
	}
	principal, ok := middleware.CurrentPrincipal(r.Context())
	if !ok {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	readerID, err := uuid.Parse(principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	if _, err := uuid.Parse(r.PathValue("userId")); err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldUserID: httpconst.ErrorMessageChatDMPeerIDInvalid})
		return
	}
	messageID, err := uuid.Parse(r.PathValue("messageId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldMessageID: httpconst.ErrorMessageChatMessageIDInvalid})
		return
	}
	if err := h.service.MarkDirectMessageRead(r.Context(), readerID, messageID); err != nil {
		if errors.Is(err, ErrDMNotEligible) {
			phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
			return
		}
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// PresenceHandler exposes group-message presence REST operations.
type PresenceHandler struct{ service PresenceServiceAPI }

// NewPresenceHandler constructs a presence handler over the service seam.
func NewPresenceHandler(service PresenceServiceAPI) *PresenceHandler {
	return &PresenceHandler{service: service}
}

// MarkCircleMessageRead records the caller's read fact, denying archived
// circles because retained archived history is read-only.
func (h *PresenceHandler) MarkCircleMessageRead(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	if err := ValidateIdempotencyKey(r.Header.Get(httpconst.HeaderIdempotencyKey)); err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldIdempotencyKey: httpconst.ErrorMessageChatIdempotencyKeyInvalid})
		return
	}
	principal, ok := middleware.CurrentPrincipal(r.Context())
	if !ok {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	readerID, err := uuid.Parse(principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	circleID, err := uuid.Parse(r.PathValue("circleId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldCircleID: httpconst.ErrorMessageCircleIDInvalid})
		return
	}
	messageID, err := uuid.Parse(r.PathValue("messageId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldMessageID: httpconst.ErrorMessageChatMessageIDInvalid})
		return
	}
	if err := h.service.MarkGroupMessageRead(r.Context(), readerID, circleID, messageID); err != nil {
		switch {
		case errors.Is(err, ErrCircleArchived):
			phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageCircleArchived, http.StatusConflict)
		case errors.Is(err, ErrCircleNotVisible):
			phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
		case errors.Is(err, ErrMessageNotVisible):
			phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorMessageForbidden, http.StatusNotFound)
		default:
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
