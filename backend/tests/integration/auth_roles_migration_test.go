//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	migration010Up   = "000010_auth_roles_profile.up.sql"
	migration010Down = "000010_auth_roles_profile.down.sql"
	migration011Up   = "000011_auth_roles_profile_alignment.up.sql"
	migration011Down = "000011_auth_roles_profile_alignment.down.sql"
)

func TestAuthRolesAlignmentMigration_WithoutCircles(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()

	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	runMigrationFile(t, conn, ctx, migration010Up)
	runMigrationFile(t, conn, ctx, migration011Up)

	assertColumnType(t, conn, ctx, schema, "user_sessions", "session_id", "uuid")
	assertColumnType(t, conn, ctx, schema, "user_sessions", "device_name", "character varying")
	assertColumnNotNull(t, conn, ctx, schema, "user_sessions", "expires_at")
	assertColumnNotNull(t, conn, ctx, schema, "user_sessions", "session_id")
	assertIndexExists(t, conn, ctx, schema, "idx_user_sessions_expires_at")
	assertIndexExists(t, conn, ctx, schema, "idx_user_sessions_revoked_at")
	assertIndexExists(t, conn, ctx, schema, "idx_user_sessions_user_id_revoked")
	assertIndexExists(t, conn, ctx, schema, "idx_circle_members_circle_role")
	assertConstraintNotExists(t, conn, ctx, schema, "circle_members", "fk_circle_members_circle_id_000011")

	runMigrationFile(t, conn, ctx, migration011Down)

	assertColumnType(t, conn, ctx, schema, "user_sessions", "session_id", "text")
	assertColumnType(t, conn, ctx, schema, "user_sessions", "device_name", "")
	assertConstraintNotExists(t, conn, ctx, schema, "circle_members", "fk_circle_members_circle_id_000011")
}

func TestAuthRolesAlignmentMigration_WithCircles(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()

	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	runMigrationFile(t, conn, ctx, migration010Up)

	// Simulate the parent circles table being present before the alignment migration.
	if _, err := conn.Exec(ctx, `
		CREATE TABLE circles (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`); err != nil {
		t.Fatalf("create circles table: %v", err)
	}

	runMigrationFile(t, conn, ctx, migration011Up)

	assertConstraintExists(t, conn, ctx, schema, "circle_members", "fk_circle_members_circle_id_000011")

	runMigrationFile(t, conn, ctx, migration011Down)

	assertConstraintNotExists(t, conn, ctx, schema, "circle_members", "fk_circle_members_circle_id_000011")
}

func openPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping database: %v", err)
	}
	return pool
}

func acquireConn(t *testing.T, pool *pgxpool.Pool, ctx context.Context) *pgxpool.Conn {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	return conn
}

func createSchema(t *testing.T, conn *pgxpool.Conn, ctx context.Context, schema string) {
	if _, err := conn.Exec(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET search_path TO %s", schema)); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
}

func dropSchema(t *testing.T, pool *pgxpool.Pool, ctx context.Context, schema string) {
	dropCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, _ = pool.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
}

func runMigrationFile(t *testing.T, conn *pgxpool.Conn, ctx context.Context, filename string) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", filename)
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration file %s: %v", filename, err)
	}
	if _, err := conn.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("execute migration %s: %v", filename, err)
	}
}

func assertColumnType(t *testing.T, conn *pgxpool.Conn, ctx context.Context, schema, table, column, wantType string) {
	var gotType string
	query := `
		SELECT data_type
		FROM information_schema.columns
		WHERE table_schema = $1
		  AND table_name = $2
		  AND column_name = $3
	`
	err := conn.QueryRow(ctx, query, schema, table, column).Scan(&gotType)
	if wantType == "" {
		if err == nil {
			t.Fatalf("column %s.%s should not exist", table, column)
		}
		return
	}
	if err != nil {
		t.Fatalf("lookup column %s.%s: %v", table, column, err)
	}
	if !strings.EqualFold(gotType, wantType) {
		t.Fatalf("column %s.%s type: got %q, want %q", table, column, gotType, wantType)
	}
}

func assertColumnNotNull(t *testing.T, conn *pgxpool.Conn, ctx context.Context, schema, table, column string) {
	var nullable string
	query := `
		SELECT is_nullable
		FROM information_schema.columns
		WHERE table_schema = $1
		  AND table_name = $2
		  AND column_name = $3
	`
	if err := conn.QueryRow(ctx, query, schema, table, column).Scan(&nullable); err != nil {
		t.Fatalf("lookup nullability %s.%s: %v", table, column, err)
	}
	if !strings.EqualFold(nullable, "NO") {
		t.Fatalf("column %s.%s should be NOT NULL, got is_nullable=%q", table, column, nullable)
	}
}

func assertIndexExists(t *testing.T, conn *pgxpool.Conn, ctx context.Context, schema, indexName string) {
	var count int
	query := `
		SELECT COUNT(*)
		FROM pg_indexes
		WHERE schemaname = $1
		  AND indexname = $2
	`
	if err := conn.QueryRow(ctx, query, schema, indexName).Scan(&count); err != nil {
		t.Fatalf("lookup index %s: %v", indexName, err)
	}
	if count != 1 {
		t.Fatalf("index %s should exist", indexName)
	}
}

func assertConstraintExists(t *testing.T, conn *pgxpool.Conn, ctx context.Context, schema, table, constraintName string) {
	var count int
	query := `
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE table_schema = $1
		  AND table_name = $2
		  AND constraint_name = $3
	`
	if err := conn.QueryRow(ctx, query, schema, table, constraintName).Scan(&count); err != nil {
		t.Fatalf("lookup constraint %s: %v", constraintName, err)
	}
	if count != 1 {
		t.Fatalf("constraint %s on %s should exist", constraintName, table)
	}
}

func assertConstraintNotExists(t *testing.T, conn *pgxpool.Conn, ctx context.Context, schema, table, constraintName string) {
	var count int
	query := `
		SELECT COUNT(*)
		FROM information_schema.table_constraints
		WHERE table_schema = $1
		  AND table_name = $2
		  AND constraint_name = $3
	`
	if err := conn.QueryRow(ctx, query, schema, table, constraintName).Scan(&count); err != nil {
		t.Fatalf("lookup constraint %s: %v", constraintName, err)
	}
	if count != 0 {
		t.Fatalf("constraint %s on %s should not exist", constraintName, table)
	}
}

func uniqueSchemaName(t *testing.T) string {
	return fmt.Sprintf("test_auth_roles_%d", time.Now().UnixNano())
}
