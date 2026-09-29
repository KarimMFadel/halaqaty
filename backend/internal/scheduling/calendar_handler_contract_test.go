//go:build contract

package scheduling

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// F-006 US2 contract tests mirror the approved feature-local and canonical
// OpenAPI operations: create/change one-off, start a recurring occurrence, and
// read the personal month calendar. The handler seam is intentionally small;
// it leaves authorization, validation, status mapping, and response projection
// observable at the HTTP boundary without a database.

const (
	calendarContractActorID    = "11111111-1111-1111-1111-111111111111"
	calendarContractCircleID   = "33333333-3333-3333-3333-333333333333"
	calendarContractScheduleID = "55555555-5555-5555-5555-555555555555"
	calendarContractSessionID  = "44444444-4444-4444-4444-444444444444"
)

type plannedSessionCommandsStub struct {
	createFn func(context.Context, CreatePlannedSessionCommand) (PlannedSessionView, error)
	changeFn func(context.Context, ChangePlannedSessionCommand) (PlannedSessionView, error)
}

func (s *plannedSessionCommandsStub) Create(ctx context.Context, cmd CreatePlannedSessionCommand) (PlannedSessionView, error) {
	if s.createFn != nil {
		return s.createFn(ctx, cmd)
	}
	return PlannedSessionView{}, nil
}
func (s *plannedSessionCommandsStub) Change(ctx context.Context, cmd ChangePlannedSessionCommand) (PlannedSessionView, error) {
	if s.changeFn != nil {
		return s.changeFn(ctx, cmd)
	}
	return PlannedSessionView{}, nil
}

type occurrenceStartCommandsStub struct {
	startFn func(context.Context, StartOccurrenceCommand) (sessions.Session, sessions.MediaConnection, error)
}

func (s *occurrenceStartCommandsStub) StartOccurrence(ctx context.Context, cmd StartOccurrenceCommand) (sessions.Session, sessions.MediaConnection, error) {
	if s.startFn != nil {
		return s.startFn(ctx, cmd)
	}
	return sessions.Session{}, sessions.MediaConnection{}, nil
}

type calendarMonthCommandsStub struct {
	monthFn func(context.Context, string, string) (CalendarMonth, error)
}

func (s *calendarMonthCommandsStub) Month(ctx context.Context, actorID, month string) (CalendarMonth, error) {
	if s.monthFn != nil {
		return s.monthFn(ctx, actorID, month)
	}
	return CalendarMonth{}, nil
}

func newCalendarContractHandler(planned *plannedSessionCommandsStub, starts *occurrenceStartCommandsStub, calendar *calendarMonthCommandsStub) *CalendarHandler {
	return NewCalendarHandler(planned, starts, calendar)
}

func calendarContractRequest(t *testing.T, method, target, body string, values map[string]string, authenticated bool) *http.Request {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	}
	if method != http.MethodGet {
		req.Header.Set(httpconst.HeaderIdempotencyKey, "calendar-contract-key")
	}
	for name, value := range values {
		req.SetPathValue(name, value)
	}
	if authenticated {
		req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: calendarContractActorID}))
	}
	return req
}

func calendarContractError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status=%d want=%d body=%s", rec.Code, status, rec.Body.String())
	}
	if got := rec.Header().Get(httpconst.HeaderContentType); got != httpconst.ContentTypeApplicationJSON {
		t.Fatalf("content type=%q", got)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if body.Error.Code != code || strings.TrimSpace(body.Error.Message) == "" {
		t.Fatalf("error envelope=%+v", body.Error)
	}
}

func TestCalendarHandlerCreateOneOffContract(t *testing.T) {
	path := map[string]string{"circleId": calendarContractCircleID}
	body := `{"mode":"one_off","title":"Special review","anchor_local_date":"2030-01-06","local_start_time":"22:30","local_end_time":"23:00","duration_minutes":30,"timezone":"Asia/Riyadh"}`
	var got CreatePlannedSessionCommand
	planned := &plannedSessionCommandsStub{createFn: func(_ context.Context, cmd CreatePlannedSessionCommand) (PlannedSessionView, error) {
		got = cmd
		return PlannedSessionView{ID: calendarContractSessionID, CircleID: calendarContractCircleID, CircleName: "Contract Circle", Title: "Special review", StartsAt: time.Date(2030, 1, 6, 19, 30, 0, 0, time.UTC), EndsAt: time.Date(2030, 1, 6, 20, 0, 0, 0, time.UTC), PlanningTimezone: "Asia/Riyadh", Status: "scheduled"}, nil
	}}
	handler := newCalendarContractHandler(planned, &occurrenceStartCommandsStub{}, &calendarMonthCommandsStub{})
	rec := httptest.NewRecorder()
	handler.CreateOneOff(rec, calendarContractRequest(t, http.MethodPost, "/api/v1/circles/"+calendarContractCircleID+"/planned-sessions", body, path, true))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got.ActorID != calendarContractActorID || got.CircleID != calendarContractCircleID || got.IdempotencyKey != "calendar-contract-key" || got.Plan.Title != "Special review" || !got.Plan.LocalDate.Equal(time.Date(2030, 1, 6, 0, 0, 0, 0, time.UTC)) || got.Plan.StartLocalTime != (LocalClock{Hour: 22, Minute: 30}) || got.Plan.EndLocalTime != (LocalClock{Hour: 23}) || got.Plan.DurationMinutes != 30 || got.Plan.Timezone != "Asia/Riyadh" {
		t.Fatalf("command scope/key: %+v", got)
	}
	var item CalendarItem
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode CalendarItem: %v", err)
	}
	if item.OccurrenceKey != calendarContractSessionID || item.SessionID == nil || *item.SessionID != calendarContractSessionID || item.CircleID != calendarContractCircleID || item.CircleName != "Contract Circle" || item.Title != "Special review" || !item.StartsAt.Equal(time.Date(2030, 1, 6, 19, 30, 0, 0, time.UTC)) || !item.EndsAt.Equal(time.Date(2030, 1, 6, 20, 0, 0, 0, time.UTC)) || item.PlanningTimezone != "Asia/Riyadh" || item.State != "scheduled" {
		t.Fatalf("CalendarItem projection: %+v", item)
	}

	t.Run("missing authentication uses standard envelope", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.CreateOneOff(rec, calendarContractRequest(t, http.MethodPost, "/api/v1/circles/"+calendarContractCircleID+"/planned-sessions", body, path, false))
		calendarContractError(t, rec, http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized)
	})
	t.Run("student cannot create plans", func(t *testing.T) {
		d := &plannedSessionCommandsStub{createFn: func(context.Context, CreatePlannedSessionCommand) (PlannedSessionView, error) {
			return PlannedSessionView{}, ErrInsufficientCircleRole
		}}
		rec := httptest.NewRecorder()
		newCalendarContractHandler(d, &occurrenceStartCommandsStub{}, &calendarMonthCommandsStub{}).CreateOneOff(rec, calendarContractRequest(t, http.MethodPost, "/api/v1/circles/"+calendarContractCircleID+"/planned-sessions", body, path, true))
		calendarContractError(t, rec, http.StatusForbidden, httpconst.ErrorCodeForbidden)
	})
	t.Run("past plan maps to validation envelope", func(t *testing.T) {
		d := &plannedSessionCommandsStub{createFn: func(context.Context, CreatePlannedSessionCommand) (PlannedSessionView, error) {
			return PlannedSessionView{}, ErrPastPlannedTime
		}}
		rec := httptest.NewRecorder()
		newCalendarContractHandler(d, &occurrenceStartCommandsStub{}, &calendarMonthCommandsStub{}).CreateOneOff(rec, calendarContractRequest(t, http.MethodPost, "/api/v1/circles/"+calendarContractCircleID+"/planned-sessions", body, path, true))
		calendarContractError(t, rec, http.StatusUnprocessableEntity, httpconst.ErrorCodeValidationFailed)
	})
}

func TestCalendarHandlerChangeOneOffContract(t *testing.T) {
	path := map[string]string{"sessionId": calendarContractSessionID}
	body := `{"expected_version":1,"cancelled":true,"confirm_overlaps":false}`
	var got ChangePlannedSessionCommand
	planned := &plannedSessionCommandsStub{changeFn: func(_ context.Context, cmd ChangePlannedSessionCommand) (PlannedSessionView, error) {
		got = cmd
		return PlannedSessionView{ID: calendarContractSessionID, CircleID: calendarContractCircleID, CircleName: "Contract Circle", Title: "Special review", StartsAt: time.Date(2030, 1, 6, 19, 30, 0, 0, time.UTC), EndsAt: time.Date(2030, 1, 6, 20, 0, 0, 0, time.UTC), PlanningTimezone: "Asia/Riyadh", Status: "scheduled", Cancelled: true}, nil
	}}
	handler := newCalendarContractHandler(planned, &occurrenceStartCommandsStub{}, &calendarMonthCommandsStub{})
	rec := httptest.NewRecorder()
	handler.ChangeOneOff(rec, calendarContractRequest(t, http.MethodPatch, "/api/v1/sessions/"+calendarContractSessionID+"/planned-details", body, path, true))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got.ActorID != calendarContractActorID || got.SessionID != calendarContractSessionID || got.IdempotencyKey != "calendar-contract-key" || got.ExpectedVersion != 1 || got.Cancelled == nil || !*got.Cancelled || got.Plan != nil {
		t.Fatalf("change command: %+v", got)
	}
	var item CalendarItem
	if err := json.Unmarshal(rec.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode CalendarItem: %v", err)
	}
	if item.State != "cancelled" {
		t.Fatalf("cancelled state=%q", item.State)
	}
	t.Run("edit one-off maps plan fields to the existing service command", func(t *testing.T) {
		editBody := `{"expected_version":1,"plan":{"mode":"one_off","title":"Updated review","anchor_local_date":"2030-01-08","local_start_time":"09:00","local_end_time":"10:30","duration_minutes":90,"timezone":"Asia/Riyadh"},"confirm_overlaps":false}`
		var edited ChangePlannedSessionCommand
		d := &plannedSessionCommandsStub{changeFn: func(_ context.Context, cmd ChangePlannedSessionCommand) (PlannedSessionView, error) {
			edited = cmd
			return PlannedSessionView{ID: calendarContractSessionID, CircleID: calendarContractCircleID, CircleName: "Contract Circle", Title: "Updated review", StartsAt: time.Date(2030, 1, 8, 6, 0, 0, 0, time.UTC), EndsAt: time.Date(2030, 1, 8, 7, 30, 0, 0, time.UTC), PlanningTimezone: "Asia/Riyadh", Status: "scheduled"}, nil
		}}
		rec := httptest.NewRecorder()
		newCalendarContractHandler(d, &occurrenceStartCommandsStub{}, &calendarMonthCommandsStub{}).ChangeOneOff(rec, calendarContractRequest(t, http.MethodPatch, "/api/v1/sessions/"+calendarContractSessionID+"/planned-details", editBody, path, true))
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		if edited.ActorID != calendarContractActorID || edited.SessionID != calendarContractSessionID || edited.IdempotencyKey != "calendar-contract-key" || edited.ExpectedVersion != 1 || edited.Plan == nil || edited.Plan.Title != "Updated review" || !edited.Plan.LocalDate.Equal(time.Date(2030, 1, 8, 0, 0, 0, 0, time.UTC)) || edited.Plan.StartLocalTime != (LocalClock{Hour: 9}) || edited.Plan.EndLocalTime != (LocalClock{Hour: 10, Minute: 30}) || edited.Plan.DurationMinutes != 90 || edited.Plan.Timezone != "Asia/Riyadh" || edited.Cancelled != nil {
			t.Fatalf("edit command: %+v", edited)
		}
	})
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"missing planned session is not found", ErrPlannedSessionNotFound, http.StatusNotFound, httpconst.ErrorCodeNotFound},
		{"started occurrence cannot be cancelled", ErrOccurrenceStarted, http.StatusConflict, httpconst.ErrorCodeConflict},
		{"unauthorized role", ErrInsufficientCircleRole, http.StatusForbidden, httpconst.ErrorCodeForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &plannedSessionCommandsStub{changeFn: func(context.Context, ChangePlannedSessionCommand) (PlannedSessionView, error) {
				return PlannedSessionView{}, tc.err
			}}
			rec := httptest.NewRecorder()
			newCalendarContractHandler(d, &occurrenceStartCommandsStub{}, &calendarMonthCommandsStub{}).ChangeOneOff(rec, calendarContractRequest(t, http.MethodPatch, "/api/v1/sessions/"+calendarContractSessionID+"/planned-details", body, path, true))
			calendarContractError(t, rec, tc.status, tc.code)
		})
	}
	t.Run("missing authentication uses standard envelope", func(t *testing.T) {
		rec := httptest.NewRecorder()
		handler.ChangeOneOff(rec, calendarContractRequest(t, http.MethodPatch, "/api/v1/sessions/"+calendarContractSessionID+"/planned-details", body, path, false))
		calendarContractError(t, rec, http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized)
	})
}

func TestCalendarHandlerStartOccurrenceUsesF005Contract(t *testing.T) {
	path := map[string]string{"circleId": calendarContractCircleID, "scheduleId": calendarContractScheduleID, "localDate": "2030-01-06"}
	var got StartOccurrenceCommand
	starts := &occurrenceStartCommandsStub{startFn: func(_ context.Context, cmd StartOccurrenceCommand) (sessions.Session, sessions.MediaConnection, error) {
		got = cmd
		return sessions.Session{ID: calendarContractSessionID, CircleID: calendarContractCircleID, CreatedBy: calendarContractActorID, Status: sessions.SessionStatusActive, MediaMode: sessions.MediaModeAudioOnly}, sessions.MediaConnection{Endpoint: "wss://media.example", Credential: sessions.MediaCredential("opaque"), ExpiresAt: time.Date(2030, 1, 6, 23, 0, 0, 0, time.UTC)}, nil
	}}
	handler := newCalendarContractHandler(&plannedSessionCommandsStub{}, starts, &calendarMonthCommandsStub{})
	request := calendarContractRequest(t, http.MethodPost, "/api/v1/circles/"+calendarContractCircleID+"/schedules/"+calendarContractScheduleID+"/occurrences/2030-01-06/start", "", path, true)
	rec := httptest.NewRecorder()
	handler.StartOccurrence(rec, request)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got.ActorID != calendarContractActorID || got.CircleID != calendarContractCircleID || got.ScheduleID != calendarContractScheduleID || got.IdempotencyKey != "calendar-contract-key" {
		t.Fatalf("start command: %+v", got)
	}
	if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Pragma") != "no-cache" {
		t.Fatalf("credential cache headers: Cache-Control=%q Pragma=%q", rec.Header().Get("Cache-Control"), rec.Header().Get("Pragma"))
	}
	var response struct {
		Session struct {
			ID               string     `json:"id"`
			CircleID         string     `json:"circle_id"`
			CreatedBy        string     `json:"created_by"`
			Status           string     `json:"status"`
			MediaMode        string     `json:"media_mode"`
			ActualStart      *time.Time `json:"actual_start"`
			ActualEnd        *time.Time `json:"actual_end"`
			ParticipantCount int        `json:"participant_count"`
			IsLocked         bool       `json:"is_locked"`
		} `json:"session"`
		MediaConnection struct {
			Endpoint   string    `json:"endpoint"`
			Credential string    `json:"credential"`
			ExpiresAt  time.Time `json:"expires_at"`
		} `json:"media_connection"`
		IsModerator bool `json:"is_moderator"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode start response: %v", err)
	}
	if response.Session.ID != calendarContractSessionID || response.Session.CircleID != calendarContractCircleID || response.Session.CreatedBy != calendarContractActorID || response.Session.Status != string(sessions.SessionStatusActive) || response.Session.MediaMode != string(sessions.MediaModeAudioOnly) || response.Session.ActualStart != nil || response.Session.ActualEnd != nil || response.Session.ParticipantCount != 0 || response.Session.IsLocked || !response.IsModerator {
		t.Fatalf("SessionStartResponse.session: %+v moderator=%v", response.Session, response.IsModerator)
	}
	if response.MediaConnection.Credential != "opaque" || response.MediaConnection.Endpoint != "wss://media.example" || !response.MediaConnection.ExpiresAt.Equal(time.Date(2030, 1, 6, 23, 0, 0, 0, time.UTC)) {
		t.Fatalf("SessionStartResponse.media_connection: %+v", response.MediaConnection)
	}
	if strings.Contains(rec.Body.String(), "media_room_ref") {
		t.Fatal("response exposed internal media room reference")
	}

	t.Run("media unavailable maps to 503 and preserves no-store", func(t *testing.T) {
		d := &occurrenceStartCommandsStub{startFn: func(context.Context, StartOccurrenceCommand) (sessions.Session, sessions.MediaConnection, error) {
			return sessions.Session{}, sessions.MediaConnection{}, errors.Join(sessions.ErrMediaUnavailable, errors.New("adapter offline"))
		}}
		rec := httptest.NewRecorder()
		newCalendarContractHandler(&plannedSessionCommandsStub{}, d, &calendarMonthCommandsStub{}).StartOccurrence(rec, calendarContractRequest(t, http.MethodPost, request.URL.Path, "", path, true))
		calendarContractError(t, rec, http.StatusServiceUnavailable, httpconst.ErrorCodeMediaUnavailable)
		if rec.Header().Get("Cache-Control") != "no-store" || rec.Header().Get("Pragma") != "no-cache" {
			t.Fatalf("503 cache headers: %v", rec.Header())
		}
		if strings.Contains(rec.Body.String(), "wss://media.example") || strings.Contains(rec.Body.String(), "opaque") {
			t.Fatalf("media failure response leaked connection data: %s", rec.Body.String())
		}
	})
	t.Run("missing authentication uses standard envelope", func(t *testing.T) {
		rec := httptest.NewRecorder()
		newCalendarContractHandler(&plannedSessionCommandsStub{}, &occurrenceStartCommandsStub{}, &calendarMonthCommandsStub{}).StartOccurrence(rec, calendarContractRequest(t, http.MethodPost, request.URL.Path, "", path, false))
		calendarContractError(t, rec, http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized)
	})
	t.Run("student cannot start scheduled sessions", func(t *testing.T) {
		d := &occurrenceStartCommandsStub{startFn: func(context.Context, StartOccurrenceCommand) (sessions.Session, sessions.MediaConnection, error) {
			return sessions.Session{}, sessions.MediaConnection{}, ErrInsufficientCircleRole
		}}
		rec := httptest.NewRecorder()
		newCalendarContractHandler(&plannedSessionCommandsStub{}, d, &calendarMonthCommandsStub{}).StartOccurrence(rec, calendarContractRequest(t, http.MethodPost, request.URL.Path, "", path, true))
		calendarContractError(t, rec, http.StatusForbidden, httpconst.ErrorCodeForbidden)
	})
	t.Run("unknown occurrence is not found", func(t *testing.T) {
		d := &occurrenceStartCommandsStub{startFn: func(context.Context, StartOccurrenceCommand) (sessions.Session, sessions.MediaConnection, error) {
			return sessions.Session{}, sessions.MediaConnection{}, ErrOccurrenceNotFound
		}}
		rec := httptest.NewRecorder()
		newCalendarContractHandler(&plannedSessionCommandsStub{}, d, &calendarMonthCommandsStub{}).StartOccurrence(rec, calendarContractRequest(t, http.MethodPost, request.URL.Path, "", path, true))
		calendarContractError(t, rec, http.StatusNotFound, httpconst.ErrorCodeNotFound)
	})
}

func TestCalendarHandlerGetPersonalMonthContract(t *testing.T) {
	var gotActor, gotMonth string
	item := CalendarItem{OccurrenceKey: calendarContractScheduleID + ":2030-01-06", CircleID: calendarContractCircleID, CircleName: "Contract Circle", Title: "Hifz Review", StartsAt: time.Date(2030, 1, 6, 19, 30, 0, 0, time.UTC), EndsAt: time.Date(2030, 1, 6, 20, 0, 0, 0, time.UTC), PlanningTimezone: "Asia/Riyadh", State: "completed"}
	calendar := &calendarMonthCommandsStub{monthFn: func(_ context.Context, actorID, month string) (CalendarMonth, error) {
		gotActor, gotMonth = actorID, month
		return CalendarMonth{Items: []CalendarItem{item}, Warnings: WarningResult{Warnings: []OverlapWarning{}}}, nil
	}}
	handler := newCalendarContractHandler(&plannedSessionCommandsStub{}, &occurrenceStartCommandsStub{}, calendar)
	req := calendarContractRequest(t, http.MethodGet, "/api/v1/calendar/me?month=2030-01", "", nil, true)
	rec := httptest.NewRecorder()
	handler.GetPersonalMonth(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if gotActor != calendarContractActorID || gotMonth != "2030-01" {
		t.Fatalf("month query actor=%q month=%q", gotActor, gotMonth)
	}
	var body struct {
		Items    []CalendarItem `json:"items"`
		Warnings WarningResult  `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode month response: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].OccurrenceKey != item.OccurrenceKey || body.Items[0].CircleID != item.CircleID || body.Items[0].CircleName != item.CircleName || body.Items[0].Title != item.Title || !body.Items[0].StartsAt.Equal(item.StartsAt) || !body.Items[0].EndsAt.Equal(item.EndsAt) || body.Items[0].State != "completed" || body.Items[0].PlanningTimezone != "Asia/Riyadh" {
		t.Fatalf("calendar item projection: %+v", body.Items)
	}
	if !body.Items[0].StartsAt.Equal(item.StartsAt) || !body.Items[0].EndsAt.Equal(item.EndsAt) || body.Warnings.Warnings == nil || len(body.Warnings.Warnings) != 0 {
		t.Fatalf("month response: %+v", body)
	}
	for _, tc := range []struct {
		name, target  string
		authenticated bool
		status        int
		code          string
	}{
		{"malformed month", "/api/v1/calendar/me?month=2030-13", true, http.StatusBadRequest, httpconst.ErrorCodeValidationFailed},
		{"missing authentication", "/api/v1/calendar/me?month=2030-01", false, http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			newCalendarContractHandler(&plannedSessionCommandsStub{}, &occurrenceStartCommandsStub{}, &calendarMonthCommandsStub{}).GetPersonalMonth(rec, calendarContractRequest(t, http.MethodGet, tc.target, "", nil, tc.authenticated))
			calendarContractError(t, rec, tc.status, tc.code)
		})
	}
}
