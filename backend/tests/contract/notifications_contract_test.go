//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/google/uuid"
)

// notificationContractStub is the HistoryStore seam with canned data; the
// contract gate verifies the wire shape, not persistence.
type notificationContractStub struct {
	page        notifications.Page
	unread      int64
	markReadErr error
}

func (s *notificationContractStub) List(_ context.Context, _ string, _ *notifications.Cursor, _ int) (notifications.Page, error) {
	return s.page, nil
}

func (s *notificationContractStub) MarkRead(_ context.Context, _ string, _ uuid.UUID) (int64, error) {
	return s.unread, s.markReadErr
}

func (s *notificationContractStub) MarkAllRead(_ context.Context, _ string) (int64, error) {
	return s.unread, nil
}

func buildNotificationContractRoutes(store notifications.HistoryStore) (get, markRead, markAll http.Handler) {
	handler := notifications.NewHandler(store, nil)
	verifier := &alwaysOKVerifier{}
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	sessionService := auth.NewSessionService(30 * 24 * time.Hour)
	authMW := middleware.NewAuthMiddleware(verifier, sessionService, repo)
	return authMW.Require(http.HandlerFunc(handler.List)),
		authMW.Require(http.HandlerFunc(handler.MarkRead)),
		authMW.Require(http.HandlerFunc(handler.MarkAllRead))
}

func TestNotificationsContract_RequiresBearerAndSession(t *testing.T) {
	t.Parallel()

	get, markRead, markAll := buildNotificationContractRoutes(&notificationContractStub{
		page: notifications.Page{Items: []notifications.Item{}},
	})

	cases := []struct {
		name       string
		handler    http.Handler
		method     string
		target     string
		authHeader string
		sessionID  string
		wantCode   string
	}{
		{"GET missing bearer", get, http.MethodGet, "/api/v1/notifications", "", testSessionID, httpconst.ErrorCodeUnauthorized},
		{"GET missing session", get, http.MethodGet, "/api/v1/notifications", bearerValid, "", httpconst.ErrorCodeSessionMissing},
		{"PUT read missing bearer", markRead, http.MethodPut, "/api/v1/notifications/" + uuid.New().String() + "/read", "", testSessionID, httpconst.ErrorCodeUnauthorized},
		{"PUT read missing session", markRead, http.MethodPut, "/api/v1/notifications/" + uuid.New().String() + "/read", bearerValid, "", httpconst.ErrorCodeSessionMissing},
		{"PUT read-all missing bearer", markAll, http.MethodPut, "/api/v1/notifications/read-all", "", testSessionID, httpconst.ErrorCodeUnauthorized},
		{"PUT read-all missing session", markAll, http.MethodPut, "/api/v1/notifications/read-all", bearerValid, "", httpconst.ErrorCodeSessionMissing},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.target, nil)
			if tc.authHeader != "" {
				req.Header.Set(httpconst.HeaderAuthorization, tc.authHeader)
			}
			if tc.sessionID != "" {
				req.Header.Set(httpconst.HeaderSessionID, tc.sessionID)
			}
			rec := httptest.NewRecorder()
			tc.handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d, want 401 body=%s", rec.Code, rec.Body.String())
			}
			var envelope phttp.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if envelope.Error.Code != tc.wantCode {
				t.Fatalf("error code: got %q want %q", envelope.Error.Code, tc.wantCode)
			}
		})
	}
}

func TestNotificationsContract_HistoryPageShape(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	readAt := createdAt.Add(time.Hour)
	itemID := uuid.New()
	store := &notificationContractStub{
		page: notifications.Page{
			Items: []notifications.Item{{
				ID:        itemID,
				Type:      notifications.EventCircleInvitation,
				SourceRef: notifications.SourceRef{Type: notifications.SourceInvitation, ID: uuid.New()},
				Title:     "Circle invitation",
				Body:      "You have a new invitation",
				CreatedAt: createdAt,
				IsRead:    true,
				ReadAt:    &readAt,
			}},
			UnreadCount: 2,
			NextCursor:  &notifications.Cursor{CreatedAt: createdAt, ID: itemID},
		},
	}
	get, _, _ := buildNotificationContractRoutes(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications?limit=20", nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()
	get.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 body=%s", rec.Code, rec.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode history page: %v", err)
	}
	if _, present := body["items"]; !present {
		t.Fatal("history page missing required items key")
	}
	if _, present := body["next_cursor"]; !present {
		t.Fatal("history page missing required next_cursor key")
	}
	if body["unread_count"] != float64(2) {
		t.Fatalf("unread_count=%v, want 2", body["unread_count"])
	}
	if _, ok := body["next_cursor"].(string); !ok {
		t.Fatalf("next_cursor=%v, want opaque string when more pages remain", body["next_cursor"])
	}

	items := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items=%v, want one entry", body["items"])
	}
	item := items[0].(map[string]any)
	wantKeys := map[string]bool{"id": true, "type": true, "title": true, "body": true, "source_ref": true, "created_at": true, "is_read": true}
	for key := range wantKeys {
		if _, present := item[key]; !present {
			t.Fatalf("notification item missing required key %q", key)
		}
	}
	for key := range item {
		if !wantKeys[key] {
			t.Fatalf("notification item exposes unexpected key %q; read_at, tokens, and internals must stay hidden", key)
		}
	}
	sourceRef, ok := item["source_ref"].(map[string]any)
	if !ok {
		t.Fatalf("source_ref=%v, want object", item["source_ref"])
	}
	if _, present := sourceRef["type"]; !present {
		t.Fatal("source_ref missing type")
	}
	if _, present := sourceRef["id"]; !present {
		t.Fatal("source_ref missing id")
	}
}

func TestNotificationsContract_HistoryQueryValidation(t *testing.T) {
	t.Parallel()

	get, _, _ := buildNotificationContractRoutes(&notificationContractStub{
		page: notifications.Page{Items: []notifications.Item{}},
	})

	for _, target := range []string{
		"/api/v1/notifications?cursor=not-a-valid-cursor",
		"/api/v1/notifications?limit=0",
		"/api/v1/notifications?limit=not-an-int",
		"/api/v1/notifications?limit=51",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
		req.Header.Set(httpconst.HeaderSessionID, testSessionID)
		rec := httptest.NewRecorder()
		get.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status got %d, want 400 body=%s", target, rec.Code, rec.Body.String())
		}
		var envelope phttp.ErrorEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode error envelope: %v", err)
		}
		if envelope.Error.Code != httpconst.ErrorCodeValidationFailed {
			t.Fatalf("%s: error code got %q, want %q", target, envelope.Error.Code, httpconst.ErrorCodeValidationFailed)
		}
	}
}

func TestNotificationsContract_ReadStateResponses(t *testing.T) {
	t.Parallel()

	notificationID := uuid.New()
	store := &notificationContractStub{unread: 4}
	_, markRead, _ := buildNotificationContractRoutes(store)

	// Single read returns the authoritative unread count.
	req := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/"+notificationID.String()+"/read", nil)
	req.SetPathValue("notificationId", notificationID.String())
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()
	markRead.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mark read status: got %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	var countBody map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &countBody); err != nil {
		t.Fatalf("decode unread count: %v", err)
	}
	if countBody["unread_count"] != float64(4) || len(countBody) != 1 {
		t.Fatalf("mark read body=%v, want exactly {unread_count: 4}", countBody)
	}

	// Unknown and foreign identifiers share one 404 (no disclosure); the
	// contract defines no 400 for this operation.
	store.markReadErr = notifications.ErrNotificationNotFound
	for _, pathValue := range []string{uuid.New().String(), "not-a-uuid"} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/notifications/"+pathValue+"/read", nil)
		req.SetPathValue("notificationId", pathValue)
		req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
		req.Header.Set(httpconst.HeaderSessionID, testSessionID)
		rec := httptest.NewRecorder()
		markRead.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("mark read %q: status got %d, want 404 body=%s", pathValue, rec.Code, rec.Body.String())
		}
		var envelope phttp.ErrorEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatalf("decode error envelope: %v", err)
		}
		if envelope.Error.Code != httpconst.ErrorCodeNotFound {
			t.Fatalf("mark read %q: error code got %q, want %q", pathValue, envelope.Error.Code, httpconst.ErrorCodeNotFound)
		}
	}

	// Bulk read-all returns the authoritative zero and only that field.
	bulkStore := &notificationContractStub{unread: 0}
	_, _, bulkMarkAll := buildNotificationContractRoutes(bulkStore)
	req = httptest.NewRequest(http.MethodPut, "/api/v1/notifications/read-all", nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec = httptest.NewRecorder()
	bulkMarkAll.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("read-all status: got %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	countBody = map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &countBody); err != nil {
		t.Fatalf("decode read-all body: %v", err)
	}
	if countBody["unread_count"] != float64(0) || len(countBody) != 1 {
		t.Fatalf("read-all body=%v, want exactly {unread_count: 0}", countBody)
	}
}
