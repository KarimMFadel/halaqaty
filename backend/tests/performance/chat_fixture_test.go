//go:build integration

package performance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	chatPerformanceCircleCount       = 10
	chatPerformanceUsersPerCircle    = 5
	chatPerformanceMessagesPerCircle = 10_000
	chatPerformanceDeletedPerCircle  = 1_000
)

var chatPerformanceMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
	"000013_create_circles.up.sql",
	"000014_circle_members_circle_fk.up.sql",
	"000015_circle_management.up.sql",
	"000016_live_sessions.up.sql",
	"000017_recitation_queue_system.up.sql",
	"000018_real_time_chat.up.sql",
}

type chatPerformanceFixture struct {
	pool    *pgxpool.Pool
	repo    *chat.Repository
	service *chat.GroupService
	circles []uuid.UUID
	users   [][]uuid.UUID
}

func newChatPerformanceFixture(t *testing.T) *chatPerformanceFixture {
	t.Helper()
	ctx := context.Background()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set; T078 requires PostgreSQL")
	}

	admin, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open PostgreSQL admin pool: %v", err)
	}
	schema := fmt.Sprintf("chat_perf_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatalf("create performance schema: %v", err)
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
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		conn.Release()
		admin.Close()
		t.Fatal("locate performance fixture")
	}
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	for _, migration := range chatPerformanceMigrations {
		sql, readErr := os.ReadFile(filepath.Join(migrationsDir, migration))
		if readErr != nil {
			conn.Release()
			admin.Close()
			t.Fatalf("read migration %s: %v", migration, readErr)
		}
		if _, execErr := conn.Exec(ctx, string(sql)); execErr != nil {
			conn.Release()
			admin.Close()
			t.Fatalf("apply migration %s: %v", migration, execErr)
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
		t.Fatalf("open performance schema pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		admin.Close()
	})

	fixture := &chatPerformanceFixture{pool: pool, repo: chat.NewRepository(pool)}
	fixture.seed(t)
	fixture.service = chat.NewGroupService(fixture.repo, rbac.NewRepository(pool), &metrics.ChatMetrics{}, nil)
	return fixture
}

func (f *chatPerformanceFixture) seed(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	periodOne := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	periodTwoOffset := time.Duration(chatPerformanceMessagesPerCircle/2) * time.Second

	allUsers := make([]uuid.UUID, 0, chatPerformanceCircleCount*chatPerformanceUsersPerCircle)
	for i := 0; i < chatPerformanceCircleCount*chatPerformanceUsersPerCircle; i++ {
		var id uuid.UUID
		if err := f.pool.QueryRow(ctx, `
			INSERT INTO users (firebase_uid, email)
			VALUES ($1, $2)
			RETURNING id
		`, fmt.Sprintf("chat-perf-user-%02d", i), fmt.Sprintf("chat-perf-user-%02d@example.com", i)).Scan(&id); err != nil {
			t.Fatalf("seed user %d: %v", i, err)
		}
		allUsers = append(allUsers, id)
	}

	f.circles = make([]uuid.UUID, 0, chatPerformanceCircleCount)
	f.users = make([][]uuid.UUID, 0, chatPerformanceCircleCount)
	for circleIndex := 0; circleIndex < chatPerformanceCircleCount; circleIndex++ {
		members := allUsers[circleIndex*chatPerformanceUsersPerCircle : (circleIndex+1)*chatPerformanceUsersPerCircle]
		var circleID uuid.UUID
		if err := f.pool.QueryRow(ctx, `
			INSERT INTO circles (name, teacher_id, invite_code)
			VALUES ($1, $2, $3)
			RETURNING id
		`, fmt.Sprintf("Chat performance circle %02d", circleIndex), members[0], fmt.Sprintf("CHAT-PERF-%02d", circleIndex)).Scan(&circleID); err != nil {
			t.Fatalf("seed circle %d: %v", circleIndex, err)
		}
		for memberIndex, userID := range members {
			joinedAt := periodOne
			if memberIndex == chatPerformanceUsersPerCircle-1 {
				joinedAt = periodOne.Add(periodTwoOffset)
			}
			role := "student"
			if memberIndex == 0 {
				role = "teacher"
			}
			if _, err := f.pool.Exec(ctx, `
				INSERT INTO circle_members (circle_id, user_id, role, joined_at)
				VALUES ($1, $2, $3, $4)
			`, circleID, userID, role, joinedAt); err != nil {
				t.Fatalf("seed circle %d member %d: %v", circleIndex, memberIndex, err)
			}
		}
		if _, err := f.pool.Exec(ctx, `
			INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, content, sent_at, deleted_at)
			SELECT $1::uuid,
			       members.id,
			       'chat-perf-' || $2::text || '-' || g::text,
			       'text',
			       CASE WHEN g % 20 = 1
			            THEN 'needle circle ' || $2::text || ' message ' || g::text || ' علم'
			            ELSE 'circle ' || $2::text || ' message ' || g::text || ' محفوظ'
			       END,
			       $3::timestamptz + g * INTERVAL '1 second',
			       CASE WHEN g % 10 = 0 THEN $3::timestamptz + INTERVAL '1 day' ELSE NULL END
			FROM generate_series(0, $4::integer - 1) AS series(g)
			JOIN LATERAL (
				SELECT id
				FROM unnest($5::uuid[]) WITH ORDINALITY AS member(id, ordinal)
				WHERE member.ordinal = (g % $6::integer) + 1
			) AS members ON TRUE
		`, circleID, fmt.Sprintf("%d", circleIndex), periodOne, chatPerformanceMessagesPerCircle, members, chatPerformanceUsersPerCircle); err != nil {
			t.Fatalf("seed circle %d messages: %v", circleIndex, err)
		}
		f.circles = append(f.circles, circleID)
		f.users = append(f.users, append([]uuid.UUID(nil), members...))
	}
}

func (f *chatPerformanceFixture) assertShape(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	var circles, users int
	if err := f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM circles").Scan(&circles); err != nil {
		t.Fatalf("count circles: %v", err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&users); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if circles != chatPerformanceCircleCount || users != chatPerformanceCircleCount*chatPerformanceUsersPerCircle {
		t.Fatalf("fixture shape circles=%d users=%d, want %d/%d", circles, users, chatPerformanceCircleCount, chatPerformanceCircleCount*chatPerformanceUsersPerCircle)
	}
	for i, circleID := range f.circles {
		var messages, deleted, periods int
		if err := f.pool.QueryRow(ctx, "SELECT COUNT(*), COUNT(*) FILTER (WHERE deleted_at IS NOT NULL) FROM messages WHERE circle_id=$1", circleID).Scan(&messages, &deleted); err != nil {
			t.Fatalf("count circle %d messages: %v", i, err)
		}
		if err := f.pool.QueryRow(ctx, "SELECT COUNT(DISTINCT joined_at) FROM circle_members WHERE circle_id=$1", circleID).Scan(&periods); err != nil {
			t.Fatalf("count circle %d membership periods: %v", i, err)
		}
		if messages != chatPerformanceMessagesPerCircle || deleted != chatPerformanceDeletedPerCircle || periods != 2 {
			t.Fatalf("circle %d shape messages=%d deleted=%d periods=%d, want %d/%d/2", i, messages, deleted, periods, chatPerformanceMessagesPerCircle, chatPerformanceDeletedPerCircle)
		}
	}
}

func TestChatPerformanceFixture_IsReproducible(t *testing.T) {
	f := newChatPerformanceFixture(t)
	f.assertShape(t)
}
