package api

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/profile"
	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// DefaultChatUploadTimeout is the F-004 US3 upload-route timeout budget
// (plan §Phase 1: 60-second upload timeout), applied in place of the global
// request timeout because a 21 MB multipart body can legitimately take
// longer than the 15-second default.
const DefaultChatUploadTimeout = 60 * time.Second

// defaultRequestTimeout is the global request timeout when MiddlewareSet
// carries none.
const defaultRequestTimeout = 15 * time.Second

// defaultMaxRequestBodyBytes is the global request-body cap for the
// JSON-based routes (1 MiB).
const defaultMaxRequestBodyBytes = 1 << 20

// MiddlewareSet defines all cross-cutting middleware dependencies.
type MiddlewareSet struct {
	Auth                  *middleware.AuthMiddleware
	Role                  *middleware.RoleMiddleware
	RateLimit             *middleware.RateLimitMiddleware
	AuthHandler           *auth.Handler
	ProfileHandler        *profile.Handler
	RBACHandler           *rbac.Handler
	SessionHandler        *sessions.Handler
	RealtimeHandler       *realtime.Handler
	RealtimeHub           *realtime.Hub
	QueueHandler          *queue.Handler
	ChatHandler           *chat.GroupHandler
	ChatModerationHandler *chat.ModerationHandler
	DirectChatHandler     *chat.DirectHandler
	ChatPresenceHandler   *chat.PresenceHandler
	ChatSendLimiter       *chat.ChatSendLimiter
	ChatUploadHandler     *chat.UploadHandler
	ChatMediaHandler      *chat.MediaHandler
	Timeout               time.Duration
	// ChatUploadTimeout overrides Timeout on the upload routes; zero selects
	// DefaultChatUploadTimeout.
	ChatUploadTimeout time.Duration
	Logger            *slog.Logger
	Metrics           *metrics.AuthMetrics
	QueueMetrics      *metrics.QueueMetrics
	ChatMetrics       *metrics.ChatMetrics
	MetricsToken      string
}

// Router wires API routes and middleware in one place.
type Router struct {
	mux *http.ServeMux
	mw  MiddlewareSet
}

// NewRouter returns a fully wired router.
func NewRouter(mw MiddlewareSet) *Router {
	router := &Router{
		mux: http.NewServeMux(),
		mw:  mw,
	}
	router.registerRoutes()
	return router
}

// Handler returns the fully wrapped HTTP handler chain.
// RateLimit.LimitByIP enforces IP budgets here (no principal yet).
// Per-user rate limiting is applied per-route inside registerRoutes, after
// auth middleware sets the principal in context.
//
// Two chains share the same core (mux + IP limiting + logging + request IDs
// + recovery): the standard chain adds the JSON content-type gate and the
// global 1 MiB body cap, while the upload chain (F-004 US3) carries the
// route-specific 21 MB multipart cap and its own timeout budget. A nested
// http.MaxBytesReader or context timeout inside the standard chain cannot
// raise the outer caps, so upload routes take their own chain.
func (r *Router) Handler() http.Handler {
	core := http.Handler(r.mux)
	// Apply IP-only rate limiting globally; per-user limiting is wired per-route.
	if r.mw.RateLimit != nil {
		core = r.mw.RateLimit.LimitByIP(core)
	}

	logger := r.mw.Logger
	if logger == nil {
		logger = slog.Default()
	}
	withObservability := func(bodyLimited http.Handler) http.Handler {
		handler := phttp.LoggerMiddleware(logger, bodyLimited)
		handler = phttp.RequestIDMiddleware(handler)
		return phttp.RecoveryMiddleware(logger, handler)
	}

	standard := withObservability(phttp.MaxBytesMiddleware(defaultMaxRequestBodyBytes, validationMiddleware(core)))
	upload := withObservability(phttp.MaxBytesMiddleware(chat.ChatUploadMaxBodyBytes, core))

	timeout := r.mw.Timeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}
	timedStandard := phttp.TimeoutMiddleware(timeout, standard)

	uploadTimeout := r.mw.ChatUploadTimeout
	if uploadTimeout <= 0 {
		uploadTimeout = DefaultChatUploadTimeout
	}
	timedUpload := phttp.TimeoutMiddleware(uploadTimeout, upload)

	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch {
		case isRealtimeWebSocketUpgrade(req):
			standard.ServeHTTP(w, req)
		case isChatUploadRequest(req):
			timedUpload.ServeHTTP(w, req)
		default:
			timedStandard.ServeHTTP(w, req)
		}
	})
}

func isRealtimeWebSocketUpgrade(req *http.Request) bool {
	return req.Method == http.MethodGet &&
		req.URL.Path == routeRealtimeWebSocketPath &&
		strings.EqualFold(req.Header.Get("Upgrade"), "websocket")
}

// isChatUploadRequest reports whether the request targets an F-004 upload
// route and therefore carries the 21 MB body cap and 60-second timeout. It
// matches the path only: unregistered methods still take the upload chain
// and fail routing, and no other route family is affected.
func isChatUploadRequest(req *http.Request) bool {
	switch req.URL.Path {
	case routeUploadsVoicePath, routeUploadsImagePath, routeUploadsFilePath:
		return true
	default:
		return false
	}
}

// requireWithUserLimit chains: auth (sets principal) → per-user rate limit → handler.
// Use this instead of Auth.Require for every protected endpoint so that the
// per-user budget is checked after the principal is known.
func (r *Router) requireWithUserLimit(next http.Handler) http.Handler {
	if r.mw.Auth == nil {
		return next
	}
	if r.mw.RateLimit == nil {
		return r.mw.Auth.Require(next)
	}
	return r.mw.Auth.Require(r.mw.RateLimit.Limit(next))
}

func (r *Router) registerRoutes() {
	r.mux.Handle(routeHealth, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	if r.mw.Metrics != nil && r.mw.MetricsToken != "" {
		r.mux.Handle(routeMetrics, r.metricsHandler())
	}

	if r.mw.Auth != nil {
		authH := r.mw.AuthHandler

		// Registration only needs a verified Firebase token; the local user row
		// does not exist until the handler provisions it.
		var registerHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageAuthHandlerNotConfigured, http.StatusInternalServerError)
		})
		if authH != nil {
			registerHandler = http.HandlerFunc(authH.Register)
		}
		r.mux.Handle(routeAuthRegister, r.mw.Auth.RequireVerifiedFirebase(registerHandler))

		// Backend session creation requires an already-registered local user.
		var sessionsHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageAuthHandlerNotConfigured, http.StatusInternalServerError)
		})
		if authH != nil {
			sessionsHandler = http.HandlerFunc(authH.CreateSession)
		}
		r.mux.Handle(routeAuthSessions, r.mw.Auth.RequireBearer(sessionsHandler))

		// Backend-session-scoped endpoints.
		var logoutHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageAuthHandlerNotConfigured, http.StatusInternalServerError)
		})
		if authH != nil {
			logoutHandler = http.HandlerFunc(authH.Logout)
		}
		r.mux.Handle(routeAuthLogout, r.requireWithUserLimit(logoutHandler))
		var profileGetHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageProfileHandlerNotConfigured, http.StatusInternalServerError)
		})
		profilePutHandler := profileGetHandler
		if r.mw.ProfileHandler != nil {
			profileGetHandler = http.HandlerFunc(r.mw.ProfileHandler.GetMe)
			profilePutHandler = http.HandlerFunc(r.mw.ProfileHandler.UpdateMe)
		}
		r.mux.Handle(routeAuthMeGet, r.requireWithUserLimit(profileGetHandler))
		r.mux.Handle(routeAuthMePut, r.requireWithUserLimit(profilePutHandler))
	}

	if r.mw.Auth != nil {
		rbacH := r.mw.RBACHandler
		var listCirclesHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		})
		if rbacH != nil {
			listCirclesHandler = http.HandlerFunc(rbacH.ListCircles)
		}
		r.mux.Handle(routeCirclesList, r.requireWithUserLimit(listCirclesHandler))
		var createCircleHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		})
		if rbacH != nil {
			createCircleHandler = http.HandlerFunc(rbacH.CreateCircle)
		}
		r.mux.Handle(routeCirclesCreate, r.requireWithUserLimit(createCircleHandler))

		var joinCircleHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		})
		if rbacH != nil {
			joinCircleHandler = http.HandlerFunc(rbacH.JoinCircle)
		}
		r.mux.Handle(routeCirclesJoin, r.requireWithUserLimit(joinCircleHandler))

		var joinPublicCircleHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		})
		if rbacH != nil {
			joinPublicCircleHandler = http.HandlerFunc(rbacH.JoinPublicCircle)
		}
		r.mux.Handle(routeCircleJoin, r.requireWithUserLimit(joinPublicCircleHandler))

		var getCircleHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		})
		if rbacH != nil {
			getCircleHandler = http.HandlerFunc(rbacH.GetCircle)
		}
		r.mux.Handle(routeCircleGet, r.requireWithUserLimit(getCircleHandler))

		var getCircleMembersHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		})
		if rbacH != nil {
			getCircleMembersHandler = http.HandlerFunc(rbacH.ListMembers)
		}
		r.mux.Handle(routeCircleMembersGet, r.requireWithUserLimit(getCircleMembersHandler))

		var discoverCirclesHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		})
		if rbacH != nil {
			discoverCirclesHandler = http.HandlerFunc(rbacH.DiscoverPublicCircles)
		}
		r.mux.Handle(routeCirclesDiscover, r.requireWithUserLimit(discoverCirclesHandler))

		var searchUsersHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		})
		if rbacH != nil {
			searchUsersHandler = http.HandlerFunc(rbacH.SearchUsers)
		}
		r.mux.Handle(routeUsersSearch, r.requireWithUserLimit(searchUsersHandler))
	}
	if r.mw.Auth != nil && r.mw.Role != nil {
		rbacH := r.mw.RBACHandler
		var assignRoleHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		})
		if rbacH != nil {
			assignRoleHandler = http.HandlerFunc(rbacH.AssignRole)
		}
		// Auth runs first so the principal exists when the role guard reads it.
		r.mux.Handle(
			routeCircleAssignRole,
			r.requireWithUserLimit(r.mw.Role.RequireAny(rbac.RoleSupervisor, rbac.RoleTeacher)(assignRoleHandler)),
		)
		var refreshInviteHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageRBACHandlerNotConfigured, http.StatusInternalServerError)
		})
		removeMemberHandler := refreshInviteHandler
		archiveCircleHandler := refreshInviteHandler
		updateCircleHandler := refreshInviteHandler
		if rbacH != nil {
			refreshInviteHandler = http.HandlerFunc(rbacH.RefreshInviteCode)
			removeMemberHandler = http.HandlerFunc(rbacH.RemoveMember)
			archiveCircleHandler = http.HandlerFunc(rbacH.ArchiveCircle)
			updateCircleHandler = http.HandlerFunc(rbacH.UpdateCircle)
		}
		r.mux.Handle(routeCircleRefreshInvite, r.requireWithUserLimit(r.mw.Role.RequireAny(rbac.RoleTeacher)(refreshInviteHandler)))
		r.mux.Handle(routeCircleRemoveMember, r.requireWithUserLimit(r.mw.Role.RequireAny(rbac.RoleTeacher)(removeMemberHandler)))
		r.mux.Handle(routeCircleArchive, r.requireWithUserLimit(r.mw.Role.RequireAny(rbac.RoleTeacher)(archiveCircleHandler)))
		r.mux.Handle(routeCircleUpdate, r.requireWithUserLimit(r.mw.Role.RequireAny(rbac.RoleTeacher)(updateCircleHandler)))
	}

	if r.mw.Auth != nil {
		var sessionHandler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			phttp.WriteError(w, httpconst.ErrorCodeInternalServerError, httpconst.ErrorMessageInternalServerError, http.StatusInternalServerError)
		})
		if r.mw.SessionHandler != nil {
			sessionHandler = r.mw.SessionHandler
		}
		r.mux.Handle(routeCircleSessionsCreate, r.requireWithUserLimit(sessionHandler))
		r.mux.Handle(routeCircleSessionsGet, r.requireWithUserLimit(sessionHandler))
		r.mux.Handle(routeSessionStart, r.requireWithUserLimit(sessionHandler))
		r.mux.Handle(routeSessionJoin, r.requireWithUserLimit(sessionHandler))
		r.mux.Handle(routeSessionEnd, r.requireWithUserLimit(sessionHandler))
		r.mux.Handle(routeSessionLock, r.requireWithUserLimit(sessionHandler))
		r.mux.Handle(routeSessionParticipantsGet, r.requireWithUserLimit(sessionHandler))
		r.mux.Handle(routeSessionMuteAll, r.requireWithUserLimit(sessionHandler))
		r.mux.Handle(routeSessionParticipantMute, r.requireWithUserLimit(sessionHandler))
		r.mux.Handle(routeSessionParticipantUnmute, r.requireWithUserLimit(sessionHandler))
		r.mux.Handle(routeSessionParticipantRemove, r.requireWithUserLimit(sessionHandler))
		if r.mw.RealtimeHandler != nil {
			r.mux.Handle(routeRealtimeTicketsCreate, r.requireWithUserLimit(http.HandlerFunc(r.mw.RealtimeHandler.CreateTicket)))
		}
		if r.mw.QueueHandler != nil {
			queueHandler := r.mw.QueueHandler
			r.mux.Handle(routeSessionQueueGet, r.requireWithUserLimit(http.HandlerFunc(queueHandler.GetQueue)))
			r.mux.Handle(routeSessionQueueRoundsCreate, r.requireWithUserLimit(http.HandlerFunc(queueHandler.CreateRound)))
			r.mux.Handle(routeSessionQueueReset, r.requireWithUserLimit(http.HandlerFunc(queueHandler.ResetQueue)))
			r.mux.Handle(routeSessionQueueAdvance, r.requireWithUserLimit(http.HandlerFunc(queueHandler.Advance)))
			r.mux.Handle(routeSessionQueueOrder, r.requireWithUserLimit(http.HandlerFunc(queueHandler.Reorder)))
			r.mux.Handle(routeSessionQueueEntryMove, r.requireWithUserLimit(http.HandlerFunc(queueHandler.MoveEntry)))
			r.mux.Handle(routeSessionQueueEntryStatus, r.requireWithUserLimit(http.HandlerFunc(queueHandler.UpdateEntryStatus)))
			r.mux.Handle(routeSessionQueueEntryGrade, r.requireWithUserLimit(http.HandlerFunc(queueHandler.GradeEntry)))
			r.mux.Handle(routeSessionQueuePolicy, r.requireWithUserLimit(http.HandlerFunc(queueHandler.UpdatePolicy)))
			r.mux.Handle(routeSessionQueueOptOut, r.requireWithUserLimit(http.HandlerFunc(queueHandler.RequestOptOut)))
			r.mux.Handle(routeSessionQueueOptOutDecision, r.requireWithUserLimit(http.HandlerFunc(queueHandler.DecideOptOutRequest)))
		}
		if r.mw.ChatHandler != nil {
			chatH := r.mw.ChatHandler
			r.mux.Handle(routeCircleMessagesGet, r.requireWithUserLimit(http.HandlerFunc(chatH.ListCircleMessages)))
			r.mux.Handle(routeCircleMessagesSearch, r.requireWithUserLimit(http.HandlerFunc(chatH.SearchCircleMessages)))
			r.mux.Handle(routeCircleMessagesPinned, r.requireWithUserLimit(http.HandlerFunc(chatH.ListPinnedCircleMessages)))
			pinHandler := http.Handler(http.HandlerFunc(chatH.PinCircleMessage))
			unpinHandler := http.Handler(http.HandlerFunc(chatH.UnpinCircleMessage))
			if r.mw.Role != nil {
				pinHandler = r.mw.Role.RequireAny(rbac.RoleTeacher, rbac.RoleSupervisor)(pinHandler)
				unpinHandler = r.mw.Role.RequireAny(rbac.RoleTeacher, rbac.RoleSupervisor)(unpinHandler)
			}
			r.mux.Handle(routeCircleMessagePin, r.requireWithUserLimit(pinHandler))
			r.mux.Handle(routeCircleMessageUnpin, r.requireWithUserLimit(unpinHandler))
			// Chat sends stack the FR-006 30-per-minute per-user-and-circle
			// fixed window on top of the generic per-user budget.
			var chatSend http.Handler = http.HandlerFunc(chatH.SendCircleMessage)
			if r.mw.ChatSendLimiter != nil {
				chatSend = r.mw.ChatSendLimiter.Limit(chatSend)
			}
			r.mux.Handle(routeCircleMessagesSend, r.requireWithUserLimit(chatSend))
			if r.mw.ChatPresenceHandler != nil {
				r.mux.Handle(routeCircleMessageRead, r.requireWithUserLimit(http.HandlerFunc(r.mw.ChatPresenceHandler.MarkCircleMessageRead)))
				r.mux.Handle(routeDirectMessageRead, r.requireWithUserLimit(http.HandlerFunc(r.mw.ChatPresenceHandler.MarkDirectMessageRead)))
			}
		}
		if r.mw.ChatModerationHandler != nil {
			r.mux.Handle(routeCircleMessageDelete, r.requireWithUserLimit(http.HandlerFunc(r.mw.ChatModerationHandler.DeleteCircleMessage)))
		}
		if r.mw.DirectChatHandler != nil {
			directH := r.mw.DirectChatHandler
			r.mux.Handle(routeDirectMessagesGet, r.requireWithUserLimit(http.HandlerFunc(directH.ListMessages)))
			var directSend http.Handler = http.HandlerFunc(directH.SendMessage)
			if r.mw.ChatSendLimiter != nil {
				directSend = r.mw.ChatSendLimiter.LimitDirect(directSend)
			}
			r.mux.Handle(routeDirectMessagesSend, r.requireWithUserLimit(directSend))
			r.mux.Handle(routeDirectMessageDelete, r.requireWithUserLimit(http.HandlerFunc(directH.DeleteMessage)))
		}
		// F-004 US3 chat media. The upload routes' 21 MB body cap and
		// 60-second timeout are selected by isChatUploadRequest in Handler;
		// the renewal route rides the standard chain.
		if r.mw.ChatUploadHandler != nil {
			uploadH := r.mw.ChatUploadHandler
			r.mux.Handle(routeUploadsVoice, r.requireWithUserLimit(http.HandlerFunc(uploadH.UploadVoice)))
			r.mux.Handle(routeUploadsImage, r.requireWithUserLimit(http.HandlerFunc(uploadH.UploadImage)))
			r.mux.Handle(routeUploadsFile, r.requireWithUserLimit(http.HandlerFunc(uploadH.UploadFile)))
		}
		if r.mw.ChatMediaHandler != nil {
			r.mux.Handle(routeMessageMediaURL, r.requireWithUserLimit(http.HandlerFunc(r.mw.ChatMediaHandler.RenewMessageMediaURL)))
		}
	}
	if r.mw.SessionHandler != nil {
		r.mux.Handle(routeWebhookLiveKit, r.mw.SessionHandler)
	}
	if r.mw.RealtimeHub != nil {
		r.mux.Handle(routeRealtimeWebSocket, r.mw.RealtimeHub)
	}
}

func (r *Router) metricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		token := req.Header.Get(httpconst.HeaderAuthorization)
		want := httpconst.AuthSchemeBearer + " " + r.mw.MetricsToken
		if subtle.ConstantTimeCompare([]byte(token), []byte(want)) != 1 {
			phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
			return
		}
		phttp.WriteJSON(w, http.StatusOK, metricsResponse{
			MetricsSummary: r.mw.Metrics.Summary(),
			Queue:          r.mw.QueueMetrics.Summary(),
			Chat:           r.mw.ChatMetrics.Summary(),
		})
	})
}

type metricsResponse struct {
	metrics.MetricsSummary
	Queue metrics.QueueMetricsSummary `json:"queue"`
	Chat  metrics.ChatMetricsSummary  `json:"chat"`
}

func validationMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			contentType := r.Header.Get(httpconst.HeaderContentType)
			if contentType == "" || httpconst.IsJSONContentType(contentType) {
				next.ServeHTTP(w, r)
				return
			}
			phttp.WriteError(
				w,
				httpconst.ErrorCodeValidationFailed,
				httpconst.ErrorMessageUnsupportedContentType,
				http.StatusUnsupportedMediaType,
			)
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}
