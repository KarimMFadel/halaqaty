//go:build contract

package contract

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
)

type presenceServiceStub struct {
	groupErr  error
	directErr error
}

func (s presenceServiceStub) MarkGroupMessageRead(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return s.groupErr
}

func (s presenceServiceStub) MarkDirectMessageRead(context.Context, uuid.UUID, uuid.UUID) error {
	return s.directErr
}

func presenceRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Idempotency-Key", "presence-contract-key")
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{
		UserID: "11111111-1111-1111-1111-111111111111",
	}))
	return req
}

func TestChatPresenceContract_GroupMarkReadAndArchivedConflict(t *testing.T) {
	circleID := "22222222-2222-2222-2222-222222222222"
	messageID := "33333333-3333-3333-3333-333333333333"
	handler := chat.NewPresenceHandler(presenceServiceStub{groupErr: chat.ErrCircleArchived})
	req := presenceRequest(http.MethodPost, "/api/v1/circles/"+circleID+"/messages/"+messageID+"/read")
	req.SetPathValue("circleId", circleID)
	req.SetPathValue("messageId", messageID)
	rec := httptest.NewRecorder()
	handler.MarkCircleMessageRead(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("archived mark-read status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestChatPresenceContract_DirectMarkReadRequiresIdempotencyKey(t *testing.T) {
	handler := chat.NewPresenceHandler(presenceServiceStub{})
	req := presenceRequest(http.MethodPost, "/api/v1/dm/22222222-2222-2222-2222-222222222222/messages/33333333-3333-3333-3333-333333333333/read")
	req.Header.Del("Idempotency-Key")
	req.SetPathValue("userId", "22222222-2222-2222-2222-222222222222")
	req.SetPathValue("messageId", "33333333-3333-3333-3333-333333333333")
	rec := httptest.NewRecorder()
	handler.MarkDirectMessageRead(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency key status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestChatPresenceContract_DirectMarkReadMapsEligibilityDenial(t *testing.T) {
	handler := chat.NewPresenceHandler(presenceServiceStub{directErr: chat.ErrDMNotEligible})
	req := presenceRequest(http.MethodPost, "/api/v1/dm/22222222-2222-2222-2222-222222222222/messages/33333333-3333-3333-3333-333333333333/read")
	req.SetPathValue("userId", "22222222-2222-2222-2222-222222222222")
	req.SetPathValue("messageId", "33333333-3333-3333-3333-333333333333")
	rec := httptest.NewRecorder()
	handler.MarkDirectMessageRead(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("ineligible DM status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
