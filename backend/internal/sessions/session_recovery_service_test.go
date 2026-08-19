package sessions

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// rotatingCredentialGateway models the media boundary issuing a new private
// credential for every authorized refresh.
type rotatingCredentialGateway struct {
	*fakeGateway
	issued int
}

func (g *rotatingCredentialGateway) IssueConnection(_ context.Context, _ MediaRoomRef, _ string, _ MediaGrants) (MediaConnection, error) {
	g.issued++
	return MediaConnection{
		Endpoint:   "wss://media.example.com/room",
		Credential: MediaCredential(fmt.Sprintf("fresh-credential-%d", g.issued)),
		ExpiresAt:  time.Now().Add(time.Hour),
	}, nil
}

// retryRecordingGateway observes the provider boundary across a failed start
// and its retry. It does not supply a recovery decision: the service must use
// its durable room-reference policy.
type retryRecordingGateway struct {
	*fakeGateway
	attempted []MediaRoomRef
}

func (g *retryRecordingGateway) EnsureRoom(_ context.Context, roomRef MediaRoomRef, _ MediaMode) error {
	g.attempted = append(g.attempted, roomRef)
	if len(g.attempted) == 1 {
		return errors.New("media provider unavailable")
	}
	return nil
}

func TestSessionRecovery_LockedEligibleParticipant_JoinSelectsDurableReconnect(t *testing.T) {
	store := newFakeStore()
	store.seedActive(Session{
		ID:               "recovery-session",
		CircleID:         us1CircleID,
		Status:           SessionStatusActive,
		MediaMode:        MediaModeAudioOnly,
		MediaRoomRef:     "room-recovery",
		IsLocked:         true,
		ParticipantCount: 1,
	})
	store.present["recovery-session"] = map[string]bool{us1Student: false}

	roles := &fakeRoles{roles: map[string]map[string]string{
		us1CircleID: {us1Student: roleStudent},
	}}
	service, err := NewServiceWithRoomKey(store, &fakeGateway{}, roles, []byte("test-room-key"))
	if err != nil {
		t.Fatal(err)
	}

	joined, connection, err := service.JoinSession(context.Background(), us1Student, "recovery-session")
	if err != nil {
		t.Fatalf("locked pre-lock reconnect: %v", err)
	}
	if connection.Credential == "" {
		t.Fatal("reconnect must return a newly issued media credential")
	}
	if joined.ParticipantCount != 2 {
		t.Fatalf("reconnect participant count = %d, want 2", joined.ParticipantCount)
	}
}

func TestSessionRecovery_RefreshIssuesFreshCredentialWithoutDuplicatingPresence(t *testing.T) {
	store := newFakeStore()
	store.seedActive(Session{
		ID:               "refresh-session",
		CircleID:         us1CircleID,
		Status:           SessionStatusActive,
		MediaMode:        MediaModeAudioOnly,
		MediaRoomRef:     "room-refresh",
		ParticipantCount: 1,
	})
	store.present["refresh-session"] = map[string]bool{us1Student: true}

	gateway := &rotatingCredentialGateway{fakeGateway: &fakeGateway{}}
	roles := &fakeRoles{roles: map[string]map[string]string{
		us1CircleID: {us1Student: roleStudent},
	}}
	service, err := NewServiceWithRoomKey(store, gateway, roles, []byte("test-room-key"))
	if err != nil {
		t.Fatal(err)
	}

	first, firstConnection, err := service.JoinSession(context.Background(), us1Student, "refresh-session")
	if err != nil {
		t.Fatalf("first authorized refresh: %v", err)
	}
	second, secondConnection, err := service.JoinSession(context.Background(), us1Student, "refresh-session")
	if err != nil {
		t.Fatalf("second authorized refresh: %v", err)
	}
	if firstConnection.Credential == secondConnection.Credential {
		t.Fatal("credential refresh must issue a new caller-private credential")
	}
	if first.ParticipantCount != 1 || second.ParticipantCount != 1 {
		t.Fatalf("credential refresh changed participant count: first=%d second=%d", first.ParticipantCount, second.ParticipantCount)
	}
}

func TestSessionRecovery_ProviderRetryKeepsRoomReferenceStableForReconciliation(t *testing.T) {
	store := newFakeStore()
	roles := &fakeRoles{roles: map[string]map[string]string{
		us1CircleID: {us1Teacher: roleTeacher},
	}}
	gateway := &retryRecordingGateway{fakeGateway: &fakeGateway{}}
	service, err := NewServiceWithRoomKey(store, gateway, roles, []byte("test-room-key"))
	if err != nil {
		t.Fatal(err)
	}

	created, err := service.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("create scheduled session: %v", err)
	}
	if _, _, err := service.StartSession(context.Background(), us1Teacher, created.ID); err == nil {
		t.Fatal("first start must surface the transient provider failure")
	}
	if _, _, err := service.StartSession(context.Background(), us1Teacher, created.ID); err != nil {
		t.Fatalf("retry start: %v", err)
	}
	if len(gateway.attempted) != 2 {
		t.Fatalf("provider attempts = %d, want 2", len(gateway.attempted))
	}
	if gateway.attempted[0] != gateway.attempted[1] {
		t.Fatalf("room references differ across retry: first=%q second=%q", gateway.attempted[0], gateway.attempted[1])
	}
}
