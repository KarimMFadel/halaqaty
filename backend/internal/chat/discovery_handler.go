package chat

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// GroupReplyService is the optional reply capability implemented by the
// production group service. It stays separate from GroupChatService so older
// text-only adapters remain source-compatible.
type GroupReplyService interface {
	ReplyText(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, string) (Message, error)
}

type groupPinnedReader interface {
	ListPinned(context.Context, uuid.UUID, uuid.UUID) ([]Message, error)
}

type groupPinMutator interface {
	Pin(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
	Unpin(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (bool, error)
}

// DiscoveryHandler contains the transport adapters for US6 discovery and pin
// operations. Production GroupService supplies all optional capabilities.
type DiscoveryHandler struct {
	service GroupChatService
}

// ErrDiscoveryNotConfigured indicates that the persistent discovery
// capability was not supplied to the handler. Discovery operations fail
// closed rather than fabricating process-local state.
var ErrDiscoveryNotConfigured = errors.New("chat: discovery capability is not configured")

// NewDiscoveryHandler constructs a discovery handler over a group service.
func NewDiscoveryHandler(service GroupChatService) *DiscoveryHandler {
	return &DiscoveryHandler{service: service}
}

// ListPinnedCircleMessages returns the authorized active pinned bar.
func (h *GroupHandler) ListPinnedCircleMessages(w http.ResponseWriter, r *http.Request) {
	if h.discovery == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	viewerID, circleID, ok := discoveryIDs(w, r)
	if !ok {
		return
	}
	messages, err := h.discovery.listPinned(r.Context(), viewerID, circleID)
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	page := paginatedMessagesResponse{Data: make([]messageResponse, 0, len(messages))}
	for _, message := range messages {
		if len(page.Data) == 5 {
			break
		}
		if message.CircleID == nil || message.State == MessageStateDeleted || message.DeletedAt != nil {
			continue
		}
		response, err := h.projectMessage(r.Context(), viewerID, message)
		if err != nil {
			writeMediaRenewalError(w, err)
			return
		}
		page.Data = append(page.Data, response)
	}
	phttp.WriteJSON(w, http.StatusOK, page)
}

// PinCircleMessage pins one authorized group message and returns its safe
// projection.
func (h *GroupHandler) PinCircleMessage(w http.ResponseWriter, r *http.Request) {
	if h.discovery == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	actorID, circleID, messageID, ok := discoveryMessageIDs(w, r)
	if !ok {
		return
	}
	if err := ValidateIdempotencyKey(r.Header.Get(httpconst.HeaderIdempotencyKey)); err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldIdempotencyKey: httpconst.ErrorMessageChatIdempotencyKeyInvalid})
		return
	}
	message, err := h.discovery.pin(r.Context(), actorID, circleID, messageID)
	if err != nil {
		writeDiscoveryError(w, err)
		return
	}
	response, err := h.projectMessage(r.Context(), actorID, message)
	if err != nil {
		writeMediaRenewalError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, response)
}

// UnpinCircleMessage removes one authorized group-message pin.
func (h *GroupHandler) UnpinCircleMessage(w http.ResponseWriter, r *http.Request) {
	if h.discovery == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	actorID, circleID, messageID, ok := discoveryMessageIDs(w, r)
	if !ok {
		return
	}
	if err := ValidateIdempotencyKey(r.Header.Get(httpconst.HeaderIdempotencyKey)); err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldIdempotencyKey: httpconst.ErrorMessageChatIdempotencyKeyInvalid})
		return
	}
	if err := h.discovery.unpin(r.Context(), actorID, circleID, messageID); err != nil {
		writeDiscoveryError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *DiscoveryHandler) listPinned(ctx context.Context, viewerID, circleID uuid.UUID) ([]Message, error) {
	if reader, ok := h.service.(groupPinnedReader); ok {
		return reader.ListPinned(ctx, viewerID, circleID)
	}
	return nil, ErrDiscoveryNotConfigured
}

func (h *DiscoveryHandler) pin(ctx context.Context, actorID, circleID, messageID uuid.UUID) (Message, error) {
	if mutator, ok := h.service.(groupPinMutator); ok {
		if _, err := mutator.Pin(ctx, actorID, circleID, messageID); err != nil {
			return Message{}, err
		}
		if reader, ok := h.service.(groupPinnedReader); ok {
			messages, err := reader.ListPinned(ctx, actorID, circleID)
			if err != nil {
				return Message{}, err
			}
			for _, message := range messages {
				if message.ID == messageID {
					return message, nil
				}
			}
			return Message{}, ErrMessageNotVisible
		}
	}
	return Message{}, ErrDiscoveryNotConfigured
}

func (h *DiscoveryHandler) unpin(ctx context.Context, actorID, circleID, messageID uuid.UUID) error {
	if mutator, ok := h.service.(groupPinMutator); ok {
		_, err := mutator.Unpin(ctx, actorID, circleID, messageID)
		return err
	}
	return ErrDiscoveryNotConfigured
}

func discoveryIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	principal, ok := middleware.CurrentPrincipal(r.Context())
	if !ok {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return uuid.Nil, uuid.Nil, false
	}
	actorID, err := uuid.Parse(principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return uuid.Nil, uuid.Nil, false
	}
	circleID, err := uuid.Parse(r.PathValue("circleId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldCircleID: httpconst.ErrorMessageCircleIDInvalid})
		return uuid.Nil, uuid.Nil, false
	}
	return actorID, circleID, true
}

func discoveryMessageIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	actorID, circleID, ok := discoveryIDs(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	messageID, err := uuid.Parse(r.PathValue("messageId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldMessageID: httpconst.ErrorMessageChatMessageIDInvalid})
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return actorID, circleID, messageID, true
}

func writeDiscoveryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrPinLimit):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageChatPinLimit, http.StatusConflict)
	case errors.Is(err, ErrCircleArchived):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageCircleArchived, http.StatusConflict)
	case errors.Is(err, ErrInvalidIdempotencyKey):
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldIdempotencyKey: httpconst.ErrorMessageChatIdempotencyKeyInvalid})
	case errors.Is(err, ErrIdempotencyConflict):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageChatIdempotencyConflict, http.StatusConflict)
	case errors.Is(err, ErrCircleNotVisible), errors.Is(err, rbac.ErrForbidden):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
	case errors.Is(err, ErrMessageNotVisible):
		phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorMessageForbidden, http.StatusNotFound)
	default:
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
	}
}
