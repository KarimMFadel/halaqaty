//go:build contract

package contract

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/api"
	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

type directServiceStub struct {
	eligible bool
	message  chat.Message
	sendErr  error
}

func (s *directServiceStub) History(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ int) ([]chat.Message, error) {
	if !s.eligible {
		return nil, chat.ErrDMNotEligible
	}
	return []chat.Message{s.message}, nil
}

func (s *directServiceStub) SendText(_ context.Context, senderID, peerID uuid.UUID, content, _ string) (chat.Message, error) {
	if s.sendErr != nil {
		return chat.Message{}, s.sendErr
	}
	if !s.eligible {
		return chat.Message{}, chat.ErrDMNotEligible
	}
	s.message = chat.Message{ID: uuid.New(), DMRecipientID: &peerID, SenderID: senderID, Type: chat.MessageTypeText, Content: content, State: chat.MessageStateActive, SentAt: time.Now().UTC()}
	return s.message, nil
}

func (s *directServiceStub) DeleteOwnMessage(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	if !s.eligible {
		return chat.ErrDMNotEligible
	}
	return nil
}

func directRouter(service chat.DirectChatService) http.Handler {
	return directRouterWithLimiter(service, nil)
}

func directRouterWithLimiter(service chat.DirectChatService, limiter *chat.ChatSendLimiter) http.Handler {
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID})
	return api.NewRouter(api.MiddlewareSet{
		Auth:              authMW,
		DirectChatHandler: chat.NewDirectHandler(service),
		ChatSendLimiter:   limiter,
	}).Handler()
}

func TestDirectContract_EligibleListAndSendAreResponseSafe(t *testing.T) {
	peerID := uuid.NewString()
	service := &directServiceStub{eligible: true}
	handler := directRouter(service)
	request := func(method, body, key string) *http.Request {
		req := httptest.NewRequest(method, "/api/v1/dm/"+peerID, strings.NewReader(body))
		req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
		req.Header.Set(httpconst.HeaderSessionID, testSessionID)
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		req.Header.Set(httpconst.HeaderIdempotencyKey, key)
		return req
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, request(http.MethodPost, `{"message_type":"text","content":"salam"}`, "dm-contract-key"))
	if rec.Code != http.StatusCreated {
		t.Fatalf("send status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "object_key") || strings.Contains(rec.Body.String(), "url") {
		t.Fatalf("direct response leaked private media fields: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, request(http.MethodGet, "", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDirectContract_IneligiblePairIsNonEnumerating(t *testing.T) {
	service := &directServiceStub{}
	handler := directRouter(service)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/dm/"+uuid.NewString(), nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("ineligible list status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "eligible") {
		t.Fatalf("ineligible response disclosed relationship details: %s", rec.Body.String())
	}
}

func TestDirectContract_RejectsSelfPeerAndMissingMedia(t *testing.T) {
	handler := directRouter(&directServiceStub{eligible: true})

	self := httptest.NewRequest(http.MethodGet, "/api/v1/dm/11111111-1111-1111-1111-111111111111", nil)
	self.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	self.Header.Set(httpconst.HeaderSessionID, testSessionID)
	// The contract router's authenticated principal is testLocalUserID; using
	// that exact path value must not turn a self-DM into a service lookup.
	self.URL.Path = "/api/v1/dm/" + testLocalUserID
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, self)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("self peer status=%d body=%s, want 400", rec.Code, rec.Body.String())
	}

	media := httptest.NewRequest(http.MethodPost, "/api/v1/dm/"+uuid.NewString(), strings.NewReader(`{"message_type":"image"}`))
	media.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	media.Header.Set(httpconst.HeaderSessionID, testSessionID)
	media.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	media.Header.Set(httpconst.HeaderIdempotencyKey, "media-missing-upload")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, media)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing upload status=%d body=%s, want 422", rec.Code, rec.Body.String())
	}
}

func TestDirectContract_DeleteRequiresIdempotencyAndReturnsNoContent(t *testing.T) {
	service := &directServiceStub{eligible: true}
	handler := directRouter(service)
	path := "/api/v1/dm/" + uuid.NewString() + "/messages/" + uuid.NewString()

	req := httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing delete key status=%d body=%s, want 400", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodDelete, path, nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	req.Header.Set(httpconst.HeaderIdempotencyKey, "delete-contract-key")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%s, want 204", rec.Code, rec.Body.String())
	}
}

func TestDirectContract_IdempotencyConflictIsSafeConflict(t *testing.T) {
	handler := directRouter(&directServiceStub{
		eligible: true,
		sendErr:  chat.ErrIdempotencyConflict,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/dm/"+uuid.NewString(), strings.NewReader(`{"message_type":"text","content":"different"}`))
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	req.Header.Set(httpconst.HeaderIdempotencyKey, "reused-key")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("idempotency conflict status=%d body=%s, want 409", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "different\"") {
		t.Fatalf("conflict response leaked request content: %s", rec.Body.String())
	}
}

func TestDirectContract_RateLimitUsesUnorderedPairBudget(t *testing.T) {
	handler := directRouterWithLimiter(&directServiceStub{eligible: true}, chat.NewChatSendLimiter(1))
	path := "/api/v1/dm/" + uuid.NewString()
	request := func() *http.Request {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"message_type":"text","content":"salam"}`))
		req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
		req.Header.Set(httpconst.HeaderSessionID, testSessionID)
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		req.Header.Set(httpconst.HeaderIdempotencyKey, uuid.NewString())
		return req
	}

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, request())
	if first.Code != http.StatusCreated {
		t.Fatalf("first direct send status=%d body=%s, want 201", first.Code, first.Body.String())
	}
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, request())
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second direct send status=%d body=%s, want 429", second.Code, second.Body.String())
	}
}
