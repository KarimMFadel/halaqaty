package main

const (
	routeHealth = "GET /health"
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
	routeCircleAssignRole = "PUT /api/v1/circles/{circleId}/members/{userId}/role"
)
