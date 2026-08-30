package api

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
	routeCirclesCreate       = "POST /api/v1/circles"
	routeCirclesJoin         = "POST /api/v1/circles/join"
	routeCirclesDiscover     = "GET /api/v1/circles/discover"
	routeCircleJoin          = "POST /api/v1/circles/{circleId}/join"
	routeCircleGet           = "GET /api/v1/circles/{circleId}"
	routeCircleUpdate        = "PUT /api/v1/circles/{circleId}"
	routeCircleMembersGet    = "GET /api/v1/circles/{circleId}/members"
	routeCircleAssignRole    = "PUT /api/v1/circles/{circleId}/members/{userId}/role"
	routeCircleRemoveMember  = "DELETE /api/v1/circles/{circleId}/members/{userId}"
	routeCircleRefreshInvite = "POST /api/v1/circles/{circleId}/invite-code/refresh"
	routeCircleArchive       = "DELETE /api/v1/circles/{circleId}"
	routeUsersSearch         = "GET /api/v1/users/search"
)

// F-005 live-session route patterns. Handler wiring arrives with T021/T034;
// these constants are the single source for the route strings.
const (
	routeCircleSessionsGet        = "GET /api/v1/circles/{circleId}/sessions"
	routeCircleSessionsCreate     = "POST /api/v1/circles/{circleId}/sessions"
	routeSessionStart             = "POST /api/v1/sessions/{sessionId}/start"
	routeSessionJoin              = "POST /api/v1/sessions/{sessionId}/join"
	routeSessionEnd               = "POST /api/v1/sessions/{sessionId}/end"
	routeSessionLock              = "POST /api/v1/sessions/{sessionId}/lock"
	routeSessionParticipantsGet   = "GET /api/v1/sessions/{sessionId}/participants"
	routeSessionMuteAll           = "POST /api/v1/sessions/{sessionId}/participants/mute-all"
	routeSessionParticipantMute   = "POST /api/v1/sessions/{sessionId}/participants/{userId}/mute"
	routeSessionParticipantUnmute = "POST /api/v1/sessions/{sessionId}/participants/{userId}/unmute"
	routeSessionParticipantRemove = "POST /api/v1/sessions/{sessionId}/participants/{userId}/remove"
	routeRealtimeTicketsCreate    = "POST /api/v1/realtime/tickets"
	routeWebhookLiveKit           = "POST /api/v1/webhooks/livekit"
	routeRealtimeWebSocket        = "GET /api/v1/ws"
)

const (
	RouteCircleSessionsGet        = routeCircleSessionsGet
	RouteCircleSessionsCreate     = routeCircleSessionsCreate
	RouteSessionStart             = routeSessionStart
	RouteSessionJoin              = routeSessionJoin
	RouteSessionEnd               = routeSessionEnd
	RouteSessionLock              = routeSessionLock
	RouteSessionParticipantsGet   = routeSessionParticipantsGet
	RouteSessionMuteAll           = routeSessionMuteAll
	RouteSessionParticipantMute   = routeSessionParticipantMute
	RouteSessionParticipantUnmute = routeSessionParticipantUnmute
	RouteSessionParticipantRemove = routeSessionParticipantRemove
	RouteRealtimeTicketsCreate    = routeRealtimeTicketsCreate
	RouteWebhookLiveKit           = routeWebhookLiveKit
)

const (
	routeSessionQueueGet            = "GET /api/v1/sessions/{sessionId}/queue"
	routeSessionQueueRoundsCreate   = "POST /api/v1/sessions/{sessionId}/queue/rounds"
	routeSessionQueueReset          = "POST /api/v1/sessions/{sessionId}/queue/reset"
	routeSessionQueueAdvance        = "POST /api/v1/sessions/{sessionId}/queue/advance"
	routeSessionQueueOrder          = "PUT /api/v1/sessions/{sessionId}/queue/order"
	routeSessionQueueEntryMove      = "POST /api/v1/sessions/{sessionId}/queue/entries/{entryId}/move"
	routeSessionQueueEntryStatus    = "PUT /api/v1/sessions/{sessionId}/queue/entries/{entryId}/status"
	routeSessionQueuePolicy         = "PATCH /api/v1/sessions/{sessionId}/queue/policy"
	routeSessionQueueOptOut         = "POST /api/v1/sessions/{sessionId}/queue/opt-out"
	routeSessionQueueOptOutDecision = "POST /api/v1/sessions/{sessionId}/queue/opt-out-requests/{requestId}/decision"
)
