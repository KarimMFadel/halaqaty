package scheduling

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// PlannedSessionCommands is the one-off planning boundary used by the handler.
type PlannedSessionCommands interface {
	Create(context.Context, CreatePlannedSessionCommand) (PlannedSessionView, error)
	Change(context.Context, ChangePlannedSessionCommand) (PlannedSessionView, error)
}

// OccurrenceStartCommands starts one recurring occurrence using F-005 semantics.
type OccurrenceStartCommands interface {
	StartOccurrence(context.Context, StartOccurrenceCommand) (sessions.Session, sessions.MediaConnection, error)
}

// CalendarMonthCommands reads one authorized personal month.
type CalendarMonthCommands interface {
	Month(context.Context, string, string) (CalendarMonth, error)
}

// CalendarHandler exposes F-006 US2 calendar and planning operations.
type CalendarHandler struct {
	planned     PlannedSessionCommands
	occurrences OccurrenceStartCommands
	calendar    CalendarMonthCommands
}

// NewCalendarHandler creates a handler from the three existing domain services.
func NewCalendarHandler(planned PlannedSessionCommands, occurrences OccurrenceStartCommands, calendar CalendarMonthCommands) *CalendarHandler {
	return &CalendarHandler{planned: planned, occurrences: occurrences, calendar: calendar}
}

func (h *CalendarHandler) actor(w http.ResponseWriter, r *http.Request) (string, bool) {
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return "", false
	}
	return principal.UserID, true
}

// CreateOneOff serves POST /api/v1/circles/{circleId}/planned-sessions.
func (h *CalendarHandler) CreateOneOff(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.actor(w, r)
	if !ok {
		return
	}
	circleID, ok := pathUUID(w, r, "circleId", httpconst.FieldCircleID)
	if !ok {
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req planInputRequest
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}
	plan, ok := oneOffPlan(w, req)
	if !ok {
		return
	}
	view, err := h.planned.Create(r.Context(), CreatePlannedSessionCommand{ActorID: actorID, CircleID: circleID, IdempotencyKey: key, Plan: plan})
	if err != nil {
		writeCalendarError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusCreated, oneOffItem(view))
}

// ChangeOneOff serves PATCH /api/v1/sessions/{sessionId}/planned-details.
func (h *CalendarHandler) ChangeOneOff(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.actor(w, r)
	if !ok {
		return
	}
	sessionID, ok := pathUUID(w, r, "sessionId", "session_id")
	if !ok {
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req struct {
		ExpectedVersion     *int              `json:"expected_version"`
		Plan                *planInputRequest `json:"plan"`
		Cancelled           *bool             `json:"cancelled"`
		ConfirmOverlaps     *bool             `json:"confirm_overlaps"`
		ConfirmedWarningIDs []string          `json:"confirmed_warning_ids"`
	}
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}
	if req.ExpectedVersion == nil || *req.ExpectedVersion < 1 || req.ConfirmOverlaps == nil || (req.Plan == nil) == (req.Cancelled == nil) {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{"plan": "provide exactly one plan or cancelled with expected_version and confirm_overlaps"})
		return
	}
	cmd := ChangePlannedSessionCommand{ActorID: actorID, SessionID: sessionID, IdempotencyKey: key, ExpectedVersion: *req.ExpectedVersion, Cancelled: req.Cancelled}
	if req.Plan != nil {
		plan, ok := oneOffPlan(w, *req.Plan)
		if !ok {
			return
		}
		cmd.Plan = &plan
	}
	view, err := h.planned.Change(r.Context(), cmd)
	if err != nil {
		writeCalendarError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, oneOffItem(view))
}

// StartOccurrence serves POST /api/v1/circles/{circleId}/schedules/{scheduleId}/occurrences/{localDate}/start.
func (h *CalendarHandler) StartOccurrence(w http.ResponseWriter, r *http.Request) {
	w.Header().Set(httpconst.HeaderCacheControl, httpconst.CacheControlNoStore)
	w.Header().Set(httpconst.HeaderPragma, httpconst.PragmaNoCache)
	actorID, ok := h.actor(w, r)
	if !ok {
		return
	}
	circleID, ok := pathUUID(w, r, "circleId", httpconst.FieldCircleID)
	if !ok {
		return
	}
	scheduleID, ok := pathUUID(w, r, "scheduleId", httpconst.FieldScheduleID)
	if !ok {
		return
	}
	date, ok := dateField(w, "local_date", r.PathValue("localDate"))
	if !ok {
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	if h.occurrences == nil {
		phttp.WriteError(w, httpconst.ErrorCodeMediaUnavailable, httpconst.ErrorCodeMediaUnavailable, http.StatusServiceUnavailable)
		return
	}
	sess, conn, err := h.occurrences.StartOccurrence(r.Context(), StartOccurrenceCommand{ActorID: actorID, CircleID: circleID, ScheduleID: scheduleID, OriginalLocalDate: date, IdempotencyKey: key})
	if err != nil {
		writeCalendarError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, map[string]any{
		"session": map[string]any{
			"id": sess.ID, "circle_id": sess.CircleID, "created_by": sess.CreatedBy,
			"status": sess.Status, "media_mode": sess.MediaMode,
			"participant_count": sess.ParticipantCount, "is_locked": sess.IsLocked,
			"end_reason":   calendarOptionalString(string(sess.EndReason)),
			"actual_start": calendarOptionalTime(sess.ActualStart), "actual_end": calendarOptionalTime(sess.ActualEnd),
		},
		"media_connection": map[string]any{"endpoint": conn.Endpoint, "credential": conn.Credential, "expires_at": conn.ExpiresAt.UTC().Format(time.RFC3339)},
		"is_moderator":     true,
	})
}

// GetPersonalMonth serves GET /api/v1/calendar/me.
func (h *CalendarHandler) GetPersonalMonth(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.actor(w, r)
	if !ok {
		return
	}
	month := r.URL.Query().Get("month")
	parsed, err := time.Parse("2006-01", month)
	if err != nil || len(month) != 7 || parsed.Format("2006-01") != month {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{"month": "month must be YYYY-MM"})
		return
	}
	result, err := h.calendar.Month(r.Context(), actorID, month)
	if err != nil {
		writeCalendarError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, result)
}

func oneOffPlan(w http.ResponseWriter, req planInputRequest) (PlannedSessionPlan, bool) {
	record, ok := decodePlan(w, req)
	if !ok {
		return PlannedSessionPlan{}, false
	}
	if record.Mode != "one_off" || req.EndLocalDate != nil || req.WeekCadence != nil || req.Weekdays != nil || req.IntervalCount != nil || req.IntervalUnit != nil || req.SelectedDates != nil {
		writeScheduleUnprocessable(w, httpconst.ErrorMessageSchedulePlanInvalid)
		return PlannedSessionPlan{}, false
	}
	return PlannedSessionPlan{LocalDate: record.AnchorLocalDate, StartLocalTime: record.StartLocalTime, EndLocalTime: record.EndLocalTime, DurationMinutes: record.DurationMinutes, Timezone: record.Timezone, Title: record.Title}, true
}

func oneOffItem(view PlannedSessionView) CalendarItem {
	id := view.ID
	state := calendarState(view.Status, nil)
	if view.Cancelled {
		state = "cancelled"
	}
	return CalendarItem{OccurrenceKey: id, SessionID: &id, CircleID: view.CircleID, CircleName: view.CircleName, Title: view.Title, StartsAt: view.StartsAt, EndsAt: view.EndsAt, PlanningTimezone: view.PlanningTimezone, State: state}
}

func calendarOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

func calendarOptionalString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func writeCalendarError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrPlannedSessionNotFound):
		phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorCodeNotFound, http.StatusNotFound)
	case errors.Is(err, sessions.ErrMediaUnavailable):
		phttp.WriteError(w, httpconst.ErrorCodeMediaUnavailable, httpconst.ErrorCodeMediaUnavailable, http.StatusServiceUnavailable)
	case errors.Is(err, sessions.ErrSessionNotFound):
		phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorCodeNotFound, http.StatusNotFound)
	case errors.Is(err, sessions.ErrSessionNotStartable), errors.Is(err, sessions.ErrSessionAlreadyActive), errors.Is(err, sessions.ErrSessionAlreadyEnded):
		writeScheduleConflict(w, httpconst.ErrorMessageScheduleConflict)
	case errors.Is(err, sessions.ErrNotCircleMember), errors.Is(err, sessions.ErrModeratorRoleRequired):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
	case errors.Is(err, ErrInvalidCalendarMonth):
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{"month": "month must be YYYY-MM"})
	default:
		writeScheduleError(w, err)
	}
}
