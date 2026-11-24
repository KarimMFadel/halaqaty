package notifications

import (
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/google/uuid"
)

// HistoryStore is the narrow persistence seam the HTTP handler depends on.
type HistoryStore interface {
	List(ctx context.Context, userID string, before *Cursor, limit int) (Page, error)
	MarkRead(ctx context.Context, userID string, notificationID uuid.UUID) (int64, error)
	MarkAllRead(ctx context.Context, userID string) (int64, error)
}

// Compile-time proof that the repository satisfies the handler seam.
var _ HistoryStore = (*Repository)(nil)

// Handler exposes the notification history and read-state REST endpoints.
type Handler struct {
	store    HistoryStore
	notifier *Notifier
}

// NewHandler constructs a notification HTTP handler.
func NewHandler(store HistoryStore, notifier *Notifier) *Handler {
	return &Handler{store: store, notifier: notifier}
}

// notificationItemResponse is the contract projection: read_at and every
// other internal field stay server-side.
type notificationItemResponse struct {
	ID        uuid.UUID `json:"id"`
	Type      EventType `json:"type"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	SourceRef SourceRef `json:"source_ref"`
	CreatedAt time.Time `json:"created_at"`
	IsRead    bool      `json:"is_read"`
}

type listNotificationsResponse struct {
	Items       []notificationItemResponse `json:"items"`
	NextCursor  *string                    `json:"next_cursor"`
	UnreadCount int64                      `json:"unread_count"`
}

type unreadCountResponse struct {
	UnreadCount int64 `json:"unread_count"`
}

// List handles GET /api/v1/notifications.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.store == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageNotificationHandlerNotConfigured, http.StatusInternalServerError)
		return
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	var before *Cursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		cursor, err := decodeHistoryCursor(raw)
		if err != nil {
			phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
				"cursor": httpconst.ErrorMessageNotificationCursorInvalid,
			})
			return
		}
		before = &cursor
	}
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		// The contract bounds limit to 1..50 and documents a 400, so the
		// upper bound is rejected here; the repository clamp stays as a
		// defense-in-depth backstop for non-HTTP callers.
		if err != nil || parsed < 1 || parsed > maxHistoryLimit {
			phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
				"limit": httpconst.ErrorMessageNotificationLimitInvalid,
			})
			return
		}
		limit = parsed
	}

	page, err := h.store.List(r.Context(), principal.UserID, before, limit)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}

	response := listNotificationsResponse{
		Items:       make([]notificationItemResponse, 0, len(page.Items)),
		UnreadCount: page.UnreadCount,
	}
	for _, item := range page.Items {
		response.Items = append(response.Items, notificationItemResponse{
			ID:        item.ID,
			Type:      item.Type,
			Title:     item.Title,
			Body:      item.Body,
			SourceRef: item.SourceRef,
			CreatedAt: item.CreatedAt,
			IsRead:    item.IsRead,
		})
	}
	if page.NextCursor != nil {
		encoded := encodeHistoryCursor(*page.NextCursor)
		response.NextCursor = &encoded
	}
	phttp.WriteJSON(w, http.StatusOK, response)
}

// MarkRead handles PUT /api/v1/notifications/{notificationId}/read.
func (h *Handler) MarkRead(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.store == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageNotificationHandlerNotConfigured, http.StatusInternalServerError)
		return
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	notificationID, err := uuid.Parse(r.PathValue("notificationId"))
	if err != nil {
		// The contract defines no 400 here: invalid, foreign, and unknown
		// identifiers stay indistinguishable (no disclosure).
		writeNotificationNotFound(w)
		return
	}
	unread, err := h.store.MarkRead(r.Context(), principal.UserID, notificationID)
	if errors.Is(err, ErrNotificationNotFound) {
		writeNotificationNotFound(w)
		return
	}
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}

	h.emitReadStateHint(r, principal.UserID, &notificationID, unread)
	phttp.WriteJSON(w, http.StatusOK, unreadCountResponse{UnreadCount: unread})
}

// MarkAllRead handles PUT /api/v1/notifications/read-all.
func (h *Handler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.store == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageNotificationHandlerNotConfigured, http.StatusInternalServerError)
		return
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	unread, err := h.store.MarkAllRead(r.Context(), principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}

	h.emitReadStateHint(r, principal.UserID, nil, unread)
	phttp.WriteJSON(w, http.StatusOK, unreadCountResponse{UnreadCount: unread})
}

// emitReadStateHint publishes the post-commit realtime hint. Emission is
// best-effort: a failure is logged but never fails the REST operation,
// because REST history stays authoritative for the unread count.
func (h *Handler) emitReadStateHint(r *http.Request, userID string, notificationID *uuid.UUID, unread int64) {
	if h.notifier == nil {
		return
	}
	if err := h.notifier.NotifyReadState(r.Context(), userID, notificationID, unread); err != nil {
		slog.WarnContext(r.Context(), "notification read-state hint emission failed", "error", err)
	}
}

func writeNotificationNotFound(w http.ResponseWriter) {
	phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorMessageNotificationNotFound, http.StatusNotFound)
}

// encodeHistoryCursor renders the stable descending history position as an
// opaque URL-safe token.
func encodeHistoryCursor(cursor Cursor) string {
	raw := cursor.CreatedAt.UTC().Format(time.RFC3339Nano) + "|" + cursor.ID.String()
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

// decodeHistoryCursor parses an opaque token back into its stable position.
func decodeHistoryCursor(raw string) (Cursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return Cursor{}, err
	}
	createdRaw, idRaw, found := strings.Cut(string(decoded), "|")
	if !found {
		return Cursor{}, errors.New("notification cursor must contain timestamp and id")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, createdRaw)
	if err != nil {
		return Cursor{}, err
	}
	id, err := uuid.Parse(idRaw)
	if err != nil {
		return Cursor{}, err
	}
	return Cursor{CreatedAt: createdAt.UTC(), ID: id}, nil
}

// PreferenceStore supplies effective notification settings for a recipient.
type PreferenceStore interface {
	GetPreferences(context.Context, string) (Preferences, error)
	UpdatePreferences(context.Context, string, []TypeSettingUpdate, *QuietHoursUpdate) error
}

type preferenceUpdateRequest struct {
	Types *[]struct {
		Type    *EventType `json:"type"`
		Enabled *bool      `json:"enabled"`
	} `json:"types"`
	QuietHours *struct {
		Enabled    *bool   `json:"enabled"`
		StartLocal *string `json:"start_local"`
		EndLocal   *string `json:"end_local"`
	} `json:"quiet_hours"`
}

// GetPreferences handles GET /api/v1/notifications/preferences.
func (h *Handler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	store, ok := h.preferenceStore(w)
	if !ok {
		return
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	result, err := store.GetPreferences(r.Context(), principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, result)
}

// UpdatePreferences handles PUT /api/v1/notifications/preferences.
func (h *Handler) UpdatePreferences(w http.ResponseWriter, r *http.Request) {
	store, ok := h.preferenceStore(w)
	if !ok {
		return
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}
	var req *preferenceUpdateRequest
	if r.Body == nil || r.Body == http.NoBody {
		writePreferenceInvalid(w)
		return
	}
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}
	if req == nil {
		writePreferenceInvalid(w)
		return
	}
	var updates []TypeSettingUpdate
	seen := map[EventType]bool{}
	if req.Types != nil {
		for _, item := range *req.Types {
			if item.Type == nil || item.Enabled == nil || !isConfigurable(*item.Type) || seen[*item.Type] {
				writePreferenceInvalid(w)
				return
			}
			seen[*item.Type] = true
			updates = append(updates, TypeSettingUpdate{Type: *item.Type, Enabled: *item.Enabled})
		}
	}
	var quiet *QuietHoursUpdate
	if req.QuietHours != nil {
		q := req.QuietHours
		if q.Enabled == nil || q.StartLocal == nil || q.EndLocal == nil {
			writePreferenceInvalid(w)
			return
		}
		_, startErr := parseLocalClock(*q.StartLocal)
		_, endErr := parseLocalClock(*q.EndLocal)
		if startErr != nil || endErr != nil || *q.StartLocal == *q.EndLocal {
			writePreferenceInvalid(w)
			return
		}
		quiet = &QuietHoursUpdate{Enabled: *q.Enabled, StartLocal: *q.StartLocal, EndLocal: *q.EndLocal}
	}
	if err := store.UpdatePreferences(r.Context(), principal.UserID, updates, quiet); err != nil {
		if errors.Is(err, ErrPreferenceNotConfigurable) || errors.Is(err, ErrInvalidQuietHours) {
			writePreferenceInvalid(w)
			return
		}
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	result, err := store.GetPreferences(r.Context(), principal.UserID)
	if err != nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) preferenceStore(w http.ResponseWriter) (PreferenceStore, bool) {
	if h != nil {
		if store, ok := h.store.(PreferenceStore); ok && store != nil {
			return store, true
		}
	}
	phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageNotificationHandlerNotConfigured, http.StatusInternalServerError)
	return nil, false
}
func writePreferenceInvalid(w http.ResponseWriter) {
	phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldBody: httpconst.ErrorMessageRequestBodyInvalid})
}
