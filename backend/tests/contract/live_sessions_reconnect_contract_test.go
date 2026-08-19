//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// T040 covers the recovery contract at the public REST/realtime boundaries.
// The authoritative sources are the canonical contracts and the F-005-local
// contract; provider failures must never expose provider material or mutate
// durable presence before a connection has been issued.

type reconnectContractGateway struct {
	issueErr error
}

func (g *reconnectContractGateway) EnsureRoom(context.Context, sessions.MediaRoomRef, sessions.MediaMode) error {
	return nil
}

func (g *reconnectContractGateway) CloseRoom(context.Context, sessions.MediaRoomRef) error {
	return nil
}

func (g *reconnectContractGateway) IssueConnection(_ context.Context, _ sessions.MediaRoomRef, _ string, _ sessions.MediaGrants) (sessions.MediaConnection, error) {
	if g.issueErr != nil {
		return sessions.MediaConnection{}, g.issueErr
	}
	return sessions.MediaConnection{
		Endpoint:   "wss://media.test.halaqaty.app/room",
		Credential: sessions.MediaCredential("private-credential"),
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	}, nil
}

func (g *reconnectContractGateway) MuteParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}

func (g *reconnectContractGateway) UnmuteParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}

func (g *reconnectContractGateway) MuteAll(context.Context, sessions.MediaRoomRef) error { return nil }

func (g *reconnectContractGateway) RemoveParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}

func reconnectContractRequest(actor, method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	return req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: actor}))
}

func TestLiveSessionsReconnectContractArtifacts(t *testing.T) {
	canonical, err := os.ReadFile("../../../docs/contracts/openapi.yaml")
	if err != nil {
		t.Fatalf("read canonical OpenAPI: %v", err)
	}
	feature, err := os.ReadFile("../../../specs/005-live-sessions-livekit/contracts/live-sessions.openapi.yaml")
	if err != nil {
		t.Fatalf("read feature OpenAPI: %v", err)
	}
	ws, err := os.ReadFile("../../../docs/contracts/ws_events.md")
	if err != nil {
		t.Fatalf("read canonical WebSocket contract: %v", err)
	}
	featureWS, err := os.ReadFile("../../../specs/005-live-sessions-livekit/contracts/live-sessions.ws_events.md")
	if err != nil {
		t.Fatalf("read feature WebSocket contract: %v", err)
	}

	for name, contract := range map[string]string{
		"canonical OpenAPI": string(canonical),
		"feature OpenAPI":   string(feature),
	} {
		for _, required := range []string{"ERR_MEDIA_UNAVAILABLE", "503", "without a credential or presence mutation"} {
			if !strings.Contains(contract, required) {
				t.Errorf("%s is missing recovery requirement %q", name, required)
			}
		}
	}
	for _, required := range []string{"fresh realtime ticket", "authorized session participant snapshot"} {
		if !strings.Contains(string(ws), required) {
			t.Errorf("canonical WebSocket contract is missing recovery requirement %q", required)
		}
	}
	for _, required := range []string{"new ticket", "participant snapshot", "1s, 2s, and 4s", "Tap to rejoin"} {
		if !strings.Contains(string(featureWS), required) {
			t.Errorf("feature WebSocket contract is missing recovery requirement %q", required)
		}
	}
}

func TestLiveSessionsReconnectTicketAndTopicContract(t *testing.T) {
	tickets := realtime.NewTicketService(&ticketCirclesStub{circles: []string{scCircleID}})
	ticket, err := tickets.Issue(context.Background(), scStudent)
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	circleTopic, err := realtime.NewCircleTopic(scCircleID)
	if err != nil {
		t.Fatalf("new circle topic: %v", err)
	}
	sessionTopic, err := realtime.NewSessionTopic("11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatalf("new session topic: %v", err)
	}
	if !ticket.Covers(circleTopic) {
		t.Fatal("ticket must cover the caller's eligible circle topic")
	}
	if ticket.Covers(sessionTopic) {
		t.Fatal("generic ticket must not authorize a session topic before a successful join")
	}
	if strings.Contains(strings.ToLower(ticket.Token), "credential") {
		t.Fatal("ticket token must not carry media credential material")
	}
}

func TestLiveSessionsReconnectDuplicatePresenceIsIdempotent(t *testing.T) {
	store := newModStoreStub()
	roles := &sessionRoleStub{roles: map[string]map[string]string{
		scCircleID: {scTeacher: "teacher", scStudent: "student"},
	}}
	service := newLiveSessionContractService(store, &reconnectContractGateway{}, roles)
	created, err := service.CreateAdHocSession(context.Background(), scTeacher, scCircleID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, _, err := service.StartSession(context.Background(), scTeacher, created.ID); err != nil {
		t.Fatalf("start session: %v", err)
	}
	first, _, err := service.JoinSession(context.Background(), scStudent, created.ID)
	if err != nil {
		t.Fatalf("first join: %v", err)
	}
	second, _, err := service.JoinSession(context.Background(), scStudent, created.ID)
	if err != nil {
		t.Fatalf("duplicate join/reconnect: %v", err)
	}
	if second.ParticipantCount != first.ParticipantCount {
		t.Fatalf("duplicate join changed participant count: got %d, want %d", second.ParticipantCount, first.ParticipantCount)
	}
}

func TestLiveSessionsReconnectTerminalAuthorizationContract(t *testing.T) {
	store := newModStoreStub()
	roles := &sessionRoleStub{roles: map[string]map[string]string{
		scCircleID: {scTeacher: "teacher", scStudent: "student"},
	}}
	service := newLiveSessionContractService(store, &reconnectContractGateway{}, roles)
	created, err := service.CreateAdHocSession(context.Background(), scTeacher, scCircleID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, _, err := service.StartSession(context.Background(), scTeacher, created.ID); err != nil {
		t.Fatalf("start session: %v", err)
	}
	if _, _, err := service.JoinSession(context.Background(), scStudent, created.ID); err != nil {
		t.Fatalf("join session: %v", err)
	}
	if _, err := store.RemoveParticipant(context.Background(), created.ID, scStudent); err != nil {
		t.Fatalf("remove participant fixture: %v", err)
	}
	if _, _, err := service.JoinSession(context.Background(), scStudent, created.ID); !errors.Is(err, sessions.ErrParticipantRemoved) {
		t.Fatalf("removed participant reconnect error = %v, want ErrParticipantRemoved", err)
	}
}

func TestLiveSessionsReconnectProviderUnavailableIs503WithoutPresenceMutation(t *testing.T) {
	store := newModStoreStub()
	gateway := &reconnectContractGateway{}
	roles := &sessionRoleStub{roles: map[string]map[string]string{
		scCircleID: {scTeacher: "teacher", scStudent: "student"},
	}}
	service := newLiveSessionContractService(store, gateway, roles)
	created, err := service.CreateAdHocSession(context.Background(), scTeacher, scCircleID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, _, err := service.StartSession(context.Background(), scTeacher, created.ID); err != nil {
		t.Fatalf("start session: %v", err)
	}
	before, err := store.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("read session before provider failure: %v", err)
	}
	gateway.issueErr = errors.New("provider unavailable")
	handler := sessions.NewHandler(service)
	req := reconnectContractRequest(scStudent, http.MethodPost, "/api/v1/sessions/"+created.ID+"/join")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("provider unavailable status = %d, want 503: %s", rec.Code, rec.Body.String())
	}
	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != "ERR_MEDIA_UNAVAILABLE" {
		t.Fatalf("provider unavailable code = %q, want ERR_MEDIA_UNAVAILABLE", envelope.Error.Code)
	}
	after, err := store.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("read session after provider failure: %v", err)
	}
	if after.ParticipantCount != before.ParticipantCount {
		t.Fatalf("provider failure mutated participant count: before %d, after %d", before.ParticipantCount, after.ParticipantCount)
	}
}
