package main

import apirouter "github.com/KarimMFadel/halaqaty/backend/internal/api"

const (
	routeCircleSessionsGet        = apirouter.RouteCircleSessionsGet
	routeCircleSessionsCreate     = apirouter.RouteCircleSessionsCreate
	routeSessionStart             = apirouter.RouteSessionStart
	routeSessionJoin              = apirouter.RouteSessionJoin
	routeSessionEnd               = apirouter.RouteSessionEnd
	routeSessionLock              = apirouter.RouteSessionLock
	routeSessionParticipantsGet   = apirouter.RouteSessionParticipantsGet
	routeSessionMuteAll           = apirouter.RouteSessionMuteAll
	routeSessionParticipantMute   = apirouter.RouteSessionParticipantMute
	routeSessionParticipantUnmute = apirouter.RouteSessionParticipantUnmute
	routeSessionParticipantRemove = apirouter.RouteSessionParticipantRemove
	routeRealtimeTicketsCreate    = apirouter.RouteRealtimeTicketsCreate
	routeWebhookLiveKit           = apirouter.RouteWebhookLiveKit
)
