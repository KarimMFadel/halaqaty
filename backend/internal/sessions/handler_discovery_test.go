package sessions

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
)

func TestHandler_ListSessions_InvalidCircleID_RejectedWithFieldError(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := doAs(handler, us1Student, http.MethodGet, "/circles/"+hBadUUID+"/sessions")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("list with invalid circle id status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	body := decodeMap(t, rec)
	errObj, _ := body["error"].(map[string]any)
	fields, _ := errObj["fields"].(map[string]any)
	if fields["circle_id"] == "" {
		t.Fatalf("validation error must name circle_id: %v", body)
	}
}

func TestHandler_ListSessions_Member_ReceivesDiscoveryData(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := doAs(handler, us1Student, http.MethodGet, "/circles/"+hTestCircleUUID+"/sessions")
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	body := decodeMap(t, rec)
	data, _ := body["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data = %v, want the seeded circle session", body)
	}
	item, _ := data[0].(map[string]any)
	if item["id"] != hTestSessionUUID || item["status"] != "scheduled" {
		t.Fatalf("session projection = %v, want seeded scheduled session", item)
	}
	if strings.Contains(rec.Body.String(), "media_room_ref") {
		t.Fatal("discovery response must never expose the media room reference")
	}
}

func TestHandler_ListSessions_NonMember_Forbidden(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := doAs(handler, "outsider-9", http.MethodGet, "/circles/"+hTestCircleUUID+"/sessions")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-member list status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatal("denials must use the standard error envelope")
	}
}

func TestHandler_ParticipantAction_InvalidTargetID_RejectedWithFieldError(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := doAs(handler, us1Teacher, http.MethodPost, "/sessions/"+hTestSessionUUID+"/participants/"+hBadUUID+"/mute")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid target id status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	body := decodeMap(t, rec)
	errObj, _ := body["error"].(map[string]any)
	fields, _ := errObj["fields"].(map[string]any)
	if fields["user_id"] == "" {
		t.Fatalf("validation error must name user_id: %v", body)
	}
}

func TestHandler_ParticipantAction_UnknownAction_NotFound(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := doAs(handler, us1Teacher, http.MethodPost, "/sessions/"+hTestSessionUUID+"/participants/"+hOtherUUID+"/kick")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown action status = %d, want 404 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestHandler_ParticipantAction_Student_Forbidden(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := doAs(handler, us1Student, http.MethodPost, "/sessions/"+hTestSessionUUID+"/participants/"+hOtherUUID+"/mute")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("student moderation status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestHandler_ParticipantAction_MalformedBody_Rejected(t *testing.T) {
	handler, _ := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/sessions/"+hTestSessionUUID+"/participants/"+hOtherUUID+"/mute", strings.NewReader("not-json"))
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: us1Teacher}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
}
