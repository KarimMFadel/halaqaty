package sessions

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// ListCircleSessions mirrors the discovery projection over the fake sessions.
func (f *fakeStore) ListCircleSessions(_ context.Context, circleID string) ([]Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	items := make([]Session, 0)
	for _, s := range f.sessions {
		if s.CircleID == circleID {
			items = append(items, *s)
		}
	}
	return items, nil
}

// discoverylessStore models stores that lack the optional discovery
// projection by exposing only the base Store interface.
type discoverylessStore struct{ Store }

func TestNewServiceWithRoomKey_EmptyRoomKey_Rejected(t *testing.T) {
	if _, err := NewServiceWithRoomKey(newFakeStore(), &fakeGateway{}, &fakeRoles{}, nil); err == nil {
		t.Fatal("NewServiceWithRoomKey must reject an empty room key")
	}
}

func TestNewService_LegacyConstructionWithoutRoomKey_FailsFastOnStart(t *testing.T) {
	store := newFakeStore()
	roles := &fakeRoles{roles: map[string]map[string]string{us1CircleID: {us1Teacher: roleTeacher}}}
	svc := NewService(store, &fakeGateway{}, roles)
	if svc == nil {
		t.Fatal("NewService must construct the legacy service")
	}
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}
	if _, _, err := svc.StartSession(context.Background(), us1Teacher, created.ID); err == nil || !strings.Contains(err.Error(), "room key") {
		t.Fatalf("legacy service without a room key must fail fast on start, got %v", err)
	}
}

func TestService_ListCircleSessions_StoreWithoutDiscovery_Rejected(t *testing.T) {
	roles := &fakeRoles{roles: map[string]map[string]string{us1CircleID: {us1Student: roleStudent}}}
	svc := NewService(discoverylessStore{Store: newFakeStore()}, &fakeGateway{}, roles)

	_, err := svc.ListCircleSessions(context.Background(), us1Student, us1CircleID)
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("ListCircleSessions over a discovery-less store error = %v, want not-configured", err)
	}
}

func TestService_ListCircleSessions_NonMember_Rejected(t *testing.T) {
	svc, _, _, roles := newUS1Service()
	seedUS1Roles(roles)

	if _, err := svc.ListCircleSessions(context.Background(), us1Outsider, us1CircleID); !errors.Is(err, ErrNotCircleMember) {
		t.Fatalf("non-member error = %v, want ErrNotCircleMember", err)
	}
}

func TestService_ListCircleSessions_Member_ReceivesOnlyOwnCircleSessions(t *testing.T) {
	svc, store, _, roles := newUS1Service()
	seedUS1Roles(roles)
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}
	store.seedActive(Session{
		ID: "other-circle-session", CircleID: "circle-other",
		Status: SessionStatusActive, MediaMode: MediaModeAudioOnly, MediaRoomRef: "room-other",
	})

	items, err := svc.ListCircleSessions(context.Background(), us1Student, us1CircleID)
	if err != nil {
		t.Fatalf("ListCircleSessions: %v", err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("items = %+v, want only the member's circle session %s", items, created.ID)
	}
}

func TestService_ListCircleSessions_StoreFailure_Propagates(t *testing.T) {
	svc, store, _, roles := newUS1Service()
	seedUS1Roles(roles)
	store.listErr = errors.New("db unavailable")

	if _, err := svc.ListCircleSessions(context.Background(), us1Student, us1CircleID); err == nil || !strings.Contains(err.Error(), "list circle sessions") {
		t.Fatalf("store failure error = %v, want wrapped list failure", err)
	}
}
