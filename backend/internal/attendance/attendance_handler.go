package attendance

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/google/uuid"
)

const maxAttendanceIdempotencyKeyLength = 128

// AttendanceCommands is the handler-facing review and correction surface.
type AttendanceCommands interface {
	List(context.Context, string, string) ([]Record, error)
	Correct(context.Context, CorrectionCommand) (Record, error)
}

// Handler exposes role-filtered attendance review and teacher corrections.
type Handler struct {
	commands AttendanceCommands
}

// NewHandler constructs an attendance HTTP handler.
func NewHandler(commands AttendanceCommands) *Handler { return &Handler{commands: commands} }

// GetAttendance serves GET /api/v1/sessions/{sessionId}/attendance.
func (h *Handler) GetAttendance(w http.ResponseWriter, r *http.Request) {
	actorID, ok := attendanceActor(w, r)
	if !ok {
		return
	}
	sessionID, ok := attendancePathUUID(w, r, "sessionId", "session_id")
	if !ok {
		return
	}
	records, err := h.commands.List(r.Context(), sessionID, actorID)
	if err != nil {
		writeAttendanceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, records)
}

// CorrectAttendance serves PATCH /api/v1/sessions/{sessionId}/attendance/{userId}.
func (h *Handler) CorrectAttendance(w http.ResponseWriter, r *http.Request) {
	actorID, ok := attendanceActor(w, r)
	if !ok {
		return
	}
	sessionID, ok := attendancePathUUID(w, r, "sessionId", "session_id")
	if !ok {
		return
	}
	userID, ok := attendancePathUUID(w, r, "userId", "user_id")
	if !ok {
		return
	}
	key := r.Header.Get(httpconst.HeaderIdempotencyKey)
	if key == "" || len(key) > maxAttendanceIdempotencyKeyLength || strings.TrimSpace(key) != key {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldIdempotencyKey: httpconst.ErrorMessageScheduleIdempotencyKeyNeeded,
		})
		return
	}
	var request struct {
		Status Status `json:"status"`
		Reason string `json:"reason"`
	}
	if !phttp.DecodeJSONBody(w, r, &request) {
		return
	}
	record, err := h.commands.Correct(r.Context(), CorrectionCommand{
		ActorID: actorID, SessionID: sessionID, UserID: userID,
		IdempotencyKey: key, Status: request.Status, Reason: request.Reason,
	})
	if err != nil {
		writeAttendanceError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, record)
}

func attendanceActor(w http.ResponseWriter, r *http.Request) (string, bool) {
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return "", false
	}
	return principal.UserID, true
}

func attendancePathUUID(w http.ResponseWriter, r *http.Request, pathName, field string) (string, bool) {
	value := r.PathValue(pathName)
	parsed, err := uuid.Parse(value)
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{field: "must be a valid UUID"})
		return "", false
	}
	return parsed.String(), true
}

func writeAttendanceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrAttendanceForbidden), errors.Is(err, ErrAttendanceArchived):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
	case errors.Is(err, ErrAttendanceNotFound):
		phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorCodeNotFound, http.StatusNotFound)
	case errors.Is(err, ErrAttendanceNotComplete), errors.Is(err, ErrCorrectionReplay):
		phttp.WriteError(w, httpconst.ErrorCodeConflict, httpconst.ErrorCodeConflict, http.StatusConflict)
	case errors.Is(err, ErrInvalidCorrection):
		phttp.WriteJSON(w, http.StatusUnprocessableEntity, phttp.ErrorEnvelope{Error: phttp.ErrorBody{
			Code: httpconst.ErrorCodeValidationFailed, Message: httpconst.ErrorMessageValidationFailed,
		}})
	default:
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
	}
}
