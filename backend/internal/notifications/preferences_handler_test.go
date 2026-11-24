package notifications_test

import (
	"context"
	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type preferencesStoreStub struct {
	historyStoreStub
	result  notifications.Preferences
	updates []notifications.TypeSettingUpdate
	quiet   *notifications.QuietHoursUpdate
}

func (s *preferencesStoreStub) GetPreferences(_ context.Context, userID string) (notifications.Preferences, error) {
	s.gotUserID = userID
	return s.result, nil
}
func (s *preferencesStoreStub) UpdatePreferences(_ context.Context, userID string, updates []notifications.TypeSettingUpdate, quiet *notifications.QuietHoursUpdate) error {
	s.gotUserID = userID
	s.updates = updates
	s.quiet = quiet
	return nil
}

type preferencesHandler interface {
	GetPreferences(http.ResponseWriter, *http.Request)
	UpdatePreferences(http.ResponseWriter, *http.Request)
}

func TestPreferencesEndpoints(t *testing.T) {
	store := &preferencesStoreStub{result: notifications.Preferences{Types: []notifications.TypeSetting{{Type: notifications.EventGroupMessage, Enabled: true, Configurable: true, DefaultEnabled: true}}, QuietHours: notifications.QuietHours{Enabled: true, StartLocal: "22:00", EndLocal: "07:00", Timezone: "Asia/Riyadh"}}}
	handler, ok := any(notifications.NewHandler(store, nil)).(preferencesHandler)
	if !ok {
		t.Fatal("preferences endpoints are not implemented")
	}
	rec := httptest.NewRecorder()
	handler.GetPreferences(rec, authenticatedRequest(t, http.MethodGet, "/api/v1/notifications/preferences"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"timezone":"Asia/Riyadh"`) || store.gotUserID != handlerUserID {
		t.Fatalf("GET status=%d body=%s user=%s", rec.Code, rec.Body.String(), store.gotUserID)
	}
	body := `{"types":[{"type":"group_message","enabled":false}],"quiet_hours":{"enabled":true,"start_local":"23:00","end_local":"06:00"}}`
	req := authenticatedRequest(t, http.MethodPut, "/api/v1/notifications/preferences")
	req.Body = io.NopCloser(strings.NewReader(body))
	rec = httptest.NewRecorder()
	handler.UpdatePreferences(rec, req)
	if rec.Code != 200 || len(store.updates) != 1 || store.updates[0].Enabled || store.quiet == nil || store.quiet.StartLocal != "23:00" {
		t.Fatalf("PUT status=%d body=%s updates=%v quiet=%v", rec.Code, rec.Body.String(), store.updates, store.quiet)
	}
}

func TestPreferencesRejectInvalidUpdates(t *testing.T) {
	handler := any(notifications.NewHandler(&preferencesStoreStub{}, nil)).(preferencesHandler)
	for _, body := range []string{``, `null`, `{"types":[{"type":"circle_invitation","enabled":false}]}`, `{"types":[{"type":"unknown","enabled":true}]}`, `{"types":[{"type":"group_message"}]}`, `{"quiet_hours":{"enabled":true,"start_local":"24:00","end_local":"07:00"}}`, `{"quiet_hours":{"enabled":true,"start_local":"22:00","end_local":"22:00"}}`, `{"quiet_hours":{"enabled":true,"start_local":"22:00","end_local":"07:00","timezone":"UTC"}}`} {
		t.Run(body, func(t *testing.T) {
			req := authenticatedRequest(t, http.MethodPut, "/api/v1/notifications/preferences")
			req.Body = io.NopCloser(strings.NewReader(body))
			rec := httptest.NewRecorder()
			handler.UpdatePreferences(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			decodeErrorEnvelope(t, rec)
		})
	}
}
