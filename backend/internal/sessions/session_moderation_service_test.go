package sessions

import (
	"context"
	"errors"
	"testing"
	"time"
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
	if s.Status == SessionStatusEnded {
		return Session{}, ErrSessionAlreadyEnded
	}
	s.Status, s.EndReason = SessionStatusEnded, reason
	return *s, nil
}

func (f *fakeStore) ReconnectPresence(_ context.Context, sessionID, userID string) (Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if f.removed[sessionID][userID] {
		return Session{}, ErrParticipantRemoved
	}
	if f.present[sessionID][userID] {
		return *s, nil
	}
	if _, hadPresence := f.present[sessionID][userID]; !hadPresence && s.IsLocked {
		return Session{}, ErrSessionLocked
	}
	if s.ParticipantCount >= maxParticipants {
		return Session{}, ErrSessionFull
	}
	if f.present[sessionID] == nil {
		f.present[sessionID] = map[string]bool{}
	}
	f.present[sessionID][userID] = true
	s.ParticipantCount++
	return *s, nil
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
func (f *fakeStore) ListSessionParticipants(_ context.Context, sessionID string) ([]ParticipantPresence, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	participants := make([]ParticipantPresence, 0, len(f.present[sessionID]))
	for userID, present := range f.present[sessionID] {
		if present {
			participants = append(participants, ParticipantPresence{
				SessionID: sessionID, UserID: userID, DisplayName: "Member",
				IsCurrentlyPresent: true,
			})
		}
	}
	return participants, nil
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

func TestModeration_LockBlocksReconnectThenAllowsIt(t *testing.T) {
	svc, store, _, roles := newUS1Service()
	seedUS1Roles(roles)
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatal(err)
	}
	started, _, err := svc.StartSession(context.Background(), us1Teacher, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.JoinSession(context.Background(), us1Student, started.ID); err != nil {
		t.Fatal(err)
	}
	store.markLeft(started.ID, us1Student)

	if _, err := svc.SetLock(context.Background(), us1Teacher, started.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.JoinSession(context.Background(), us1Student, started.ID); err != nil {
		t.Fatalf("eligible locked reconnect: %v", err)
	}
	if _, err := svc.SetLock(context.Background(), us1Super, started.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.JoinSession(context.Background(), us1Student, started.ID); err != nil {
		t.Fatalf("unlocked reconnect: %v", err)
	}
}

func TestModeration_MutePreservesParticipantEntitlement(t *testing.T) {
	svc, _, gateway, roles := newUS1Service()
	seedUS1Roles(roles)
	created, _ := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	started, _, _ := svc.StartSession(context.Background(), us1Teacher, created.ID)
	if _, _, err := svc.JoinSession(context.Background(), us1Student, started.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.MuteParticipant(context.Background(), us1Teacher, started.ID, us1Student); err != nil {
		t.Fatal(err)
	}
	if err := svc.UnmuteParticipant(context.Background(), us1Teacher, started.ID, us1Student); err != nil {
		t.Fatal(err)
	}
	if err := svc.MuteAll(context.Background(), us1Teacher, started.ID); err != nil {
		t.Fatal(err)
	}

	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if len(gateway.muted) != 1 || gateway.muted[0] != us1Student {
		t.Fatalf("muted = %v", gateway.muted)
	}
	if len(gateway.unmuted) != 1 || gateway.unmuted[0] != us1Student {
		t.Fatalf("unmuted = %v", gateway.unmuted)
	}
	if gateway.muteAll != 1 {
		t.Fatalf("mute-all calls = %d", gateway.muteAll)
	}
}

func TestModeration_HandStateAndEndAreIdempotent(t *testing.T) {
	svc, store, _, roles := newUS1Service()
	seedUS1Roles(roles)
	created, _ := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	started, _, _ := svc.StartSession(context.Background(), us1Teacher, created.ID)
	if _, _, err := svc.JoinSession(context.Background(), us1Student, started.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetHand(context.Background(), us1Student, started.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetHand(context.Background(), us1Student, started.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EndSession(context.Background(), us1Teacher, started.ID, EndReasonDurationLimit); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	ended := store.sessions[started.ID].Status
	store.mu.Unlock()
	if ended != SessionStatusEnded {
		t.Fatalf("status = %q, want ended", ended)
	}
	if _, err := svc.EndSession(context.Background(), us1Super, started.ID, EndReasonManual); !errors.Is(err, ErrSessionAlreadyEnded) {
		t.Fatalf("second end = %v, want ErrSessionAlreadyEnded", err)
	}
}

func TestRealtimeHandCommandEventIDIsStableAcrossDuplicateDelivery(t *testing.T) {
	svc, store, _, roles := newUS1Service()
	seedUS1Roles(roles)
	created, _ := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	started, _, _ := svc.StartSession(context.Background(), us1Teacher, created.ID)
	if _, _, err := svc.JoinSession(context.Background(), us1Student, started.ID); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.present[started.ID][us1Student] = true
	store.mu.Unlock()
	firstID, _, err := svc.HandleRealtimeCommand(context.Background(), us1Student, started.ID, "cmd.raise_hand")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Millisecond)
	secondID, _, err := svc.HandleRealtimeCommand(context.Background(), us1Student, started.ID, "cmd.raise_hand")
	if err != nil {
		t.Fatal(err)
	}
	if firstID != secondID {
		t.Fatalf("duplicate command IDs differ: %q != %q", firstID, secondID)
	}
}

func TestRealtimeSnapshotIncludesEnvelopeTimestamp(t *testing.T) {
	svc, store, _, roles := newUS1Service()
	seedUS1Roles(roles)
	created, _ := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	started, _, _ := svc.StartSession(context.Background(), us1Teacher, created.ID)
	snapshot, err := svc.RealtimeSnapshot(context.Background(), us1Teacher, started.ID)
	if err != nil {
		t.Fatal(err)
	}
	if timestamp, ok := snapshot["timestamp"].(string); !ok || timestamp == "" {
		t.Fatalf("snapshot timestamp = %v, want non-empty string", snapshot["timestamp"])
	}
	_ = store
}
