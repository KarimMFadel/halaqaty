package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/profile"
	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/google/uuid"
)

const (
	wiringBearerToken   = "wiring-bearer-token"
	wiringFirebaseUID   = "firebase-wiring-user"
	wiringLocalUserID   = "88888888-8888-8888-8888-888888888888"
	wiringSessionID     = "wiring-session-1"
	wiringOtherUserID   = "77777777-7777-7777-7777-777777777777"
	wiringSessionIDBad  = "wiring-session-other-user"
	wiringCircleID      = "00000000-0000-0000-0000-000000000001"
	wiringSessionIDPath = "00000000-0000-0000-0000-000000000002"
	wiringEntryID       = "00000000-0000-0000-0000-000000000003"
	wiringRequestID     = "00000000-0000-0000-0000-000000000004"
)

// wiringVerifier accepts exactly one bearer token.
type wiringVerifier struct{}

func (wiringVerifier) Verify(_ context.Context, bearerToken string) (*auth.DecodedToken, error) {
	if bearerToken != wiringBearerToken {
		return nil, errors.New("invalid token")
	}
	return &auth.DecodedToken{UID: wiringFirebaseUID, Email: "wiring@halaqaty.app"}, nil
}

// wiringSessionRepo resolves the wiring principal and one live backend session.
type wiringSessionRepo struct{}

func (wiringSessionRepo) GetByID(_ context.Context, sessionID string) (auth.Session, error) {
	switch sessionID {
	case wiringSessionID:
		return auth.Session{
			ID:             wiringSessionID,
			UserID:         wiringLocalUserID,
			LastActivityAt: time.Now().UTC(),
			ExpiresAt:      time.Now().UTC().Add(time.Hour),
		}, nil
	case wiringSessionIDBad:
		return auth.Session{
			ID:             wiringSessionIDBad,
			UserID:         wiringOtherUserID,
			LastActivityAt: time.Now().UTC(),
			ExpiresAt:      time.Now().UTC().Add(time.Hour),
		}, nil
	default:
		return auth.Session{}, auth.ErrSessionNotFound
	}
}

func (wiringSessionRepo) Touch(context.Context, string, time.Time) error { return nil }

func (wiringSessionRepo) GetLocalUserIDByFirebaseUID(_ context.Context, firebaseUID string) (string, error) {
	if firebaseUID != wiringFirebaseUID {
		return "", auth.ErrUserNotFound
	}
	return wiringLocalUserID, nil
}

type wiringRoleRepo struct{}

type wiringModerationService struct{ called bool }

func (s *wiringModerationService) Delete(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	s.called = true
	return nil
}

func (wiringRoleRepo) RoleForUserInCircle(context.Context, string, string) (string, error) {
	return "student", nil
}

func wiringAuthMiddleware() *middleware.AuthMiddleware {
	return middleware.NewAuthMiddleware(wiringVerifier{}, auth.NewSessionService(time.Hour), wiringSessionRepo{})
}

// wiringProfileHandler returns a profile handler backed by an in-memory store.
func wiringProfileHandler() *profile.Handler {
	displayName := "Wiring User"
	store := &wiringProfileStore{
		record: profile.Record{
			Profile: auth.UserProfile{
				ID:                wiringLocalUserID,
				FirebaseUID:       wiringFirebaseUID,
				DisplayName:       &displayName,
				PreferredLanguage: "ar",
				CreatedAt:         time.Now().UTC(),
			},
		},
	}
	return profile.NewHandler(profile.NewService(store))
}

type wiringProfileStore struct {
	record profile.Record
}

func (s *wiringProfileStore) GetByUserID(context.Context, string) (profile.Record, error) {
	return s.record, nil
}

func (s *wiringProfileStore) UpdateByUserID(_ context.Context, in profile.UpdateInput) error {
	s.record.CompletedAt = in.CompletedAt
	return nil
}

func wiringAuthenticatedRequest(method, path, body string) *http.Request {
	var req *http.Request
	if body == "" {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	}
	req.Header.Set(httpconst.HeaderAuthorization, "Bearer "+wiringBearerToken)
	req.Header.Set(httpconst.HeaderSessionID, wiringSessionID)
	return req
}

// fullWiringMiddlewareSet wires every optional handler so that all route
// registration branches in registerRoutes execute.
func fullWiringMiddlewareSet(authMW *middleware.AuthMiddleware, extras ...func(*MiddlewareSet)) MiddlewareSet {
	mw := MiddlewareSet{
		Auth:              authMW,
		Role:              middleware.NewRoleMiddleware(wiringRoleRepo{}),
		ProfileHandler:    wiringProfileHandler(),
		SessionHandler:    sessions.NewHandler(nil),
		RealtimeHandler:   realtime.NewHandler(nil),
		RealtimeHub:       realtime.NewHub(nil, nil),
		QueueHandler:      queue.NewHandler(nil, nil, nil, nil, nil),
		ChatHandler:       chat.NewGroupHandler(nil),
		ChatSendLimiter:   chat.NewChatSendLimiter(30),
		ChatUploadHandler: chat.NewUploadHandler(nil),
		ChatMediaHandler:  chat.NewMediaHandler(nil),
	}
	for _, apply := range extras {
		apply(&mw)
	}
	return mw
}

func TestRegisterRoutes_EveryProtectedRouteRejectsUnauthenticatedRequests(t *testing.T) {
	router := NewRouter(fullWiringMiddlewareSet(wiringAuthMiddleware()))

	health := httptest.NewRecorder()
	router.Handler().ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("GET /health: got %d, want %d", health.Code, http.StatusOK)
	}

	sessionRoutes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/v1/auth/register"},
		{http.MethodPost, "/api/v1/auth/sessions"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodGet, "/api/v1/auth/me"},
		{http.MethodPut, "/api/v1/auth/me"},
		{http.MethodPost, "/api/v1/circles"},
		{http.MethodPost, "/api/v1/circles/join"},
		{http.MethodGet, "/api/v1/circles/discover"},
		{http.MethodPost, "/api/v1/circles/" + wiringCircleID + "/join"},
		{http.MethodGet, "/api/v1/circles/" + wiringCircleID},
		{http.MethodPut, "/api/v1/circles/" + wiringCircleID},
		{http.MethodGet, "/api/v1/circles/" + wiringCircleID + "/members"},
		{http.MethodPut, "/api/v1/circles/" + wiringCircleID + "/members/" + wiringOtherUserID + "/role"},
		{http.MethodDelete, "/api/v1/circles/" + wiringCircleID + "/members/" + wiringOtherUserID},
		{http.MethodPost, "/api/v1/circles/" + wiringCircleID + "/invite-code/refresh"},
		{http.MethodDelete, "/api/v1/circles/" + wiringCircleID},
		{http.MethodGet, "/api/v1/users/search"},
		{http.MethodPost, "/api/v1/circles/" + wiringCircleID + "/sessions"},
		{http.MethodGet, "/api/v1/circles/" + wiringCircleID + "/sessions"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/start"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/join"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/end"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/lock"},
		{http.MethodGet, "/api/v1/sessions/" + wiringSessionIDPath + "/participants"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/participants/mute-all"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/participants/" + wiringOtherUserID + "/mute"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/participants/" + wiringOtherUserID + "/unmute"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/participants/" + wiringOtherUserID + "/remove"},
		{http.MethodPost, "/api/v1/realtime/tickets"},
		{http.MethodGet, "/api/v1/circles/" + wiringCircleID + "/messages"},
		{http.MethodPost, "/api/v1/circles/" + wiringCircleID + "/messages"},
		{http.MethodPost, "/api/v1/uploads/voice"},
		{http.MethodPost, "/api/v1/uploads/image"},
		{http.MethodPost, "/api/v1/uploads/file"},
		{http.MethodPost, "/api/v1/messages/" + wiringSessionIDPath + "/media-url"},
		{http.MethodGet, "/api/v1/sessions/" + wiringSessionIDPath + "/queue"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/queue/rounds"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/queue/reset"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/queue/advance"},
		{http.MethodPut, "/api/v1/sessions/" + wiringSessionIDPath + "/queue/order"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/queue/entries/" + wiringEntryID + "/move"},
		{http.MethodPut, "/api/v1/sessions/" + wiringSessionIDPath + "/queue/entries/" + wiringEntryID + "/status"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/queue/entries/" + wiringEntryID + "/grade"},
		{http.MethodPatch, "/api/v1/sessions/" + wiringSessionIDPath + "/queue/policy"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/queue/opt-out"},
		{http.MethodPost, "/api/v1/sessions/" + wiringSessionIDPath + "/queue/opt-out-requests/" + wiringRequestID + "/decision"},
	}

	for _, route := range sessionRoutes {
		route := route
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			router.Handler().ServeHTTP(rec, httptest.NewRequest(route.method, route.path, nil))

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
			var envelope phttp.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error envelope: %v body=%s", err, rec.Body.String())
			}
			if envelope.Error.Code != httpconst.ErrorCodeUnauthorized {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeUnauthorized)
			}
		})
	}
}

func TestRegisterRoutes_WiresAuthenticatedGroupMessageDelete(t *testing.T) {
	service := &wiringModerationService{}
	router := NewRouter(fullWiringMiddlewareSet(wiringAuthMiddleware(), func(mw *MiddlewareSet) {
		mw.ChatModerationHandler = chat.NewModerationHandler(service)
	}))
	req := wiringAuthenticatedRequest(http.MethodDelete, "/api/v1/circles/"+wiringCircleID+"/messages/"+wiringEntryID, "")
	req.Header.Set(httpconst.HeaderIdempotencyKey, "wiring-delete-key")
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if !service.called {
		t.Fatal("group message delete service was not called")
	}
}

func TestRegisterRoutes_RealtimeWebSocketWithoutTicketsUnavailable(t *testing.T) {
	router := NewRouter(fullWiringMiddlewareSet(wiringAuthMiddleware()))

	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestRegisterRoutes_UnconfiguredHandlersReturnTypedInternalErrors(t *testing.T) {
	router := NewRouter(fullWiringMiddlewareSet(wiringAuthMiddleware(), func(mw *MiddlewareSet) {
		mw.ProfileHandler = nil
		mw.RBACHandler = nil
		mw.AuthHandler = nil
		mw.SessionHandler = nil
	}))

	cases := []struct {
		name        string
		method      string
		path        string
		body        string
		wantMessage string
	}{
		{
			name:        "profile route without handler reports profile handler not configured",
			method:      http.MethodGet,
			path:        "/api/v1/auth/me",
			wantMessage: httpconst.ErrorMessageProfileHandlerNotConfigured,
		},
		{
			name:        "register without auth handler reports auth handler not configured",
			method:      http.MethodPost,
			path:        "/api/v1/auth/register",
			wantMessage: httpconst.ErrorMessageAuthHandlerNotConfigured,
		},
		{
			name:        "circle creation without rbac handler reports rbac handler not configured",
			method:      http.MethodPost,
			path:        "/api/v1/circles",
			body:        `{"name":"Wiring Circle"}`,
			wantMessage: httpconst.ErrorMessageRBACHandlerNotConfigured,
		},
		{
			name:        "chat list without service reports internal server error",
			method:      http.MethodGet,
			path:        "/api/v1/circles/" + wiringCircleID + "/messages",
			wantMessage: httpconst.ErrorMessageInternalServerError,
		},
		{
			name:        "chat upload without service reports internal server error",
			method:      http.MethodPost,
			path:        "/api/v1/uploads/voice",
			wantMessage: httpconst.ErrorMessageInternalServerError,
		},
		{
			name:        "media renewal without service reports internal server error",
			method:      http.MethodPost,
			path:        "/api/v1/messages/" + wiringSessionIDPath + "/media-url",
			wantMessage: httpconst.ErrorMessageInternalServerError,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := wiringAuthenticatedRequest(tc.method, tc.path, tc.body)
			if tc.path == "/api/v1/auth/register" {
				req.Header.Del(httpconst.HeaderSessionID)
			}
			rec := httptest.NewRecorder()
			router.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusInternalServerError {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusInternalServerError, rec.Body.String())
			}
			var envelope phttp.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if envelope.Error.Code != httpconst.ErrorCodeInternalServerError {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeInternalServerError)
			}
			if envelope.Error.Message != tc.wantMessage {
				t.Fatalf("error message: got %q, want %q", envelope.Error.Message, tc.wantMessage)
			}
		})
	}
}

func TestRequireWithUserLimit_NoAuthMiddlewarePassesThrough(t *testing.T) {
	router := NewRouter(MiddlewareSet{})
	reached := false
	handler := router.requireWithUserLimit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))

	if rec.Code != http.StatusOK || !reached {
		t.Fatalf("passthrough: got status %d reached=%v, want 200 reached=true", rec.Code, reached)
	}
}

func TestRouter_AuthWithoutRateLimitStillServesRepeatedRequests(t *testing.T) {
	router := NewRouter(MiddlewareSet{Auth: wiringAuthMiddleware(), ProfileHandler: wiringProfileHandler()})

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, wiringAuthenticatedRequest(http.MethodGet, "/api/v1/auth/me", ""))

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want %d body=%s", i+1, rec.Code, http.StatusOK, rec.Body.String())
		}
		var profileResponse auth.UserProfile
		if err := json.Unmarshal(rec.Body.Bytes(), &profileResponse); err != nil {
			t.Fatalf("decode profile response: %v", err)
		}
		if profileResponse.ID != wiringLocalUserID {
			t.Fatalf("request %d profile id: got %q, want %q", i+1, profileResponse.ID, wiringLocalUserID)
		}
	}
}

func TestRouter_PerUserRateLimitAppliesAfterAuthentication(t *testing.T) {
	router := NewRouter(MiddlewareSet{
		Auth:           wiringAuthMiddleware(),
		RateLimit:      middleware.NewRateLimitMiddleware(0, 1),
		ProfileHandler: wiringProfileHandler(),
	})

	first := httptest.NewRecorder()
	router.Handler().ServeHTTP(first, wiringAuthenticatedRequest(http.MethodGet, "/api/v1/auth/me", ""))
	if first.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want %d body=%s", first.Code, http.StatusOK, first.Body.String())
	}
	var profileResponse auth.UserProfile
	if err := json.Unmarshal(first.Body.Bytes(), &profileResponse); err != nil {
		t.Fatalf("decode profile response: %v", err)
	}
	if profileResponse.ID != wiringLocalUserID {
		t.Fatalf("profile id: got %q, want %q", profileResponse.ID, wiringLocalUserID)
	}

	second := httptest.NewRecorder()
	router.Handler().ServeHTTP(second, wiringAuthenticatedRequest(http.MethodGet, "/api/v1/auth/me", ""))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want %d body=%s", second.Code, http.StatusTooManyRequests, second.Body.String())
	}
	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(second.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != httpconst.ErrorCodeRateLimitExceeded {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeRateLimitExceeded)
	}
	if envelope.Error.Message != httpconst.ErrorMessageRateLimitExceeded {
		t.Fatalf("error message: got %q, want %q", envelope.Error.Message, httpconst.ErrorMessageRateLimitExceeded)
	}
}

func TestRouter_SessionUserMismatchIsRejectedAsUnauthorized(t *testing.T) {
	router := NewRouter(MiddlewareSet{Auth: wiringAuthMiddleware(), ProfileHandler: wiringProfileHandler()})

	req := wiringAuthenticatedRequest(http.MethodGet, "/api/v1/auth/me", "")
	req.Header.Set(httpconst.HeaderSessionID, wiringSessionIDBad)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != httpconst.ErrorCodeSessionUserMismatch {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeSessionUserMismatch)
	}
}

func TestRouter_MetricsHandler_RequiresBearerToken(t *testing.T) {
	authMetrics := new(metrics.AuthMetrics)
	authMetrics.RecordRequest(time.Millisecond)
	queueMetrics := new(metrics.QueueMetrics)
	queueMetrics.RecordOutboxParked()
	chatMetrics := new(metrics.ChatMetrics)
	chatMetrics.RecordOutbox(metrics.ChatOutboxDelivered)
	router := NewRouter(MiddlewareSet{
		Metrics:      authMetrics,
		QueueMetrics: queueMetrics,
		ChatMetrics:  chatMetrics,
		MetricsToken: "wiring-metrics-token",
	})

	t.Run("missing or wrong token is rejected", func(t *testing.T) {
		for _, token := range []string{"", "Bearer wrong-token"} {
			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			if token != "" {
				req.Header.Set(httpconst.HeaderAuthorization, token)
			}
			rec := httptest.NewRecorder()
			router.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("token %q: got %d, want %d", token, rec.Code, http.StatusUnauthorized)
			}
			var envelope phttp.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if envelope.Error.Code != httpconst.ErrorCodeUnauthorized {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeUnauthorized)
			}
		}
	})

	t.Run("valid token returns auth, queue, and chat summaries", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer wiring-metrics-token")
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		var summary struct {
			metrics.MetricsSummary
			Queue metrics.QueueMetricsSummary `json:"queue"`
			Chat  metrics.ChatMetricsSummary  `json:"chat"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &summary); err != nil {
			t.Fatalf("decode metrics response: %v", err)
		}
		if summary.RequestsTotal != 1 {
			t.Fatalf("requests total: got %d, want 1", summary.RequestsTotal)
		}
		if summary.Queue.OutboxParkedTotal != 1 {
			t.Fatalf("queue parked total: got %d, want 1", summary.Queue.OutboxParkedTotal)
		}
		if summary.Chat.Outbox[metrics.ChatOutboxDelivered] != 1 {
			t.Fatalf("chat delivered total: got %d, want 1", summary.Chat.Outbox[metrics.ChatOutboxDelivered])
		}
	})

	t.Run("nil chat metrics set does not panic", func(t *testing.T) {
		nilChatRouter := NewRouter(MiddlewareSet{
			Metrics:      new(metrics.AuthMetrics),
			QueueMetrics: new(metrics.QueueMetrics),
			MetricsToken: "wiring-metrics-token",
		})
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer wiring-metrics-token")
		rec := httptest.NewRecorder()
		nilChatRouter.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
	})
}

func TestRouter_MetricsRouteAbsentWithoutToken(t *testing.T) {
	router := NewRouter(MiddlewareSet{Metrics: new(metrics.AuthMetrics)})

	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d (metrics route must not register without a token)", rec.Code, http.StatusNotFound)
	}
}

func TestValidationMiddleware_ContentTypePolicy(t *testing.T) {
	router := NewRouter(MiddlewareSet{Auth: wiringAuthMiddleware()})

	cases := []struct {
		name        string
		method      string
		path        string
		contentType string
		wantStatus  int
	}{
		{
			name:        "post with non-json content type is rejected before auth",
			method:      http.MethodPost,
			path:        "/api/v1/auth/register",
			contentType: "text/plain",
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "patch with non-json content type is rejected",
			method:      http.MethodPatch,
			path:        "/api/v1/sessions/" + wiringSessionIDPath + "/queue/policy",
			contentType: "application/x-www-form-urlencoded",
			wantStatus:  http.StatusUnsupportedMediaType,
		},
		{
			name:        "post with json content type passes validation",
			method:      http.MethodPost,
			path:        "/api/v1/auth/register",
			contentType: httpconst.ContentTypeApplicationJSON,
			wantStatus:  http.StatusUnauthorized,
		},
		{
			name:        "post with json content type and charset passes validation",
			method:      http.MethodPost,
			path:        "/api/v1/auth/register",
			contentType: "application/json; charset=utf-8",
			wantStatus:  http.StatusUnauthorized,
		},
		{
			name:        "post without content type passes validation",
			method:      http.MethodPost,
			path:        "/api/v1/auth/register",
			contentType: "",
			wantStatus:  http.StatusUnauthorized,
		},
		{
			name:        "get with non-json content type is not validated",
			method:      http.MethodGet,
			path:        "/api/v1/auth/me",
			contentType: "text/plain",
			wantStatus:  http.StatusUnauthorized,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.contentType != "" {
				req.Header.Set(httpconst.HeaderContentType, tc.contentType)
			}
			rec := httptest.NewRecorder()
			router.Handler().ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus != http.StatusUnsupportedMediaType {
				return
			}
			var envelope phttp.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("decode error envelope: %v", err)
			}
			if envelope.Error.Code != httpconst.ErrorCodeValidationFailed {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeValidationFailed)
			}
			if envelope.Error.Message != httpconst.ErrorMessageUnsupportedContentType {
				t.Fatalf("error message: got %q, want %q", envelope.Error.Message, httpconst.ErrorMessageUnsupportedContentType)
			}
		})
	}
}
