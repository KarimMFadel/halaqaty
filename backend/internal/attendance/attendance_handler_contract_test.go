package attendance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

type attendanceCommandStub struct {
	listErr    error
	correctErr error
	listed     []Record
	command    CorrectionCommand
}

func (s *attendanceCommandStub) List(context.Context, string, string) ([]Record, error) {
	return s.listed, s.listErr
}

func (s *attendanceCommandStub) Correct(_ context.Context, command CorrectionCommand) (Record, error) {
	s.command = command
	return Record{SessionID: command.SessionID, UserID: command.UserID, Status: command.Status, Source: "manual"}, s.correctErr
}

func TestAttendanceHandler_GetReturnsOnlyServiceProjection(t *testing.T) {
	stub := &attendanceCommandStub{listed: []Record{{SessionID: contractAttendanceSessionID, UserID: contractAttendanceUserID, Status: StatusPresent, Source: "automatic"}}}
	handler := NewHandler(stub)
	recorder := httptest.NewRecorder()
	handler.GetAttendance(recorder, contractAttendanceRequest(http.MethodGet, "/api/v1/sessions/"+contractAttendanceSessionID+"/attendance", ""))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"present"`) || !strings.Contains(recorder.Body.String(), `"source":"automatic"`) {
		t.Fatalf("GET status/body = %d %s", recorder.Code, recorder.Body.String())
	}
	unauthenticated := httptest.NewRecorder()
	handler.GetAttendance(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+contractAttendanceSessionID+"/attendance", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated GET status = %d", unauthenticated.Code)
	}
}

func TestAttendanceHandler_CorrectionContractValidationAndSafeErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		path string
		body string
		key  string
		err  error
		want int
	}{
		{name: "invalid session id", path: "/api/v1/sessions/not-a-uuid/attendance/" + contractAttendanceUserID, body: `{"status":"present","reason":"valid"}`, key: "retry-1", want: http.StatusBadRequest},
		{name: "missing idempotency key", path: "/api/v1/sessions/" + contractAttendanceSessionID + "/attendance/" + contractAttendanceUserID, body: `{"status":"present","reason":"valid"}`, want: http.StatusBadRequest},
		{name: "invalid JSON", path: "/api/v1/sessions/" + contractAttendanceSessionID + "/attendance/" + contractAttendanceUserID, body: `{`, key: "retry-1", want: http.StatusBadRequest},
		{name: "forbidden current role", path: "/api/v1/sessions/" + contractAttendanceSessionID + "/attendance/" + contractAttendanceUserID, body: `{"status":"present","reason":"valid"}`, key: "retry-1", err: ErrAttendanceForbidden, want: http.StatusForbidden},
		{name: "cross-circle or missing record is concealed", path: "/api/v1/sessions/" + contractAttendanceSessionID + "/attendance/" + contractAttendanceUserID, body: `{"status":"present","reason":"valid"}`, key: "retry-1", err: ErrAttendanceNotFound, want: http.StatusNotFound},
		{name: "duplicate key with changed body conflicts", path: "/api/v1/sessions/" + contractAttendanceSessionID + "/attendance/" + contractAttendanceUserID, body: `{"status":"present","reason":"valid"}`, key: "retry-1", err: ErrCorrectionReplay, want: http.StatusConflict},
		{name: "invalid correction is rejected", path: "/api/v1/sessions/" + contractAttendanceSessionID + "/attendance/" + contractAttendanceUserID, body: `{"status":"present","reason":"valid"}`, key: "retry-1", err: ErrInvalidCorrection, want: http.StatusUnprocessableEntity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := &attendanceCommandStub{correctErr: tc.err}
			handler := NewHandler(stub)
			req := contractAttendanceRequest(http.MethodPatch, tc.path, tc.body)
			if tc.key != "" {
				req.Header.Set(httpconst.HeaderIdempotencyKey, tc.key)
			}
			recorder := httptest.NewRecorder()
			handler.CorrectAttendance(recorder, req)
			if recorder.Code != tc.want {
				t.Fatalf("PATCH status/body = %d %s, want %d", recorder.Code, recorder.Body.String(), tc.want)
			}
			if tc.err != nil && strings.Contains(recorder.Body.String(), tc.err.Error()) {
				t.Fatalf("response leaked internal error: %s", recorder.Body.String())
			}
		})
	}
	stub := &attendanceCommandStub{}
	req := contractAttendanceRequest(http.MethodPatch, "/api/v1/sessions/"+contractAttendanceSessionID+"/attendance/"+contractAttendanceUserID, `{"status":"excused","reason":"Approved"}`)
	req.Header.Set(httpconst.HeaderIdempotencyKey, "retry-1")
	recorder := httptest.NewRecorder()
	NewHandler(stub).CorrectAttendance(recorder, req)
	if recorder.Code != http.StatusOK || stub.command.ActorID != contractAttendanceActorID || stub.command.Status != StatusExcused || stub.command.IdempotencyKey != "retry-1" {
		t.Fatalf("PATCH command/status = %+v/%d", stub.command, recorder.Code)
	}
}

const (
	contractAttendanceActorID   = "11111111-1111-4111-8111-111111111111"
	contractAttendanceSessionID = "22222222-2222-4222-8222-222222222222"
	contractAttendanceUserID    = "33333333-3333-4333-8333-333333333333"
)

func contractAttendanceRequest(method, path, body string) *http.Request {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range parts {
		if part == "sessions" && i+1 < len(parts) {
			req.SetPathValue("sessionId", parts[i+1])
		}
		if part == "attendance" && i+1 < len(parts) {
			req.SetPathValue("userId", parts[i+1])
		}
	}
	return req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: contractAttendanceActorID}))
}
