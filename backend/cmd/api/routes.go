package main

const (
	routeHealth = "GET /health"
)

const (
	routeAuthRegister = "POST /auth/register"
	routeAuthSessions = "POST /auth/sessions"
	routeAuthLogout   = "POST /auth/logout"
	routeAuthMeGet    = "GET /auth/me"
	routeAuthMePut    = "PUT /auth/me"
)

const (
	routeCirclesCreate    = "POST /circles"
	routeCircleAssignRole = "PUT /circles/{circleId}/members/{userId}/role"
)
