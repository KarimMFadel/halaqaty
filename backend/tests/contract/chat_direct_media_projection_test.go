//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/api"
	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

type directMediaProjectionStub struct {
	chatGroupMediaServiceStub
	renewErr error
}

func (s *directMediaProjectionStub) SendDirectMedia(_ context.Context, input chat.SendDirectMediaInput) (chat.Message, error) {
	return chat.Message{ID: uuid.New(), SenderID: input.SenderID, DMRecipientID: &input.PeerID, Type: input.MessageType, State: chat.MessageStateActive}, nil
}

func (s *directMediaProjectionStub) RenewMediaURL(ctx context.Context, viewer, message uuid.UUID) (chat.MediaAccess, error) {
	if s.renewErr != nil {
		return chat.MediaAccess{}, s.renewErr
	}
	return s.chatGroupMediaServiceStub.RenewMediaURL(ctx, viewer, message)
}

func TestDirectContract_MediaHistoryProjectsOnlyCurrentlyAuthorizedActiveMedia(t *testing.T) {
	peer := uuid.New()
	deletedAt := time.Now().UTC()
	for _, tc := range []struct {
		name     string
		deleted  *time.Time
		renewErr error
		status   int
		wantURL  bool
	}{
		{"active media", nil, nil, http.StatusOK, true},
		{"revoked authorization", nil, chat.ErrDMNotEligible, http.StatusForbidden, false},
		{"deleted media marker", &deletedAt, chat.ErrMessageNotVisible, http.StatusOK, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := chat.Message{ID: uuid.New(), SenderID: uuid.MustParse(testLocalUserID), DMRecipientID: &peer, Type: chat.MessageTypeImage, State: chat.MessageStateActive, DeletedAt: tc.deleted}
			if tc.deleted != nil {
				message.State = chat.MessageStateDeleted
			}
			handler := chat.NewDirectHandler(&directServiceStub{eligible: true, message: message})
			handler.SetMediaService(&directMediaProjectionStub{renewErr: tc.renewErr})
			authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID})
			router := api.NewRouter(api.MiddlewareSet{Auth: authMW, DirectChatHandler: handler}).Handler()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/dm/"+peer.String(), nil)
			req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
			req.Header.Set(httpconst.HeaderSessionID, testSessionID)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.status != http.StatusOK {
				return
			}
			var page struct {
				Data []map[string]any `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
				t.Fatal(err)
			}
			if len(page.Data) != 1 {
				t.Fatalf("history=%s", rec.Body.String())
			}
			projection := page.Data[0]
			if tc.wantURL && projection["media_url"] != "https://media.example/signed" {
				t.Fatalf("missing authorized media: %s", rec.Body.String())
			}
			if !tc.wantURL && projection["media_url"] != nil {
				t.Fatalf("deleted media leaked URL: %s", rec.Body.String())
			}
			if projection["object_key"] != nil {
				t.Fatalf("private object key leaked: %s", rec.Body.String())
			}
			if projection["dm_recipient_id"] != peer.String() {
				t.Fatalf("recipient lost: %s", rec.Body.String())
			}
		})
	}
}
