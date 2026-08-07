//go:build integration

package integration

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCircleManagementMigration_PreservesLegacyRowsAndRollsBack(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range []string{
		"000010_auth_roles_profile.up.sql",
		"000011_auth_roles_profile_alignment.up.sql",
		"000012_auth_profiles_display_name.up.sql",
		"000013_create_circles.up.sql",
		"000014_circle_members_circle_fk.up.sql",
	} {
		runMigrationFile(t, conn, ctx, migration)
	}

	userID := "11111111-1111-1111-1111-111111111111"
	circleID := "22222222-2222-2222-2222-222222222222"
	if _, err := conn.Exec(ctx, `INSERT INTO users (id, firebase_uid, email) VALUES ($1::uuid, 'firebase-circle', 'circle@example.com')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO circles (id, name, teacher_id, invite_code) VALUES ($1::uuid, 'Legacy', $2::uuid, 'HLQ-AAAAAA')`, circleID, userID); err != nil {
		t.Fatalf("seed circle: %v", err)
	}

	runMigrationFile(t, conn, ctx, "000015_circle_management.up.sql")
	var capacity int
	var gender, language string
	if err := conn.QueryRow(ctx, `SELECT max_capacity, gender_restriction, language FROM circles WHERE id = $1::uuid`, circleID).Scan(&capacity, &gender, &language); err != nil {
		t.Fatalf("read migrated circle: %v", err)
	}
	if capacity != 50 || gender != "unspecified" || language != "ar" {
		t.Fatalf("unexpected migrated defaults: capacity=%d gender=%q language=%q", capacity, gender, language)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1::uuid, $2::uuid, 'teacher')`, circleID, userID); err != nil {
		t.Fatalf("seed legacy membership: %v", err)
	}

	// The migration is safe to replay: constraints are replaced and the index
	// is guarded without changing the existing row or membership.
	runMigrationFile(t, conn, ctx, "000015_circle_management.up.sql")
	var memberCount int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM circle_members WHERE circle_id = $1::uuid`, circleID).Scan(&memberCount); err != nil {
		t.Fatalf("count preserved membership: %v", err)
	}
	if memberCount != 1 {
		t.Fatalf("rerun changed preserved membership count: got %d", memberCount)
	}

	var indexCount int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM pg_indexes WHERE schemaname = current_schema() AND indexname = 'idx_circles_public_active'`).Scan(&indexCount); err != nil {
		t.Fatalf("check public index: %v", err)
	}
	if indexCount != 1 {
		t.Fatalf("expected one public index, got %d", indexCount)
	}

	runMigrationFile(t, conn, ctx, "000015_circle_management.down.sql")
	assertColumnNotExists(t, conn, ctx, schema, "circles", "max_capacity")
	var preservedName string
	if err := conn.QueryRow(ctx, `SELECT name FROM circles WHERE id = $1::uuid`, circleID).Scan(&preservedName); err != nil {
		t.Fatalf("read preserved circle after rollback: %v", err)
	}
	if preservedName != "Legacy" {
		t.Fatalf("rollback removed or changed legacy circle: %q", preservedName)
	}
}

func TestCircleManagementMigration_ContainsNoHardDelete(t *testing.T) {
	for _, name := range []string{"000015_circle_management.up.sql", "000015_circle_management.down.sql"} {
		contents, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(strings.ToLower(string(contents)), "delete from circles") {
			t.Fatalf("%s introduces hard deletion of circles", name)
		}
	}
}

func assertColumnNotExists(t *testing.T, conn *pgxpool.Conn, ctx context.Context, schema, table, column string) {
	t.Helper()
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema = $1 AND table_name = $2 AND column_name = $3)`, schema, table, column).Scan(&exists); err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	if exists {
		t.Fatalf("column %s.%s still exists after rollback", table, column)
	}
}
