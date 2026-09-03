//go:build integration

package sessions

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

const missingConnectionSessionUUID = "99999999-9999-4999-8999-999999999999"

// connectionCallbacks records provider callbacks and can fail each one.
type connectionCallbacks struct {
	ensured      []MediaRoomRef
	ensuredModes []MediaMode
	issuedRooms  []MediaRoomRef
	ensureErr    error
	issueErr     error
}

func (c *connectionCallbacks) ensure(_ context.Context, roomRef MediaRoomRef, mode MediaMode) error {
	c.ensured = append(c.ensured, roomRef)
	c.ensuredModes = append(c.ensuredModes, mode)
	return c.ensureErr
}

func (c *connectionCallbacks) issue(_ context.Context, roomRef MediaRoomRef, _ MediaGrants) (MediaConnection, error) {
	c.issuedRooms = append(c.issuedRooms, roomRef)
	if c.issueErr != nil {
		return MediaConnection{}, c.issueErr
	}
	return MediaConnection{
		Endpoint:   "wss://media.example.com",
		Credential: MediaCredential("cred-" + string(roomRef)),
		ExpiresAt:  time.Now().Add(time.Hour),
	}, nil
}

func fillToCapacity(t *testing.T, repo *Repository, sessionID string) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(),
		`UPDATE sessions SET participant_count = 50 WHERE id = $1::uuid`, sessionID); err != nil {
		t.Fatalf("fill session to capacity: %v", err)
	}
}

// ---- StartSessionWithConnection -------------------------------------------------

func TestSessionRepository_StartSessionWithConnection_ScheduledActivatesAndAdmitsStarter(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "swc-teacher")
	circle := seedRepoCircle(t, repo, teacher)
	created, err := repo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatalf("create ad-hoc session: %v", err)
	}

	cbs := &connectionCallbacks{}
	started, conn, err := repo.StartSessionWithConnection(ctx, created.ID, teacher, "room-swc-1",
		MediaGrants{CanPublishAudio: true}, cbs.ensure, cbs.issue)
	if err != nil {
		t.Fatalf("StartSessionWithConnection: %v", err)
	}
	if started.Status != SessionStatusActive || started.MediaRoomRef != "room-swc-1" || started.ParticipantCount != 1 {
		t.Fatalf("started state wrong: %+v", started)
	}
	if conn.Credential != "cred-room-swc-1" {
		t.Fatalf("connection = %+v, want the issued credential", conn)
	}
	if len(cbs.ensured) != 1 || cbs.ensured[0] != "room-swc-1" || cbs.ensuredModes[0] != MediaModeAudioOnly {
		t.Fatalf("ensure calls = %v/%v, want the requested audio-only room", cbs.ensured, cbs.ensuredModes)
	}
	if len(cbs.issuedRooms) != 1 || cbs.issuedRooms[0] != "room-swc-1" {
		t.Fatalf("issue calls = %v, want the persisted room ref", cbs.issuedRooms)
	}
	rows := repoPresenceRows(t, repo, created.ID)
	if len(rows) != 1 || !rows[0].IsCurrentlyPresent || rows[0].UserID != teacher {
		t.Fatalf("starter not admitted: %+v", rows)
	}
}

func TestSessionRepository_StartSessionWithConnection_MissingSession_ReturnsNotFound(t *testing.T) {
	repo := newSessionRepository(t)
	teacher := seedRepoUser(t, repo, "swc-missing")
	cbs := &connectionCallbacks{}
	_, _, err := repo.StartSessionWithConnection(context.Background(), missingConnectionSessionUUID, teacher,
		"room-x", MediaGrants{}, cbs.ensure, cbs.issue)
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("missing session error = %v, want ErrSessionNotFound", err)
	}
	if len(cbs.ensured) != 0 || len(cbs.issuedRooms) != 0 {
		t.Fatalf("no provider call may happen for a missing session: %v %v", cbs.ensured, cbs.issuedRooms)
	}
}

func TestSessionRepository_StartSessionWithConnection_EndedSession_RejectedWithoutProviderCalls(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "swc-ended")
	circle := seedRepoCircle(t, repo, teacher)
	session := startRepoSession(t, repo, circle, teacher)
	if _, err := repo.EndSession(ctx, session.ID, EndReasonManual); err != nil {
		t.Fatalf("end session: %v", err)
	}

	cbs := &connectionCallbacks{}
	_, _, err := repo.StartSessionWithConnection(ctx, session.ID, teacher, "room-ended", MediaGrants{}, cbs.ensure, cbs.issue)
	if !errors.Is(err, ErrSessionAlreadyEnded) {
		t.Fatalf("ended session error = %v, want ErrSessionAlreadyEnded", err)
	}
	if len(cbs.ensured) != 0 || len(cbs.issuedRooms) != 0 {
		t.Fatalf("no provider call may happen for an ended session: %v %v", cbs.ensured, cbs.issuedRooms)
	}
}

func TestSessionRepository_StartSessionWithConnection_ActiveWithoutRoomRef_Rejected(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "swc-noref")
	circle := seedRepoCircle(t, repo, teacher)
	session := startRepoSession(t, repo, circle, teacher)
	if _, err := repo.pool.Exec(ctx, `UPDATE sessions SET media_room_ref = NULL WHERE id = $1::uuid`, session.ID); err != nil {
		t.Fatalf("clear room ref: %v", err)
	}

	cbs := &connectionCallbacks{}
	_, _, err := repo.StartSessionWithConnection(ctx, session.ID, teacher, "room-ignored", MediaGrants{}, cbs.ensure, cbs.issue)
	if err == nil || !strings.Contains(err.Error(), "no media room reference") {
		t.Fatalf("active session without room ref error = %v, want missing-room-ref failure", err)
	}
	if len(cbs.issuedRooms) != 0 {
		t.Fatalf("no credential may be issued without a persisted room ref: %v", cbs.issuedRooms)
	}
}

func TestSessionRepository_StartSessionWithConnection_ProviderFailure_RollsBackActivation(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "swc-rollback")
	circle := seedRepoCircle(t, repo, teacher)

	cases := []struct {
		name      string
		ensureErr error
		issueErr  error
	}{
		{name: "room ensure fails", ensureErr: errors.New("provider unavailable")},
		{name: "credential issuance fails", issueErr: errors.New("provider unavailable")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			created, err := repo.CreateAdHocSession(ctx, circle, teacher)
			if err != nil {
				t.Fatalf("create ad-hoc session: %v", err)
			}
			cbs := &connectionCallbacks{ensureErr: tc.ensureErr, issueErr: tc.issueErr}
			if _, _, err := repo.StartSessionWithConnection(ctx, created.ID, teacher, "room-rb",
				MediaGrants{}, cbs.ensure, cbs.issue); err == nil {
				t.Fatal("StartSessionWithConnection must surface the provider failure")
			}
			after, err := repo.GetSession(ctx, created.ID)
			if err != nil {
				t.Fatalf("get session after failure: %v", err)
			}
			if after.Status != SessionStatusScheduled || after.MediaRoomRef != "" || after.ParticipantCount != 0 {
				t.Fatalf("provider failure must roll back every durable mutation: %+v", after)
			}
			if rows := repoPresenceRows(t, repo, created.ID); len(rows) != 0 {
				t.Fatalf("provider failure must leave no presence behind: %+v", rows)
			}
		})
	}
}

func TestSessionRepository_StartSessionWithConnection_PresentStarter_BypassesLockAndCapacity(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "swc-present")
	circle := seedRepoCircle(t, repo, teacher)
	session := startRepoSession(t, repo, circle, teacher)

	cbs := &connectionCallbacks{}
	if _, _, err := repo.StartSessionWithConnection(ctx, session.ID, teacher, session.MediaRoomRef,
		MediaGrants{}, cbs.ensure, cbs.issue); err != nil {
		t.Fatalf("first start with connection: %v", err)
	}
	if _, err := repo.SetLock(ctx, session.ID, true); err != nil {
		t.Fatalf("lock session: %v", err)
	}
	fillToCapacity(t, repo, session.ID)

	restarted, conn, err := repo.StartSessionWithConnection(ctx, session.ID, teacher, session.MediaRoomRef,
		MediaGrants{}, cbs.ensure, cbs.issue)
	if err != nil {
		t.Fatalf("restart of a present starter must bypass lock and capacity: %v", err)
	}
	if restarted.ParticipantCount != maxParticipants {
		t.Fatalf("participant count = %d, want unchanged %d", restarted.ParticipantCount, maxParticipants)
	}
	if conn.Credential == "" {
		t.Fatal("present starter must still receive a fresh credential")
	}
	if rows := repoPresenceRows(t, repo, session.ID); len(rows) != 1 {
		t.Fatalf("presence rows = %d, want the single starter row", len(rows))
	}
}

func TestSessionRepository_StartSessionWithConnection_IneligibleStarters_Rejected(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "swc-gates")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "swc-gates-member")

	t.Run("locked session rejects a brand-new starter", func(t *testing.T) {
		locked := startRepoSession(t, repo, circle, teacher)
		if _, err := repo.SetLock(ctx, locked.ID, true); err != nil {
			t.Fatalf("lock session: %v", err)
		}
		cbs := &connectionCallbacks{}
		_, _, err := repo.StartSessionWithConnection(ctx, locked.ID, member, "room-locked",
			MediaGrants{}, cbs.ensure, cbs.issue)
		if !errors.Is(err, ErrSessionLocked) {
			t.Fatalf("locked new starter error = %v, want ErrSessionLocked", err)
		}
		if len(cbs.issuedRooms) != 0 {
			t.Fatalf("no credential may be issued to a locked-out starter: %v", cbs.issuedRooms)
		}
	})

	t.Run("absent starter at capacity is rejected before issuance", func(t *testing.T) {
		capped := startRepoSession(t, repo, circle, teacher)
		if _, err := repo.JoinSession(ctx, capped.ID, member); err != nil {
			t.Fatalf("join: %v", err)
		}
		if _, err := repo.LeaveSession(ctx, capped.ID, member); err != nil {
			t.Fatalf("leave: %v", err)
		}
		fillToCapacity(t, repo, capped.ID)
		cbs := &connectionCallbacks{}
		_, _, err := repo.StartSessionWithConnection(ctx, capped.ID, member, "room-capped",
			MediaGrants{}, cbs.ensure, cbs.issue)
		if !errors.Is(err, ErrSessionFull) {
			t.Fatalf("absent starter at capacity error = %v, want ErrSessionFull", err)
		}
		if len(cbs.issuedRooms) != 0 {
			t.Fatalf("no credential may be issued at capacity: %v", cbs.issuedRooms)
		}
	})

	t.Run("removed starter is never readmitted", func(t *testing.T) {
		removed := startRepoSession(t, repo, circle, teacher)
		cbs := &connectionCallbacks{}
		if _, _, err := repo.StartSessionWithConnection(ctx, removed.ID, teacher, removed.MediaRoomRef,
			MediaGrants{}, cbs.ensure, cbs.issue); err != nil {
			t.Fatalf("admit starter: %v", err)
		}
		if _, err := repo.RemoveParticipant(ctx, removed.ID, teacher); err != nil {
			t.Fatalf("remove starter: %v", err)
		}
		_, _, err := repo.StartSessionWithConnection(ctx, removed.ID, teacher, "room-removed",
			MediaGrants{}, cbs.ensure, cbs.issue)
		if !errors.Is(err, ErrParticipantRemoved) {
			t.Fatalf("removed starter error = %v, want ErrParticipantRemoved", err)
		}
		after, err := repo.GetSession(ctx, removed.ID)
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		if after.ParticipantCount != 0 {
			t.Fatalf("rejected restart must not change the count: %+v", after)
		}
	})
}

// ---- JoinSessionWithConnection --------------------------------------------------

func TestSessionRepository_JoinSessionWithConnection_AdmitsAndIssuesPerParticipant(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "jwc-teacher")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "jwc-member")
	session := startRepoSession(t, repo, circle, teacher)

	cbs := &connectionCallbacks{}
	joined, conn, err := repo.JoinSessionWithConnection(ctx, session.ID, member, MediaGrants{}, cbs.issue)
	if err != nil {
		t.Fatalf("join with connection: %v", err)
	}
	if joined.ParticipantCount != 1 {
		t.Fatalf("count after join = %d, want 1", joined.ParticipantCount)
	}
	if conn.Credential != MediaCredential("cred-"+string(session.MediaRoomRef)) {
		t.Fatalf("connection = %+v, want a credential for the persisted room", conn)
	}

	duplicate, conn, err := repo.JoinSessionWithConnection(ctx, session.ID, member, MediaGrants{}, cbs.issue)
	if err != nil {
		t.Fatalf("duplicate join with connection: %v", err)
	}
	if duplicate.ParticipantCount != 1 {
		t.Fatalf("duplicate join changed the count: %d, want 1", duplicate.ParticipantCount)
	}
	if conn.Credential == "" {
		t.Fatal("duplicate delivery must still issue the caller a credential")
	}
	if len(cbs.issuedRooms) != 2 || cbs.issuedRooms[0] != session.MediaRoomRef || cbs.issuedRooms[1] != session.MediaRoomRef {
		t.Fatalf("issue calls = %v, want one per delivery against the persisted room", cbs.issuedRooms)
	}
}

func TestSessionRepository_JoinSessionWithConnection_NonActiveStates_Rejected(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "jwc-states")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "jwc-states-member")

	scheduled, err := repo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatalf("create scheduled session: %v", err)
	}
	ended := startRepoSession(t, repo, circle, teacher)
	if _, err := repo.EndSession(ctx, ended.ID, EndReasonManual); err != nil {
		t.Fatalf("end session: %v", err)
	}

	cases := []struct {
		name      string
		sessionID string
		wantErr   error
	}{
		{name: "scheduled", sessionID: scheduled.ID, wantErr: ErrSessionNotStartable},
		{name: "ended", sessionID: ended.ID, wantErr: ErrSessionAlreadyEnded},
		{name: "missing", sessionID: missingConnectionSessionUUID, wantErr: ErrSessionNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cbs := &connectionCallbacks{}
			_, _, err := repo.JoinSessionWithConnection(ctx, tc.sessionID, member, MediaGrants{}, cbs.issue)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("join %s session error = %v, want %v", tc.name, err, tc.wantErr)
			}
			if len(cbs.issuedRooms) != 0 {
				t.Fatalf("no credential may be issued for a %s session: %v", tc.name, cbs.issuedRooms)
			}
		})
	}
}

func TestSessionRepository_JoinSessionWithConnection_IssueFailure_LeavesNoPresence(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "jwc-issuefail")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "jwc-issuefail-member")
	session := startRepoSession(t, repo, circle, teacher)

	cbs := &connectionCallbacks{issueErr: errors.New("provider unavailable")}
	if _, _, err := repo.JoinSessionWithConnection(ctx, session.ID, member, MediaGrants{}, cbs.issue); err == nil {
		t.Fatal("join with connection must surface the issuance failure")
	}
	after, err := repo.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if after.ParticipantCount != 0 {
		t.Fatalf("issuance failure must not admit the participant: %+v", after)
	}
	if rows := repoPresenceRows(t, repo, session.ID); len(rows) != 0 {
		t.Fatalf("issuance failure must leave no presence behind: %+v", rows)
	}
}

func TestSessionRepository_JoinSessionWithConnection_GatesBeforeIssue(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "jwc-gates")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "jwc-gates-member")

	t.Run("locked session rejects a new participant before issuance", func(t *testing.T) {
		locked := startRepoSession(t, repo, circle, teacher)
		if _, err := repo.SetLock(ctx, locked.ID, true); err != nil {
			t.Fatalf("lock session: %v", err)
		}
		cbs := &connectionCallbacks{}
		_, _, err := repo.JoinSessionWithConnection(ctx, locked.ID, member, MediaGrants{}, cbs.issue)
		if !errors.Is(err, ErrSessionLocked) {
			t.Fatalf("locked new participant error = %v, want ErrSessionLocked", err)
		}
		if len(cbs.issuedRooms) != 0 {
			t.Fatalf("no credential may be issued through the lock: %v", cbs.issuedRooms)
		}
	})

	t.Run("absent participant at capacity is rejected before issuance", func(t *testing.T) {
		capped := startRepoSession(t, repo, circle, teacher)
		if _, err := repo.JoinSession(ctx, capped.ID, member); err != nil {
			t.Fatalf("join: %v", err)
		}
		if _, err := repo.LeaveSession(ctx, capped.ID, member); err != nil {
			t.Fatalf("leave: %v", err)
		}
		fillToCapacity(t, repo, capped.ID)
		cbs := &connectionCallbacks{}
		_, _, err := repo.JoinSessionWithConnection(ctx, capped.ID, member, MediaGrants{}, cbs.issue)
		if !errors.Is(err, ErrSessionFull) {
			t.Fatalf("absent participant at capacity error = %v, want ErrSessionFull", err)
		}
		if len(cbs.issuedRooms) != 0 {
			t.Fatalf("no credential may be issued at capacity: %v", cbs.issuedRooms)
		}
	})

	t.Run("removed participant is rejected and not readmitted", func(t *testing.T) {
		removed := startRepoSession(t, repo, circle, teacher)
		if _, err := repo.JoinSession(ctx, removed.ID, member); err != nil {
			t.Fatalf("join: %v", err)
		}
		if _, err := repo.RemoveParticipant(ctx, removed.ID, member); err != nil {
			t.Fatalf("remove participant: %v", err)
		}
		cbs := &connectionCallbacks{}
		_, _, err := repo.JoinSessionWithConnection(ctx, removed.ID, member, MediaGrants{}, cbs.issue)
		if !errors.Is(err, ErrParticipantRemoved) {
			t.Fatalf("removed participant error = %v, want ErrParticipantRemoved", err)
		}
		after, err := repo.GetSession(ctx, removed.ID)
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		if after.ParticipantCount != 0 {
			t.Fatalf("rejected join must not change the count: %+v", after)
		}
		if rows := repoPresenceRows(t, repo, removed.ID); len(rows) != 1 || rows[0].RemovedAt == nil || rows[0].IsCurrentlyPresent {
			t.Fatalf("removal state must be preserved: %+v", rows)
		}
	})
}

func TestSessionRepository_JoinSessionWithConnection_AbsentRejoin_RestoresPresence(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "jwc-rejoin")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "jwc-rejoin-member")
	session := startRepoSession(t, repo, circle, teacher)

	if _, err := repo.JoinSession(ctx, session.ID, member); err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := repo.LeaveSession(ctx, session.ID, member); err != nil {
		t.Fatalf("leave: %v", err)
	}

	cbs := &connectionCallbacks{}
	rejoined, conn, err := repo.JoinSessionWithConnection(ctx, session.ID, member, MediaGrants{}, cbs.issue)
	if err != nil {
		t.Fatalf("rejoin with connection: %v", err)
	}
	if rejoined.ParticipantCount != 1 {
		t.Fatalf("count after rejoin = %d, want 1", rejoined.ParticipantCount)
	}
	if conn.Credential == "" {
		t.Fatal("rejoining participant must receive a credential")
	}
	rows := repoPresenceRows(t, repo, session.ID)
	if len(rows) != 1 || !rows[0].IsCurrentlyPresent || rows[0].ReconnectCount != 1 {
		t.Fatalf("rejoin must restore presence and count the reconnect: %+v", rows)
	}
}

// ---- ListCircleSessions / SetHandLowered ----------------------------------------

func TestSessionRepository_ListCircleSessions_ReturnsDiscoveryVisibleOnly(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "list-teacher")
	circle := seedRepoCircle(t, repo, teacher)

	var otherCircle string
	if err := repo.pool.QueryRow(ctx, `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('Other Circle', $1::uuid, 'HLQ-REPO02')
		RETURNING id::text
	`, teacher).Scan(&otherCircle); err != nil {
		t.Fatalf("seed other circle: %v", err)
	}

	active := startRepoSession(t, repo, circle, teacher)
	ended := startRepoSession(t, repo, circle, teacher)
	if _, err := repo.EndSession(ctx, ended.ID, EndReasonManual); err != nil {
		t.Fatalf("end session: %v", err)
	}
	scheduled, err := repo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatalf("create scheduled session: %v", err)
	}
	if _, err := repo.CreateAdHocSession(ctx, otherCircle, teacher); err != nil {
		t.Fatalf("create other-circle session: %v", err)
	}

	// Force distinct creation times so the newest-first order is deterministic.
	for _, tc := range []struct {
		sessionID string
		hoursAgo  int
	}{
		{sessionID: active.ID, hoursAgo: 3},
		{sessionID: scheduled.ID, hoursAgo: 1},
	} {
		if _, err := repo.pool.Exec(ctx,
			`UPDATE sessions SET created_at = NOW() - ($2::int * INTERVAL '1 hour') WHERE id = $1::uuid`,
			tc.sessionID, tc.hoursAgo); err != nil {
			t.Fatalf("backdate session %s: %v", tc.sessionID, err)
		}
	}

	items, err := repo.ListCircleSessions(ctx, circle)
	if err != nil {
		t.Fatalf("list circle sessions: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want only the scheduled and active sessions (ended and other-circle excluded)", len(items))
	}
	if items[0].ID != scheduled.ID || items[1].ID != active.ID {
		t.Fatalf("order = [%s %s], want newest-first [scheduled active]", items[0].ID, items[1].ID)
	}
	if items[0].MediaRoomRef != "" {
		t.Fatalf("scheduled session must not carry a room ref: %+v", items[0])
	}
	if items[1].MediaRoomRef == "" {
		t.Fatalf("active session must carry its room ref: %+v", items[1])
	}
	for _, item := range items {
		if item.CircleID != circle {
			t.Fatalf("session of another circle leaked into discovery: %+v", item)
		}
	}
}

func TestSessionRepository_SetHandLowered_IneligibleParticipants_Rejected(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "lower-teacher")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "lower-member")
	session := startRepoSession(t, repo, circle, teacher)

	if err := repo.SetHandLowered(ctx, session.ID, member); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("lower without join error = %v, want ErrParticipantRemoved", err)
	}

	if _, err := repo.JoinSession(ctx, session.ID, member); err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := repo.LeaveSession(ctx, session.ID, member); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if err := repo.SetHandLowered(ctx, session.ID, member); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("lower while absent error = %v, want ErrParticipantRemoved", err)
	}
}
