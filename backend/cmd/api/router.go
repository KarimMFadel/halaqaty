package main

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/profile"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// MiddlewareSet defines all cross-cutting middleware dependencies.
type MiddlewareSet struct {
	Auth           *middleware.AuthMiddleware
	Role           *middleware.RoleMiddleware
	RateLimit      *middleware.RateLimitMiddleware
	AuthHandler    *auth.Handler
	ProfileHandler *profile.Handler
	RBACHandler    *rbac.Handler
	Timeout        time.Duration
	Logger         *slog.Logger
	Metrics        *metrics.AuthMetrics
	MetricsToken   string
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
func (r *Router) Handler() http.Handler {
	handler := http.Handler(r.mux)
	// Apply IP-only rate limiting globally; per-user limiting is wired per-route.
	if r.mw.RateLimit != nil {
		handler = r.mw.RateLimit.LimitByIP(handler)
	}
	handler = validationMiddleware(handler)
	// Limit request body to 1 MiB to prevent unbounded reads on auth/profile routes.
	handler = phttp.MaxBytesMiddleware(1<<20, handler)

	logger := r.mw.Logger
	if logger == nil {
		logger = slog.Default()
	}
	handler = phttp.LoggerMiddleware(logger, handler)
	handler = phttp.RequestIDMiddleware(handler)
	handler = phttp.RecoveryMiddleware(logger, handler)

	timeout := r.mw.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return phttp.TimeoutMiddleware(timeout, handler)
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
		var profilePutHandler http.Handler = profileGetHandler
		if r.mw.ProfileHandler != nil {
			profileGetHandler = http.HandlerFunc(r.mw.ProfileHandler.GetMe)
			profilePutHandler = http.HandlerFunc(r.mw.ProfileHandler.UpdateMe)
		}
		r.mux.Handle(routeAuthMeGet, r.requireWithUserLimit(profileGetHandler))
		r.mux.Handle(routeAuthMePut, r.requireWithUserLimit(profilePutHandler))
	}

	if r.mw.Auth != nil {
		rbacH := r.mw.RBACHandler
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
			r.requireWithUserLimit(r.mw.Role.RequireAny("supervisor", "teacher")(assignRoleHandler)),
		)
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
		phttp.WriteJSON(w, http.StatusOK, r.mw.Metrics.Summary())
	})
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
