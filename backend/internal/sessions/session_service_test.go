package sessions

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---- test doubles -----------------------------------------------------------

// fakeStore is an in-memory Store that mirrors the durable semantics of
// *Repository: scheduled→active→ended CAS, capacity 50, lock gate, and
// idempotent duplicate joins (FR-009, FR-016).
type fakeStore struct {
	mu        sync.Mutex
	sessions  map[string]*Session
	present   map[string]map[string]bool // sessionID -> userID -> present
	removed   map[string]map[string]bool // sessionID -> userID -> durably removed
	nextID    int
	createErr error
	startErr  error
	joinErr   error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		sessions: map[string]*Session{},
		present:  map[string]map[string]bool{},
		removed:  map[string]map[string]bool{},
	}
}

func (f *fakeStore) CreateAdHocSession(_ context.Context, circleID, createdBy string) (Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return Session{}, f.createErr
	}
	f.nextID++
	id := fmt.Sprintf("session-%d", f.nextID)
	s := Session{
		ID:        id,
		CircleID:  circleID,
		CreatedBy: createdBy,
		Status:    SessionStatusScheduled,
		MediaMode: MediaModeAudioOnly,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	f.sessions[id] = &s
	return s, nil
}

func (f *fakeStore) StartSession(_ context.Context, sessionID string, roomRef MediaRoomRef) (Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.startErr != nil {
		return Session{}, f.startErr
	}
	s, ok := f.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	switch s.Status {
	case SessionStatusActive:
		return Session{}, ErrSessionAlreadyActive
	case SessionStatusEnded:
		return Session{}, ErrSessionAlreadyEnded
	}
	s.Status = SessionStatusActive
	s.MediaRoomRef = roomRef
	now := time.Now()
	s.ActualStart = &now
	s.UpdatedAt = now
	return *s, nil
}

func (f *fakeStore) JoinSession(_ context.Context, sessionID, userID string) (Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.joinErr != nil {
		return Session{}, f.joinErr
	}
	s, ok := f.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	if s.Status != SessionStatusActive {
		if s.Status == SessionStatusEnded {
			return Session{}, ErrSessionAlreadyEnded
		}
		return Session{}, ErrSessionNotStartable
	}
	if f.removed[sessionID][userID] {
		return Session{}, ErrParticipantRemoved
	}
	if f.present[sessionID][userID] {
		return *s, nil // idempotent duplicate join (FR-016)
	}
	if s.IsLocked {
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

func (f *fakeStore) GetSession(_ context.Context, sessionID string) (Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[sessionID]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return *s, nil
}

// seedActive stores a copy of an already-active session (room ref included).
func (f *fakeStore) seedActive(s Session) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := s
	f.sessions[s.ID] = &cp
}

// markLeft releases one present participant without a Store method.
func (f *fakeStore) markLeft(sessionID, userID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.sessions[sessionID]
	if f.present[sessionID][userID] {
		f.present[sessionID][userID] = false
		s.ParticipantCount--
	}
}

// fakeGateway records calls and can fail EnsureRoom/IssueConnection on demand.
type fakeGateway struct {
	mu          sync.Mutex
	ensured     []MediaRoomRef
	closed      []MediaRoomRef
	issued      []issuedCall
	ensureErr   error
	issueErr    error
	ensureCount int
	muted       []string
	unmuted     []string
	muteAll     int
	removed     []string
}

type issuedCall struct {
	roomRef MediaRoomRef
	userID  string
	grants  MediaGrants
}

func (f *fakeGateway) EnsureRoom(_ context.Context, roomRef MediaRoomRef, _ MediaMode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ensureCount++
	if f.ensureErr != nil {
		return f.ensureErr
	}
	f.ensured = append(f.ensured, roomRef)
	return nil
}

func (f *fakeGateway) CloseRoom(_ context.Context, roomRef MediaRoomRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = append(f.closed, roomRef)
	return nil
}

func (f *fakeGateway) IssueConnection(_ context.Context, roomRef MediaRoomRef, userID string, grants MediaGrants) (MediaConnection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.issueErr != nil {
		return MediaConnection{}, f.issueErr
	}
	f.issued = append(f.issued, issuedCall{roomRef: roomRef, userID: userID, grants: grants})
	return MediaConnection{
		Endpoint:   "wss://media.example.com/room",
		Credential: MediaCredential("cred-" + string(roomRef) + "-" + userID),
		ExpiresAt:  time.Now().Add(time.Hour),
	}, nil
}

func (f *fakeGateway) MuteParticipant(_ context.Context, _ MediaRoomRef, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.muted = append(f.muted, userID)
	return nil
}
func (f *fakeGateway) UnmuteParticipant(_ context.Context, _ MediaRoomRef, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unmuted = append(f.unmuted, userID)
	return nil
}

func (f *fakeGateway) MuteAll(_ context.Context, _ MediaRoomRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.muteAll++
	return nil
}
func (f *fakeGateway) RemoveParticipant(_ context.Context, _ MediaRoomRef, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removed = append(f.removed, userID)
	return nil
}

// fakeRoles maps circle memberships; "" means not a member.
type fakeRoles struct {
	roles map[string]map[string]string // circleID -> userID -> role
	err   error
}

func (f *fakeRoles) RoleInCircle(_ context.Context, circleID, userID string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.roles[circleID][userID], nil
}

// newUS1Service wires a service over fresh fakes and returns everything.
func newUS1Service() (*Service, *fakeStore, *fakeGateway, *fakeRoles) {
	store := newFakeStore()
	gw := &fakeGateway{}
	roles := &fakeRoles{roles: map[string]map[string]string{}}
	return NewService(store, gw, roles), store, gw, roles
}

const (
	us1CircleID = "circle-1"
	us1Teacher  = "teacher-1"
	us1Super    = "supervisor-1"
	us1Student  = "student-1"
	us1Outsider = "outsider-1"
)

func seedUS1Roles(roles *fakeRoles) {
	roles.roles[us1CircleID] = map[string]string{
		us1Teacher: "teacher",
		us1Super:   "supervisor",
		us1Student: "student",
	}
}

// ---- T013: create authorization ---------------------------------------------

func TestCreateAdHocSessionAuthorization(t *testing.T) {
	tests := []struct {
		name    string
		actor   string
		wantErr error
	}{
		{name: "teacher creates scheduled ad-hoc session", actor: us1Teacher},
		{name: "supervisor creates scheduled ad-hoc session", actor: us1Super},
		{name: "student denied", actor: us1Student, wantErr: ErrModeratorRoleRequired},
		{name: "non-member denied", actor: us1Outsider, wantErr: ErrNotCircleMember},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _, roles := newUS1Service()
			seedUS1Roles(roles)
			created, err := svc.CreateAdHocSession(context.Background(), tc.actor, us1CircleID)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("CreateAdHocSession(%q) error = %v, want %v", tc.actor, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("CreateAdHocSession(%q) error = %v", tc.actor, err)
			}
			if created.Status != SessionStatusScheduled {
				t.Fatalf("created status = %q, want scheduled", created.Status)
			}
			if created.MediaMode != MediaModeAudioOnly {
				t.Fatalf("created media mode = %q, want audio_only", created.MediaMode)
			}
			if created.MediaRoomRef != "" {
				t.Fatalf("created session must not carry a media room reference, got %q", created.MediaRoomRef)
			}
		})
	}
}

// ---- T013: start lifecycle ----------------------------------------------------

func TestStartSessionActivatesAndIssuesModeratorConnection(t *testing.T) {
	svc, store, gw, roles := newUS1Service()
	seedUS1Roles(roles)
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}

	started, conn, err := svc.StartSession(context.Background(), us1Teacher, created.ID)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if started.Status != SessionStatusActive {
		t.Fatalf("started status = %q, want active", started.Status)
	}
	if started.MediaRoomRef == "" {
		t.Fatal("active session must persist an opaque media room reference")
	}
	if conn.Endpoint == "" || conn.Credential == "" || conn.ExpiresAt.IsZero() {
		t.Fatalf("start must issue a complete media connection, got %+v", conn)
	}
	if got := gw.issued[0].grants; !got.CanPublishAudio {
		t.Fatalf("teacher start grants CanPublishAudio=false, want true (moderator)")
	}
	// The starter is admitted as a present participant.
	present, err := store.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if present.ParticipantCount != 1 {
		t.Fatalf("participant count after start = %d, want 1 (starter admitted)", present.ParticipantCount)
	}
}

func TestStartSessionStateErrors(t *testing.T) {
	svc, store, gw, roles := newUS1Service()
	seedUS1Roles(roles)

	ended := Session{ID: "ended-1", CircleID: us1CircleID, Status: SessionStatusEnded, MediaMode: MediaModeAudioOnly}
	store.seedActive(ended)
	if _, _, err := svc.StartSession(context.Background(), us1Teacher, ended.ID); !errors.Is(err, ErrSessionAlreadyEnded) {
		t.Fatalf("start ended session error = %v, want ErrSessionAlreadyEnded", err)
	}

	if _, _, err := svc.StartSession(context.Background(), us1Teacher, "missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("start unknown session error = %v, want ErrSessionNotFound", err)
	}
	if gw.ensureCount != 0 {
		t.Fatalf("no room may be ensured for a non-startable session, got %d calls", gw.ensureCount)
	}

	// A student or outsider can never start.
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}
	if _, _, err := svc.StartSession(context.Background(), us1Student, created.ID); !errors.Is(err, ErrModeratorRoleRequired) {
		t.Fatalf("student start error = %v, want ErrModeratorRoleRequired", err)
	}
	if _, _, err := svc.StartSession(context.Background(), us1Outsider, created.ID); !errors.Is(err, ErrNotCircleMember) {
		t.Fatalf("outsider start error = %v, want ErrNotCircleMember", err)
	}
}

func TestStartSessionEnsureRoomFailureLeavesScheduled(t *testing.T) {
	svc, store, gw, roles := newUS1Service()
	seedUS1Roles(roles)
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}
	gw.ensureErr = errors.New("provider unavailable")

	if _, _, err := svc.StartSession(context.Background(), us1Teacher, created.ID); err == nil {
		t.Fatal("StartSession must fail when the media gateway cannot ensure the room")
	}
	after, err := store.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if after.Status != SessionStatusScheduled || after.MediaRoomRef != "" {
		t.Fatalf("session must stay scheduled without a room ref after ensure failure, got status=%q ref=%q", after.Status, after.MediaRoomRef)
	}
	if len(gw.issued) != 0 {
		t.Fatalf("no connection may be issued after ensure failure, got %d", len(gw.issued))
	}
}

func TestStartSessionIdempotentRestartReusesPersistedRoom(t *testing.T) {
	svc, _, gw, roles := newUS1Service()
	seedUS1Roles(roles)
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}
	first, connA, err := svc.StartSession(context.Background(), us1Teacher, created.ID)
	if err != nil {
		t.Fatalf("first StartSession: %v", err)
	}

	second, connB, err := svc.StartSession(context.Background(), us1Super, created.ID)
	if err != nil {
		t.Fatalf("second StartSession by another moderator: %v", err)
	}
	if second.MediaRoomRef != first.MediaRoomRef {
		t.Fatalf("idempotent start must reuse the persisted room ref: %q vs %q", second.MediaRoomRef, first.MediaRoomRef)
	}
	if connA.Credential == connB.Credential {
		t.Fatal("each start must issue a fresh identity-specific credential")
	}
	if gw.ensureCount != 1 {
		t.Fatalf("restart of an active session must not ensure a new room, got %d ensures", gw.ensureCount)
	}
}

func TestStartSessionConcurrentStartsConvergeToOneRoom(t *testing.T) {
	svc, _, gw, roles := newUS1Service()
	seedUS1Roles(roles)
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}

	const starters = 4
	var wg sync.WaitGroup
	type result struct {
		roomRef MediaRoomRef
		conn    MediaConnection
		err     error
	}
	results := make([]result, starters)
	for i := 0; i < starters; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			started, conn, err := svc.StartSession(context.Background(), us1Teacher, created.ID)
			results[i] = result{roomRef: started.MediaRoomRef, conn: conn, err: err}
		}(i)
	}
	wg.Wait()

	ensuredRefs := make(map[MediaRoomRef]bool)
	gw.mu.Lock()
	for _, ref := range gw.ensured {
		ensuredRefs[ref] = true
	}
	closedRefs := append([]MediaRoomRef(nil), gw.closed...)
	issuedRefs := append([]issuedCall(nil), gw.issued...)
	gw.mu.Unlock()

	var persistedRef MediaRoomRef
	for _, r := range results {
		if r.err != nil {
			t.Fatalf("concurrent start returned error: %v", r.err)
		}
		if persistedRef == "" {
			persistedRef = r.roomRef
		}
		if r.roomRef != persistedRef {
			t.Fatalf("starters disagree on room ref: %q vs %q", r.roomRef, persistedRef)
		}
		if r.conn.Credential == "" {
			t.Fatal("every starter must receive a connection")
		}
	}
	if !ensuredRefs[persistedRef] {
		t.Fatalf("persisted room ref %q was never ensured", persistedRef)
	}
	// Every orphan room (ensured but not persisted) must be closed.
	for ref := range ensuredRefs {
		if ref == persistedRef {
			continue
		}
		found := false
		for _, closed := range closedRefs {
			if closed == ref {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("orphan room ref %q was not closed", ref)
		}
	}
	for _, call := range issuedRefs {
		if call.roomRef != persistedRef {
			t.Fatalf("connection issued against non-persisted room ref %q", call.roomRef)
		}
	}
}

// ---- T013: join ---------------------------------------------------------------

func TestJoinSessionGrantsLeastPrivilegeConnections(t *testing.T) {
	svc, _, gw, roles := newUS1Service()
	seedUS1Roles(roles)
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}
	started, _, err := svc.StartSession(context.Background(), us1Teacher, created.ID)
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Student joins: listen-only connection (constitution §IV.4).
	_, studentConn, err := svc.JoinSession(context.Background(), us1Student, created.ID)
	if err != nil {
		t.Fatalf("student JoinSession: %v", err)
	}
	if studentConn.Credential == "" {
		t.Fatal("student must receive their own connection")
	}
	var studentGrant *MediaGrants
	for _, call := range gw.issued {
		if call.userID == us1Student {
			studentGrant = &call.grants
		}
	}
	if studentGrant == nil {
		t.Fatal("student connection was never issued")
	}
	if studentGrant.CanPublishAudio {
		t.Fatal("student join must never grant audio publishing outside an F-003 reciter turn")
	}

	// Teacher joining later still gets publish rights.
	_, _, err = svc.JoinSession(context.Background(), us1Super, created.ID)
	if err != nil {
		t.Fatalf("supervisor JoinSession: %v", err)
	}
	supervisorPublish := false
	gw.mu.Lock()
	for _, call := range gw.issued {
		if call.userID == us1Super && call.grants.CanPublishAudio {
			supervisorPublish = true
		}
	}
	gw.mu.Unlock()
	if !supervisorPublish {
		t.Fatal("supervisor join must grant audio publishing")
	}
	if started.ParticipantCount == 0 {
		t.Fatal("sanity: started session must count participants")
	}
}

func TestJoinSessionAuthorizationAndState(t *testing.T) {
	svc, store, gw, roles := newUS1Service()
	seedUS1Roles(roles)
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}

	// Non-member and unknown session.
	if _, _, err := svc.JoinSession(context.Background(), us1Outsider, created.ID); !errors.Is(err, ErrNotCircleMember) {
		t.Fatalf("outsider join error = %v, want ErrNotCircleMember", err)
	}
	if _, _, err := svc.JoinSession(context.Background(), us1Student, "missing"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown session join error = %v, want ErrSessionNotFound", err)
	}
	// Scheduled session is not joinable.
	if _, _, err := svc.JoinSession(context.Background(), us1Student, created.ID); !errors.Is(err, ErrSessionNotStartable) {
		t.Fatalf("scheduled join error = %v, want ErrSessionNotStartable", err)
	}
	// Ended session is not joinable.
	ended := Session{ID: "ended-2", CircleID: us1CircleID, Status: SessionStatusEnded, MediaMode: MediaModeAudioOnly}
	store.seedActive(ended)
	if _, _, err := svc.JoinSession(context.Background(), us1Student, ended.ID); !errors.Is(err, ErrSessionAlreadyEnded) {
		t.Fatalf("ended join error = %v, want ErrSessionAlreadyEnded", err)
	}
	if len(gw.issued) != 0 {
		t.Fatalf("no connection may be issued for denied joins, got %d", len(gw.issued))
	}
}

func TestJoinSessionCapacityBoundary(t *testing.T) {
	svc, store, _, roles := newUS1Service()
	seedUS1Roles(roles)
	roles.roles[us1CircleID]["student-2"] = "student"
	roles.roles[us1CircleID]["student-3"] = "student"
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}
	if _, _, err := svc.StartSession(context.Background(), us1Teacher, created.ID); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	// Seed the room to 49 present participants; the starter is already #1,
	// so add 48 more through the store to reach 49.
	for i := 0; i < 48; i++ {
		userID := fmt.Sprintf("filler-%d", i)
		if _, err := store.JoinSession(context.Background(), created.ID, userID); err != nil {
			t.Fatalf("seed filler %d: %v", i, err)
		}
	}

	// The 50th distinct participant joins through the service.
	_, _, err = svc.JoinSession(context.Background(), us1Student, created.ID)
	if err != nil {
		t.Fatalf("50th participant join: %v", err)
	}

	// The 51st is rejected.
	if _, _, err := svc.JoinSession(context.Background(), "student-2", created.ID); !errors.Is(err, ErrSessionFull) {
		t.Fatalf("51st join error = %v, want ErrSessionFull", err)
	}

	// An already-present participant re-joins idempotently even at capacity.
	if _, _, err := svc.JoinSession(context.Background(), us1Student, created.ID); err != nil {
		t.Fatalf("idempotent re-join at capacity: %v", err)
	}

	// After one leaves, a new distinct participant may join.
	store.markLeft(created.ID, us1Student)
	if _, _, err := svc.JoinSession(context.Background(), "student-3", created.ID); err != nil {
		t.Fatalf("join after a leave freed capacity: %v", err)
	}
}

func TestJoinSessionIdempotentConnectionIssuance(t *testing.T) {
	svc, store, gw, roles := newUS1Service()
	seedUS1Roles(roles)
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}
	if _, _, err := svc.StartSession(context.Background(), us1Teacher, created.ID); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	first, connA, err := svc.JoinSession(context.Background(), us1Student, created.ID)
	if err != nil {
		t.Fatalf("first join: %v", err)
	}
	second, connB, err := svc.JoinSession(context.Background(), us1Student, created.ID)
	if err != nil {
		t.Fatalf("second join: %v", err)
	}
	if connA.Credential == "" || connB.Credential == "" {
		t.Fatal("every join must return the caller's own connection")
	}
	if second.ParticipantCount != first.ParticipantCount {
		t.Fatalf("duplicate join changed participant count: %d vs %d", second.ParticipantCount, first.ParticipantCount)
	}
	// Every join issues a connection; the fake gateway is deterministic, so
	// freshness is asserted by the issue count, not by value inequality.
	issues := 0
	gw.mu.Lock()
	for _, call := range gw.issued {
		if call.userID == us1Student {
			issues++
		}
	}
	gw.mu.Unlock()
	if issues != 2 {
		t.Fatalf("duplicate join must issue one connection per call, got %d", issues)
	}
	after, err := store.GetSession(context.Background(), created.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if after.ParticipantCount != 2 { // starter + student
		t.Fatalf("participant count after duplicate join = %d, want 2", after.ParticipantCount)
	}
}

func TestJoinSessionRemovedParticipantDenied(t *testing.T) {
	svc, store, gw, roles := newUS1Service()
	seedUS1Roles(roles)
	created, err := svc.CreateAdHocSession(context.Background(), us1Teacher, us1CircleID)
	if err != nil {
		t.Fatalf("CreateAdHocSession: %v", err)
	}
	if _, _, err := svc.StartSession(context.Background(), us1Teacher, created.ID); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if _, _, err := svc.JoinSession(context.Background(), us1Student, created.ID); err != nil {
		t.Fatalf("first join: %v", err)
	}
	// Durably remove the student directly in the store (moderation is US2).
	store.mu.Lock()
	if store.removed[created.ID] == nil {
		store.removed[created.ID] = map[string]bool{}
	}
	store.removed[created.ID][us1Student] = true
	store.mu.Unlock()

	if _, _, err := svc.JoinSession(context.Background(), us1Student, created.ID); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("removed participant join error = %v, want ErrParticipantRemoved", err)
	}
	issued := 0
	gw.mu.Lock()
	issued = len(gw.issued)
	gw.mu.Unlock()
	if issued != 2 { // starter + first student join only
		t.Fatalf("removed participant must not receive a connection, %d issued", issued)
	}
}
