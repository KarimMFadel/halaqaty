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
}

func (s *directServiceStub) History(_ context.Context, _, _ uuid.UUID, _ *uuid.UUID, _ int) ([]chat.Message, error) {
	if !s.eligible {
		return nil, chat.ErrDMNotEligible
	}
	return []chat.Message{s.message}, nil
}

func (s *directServiceStub) SendText(_ context.Context, senderID, peerID uuid.UUID, content, _ string) (chat.Message, error) {
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
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID})
	return api.NewRouter(api.MiddlewareSet{Auth: authMW, DirectChatHandler: chat.NewDirectHandler(service)}).Handler()
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
