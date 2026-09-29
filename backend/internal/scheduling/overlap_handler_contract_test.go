//go:build contract

package scheduling

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
)

type overlapPreviewStub struct {
	warnings WarningResult
	err      error
	called   bool
}

func (s *overlapPreviewStub) Preview(_ context.Context, _, _ string, _ RevisionRecord) (WarningResult, error) {
	s.called = true
	return s.warnings, s.err
}

func TestOverlapPreviewReturnsActionablePrivacySafeWarning(t *testing.T) {
	start := time.Date(2030, 1, 2, 10, 0, 0, 0, time.UTC)
	service := &overlapPreviewStub{warnings: WarningResult{Warnings: []OverlapWarning{{
		WarningID: "review-id", FirstOccurrenceKey: "planned-a", SecondOccurrenceKey: "planned-b",
		FirstCircleName: "A", SecondCircleName: "B", OverlapStartsAt: start.Format(time.RFC3339), OverlapEndsAt: start.Add(time.Hour).Format(time.RFC3339),
	}}}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/circles/"+contractCircleID+"/planning-preview", strings.NewReader(contractPlanBody()))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("circleId", contractCircleID)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: contractActorID}))
	rec := httptest.NewRecorder()
	NewOverlapHandler(service).Preview(rec, req)
	if rec.Code != http.StatusOK || !service.called {
		t.Fatalf("status=%d called=%v body=%s", rec.Code, service.called, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "user_id") || !strings.Contains(rec.Body.String(), "review-id") || !strings.Contains(rec.Body.String(), "overlap_starts_at") {
		t.Fatalf("warning projection missing actionable fields or disclosed member data: %s", rec.Body.String())
	}
}

func TestOverlapPreviewMapsRoleDenialWithoutPrivateDetails(t *testing.T) {
	service := &overlapPreviewStub{err: ErrInsufficientCircleRole}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(contractPlanBody()))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("circleId", contractCircleID)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: contractActorID}))
	rec := httptest.NewRecorder()
	NewOverlapHandler(service).Preview(rec, req)
	if rec.Code != http.StatusForbidden || strings.Contains(rec.Body.String(), contractActorID) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestScheduleCreateEchoesReviewedWarningIDs(t *testing.T) {
	body := strings.TrimSuffix(contractPlanBody(), "\n\t}") + `,
		"confirm_overlaps": true,
		"confirmed_warning_ids": ["reviewed-1"]
	}`
	var got CreateScheduleCommand
	stub := &scheduleServiceStub{createFn: func(_ context.Context, cmd CreateScheduleCommand) (Schedule, error) {
		got = cmd
		return Schedule{ID: contractScheduleID, CircleID: contractCircleID, CurrentVersion: 1}, nil
	}}
	req := scheduleContractRequest(t, http.MethodPost, "/api/v1/circles/"+contractCircleID+"/schedules", body, map[string]string{"circleId": contractCircleID})
	rec := httptest.NewRecorder()
	NewScheduleHandler(stub).CreateSchedule(rec, req)
	if rec.Code != http.StatusCreated || !got.ConfirmOverlaps || len(got.ConfirmedWarningIDs) != 1 || got.ConfirmedWarningIDs[0] != "reviewed-1" {
		t.Fatalf("status=%d command=%#v body=%s", rec.Code, got, rec.Body.String())
	}
}

func TestWriteConflictReturnsRefreshedWarningIDs(t *testing.T) {
	warning := OverlapWarning{WarningID: "fresh-1", FirstOccurrenceKey: "a", SecondOccurrenceKey: "b", FirstCircleName: "A", SecondCircleName: "B", OverlapStartsAt: "2030-01-02T10:00:00Z", OverlapEndsAt: "2030-01-02T11:00:00Z"}
	stub := &scheduleServiceStub{createFn: func(context.Context, CreateScheduleCommand) (Schedule, error) {
		return Schedule{}, &OverlapConfirmationError{Warnings: []OverlapWarning{warning}}
	}}
	body := strings.TrimSuffix(contractPlanBody(), "\n\t}") + `,
		"confirm_overlaps": true,
		"confirmed_warning_ids": ["stale-id"]
	}`
	req := scheduleContractRequest(t, http.MethodPost, "/api/v1/circles/"+contractCircleID+"/schedules", body, map[string]string{"circleId": contractCircleID})
	rec := httptest.NewRecorder()
	NewScheduleHandler(stub).CreateSchedule(rec, req)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "fresh-1") || !strings.Contains(rec.Body.String(), "overlap_starts_at") || strings.Contains(rec.Body.String(), "user_id") {
		t.Fatalf("refreshed warning conflict was not safe/actionable: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOneOffWriteEchoesReviewedWarningIDsAndReturnsRefreshedConflict(t *testing.T) {
	body := `{"mode":"one_off","title":"Review","anchor_local_date":"2030-01-06","local_start_time":"22:30","local_end_time":"23:00","duration_minutes":30,"timezone":"Asia/Riyadh","confirm_overlaps":true,"confirmed_warning_ids":["reviewed-one-off"]}`
	var got CreatePlannedSessionCommand
	planned := &plannedSessionCommandsStub{createFn: func(_ context.Context, cmd CreatePlannedSessionCommand) (PlannedSessionView, error) {
		got = cmd
		return PlannedSessionView{}, &OverlapConfirmationError{Warnings: []OverlapWarning{{
			WarningID: "fresh-one-off", FirstOccurrenceKey: "planned-a", SecondOccurrenceKey: "planned-b",
			FirstCircleName: "A", SecondCircleName: "B", OverlapStartsAt: "2030-01-06T19:45:00Z", OverlapEndsAt: "2030-01-06T20:00:00Z",
		}}}
	}}
	req := calendarContractRequest(t, http.MethodPost, "/api/v1/circles/"+calendarContractCircleID+"/planned-sessions", body, map[string]string{"circleId": calendarContractCircleID}, true)
	rec := httptest.NewRecorder()
	newCalendarContractHandler(planned, &occurrenceStartCommandsStub{}, &calendarMonthCommandsStub{}).CreateOneOff(rec, req)
	if !got.ConfirmOverlaps || len(got.ConfirmedWarningIDs) != 1 || got.ConfirmedWarningIDs[0] != "reviewed-one-off" {
		t.Fatalf("review confirmation was not forwarded: %#v", got)
	}
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "fresh-one-off") || !strings.Contains(rec.Body.String(), "overlap_ends_at") || strings.Contains(rec.Body.String(), "user_id") {
		t.Fatalf("one-off conflict missing safe refreshed warning: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNonOverlappingWriteDoesNotRequireConfirmation(t *testing.T) {
	var got CreateScheduleCommand
	stub := &scheduleServiceStub{createFn: func(_ context.Context, cmd CreateScheduleCommand) (Schedule, error) {
		got = cmd
		return Schedule{ID: contractScheduleID, CircleID: contractCircleID, CurrentVersion: 1}, nil
	}}
	req := scheduleContractRequest(t, http.MethodPost, "/api/v1/circles/"+contractCircleID+"/schedules", contractPlanBody(), map[string]string{"circleId": contractCircleID})
	rec := httptest.NewRecorder()
	NewScheduleHandler(stub).CreateSchedule(rec, req)
	if rec.Code != http.StatusCreated || got.ConfirmOverlaps || len(got.ConfirmedWarningIDs) != 0 {
		t.Fatalf("non-overlap write required confirmation: status=%d command=%#v body=%s", rec.Code, got, rec.Body.String())
	}
}
