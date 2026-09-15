package chat

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// DirectChatService is the HTTP seam for eligible pair conversations.
type DirectChatService interface {
	History(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, int) ([]Message, error)
	SendText(context.Context, uuid.UUID, uuid.UUID, string, string) (Message, error)
	ReplyText(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, string) (Message, error)
	DeleteOwnMessage(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error
}

// DirectMediaService is the optional media seam for direct messages.
type DirectMediaService interface {
	SendDirectMedia(context.Context, SendDirectMediaInput) (Message, error)
	RenewMediaURL(context.Context, uuid.UUID, uuid.UUID) (MediaAccess, error)
}

// DirectHandler exposes list/send/delete operations for an eligible pair.
type DirectHandler struct {
	service DirectChatService
	media   DirectMediaService
}

// NewDirectHandler constructs a direct-message handler.
func NewDirectHandler(service DirectChatService) *DirectHandler {
	return &DirectHandler{service: service}
}

// SetMediaService wires direct media operations.
func (h *DirectHandler) SetMediaService(media DirectMediaService) { h.media = media }

// ListMessages returns the newest-first history for one currently eligible peer.
func (h *DirectHandler) ListMessages(w http.ResponseWriter, r *http.Request) {
	viewerID, peerID, ok := h.ids(w, r)
	if !ok || h.service == nil {
		return
	}
	limit, ok := parseHistoryLimit(w, r)
	if !ok {
		return
	}
	before, ok := parseBeforeCursor(w, r)
	if !ok {
		return
	}
	messages, err := h.service.History(r.Context(), viewerID, peerID, before, limit)
	if err != nil {
		writeDirectError(w, err)
		return
	}
	page := paginatedMessagesResponse{Data: make([]messageResponse, 0, len(messages)), HasMore: len(messages) == limit}
	for _, message := range messages {
		response := newMessageResponseForViewer(message, viewerID)
		page.Data = append(page.Data, response)
	}
	if page.HasMore && len(page.Data) > 0 {
		next := page.Data[len(page.Data)-1].ID
		page.NextBefore = &next
	}
	phttp.WriteJSON(w, http.StatusOK, page)
}

// SendMessage accepts a text or staged direct media message.
func (h *DirectHandler) SendMessage(w http.ResponseWriter, r *http.Request) {
	viewerID, peerID, ok := h.ids(w, r)
	if !ok || h.service == nil {
		return
	}
	var request sendMessageRequest
	if !phttp.DecodeJSONBody(w, r, &request) {
		return
	}
	var sent Message
	var err error
	switch MessageType(request.MessageType) {
	case MessageTypeText:
		if field, message, valid := request.validateTextSend(); !valid {
			writeFieldUnprocessable(w, field, message)
			return
		}
		if request.ReplyToID != nil {
			replyToID, parseErr := uuid.Parse(*request.ReplyToID)
			if parseErr != nil {
				phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldReplyToID: httpconst.ErrorMessageChatMessageIDInvalid})
				return
			}
			sent, err = h.service.ReplyText(r.Context(), viewerID, peerID, replyToID, request.Content, r.Header.Get(httpconst.HeaderIdempotencyKey))
		} else {
			sent, err = h.service.SendText(r.Context(), viewerID, peerID, request.Content, r.Header.Get(httpconst.HeaderIdempotencyKey))
		}
	case MessageTypeVoice, MessageTypeImage, MessageTypeFile:
		if request.ReplyToID != nil {
			writeFieldUnprocessable(w, httpconst.FieldReplyToID, httpconst.ErrorMessageChatReplyUnsupported)
			return
		}
		if h.media == nil || request.UploadID == nil {
			writeFieldUnprocessable(w, httpconst.FieldUploadID, httpconst.ErrorMessageChatUploadIDRequired)
			return
		}
		uploadID, parseErr := uuid.Parse(*request.UploadID)
		if parseErr != nil {
			writeFieldUnprocessable(w, httpconst.FieldUploadID, httpconst.ErrorMessageChatUploadIDInvalid)
			return
		}
		sent, err = h.media.SendDirectMedia(r.Context(), SendDirectMediaInput{SenderID: viewerID, PeerID: peerID, UploadID: uploadID, MessageType: MessageType(request.MessageType), IdempotencyKey: r.Header.Get(httpconst.HeaderIdempotencyKey)})
	default:
		writeFieldUnprocessable(w, httpconst.FieldMessageType, httpconst.ErrorMessageChatMessageTypeInvalid)
		return
	}
	if err != nil {
		writeDirectError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusCreated, newMessageResponse(sent))
}

// DeleteMessage deletes only the caller's own recent direct message.
func (h *DirectHandler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	viewerID, peerID, ok := h.ids(w, r)
	if !ok || h.service == nil {
		return
	}
	messageID, err := uuid.Parse(r.PathValue("messageId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldMessageID: httpconst.ErrorMessageChatMessageIDInvalid})
		return
	}
	if err := ValidateIdempotencyKey(r.Header.Get(httpconst.HeaderIdempotencyKey)); err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldIdempotencyKey: httpconst.ErrorMessageChatIdempotencyKeyInvalid})
		return
	}
	if err := h.service.DeleteOwnMessage(r.Context(), viewerID, peerID, messageID); err != nil {
		writeDirectError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *DirectHandler) ids(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	principal, ok := middleware.CurrentPrincipal(r.Context())
	if !ok {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return uuid.Nil, uuid.Nil, false
	}
	viewerID, err := uuid.Parse(principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return uuid.Nil, uuid.Nil, false
	}
	peerID, err := uuid.Parse(r.PathValue("userId"))
	if err != nil || peerID == viewerID {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldUserID: httpconst.ErrorMessageChatDMPeerIDInvalid})
		return uuid.Nil, uuid.Nil, false
	}
	return viewerID, peerID, true
}

func writeDirectError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrDMNotEligible), errors.Is(err, ErrMessageNotVisible):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
	case errors.Is(err, ErrDirectDeleteConflict):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageForbidden, http.StatusConflict)
	case errors.Is(err, ErrInvalidIdempotencyKey):
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldIdempotencyKey: httpconst.ErrorMessageChatIdempotencyKeyInvalid})
	case errors.Is(err, ErrIdempotencyConflict):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageChatIdempotencyConflict, http.StatusConflict)
	case errors.Is(err, ErrInvalidText):
		writeFieldUnprocessable(w, httpconst.FieldContent, httpconst.ErrorMessageChatTextInvalid)
	case errors.Is(err, ErrUploadNotAttachable), errors.Is(err, ErrUploadNotStaged):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageChatUploadNotAttachable, http.StatusConflict)
	default:
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
	}
}
