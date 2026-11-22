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

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCircleInvitationMigration_ConstraintsRollbackAndExistingData(t *testing.T) {
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
		"000015_circle_management.up.sql",
		"000016_live_sessions.up.sql",
		"000017_recitation_queue_system.up.sql",
		"000018_real_time_chat.up.sql",
		"000019_account_deletion_tombstone.up.sql",
		"000020_schedule_calendar_attendance.up.sql",
		"000021_notification_system.up.sql",
	} {
		runMigrationFile(t, conn, ctx, migration)
	}
	userID := seedLiveSessionUser(t, conn, ctx, "invitation-migration")
	circleID := "22222222-2222-2222-2222-222222222222"
	if _, err := conn.Exec(ctx, `
		INSERT INTO circles (id, name, teacher_id, invite_code)
		VALUES ($1::uuid, 'Existing circle', $2::uuid, 'HLQ-EXIST')
	`, circleID, userID); err != nil {
		t.Fatalf("seed existing circle: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO circle_members (circle_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, 'teacher')
	`, circleID, userID); err != nil {
		t.Fatalf("seed existing membership: %v", err)
	}
	runMigrationFile(t, conn, ctx, "000022_circle_invitations.up.sql")

	inviterID := seedLiveSessionUser(t, conn, ctx, "invitation-sender")
	recipientID := seedLiveSessionUser(t, conn, ctx, "invitation-recipient")
	var priorID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO circle_invitations (circle_id, inviter_user_id, recipient_user_id, role)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'teacher') RETURNING id::text
	`, circleID, inviterID, recipientID).Scan(&priorID); err != nil {
		t.Fatalf("insert pending invitation: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO circle_invitations (circle_id, inviter_user_id, recipient_user_id, role)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'student')
	`, circleID, inviterID, recipientID); err == nil {
		t.Fatal("second pending invitation for one recipient unexpectedly succeeded")
	}
	if _, err := conn.Exec(ctx, `
		UPDATE circle_invitations SET state = 'superseded', resolved_at = NOW() WHERE id = $1::uuid
	`, priorID); err != nil {
		t.Fatalf("supersede prior invitation: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO circle_invitations (circle_id, inviter_user_id, recipient_user_id, role)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'student')
	`, circleID, inviterID, recipientID); err != nil {
		t.Fatalf("insert replacement invitation: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO circle_invitations (circle_id, inviter_user_id, recipient_user_id, role)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'supervisor')
	`, circleID, inviterID, seedLiveSessionUser(t, conn, ctx, "invalid-role-recipient")); err == nil {
		t.Fatal("invalid invitation role unexpectedly succeeded")
	}
	var preservedMembers int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM circle_members WHERE circle_id = $1::uuid`, circleID).Scan(&preservedMembers); err != nil {
		t.Fatalf("verify existing membership after migration: %v", err)
	}
	if preservedMembers != 1 {
		t.Fatalf("migration changed existing membership count: %d", preservedMembers)
	}
	if _, err := conn.Exec(ctx, `DELETE FROM circle_invitations`); err != nil {
		t.Fatalf("clear invitation history for rollback: %v", err)
	}
	runMigrationFile(t, conn, ctx, "000022_circle_invitations.down.sql")
	var preservedName string
	if err := conn.QueryRow(ctx, `SELECT name FROM circles WHERE id = $1::uuid`, circleID).Scan(&preservedName); err != nil {
		t.Fatalf("read existing circle after rollback: %v", err)
	}
	if preservedName != "Existing circle" {
		t.Fatalf("migration changed existing circle: %q", preservedName)
	}
}

func TestCircleInvitationMigration_RefusesToDropInvitationHistory(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)
	for _, migration := range []string{
		"000010_auth_roles_profile.up.sql", "000011_auth_roles_profile_alignment.up.sql",
		"000012_auth_profiles_display_name.up.sql", "000013_create_circles.up.sql",
		"000014_circle_members_circle_fk.up.sql", "000015_circle_management.up.sql",
	} {
		runMigrationFile(t, conn, ctx, migration)
	}
	userID := seedLiveSessionUser(t, conn, ctx, "invitation-down-guard")
	circleID := "33333333-3333-3333-3333-333333333333"
	if _, err := conn.Exec(ctx, `INSERT INTO circles (id, name, teacher_id, invite_code) VALUES ($1::uuid, 'Guard', $2::uuid, 'HLQ-GUARD')`, circleID, userID); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	runMigrationFile(t, conn, ctx, "000022_circle_invitations.up.sql")
	if _, err := conn.Exec(ctx, `INSERT INTO circle_invitations (circle_id, inviter_user_id, recipient_user_id, role) VALUES ($1::uuid, $2::uuid, $2::uuid, 'teacher')`, circleID, userID); err == nil {
		t.Fatal("self invitation unexpectedly succeeded")
	}
	recipientID := seedLiveSessionUser(t, conn, ctx, "invitation-down-recipient")
	if _, err := conn.Exec(ctx, `INSERT INTO circle_invitations (circle_id, inviter_user_id, recipient_user_id, role) VALUES ($1::uuid, $2::uuid, $3::uuid, 'teacher')`, circleID, userID, recipientID); err != nil {
		t.Fatalf("seed invitation history: %v", err)
	}
	err := executeMigrationFile(conn, ctx, "000022_circle_invitations.down.sql")
	if err == nil || !strings.Contains(err.Error(), "invitation history exists") {
		t.Fatalf("rollback error = %v, want invitation-history guard", err)
	}
	var retained int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM circle_invitations`).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("invitation history after refused rollback: count=%d err=%v", retained, err)
	}
}

func executeMigrationFile(conn *pgxpool.Conn, ctx context.Context, filename string) error {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("resolve migration test path")
	}
	migrationPath := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", filename)
	sql, err := os.ReadFile(migrationPath)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", filename, err)
	}
	_, err = conn.Exec(ctx, string(sql))
	return err
}
