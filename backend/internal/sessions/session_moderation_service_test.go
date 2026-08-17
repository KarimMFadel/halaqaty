package sessions

import (
	"context"
	"errors"
	"testing"
)

func (f *fakeStore) SetLock(_ context.Context, sessionID string, locked bool) (Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	s.IsLocked = locked
	return *s, nil
}

func (f *fakeStore) EndSession(_ context.Context, sessionID string, reason EndReason) (Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	s.Status, s.EndReason = SessionStatusEnded, reason
	return *s, nil
}

func (f *fakeStore) ReconnectPresence(_ context.Context, sessionID, userID string) (Session, error) {
	return f.JoinSession(context.Background(), sessionID, userID)
}

func (f *fakeStore) RemoveParticipant(_ context.Context, sessionID, userID string) (Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if f.removed[sessionID] == nil {
		f.removed[sessionID] = map[string]bool{}
	}
	f.removed[sessionID][userID] = true
	if f.present[sessionID][userID] {
		f.present[sessionID][userID] = false
		s.ParticipantCount--
	}
	return *s, nil
}

func (f *fakeStore) SetHandRaised(_ context.Context, sessionID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.present[sessionID][userID] {
		return ErrParticipantRemoved
	}
	return nil
}
func (f *fakeStore) SetHandLowered(_ context.Context, sessionID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.present[sessionID][userID] {
		return ErrParticipantRemoved
	}
	return nil
}
func (f *fakeStore) ListSessionParticipants(context.Context, string) ([]ParticipantPresence, error) {
	return []ParticipantPresence{}, nil
}

func TestModeration_AllModeratorRolesCanLockAndUnlock(t *testing.T) {
	svc, store, _, roles := newUS1Service()
	roles.roles[us1CircleID] = map[string]string{us1Teacher: roleTeacher, us1Super: roleSupervisor, us1Student: "student"}
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatal(err)
	}
	started, _, err := svc.StartSession(context.Background(), us1Teacher, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{us1Teacher, us1Super} {
		locked, err := svc.SetLock(context.Background(), actor, started.ID, true)
		if err != nil || !locked.IsLocked {
			t.Fatalf("%s lock: %v, %+v", actor, err, locked)
		}
		if _, err := svc.SetLock(context.Background(), actor, started.ID, false); err != nil {
			t.Fatalf("%s unlock: %v", actor, err)
		}
	}
	if _, err := svc.SetLock(context.Background(), us1Student, started.ID, true); !errors.Is(err, ErrModeratorRoleRequired) {
		t.Fatalf("student lock error = %v", err)
	}
	_ = store
	_ = roles
}

func TestModeration_RemovalBlocksReconnectAndHand(t *testing.T) {
	svc, store, gw, roles := newUS1Service()
	roles.roles[us1CircleID] = map[string]string{us1Teacher: roleTeacher, us1Super: roleSupervisor, us1Student: "student"}
	created, _ := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	started, _, _ := svc.StartSession(context.Background(), us1Teacher, created.ID)
	if _, _, err := svc.JoinSession(context.Background(), us1Student, started.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RemoveParticipant(context.Background(), us1Teacher, started.ID, us1Student); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.JoinSession(context.Background(), us1Student, started.ID); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("rejoin error = %v", err)
	}
	if err := svc.SetHand(context.Background(), us1Student, started.ID, true); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("hand error = %v", err)
	}
	if len(gw.closed) != 0 || store.sessions[started.ID].ParticipantCount != 1 {
		t.Fatal("removal must not close the whole room")
	}
}

func TestModeration_EndClosesProviderRoom(t *testing.T) {
	svc, _, gw, roles := newUS1Service()
	roles.roles[us1CircleID] = map[string]string{us1Teacher: roleTeacher, us1Super: roleSupervisor, us1Student: "student"}
	created, _ := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	started, _, _ := svc.StartSession(context.Background(), us1Teacher, created.ID)
	if _, err := svc.EndSession(context.Background(), us1Teacher, started.ID, EndReasonManual); err != nil {
		t.Fatal(err)
	}
	if len(gw.closed) != 1 || gw.closed[0] != started.MediaRoomRef {
		t.Fatalf("closed rooms = %v", gw.closed)
	}
}
