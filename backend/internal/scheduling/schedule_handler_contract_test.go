//go:build contract

package scheduling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// F-006 US1 contract tests for the circle schedule REST surface
// (contracts/schedule-calendar.openapi.yaml): list/create, series patch and
// occurrence patch. The service is stubbed at the ScheduleCommands boundary so
// every approved status mapping (400/401/403/404/409/422/429) and the full
// response projections are exercised without a database.

const (
	contractActorID    = "11111111-1111-1111-1111-111111111111"
	contractCircleID   = "33333333-3333-3333-3333-333333333333"
	contractScheduleID = "55555555-5555-5555-5555-555555555555"
	contractSessionID  = "44444444-4444-4444-4444-444444444444"
)

// scheduleServiceStub implements ScheduleCommands with programmable outcomes.
type scheduleServiceStub struct {
	listFn             func(context.Context, string, string) ([]ScheduleView, error)
	createFn           func(context.Context, CreateScheduleCommand) (Schedule, error)
	changeFn           func(context.Context, ChangeScheduleCommand) (Schedule, error)
	changeOccurrenceFn func(context.Context, ChangeOccurrenceCommand) (OccurrenceException, error)
	occurrenceViewFn   func(context.Context, string, string, string, time.Time) (OccurrenceView, error)
}

func (s *scheduleServiceStub) List(ctx context.Context, circleID, actorID string) ([]ScheduleView, error) {
	if s.listFn != nil {
		return s.listFn(ctx, circleID, actorID)
	}
	return []ScheduleView{}, nil
}

func (s *scheduleServiceStub) Create(ctx context.Context, cmd CreateScheduleCommand) (Schedule, error) {
	if s.createFn != nil {
		return s.createFn(ctx, cmd)
	}
	return Schedule{ID: contractScheduleID, CircleID: cmd.CircleID, CreatedBy: cmd.ActorID, CurrentVersion: 1}, nil
}

func (s *scheduleServiceStub) Change(ctx context.Context, cmd ChangeScheduleCommand) (Schedule, error) {
	if s.changeFn != nil {
		return s.changeFn(ctx, cmd)
	}
	return Schedule{ID: cmd.ScheduleID, CircleID: cmd.CircleID, CreatedBy: cmd.ActorID, CurrentVersion: cmd.ExpectedVersion + 1}, nil
}

func (s *scheduleServiceStub) ChangeOccurrence(ctx context.Context, cmd ChangeOccurrenceCommand) (OccurrenceException, error) {
	if s.changeOccurrenceFn != nil {
		return s.changeOccurrenceFn(ctx, cmd)
	}
	return OccurrenceException{ScheduleID: cmd.ScheduleID, OriginalLocalDate: cmd.OriginalLocalDate, Version: 1}, nil
}

func (s *scheduleServiceStub) OccurrenceView(_ context.Context, circleID, scheduleID, _ string, original time.Time) (OccurrenceView, error) {
	if s.occurrenceViewFn != nil {
		return s.occurrenceViewFn(context.Background(), circleID, scheduleID, contractActorID, original)
	}
	// Asia/Riyadh (UTC+3): 22:30 local is 19:30 UTC.
	start := time.Date(2030, 1, 2, 19, 30, 0, 0, time.UTC)
	return OccurrenceView{
		ScheduleID:        scheduleID,
		CircleID:          circleID,
		CircleName:        "Contract Circle",
		OriginalLocalDate: original,
		Title:             "Hifz Review",
		StartsAt:          start,
		EndsAt:            start.Add(30 * time.Minute),
		Timezone:          "Asia/Riyadh",
	}, nil
}

// ---- request / assertion helpers -------------------------------------------

func contractPlanBody() string {
	return `{
		"mode": "weekday_pattern",
		"title": "Hifz Review",
		"anchor_local_date": "2030-01-06",
		"local_start_time": "22:30",
		"local_end_time": "23:00",
		"duration_minutes": 30,
		"timezone": "Asia/Riyadh",
		"week_cadence": 1,
		"weekdays": [1, 3],
		"confirm_overlaps": false
	}`
}

func scheduleContractRequest(t *testing.T, method, target, body string, pathValues map[string]string) *http.Request {
	t.Helper()
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, target, nil)
	} else {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	}
	if method != http.MethodGet {
		req.Header.Set(httpconst.HeaderIdempotencyKey, "contract-key-1")
	}
	for name, value := range pathValues {
		req.SetPathValue(name, value)
	}
	return req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: contractActorID}))
}

func decodeContractBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response body: %v body=%q", err, rec.Body.String())
	}
	return body
}

func assertContractError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, wantStatus, rec.Body.String())
	}
	if ct := rec.Header().Get(httpconst.HeaderContentType); ct != httpconst.ContentTypeApplicationJSON {
		t.Fatalf("content type: got %q, want %q", ct, httpconst.ContentTypeApplicationJSON)
	}
	body := decodeContractBody(t, rec)
	errBody, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope missing: body=%s", rec.Body.String())
	}
	if errBody["code"] != wantCode {
		t.Fatalf("error code: got %v, want %q body=%s", errBody["code"], wantCode, rec.Body.String())
	}
	if message, _ := errBody["message"].(string); strings.TrimSpace(message) == "" {
		t.Fatalf("error message empty: body=%s", rec.Body.String())
	}
}

func assertScheduleProjection(t *testing.T, schedule map[string]any, wantID, wantCircleID string, wantVersion float64) {
	t.Helper()
	if schedule["id"] != wantID {
		t.Fatalf("schedule id: got %v, want %q", schedule["id"], wantID)
	}
	if schedule["circle_id"] != wantCircleID {
		t.Fatalf("schedule circle_id: got %v, want %q", schedule["circle_id"], wantCircleID)
	}
	if schedule["version"] != wantVersion {
		t.Fatalf("schedule version: got %v, want %v", schedule["version"], wantVersion)
	}
	plan, ok := schedule["plan"].(map[string]any)
	if !ok {
		t.Fatalf("schedule plan missing: %v", schedule)
	}
	for _, field := range []string{"mode", "anchor_local_date", "local_start_time", "local_end_time", "duration_minutes", "timezone"} {
		if _, present := plan[field]; !present {
			t.Fatalf("plan field %q missing: %v", field, plan)
		}
	}
}

// ---- list -------------------------------------------------------------------

func TestScheduleHandler_ListSchedules(t *testing.T) {
	circlePath := map[string]string{"circleId": contractCircleID}

	t.Run("member receives the full schedule projection", func(t *testing.T) {
		anchor := time.Date(2030, 1, 6, 0, 0, 0, 0, time.UTC)
		stub := &scheduleServiceStub{listFn: func(_ context.Context, circleID, actorID string) ([]ScheduleView, error) {
			if circleID != contractCircleID || actorID != contractActorID {
				t.Fatalf("list scope: got circle=%q actor=%q", circleID, actorID)
			}
			return []ScheduleView{{
				Schedule:           Schedule{ID: contractScheduleID, CircleID: contractCircleID, CurrentVersion: 2},
				OccurrenceVersions: map[string]int{"2030-01-06": 3},
				Plan: RevisionRecord{
					Revision:        Revision{Version: 2, Mode: ModeWeekdayPattern, AnchorLocalDate: anchor, WeekCadence: 1, Weekdays: []int{1, 3}},
					Title:           "Hifz Review",
					StartLocalTime:  LocalClock{Hour: 22, Minute: 30},
					EndLocalTime:    LocalClock{Hour: 23, Minute: 0},
					DurationMinutes: 30,
					Timezone:        "Asia/Riyadh",
				},
			}}, nil
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.ListSchedules(rec, scheduleContractRequest(t, http.MethodGet, "/api/v1/circles/"+contractCircleID+"/schedules", "", circlePath))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		body := decodeContractBody(t, rec)
		data, ok := body["data"].([]any)
		if !ok || len(data) != 1 {
			t.Fatalf("data: got %v", body)
		}
		schedule, ok := data[0].(map[string]any)
		if !ok {
			t.Fatalf("schedule item: %v", data[0])
		}
		assertScheduleProjection(t, schedule, contractScheduleID, contractCircleID, 2)
		versions, ok := schedule["occurrence_versions"].(map[string]any)
		if !ok || versions["2030-01-06"] != float64(3) {
			t.Fatalf("retained occurrence versions missing: %v", schedule["occurrence_versions"])
		}
		plan := schedule["plan"].(map[string]any)
		if plan["mode"] != "weekday_pattern" || plan["title"] != "Hifz Review" || plan["timezone"] != "Asia/Riyadh" {
			t.Fatalf("plan projection: %v", plan)
		}
		if plan["anchor_local_date"] != "2030-01-06" || plan["local_start_time"] != "22:30" || plan["local_end_time"] != "23:00" {
			t.Fatalf("plan local times: %v", plan)
		}
		if plan["week_cadence"] != float64(1) {
			t.Fatalf("plan week_cadence: %v", plan)
		}
		weekdays, ok := plan["weekdays"].([]any)
		if !ok || len(weekdays) != 2 {
			t.Fatalf("plan weekdays: %v", plan)
		}
	})

	t.Run("empty list returns an empty data array", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		rec := httptest.NewRecorder()
		handler.ListSchedules(rec, scheduleContractRequest(t, http.MethodGet, "/api/v1/circles/"+contractCircleID+"/schedules", "", circlePath))
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
		}
		body := decodeContractBody(t, rec)
		data, ok := body["data"].([]any)
		if !ok || len(data) != 0 {
			t.Fatalf("data: got %v, want empty array", body)
		}
	})

	t.Run("missing principal is unauthorized", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		req := scheduleContractRequest(t, http.MethodGet, "/api/v1/circles/"+contractCircleID+"/schedules", "", circlePath)
		req = req.WithContext(context.Background())
		rec := httptest.NewRecorder()
		handler.ListSchedules(rec, req)
		assertContractError(t, rec, http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized)
	})

	t.Run("invalid circle id is a validation error", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		rec := httptest.NewRecorder()
		handler.ListSchedules(rec, scheduleContractRequest(t, http.MethodGet, "/api/v1/circles/not-a-uuid/schedules", "", map[string]string{"circleId": "not-a-uuid"}))
		assertContractError(t, rec, http.StatusBadRequest, httpconst.ErrorCodeValidationFailed)
	})

	t.Run("non-member or unknown circle is not found", func(t *testing.T) {
		stub := &scheduleServiceStub{listFn: func(context.Context, string, string) ([]ScheduleView, error) {
			return nil, ErrCircleNotFoundOrDenied
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.ListSchedules(rec, scheduleContractRequest(t, http.MethodGet, "/api/v1/circles/"+contractCircleID+"/schedules", "", circlePath))
		assertContractError(t, rec, http.StatusNotFound, httpconst.ErrorCodeNotFound)
	})

	t.Run("unexpected failure is an internal error without details", func(t *testing.T) {
		stub := &scheduleServiceStub{listFn: func(context.Context, string, string) ([]ScheduleView, error) {
			return nil, errors.New("db connection refused: secret detail")
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.ListSchedules(rec, scheduleContractRequest(t, http.MethodGet, "/api/v1/circles/"+contractCircleID+"/schedules", "", circlePath))
		assertContractError(t, rec, http.StatusInternalServerError, httpconst.ErrorCodeInternalServerError)
		if strings.Contains(rec.Body.String(), "secret detail") {
			t.Fatalf("internal error leaks details: %s", rec.Body.String())
		}
	})

	t.Run("per user rate limit returns 429", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		limited := middleware.NewRateLimitMiddleware(0, 1).Limit(http.HandlerFunc(handler.ListSchedules))
		first := httptest.NewRecorder()
		limited.ServeHTTP(first, scheduleContractRequest(t, http.MethodGet, "/api/v1/circles/"+contractCircleID+"/schedules", "", circlePath))
		if first.Code != http.StatusOK {
			t.Fatalf("first request: got %d body=%s", first.Code, first.Body.String())
		}
		second := httptest.NewRecorder()
		limited.ServeHTTP(second, scheduleContractRequest(t, http.MethodGet, "/api/v1/circles/"+contractCircleID+"/schedules", "", circlePath))
		assertContractError(t, second, http.StatusTooManyRequests, httpconst.ErrorCodeRateLimitExceeded)
	})
}

// ---- create -----------------------------------------------------------------

func TestScheduleHandler_CreateSchedule(t *testing.T) {
	circlePath := map[string]string{"circleId": contractCircleID}
	target := "/api/v1/circles/" + contractCircleID + "/schedules"

	t.Run("teacher creates a weekly recurrence and receives the created schedule", func(t *testing.T) {
		var captured CreateScheduleCommand
		stub := &scheduleServiceStub{createFn: func(_ context.Context, cmd CreateScheduleCommand) (Schedule, error) {
			captured = cmd
			return Schedule{ID: contractScheduleID, CircleID: cmd.CircleID, CreatedBy: cmd.ActorID, CurrentVersion: 1}, nil
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.CreateSchedule(rec, scheduleContractRequest(t, http.MethodPost, target, contractPlanBody(), circlePath))

		if rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		if captured.ActorID != contractActorID || captured.CircleID != contractCircleID || captured.IdempotencyKey != "contract-key-1" {
			t.Fatalf("command scope: %+v", captured)
		}
		if captured.Plan.Mode != ModeWeekdayPattern || captured.Plan.WeekCadence != 1 || len(captured.Plan.Weekdays) != 2 {
			t.Fatalf("command plan: %+v", captured.Plan)
		}
		if captured.Plan.StartLocalTime != (LocalClock{Hour: 22, Minute: 30}) || captured.Plan.EndLocalTime != (LocalClock{Hour: 23, Minute: 0}) {
			t.Fatalf("command clocks: %+v", captured.Plan)
		}
		if captured.Plan.DurationMinutes != 30 || captured.Plan.Timezone != "Asia/Riyadh" || captured.Plan.Title != "Hifz Review" {
			t.Fatalf("command details: %+v", captured.Plan)
		}
		assertScheduleProjection(t, decodeContractBody(t, rec), contractScheduleID, contractCircleID, 1)
	})

	t.Run("student role is denied", func(t *testing.T) {
		stub := &scheduleServiceStub{createFn: func(context.Context, CreateScheduleCommand) (Schedule, error) {
			return Schedule{}, ErrInsufficientCircleRole
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.CreateSchedule(rec, scheduleContractRequest(t, http.MethodPost, target, contractPlanBody(), circlePath))
		assertContractError(t, rec, http.StatusForbidden, httpconst.ErrorCodeForbidden)
	})

	t.Run("archived circle rejects the write", func(t *testing.T) {
		stub := &scheduleServiceStub{createFn: func(context.Context, CreateScheduleCommand) (Schedule, error) {
			return Schedule{}, ErrCircleArchived
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.CreateSchedule(rec, scheduleContractRequest(t, http.MethodPost, target, contractPlanBody(), circlePath))
		assertContractError(t, rec, http.StatusForbidden, httpconst.ErrorCodeForbidden)
	})

	t.Run("non-member circle is not found", func(t *testing.T) {
		stub := &scheduleServiceStub{createFn: func(context.Context, CreateScheduleCommand) (Schedule, error) {
			return Schedule{}, ErrCircleNotFoundOrDenied
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.CreateSchedule(rec, scheduleContractRequest(t, http.MethodPost, target, contractPlanBody(), circlePath))
		assertContractError(t, rec, http.StatusNotFound, httpconst.ErrorCodeNotFound)
	})

	t.Run("idempotency replay with different input conflicts", func(t *testing.T) {
		stub := &scheduleServiceStub{createFn: func(context.Context, CreateScheduleCommand) (Schedule, error) {
			return Schedule{}, ErrReplayConflict
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.CreateSchedule(rec, scheduleContractRequest(t, http.MethodPost, target, contractPlanBody(), circlePath))
		assertContractError(t, rec, http.StatusConflict, httpconst.ErrorCodeConflict)
	})

	t.Run("past planned time is unprocessable", func(t *testing.T) {
		stub := &scheduleServiceStub{createFn: func(context.Context, CreateScheduleCommand) (Schedule, error) {
			return Schedule{}, ErrPastPlannedTime
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.CreateSchedule(rec, scheduleContractRequest(t, http.MethodPost, target, contractPlanBody(), circlePath))
		assertContractError(t, rec, http.StatusUnprocessableEntity, httpconst.ErrorCodeValidationFailed)
	})

	t.Run("invalid scheduling semantics are unprocessable", func(t *testing.T) {
		semanticErrors := []error{
			ErrUnknownRecurrenceMode, ErrInvalidWeekdayPattern, ErrInvalidInterval,
			ErrInvalidSelectedDates, ErrInvalidRevision, ErrInvalidTimezone,
			ErrInvalidLocalClock, ErrInvalidDuration, ErrEndClockMismatch,
			ErrInvalidScheduleCommand,
		}
		for _, domainErr := range semanticErrors {
			stub := &scheduleServiceStub{createFn: func(context.Context, CreateScheduleCommand) (Schedule, error) {
				return Schedule{}, fmt.Errorf("validate: %w", domainErr)
			}}
			handler := NewScheduleHandler(stub)
			rec := httptest.NewRecorder()
			handler.CreateSchedule(rec, scheduleContractRequest(t, http.MethodPost, target, contractPlanBody(), circlePath))
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("%v: status got %d, want %d body=%s", domainErr, rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
			}
		}
	})

	t.Run("request syntax errors are 400", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		cases := []struct {
			name string
			body string
			key  string
		}{
			{name: "malformed json", body: `{"mode":`, key: "k"},
			{name: "unknown field", body: `{"mode":"interval","anchor_local_date":"2030-01-06","local_start_time":"22:30","local_end_time":"23:00","duration_minutes":30,"timezone":"Asia/Riyadh","surprise":true}`, key: "k"},
			{name: "missing mode", body: `{"anchor_local_date":"2030-01-06","local_start_time":"22:30","local_end_time":"23:00","duration_minutes":30,"timezone":"Asia/Riyadh"}`, key: "k"},
			{name: "missing timezone", body: `{"mode":"interval","anchor_local_date":"2030-01-06","local_start_time":"22:30","local_end_time":"23:00","duration_minutes":30}`, key: "k"},
			{name: "invalid anchor date", body: `{"mode":"interval","anchor_local_date":"2030-13-40","local_start_time":"22:30","local_end_time":"23:00","duration_minutes":30,"timezone":"Asia/Riyadh"}`, key: "k"},
			{name: "invalid start clock", body: `{"mode":"interval","anchor_local_date":"2030-01-06","local_start_time":"25:30","local_end_time":"23:00","duration_minutes":30,"timezone":"Asia/Riyadh"}`, key: "k"},
			{name: "invalid end date", body: `{"mode":"interval","anchor_local_date":"2030-01-06","local_start_time":"22:30","local_end_time":"23:00","duration_minutes":30,"timezone":"Asia/Riyadh","end_local_date":"tomorrow"}`, key: "k"},
			{name: "missing idempotency key", body: contractPlanBody(), key: ""},
			{name: "idempotency key too long", body: contractPlanBody(), key: strings.Repeat("k", 129)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				req := scheduleContractRequest(t, http.MethodPost, target, tc.body, circlePath)
				if tc.key == "" {
					req.Header.Del(httpconst.HeaderIdempotencyKey)
				} else {
					req.Header.Set(httpconst.HeaderIdempotencyKey, tc.key)
				}
				rec := httptest.NewRecorder()
				handler.CreateSchedule(rec, req)
				assertContractError(t, rec, http.StatusBadRequest, httpconst.ErrorCodeValidationFailed)
			})
		}
	})

	t.Run("invalid circle id is a validation error", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		rec := httptest.NewRecorder()
		handler.CreateSchedule(rec, scheduleContractRequest(t, http.MethodPost, "/api/v1/circles/nope/schedules", contractPlanBody(), map[string]string{"circleId": "nope"}))
		assertContractError(t, rec, http.StatusBadRequest, httpconst.ErrorCodeValidationFailed)
	})

	t.Run("missing principal is unauthorized", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		req := scheduleContractRequest(t, http.MethodPost, target, contractPlanBody(), circlePath).WithContext(context.Background())
		rec := httptest.NewRecorder()
		handler.CreateSchedule(rec, req)
		assertContractError(t, rec, http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized)
	})

	t.Run("per user rate limit returns 429", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		limited := middleware.NewRateLimitMiddleware(0, 1).Limit(http.HandlerFunc(handler.CreateSchedule))
		first := httptest.NewRecorder()
		limited.ServeHTTP(first, scheduleContractRequest(t, http.MethodPost, target, contractPlanBody(), circlePath))
		if first.Code != http.StatusCreated {
			t.Fatalf("first request: got %d body=%s", first.Code, first.Body.String())
		}
		second := httptest.NewRecorder()
		limited.ServeHTTP(second, scheduleContractRequest(t, http.MethodPost, target, contractPlanBody(), circlePath))
		assertContractError(t, second, http.StatusTooManyRequests, httpconst.ErrorCodeRateLimitExceeded)
	})
}

// ---- series change / stop ----------------------------------------------------

func TestScheduleHandler_ChangeSchedule(t *testing.T) {
	path := map[string]string{"circleId": contractCircleID, "scheduleId": contractScheduleID}
	target := "/api/v1/circles/" + contractCircleID + "/schedules/" + contractScheduleID
	changeBody := `{
		"expected_version": 1,
		"effective_local_date": "2030-02-01",
		"plan": {
			"mode": "interval",
			"anchor_local_date": "2030-01-06",
			"local_start_time": "20:00",
			"local_end_time": "21:00",
			"duration_minutes": 60,
			"timezone": "Asia/Riyadh",
			"interval_count": 2,
			"interval_unit": "week"
		},
		"confirm_overlaps": false
	}`

	t.Run("manager changes the series and receives the bumped schedule", func(t *testing.T) {
		var captured ChangeScheduleCommand
		stub := &scheduleServiceStub{changeFn: func(_ context.Context, cmd ChangeScheduleCommand) (Schedule, error) {
			captured = cmd
			return Schedule{ID: cmd.ScheduleID, CircleID: cmd.CircleID, CurrentVersion: cmd.ExpectedVersion + 1}, nil
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.ChangeSchedule(rec, scheduleContractRequest(t, http.MethodPatch, target, changeBody, path))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if captured.ActorID != contractActorID || captured.CircleID != contractCircleID || captured.ScheduleID != contractScheduleID {
			t.Fatalf("command scope: %+v", captured)
		}
		if captured.ExpectedVersion != 1 || captured.IdempotencyKey != "contract-key-1" || captured.Stop {
			t.Fatalf("command fields: %+v", captured)
		}
		if captured.EffectiveLocalDate.Format(time.DateOnly) != "2030-02-01" {
			t.Fatalf("effective date: %v", captured.EffectiveLocalDate)
		}
		if captured.Plan.Mode != ModeInterval || captured.Plan.IntervalCount != 2 || captured.Plan.IntervalUnit != IntervalUnitWeek {
			t.Fatalf("command plan: %+v", captured.Plan)
		}
		body := decodeContractBody(t, rec)
		assertScheduleProjection(t, body, contractScheduleID, contractCircleID, 2)
		plan := body["plan"].(map[string]any)
		if plan["mode"] != "interval" || plan["interval_count"] != float64(2) || plan["interval_unit"] != "week" {
			t.Fatalf("plan projection: %v", plan)
		}
	})

	t.Run("stop keeps the stop boundary in the response", func(t *testing.T) {
		boundary := time.Date(2030, 2, 1, 0, 0, 0, 0, time.UTC)
		stub := &scheduleServiceStub{changeFn: func(_ context.Context, cmd ChangeScheduleCommand) (Schedule, error) {
			if !cmd.Stop {
				t.Fatalf("stop flag not propagated: %+v", cmd)
			}
			return Schedule{ID: cmd.ScheduleID, CircleID: cmd.CircleID, CurrentVersion: 2, StoppedFromLocalDate: &boundary}, nil
		}}
		handler := NewScheduleHandler(stub)
		stopBody := strings.Replace(changeBody, `"confirm_overlaps": false`, `"stop": true, "confirm_overlaps": false`, 1)
		rec := httptest.NewRecorder()
		handler.ChangeSchedule(rec, scheduleContractRequest(t, http.MethodPatch, target, stopBody, path))
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
		}
		body := decodeContractBody(t, rec)
		if body["stopped_from_local_date"] != "2030-02-01" {
			t.Fatalf("stopped_from_local_date: %v", body)
		}
	})

	t.Run("stale version conflicts with the current outcome", func(t *testing.T) {
		stub := &scheduleServiceStub{changeFn: func(context.Context, ChangeScheduleCommand) (Schedule, error) {
			return Schedule{}, ErrScheduleConflict
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.ChangeSchedule(rec, scheduleContractRequest(t, http.MethodPatch, target, changeBody, path))
		assertContractError(t, rec, http.StatusConflict, httpconst.ErrorCodeConflict)
	})

	t.Run("unknown schedule is not found", func(t *testing.T) {
		stub := &scheduleServiceStub{changeFn: func(context.Context, ChangeScheduleCommand) (Schedule, error) {
			return Schedule{}, ErrScheduleNotFound
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.ChangeSchedule(rec, scheduleContractRequest(t, http.MethodPatch, target, changeBody, path))
		assertContractError(t, rec, http.StatusNotFound, httpconst.ErrorCodeNotFound)
	})

	t.Run("student role is denied and archived circle rejects the write", func(t *testing.T) {
		for _, domainErr := range []error{ErrInsufficientCircleRole, ErrCircleArchived} {
			stub := &scheduleServiceStub{changeFn: func(context.Context, ChangeScheduleCommand) (Schedule, error) {
				return Schedule{}, domainErr
			}}
			handler := NewScheduleHandler(stub)
			rec := httptest.NewRecorder()
			handler.ChangeSchedule(rec, scheduleContractRequest(t, http.MethodPatch, target, changeBody, path))
			assertContractError(t, rec, http.StatusForbidden, httpconst.ErrorCodeForbidden)
		}
	})

	t.Run("past effective boundary is unprocessable", func(t *testing.T) {
		stub := &scheduleServiceStub{changeFn: func(context.Context, ChangeScheduleCommand) (Schedule, error) {
			return Schedule{}, ErrPastPlannedTime
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.ChangeSchedule(rec, scheduleContractRequest(t, http.MethodPatch, target, changeBody, path))
		assertContractError(t, rec, http.StatusUnprocessableEntity, httpconst.ErrorCodeValidationFailed)
	})

	t.Run("request syntax errors are 400", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		cases := []struct {
			name string
			body string
		}{
			{name: "missing expected version", body: `{"effective_local_date":"2030-02-01","plan":{"mode":"interval","anchor_local_date":"2030-01-06","local_start_time":"20:00","local_end_time":"21:00","duration_minutes":60,"timezone":"Asia/Riyadh"},"confirm_overlaps":false}`},
			{name: "zero expected version", body: `{"expected_version":0,"effective_local_date":"2030-02-01","plan":{"mode":"interval","anchor_local_date":"2030-01-06","local_start_time":"20:00","local_end_time":"21:00","duration_minutes":60,"timezone":"Asia/Riyadh"},"confirm_overlaps":false}`},
			{name: "missing plan", body: `{"expected_version":1,"effective_local_date":"2030-02-01","confirm_overlaps":false}`},
			{name: "missing confirm overlaps", body: `{"expected_version":1,"effective_local_date":"2030-02-01","plan":{"mode":"interval","anchor_local_date":"2030-01-06","local_start_time":"20:00","local_end_time":"21:00","duration_minutes":60,"timezone":"Asia/Riyadh"}}`},
			{name: "invalid effective date", body: `{"expected_version":1,"effective_local_date":"next week","plan":{"mode":"interval","anchor_local_date":"2030-01-06","local_start_time":"20:00","local_end_time":"21:00","duration_minutes":60,"timezone":"Asia/Riyadh"},"confirm_overlaps":false}`},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				handler.ChangeSchedule(rec, scheduleContractRequest(t, http.MethodPatch, target, tc.body, path))
				assertContractError(t, rec, http.StatusBadRequest, httpconst.ErrorCodeValidationFailed)
			})
		}
	})

	t.Run("invalid schedule id is a validation error", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		rec := httptest.NewRecorder()
		handler.ChangeSchedule(rec, scheduleContractRequest(t, http.MethodPatch, target, changeBody, map[string]string{"circleId": contractCircleID, "scheduleId": "nope"}))
		assertContractError(t, rec, http.StatusBadRequest, httpconst.ErrorCodeValidationFailed)
	})

	t.Run("missing principal is unauthorized", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		req := scheduleContractRequest(t, http.MethodPatch, target, changeBody, path).WithContext(context.Background())
		rec := httptest.NewRecorder()
		handler.ChangeSchedule(rec, req)
		assertContractError(t, rec, http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized)
	})

	t.Run("per user rate limit returns 429", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		limited := middleware.NewRateLimitMiddleware(0, 1).Limit(http.HandlerFunc(handler.ChangeSchedule))
		first := httptest.NewRecorder()
		limited.ServeHTTP(first, scheduleContractRequest(t, http.MethodPatch, target, changeBody, path))
		if first.Code != http.StatusOK {
			t.Fatalf("first request: got %d body=%s", first.Code, first.Body.String())
		}
		second := httptest.NewRecorder()
		limited.ServeHTTP(second, scheduleContractRequest(t, http.MethodPatch, target, changeBody, path))
		assertContractError(t, second, http.StatusTooManyRequests, httpconst.ErrorCodeRateLimitExceeded)
	})
}

// ---- occurrence change --------------------------------------------------------

func TestScheduleHandler_ChangeScheduleOccurrence(t *testing.T) {
	path := map[string]string{"circleId": contractCircleID, "scheduleId": contractScheduleID, "localDate": "2030-01-02"}
	target := "/api/v1/circles/" + contractCircleID + "/schedules/" + contractScheduleID + "/occurrences/2030-01-02"
	occurrenceBody := `{
		"expected_series_version": 1,
		"expected_occurrence_version": 0,
		"cancelled": true,
		"confirm_overlaps": false
	}`

	t.Run("manager cancels one occurrence and receives the calendar item", func(t *testing.T) {
		var captured ChangeOccurrenceCommand
		stub := &scheduleServiceStub{
			changeOccurrenceFn: func(_ context.Context, cmd ChangeOccurrenceCommand) (OccurrenceException, error) {
				captured = cmd
				cancelledAt := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
				return OccurrenceException{ScheduleID: cmd.ScheduleID, OriginalLocalDate: cmd.OriginalLocalDate, Version: 1, CancelledAt: &cancelledAt}, nil
			},
			occurrenceViewFn: func(_ context.Context, circleID, scheduleID, actorID string, original time.Time) (OccurrenceView, error) {
				start := time.Date(2030, 1, 2, 19, 30, 0, 0, time.UTC)
				return OccurrenceView{
					ScheduleID:        scheduleID,
					CircleID:          circleID,
					CircleName:        "Contract Circle",
					OriginalLocalDate: original,
					Title:             "Hifz Review",
					StartsAt:          start,
					EndsAt:            start.Add(30 * time.Minute),
					Timezone:          "Asia/Riyadh",
					Cancelled:         true,
				}, nil
			},
		}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.ChangeScheduleOccurrence(rec, scheduleContractRequest(t, http.MethodPatch, target, occurrenceBody, path))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if captured.ActorID != contractActorID || captured.CircleID != contractCircleID || captured.ScheduleID != contractScheduleID {
			t.Fatalf("command scope: %+v", captured)
		}
		if captured.OriginalLocalDate.Format(time.DateOnly) != "2030-01-02" || captured.ExpectedSeriesVersion != 1 || captured.ExpectedOccurrenceVersion != 0 {
			t.Fatalf("command identity: %+v", captured)
		}
		if captured.Cancelled == nil || !*captured.Cancelled {
			t.Fatalf("cancel flag: %+v", captured.Cancelled)
		}
		body := decodeContractBody(t, rec)
		if body["occurrence_key"] != contractScheduleID+":2030-01-02" {
			t.Fatalf("occurrence_key: %v", body["occurrence_key"])
		}
		if body["circle_id"] != contractCircleID || body["circle_name"] != "Contract Circle" || body["title"] != "Hifz Review" {
			t.Fatalf("calendar identity: %v", body)
		}
		if body["starts_at"] != "2030-01-02T19:30:00Z" || body["ends_at"] != "2030-01-02T20:00:00Z" {
			t.Fatalf("resolved instants: %v", body)
		}
		if body["planning_timezone"] != "Asia/Riyadh" || body["state"] != "cancelled" {
			t.Fatalf("timezone/state: %v", body)
		}
		if _, present := body["session_id"]; present && body["session_id"] != nil {
			t.Fatalf("session_id should be null for a virtual occurrence: %v", body)
		}
	})

	t.Run("moved materialized occurrence carries its session id", func(t *testing.T) {
		stub := &scheduleServiceStub{occurrenceViewFn: func(_ context.Context, circleID, scheduleID, _ string, original time.Time) (OccurrenceView, error) {
			start := time.Date(2030, 1, 5, 19, 30, 0, 0, time.UTC)
			return OccurrenceView{
				ScheduleID:        scheduleID,
				CircleID:          circleID,
				CircleName:        "Contract Circle",
				OriginalLocalDate: original,
				Title:             "Hifz Review",
				StartsAt:          start,
				EndsAt:            start.Add(30 * time.Minute),
				Timezone:          "Asia/Riyadh",
				SessionID:         ptrString(contractSessionID),
			}, nil
		}}
		handler := NewScheduleHandler(stub)
		moveBody := `{
			"expected_series_version": 1,
			"expected_occurrence_version": 1,
			"replacement_local_date": "2030-01-05",
			"confirm_overlaps": false
		}`
		rec := httptest.NewRecorder()
		handler.ChangeScheduleOccurrence(rec, scheduleContractRequest(t, http.MethodPatch, target, moveBody, path))
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
		}
		body := decodeContractBody(t, rec)
		if body["session_id"] != contractSessionID || body["state"] != "scheduled" {
			t.Fatalf("moved item: %v", body)
		}
	})

	t.Run("stale occurrence or series version conflicts", func(t *testing.T) {
		for _, domainErr := range []error{ErrScheduleConflict, ErrReplayConflict} {
			stub := &scheduleServiceStub{changeOccurrenceFn: func(context.Context, ChangeOccurrenceCommand) (OccurrenceException, error) {
				return OccurrenceException{}, domainErr
			}}
			handler := NewScheduleHandler(stub)
			rec := httptest.NewRecorder()
			handler.ChangeScheduleOccurrence(rec, scheduleContractRequest(t, http.MethodPatch, target, occurrenceBody, path))
			assertContractError(t, rec, http.StatusConflict, httpconst.ErrorCodeConflict)
		}
	})

	t.Run("started occurrence cannot change", func(t *testing.T) {
		stub := &scheduleServiceStub{changeOccurrenceFn: func(context.Context, ChangeOccurrenceCommand) (OccurrenceException, error) {
			return OccurrenceException{}, ErrOccurrenceStarted
		}}
		handler := NewScheduleHandler(stub)
		rec := httptest.NewRecorder()
		handler.ChangeScheduleOccurrence(rec, scheduleContractRequest(t, http.MethodPatch, target, occurrenceBody, path))
		assertContractError(t, rec, http.StatusConflict, httpconst.ErrorCodeConflict)
	})

	t.Run("unknown occurrence or schedule is not found", func(t *testing.T) {
		for _, domainErr := range []error{ErrOccurrenceNotFound, ErrScheduleNotFound, ErrCircleNotFoundOrDenied} {
			stub := &scheduleServiceStub{changeOccurrenceFn: func(context.Context, ChangeOccurrenceCommand) (OccurrenceException, error) {
				return OccurrenceException{}, domainErr
			}}
			handler := NewScheduleHandler(stub)
			rec := httptest.NewRecorder()
			handler.ChangeScheduleOccurrence(rec, scheduleContractRequest(t, http.MethodPatch, target, occurrenceBody, path))
			assertContractError(t, rec, http.StatusNotFound, httpconst.ErrorCodeNotFound)
		}
	})

	t.Run("student role is denied and archived circle rejects the write", func(t *testing.T) {
		for _, domainErr := range []error{ErrInsufficientCircleRole, ErrCircleArchived} {
			stub := &scheduleServiceStub{changeOccurrenceFn: func(context.Context, ChangeOccurrenceCommand) (OccurrenceException, error) {
				return OccurrenceException{}, domainErr
			}}
			handler := NewScheduleHandler(stub)
			rec := httptest.NewRecorder()
			handler.ChangeScheduleOccurrence(rec, scheduleContractRequest(t, http.MethodPatch, target, occurrenceBody, path))
			assertContractError(t, rec, http.StatusForbidden, httpconst.ErrorCodeForbidden)
		}
	})

	t.Run("invalid occurrence semantics are unprocessable", func(t *testing.T) {
		for _, domainErr := range []error{ErrPastPlannedTime, ErrInvalidScheduleCommand, ErrInvalidException} {
			stub := &scheduleServiceStub{changeOccurrenceFn: func(context.Context, ChangeOccurrenceCommand) (OccurrenceException, error) {
				return OccurrenceException{}, fmt.Errorf("validate: %w", domainErr)
			}}
			handler := NewScheduleHandler(stub)
			rec := httptest.NewRecorder()
			handler.ChangeScheduleOccurrence(rec, scheduleContractRequest(t, http.MethodPatch, target, occurrenceBody, path))
			assertContractError(t, rec, http.StatusUnprocessableEntity, httpconst.ErrorCodeValidationFailed)
		}
	})

	t.Run("request syntax errors are 400", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		cases := []struct {
			name       string
			body       string
			pathValues map[string]string
		}{
			{name: "invalid local date path", body: occurrenceBody, pathValues: map[string]string{"circleId": contractCircleID, "scheduleId": contractScheduleID, "localDate": "yesterday"}},
			{name: "missing series version", body: `{"expected_occurrence_version":0,"cancelled":true,"confirm_overlaps":false}`, pathValues: path},
			{name: "missing occurrence version", body: `{"expected_series_version":1,"cancelled":true,"confirm_overlaps":false}`, pathValues: path},
			{name: "negative occurrence version", body: `{"expected_series_version":1,"expected_occurrence_version":-1,"cancelled":true,"confirm_overlaps":false}`, pathValues: path},
			{name: "missing confirm overlaps", body: `{"expected_series_version":1,"expected_occurrence_version":0,"cancelled":true}`, pathValues: path},
			{name: "invalid replacement date", body: `{"expected_series_version":1,"expected_occurrence_version":0,"replacement_local_date":"soon","confirm_overlaps":false}`, pathValues: path},
			{name: "invalid replacement clock", body: `{"expected_series_version":1,"expected_occurrence_version":0,"replacement_local_time":"9pm","confirm_overlaps":false}`, pathValues: path},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				rec := httptest.NewRecorder()
				handler.ChangeScheduleOccurrence(rec, scheduleContractRequest(t, http.MethodPatch, target, tc.body, tc.pathValues))
				assertContractError(t, rec, http.StatusBadRequest, httpconst.ErrorCodeValidationFailed)
			})
		}
	})

	t.Run("missing principal is unauthorized", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		req := scheduleContractRequest(t, http.MethodPatch, target, occurrenceBody, path).WithContext(context.Background())
		rec := httptest.NewRecorder()
		handler.ChangeScheduleOccurrence(rec, req)
		assertContractError(t, rec, http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized)
	})

	t.Run("per user rate limit returns 429", func(t *testing.T) {
		handler := NewScheduleHandler(&scheduleServiceStub{})
		limited := middleware.NewRateLimitMiddleware(0, 1).Limit(http.HandlerFunc(handler.ChangeScheduleOccurrence))
		first := httptest.NewRecorder()
		limited.ServeHTTP(first, scheduleContractRequest(t, http.MethodPatch, target, occurrenceBody, path))
		if first.Code != http.StatusOK {
			t.Fatalf("first request: got %d body=%s", first.Code, first.Body.String())
		}
		second := httptest.NewRecorder()
		limited.ServeHTTP(second, scheduleContractRequest(t, http.MethodPatch, target, occurrenceBody, path))
		assertContractError(t, second, http.StatusTooManyRequests, httpconst.ErrorCodeRateLimitExceeded)
	})
}

func ptrString(v string) *string { return &v }
