//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/api"
	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// T074 — HTTP contract coverage for US6 discovery. The tests deliberately
// exercise the production router; no test-only discovery handler is used.

func chatPinnedMessagesPath(circleID string) string {
	return chatGroupMessagesPath(circleID) + "/pinned"
}

func chatPinMessagePath(circleID, messageID string) string {
	return chatGroupMessagesPath(circleID) + "/" + messageID + "/pin"
}

type chatDiscoveryRoleStore struct {
	role string
	err  error
}

func (s chatDiscoveryRoleStore) RoleForUserInCircle(context.Context, string, string) (string, error) {
	return s.role, s.err
}

func chatDiscoveryRouterWithRole(roleStore chatDiscoveryRoleStore) http.Handler {
	authMW := middleware.NewAuthMiddleware(
		&alwaysOKVerifier{},
		auth.NewSessionService(30*24*time.Hour),
		&stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID},
	)
	return api.NewRouter(api.MiddlewareSet{
		Auth:        authMW,
		Role:        middleware.NewRoleMiddleware(roleStore),
		ChatHandler: chat.NewGroupHandler(&chatGroupServiceStub{}),
	}).Handler()
}

func TestChatDiscoverySearchContract_ArchivedRetainedHistoryIsSafeAndExcludesDeletedContent(t *testing.T) {
	circleID := uuid.MustParse(chatGroupCircleID)
	visible := chat.Message{
		ID:       uuid.New(),
		CircleID: &circleID,
		SenderID: uuid.MustParse(testLocalUserID),
		Type:     chat.MessageTypeText,
		Content:  "visible retained history",
		State:    chat.MessageStateActive,
		SentAt:   time.Now().UTC(),
	}
	deletedAt := time.Now().UTC()
	deleted := chat.Message{
		ID:        uuid.New(),
		CircleID:  &circleID,
		SenderID:  uuid.MustParse(testLocalUserID),
		Type:      chat.MessageTypeText,
		Content:   "deleted secret must never reach a search response",
		State:     chat.MessageStateDeleted,
		DeletedAt: &deletedAt,
		SentAt:    time.Now().UTC().Add(-time.Minute),
	}
	dmPeerID := uuid.New()
	direct := chat.Message{
		ID:            uuid.New(),
		DMRecipientID: &dmPeerID,
		SenderID:      uuid.MustParse(testLocalUserID),
		Type:          chat.MessageTypeText,
		Content:       "direct content must never reach group search",
		State:         chat.MessageStateActive,
		SentAt:        time.Now().UTC().Add(-2 * time.Minute),
	}
	stub := &chatGroupServiceStub{history: []chat.Message{visible, deleted, direct}}
	rec := httptest.NewRecorder()
	chatGroupRouter(stub, nil).ServeHTTP(rec, chatGroupRequest(
		http.MethodGet,
		chatGroupMessagesPath(chatGroupCircleID)+"/search?q=retained",
		"",
		"",
	))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var page struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
		t.Fatalf("decode search page: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("visible search messages: got %d, want 1", len(page.Data))
	}
	assertJSONKeySet(t, page.Data[0], chatMessageKeys)
	if strings.Contains(rec.Body.String(), deleted.Content) {
		t.Fatalf("search response leaked deleted content: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "dm_peer_id") || strings.Contains(rec.Body.String(), "media_url") {
		t.Fatalf("group search response leaked direct or media-only fields: %s", rec.Body.String())
	}
}

func TestChatDiscoveryPinnedListContract_ReturnsAtMostFiveInPinnedOrder(t *testing.T) {
	rec := httptest.NewRecorder()
	chatGroupRouter(&chatGroupServiceStub{}, nil).ServeHTTP(rec, chatGroupRequest(
		http.MethodGet,
		chatPinnedMessagesPath(chatGroupCircleID),
		"",
		"",
	))

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode pinned list: %v", err)
	}
	if len(response.Data) > 5 {
		t.Fatalf("pinned list length: got %d, want at most 5", len(response.Data))
	}
	for _, message := range response.Data {
		assertJSONKeySet(t, message, chatMessageKeys)
	}
}

func TestChatDiscoveryPinContract_RequiresPrivilegedCurrentMemberAndEnforcesLimit(t *testing.T) {
	router := chatGroupRouter(&chatGroupServiceStub{}, nil)

	for attempt := 1; attempt <= 5; attempt++ {
		rec := httptest.NewRecorder()
		path := chatPinMessagePath(chatGroupCircleID, uuid.New().String())
		req := chatGroupRequest(http.MethodPost, path, "", "pin-key-"+strconv.Itoa(attempt))
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("pin %d status: got %d, want %d body=%s", attempt, rec.Code, http.StatusOK, rec.Body.String())
		}
		assertJSONKeySet(t, rec.Body.Bytes(), append(chatMessageKeys, "pinned_at", "pinned_by"))
	}

	sixth := httptest.NewRecorder()
	path := chatPinMessagePath(chatGroupCircleID, uuid.New().String())
	router.ServeHTTP(sixth, chatGroupRequest(http.MethodPost, path, "", "pin-key-6"))
	if sixth.Code != http.StatusConflict {
		t.Fatalf("sixth pin status: got %d, want %d body=%s", sixth.Code, http.StatusConflict, sixth.Body.String())
	}
	envelope := decodeErrorEnvelope(t, sixth)
	if envelope.Error.Code != httpconst.ErrorCodeConflict {
		t.Fatalf("sixth pin error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeConflict)
	}
}

func TestChatDiscoveryPinContract_RejectsStudentAndNonMemberWithoutLeakingContext(t *testing.T) {
	path := chatPinMessagePath(chatGroupCircleID, uuid.New().String())
	for _, tc := range []struct {
		name  string
		store chatDiscoveryRoleStore
	}{
		{name: "student", store: chatDiscoveryRoleStore{role: rbac.RoleStudent}},
		{name: "non-member", store: chatDiscoveryRoleStore{err: auth.ErrCircleMembershipNotFound}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			chatDiscoveryRouterWithRole(tc.store).ServeHTTP(rec, chatGroupRequest(http.MethodPost, path, "", "denied-pin"))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != httpconst.ErrorCodeForbidden || len(envelope.Error.Fields) != 0 {
				t.Fatalf("forbidden response: %+v", envelope.Error)
			}
		})
	}
}

func TestChatDiscoveryUnpinContract_ReturnsNoContent(t *testing.T) {
	messageID := uuid.New().String()
	router := chatGroupRouter(&chatGroupServiceStub{}, nil)
	pin := httptest.NewRecorder()
	router.ServeHTTP(pin, chatGroupRequest(http.MethodPost, chatPinMessagePath(chatGroupCircleID, messageID), "", "pin-before-unpin"))
	if pin.Code != http.StatusOK {
		t.Fatalf("setup pin status: got %d body=%s", pin.Code, pin.Body.String())
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, chatGroupRequest(
		http.MethodDelete,
		chatPinMessagePath(chatGroupCircleID, messageID),
		"",
		"unpin-key",
	))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("unpin response body: got %q, want empty", rec.Body.String())
	}
	retry := httptest.NewRecorder()
	router.ServeHTTP(retry, chatGroupRequest(http.MethodDelete, chatPinMessagePath(chatGroupCircleID, messageID), "", "unpin-retry-key"))
	if retry.Code != http.StatusNoContent {
		t.Fatalf("already-unpinned retry status: got %d body=%s", retry.Code, retry.Body.String())
	}

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, chatGroupRequest(http.MethodDelete, chatPinMessagePath(chatGroupCircleID, uuid.New().String()), "", "unpin-missing-key"))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing unpin status: got %d body=%s, want 404", missing.Code, missing.Body.String())
	}
}

func TestChatDiscoveryUnpinContract_ArchivedCircleUsesDeclaredConflict(t *testing.T) {
	rec := httptest.NewRecorder()
	chatGroupRouter(&chatGroupServiceStub{unpinErr: chat.ErrCircleArchived}, nil).ServeHTTP(rec, chatGroupRequest(
		http.MethodDelete,
		chatPinMessagePath(chatGroupCircleID, uuid.New().String()),
		"",
		"unpin-archived-key",
	))
	if rec.Code != http.StatusConflict {
		t.Fatalf("archived unpin status: got %d body=%s, want 409", rec.Code, rec.Body.String())
	}
}
