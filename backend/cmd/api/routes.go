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

	routeCircleMessagesGet    = apirouter.RouteCircleMessagesGet
	routeCircleMessagesSend   = apirouter.RouteCircleMessagesSend
	routeCircleMessagesSearch = apirouter.RouteCircleMessagesSearch
	routeCircleMessagesPinned = apirouter.RouteCircleMessagesPinned
	routeCircleMessagePin     = apirouter.RouteCircleMessagePin
	routeCircleMessageUnpin   = apirouter.RouteCircleMessageUnpin

	routeUploadsVoice    = apirouter.RouteUploadsVoice
	routeUploadsImage    = apirouter.RouteUploadsImage
	routeUploadsFile     = apirouter.RouteUploadsFile
	routeMessageMediaURL = apirouter.RouteMessageMediaURL

	routeCirclePlanningPreview       = apirouter.RouteCirclePlanningPreview
	routeCircleSchedulesGet          = apirouter.RouteCircleSchedulesGet
	routeCircleSchedulesCreate       = apirouter.RouteCircleSchedulesCreate
	routeCircleScheduleChange        = apirouter.RouteCircleScheduleChange
	routeScheduleOccurrenceChange    = apirouter.RouteScheduleOccurrenceChange
	routeScheduleOccurrenceStart     = apirouter.RouteScheduleOccurrenceStart
	routeCirclePlannedSessionsCreate = apirouter.RouteCirclePlannedSessionsCreate
	routeSessionPlannedDetailsChange = apirouter.RouteSessionPlannedDetailsChange
	routeCalendarMeGet               = apirouter.RouteCalendarMeGet
	routeSessionAttendanceGet        = apirouter.RouteSessionAttendanceGet
	routeSessionAttendanceCorrect    = apirouter.RouteSessionAttendanceCorrect
)
