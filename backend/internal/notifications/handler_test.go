package notifications_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/google/uuid"
)

var handlerUserID = uuid.New().String()

type historyStoreStub struct {
	page          notifications.Page
	unread        int64
	listErr       error
	markReadErr   error
	markAllErr    error
	gotUserID     string
	gotBefore     *notifications.Cursor
	gotLimit      int
	gotMarkReadID uuid.UUID
}

func (s *historyStoreStub) List(_ context.Context, userID string, before *notifications.Cursor, limit int) (notifications.Page, error) {
	s.gotUserID = userID
	s.gotBefore = before
	s.gotLimit = limit
	return s.page, s.listErr
}

func (s *historyStoreStub) MarkRead(_ context.Context, userID string, notificationID uuid.UUID) (int64, error) {
	s.gotUserID = userID
	s.gotMarkReadID = notificationID
	return s.unread, s.markReadErr
}

func (s *historyStoreStub) MarkAllRead(_ context.Context, userID string) (int64, error) {
	s.gotUserID = userID
	return s.unread, s.markAllErr
}

func authenticatedRequest(t *testing.T, method, target string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: handlerUserID}))
}

func decodeErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder) phttp.ErrorEnvelope {
	t.Helper()
	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v body=%s", err, rec.Body.String())
	}
	return envelope
}

func TestHandlerListValidatesQueryParameters(t *testing.T) {
	store := &historyStoreStub{}
	handler := notifications.NewHandler(store, nil)

	cases := []struct {
		name   string
		target string
	}{
		{"malformed cursor", "/api/v1/notifications?cursor=not-a-cursor"},
		{"cursor with wrong parts", "/api/v1/notifications?cursor=" + "aGVsbG98d29ybGR8ZXh0cmE"},
		{"non-integer limit", "/api/v1/notifications?limit=abc"},
		{"zero limit", "/api/v1/notifications?limit=0"},
		{"negative limit", "/api/v1/notifications?limit=-3"},
		{"limit above maximum", "/api/v1/notifications?limit=51"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.List(rec, authenticatedRequest(t, http.MethodGet, tc.target))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s, want 400", rec.Code, rec.Body.String())
			}
			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != httpconst.ErrorCodeValidationFailed {
				t.Fatalf("code=%q, want %q", envelope.Error.Code, httpconst.ErrorCodeValidationFailed)
			}
		})
	}
}

func TestHandlerListReturnsContractShapeAndRoundTripsCursor(t *testing.T) {
	readAt := time.Now().UTC()
	cursorTime := time.Date(2025, 1, 2, 3, 4, 5, 123456789, time.UTC)
	cursorID := uuid.New()
	itemID := uuid.New()
	store := &historyStoreStub{
		page: notifications.Page{
			Items: []notifications.Item{{
				ID:        itemID,
				Type:      notifications.EventGroupMessage,
				SourceRef: notifications.SourceRef{Type: notifications.SourceMessage, ID: uuid.New()},
				Title:     "New message",
				Body:      "Open the app to view it",
				CreatedAt: cursorTime.Add(time.Hour),
				IsRead:    true,
				ReadAt:    &readAt,
			}},
			UnreadCount: 3,
			NextCursor:  &notifications.Cursor{CreatedAt: cursorTime, ID: cursorID},
		},
	}
	handler := notifications.NewHandler(store, nil)

	rec := httptest.NewRecorder()
	handler.List(rec, authenticatedRequest(t, http.MethodGet, "/api/v1/notifications?limit=50"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	if store.gotUserID != handlerUserID || store.gotLimit != 50 {
		t.Fatalf("List got user=%q limit=%d, want user=%q limit=50", store.gotUserID, store.gotLimit, handlerUserID)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode list body: %v", err)
	}
	if body["unread_count"] != float64(3) {
		t.Fatalf("unread_count=%v, want 3", body["unread_count"])
	}
	rawCursor, ok := body["next_cursor"].(string)
	if !ok || rawCursor == "" {
		t.Fatalf("next_cursor=%v, want opaque string", body["next_cursor"])
	}
	items, ok := body["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("items=%v, want one entry", body["items"])
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("item type %T, want object", items[0])
	}
	wantKeys := map[string]bool{"id": true, "type": true, "title": true, "body": true, "source_ref": true, "created_at": true, "is_read": true}
	for key := range item {
		if !wantKeys[key] {
			t.Fatalf("item exposes unexpected key %q (read_at and other internals must stay hidden)", key)
		}
	}
	for key := range wantKeys {
		if _, present := item[key]; !present {
			t.Fatalf("item missing required key %q", key)
		}
	}
	if item["id"] != itemID.String() || item["type"] != string(notifications.EventGroupMessage) {
		t.Fatalf("item id/type = %v/%v", item["id"], item["type"])
	}

	// The opaque cursor must round-trip back into the same stable position.
	followUp := &historyStoreStub{}
	followUpHandler := notifications.NewHandler(followUp, nil)
	followRec := httptest.NewRecorder()
	followUpHandler.List(followRec, authenticatedRequest(t, http.MethodGet, "/api/v1/notifications?cursor="+rawCursor))
	if followRec.Code != http.StatusOK {
		t.Fatalf("follow-up status=%d body=%s, want 200", followRec.Code, followRec.Body.String())
	}
	if followUp.gotBefore == nil || !followUp.gotBefore.CreatedAt.Equal(cursorTime) || followUp.gotBefore.ID != cursorID {
		t.Fatalf("round-tripped cursor = %+v, want %v/%s", followUp.gotBefore, cursorTime, cursorID)
	}
}

func TestHandlerListEmptyPageSerializesEmptyArrayAndNullCursor(t *testing.T) {
	handler := notifications.NewHandler(&historyStoreStub{page: notifications.Page{Items: []notifications.Item{}}}, nil)
	rec := httptest.NewRecorder()
	handler.List(rec, authenticatedRequest(t, http.MethodGet, "/api/v1/notifications"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if string(body["items"]) != "[]" {
		t.Fatalf("items=%s, want []", body["items"])
	}
	if string(body["next_cursor"]) != "null" {
		t.Fatalf("next_cursor=%s, want null key present", body["next_cursor"])
	}
}

func TestHandlerRequiresAuthenticatedPrincipal(t *testing.T) {
	handler := notifications.NewHandler(&historyStoreStub{}, nil)
	notificationID := uuid.New()

	cases := []struct {
		name    string
		handle  http.HandlerFunc
		prepare func(*http.Request)
	}{
		{"list", handler.List, func(*http.Request) {}},
		{"mark read", handler.MarkRead, func(r *http.Request) { r.SetPathValue("notificationId", notificationID.String()) }},
		{"mark all read", handler.MarkAllRead, func(*http.Request) {}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
			tc.prepare(req)
			rec := httptest.NewRecorder()
			tc.handle(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s, want 401", rec.Code, rec.Body.String())
			}
			if envelope := decodeErrorEnvelope(t, rec); envelope.Error.Code != httpconst.ErrorCodeUnauthorized {
				t.Fatalf("code=%q, want %q", envelope.Error.Code, httpconst.ErrorCodeUnauthorized)
			}
		})
	}
}

func TestHandlerMarkReadHidesUnknownForeignAndInvalidIdentifiers(t *testing.T) {
	notificationID := uuid.New()

	cases := []struct {
		name      string
		pathValue string
		storeErr  error
	}{
		{"invalid uuid is not a 400", "not-a-uuid", nil},
		{"unknown notification", notificationID.String(), notifications.ErrNotificationNotFound},
		{"foreign notification", notificationID.String(), notifications.ErrNotificationNotFound},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			sender := &fakeUserEventSender{}
			handler := notifications.NewHandler(&historyStoreStub{markReadErr: tc.storeErr}, notifications.NewNotifier(sender))
			req := authenticatedRequest(t, http.MethodPut, "/api/v1/notifications/"+tc.pathValue+"/read")
			req.SetPathValue("notificationId", tc.pathValue)
			rec := httptest.NewRecorder()
			handler.MarkRead(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status=%d body=%s, want 404", rec.Code, rec.Body.String())
			}
			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != httpconst.ErrorCodeNotFound {
				t.Fatalf("code=%q, want %q", envelope.Error.Code, httpconst.ErrorCodeNotFound)
			}
			if sender.calls != 0 {
				t.Fatalf("notifier called %d times on 404, want 0", sender.calls)
			}
		})
	}
}

func TestHandlerMarkReadReturnsCountAndEmitsPostCommitHint(t *testing.T) {
	notificationID := uuid.New()
	sender := &fakeUserEventSender{}
	store := &historyStoreStub{unread: 5}
	handler := notifications.NewHandler(store, notifications.NewNotifier(sender))

	req := authenticatedRequest(t, http.MethodPut, "/api/v1/notifications/"+notificationID.String()+"/read")
	req.SetPathValue("notificationId", notificationID.String())
	rec := httptest.NewRecorder()
	handler.MarkRead(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var body struct {
		UnreadCount int64 `json:"unread_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.UnreadCount != 5 {
		t.Fatalf("unread_count=%d, want 5", body.UnreadCount)
	}
	if store.gotMarkReadID != notificationID || store.gotUserID != handlerUserID {
		t.Fatalf("MarkRead got id=%s user=%q", store.gotMarkReadID, store.gotUserID)
	}
	if sender.calls != 1 {
		t.Fatalf("notifier calls=%d, want 1", sender.calls)
	}
	payload := sender.delivery.Payload.(map[string]any)["payload"].(map[string]any)
	if payload["notification_id"] != notificationID.String() || payload["unread_count"] != int64(5) {
		t.Fatalf("hint payload=%v, want notification id and post-commit count", payload)
	}
}

func TestHandlerMarkReadHintFailureStillSucceeds(t *testing.T) {
	notificationID := uuid.New()
	sender := &fakeUserEventSender{err: errors.New("hub down")}
	handler := notifications.NewHandler(&historyStoreStub{unread: 1}, notifications.NewNotifier(sender))

	req := authenticatedRequest(t, http.MethodPut, "/api/v1/notifications/"+notificationID.String()+"/read")
	req.SetPathValue("notificationId", notificationID.String())
	rec := httptest.NewRecorder()
	handler.MarkRead(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200 even when the hint fails (REST is authoritative)", rec.Code, rec.Body.String())
	}
}

func TestHandlerMarkAllReadReturnsCountAndEmitsBulkHint(t *testing.T) {
	sender := &fakeUserEventSender{}
	store := &historyStoreStub{unread: 0}
	handler := notifications.NewHandler(store, notifications.NewNotifier(sender))

	rec := httptest.NewRecorder()
	handler.MarkAllRead(rec, authenticatedRequest(t, http.MethodPut, "/api/v1/notifications/read-all"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", rec.Code, rec.Body.String())
	}
	var body struct {
		UnreadCount int64 `json:"unread_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.UnreadCount != 0 || store.gotUserID != handlerUserID {
		t.Fatalf("unread=%d user=%q", body.UnreadCount, store.gotUserID)
	}
	if sender.calls != 1 {
		t.Fatalf("notifier calls=%d, want 1", sender.calls)
	}
	payload := sender.delivery.Payload.(map[string]any)["payload"].(map[string]any)
	if _, present := payload["notification_id"]; present {
		t.Fatalf("bulk hint %v must omit notification_id", payload)
	}
}
