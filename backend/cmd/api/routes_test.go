package main

import "testing"

// TestF005RouteConstantsMatchContract pins every F-005 route constant to the
// method and path of its operation in the canonical OpenAPI contract
// (docs/contracts/openapi.yaml, server base /api/v1 — not the feature-local
// specs/ copy, which may lag until T055 syncs it). Wiring is T021/T034.
func TestF005RouteConstantsMatchContract(t *testing.T) {
	tests := []struct {
		operationID string
		route       string
		want        string
	}{
		{operationID: "listCircleSessions", route: routeCircleSessionsGet, want: "GET /api/v1/circles/{circleId}/sessions"},
		{operationID: "createCircleSession", route: routeCircleSessionsCreate, want: "POST /api/v1/circles/{circleId}/sessions"},
		{operationID: "startSession", route: routeSessionStart, want: "POST /api/v1/sessions/{sessionId}/start"},
		{operationID: "joinSession", route: routeSessionJoin, want: "POST /api/v1/sessions/{sessionId}/join"},
		{operationID: "endSession", route: routeSessionEnd, want: "POST /api/v1/sessions/{sessionId}/end"},
		{operationID: "setSessionLock", route: routeSessionLock, want: "POST /api/v1/sessions/{sessionId}/lock"},
		{operationID: "listSessionParticipants", route: routeSessionParticipantsGet, want: "GET /api/v1/sessions/{sessionId}/participants"},
		{operationID: "muteAllSessionParticipants", route: routeSessionMuteAll, want: "POST /api/v1/sessions/{sessionId}/participants/mute-all"},
		{operationID: "muteSessionParticipant", route: routeSessionParticipantMute, want: "POST /api/v1/sessions/{sessionId}/participants/{userId}/mute"},
		{operationID: "unmuteSessionParticipant", route: routeSessionParticipantUnmute, want: "POST /api/v1/sessions/{sessionId}/participants/{userId}/unmute"},
		{operationID: "removeSessionParticipant", route: routeSessionParticipantRemove, want: "POST /api/v1/sessions/{sessionId}/participants/{userId}/remove"},
		{operationID: "createRealtimeTicket", route: routeRealtimeTicketsCreate, want: "POST /api/v1/realtime/tickets"},
		{operationID: "receiveLiveKitWebhook", route: routeWebhookLiveKit, want: "POST /api/v1/webhooks/livekit"},
	}

	seen := make(map[string]string, len(tests))
	for _, tt := range tests {
		if tt.route != tt.want {
			t.Errorf("%s: got %q, want %q", tt.operationID, tt.route, tt.want)
		}
		if prev, dup := seen[tt.route]; dup {
			t.Errorf("duplicate route constant %q used by %s and %s", tt.route, prev, tt.operationID)
		}
		seen[tt.route] = tt.operationID
	}
	if len(tests) != 13 {
		t.Fatalf("contract coverage: got %d F-005 operations, want 13", len(tests))
	}
}
