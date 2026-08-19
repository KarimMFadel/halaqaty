//go:build integration

package integration

import (
	"context"
	"testing"
)

func TestLiveSessionsMigration_F005OwnsSessionLifecycleTables(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()
	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range append(liveSessionHeadMigrations, migration016Up) {
		runMigrationFile(t, conn, ctx, migration)
	}

	for _, table := range []string{"sessions", "session_participant_presence"} {
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatalf("check F-005 table %s: %v", table, err)
		}
		if !exists {
			t.Fatalf("F-005 table %s is missing", table)
		}
	}

	var attendanceExists bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.session_attendance') IS NOT NULL`).Scan(&attendanceExists); err != nil {
		t.Fatalf("check F-006 attendance table: %v", err)
	}
	if attendanceExists {
		t.Fatal("F-005 migration must not redefine F-006 attendance ownership")
	}

	var scheduledAtNullable string
	if err := conn.QueryRow(ctx, `
		SELECT is_nullable FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = 'sessions' AND column_name = 'scheduled_at'`).Scan(&scheduledAtNullable); err != nil {
		t.Fatalf("read scheduled_at ownership: %v", err)
	}
	if scheduledAtNullable != "YES" {
		t.Fatalf("F-005 ad-hoc scheduled_at must remain nullable, got %q", scheduledAtNullable)
	}
}
