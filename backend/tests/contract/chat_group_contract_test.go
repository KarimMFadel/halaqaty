//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/api"
	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// T023 — behavioral REST contract tests for the F-004 US1 group list/send
// surface, pinned to specs/004-real-time-chat/contracts/chat.openapi.yaml
// (listCircleMessages + sendCircleMessage) and the canonical
// docs/contracts/openapi.yaml copies of those operations.
//
// The suite runs the production chat.GroupHandler through the production
// api.Router with the real auth middleware and the real chat send limiter
// over a deterministic service stub, so route registration, middleware
// order, status codes, error envelopes, non-enumerating denials, rate
// limits, and response safety are proven without PostgreSQL. The service
// behaviors behind the seam (membership-period history, single-row
// idempotent sends, and identical ErrCircleNotVisible for non-members and
// unknown circles) are proven against real PostgreSQL in
// backend/internal/chat/group_service_integration_test.go; this file pins
// only what the wire contract declares.

const (
	chatGroupCircleID      = "22222222-2222-2222-2222-222222222222"
	chatGroupOtherCircleID = "99999999-9999-9999-9999-999999999999"
	chatGroupTextBody      = `{"message_type":"text","content":"Salam, ready to recite"}`
)

// chatGroupHistoryCall records one History call reaching the service stub.
type chatGroupHistoryCall struct {
	viewerID uuid.UUID
	circleID uuid.UUID
	before   *uuid.UUID
	limit    int
}

// chatGroupSendCall records one SendText call reaching the service stub.
type chatGroupSendCall struct {
	senderID uuid.UUID
	circleID uuid.UUID
	content  string
	key      string
}

// chatGroupServiceStub is a deterministic chat.GroupChatService. SendText
// mirrors the durable idempotency contract: the first call commits a message
// and every later call returns that same committed message (FR-007).
type chatGroupServiceStub struct {
	history      []chat.Message
	historyErr   error
	historyCalls []chatGroupHistoryCall
	sendErr      error
	sendCalls    []chatGroupSendCall
	committed    chat.Message
	pinned       map[uuid.UUID]chat.Message
	pinErr       error
	unpinErr     error
}

func (s *chatGroupServiceStub) History(_ context.Context, viewerID, circleID uuid.UUID, before *uuid.UUID, limit int) ([]chat.Message, error) {
	s.historyCalls = append(s.historyCalls, chatGroupHistoryCall{viewerID: viewerID, circleID: circleID, before: before, limit: limit})
	if s.historyErr != nil {
		return nil, s.historyErr
	}
	return s.history, nil
}

func (s *chatGroupServiceStub) Search(_ context.Context, viewerID, circleID uuid.UUID, _ string, before *uuid.UUID, limit int) ([]chat.Message, error) {
	s.historyCalls = append(s.historyCalls, chatGroupHistoryCall{viewerID: viewerID, circleID: circleID, before: before, limit: limit})
	if s.historyErr != nil {
		return nil, s.historyErr
	}
	return s.history, nil
}

func (s *chatGroupServiceStub) SendText(_ context.Context, senderID, circleID uuid.UUID, content, key string) (chat.Message, error) {
	s.sendCalls = append(s.sendCalls, chatGroupSendCall{senderID: senderID, circleID: circleID, content: content, key: key})
	if s.sendErr != nil {
		return chat.Message{}, s.sendErr
	}
	if s.committed.ID == uuid.Nil {
		s.committed = chat.Message{
			ID:       uuid.New(),
			CircleID: &circleID,
			SenderID: senderID,
			Type:     chat.MessageTypeText,
			Content:  strings.TrimSpace(content),
			State:    chat.MessageStateActive,
			SentAt:   time.Now().UTC(),
		}
	}
	return s.committed, nil
}

func (s *chatGroupServiceStub) ListPinned(context.Context, uuid.UUID, uuid.UUID) ([]chat.Message, error) {
	messages := make([]chat.Message, 0, len(s.pinned))
	for _, message := range s.pinned {
		if message.PinnedAt != nil {
			messages = append(messages, message)
		}
	}
	return messages, nil
}

func (s *chatGroupServiceStub) Pin(_ context.Context, actorID, circleID, messageID uuid.UUID) (bool, error) {
	if s.pinErr != nil {
		return false, s.pinErr
	}
	if s.pinned == nil {
		s.pinned = make(map[uuid.UUID]chat.Message)
	}
	if message, ok := s.pinned[messageID]; ok && message.PinnedAt != nil {
		return false, nil
	}
	pinnedCount := 0
	for _, message := range s.pinned {
		if message.PinnedAt != nil {
			pinnedCount++
		}
	}
	if pinnedCount >= 5 {
		return false, chat.ErrPinLimit
	}
	circle := circleID
	actor := actorID
	now := time.Now().UTC()
	s.pinned[messageID] = chat.Message{ID: messageID, CircleID: &circle, SenderID: actorID, Type: chat.MessageTypeText, Content: "pinned", State: chat.MessageStateActive, SentAt: now, PinnedBy: &actor, PinnedAt: &now}
	return true, nil
}

func (s *chatGroupServiceStub) Unpin(_ context.Context, _ uuid.UUID, _ uuid.UUID, messageID uuid.UUID) (bool, error) {
	if s.unpinErr != nil {
		return false, s.unpinErr
	}
	message, ok := s.pinned[messageID]
	if !ok {
		return false, chat.ErrMessageNotVisible
	}
	if message.PinnedAt == nil {
		return false, nil
	}
	message.PinnedAt = nil
	message.PinnedBy = nil
	s.pinned[messageID] = message
	return true, nil
}

// chatGroupRouter wires the production router with the real auth middleware
// (alwaysOKVerifier + the shared session stub authenticate as
// testLocalUserID), the handler under test, and the optional real send
// limiter, so contract cases exercise the exact deployed middleware order.
func chatGroupRouter(service chat.GroupChatService, limiter *chat.ChatSendLimiter) http.Handler {
	authMW := middleware.NewAuthMiddleware(
		&alwaysOKVerifier{},
		auth.NewSessionService(30*24*time.Hour),
		&stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID},
	)
	return api.NewRouter(api.MiddlewareSet{
		Auth:            authMW,
		ChatHandler:     chat.NewGroupHandler(service),
		ChatSendLimiter: limiter,
	}).Handler()
}

// chatGroupRequest builds one authenticated group-chat request.
func chatGroupRequest(method, path, body, idempotencyKey string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	}
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	if idempotencyKey != "" {
		req.Header.Set(httpconst.HeaderIdempotencyKey, idempotencyKey)
	}
	return req
}

// chatGroupMessagesPath is the group messages path for one circle.
func chatGroupMessagesPath(circleID string) string {
	return "/api/v1/circles/" + circleID + "/messages"
}

// chatGroupHistoryFixture returns n newest-first text messages.
func chatGroupHistoryFixture(n int) []chat.Message {
	circle := uuid.MustParse(chatGroupCircleID)
	sender := uuid.MustParse(testLocalUserID)
	base := time.Now().UTC().Add(-time.Duration(n) * time.Minute)
	messages := make([]chat.Message, 0, n)
	for i := 0; i < n; i++ {
		messages = append(messages, chat.Message{
			ID:       uuid.New(),
			CircleID: &circle,
			SenderID: sender,
			Type:     chat.MessageTypeText,
			Content:  fmt.Sprintf("message %d", i+1),
			State:    chat.MessageStateActive,
			SentAt:   base.Add(time.Duration(n-1-i) * time.Minute),
		})
	}
	return messages
}

// chatMessageKeys is the exact response-safe Message projection key set:
// identifiers, content, server timestamps, and state only (SR-006).
var chatMessageKeys = []string{"id", "circle_id", "sender_id", "message_type", "content", "sent_at", "delivery_status"}

func assertJSONKeySet(t *testing.T, raw json.RawMessage, want []string) {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatalf("decode json object: %v body=%s", err, raw)
	}
	if len(object) != len(want) {
		t.Fatalf("key set: got %d keys (%v), want exactly %d (%v)", len(object), object, len(want), want)
	}
	for _, key := range want {
		if _, ok := object[key]; !ok {
			t.Fatalf("missing key %q in %s", key, raw)
		}
	}
}

func decodeErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder) phttp.ErrorEnvelope {
	t.Helper()
	var envelope phttp.ErrorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
		t.Fatalf("decode error envelope: %v body=%s", err, rec.Body.String())
	}
	return envelope
}

func TestChatGroupListContract(t *testing.T) {
	t.Parallel()

	t.Run("newest page returns the safe Message projection", func(t *testing.T) {
		t.Parallel()
		stub := &chatGroupServiceStub{history: chatGroupHistoryFixture(3)}
		rec := httptest.NewRecorder()
		chatGroupRouter(stub, nil).ServeHTTP(rec, chatGroupRequest(http.MethodGet, chatGroupMessagesPath(chatGroupCircleID), "", ""))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var page struct {
			Data       []json.RawMessage `json:"data"`
			HasMore    bool              `json:"has_more"`
			NextBefore any               `json:"next_before"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
			t.Fatalf("decode page: %v", err)
		}
		if len(page.Data) != 3 {
			t.Fatalf("data length: got %d, want 3", len(page.Data))
		}
		if page.HasMore {
			t.Fatal("has_more: got true, want false for a short page")
		}
		if page.NextBefore != nil {
			t.Fatalf("next_before: got %v, want null for a short page", page.NextBefore)
		}
		var previous time.Time
		for i, raw := range page.Data {
			assertJSONKeySet(t, raw, chatMessageKeys)
			var message struct {
				ID             string `json:"id"`
				CircleID       string `json:"circle_id"`
				SenderID       string `json:"sender_id"`
				MessageType    string `json:"message_type"`
				Content        string `json:"content"`
				SentAt         string `json:"sent_at"`
				DeliveryStatus string `json:"delivery_status"`
			}
			if err := json.Unmarshal(raw, &message); err != nil {
				t.Fatalf("decode message %d: %v", i, err)
			}
			if _, err := uuid.Parse(message.ID); err != nil {
				t.Fatalf("message %d id: %v", i, err)
			}
			if message.CircleID != chatGroupCircleID || message.SenderID != testLocalUserID {
				t.Fatalf("message %d identifiers: circle=%q sender=%q", i, message.CircleID, message.SenderID)
			}
			if message.MessageType != string(chat.MessageTypeText) || message.Content == "" {
				t.Fatalf("message %d payload: type=%q content=%q", i, message.MessageType, message.Content)
			}
			if message.DeliveryStatus != string(chat.DeliveryStatusDelivered) {
				t.Fatalf("message %d delivery_status: got %q, want %q", i, message.DeliveryStatus, chat.DeliveryStatusDelivered)
			}
			sentAt, err := time.Parse(time.RFC3339Nano, message.SentAt)
			if err != nil {
				t.Fatalf("message %d sent_at: %v", i, err)
			}
			if i > 0 && !sentAt.Before(previous) {
				t.Fatalf("message %d sent_at %v not strictly before the previous %v (newest-first contract order)", i, sentAt, previous)
			}
			previous = sentAt
		}
		if len(stub.historyCalls) != 1 {
			t.Fatalf("history calls: got %d, want 1", len(stub.historyCalls))
		}
	})

	t.Run("full page exposes has_more and next_before", func(t *testing.T) {
		t.Parallel()
		stub := &chatGroupServiceStub{history: chatGroupHistoryFixture(2)}
		rec := httptest.NewRecorder()
		chatGroupRouter(stub, nil).ServeHTTP(rec, chatGroupRequest(http.MethodGet, chatGroupMessagesPath(chatGroupCircleID)+"?limit=2", "", ""))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var page struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			HasMore    bool `json:"has_more"`
			NextBefore any  `json:"next_before"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
			t.Fatalf("decode page: %v", err)
		}
		if !page.HasMore {
			t.Fatal("has_more: got false, want true when the page is full")
		}
		if len(page.Data) != 2 || page.NextBefore != page.Data[1].ID {
			t.Fatalf("next_before: got %v, want the oldest page id %q", page.NextBefore, page.Data[len(page.Data)-1].ID)
		}
	})

	t.Run("cursor and limit reach the service", func(t *testing.T) {
		t.Parallel()
		before := uuid.New()
		stub := &chatGroupServiceStub{history: chatGroupHistoryFixture(1)}
		rec := httptest.NewRecorder()
		path := chatGroupMessagesPath(chatGroupCircleID) + "?before=" + before.String() + "&limit=7"
		chatGroupRouter(stub, nil).ServeHTTP(rec, chatGroupRequest(http.MethodGet, path, "", ""))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		call := stub.historyCalls[0]
		if call.before == nil || call.before.String() != before.String() {
			t.Fatalf("before cursor: got %v, want %s", call.before, before)
		}
		if call.limit != 7 {
			t.Fatalf("limit: got %d, want 7", call.limit)
		}
		if call.viewerID.String() != testLocalUserID || call.circleID.String() != chatGroupCircleID {
			t.Fatalf("call context: viewer=%s circle=%s", call.viewerID, call.circleID)
		}
	})

	cases := []struct {
		name        string
		path        string
		stubErr     error
		noBearer    bool
		noSession   bool
		wantStatus  int
		wantCode    string
		wantField   string
		notContains string
	}{
		{
			name:       "missing bearer returns 401",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			noBearer:   true,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeUnauthorized,
		},
		{
			name:       "missing session returns 401",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			noSession:  true,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionMissing,
		},
		{
			name:       "invalid circle id returns 400",
			path:       chatGroupMessagesPath("not-a-uuid"),
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldCircleID,
		},
		{
			name:       "limit zero returns 400",
			path:       chatGroupMessagesPath(chatGroupCircleID) + "?limit=0",
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldLimit,
		},
		{
			name:       "limit above maximum returns 400",
			path:       chatGroupMessagesPath(chatGroupCircleID) + "?limit=101",
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldLimit,
		},
		{
			name:       "non-numeric limit returns 400",
			path:       chatGroupMessagesPath(chatGroupCircleID) + "?limit=many",
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldLimit,
		},
		{
			name:       "malformed before cursor returns 400",
			path:       chatGroupMessagesPath(chatGroupCircleID) + "?before=not-a-uuid",
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldBefore,
		},
		{
			name:       "unknown before cursor returns 400",
			path:       chatGroupMessagesPath(chatGroupCircleID) + "?before=" + uuid.New().String(),
			stubErr:    chat.ErrInvalidCursor,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldBefore,
		},
		{
			name:        "repository failure returns 500 without internals",
			path:        chatGroupMessagesPath(chatGroupCircleID),
			stubErr:     fmt.Errorf("load chat history: %w", errors.New("connection refused")),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    httpconst.ErrorCodeInternalServerError,
			notContains: "connection refused",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stub := &chatGroupServiceStub{historyErr: tc.stubErr}
			req := chatGroupRequest(http.MethodGet, tc.path, "", "")
			if tc.noBearer {
				req.Header.Del(httpconst.HeaderAuthorization)
			}
			if tc.noSession {
				req.Header.Del(httpconst.HeaderSessionID)
			}
			rec := httptest.NewRecorder()
			chatGroupRouter(stub, nil).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != tc.wantCode {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, tc.wantCode)
			}
			if tc.wantField != "" {
				if _, ok := envelope.Error.Fields[tc.wantField]; !ok {
					t.Fatalf("expected field %q in Fields=%v", tc.wantField, envelope.Error.Fields)
				}
			}
			if tc.notContains != "" && strings.Contains(rec.Body.String(), tc.notContains) {
				t.Fatalf("error body leaks internal detail %q: %s", tc.notContains, rec.Body.String())
			}
		})
	}

	t.Run("non-member and unknown circle denials are indistinguishable", func(t *testing.T) {
		t.Parallel()
		bodies := make([]string, 0, 2)
		for _, name := range []string{"non-member", "unknown circle"} {
			stub := &chatGroupServiceStub{historyErr: chat.ErrCircleNotVisible}
			rec := httptest.NewRecorder()
			chatGroupRouter(stub, nil).ServeHTTP(rec, chatGroupRequest(http.MethodGet, chatGroupMessagesPath(chatGroupCircleID), "", ""))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s: status: got %d, want %d body=%s", name, rec.Code, http.StatusForbidden, rec.Body.String())
			}
			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != httpconst.ErrorCodeForbidden {
				t.Fatalf("%s: error code: got %q, want %q", name, envelope.Error.Code, httpconst.ErrorCodeForbidden)
			}
			if len(envelope.Error.Fields) != 0 {
				t.Fatalf("%s: forbidden envelope must carry no field details: %v", name, envelope.Error.Fields)
			}
			bodies = append(bodies, rec.Body.String())
		}
		if bodies[0] != bodies[1] {
			t.Fatalf("denial bodies differ, want byte-identical responses: %q vs %q", bodies[0], bodies[1])
		}
	})
}

func TestChatGroupSendContract(t *testing.T) {
	t.Parallel()

	t.Run("send text returns the durable Message", func(t *testing.T) {
		t.Parallel()
		stub := &chatGroupServiceStub{}
		rec := httptest.NewRecorder()
		chatGroupRouter(stub, nil).ServeHTTP(rec, chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), chatGroupTextBody, "chat-key-1"))

		if rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		assertJSONKeySet(t, rec.Body.Bytes(), chatMessageKeys)
		var message struct {
			ID             string `json:"id"`
			CircleID       string `json:"circle_id"`
			SenderID       string `json:"sender_id"`
			MessageType    string `json:"message_type"`
			Content        string `json:"content"`
			SentAt         string `json:"sent_at"`
			DeliveryStatus string `json:"delivery_status"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&message); err != nil {
			t.Fatalf("decode message: %v", err)
		}
		if _, err := uuid.Parse(message.ID); err != nil {
			t.Fatalf("message id: %v", err)
		}
		if message.CircleID != chatGroupCircleID || message.SenderID != testLocalUserID {
			t.Fatalf("identifiers: circle=%q sender=%q", message.CircleID, message.SenderID)
		}
		if message.MessageType != string(chat.MessageTypeText) || message.Content != "Salam, ready to recite" {
			t.Fatalf("payload: type=%q content=%q", message.MessageType, message.Content)
		}
		if message.DeliveryStatus != string(chat.DeliveryStatusDelivered) {
			t.Fatalf("delivery_status: got %q, want %q", message.DeliveryStatus, chat.DeliveryStatusDelivered)
		}
		if _, err := time.Parse(time.RFC3339Nano, message.SentAt); err != nil {
			t.Fatalf("sent_at: %v", err)
		}
		if len(stub.sendCalls) != 1 {
			t.Fatalf("send calls: got %d, want 1", len(stub.sendCalls))
		}
		call := stub.sendCalls[0]
		if call.senderID.String() != testLocalUserID || call.circleID.String() != chatGroupCircleID {
			t.Fatalf("call context: sender=%s circle=%s", call.senderID, call.circleID)
		}
		if call.content != "Salam, ready to recite" || call.key != "chat-key-1" {
			t.Fatalf("call payload: content=%q key=%q", call.content, call.key)
		}
	})

	t.Run("same idempotency key replays the same message", func(t *testing.T) {
		t.Parallel()
		stub := &chatGroupServiceStub{}
		router := chatGroupRouter(stub, nil)
		bodies := make([]string, 0, 2)
		for i := 0; i < 2; i++ {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), chatGroupTextBody, "chat-replay-key"))
			if rec.Code != http.StatusCreated {
				t.Fatalf("send %d status: got %d, want %d body=%s", i+1, rec.Code, http.StatusCreated, rec.Body.String())
			}
			bodies = append(bodies, rec.Body.String())
		}
		if bodies[0] != bodies[1] {
			t.Fatalf("idempotent replay bodies differ: %q vs %q", bodies[0], bodies[1])
		}
		if len(stub.sendCalls) != 2 {
			t.Fatalf("send calls: got %d, want 2 (replays resolve through the durable store)", len(stub.sendCalls))
		}
	})

	cases := []struct {
		name        string
		path        string
		body        string
		key         string
		noKey       bool
		noBearer    bool
		noSession   bool
		stubErr     error
		wantStatus  int
		wantCode    string
		wantField   string
		wantMessage string
		notContains string
	}{
		{
			name:       "missing bearer returns 401",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       chatGroupTextBody,
			key:        "deny-key",
			noBearer:   true,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeUnauthorized,
		},
		{
			name:       "missing session returns 401",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       chatGroupTextBody,
			key:        "deny-key",
			noSession:  true,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionMissing,
		},
		{
			name:       "invalid circle id returns 400",
			path:       chatGroupMessagesPath("not-a-uuid"),
			body:       chatGroupTextBody,
			key:        "deny-key",
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldCircleID,
		},
		{
			name:       "malformed json body returns 400",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       `{"message_type":`,
			key:        "deny-key",
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldBody,
		},
		{
			name:       "unknown body field returns 400",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       `{"message_type":"text","content":"hi","unexpected_field":1}`,
			key:        "deny-key",
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldBody,
		},
		{
			name:       "missing idempotency key returns 400",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       chatGroupTextBody,
			noKey:      true,
			stubErr:    chat.ErrInvalidIdempotencyKey,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldIdempotencyKey,
		},
		{
			name:       "oversized idempotency key returns 400",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       chatGroupTextBody,
			key:        strings.Repeat("k", chat.MaxIdempotencyKeyLength+1),
			stubErr:    chat.ErrInvalidIdempotencyKey,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldIdempotencyKey,
		},
		{
			name:       "unknown message type returns 422",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       `{"message_type":"video","content":""}`,
			key:        "deny-key",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldMessageType,
		},
		{
			name:       "missing message type returns 422",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       `{"content":"hi"}`,
			key:        "deny-key",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldMessageType,
		},
		{
			name:       "text with upload id returns 422",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       `{"message_type":"text","content":"hi","upload_id":"11111111-1111-1111-1111-111111111111"}`,
			key:        "deny-key",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldUploadID,
		},
		{
			name:       "legacy media key returns 422",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       `{"message_type":"text","content":"hi","media_key":"legacy-key"}`,
			key:        "deny-key",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldMediaKey,
		},
		{
			name:       "reply target returns 422",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       `{"message_type":"text","content":"hi","reply_to_id":"11111111-1111-1111-1111-111111111111"}`,
			key:        "deny-key",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldReplyToID,
		},
		{
			name:       "empty text returns 422",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       `{"message_type":"text","content":"   "}`,
			key:        "deny-key",
			stubErr:    chat.ErrInvalidText,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldContent,
		},
		{
			name:       "overlong text returns 422",
			path:       chatGroupMessagesPath(chatGroupCircleID),
			body:       `{"message_type":"text","content":"` + strings.Repeat("a", chat.MaxTextRunes+1) + `"}`,
			key:        "deny-key",
			stubErr:    chat.ErrInvalidText,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldContent,
		},
		{
			name:        "archived circle returns 409",
			path:        chatGroupMessagesPath(chatGroupCircleID),
			body:        chatGroupTextBody,
			key:         "deny-key",
			stubErr:     chat.ErrCircleArchived,
			wantStatus:  http.StatusConflict,
			wantCode:    httpconst.ErrorCodeConflict,
			wantMessage: httpconst.ErrorMessageCircleArchived,
		},
		{
			name:        "repository failure returns 500 without internals",
			path:        chatGroupMessagesPath(chatGroupCircleID),
			body:        chatGroupTextBody,
			key:         "deny-key",
			stubErr:     fmt.Errorf("send chat message: %w", errors.New("connection refused")),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    httpconst.ErrorCodeInternalServerError,
			notContains: "connection refused",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stub := &chatGroupServiceStub{sendErr: tc.stubErr}
			key := tc.key
			if !tc.noKey && key == "" {
				key = "deny-key"
			}
			req := chatGroupRequest(http.MethodPost, tc.path, tc.body, key)
			if tc.noKey {
				req.Header.Del(httpconst.HeaderIdempotencyKey)
			}
			if tc.noBearer {
				req.Header.Del(httpconst.HeaderAuthorization)
			}
			if tc.noSession {
				req.Header.Del(httpconst.HeaderSessionID)
			}
			rec := httptest.NewRecorder()
			chatGroupRouter(stub, nil).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != tc.wantCode {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, tc.wantCode)
			}
			if tc.wantMessage != "" && envelope.Error.Message != tc.wantMessage {
				t.Fatalf("error message: got %q, want %q", envelope.Error.Message, tc.wantMessage)
			}
			if tc.wantField != "" {
				if _, ok := envelope.Error.Fields[tc.wantField]; !ok {
					t.Fatalf("expected field %q in Fields=%v", tc.wantField, envelope.Error.Fields)
				}
			}
			if tc.notContains != "" && strings.Contains(rec.Body.String(), tc.notContains) {
				t.Fatalf("error body leaks internal detail %q: %s", tc.notContains, rec.Body.String())
			}
		})
	}

	t.Run("non-member and unknown circle denials are indistinguishable", func(t *testing.T) {
		t.Parallel()
		bodies := make([]string, 0, 2)
		for _, name := range []string{"non-member", "unknown circle"} {
			stub := &chatGroupServiceStub{sendErr: chat.ErrCircleNotVisible}
			rec := httptest.NewRecorder()
			chatGroupRouter(stub, nil).ServeHTTP(rec, chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), chatGroupTextBody, "deny-key"))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s: status: got %d, want %d body=%s", name, rec.Code, http.StatusForbidden, rec.Body.String())
			}
			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != httpconst.ErrorCodeForbidden {
				t.Fatalf("%s: error code: got %q, want %q", name, envelope.Error.Code, httpconst.ErrorCodeForbidden)
			}
			if len(envelope.Error.Fields) != 0 {
				t.Fatalf("%s: forbidden envelope must carry no field details: %v", name, envelope.Error.Fields)
			}
			bodies = append(bodies, rec.Body.String())
		}
		if bodies[0] != bodies[1] {
			t.Fatalf("denial bodies differ, want byte-identical responses: %q vs %q", bodies[0], bodies[1])
		}
	})
}

func TestChatGroupSendRateLimitContract(t *testing.T) {
	t.Parallel()

	const perMinute = 30 // FR-006: 30 sends per minute per user and circle
	stub := &chatGroupServiceStub{}
	router := chatGroupRouter(stub, chat.NewChatSendLimiter(perMinute))

	for i := 0; i < perMinute; i++ {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), chatGroupTextBody, fmt.Sprintf("rate-key-%d", i)))
		if rec.Code != http.StatusCreated {
			t.Fatalf("send %d status: got %d, want %d body=%s", i+1, rec.Code, http.StatusCreated, rec.Body.String())
		}
	}

	over := httptest.NewRecorder()
	router.ServeHTTP(over, chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), chatGroupTextBody, "rate-key-over"))
	if over.Code != http.StatusTooManyRequests {
		t.Fatalf("send %d status: got %d, want %d body=%s", perMinute+1, over.Code, http.StatusTooManyRequests, over.Body.String())
	}
	envelope := decodeErrorEnvelope(t, over)
	if envelope.Error.Code != httpconst.ErrorCodeRateLimitExceeded {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeRateLimitExceeded)
	}
	if envelope.Error.Message != httpconst.ErrorMessageRateLimitExceeded {
		t.Fatalf("error message: got %q, want %q", envelope.Error.Message, httpconst.ErrorMessageRateLimitExceeded)
	}
	if retryAfter := over.Header().Get("Retry-After"); retryAfter != "" {
		t.Fatalf("Retry-After: got %q, want none (the chat contract declares no Retry-After header)", retryAfter)
	}
	if len(stub.sendCalls) != perMinute {
		t.Fatalf("send calls: got %d, want %d (over-budget sends must not reach the service)", len(stub.sendCalls), perMinute)
	}

	t.Run("budget is per user and circle", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupOtherCircleID), chatGroupTextBody, "rate-key-other-circle"))
		if rec.Code != http.StatusCreated {
			t.Fatalf("other-circle send status: got %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
	})

	t.Run("list stays available after an exhausted send budget", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, chatGroupRequest(http.MethodGet, chatGroupMessagesPath(chatGroupCircleID), "", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("list status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})
}

func TestChatGroupResponseSafetyContract(t *testing.T) {
	t.Parallel()

	leakMarkers := []string{testSessionID, "valid-register-token", "postgres", "connection refused"}

	t.Run("send response carries only safe Message fields", func(t *testing.T) {
		t.Parallel()
		stub := &chatGroupServiceStub{}
		rec := httptest.NewRecorder()
		chatGroupRouter(stub, nil).ServeHTTP(rec, chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), chatGroupTextBody, "safety-key"))
		if rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		assertJSONKeySet(t, rec.Body.Bytes(), chatMessageKeys)
		for _, marker := range leakMarkers {
			if strings.Contains(rec.Body.String(), marker) {
				t.Fatalf("send body leaks %q: %s", marker, rec.Body.String())
			}
		}
	})

	t.Run("list response carries only safe page fields", func(t *testing.T) {
		t.Parallel()
		stub := &chatGroupServiceStub{history: chatGroupHistoryFixture(2)}
		rec := httptest.NewRecorder()
		chatGroupRouter(stub, nil).ServeHTTP(rec, chatGroupRequest(http.MethodGet, chatGroupMessagesPath(chatGroupCircleID), "", ""))
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		body := rec.Body.Bytes()
		var page map[string]json.RawMessage
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatalf("decode page: %v", err)
		}
		assertJSONKeySet(t, body, []string{"data", "has_more", "next_before"})
		var data []json.RawMessage
		if err := json.Unmarshal(page["data"], &data); err != nil {
			t.Fatalf("decode data: %v", err)
		}
		for _, raw := range data {
			assertJSONKeySet(t, raw, chatMessageKeys)
		}
		for _, marker := range leakMarkers {
			if strings.Contains(rec.Body.String(), marker) {
				t.Fatalf("list body leaks %q: %s", marker, rec.Body.String())
			}
		}
	})

	t.Run("error responses stay within the standard envelope", func(t *testing.T) {
		t.Parallel()
		stub := &chatGroupServiceStub{sendErr: chat.ErrCircleNotVisible}
		rec := httptest.NewRecorder()
		chatGroupRouter(stub, nil).ServeHTTP(rec, chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), chatGroupTextBody, "safety-key"))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status: got %d, want %d", rec.Code, http.StatusForbidden)
		}
		assertJSONKeySet(t, rec.Body.Bytes(), []string{"error"})
		var envelope struct {
			Error map[string]json.RawMessage `json:"error"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&envelope); err != nil {
			t.Fatalf("decode envelope: %v", err)
		}
		for _, key := range []string{"code", "message"} {
			if _, ok := envelope.Error[key]; !ok {
				t.Fatalf("error object missing key %q: %s", key, rec.Body.String())
			}
		}
		if _, ok := envelope.Error["fields"]; ok && len(envelope.Error) != 3 {
			t.Fatalf("error object carries unexpected keys: %s", rec.Body.String())
		}
		for _, marker := range leakMarkers {
			if strings.Contains(rec.Body.String(), marker) {
				t.Fatalf("error body leaks %q: %s", marker, rec.Body.String())
			}
		}
	})
}

// chatGroupMediaSendCall records one SendGroupMedia call reaching the media
// stub.
type chatGroupMediaSendCall struct {
	input chat.SendGroupMediaInput
}

// chatGroupMediaServiceStub is a deterministic chat.GroupMediaService.
// SendGroupMedia mirrors the durable idempotency contract: the first call
// commits a media message and every later call returns that same committed
// message (FR-007).
type chatGroupMediaServiceStub struct {
	sendCalls []chatGroupMediaSendCall
	sendErr   error
	committed chat.Message
}

func (s *chatGroupMediaServiceStub) RenewMediaURL(context.Context, uuid.UUID, uuid.UUID) (chat.MediaAccess, error) {
	return chat.MediaAccess{URL: &url.URL{Scheme: "https", Host: "media.example", Path: "/signed"}, ExpiresAt: time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), FileName: "lesson.ogg", VoiceDurationSeconds: 2}, nil
}

func TestChatGroupHistoryProjectsMedia(t *testing.T) {
	circle := uuid.MustParse(chatGroupCircleID)
	handler := chat.NewGroupHandler(&chatGroupServiceStub{history: []chat.Message{
		{ID: uuid.New(), CircleID: &circle, Type: chat.MessageTypeVoice, State: chat.MessageStateActive},
		{ID: uuid.New(), CircleID: &circle, Type: chat.MessageTypeText, Content: "hello", State: chat.MessageStateActive},
	}})
	handler.SetMediaService(&chatGroupMediaServiceStub{})
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID})
	router := api.NewRouter(api.MiddlewareSet{Auth: authMW, ChatHandler: handler}).Handler()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, chatGroupRequest(http.MethodGet, chatGroupMessagesPath(chatGroupCircleID), "", ""))
	if response.Code != http.StatusOK {
		t.Fatalf("history status=%d body=%s", response.Code, response.Body.String())
	}
	var page struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Data) != 2 || page.Data[0]["media_url"] != "https://media.example/signed" || page.Data[0]["voice_duration_seconds"] != float64(2) || page.Data[0]["file_name"] != "lesson.ogg" || page.Data[0]["media_url_expires_at"] == nil {
		t.Fatalf("incomplete history media: %s", response.Body.String())
	}
	if _, exists := page.Data[1]["media_url"]; exists {
		t.Fatal("text response gained media URL")
	}
}

func (s *chatGroupMediaServiceStub) SendGroupMedia(_ context.Context, in chat.SendGroupMediaInput) (chat.Message, error) {
	s.sendCalls = append(s.sendCalls, chatGroupMediaSendCall{input: in})
	if s.sendErr != nil {
		return chat.Message{}, s.sendErr
	}
	if s.committed.ID == uuid.Nil {
		s.committed = chat.Message{
			ID:       uuid.New(),
			CircleID: &in.CircleID,
			SenderID: in.SenderID,
			Type:     in.MessageType,
			UploadID: &in.UploadID,
			State:    chat.MessageStateActive,
			SentAt:   time.Now().UTC(),
		}
	}
	return s.committed, nil
}

// chatGroupMediaRouter wires the production router with the group handler
// carrying the media seam, so media-send cases exercise the exact deployed
// middleware order.
func chatGroupMediaRouter(media chat.GroupMediaService) http.Handler {
	authMW := middleware.NewAuthMiddleware(
		&alwaysOKVerifier{},
		auth.NewSessionService(30*24*time.Hour),
		&stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID},
	)
	handler := chat.NewGroupHandler(&chatGroupServiceStub{})
	handler.SetMediaService(media)
	return api.NewRouter(api.MiddlewareSet{
		Auth:        authMW,
		ChatHandler: handler,
	}).Handler()
}

// chatGroupVoiceSendBody is one well-formed voice send.
const chatGroupVoiceSendBody = `{"message_type":"voice","upload_id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}`

func TestChatGroupMediaSendContract(t *testing.T) {
	t.Parallel()

	t.Run("media send routes through the media seam and returns the durable Message", func(t *testing.T) {
		t.Parallel()
		stub := &chatGroupMediaServiceStub{}
		rec := httptest.NewRecorder()
		chatGroupMediaRouter(stub).ServeHTTP(rec, chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), chatGroupVoiceSendBody, "media-key-1"))

		if rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusCreated, rec.Body.String())
		}
		// Media messages carry no content, so the safe projection omits the key.
		assertJSONKeySet(t, rec.Body.Bytes(), []string{"id", "circle_id", "sender_id", "message_type", "sent_at", "delivery_status", "media_url", "media_url_expires_at", "file_name", "voice_duration_seconds"})
		var message struct {
			MessageType string `json:"message_type"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&message); err != nil {
			t.Fatalf("decode message: %v body=%s", err, rec.Body.String())
		}
		if message.MessageType != string(chat.MessageTypeVoice) {
			t.Fatalf("message_type: got %q, want voice", message.MessageType)
		}
		if len(stub.sendCalls) != 1 {
			t.Fatalf("media send calls: got %d, want 1", len(stub.sendCalls))
		}
		call := stub.sendCalls[0].input
		if call.SenderID.String() != testLocalUserID || call.CircleID.String() != chatGroupCircleID {
			t.Fatalf("call context: sender=%s circle=%s", call.SenderID, call.CircleID)
		}
		if call.UploadID.String() != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" || call.MessageType != chat.MessageTypeVoice {
			t.Fatalf("call payload: upload=%s type=%s", call.UploadID, call.MessageType)
		}
		if call.IdempotencyKey != "media-key-1" {
			t.Fatalf("idempotency key: got %q, want the request header", call.IdempotencyKey)
		}
		if len(stub.sendCalls) != 1 {
			t.Fatalf("send calls: got %d, want 1", len(stub.sendCalls))
		}
	})

	t.Run("same idempotency key replays the same media message", func(t *testing.T) {
		t.Parallel()
		stub := &chatGroupMediaServiceStub{}
		router := chatGroupMediaRouter(stub)
		bodies := make([]string, 0, 2)
		for i := 0; i < 2; i++ {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), chatGroupVoiceSendBody, "media-replay-key"))
			if rec.Code != http.StatusCreated {
				t.Fatalf("send %d status: got %d, want %d body=%s", i+1, rec.Code, http.StatusCreated, rec.Body.String())
			}
			bodies = append(bodies, rec.Body.String())
		}
		if bodies[0] != bodies[1] {
			t.Fatalf("idempotent replay bodies differ: %q vs %q", bodies[0], bodies[1])
		}
	})

	cases := []struct {
		name        string
		body        string
		key         string
		noKey       bool
		stubErr     error
		wantStatus  int
		wantCode    string
		wantField   string
		wantMessage string
	}{
		{
			name:       "missing upload id returns 422",
			body:       `{"message_type":"image"}`,
			key:        "media-deny-key",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldUploadID,
		},
		{
			name:       "malformed upload id returns 400",
			body:       `{"message_type":"file","upload_id":"not-a-uuid"}`,
			key:        "media-deny-key",
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldUploadID,
		},
		{
			name:       "media send with content returns 422",
			body:       `{"message_type":"voice","upload_id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","content":"hi"}`,
			key:        "media-deny-key",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldContent,
		},
		{
			name:       "legacy media key returns 422",
			body:       `{"message_type":"voice","upload_id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","media_key":"legacy"}`,
			key:        "media-deny-key",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldMediaKey,
		},
		{
			name:       "reply target returns 422",
			body:       `{"message_type":"voice","upload_id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","reply_to_id":"11111111-1111-1111-1111-111111111111"}`,
			key:        "media-deny-key",
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldReplyToID,
		},
		{
			name:        "already attached upload returns 409",
			body:        chatGroupVoiceSendBody,
			key:         "media-deny-key",
			stubErr:     chat.ErrUploadNotStaged,
			wantStatus:  http.StatusConflict,
			wantCode:    httpconst.ErrorCodeConflict,
			wantMessage: httpconst.ErrorMessageChatUploadNotAttachable,
		},
		{
			name:        "foreign upload returns 409",
			body:        chatGroupVoiceSendBody,
			key:         "media-deny-key",
			stubErr:     chat.ErrUploadNotAttachable,
			wantStatus:  http.StatusConflict,
			wantCode:    httpconst.ErrorCodeConflict,
			wantMessage: httpconst.ErrorMessageChatUploadNotAttachable,
		},
		{
			name:        "archived circle returns 409",
			body:        chatGroupVoiceSendBody,
			key:         "media-deny-key",
			stubErr:     chat.ErrCircleArchived,
			wantStatus:  http.StatusConflict,
			wantCode:    httpconst.ErrorCodeConflict,
			wantMessage: httpconst.ErrorMessageCircleArchived,
		},
		{
			name:       "repository failure returns 500 without internals",
			body:       chatGroupVoiceSendBody,
			key:        "media-deny-key",
			stubErr:    fmt.Errorf("send chat media message: %w", errors.New("connection refused")),
			wantStatus: http.StatusInternalServerError,
			wantCode:   httpconst.ErrorCodeInternalServerError,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stub := &chatGroupMediaServiceStub{sendErr: tc.stubErr}
			req := chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), tc.body, tc.key)
			if tc.noKey {
				req.Header.Del(httpconst.HeaderIdempotencyKey)
			}
			rec := httptest.NewRecorder()
			chatGroupMediaRouter(stub).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != tc.wantCode {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, tc.wantCode)
			}
			if tc.wantMessage != "" && envelope.Error.Message != tc.wantMessage {
				t.Fatalf("error message: got %q, want %q", envelope.Error.Message, tc.wantMessage)
			}
			if tc.wantField != "" {
				if _, ok := envelope.Error.Fields[tc.wantField]; !ok {
					t.Fatalf("expected field %q in Fields=%v", tc.wantField, envelope.Error.Fields)
				}
			}
			if strings.Contains(rec.Body.String(), "connection refused") {
				t.Fatalf("error body leaks internal detail: %s", rec.Body.String())
			}
		})
	}

	t.Run("non-member and unknown circle denials stay non-enumerating", func(t *testing.T) {
		t.Parallel()
		stub := &chatGroupMediaServiceStub{sendErr: chat.ErrCircleNotVisible}
		rec := httptest.NewRecorder()
		chatGroupMediaRouter(stub).ServeHTTP(rec, chatGroupRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), chatGroupVoiceSendBody, "media-deny-key"))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
		}
		envelope := decodeErrorEnvelope(t, rec)
		if envelope.Error.Code != httpconst.ErrorCodeForbidden {
			t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeForbidden)
		}
		if len(envelope.Error.Fields) != 0 {
			t.Fatalf("forbidden envelope must carry no field details: %v", envelope.Error.Fields)
		}
	})
}
