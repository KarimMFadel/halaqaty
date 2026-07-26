package main

import (
	"encoding/json"
	"net/http"
	"time"

	"halaqaty/backend/internal/middleware"
	"halaqaty/backend/internal/platform/httpconst"
)

// MiddlewareSet defines all cross-cutting middleware dependencies.
type MiddlewareSet struct {
	Auth      *middleware.AuthMiddleware
	Role      *middleware.RoleMiddleware
	RateLimit *middleware.RateLimitMiddleware
	Timeout   time.Duration
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
		r.mux.Handle(routeAuthMe, r.mw.Auth.Require(http.HandlerFunc(authMeEndpoint)))
	}

	if r.mw.Auth != nil && r.mw.Role != nil {
		protected := r.mw.Auth.Require(http.HandlerFunc(circleRoleAssignmentEndpoint))
		r.mux.Handle(
			routeCircleAssignRole,
			r.mw.Role.RequireAny("supervisor", "teacher")(protected),
		)
	}
}

func authMeEndpoint(w http.ResponseWriter, _ *http.Request) {
	writeNotImplementedError(
		w,
		httpconst.ErrorCodeNotImplemented,
		httpconst.ErrorMessageAuthMeNotImplemented,
	)
}

func circleRoleAssignmentEndpoint(w http.ResponseWriter, _ *http.Request) {
	writeNotImplementedError(
		w,
		httpconst.ErrorCodeNotImplemented,
		httpconst.ErrorMessageCircleRoleAssignNotImplemented,
	)
}

func writeNotImplementedError(w http.ResponseWriter, code string, message string) {
	w.Header().Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]map[string]string{
		"error": {
			"code":    code,
			"message": message,
		},
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
			http.Error(w, httpconst.ErrorMessageUnsupportedContentType, http.StatusUnsupportedMediaType)
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}
