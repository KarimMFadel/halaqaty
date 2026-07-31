package main

import (
	"log/slog"
	"net/http"
	"time"

	"halaqaty/backend/internal/middleware"
	phttp "halaqaty/backend/internal/platform/http"
	"halaqaty/backend/internal/platform/httpconst"
)

// MiddlewareSet defines all cross-cutting middleware dependencies.
type MiddlewareSet struct {
	Auth      *middleware.AuthMiddleware
	Role      *middleware.RoleMiddleware
	RateLimit *middleware.RateLimitMiddleware
	Timeout   time.Duration
	Logger    *slog.Logger
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
func (r *Router) Handler() http.Handler {
	handler := http.Handler(r.mux)
	if r.mw.RateLimit != nil {
		handler = r.mw.RateLimit.Limit(handler)
	}
	handler = validationMiddleware(handler)

	logger := r.mw.Logger
	if logger == nil {
		logger = slog.Default()
	}
	handler = phttp.LoggerMiddleware(logger, handler)
	handler = phttp.RequestIDMiddleware(handler)
	handler = phttp.IdempotencyKeyMiddleware(handler)
	handler = phttp.RecoveryMiddleware(logger, handler)

	timeout := r.mw.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return http.TimeoutHandler(handler, timeout, httpconst.ErrorMessageRequestTimeout)
}

func (r *Router) registerRoutes() {
	r.mux.Handle(routeHealth, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	if r.mw.Auth != nil {
		// Registration only needs a verified Firebase token; the local user row
		// does not exist until the handler provisions it.
		r.mux.Handle(routeAuthRegister, r.mw.Auth.RequireVerifiedFirebase(http.HandlerFunc(authRegisterEndpoint)))

		// Backend session creation requires an already-registered local user.
		r.mux.Handle(routeAuthSessions, r.mw.Auth.RequireBearer(http.HandlerFunc(authSessionsEndpoint)))

		// Bearer + backend-session endpoints.
		r.mux.Handle(routeAuthLogout, r.mw.Auth.Require(http.HandlerFunc(authLogoutEndpoint)))
		r.mux.Handle(routeAuthMeGet, r.mw.Auth.Require(http.HandlerFunc(authMeEndpoint)))
		r.mux.Handle(routeAuthMePut, r.mw.Auth.Require(http.HandlerFunc(authMeEndpoint)))
	}

	if r.mw.Auth != nil && r.mw.Role != nil {
		protected := r.mw.Auth.Require(http.HandlerFunc(circleRoleAssignmentEndpoint))
		r.mux.Handle(
			routeCircleAssignRole,
			r.mw.Role.RequireAny("supervisor", "teacher")(protected),
		)
	}
}

func authRegisterEndpoint(w http.ResponseWriter, _ *http.Request) {
	phttp.WriteError(
		w,
		httpconst.ErrorCodeNotImplemented,
		httpconst.ErrorMessageAuthRegisterNotImplemented,
		http.StatusNotImplemented,
	)
}

func authSessionsEndpoint(w http.ResponseWriter, _ *http.Request) {
	phttp.WriteError(
		w,
		httpconst.ErrorCodeNotImplemented,
		httpconst.ErrorMessageAuthSessionsNotImplemented,
		http.StatusNotImplemented,
	)
}

func authLogoutEndpoint(w http.ResponseWriter, _ *http.Request) {
	phttp.WriteError(
		w,
		httpconst.ErrorCodeNotImplemented,
		httpconst.ErrorMessageAuthLogoutNotImplemented,
		http.StatusNotImplemented,
	)
}

func authMeEndpoint(w http.ResponseWriter, _ *http.Request) {
	phttp.WriteError(
		w,
		httpconst.ErrorCodeNotImplemented,
		httpconst.ErrorMessageAuthMeNotImplemented,
		http.StatusNotImplemented,
	)
}

func circleRoleAssignmentEndpoint(w http.ResponseWriter, _ *http.Request) {
	phttp.WriteError(
		w,
		httpconst.ErrorCodeNotImplemented,
		httpconst.ErrorMessageCircleRoleAssignNotImplemented,
		http.StatusNotImplemented,
	)
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
			http.Error(w, httpconst.ErrorMessageUnsupportedContentType, http.StatusUnsupportedMediaType)
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}
