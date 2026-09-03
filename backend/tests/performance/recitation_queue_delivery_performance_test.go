//go:build integration

package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// T064 / SC-008: PostgreSQL commit through the production outbox projector to
// one connected, authorized client. The measured value is recorded through the
// normal QueueMetrics histogram; this test deliberately adds no tracing path.
func TestRecitationQueueDeliveryPerformance_SC008(t *testing.T) {
	const actions = 100
	f := newSC008Fixture(t)
	ctx := context.Background()
	repo := queue.NewQueueRepository(f.pool)
	roles := sc008RoleReader{pool: f.pool}
	sessionService, err := sessions.NewServiceWithRoomKey(sessions.NewSessionRepository(f.pool), sc008NoopMediaGateway{}, roles, []byte("sc008-room-key-012345678901234567890123456789"))
	if err != nil {
		t.Fatalf("create session authorizer: %v", err)
	}
	tickets := realtime.NewTicketService(sc008CircleReader{pool: f.pool})
	hub := realtime.NewHub(tickets, sessionService)
	projector := queue.NewRealtimeOutboxProjector(repo, hub)
	hub.RegisterSessionEventProvider(projector.QueueState)
	server := httptest.NewServer(hub)
	defer server.Close()

	conn, _ := sc008Subscribe(t, ctx, server.URL, tickets, f.studentID, f.sessionID)
	defer conn.Close()
	queueMetrics := new(metrics.QueueMetrics)
	dispatcher := queue.NewOutboxDispatcher(repo, projector, queueMetrics, nil, nil, nil)
	if _, err := f.pool.Exec(ctx, `UPDATE queue_event_outbox SET delivered_at = NOW() WHERE session_id = $1::uuid`, f.sessionID); err != nil {
		t.Fatalf("clear setup outbox: %v", err)
	}

	position := 2
	for i := 0; i < actions; i++ {
		// Start before the mutation so this is a conservative upper bound: the
		// required commit-to-dispatch interval is strictly contained within it.
		started := time.Now()
		version := sc008RoundVersion(t, f.pool, f.sessionID)
		if _, err := queue.NewRoundService(repo).Move(ctx, f.entryID, version, position); err != nil {
			t.Fatalf("commit queue mutation %d: %v", i, err)
		}
		if err := dispatcher.DispatchDue(ctx, 1); err != nil {
			t.Fatalf("dispatch committed queue mutation %d: %v", i, err)
		}
		sc008ReadEvent(t, conn, realtime.EventQueueReordered)
		queueMetrics.RecordEventDeliveryLag(time.Since(started))
		if position == 2 {
			position = 1
		} else {
			position = 2
		}
	}

	// A disconnected client is excluded from the lag population. Its recovery
	// is proven by a fresh authorized subscription receiving queue.state at the
	// current durable round version (FR-009 re-fetch behavior).
	if err := conn.Close(); err != nil {
		t.Fatalf("disconnect client: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	version := sc008RoundVersion(t, f.pool, f.sessionID)
	if _, err := queue.NewRoundService(repo).Move(ctx, f.entryID, version, position); err != nil {
		t.Fatalf("commit disconnected-client mutation: %v", err)
	}
	if err := dispatcher.DispatchDue(ctx, 1); err != nil {
		t.Fatalf("dispatch disconnected-client mutation: %v", err)
	}
	currentVersion := sc008RoundVersion(t, f.pool, f.sessionID)
	recovered, state := sc008Subscribe(t, ctx, server.URL, tickets, f.studentID, f.sessionID)
	defer recovered.Close()
	if got := sc008PayloadVersion(t, state); got != currentVersion {
		t.Fatalf("reconnected queue state version=%d, want durable version=%d", got, currentVersion)
	}

	summary := queueMetrics.Summary().EventDeliveryLag
	t.Logf("SC-008 committed actions=%d connected-client p95=%dms max=%dms; disconnected delivery excluded and recovered by queue.state re-fetch", actions, summary.P95Ms, summary.MaxMs)
	if summary.P95Ms > 500 {
		t.Fatalf("SC-008 p95=%dms, want <=500ms", summary.P95Ms)
	}
}

type sc008Fixture struct {
	pool                 *pgxpool.Pool
	sessionID, studentID string
	entryID              string
}

func newSC008Fixture(t *testing.T) *sc008Fixture {
	t.Helper()
	ctx := context.Background()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set; SC-008 requires PostgreSQL")
	}
	admin, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", err)
	}
	schema := fmt.Sprintf("sc008_queue_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatalf("create test schema: %v", err)
	}
	conn, err := admin.Acquire(ctx)
	if err != nil {
		admin.Close()
		t.Fatalf("acquire migration connection: %v", err)
	}
	if _, err := conn.Exec(ctx, "SET search_path TO "+schema); err != nil {
		conn.Release()
		admin.Close()
		t.Fatalf("set migration search path: %v", err)
	}
	_, file, _, _ := runtime.Caller(0)
	migrations := filepath.Join(filepath.Dir(file), "..", "..", "migrations")
	for _, name := range []string{"000010_auth_roles_profile.up.sql", "000011_auth_roles_profile_alignment.up.sql", "000012_auth_profiles_display_name.up.sql", "000013_create_circles.up.sql", "000014_circle_members_circle_fk.up.sql", "000015_circle_management.up.sql", "000016_live_sessions.up.sql", "000017_recitation_queue_system.up.sql"} {
		sql, readErr := os.ReadFile(filepath.Join(migrations, name))
		if readErr != nil {
			conn.Release()
			admin.Close()
			t.Fatalf("read migration %s: %v", name, readErr)
		}
		if _, err := conn.Exec(ctx, string(sql)); err != nil {
			conn.Release()
			admin.Close()
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	conn.Release()
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		admin.Close()
		t.Fatalf("parse PostgreSQL config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		admin.Close()
		t.Fatalf("open schema pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		admin.Close()
	})

	teacher := sc008SeedUser(t, pool, "teacher")
	student := sc008SeedUser(t, pool, "student")
	peer := sc008SeedUser(t, pool, "peer")
	var circleID, sessionID string
	if err := pool.QueryRow(ctx, `INSERT INTO circles (name, teacher_id, invite_code) VALUES ('SC-008', $1::uuid, 'SC008-PERF') RETURNING id::text`, teacher).Scan(&circleID); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	for _, member := range []struct{ id, role string }{{teacher, "teacher"}, {student, "student"}, {peer, "student"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1::uuid, $2::uuid, $3)`, circleID, member.id, member.role); err != nil {
			t.Fatalf("seed membership: %v", err)
		}
	}
	if err := pool.QueryRow(ctx, `INSERT INTO sessions (circle_id, created_by, status, actual_start, media_room_ref) VALUES ($1::uuid, $2::uuid, 'active', NOW(), 'sc008-room') RETURNING id::text`, circleID, teacher).Scan(&sessionID); err != nil {
		t.Fatalf("seed active session: %v", err)
	}
	for _, userID := range []string{student, peer} {
		if _, err := pool.Exec(ctx, `INSERT INTO session_participant_presence (session_id, user_id, first_joined_at, last_joined_at, is_currently_present) VALUES ($1::uuid, $2::uuid, NOW(), NOW(), TRUE)`, sessionID, userID); err != nil {
			t.Fatalf("seed participant presence: %v", err)
		}
	}
	repo := queue.NewQueueRepository(pool)
	round, err := queue.NewRoundService(repo).Prepare(ctx, queue.PrepareRoundInput{SessionID: sessionID, Type: queue.RoundTypeRevision, SurahID: 1, FromAyah: 1, ToAyah: 7, SurahAyahCount: 7, CreatedBy: teacher, Preorder: []string{student, peer}})
	if err != nil {
		t.Fatalf("prepare and activate queue: %v", err)
	}
	var entryID string
	if err := pool.QueryRow(ctx, `SELECT id::text FROM recitation_queue_entries WHERE queue_id = $1::uuid ORDER BY position LIMIT 1`, round.ID).Scan(&entryID); err != nil {
		t.Fatalf("load mutable entry: %v", err)
	}
	return &sc008Fixture{pool: pool, sessionID: sessionID, studentID: student, entryID: entryID}
}

func sc008SeedUser(t *testing.T, pool *pgxpool.Pool, name string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (firebase_uid, email) VALUES ($1, $2) RETURNING id::text`, "sc008-"+name, "sc008-"+name+"@example.test").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", name, err)
	}
	return id
}

type sc008CircleReader struct{ pool *pgxpool.Pool }

func (r sc008CircleReader) ListCircleIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT circle_id::text FROM circle_members WHERE user_id = $1::uuid`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type sc008RoleReader struct{ pool *pgxpool.Pool }

func (r sc008RoleReader) RoleInCircle(ctx context.Context, circleID, userID string) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx, `SELECT role FROM circle_members WHERE circle_id = $1::uuid AND user_id = $2::uuid`, circleID, userID).Scan(&role)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return role, err
}

type sc008NoopMediaGateway struct{}

func (sc008NoopMediaGateway) EnsureRoom(context.Context, sessions.MediaRoomRef, sessions.MediaMode) error {
	return nil
}
func (sc008NoopMediaGateway) CloseRoom(context.Context, sessions.MediaRoomRef) error { return nil }
func (sc008NoopMediaGateway) IssueConnection(context.Context, sessions.MediaRoomRef, string, sessions.MediaGrants) (sessions.MediaConnection, error) {
	return sessions.MediaConnection{}, nil
}
func (sc008NoopMediaGateway) MuteParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}
func (sc008NoopMediaGateway) UnmuteParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}
func (sc008NoopMediaGateway) MuteAll(context.Context, sessions.MediaRoomRef) error { return nil }
func (sc008NoopMediaGateway) RemoveParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}

func sc008Subscribe(t *testing.T, ctx context.Context, serverURL string, tickets *realtime.TicketService, userID, sessionID string) (*websocket.Conn, map[string]any) {
	t.Helper()
	ticket, err := tickets.Issue(ctx, userID)
	if err != nil {
		t.Fatalf("issue authorized realtime ticket: %v", err)
	}
	conn, _, err := websocket.DefaultDialer.Dial(strings.Replace(serverURL, "http://", "ws://", 1)+"?token="+ticket.Token, nil)
	if err != nil {
		t.Fatalf("dial authorized realtime client: %v", err)
	}
	if err := conn.WriteJSON(map[string]any{"action": "subscribe", "topic": "session." + sessionID}); err != nil {
		conn.Close()
		t.Fatalf("subscribe authorized realtime client: %v", err)
	}
	sc008ReadEvent(t, conn, "subscribed")
	return conn, sc008ReadEvent(t, conn, realtime.EventQueueState)
}

func sc008ReadEvent(t *testing.T, conn *websocket.Conn, wantType string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set realtime read deadline: %v", err)
		}
		var event map[string]any
		if err := conn.ReadJSON(&event); err != nil {
			t.Fatalf("read realtime %s: %v", wantType, err)
		}
		if event["type"] == wantType {
			return event
		}
	}
	t.Fatalf("did not receive realtime %s", wantType)
	return nil
}

func sc008RoundVersion(t *testing.T, pool *pgxpool.Pool, sessionID string) int64 {
	t.Helper()
	var version int64
	if err := pool.QueryRow(context.Background(), `SELECT version FROM recitation_queue WHERE session_id = $1::uuid AND lifecycle = 'active'`, sessionID).Scan(&version); err != nil {
		t.Fatalf("load active round version: %v", err)
	}
	return version
}

func sc008PayloadVersion(t *testing.T, event map[string]any) int64 {
	t.Helper()
	payload, ok := event["payload"].(map[string]any)
	if !ok {
		encoded, _ := json.Marshal(event["payload"])
		t.Fatalf("queue.state payload is not an object: %s", encoded)
	}
	version, ok := payload["version"].(float64)
	if !ok {
		t.Fatalf("queue.state version has type %T", payload["version"])
	}
	return int64(version)
}
