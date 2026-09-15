//go:build contract

package contract

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/api"
	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/google/uuid"
)

func TestChatModerationContract_DeleteRouteUsesNoContentAndRedactsResponse(t *testing.T) {
	service := &moderationContractStub{}
	path := "/api/v1/circles/" + uuid.NewString() + "/messages/" + uuid.NewString()
	req := chatGroupRequest(http.MethodDelete, path, "", "moderation-key")
	rec := httptest.NewRecorder()
	chatModerationRouter(service).ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("delete response body leaked audit/content: %s", rec.Body.String())
	}
}

type moderationContractStub struct{}

func (moderationContractStub) Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error { return nil }

func chatModerationRouter(service chat.ModerationService) http.Handler {
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID})
	return api.NewRouter(api.MiddlewareSet{Auth: authMW, ChatModerationHandler: chat.NewModerationHandler(service)}).Handler()
}

var _ = chat.MessageStateDeleted
