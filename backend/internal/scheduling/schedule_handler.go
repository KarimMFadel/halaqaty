package scheduling

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// maxIdempotencyKeyLength mirrors the contract Idempotency-Key parameter bound.
const maxIdempotencyKeyLength = 128

// localClockPattern mirrors the contract '^([01][0-9]|2[0-3]):[0-5][0-9]$'.
var localClockPattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// ScheduleCommands is the handler-facing surface of ScheduleService. The
// contract tests stub this boundary; the production service satisfies it.
type ScheduleCommands interface {
	List(ctx context.Context, circleID, actorID string) ([]ScheduleView, error)
	Create(ctx context.Context, cmd CreateScheduleCommand) (Schedule, error)
	Change(ctx context.Context, cmd ChangeScheduleCommand) (Schedule, error)
	ChangeOccurrence(ctx context.Context, cmd ChangeOccurrenceCommand) (OccurrenceException, error)
	OccurrenceView(ctx context.Context, circleID, scheduleID, actorID string, original time.Time) (OccurrenceView, error)
}

// ScheduleHandler exposes the F-006 US1 schedule REST endpoints.
type ScheduleHandler struct {
	service ScheduleCommands
}

// NewScheduleHandler constructs the schedule HTTP handler.
func NewScheduleHandler(service ScheduleCommands) *ScheduleHandler {
	return &ScheduleHandler{service: service}
}

// OccurrenceKey is the stable public identity of one recurring occurrence:
// the schedule UUID plus the original local date (ADR-026); it stays stable
// even when the occurrence is moved.
func OccurrenceKey(scheduleID string, originalLocalDate time.Time) string {
	return scheduleID + ":" + originalLocalDate.Format(time.DateOnly)
}

// OverlapWarning is the privacy-safe warning projection the conflict contract
// reserves for US4 overlap detection. US1 responses carry an empty list.
type OverlapWarning struct {
	WarningID           string `json:"warning_id"`
	FirstOccurrenceKey  string `json:"first_occurrence_key"`
	SecondOccurrenceKey string `json:"second_occurrence_key"`
	FirstCircleName     string `json:"first_circle_name"`
	SecondCircleName    string `json:"second_circle_name"`
	OverlapStartsAt     string `json:"overlap_starts_at"`
	OverlapEndsAt       string `json:"overlap_ends_at"`
}

// ListSchedules serves GET /api/v1/circles/{circleId}/schedules.
func (h *ScheduleHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	circleID, ok := pathUUID(w, r, "circleId", httpconst.FieldCircleID)
	if !ok {
		return
	}
	views, err := h.service.List(r.Context(), circleID, actorID)
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	data := make([]map[string]any, 0, len(views))
	for _, view := range views {
		item := scheduleResponse(view.Schedule, view.Plan)
		versions := view.OccurrenceVersions
		if versions == nil {
			versions = map[string]int{}
		}
		item["occurrence_versions"] = versions
		data = append(data, item)
	}
	phttp.WriteJSON(w, http.StatusOK, map[string]any{"data": data})
}

// CreateSchedule serves POST /api/v1/circles/{circleId}/schedules.
func (h *ScheduleHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.authenticate(w, r)
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
	plan, ok := decodePlan(w, req)
	if !ok {
		return
	}
	created, err := h.service.Create(r.Context(), CreateScheduleCommand{
		ActorID: actorID, CircleID: circleID, IdempotencyKey: key, Plan: plan,
		ConfirmOverlaps: req.ConfirmOverlaps != nil && *req.ConfirmOverlaps, ConfirmedWarningIDs: req.ConfirmedWarningIDs,
	})
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusCreated, scheduleResponse(created, plan))
}

// ChangeSchedule serves PATCH /api/v1/circles/{circleId}/schedules/{scheduleId}.
func (h *ScheduleHandler) ChangeSchedule(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.authenticate(w, r)
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
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req changeScheduleRequest
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}
	fields := map[string]string{}
	if req.ExpectedVersion == nil || *req.ExpectedVersion < 1 {
		fields[httpconst.FieldExpectedVersion] = "expected_version must be a positive integer"
	}
	if req.ConfirmOverlaps == nil {
		fields[httpconst.FieldConfirmOverlaps] = "confirm_overlaps is required"
	}
	var effective time.Time
	if req.EffectiveLocalDate == nil {
		fields[httpconst.FieldEffectiveLocalDate] = "effective_local_date is required"
	} else if parsed, err := time.Parse(time.DateOnly, *req.EffectiveLocalDate); err != nil {
		fields[httpconst.FieldEffectiveLocalDate] = "effective_local_date must be a YYYY-MM-DD date"
	} else {
		effective = parsed
	}
	if req.Plan == nil {
		fields[httpconst.FieldPlan] = "plan is required"
	}
	if len(fields) > 0 {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, fields)
		return
	}
	plan, ok := decodePlan(w, *req.Plan)
	if !ok {
		return
	}
	changed, err := h.service.Change(r.Context(), ChangeScheduleCommand{
		ActorID: actorID, CircleID: circleID, ScheduleID: scheduleID, IdempotencyKey: key,
		ExpectedVersion: *req.ExpectedVersion, EffectiveLocalDate: effective, Plan: plan,
		Stop: req.Stop, ConfirmOverlaps: *req.ConfirmOverlaps, ConfirmedWarningIDs: req.ConfirmedWarningIDs,
	})
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, scheduleResponse(changed, plan))
}

// ChangeScheduleOccurrence serves PATCH
// /api/v1/circles/{circleId}/schedules/{scheduleId}/occurrences/{localDate}.
func (h *ScheduleHandler) ChangeScheduleOccurrence(w http.ResponseWriter, r *http.Request) {
	actorID, ok := h.authenticate(w, r)
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
	original, err := time.Parse(time.DateOnly, r.PathValue("localDate"))
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldLocalDate: "local_date must be a YYYY-MM-DD date",
		})
		return
	}
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var req changeOccurrenceRequest
	if !phttp.DecodeJSONBody(w, r, &req) {
		return
	}
	fields := map[string]string{}
	if req.ExpectedSeriesVersion == nil || *req.ExpectedSeriesVersion < 1 {
		fields["expected_series_version"] = "expected_series_version must be a positive integer"
	}
	if req.ExpectedOccurrenceVersion == nil || *req.ExpectedOccurrenceVersion < 0 {
		fields["expected_occurrence_version"] = "expected_occurrence_version must be zero or a positive integer"
	}
	if req.ConfirmOverlaps == nil {
		fields[httpconst.FieldConfirmOverlaps] = "confirm_overlaps is required"
	}
	if len(fields) > 0 {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, fields)
		return
	}
	cmd := ChangeOccurrenceCommand{
		ActorID: actorID, CircleID: circleID, ScheduleID: scheduleID, IdempotencyKey: key,
		OriginalLocalDate:          original,
		ExpectedSeriesVersion:      *req.ExpectedSeriesVersion,
		ExpectedOccurrenceVersion:  *req.ExpectedOccurrenceVersion,
		ConfirmOverlaps:            *req.ConfirmOverlaps,
		ConfirmedWarningIDs:        req.ConfirmedWarningIDs,
		Title:                      req.Title,
		Cancelled:                  req.Cancelled,
		ReplacementDurationMinutes: req.DurationMinutes,
	}
	if req.ReplacementLocalDate != nil {
		parsed, err := time.Parse(time.DateOnly, *req.ReplacementLocalDate)
		if err != nil {
			phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
				"replacement_local_date": "replacement_local_date must be a YYYY-MM-DD date",
			})
			return
		}
		cmd.ReplacementLocalDate = &parsed
	}
	startClock, ok := optionalClockField(w, "replacement_local_time", req.ReplacementLocalTime)
	if !ok {
		return
	}
	endClock, ok := optionalClockField(w, "replacement_end_local_time", req.ReplacementEndLocalTime)
	if !ok {
		return
	}
	cmd.ReplacementStartLocalTime = startClock
	cmd.ReplacementEndLocalTime = endClock
	if _, err := h.service.ChangeOccurrence(r.Context(), cmd); err != nil {
		writeScheduleError(w, err)
		return
	}
	view, err := h.service.OccurrenceView(r.Context(), circleID, scheduleID, actorID, original)
	if err != nil {
		writeScheduleError(w, err)
		return
	}
	phttp.WriteJSON(w, http.StatusOK, occurrenceCalendarItem(view))
}

// ---- request shapes ----------------------------------------------------------

// planInputRequest mirrors the contract PlanInput schema; pointer fields let
// the handler distinguish an absent required field from a zero value.
type planInputRequest struct {
	Mode                *string   `json:"mode"`
	Title               *string   `json:"title"`
	AnchorLocalDate     *string   `json:"anchor_local_date"`
	LocalStartTime      *string   `json:"local_start_time"`
	LocalEndTime        *string   `json:"local_end_time"`
	DurationMinutes     *int      `json:"duration_minutes"`
	Timezone            *string   `json:"timezone"`
	EndLocalDate        *string   `json:"end_local_date"`
	WeekCadence         *int      `json:"week_cadence"`
	Weekdays            *[]int    `json:"weekdays"`
	IntervalCount       *int      `json:"interval_count"`
	IntervalUnit        *string   `json:"interval_unit"`
	SelectedDates       *[]string `json:"selected_dates"`
	ConfirmOverlaps     *bool     `json:"confirm_overlaps"`
	ConfirmedWarningIDs []string  `json:"confirmed_warning_ids"`
}

// changeScheduleRequest mirrors the series patch contract body.
type changeScheduleRequest struct {
	ExpectedVersion     *int              `json:"expected_version"`
	EffectiveLocalDate  *string           `json:"effective_local_date"`
	Plan                *planInputRequest `json:"plan"`
	Stop                bool              `json:"stop"`
	ConfirmOverlaps     *bool             `json:"confirm_overlaps"`
	ConfirmedWarningIDs []string          `json:"confirmed_warning_ids"`
}

// changeOccurrenceRequest mirrors the occurrence patch contract body.
type changeOccurrenceRequest struct {
	ExpectedSeriesVersion     *int     `json:"expected_series_version"`
	ExpectedOccurrenceVersion *int     `json:"expected_occurrence_version"`
	ReplacementLocalDate      *string  `json:"replacement_local_date"`
	ReplacementLocalTime      *string  `json:"replacement_local_time"`
	ReplacementEndLocalTime   *string  `json:"replacement_end_local_time"`
	DurationMinutes           *int     `json:"duration_minutes"`
	Title                     *string  `json:"title"`
	Cancelled                 *bool    `json:"cancelled"`
	ConfirmOverlaps           *bool    `json:"confirm_overlaps"`
	ConfirmedWarningIDs       []string `json:"confirmed_warning_ids"`
}

// ---- decoding helpers --------------------------------------------------------

func (h *ScheduleHandler) authenticate(w http.ResponseWriter, r *http.Request) (string, bool) {
	if h == nil || h.service == nil {
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		return "", false
	}
	principal, ok := auth.CurrentPrincipal(r.Context())
	if !ok || principal.UserID == "" {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
		return "", false
	}
	return principal.UserID, true
}

// pathUUID validates one path parameter as a UUID; a malformed identifier is
// a 400 validation error, never a repository 500.
func pathUUID(w http.ResponseWriter, r *http.Request, name, field string) (string, bool) {
	value := r.PathValue(name)
	if _, err := uuid.Parse(value); err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			field: field + " must be a valid UUID",
		})
		return "", false
	}
	return value, true
}

// idempotencyKey enforces the contract's required Idempotency-Key header.
func idempotencyKey(w http.ResponseWriter, r *http.Request) (string, bool) {
	key := r.Header.Get(httpconst.HeaderIdempotencyKey)
	if key == "" || len(key) > maxIdempotencyKeyLength {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			httpconst.FieldIdempotencyKey: httpconst.ErrorMessageScheduleIdempotencyKeyNeeded,
		})
		return "", false
	}
	return key, true
}

// decodePlan validates PlanInput syntax (presence, date and clock formats)
// and converts it to a revision record; semantic validation stays in the
// service so invalid scheduling semantics surface as 422, not 400.
func decodePlan(w http.ResponseWriter, req planInputRequest) (RevisionRecord, bool) {
	fields := map[string]string{}
	if req.Mode == nil {
		fields["mode"] = "mode is required"
	}
	if req.AnchorLocalDate == nil {
		fields["anchor_local_date"] = "anchor_local_date is required"
	}
	if req.LocalStartTime == nil {
		fields["local_start_time"] = "local_start_time is required"
	}
	if req.LocalEndTime == nil {
		fields["local_end_time"] = "local_end_time is required"
	}
	if req.DurationMinutes == nil {
		fields["duration_minutes"] = "duration_minutes is required"
	}
	if req.Timezone == nil {
		fields["timezone"] = "timezone is required"
	}
	if len(fields) > 0 {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, fields)
		return RevisionRecord{}, false
	}

	record := RevisionRecord{
		Revision:        Revision{Mode: RecurrenceMode(*req.Mode)},
		DurationMinutes: *req.DurationMinutes,
		Timezone:        *req.Timezone,
	}
	if req.Title != nil {
		record.Title = *req.Title
	}

	anchor, ok := dateField(w, "anchor_local_date", *req.AnchorLocalDate)
	if !ok {
		return RevisionRecord{}, false
	}
	record.AnchorLocalDate = anchor
	if req.EndLocalDate != nil {
		end, ok := dateField(w, "end_local_date", *req.EndLocalDate)
		if !ok {
			return RevisionRecord{}, false
		}
		record.EndLocalDate = &end
	}
	startClock, ok := clockField(w, "local_start_time", *req.LocalStartTime)
	if !ok {
		return RevisionRecord{}, false
	}
	endClock, ok := clockField(w, "local_end_time", *req.LocalEndTime)
	if !ok {
		return RevisionRecord{}, false
	}
	record.StartLocalTime = startClock
	record.EndLocalTime = endClock

	if req.WeekCadence != nil {
		record.WeekCadence = *req.WeekCadence
	}
	if req.Weekdays != nil {
		record.Weekdays = *req.Weekdays
	}
	if req.IntervalCount != nil {
		record.IntervalCount = *req.IntervalCount
	}
	if req.IntervalUnit != nil {
		record.IntervalUnit = IntervalUnit(*req.IntervalUnit)
	}
	if req.SelectedDates != nil {
		for _, value := range *req.SelectedDates {
			date, ok := dateField(w, "selected_dates", value)
			if !ok {
				return RevisionRecord{}, false
			}
			record.SelectedDates = append(record.SelectedDates, date)
		}
	}
	return record, true
}

func dateField(w http.ResponseWriter, field, value string) (time.Time, bool) {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			field: field + " must be a YYYY-MM-DD date",
		})
		return time.Time{}, false
	}
	return parsed, true
}

func clockField(w http.ResponseWriter, field, value string) (LocalClock, bool) {
	if !localClockPattern.MatchString(value) {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			field: field + " must be a HH:MM local clock time",
		})
		return LocalClock{}, false
	}
	clock, err := parseLocalClock(value)
	if err != nil {
		phttp.WriteValidationError(w, httpconst.ErrorMessageValidationFailed, map[string]string{
			field: field + " must be a HH:MM local clock time",
		})
		return LocalClock{}, false
	}
	return clock, true
}

// optionalClockField parses an optional replacement clock; nil stays nil.
func optionalClockField(w http.ResponseWriter, field string, value *string) (*LocalClock, bool) {
	if value == nil {
		return nil, true
	}
	clock, ok := clockField(w, field, *value)
	if !ok {
		return nil, false
	}
	return &clock, true
}

// ---- response projections ----------------------------------------------------

// scheduleResponse projects the contract Schedule schema.
func scheduleResponse(sch Schedule, plan RevisionRecord) map[string]any {
	var stopped any
	if sch.StoppedFromLocalDate != nil {
		stopped = sch.StoppedFromLocalDate.Format(time.DateOnly)
	}
	return map[string]any{
		"id":                      sch.ID,
		"circle_id":               sch.CircleID,
		"version":                 sch.CurrentVersion,
		"stopped_from_local_date": stopped,
		"plan":                    planProjection(plan),
	}
}

// planProjection projects the contract PlanInput schema from a retained
// revision; only the current mode's selectors are emitted.
func planProjection(plan RevisionRecord) map[string]any {
	title := plan.Title
	if title == "" {
		title = defaultScheduleTitle
	}
	var endDate any
	if plan.EndLocalDate != nil {
		endDate = plan.EndLocalDate.Format(time.DateOnly)
	}
	out := map[string]any{
		"mode":              string(plan.Mode),
		"title":             title,
		"anchor_local_date": plan.AnchorLocalDate.Format(time.DateOnly),
		"local_start_time":  formatLocalClock(plan.StartLocalTime),
		"local_end_time":    formatLocalClock(plan.EndLocalTime),
		"duration_minutes":  plan.DurationMinutes,
		"timezone":          plan.Timezone,
		"end_local_date":    endDate,
	}
	switch plan.Mode {
	case ModeWeekdayPattern:
		out["week_cadence"] = plan.WeekCadence
		out["weekdays"] = plan.Weekdays
	case ModeInterval:
		out["interval_count"] = plan.IntervalCount
		out["interval_unit"] = string(plan.IntervalUnit)
	case ModeSelectedDates:
		dates := make([]string, 0, len(plan.SelectedDates))
		for _, date := range plan.SelectedDates {
			dates = append(dates, date.Format(time.DateOnly))
		}
		out["selected_dates"] = dates
	}
	return out
}

// occurrenceCalendarItem projects the contract CalendarItem schema for one
// changed occurrence. Only unstarted occurrences reach this point, so the
// lifecycle state is either scheduled or cancelled.
func occurrenceCalendarItem(view OccurrenceView) map[string]any {
	state := "scheduled"
	if view.Cancelled {
		state = "cancelled"
	}
	var sessionID any
	if view.SessionID != nil {
		sessionID = *view.SessionID
	}
	return map[string]any{
		"occurrence_key":    OccurrenceKey(view.ScheduleID, view.OriginalLocalDate),
		"session_id":        sessionID,
		"circle_id":         view.CircleID,
		"circle_name":       view.CircleName,
		"title":             view.Title,
		"starts_at":         view.StartsAt.UTC().Format(time.RFC3339),
		"ends_at":           view.EndsAt.UTC().Format(time.RFC3339),
		"planning_timezone": view.Timezone,
		"state":             state,
	}
}

// ---- error mapping -----------------------------------------------------------

// scheduleConflictEnvelope is the standard error envelope extended with the
// contracted overlap-warning result, including an empty list when no warning applies.
type scheduleConflictEnvelope struct {
	Error struct {
		Code     string        `json:"code"`
		Message  string        `json:"message"`
		Warnings WarningResult `json:"warnings"`
	} `json:"error"`
}

func writeScheduleConflict(w http.ResponseWriter, message string) {
	writeScheduleConflictWarnings(w, message, nil)
}

func writeScheduleConflictWarnings(w http.ResponseWriter, message string, warnings []OverlapWarning) {
	var envelope scheduleConflictEnvelope
	envelope.Error.Code = httpconst.ErrorCodeConflict
	envelope.Error.Message = message
	envelope.Error.Warnings = WarningResult{Warnings: warnings}
	if envelope.Error.Warnings.Warnings == nil {
		envelope.Error.Warnings.Warnings = []OverlapWarning{}
	}
	phttp.WriteJSON(w, http.StatusConflict, envelope)
}

func writeScheduleUnprocessable(w http.ResponseWriter, message string) {
	phttp.WriteJSON(w, http.StatusUnprocessableEntity, phttp.ErrorEnvelope{
		Error: phttp.ErrorBody{Code: httpconst.ErrorCodeValidationFailed, Message: message},
	})
}

// writeScheduleError maps scheduling domain errors to the contract status
// codes: 404 hides unavailable resources, 403 denies roles and archived
// writes, 409 surfaces conflicts and started history, 422 reports invalid
// scheduling semantics, and anything unexpected stays a detail-free 500.
func writeScheduleError(w http.ResponseWriter, err error) {
	var overlapErr *OverlapConfirmationError
	if errors.As(err, &overlapErr) {
		writeScheduleConflictWarnings(w, "Review the updated scheduling conflicts before saving.", overlapErr.Warnings)
		return
	}
	switch {
	case errors.Is(err, ErrCircleNotFoundOrDenied):
		phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorMessageCircleNotFound, http.StatusNotFound)
	case errors.Is(err, ErrScheduleNotFound):
		phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorMessageScheduleNotFound, http.StatusNotFound)
	case errors.Is(err, ErrOccurrenceNotFound):
		phttp.WriteError(w, httpconst.ErrorCodeNotFound, httpconst.ErrorMessageScheduleOccurrenceNotFound, http.StatusNotFound)
	case errors.Is(err, ErrInsufficientCircleRole):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageForbidden, http.StatusForbidden)
	case errors.Is(err, ErrCircleArchived):
		phttp.WriteError(w, httpconst.ErrorCodeForbidden, httpconst.ErrorMessageCircleArchived, http.StatusForbidden)
	case errors.Is(err, ErrScheduleConflict):
		writeScheduleConflict(w, httpconst.ErrorMessageScheduleConflict)
	case errors.Is(err, ErrReplayConflict):
		writeScheduleConflict(w, httpconst.ErrorMessageScheduleIdempotencyConflict)
	case errors.Is(err, ErrOccurrenceStarted):
		writeScheduleConflict(w, httpconst.ErrorMessageScheduleOccurrenceStarted)
	case errors.Is(err, ErrPastPlannedTime):
		writeScheduleUnprocessable(w, httpconst.ErrorMessageSchedulePastPlannedTime)
	case errors.Is(err, ErrInvalidScheduleCommand), errors.Is(err, ErrInvalidRevision),
		errors.Is(err, ErrUnknownRecurrenceMode), errors.Is(err, ErrInvalidWeekdayPattern),
		errors.Is(err, ErrInvalidInterval), errors.Is(err, ErrInvalidSelectedDates),
		errors.Is(err, ErrInvalidTimezone), errors.Is(err, ErrInvalidLocalClock),
		errors.Is(err, ErrInvalidDuration), errors.Is(err, ErrEndClockMismatch),
		errors.Is(err, ErrInvalidException):
		writeScheduleUnprocessable(w, httpconst.ErrorMessageSchedulePlanInvalid)
	default:
		phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
	}
}
