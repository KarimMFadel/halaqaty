package main

const (
	routeHealth  = "GET /health"
	routeMetrics = "GET /metrics"
)

const (
	routeAuthRegister = "POST /api/v1/auth/register"
	routeAuthSessions = "POST /api/v1/auth/sessions"
	routeAuthLogout   = "POST /api/v1/auth/logout"
	routeAuthMeGet    = "GET /api/v1/auth/me"
	routeAuthMePut    = "PUT /api/v1/auth/me"
)

const (
	routeCirclesCreate    = "POST /api/v1/circles"
	routeCirclesJoin      = "POST /api/v1/circles/join"
	routeCirclesDiscover  = "GET /api/v1/circles/discover"
	routeCircleJoin       = "POST /api/v1/circles/{circleId}/join"
	routeCircleAssignRole = "PUT /api/v1/circles/{circleId}/members/{userId}/role"
	routeUsersSearch      = "GET /api/v1/users/search"
)
