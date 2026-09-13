package chat

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// MaxSendsPerMinute is the F-004 group-send budget per user and circle
// (FR-006: 30 messages per minute).
const MaxSendsPerMinute = 30

// GroupChatService is the transport-facing seam the group handler depends
// on; *GroupService satisfies it in production, and the REST contract tests
// substitute deterministic fakes so the HTTP surface does not require
// PostgreSQL.
type GroupChatService interface {
	// History returns one circle history page for a current member.
	History(ctx context.Context, viewerID, circleID uuid.UUID, before *uuid.UUID, limit int) ([]Message, error)
	// Search returns one retained search page for a current member.
	Search(ctx context.Context, viewerID, circleID uuid.UUID, query string, before *uuid.UUID, limit int) ([]Message, error)
	// SendText durably accepts one idempotent group text message.
	SendText(ctx context.Context, senderID, circleID uuid.UUID, content, idempotencyKey string) (Message, error)
}

// GroupMediaService is the transport-facing seam for US3 media sends; the
// *UploadService satisfies it in production, and the REST contract tests
// substitute deterministic fakes. Attach-once, uploader/context binding, and
// non-enumerating failures are service-side.
type GroupMediaService interface {
	// SendGroupMedia durably accepts one idempotent group media message
	// bound to a staged upload.
	SendGroupMedia(ctx context.Context, in SendGroupMediaInput) (Message, error)
	// RenewMediaURL returns currently authorized media and safe display metadata.
	RenewMediaURL(ctx context.Context, viewerID, messageID uuid.UUID) (MediaAccess, error)
}

// GroupHandler exposes the F-004 US1 group-chat REST operations. Handlers
// decode, delegate to the service seam, and project responses; they contain
// no SQL and no business logic.
type GroupHandler struct {
	service GroupChatService
	media   GroupMediaService
}

// NewGroupHandler constructs the group chat handler over the service seam.
// A nil service reports internal server errors, matching the unconfigured
// handler convention of the other route families.
func NewGroupHandler(service GroupChatService) *GroupHandler {
	return &GroupHandler{service: service}
}

// SetMediaService wires the US3 media-send seam, mirroring the optional
// dependency convention of sessions.Handler.SetWebhookVerifier. Without it
// the send route serves text messages only and media sends report internal
// server errors.
func (h *GroupHandler) SetMediaService(media GroupMediaService) {
	h.media = media
}

// sendMessageRequest mirrors the canonical SendMessageRequest contract
// shape. US1 serves text and US3 serves media sends; replies (US6) extend
// this surface with their story.
type sendMessageRequest struct {
	MessageType string  `json:"message_type"`
	Content     string  `json:"content"`
	UploadID    *string `json:"upload_id"`
	MediaKey    *string `json:"media_key"`
	ReplyToID   *string `json:"reply_to_id"`
}

// validateTextSend enforces the US1 text request shape: a text message
// carries only content. The dispatch switch in SendCircleMessage has already
// established the type. Field-level rejections map to the contract's 422.
func (r sendMessageRequest) validateTextSend() (field, message string, ok bool) {
	switch {
	case r.UploadID != nil:
		return httpconst.FieldUploadID, httpconst.ErrorMessageChatUploadIDNotAllowed, false
	case r.MediaKey != nil && strings.TrimSpace(*r.MediaKey) != "":
		return httpconst.FieldMediaKey, httpconst.ErrorMessageChatMediaKeyUnsupported, false
	case r.ReplyToID != nil:
		return httpconst.FieldReplyToID, httpconst.ErrorMessageChatReplyUnsupported, false
	}
	return "", "", true
}

// messageResponse is the response-safe canonical REST Message projection:
// identifiers, content, timestamps, and currently authorized media links.
// Object keys, tokens, and session identifiers are never exposed.
type messageResponse struct {
	ID                   string `json:"id"`
	CircleID             string `json:"circle_id,omitempty"`
	DMRecipientID        string `json:"dm_peer_id,omitempty"`
	SenderID             string `json:"sender_id"`
	MessageType          string `json:"message_type"`
	Content              string `json:"content,omitempty"`
	SentAt               string `json:"sent_at"`
	DeliveryStatus       string `json:"delivery_status"`
	MediaURL             string `json:"media_url,omitempty"`
	MediaURLExpiresAt    string `json:"media_url_expires_at,omitempty"`
	FileName             string `json:"file_name,omitempty"`
	VoiceDurationSeconds int    `json:"voice_duration_seconds,omitempty"`
}

// newMessageResponse projects one durable message. Delivery state is
// server-authoritative: a persisted group message is at least delivered
// (read receipts arrive with US5).
func newMessageResponse(msg Message) messageResponse {
	response := messageResponse{
		ID:             msg.ID.String(),
		SenderID:       msg.SenderID.String(),
		MessageType:    string(msg.Type),
		Content:        msg.Content,
		SentAt:         msg.SentAt.UTC().Format(time.RFC3339Nano),
		DeliveryStatus: string(DeliveryStatusDelivered),
	}
	if msg.CircleID != nil {
		response.CircleID = msg.CircleID.String()
	}
	if msg.DMRecipientID != nil {
		response.DMRecipientID = msg.DMRecipientID.String()
	}
	return response
}

// projectMessage adds freshly authorized media only to REST media responses.
func (h *GroupHandler) projectMessage(ctx context.Context, viewerID uuid.UUID, msg Message) (messageResponse, error) {
	response := newMessageResponse(msg)
	if msg.Type == MessageTypeText {
		return response, nil
	}
	if h.media == nil {
		return messageResponse{}, fmt.Errorf("chat media projection is not configured")
	}
	access, err := h.media.RenewMediaURL(ctx, viewerID, msg.ID)
	if err != nil {
		return messageResponse{}, err
	}
	response.MediaURL = access.URL.String()
	response.MediaURLExpiresAt = access.ExpiresAt.UTC().Format(time.RFC3339Nano)
	response.FileName = access.FileName
	response.VoiceDurationSeconds = access.VoiceDurationSeconds
	return response, nil
}

// paginatedMessagesResponse mirrors the canonical PaginatedMessages schema.
type paginatedMessagesResponse struct {
	Data       []messageResponse `json:"data"`
	HasMore    bool              `json:"has_more"`
	NextBefore *string           `json:"next_before"`
}

// ListCircleMessages implements the listCircleMessages contract operation:
// one newest-first group history page for a current member. A full page
// reports has_more with the oldest page id as the next cursor; a caller
// paginating past the end receives one final empty page.
func (h *GroupHandler) ListCircleMessages(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	principal, ok := middleware.CurrentPrincipal(r.Context())
	if !ok {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	viewerID, err := uuid.Parse(principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	circleID, err := uuid.Parse(r.PathValue("circleId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldCircleID: httpconst.ErrorMessageCircleIDInvalid,
		})
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

	messages, err := h.service.History(r.Context(), viewerID, circleID, before, limit)
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	page := paginatedMessagesResponse{
		Data:       make([]messageResponse, 0, len(messages)),
		HasMore:    len(messages) == limit,
		NextBefore: nil,
	}
	for _, msg := range messages {
		response, err := h.projectMessage(r.Context(), viewerID, msg)
		if err != nil {
			writeMediaRenewalError(w, err)
			return
		}
		page.Data = append(page.Data, response)
	}
	if page.HasMore && len(page.Data) > 0 {
		next := page.Data[len(page.Data)-1].ID
		page.NextBefore = &next
	}
	phttp.WriteJSON(w, http.StatusOK, page)
}

// SearchCircleMessages implements the retained group-history search contract.
func (h *GroupHandler) SearchCircleMessages(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	principal, ok := middleware.CurrentPrincipal(r.Context())
	if !ok {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	viewerID, err := uuid.Parse(principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	circleID, err := uuid.Parse(r.PathValue("circleId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldCircleID: httpconst.ErrorMessageCircleIDInvalid})
		return
	}
	query, err := ValidateSearchQuery(r.URL.Query().Get(httpconst.FieldQuery))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldQuery: httpconst.ErrorMessageChatSearchInvalid})
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
	messages, err := h.service.Search(r.Context(), viewerID, circleID, query, before, limit)
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	page := paginatedMessagesResponse{Data: make([]messageResponse, 0, len(messages)), HasMore: len(messages) == limit}
	for _, message := range messages {
		response, err := h.projectMessage(r.Context(), viewerID, message)
		if err != nil {
			writeMediaRenewalError(w, err)
			return
		}
		page.Data = append(page.Data, response)
	}
	if page.HasMore && len(page.Data) > 0 {
		next := page.Data[len(page.Data)-1].ID
		page.NextBefore = &next
	}
	phttp.WriteJSON(w, http.StatusOK, page)
}

// SendCircleMessage implements the sendCircleMessage contract operation: one
// idempotent group text send (US1) or media send (US3). Fresh inserts and
// idempotent replays both return 201 with the durable message (FR-007).
func (h *GroupHandler) SendCircleMessage(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	principal, ok := middleware.CurrentPrincipal(r.Context())
	if !ok {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	senderID, err := uuid.Parse(principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	circleID, err := uuid.Parse(r.PathValue("circleId"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldCircleID: httpconst.ErrorMessageCircleIDInvalid,
		})
		return
	}
	var request sendMessageRequest
	if !phttp.DecodeJSONBody(w, r, &request) {
		return
	}

	switch MessageType(request.MessageType) {
	case MessageTypeText:
		if field, message, ok := request.validateTextSend(); !ok {
			writeFieldUnprocessable(w, field, message)
			return
		}
		sent, err := h.service.SendText(r.Context(), senderID, circleID, request.Content, r.Header.Get(httpconst.HeaderIdempotencyKey))
		if err != nil {
			writeSendError(w, err)
			return
		}
		phttp.WriteJSON(w, http.StatusCreated, newMessageResponse(sent))
	case MessageTypeVoice, MessageTypeImage, MessageTypeFile:
		h.sendMedia(w, r, request, senderID, circleID, MessageType(request.MessageType))
	default:
		writeFieldUnprocessable(w, httpconst.FieldMessageType, httpconst.ErrorMessageChatMessageTypeInvalid)
	}
}

// sendMedia validates one US3 media send (message_type voice|image|file plus
// upload_id), delegates to the media seam, and writes the documented
// response. Malformed upload ids are 400; a missing upload id or disallowed
// payload field is 422, mirroring the text-send shapes.
func (h *GroupHandler) sendMedia(w http.ResponseWriter, r *http.Request, request sendMessageRequest, senderID, circleID uuid.UUID, msgType MessageType) {
	if h.media == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	if request.UploadID == nil {
		writeFieldUnprocessable(w, httpconst.FieldUploadID, httpconst.ErrorMessageChatUploadIDRequired)
		return
	}
	uploadID, err := uuid.Parse(*request.UploadID)
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldUploadID: httpconst.ErrorMessageChatUploadIDInvalid,
		})
		return
	}
	if strings.TrimSpace(request.Content) != "" {
		writeFieldUnprocessable(w, httpconst.FieldContent, httpconst.ErrorMessageChatMediaContentNotAllowed)
		return
	}
	if request.MediaKey != nil && strings.TrimSpace(*request.MediaKey) != "" {
		writeFieldUnprocessable(w, httpconst.FieldMediaKey, httpconst.ErrorMessageChatMediaKeyUnsupported)
		return
	}
	if request.ReplyToID != nil {
		writeFieldUnprocessable(w, httpconst.FieldReplyToID, httpconst.ErrorMessageChatReplyUnsupported)
		return
	}

	sent, err := h.media.SendGroupMedia(r.Context(), SendGroupMediaInput{
		SenderID:       senderID,
		CircleID:       circleID,
		UploadID:       uploadID,
		MessageType:    msgType,
		IdempotencyKey: r.Header.Get(httpconst.HeaderIdempotencyKey),
	})
	if err != nil {
		writeMediaSendError(w, err)
		return
	}
	response, err := h.projectMessage(r.Context(), senderID, sent)
	if err != nil {
		writeMediaRenewalError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusCreated, response)
}

// parseHistoryLimit reads the optional limit query parameter, bounded to the
// contract's 1–100 range with the documented default. The error response is
// already written when ok is false.
func parseHistoryLimit(w http.ResponseWriter, r *http.Request) (int, bool) {
	raw := r.URL.Query().Get(httpconst.FieldLimit)
	if raw == "" {
		return DefaultHistoryPageSize, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > MaxHistoryPageSize {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldLimit: httpconst.ErrorMessageChatLimitInvalid,
		})
		return 0, false
	}
	return limit, true
}

// parseBeforeCursor reads the optional before UUID cursor. The error
// response is already written when ok is false.
func parseBeforeCursor(w http.ResponseWriter, r *http.Request) (*uuid.UUID, bool) {
	raw := r.URL.Query().Get(httpconst.FieldBefore)
	if raw == "" {
		return nil, true
	}
	before, err := uuid.Parse(raw)
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldBefore: httpconst.ErrorMessageCursorInvalid,
		})
		return nil, false
	}
	return &before, true
}

// writeHistoryError maps history service errors onto the contract's declared
// responses: a non-visible circle (non-member or unknown, indistinguishable)
// is 403, an unresolvable cursor is 400, and anything else is an internal
// error that must not leak details.
func writeHistoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrCircleNotVisible):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
	case errors.Is(err, ErrInvalidCursor):
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldBefore: httpconst.ErrorMessageCursorInvalid,
		})
	default:
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
	}
}

// writeSendError maps send service errors onto the contract's declared
// responses: an invalid Idempotency-Key is 400, invalid text is 422, a
// non-visible circle (non-member or unknown, indistinguishable) is 403, an
// archived circle is 409, and anything else is an internal error that must
// not leak details.
func writeSendError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrIdempotencyConflict):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageChatIdempotencyConflict, http.StatusConflict)
	case errors.Is(err, ErrInvalidIdempotencyKey):
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldIdempotencyKey: httpconst.ErrorMessageChatIdempotencyKeyInvalid,
		})
	case errors.Is(err, ErrInvalidText):
		writeFieldUnprocessable(w, httpconst.FieldContent, httpconst.ErrorMessageChatTextInvalid)
	case errors.Is(err, ErrCircleNotVisible):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
	case errors.Is(err, ErrCircleArchived):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageCircleArchived, http.StatusConflict)
	default:
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
	}
}

// writeMediaSendError maps media-send service errors onto the contract's
// declared responses, mirroring writeSendError plus the US3 attach
// sentinels: an invalid Idempotency-Key is 400, a payload rejected by the
// service despite handler validation is 422, a non-visible circle is the
// same non-enumerating 403, an archived circle is 409, an upload that is
// not attachable (single-use, other uploader, or wrong context) is 409, and
// anything else is an internal error that must not leak details.
func writeMediaSendError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrIdempotencyConflict):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageChatIdempotencyConflict, http.StatusConflict)
	case errors.Is(err, ErrInvalidIdempotencyKey):
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldIdempotencyKey: httpconst.ErrorMessageChatIdempotencyKeyInvalid,
		})
	case errors.Is(err, ErrInvalidPayload):
		writeFieldUnprocessable(w, httpconst.FieldUploadID, httpconst.ErrorMessageChatUploadIDRequired)
	case errors.Is(err, ErrCircleNotVisible):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
	case errors.Is(err, ErrCircleArchived):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageCircleArchived, http.StatusConflict)
	case errors.Is(err, ErrUploadNotAttachable), errors.Is(err, ErrUploadNotStaged):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorMessageChatUploadNotAttachable, http.StatusConflict)
	default:
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
	}
}

// writeFieldUnprocessable writes one 422 validation envelope with a
// field-level detail, mirroring the F-003 queue validation responses.
func writeFieldUnprocessable(w http.ResponseWriter, field, message string) {
	phttp.WriteJSON(w, http.StatusUnprocessableEntity, phttp.ErrorEnvelope{
		Error: phttp.ErrorBody{
			Code:    httpconst.ErrorCodeValidationFailed,
			Message: httpconst.ErrorMessageValidationFailed,
			Fields:  map[string]string{field: message},
		},
	})
}

// chatSendWindowCounter is one fixed-window counter for a (user, circle) key.
type chatSendWindowCounter struct {
	windowStart time.Time
	count       int
}

// chatSendLimiterPurgeThreshold bounds the counter map: once exceeded, stale
// windows are dropped inline on the request path.
// ponytail: inline purge at this threshold; replace with a periodic sweeper
// only if (user, circle) key cardinality outgrows a request-path scan.
const chatSendLimiterPurgeThreshold = 4096

// ChatSendLimiter enforces the F-004 group-send budget (MaxSendsPerMinute
// per user and circle, FR-006) using the same one-minute fixed-window design
// as middleware.RateLimitMiddleware, keyed by user and circle instead of
// user alone.
type ChatSendLimiter struct {
	perMinute int
	nowFn     func() time.Time

	mu       sync.Mutex
	counters map[string]chatSendWindowCounter
}

// NewChatSendLimiter creates a one-minute fixed-window limiter allowing
// perMinute sends per user and circle.
func NewChatSendLimiter(perMinute int) *ChatSendLimiter {
	return &ChatSendLimiter{
		perMinute: perMinute,
		nowFn:     time.Now,
		counters:  map[string]chatSendWindowCounter{},
	}
}

// Limit rejects group sends over the per-user-and-circle budget with the
// standard 429 envelope. It must wrap the send handler after auth middleware
// so the principal is in context; the 429 carries no Retry-After header
// because the chat contract declares none.
func (l *ChatSendLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if l != nil && l.perMinute > 0 {
			if principal, ok := middleware.CurrentPrincipal(r.Context()); ok && principal.UserID != "" {
				if l.hitLimit(principal.UserID + "\x00" + canonicalLimiterCircleID(r.PathValue("circleId"))) {
					phttp.WriteError(w, httpconst.ErrorCodeRateLimitExceeded, httpconst.ErrorMessageRateLimitExceeded, http.StatusTooManyRequests)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// canonicalLimiterCircleID canonicalizes the circle path segment so textual
// UUID variants ({braced}, urn:uuid:, uppercase) share one rate-limit budget.
// Unparseable values fall back to the raw segment; such requests fail
// membership authorization anyway.
func canonicalLimiterCircleID(raw string) string {
	if id, err := uuid.Parse(raw); err == nil {
		return id.String()
	}
	return raw
}

// hitLimit counts one send for the key in the current one-minute window and
// reports whether the budget is exceeded.
func (l *ChatSendLimiter) hitLimit(key string) bool {
	now := l.nowFn().UTC()
	windowStart := now.Truncate(time.Minute)

	l.mu.Lock()
	defer l.mu.Unlock()

	if len(l.counters) >= chatSendLimiterPurgeThreshold {
		for key, counter := range l.counters {
			if counter.windowStart.Before(windowStart) {
				delete(l.counters, key)
			}
		}
	}

	counter := l.counters[key]
	if counter.windowStart != windowStart {
		counter = chatSendWindowCounter{windowStart: windowStart}
	}
	counter.count++
	l.counters[key] = counter
	return counter.count > l.perMinute
}
