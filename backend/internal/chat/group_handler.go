package chat

import (
	"context"
	"errors"
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
	// SendText durably accepts one idempotent group text message.
	SendText(ctx context.Context, senderID, circleID uuid.UUID, content, idempotencyKey string) (Message, error)
}

// GroupHandler exposes the F-004 US1 group-chat REST operations. Handlers
// decode, delegate to the service seam, and project responses; they contain
// no SQL and no business logic.
type GroupHandler struct {
	service GroupChatService
}

// NewGroupHandler constructs the group chat handler over the service seam.
// A nil service reports internal server errors, matching the unconfigured
// handler convention of the other route families.
func NewGroupHandler(service GroupChatService) *GroupHandler {
	return &GroupHandler{service: service}
}

// sendMessageRequest mirrors the canonical SendMessageRequest contract
// shape. US1 accepts text messages only; media sends (US3) and replies (US6)
// extend this surface with their stories.
type sendMessageRequest struct {
	MessageType string  `json:"message_type"`
	Content     string  `json:"content"`
	UploadID    *string `json:"upload_id"`
	MediaKey    *string `json:"media_key"`
	ReplyToID   *string `json:"reply_to_id"`
}

// validateTextSend enforces the US1 request shape: a text message carries
// only content. Field-level rejections map to the contract's 422.
func (r sendMessageRequest) validateTextSend() (field, message string, ok bool) {
	switch {
	case r.MessageType != string(MessageTypeText):
		return httpconst.FieldMessageType, httpconst.ErrorMessageChatMessageTypeTextOnly, false
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
// identifiers, content, server-authoritative timestamps, and state only —
// never object keys, URLs, tokens, or session identifiers (SR-006).
type messageResponse struct {
	ID             string `json:"id"`
	CircleID       string `json:"circle_id,omitempty"`
	SenderID       string `json:"sender_id"`
	MessageType    string `json:"message_type"`
	Content        string `json:"content,omitempty"`
	SentAt         string `json:"sent_at"`
	DeliveryStatus string `json:"delivery_status"`
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
	return response
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
		page.Data = append(page.Data, newMessageResponse(msg))
	}
	if page.HasMore && len(page.Data) > 0 {
		next := page.Data[len(page.Data)-1].ID
		page.NextBefore = &next
	}
	phttp.WriteJSON(w, http.StatusOK, page)
}

// SendCircleMessage implements the sendCircleMessage contract operation: one
// idempotent group text send. Fresh inserts and idempotent replays both
// return 201 with the durable message (FR-007).
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
