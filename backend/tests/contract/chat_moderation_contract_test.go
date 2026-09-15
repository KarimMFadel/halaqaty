//go:build contract

package contract

import (
	"context"
	"github.com/KarimMFadel/halaqaty/backend/internal/api"
	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/google/uuid"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestChatModerationContract_DeleteMapsRBACConflictAndNotFoundWithoutLeakage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{name: "non teacher forbidden", err: chat.ErrMessageNotVisible, status: http.StatusNotFound},
		{name: "late conflict", err: chat.ErrDirectDeleteConflict, status: http.StatusConflict},
		{name: "relationship forbidden", err: chat.ErrDMNotEligible, status: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := "/api/v1/circles/" + uuid.NewString() + "/messages/" + uuid.NewString()
			req := chatGroupRequest(http.MethodDelete, path, "", "moderation-key")
			rec := httptest.NewRecorder()
			chatModerationRouter(&moderationContractStub{err: tc.err}).ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tc.status, rec.Body.String())
			}
			for _, forbidden := range []string{"content", "object_key", "media_url", "teacher_delete"} {
				if strings.Contains(rec.Body.String(), forbidden) {
					t.Fatalf("response leaks %q: %s", forbidden, rec.Body.String())
				}
			}
		})
	}
}

func TestChatModerationContract_GroupAuthorizationMatrix(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "sender", err: nil, want: http.StatusNoContent},
		{name: "teacher", err: nil, want: http.StatusNoContent},
		{name: "non-teacher", err: chat.ErrMessageNotVisible, want: http.StatusNotFound},
		{name: "non-member", err: chat.ErrCircleNotVisible, want: http.StatusForbidden},
		{name: "deadline conflict", err: chat.ErrDirectDeleteConflict, want: http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			path := "/api/v1/circles/" + uuid.NewString() + "/messages/" + uuid.NewString()
			chatModerationRouter(&moderationContractStub{err: tc.err}).ServeHTTP(rec, chatGroupRequest(http.MethodDelete, path, "", "matrix-key"))
			if rec.Code != tc.want {
				t.Fatalf("status=%d, want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
			if rec.Body.Len() != 0 && tc.want == http.StatusNoContent {
				t.Fatalf("successful response leaked body: %s", rec.Body.String())
			}
		})
	}
}

func TestChatModerationContract_GroupDeleteReplayIsResponseSafe(t *testing.T) {
	stub := &moderationContractStub{}
	router := chatModerationRouter(stub)
	path := "/api/v1/circles/" + uuid.NewString() + "/messages/" + uuid.NewString()
	for attempt := 0; attempt < 2; attempt++ {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, chatGroupRequest(http.MethodDelete, path, "", "same-idempotency-key"))
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("replay %d status=%d body=%q, want 204 with empty body", attempt+1, rec.Code, rec.Body.String())
		}
	}
}

type moderationDirectContractStub struct {
	deleteErr   error
	deleteCalls int
}

func (s *moderationDirectContractStub) History(context.Context, uuid.UUID, uuid.UUID, *uuid.UUID, int) ([]chat.Message, error) {
	return nil, nil
}
func (s *moderationDirectContractStub) SendText(context.Context, uuid.UUID, uuid.UUID, string, string) (chat.Message, error) {
	return chat.Message{}, nil
}
func (s *moderationDirectContractStub) ReplyText(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, string) (chat.Message, error) {
	return chat.Message{}, nil
}
func (s *moderationDirectContractStub) DeleteOwnMessage(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	s.deleteCalls++
	return s.deleteErr
}

func TestChatModerationContract_DirectRouteAuthorizationAndDeadlineMatrix(t *testing.T) {
	peerID, messageID := uuid.NewString(), uuid.NewString()
	cases := []struct {
		name string
		err  error
		want int
	}{
		{name: "sender", want: http.StatusNoContent},
		{name: "non-member", err: chat.ErrDMNotEligible, want: http.StatusForbidden},
		{name: "non-sender", err: chat.ErrMessageNotVisible, want: http.StatusForbidden},
		{name: "deadline conflict", err: chat.ErrDirectDeleteConflict, want: http.StatusConflict},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &moderationDirectContractStub{deleteErr: tc.err}
			rec := httptest.NewRecorder()
			req := chatGroupRequest(http.MethodDelete, "/api/v1/dm/"+peerID+"/messages/"+messageID, "", "dm-matrix-key")
			directRouter(stub).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status=%d, want %d body=%s", rec.Code, tc.want, rec.Body.String())
			}
			if strings.Contains(rec.Body.String(), "content") || strings.Contains(rec.Body.String(), "audit") {
				t.Fatalf("response leaked moderation details: %s", rec.Body.String())
			}
		})
	}
}

func TestChatModerationContract_DirectDeleteReplayDelegatesEachAttemptSafely(t *testing.T) {
	peerID, messageID := uuid.NewString(), uuid.NewString()
	stub := &moderationDirectContractStub{}
	router := directRouter(stub)
	for attempt := 0; attempt < 2; attempt++ {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, chatGroupRequest(http.MethodDelete, "/api/v1/dm/"+peerID+"/messages/"+messageID, "", "same-dm-delete-key"))
		if rec.Code != http.StatusNoContent || rec.Body.Len() != 0 {
			t.Fatalf("replay %d status=%d body=%q, want 204 with empty body", attempt+1, rec.Code, rec.Body.String())
		}
	}
	if stub.deleteCalls != 2 {
		t.Fatalf("direct delete service calls=%d, want one safe domain call per replay", stub.deleteCalls)
	}
}

func TestChatModerationContract_AuthorizationResponsesAreAuditSafe(t *testing.T) {
	checks := []struct {
		name  string
		route func(error) http.Handler
		path  string
		want  int
	}{
		{name: "group sender", route: func(err error) http.Handler { return chatModerationRouter(&moderationContractStub{err: err}) }, path: "/api/v1/circles/" + uuid.NewString() + "/messages/" + uuid.NewString(), want: http.StatusNoContent},
		{name: "group teacher", route: func(err error) http.Handler { return chatModerationRouter(&moderationContractStub{err: err}) }, path: "/api/v1/circles/" + uuid.NewString() + "/messages/" + uuid.NewString(), want: http.StatusNoContent},
		{name: "group non-teacher", route: func(err error) http.Handler { return chatModerationRouter(&moderationContractStub{err: err}) }, path: "/api/v1/circles/" + uuid.NewString() + "/messages/" + uuid.NewString(), want: http.StatusNotFound},
		{name: "group non-member", route: func(err error) http.Handler { return chatModerationRouter(&moderationContractStub{err: err}) }, path: "/api/v1/circles/" + uuid.NewString() + "/messages/" + uuid.NewString(), want: http.StatusForbidden},
		{name: "direct sender", route: func(err error) http.Handler { return directRouter(&moderationDirectContractStub{deleteErr: err}) }, path: "/api/v1/dm/" + uuid.NewString() + "/messages/" + uuid.NewString(), want: http.StatusNoContent},
		{name: "direct non-member", route: func(err error) http.Handler { return directRouter(&moderationDirectContractStub{deleteErr: err}) }, path: "/api/v1/dm/" + uuid.NewString() + "/messages/" + uuid.NewString(), want: http.StatusForbidden},
		{name: "direct non-sender", route: func(err error) http.Handler { return directRouter(&moderationDirectContractStub{deleteErr: err}) }, path: "/api/v1/dm/" + uuid.NewString() + "/messages/" + uuid.NewString(), want: http.StatusForbidden},
	}
	errorsByName := map[string]error{
		"group non-teacher": chat.ErrMessageNotVisible,
		"group non-member":  chat.ErrCircleNotVisible,
		"direct non-member": chat.ErrDMNotEligible,
		"direct non-sender": chat.ErrMessageNotVisible,
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.route(errorsByName[tc.name]).ServeHTTP(rec, chatGroupRequest(http.MethodDelete, tc.path, "", "audit-safe-key"))
			if rec.Code != tc.want || (tc.want == http.StatusNoContent && rec.Body.Len() != 0) {
				t.Fatalf("status=%d body=%q, want %d and empty success body", rec.Code, rec.Body.String(), tc.want)
			}
			for _, forbidden := range []string{"content", "object_key", "media_url", "audit", "teacher_delete"} {
				if strings.Contains(rec.Body.String(), forbidden) {
					t.Fatalf("response leaks %q: %s", forbidden, rec.Body.String())
				}
			}
		})
	}
}

type moderationContractStub struct{ err error }

func (s moderationContractStub) Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return s.err
}

func chatModerationRouter(service chat.ModerationService) http.Handler {
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID})
	return api.NewRouter(api.MiddlewareSet{Auth: authMW, ChatModerationHandler: chat.NewModerationHandler(service)}).Handler()
}

var _ = chat.MessageStateDeleted
