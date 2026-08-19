//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

func TestLiveSessionsDiscoveryUsesCanonicalCircleOperation(t *testing.T) {
	store, _, roles, service := newSessionFixture()
	roles.roles[scCircleID][scTeacher] = "teacher"
	created, err := service.CreateAdHocSession(context.Background(), scTeacher, scCircleID)
	if err != nil {
		t.Fatalf("create scheduled session: %v", err)
	}
	if _, _, err := service.StartSession(context.Background(), scTeacher, created.ID); err != nil {
		t.Fatalf("start active session: %v", err)
	}
	ended, err := store.CreateAdHocSession(context.Background(), scCircleID, scTeacher)
	if err != nil {
		t.Fatalf("create ended fixture: %v", err)
	}
	store.sessions[ended.ID].Status = sessions.SessionStatusEnded

	handler := sessions.NewHandler(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/circles/"+scCircleID+"/sessions", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: scTeacher}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("discovery status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode discovery response: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0]["status"] != string(sessions.SessionStatusActive) {
		t.Fatalf("discovery data = %v, want only the active canonical session", payload.Data)
	}
}
