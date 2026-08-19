package sessions

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// MediaWebhookEventType is the provider-neutral type of a verified media event.
type MediaWebhookEventType string

const (
	EventParticipantJoined MediaWebhookEventType = "participant_joined"
	EventParticipantLeft   MediaWebhookEventType = "participant_left"
	EventRoomFinished      MediaWebhookEventType = "room_finished"
)

// MediaWebhookEvent is the neutral event delivered by a signed media verifier.
type MediaWebhookEvent struct {
	ID        string
	Type      MediaWebhookEventType
	RoomRef   MediaRoomRef
	Identity  string
	Timestamp time.Time
}

// MediaWebhookVerifier verifies and translates provider webhook requests.
type MediaWebhookVerifier interface {
	Verify(*http.Request) (MediaWebhookEvent, error)
}

// ErrWebhookSignature indicates an invalid or missing webhook signature.
var ErrWebhookSignature = errors.New("invalid webhook signature")

// Handler exposes the F-005 REST and media webhook endpoints.
type Handler struct {
	service *Service
	webhook MediaWebhookVerifier
}

// NewHandler constructs a session HTTP handler.
func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// SetWebhookVerifier injects the provider adapter used by the public webhook
// route. The verifier remains outside the sessions domain boundary.
func (h *Handler) SetWebhookVerifier(verifier MediaWebhookVerifier) {
	if h != nil {
		h.webhook = verifier
	}
}

// ServeHTTP dispatches the F-005 create, start, and join operations.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/") == "webhooks/livekit" && r.Method == http.MethodPost {
		h.HandleMediaWebhook(w, r)
		return
	}
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return
	}

	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/"), "/")
	// Session-scoped operations validate the path identifier once here so a
	// malformed UUID is a 400 validation error, never a repository 500.
	if len(parts) >= 2 && parts[0] == "sessions" && !validUUID(parts[1]) {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{"session_id": "session_id must be a valid UUID"})
		return
	}
	switch {
	case len(parts) == 3 && parts[0] == "circles" && parts[2] == "sessions" && r.Method == http.MethodPost:
		h.create(w, r, principal.UserID, parts[1])
	case len(parts) == 3 && parts[0] == "sessions" && r.Method == http.MethodPost:
		switch parts[2] {
		case "start":
			h.start(w, r, principal.UserID, parts[1])
		case "join":
			h.join(w, r, principal.UserID, parts[1])
		case "end":
			h.end(w, r, principal.UserID, parts[1])
		case "lock":
			h.lock(w, r, principal.UserID, parts[1])
		default:
			writeSessionError(w, ErrSessionNotFound)
		}
	case len(parts) == 3 && parts[0] == "sessions" && parts[2] == "participants" && r.Method == http.MethodGet:
		h.participants(w, r, principal.UserID, parts[1])
	case len(parts) == 4 && parts[0] == "sessions" && parts[2] == "participants" && r.Method == http.MethodPost:
		switch parts[3] {
		case "mute-all":
			h.muteAll(w, r, principal.UserID, parts[1])
		default:
			writeSessionError(w, ErrSessionNotFound)
		}
	case len(parts) == 5 && parts[0] == "sessions" && parts[2] == "participants" && r.Method == http.MethodPost:
		h.participantAction(w, r, principal.UserID, parts[1], parts[3], parts[4])
	default:
		writeSessionError(w, ErrSessionNotFound)
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request, actorID, circleID string) {
	if !validUUID(circleID) {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldCircleID: "circle_id must be a valid UUID"})
		return
	}
	if !phttp.DecodeJSONBody(w, r, &struct{}{}) {
		return
	}
	sess, err := h.service.CreateAdHocSession(r.Context(), actorID, circleID)
	if err != nil {
		writeSessionError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusCreated, publicSession(sess))
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request, actorID, sessionID string) {
	h.lifecycle(w, r, actorID, sessionID, true)
}

func (h *Handler) join(w http.ResponseWriter, r *http.Request, actorID, sessionID string) {
	h.lifecycle(w, r, actorID, sessionID, false)
}

func (h *Handler) end(w http.ResponseWriter, r *http.Request, actorID, sessionID string) {
	var req struct {
		EndReason EndReason `json:"end_reason"`
	}
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}
	if req.EndReason == "" {
		req.EndReason = EndReasonManual
	}
	sess, err := h.service.EndSession(r.Context(), actorID, sessionID, req.EndReason)
	if err != nil {
		writeSessionError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, publicSession(sess))
}

func (h *Handler) lock(w http.ResponseWriter, r *http.Request, actorID, sessionID string) {
	var req struct {
		Locked bool `json:"locked"`
	}
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}
	sess, err := h.service.SetLock(r.Context(), actorID, sessionID, req.Locked)
	if err != nil {
		writeSessionError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, publicSession(sess))
}

func (h *Handler) participants(w http.ResponseWriter, r *http.Request, actorID, sessionID string) {
	participants, err := h.service.ListParticipants(r.Context(), actorID, sessionID)
	if err != nil {
		writeSessionError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(participants))
	for _, p := range participants {
		items = append(items, publicParticipant(p))
	}
	phttp.WriteJSON(w, http.StatusOK, map[string]any{"data": items})
}

func (h *Handler) muteAll(w http.ResponseWriter, r *http.Request, actorID, sessionID string) {
	if !phttp.DecodeJSONBody(w, r, &struct{}{}) {
		return
	}
	if err := h.service.MuteAll(r.Context(), actorID, sessionID); err != nil {
		writeSessionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) participantAction(w http.ResponseWriter, r *http.Request, actorID, sessionID, targetID, action string) {
	if !validUUID(targetID) {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{httpconst.FieldUserID: "user_id must be a valid UUID"})
		return
	}
	if !phttp.DecodeJSONBody(w, r, &struct{}{}) {
		return
	}
	var err error
	switch action {
	case "mute":
		err = h.service.MuteParticipant(r.Context(), actorID, sessionID, targetID)
	case "unmute":
		err = h.service.UnmuteParticipant(r.Context(), actorID, sessionID, targetID)
	case "remove":
		_, err = h.service.RemoveParticipant(r.Context(), actorID, sessionID, targetID)
	default:
		writeSessionError(w, ErrSessionNotFound)
		return
	}
	if err != nil {
		writeSessionError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) lifecycle(w http.ResponseWriter, r *http.Request, actorID, sessionID string, start bool) {
	if !validUUID(sessionID) {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{"session_id": "session_id must be a valid UUID"})
		return
	}
	var (
		sess Session
		conn MediaConnection
		err  error
	)
	if start {
		sess, conn, err = h.service.StartSession(r.Context(), actorID, sessionID)
	} else {
		sess, conn, err = h.service.JoinSession(r.Context(), actorID, sessionID)
	}
	if err != nil {
		writeSessionError(w, err)
		return
	}
	isModerator, err := h.service.IsModerator(r.Context(), actorID, sessionID)
	if err != nil {
		writeSessionError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	phttp.WriteJSON(w, http.StatusOK, map[string]any{
		"session":          publicSession(sess),
		"media_connection": publicConnection(conn),
		"is_moderator":     isModerator,
	})
}

// HandleMediaWebhook verifies a provider callback and acknowledges it without
// exposing provider data. Event application is owned by the later moderation
// and presence tasks.
func (h *Handler) HandleMediaWebhook(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.webhook == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return
	}
	if _, err := h.webhook.Verify(r); err != nil {
		if errors.Is(err, ErrWebhookSignature) {
			phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
			return
		}
		phttp.WriteError(w, httpconst.ErrorCodeValidationFailed, httpconst.ErrorMessageValidationFailed, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validUUID(value string) bool { _, err := uuid.Parse(value); return err == nil }

func publicSession(s Session) map[string]any {
	return map[string]any{
		"id": s.ID, "circle_id": s.CircleID, "created_by": s.CreatedBy,
		"status": s.Status, "media_mode": s.MediaMode,
		"participant_count": s.ParticipantCount, "is_locked": s.IsLocked,
		"end_reason":   optionalString(string(s.EndReason)),
		"actual_start": optionalTime(s.ActualStart), "actual_end": optionalTime(s.ActualEnd),
	}
}

func publicConnection(c MediaConnection) map[string]any {
	return map[string]any{
		"endpoint": c.Endpoint, "credential": c.Credential,
		"expires_at": c.ExpiresAt.UTC().Format(time.RFC3339),
	}
}

// publicParticipant projects one presence row to the canonical
// SessionParticipant schema: user_id, display_name, role,
// is_currently_present, hand_raised_at. Internal columns (join/leave
// timestamps, reconnect counts, removal state) never leave the service.
func publicParticipant(p ParticipantPresence) map[string]any {
	role := p.Role
	if role == "" {
		role = roleStudent
	}
	return map[string]any{
		"user_id": p.UserID, "display_name": p.DisplayName, "role": role,
		"is_currently_present": p.IsCurrentlyPresent, "hand_raised_at": optionalTime(p.HandRaisedAt),
	}
}

func optionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

func optionalString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func writeSessionError(w http.ResponseWriter, err error) {
	code, status := sessionHTTPError(err)
	phttp.WriteError(w, code, code, status)
}

func sessionHTTPError(err error) (string, int) {
	switch {
	case errors.Is(err, ErrSessionNotFound):
		return httpconst.ErrorCodeNotFound, http.StatusNotFound
	case errors.Is(err, ErrSessionNotStartable), errors.Is(err, ErrSessionAlreadyActive), errors.Is(err, ErrSessionAlreadyEnded), errors.Is(err, ErrSessionFull), errors.Is(err, ErrSessionLocked):
		return httpconst.ErrorCodeConflict, http.StatusConflict
	case errors.Is(err, ErrNotCircleMember), errors.Is(err, ErrModeratorRoleRequired), errors.Is(err, ErrParticipantRemoved):
		return httpconst.ErrorCodeForbidden, http.StatusForbidden
	default:
		return httpconst.ErrorCodeInternalServerError, http.StatusInternalServerError
	}
}
