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
	MarkDirectMessageRead(ctx context.Context, readerID, peerID, messageID uuid.UUID) error
}

// NewTypingCommandHandler creates the non-persistent typing command handler.
// The projector supplies the same immediate ticket, backend-session, and
// current conversation authorization used for durable realtime delivery.
func NewTypingCommandHandler(projector *RealtimeProjector) realtime.ChatCommandHandler {
	return func(ctx context.Context, command realtime.ChatCommand) error {
		if projector == nil || projector.hub == nil {
			return ErrInvalidContext
		}
		if _, err := uuid.Parse(command.RequestID); err != nil {
			return ErrInvalidContext
		}
		isTyping, ok := command.Payload["is_typing"].(bool)
		if !ok {
			return ErrInvalidContext
		}
		circleRaw, hasCircle := command.Payload[httpconst.FieldCircleID].(string)
		peerRaw, hasPeer := command.Payload["dm_peer_id"].(string)
		if hasCircle == hasPeer {
			return ErrInvalidContext
		}
		var err error
		if hasCircle {
			err = projector.broadcastGroupTyping(ctx, command, circleRaw, isTyping)
		} else {
			err = projector.broadcastDirectTyping(ctx, command, peerRaw, isTyping)
		}
		if errors.Is(err, ErrCircleNotVisible) || errors.Is(err, ErrDMNotEligible) {
			return realtime.NewAuthorizationError(err)
		}
		return err
	}
}

func (p *RealtimeProjector) broadcastGroupTyping(ctx context.Context, command realtime.ChatCommand, circleRaw string, isTyping bool) error {
	circleID, err := uuid.Parse(circleRaw)
	if err != nil || p.membership == nil {
		return ErrInvalidContext
	}
	circle, err := p.membership.FindCircleByID(ctx, circleID.String())
	if err != nil || circle.IsArchived {
		return ErrCircleNotVisible
	}
	authorize := p.authorizeTypingCircle(circleID)
	allowed, err := authorize(ctx, command.Connection)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrCircleNotVisible
	}
	topic, err := realtime.NewCircleTopic(circleID.String())
	if err != nil {
		return fmt.Errorf("build typing topic: %w", err)
	}
	return p.hub.BroadcastAuthorized(ctx, topic, realtime.AuthorizedDelivery{EventID: typingEventID(command), Payload: typingPayload(command, isTyping, circleID.String(), ""), Authorize: func(ctx context.Context, connection realtime.ConnectionIdentity) (bool, error) {
		if connection.UserID == command.Connection.UserID {
			return false, nil
		}
		return authorize(ctx, connection)
	}})
}

func (p *RealtimeProjector) broadcastDirectTyping(ctx context.Context, command realtime.ChatCommand, peerRaw string, isTyping bool) error {
	peerID, err := uuid.Parse(peerRaw)
	if err != nil {
		return ErrInvalidContext
	}
	authorize := p.authorizeTypingDirect(peerID)
	allowed, err := authorize(ctx, command.Connection)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrDMNotEligible
	}
	return p.hub.SendToUsers(ctx, []string{peerID.String()}, realtime.AuthorizedDelivery{EventID: typingEventID(command), Payload: typingPayload(command, isTyping, "", command.Connection.UserID), Authorize: func(ctx context.Context, connection realtime.ConnectionIdentity) (bool, error) {
		if connection.UserID == command.Connection.UserID {
			return false, nil
		}
		return p.authorizeTypingDirect(uuid.MustParse(command.Connection.UserID))(ctx, connection)
	}})
}

func typingEventID(command realtime.ChatCommand) string {
	if command.RequestID != "" {
		return command.RequestID
	}
	return uuid.NewString()
}

func typingPayload(command realtime.ChatCommand, isTyping bool, circleID, peerID string) map[string]any {
	now := time.Now().UTC()
	payload := map[string]any{"user_id": command.Connection.UserID, "circle_id": nil, "dm_peer_id": nil, "is_typing": isTyping, "expires_at": now.Add(5 * time.Second).Format(time.RFC3339Nano)}
	if circleID != "" {
		payload["circle_id"] = circleID
	}
	if peerID != "" {
		payload["dm_peer_id"] = peerID
	}
	return map[string]any{"type": realtime.EventChatTyping, "event_id": typingEventID(command), "occurred_at": now.Format(time.RFC3339Nano), "payload": payload}
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
	peerID, err := uuid.Parse(r.PathValue("userId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldUserID: httpconst.ErrorMessageChatDMPeerIDInvalid})
		return
	}
	messageID, err := uuid.Parse(r.PathValue("messageId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldMessageID: httpconst.ErrorMessageChatMessageIDInvalid})
		return
	}
	if err := h.service.MarkDirectMessageRead(r.Context(), readerID, peerID, messageID); err != nil {
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
