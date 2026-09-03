//go:build integration

package rbac

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var rbacRepoMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
	"000013_create_circles.up.sql",
	"000014_circle_members_circle_fk.up.sql",
	"000015_circle_management.up.sql",
}

// newIntegrationRepository opens an isolated schema with the auth + circle
// migration chain applied and returns a repository bound to that schema.
func newIntegrationRepository(t *testing.T) *Repository {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_rbac_repo_%d", time.Now().UnixNano())
	sep := "?"
	if strings.Contains(dbURL, sep) {
		sep = "&"
	}
	pool, err := pgxpool.New(ctx, dbURL+sep+"search_path="+schema)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		pool.Close()
	})
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	for _, name := range rbacRepoMigrations {
		sql, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	return NewRepository(pool)
}

func seedIntegrationUser(t *testing.T, repo *Repository, label string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-rbac-"+label, label+"-rbac@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	return id
}

func seedIntegrationProfile(t *testing.T, repo *Repository, userID string, displayName, fullName *string) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO profiles (user_id, display_name, full_name)
		VALUES ($1::uuid, $2, $3)
	`, userID, displayName, fullName); err != nil {
		t.Fatalf("seed profile for %s: %v", userID, err)
	}
}

func seedIntegrationCircle(t *testing.T, repo *Repository, name, teacherID, inviteCode string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ($1, $2::uuid, $3)
		RETURNING id::text
	`, name, teacherID, inviteCode).Scan(&id); err != nil {
		t.Fatalf("seed circle %s: %v", name, err)
	}
	return id
}

func seedIntegrationMember(t *testing.T, repo *Repository, circleID, userID, role string) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO circle_members (circle_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, $3)
	`, circleID, userID, role); err != nil {
		t.Fatalf("seed member %s in %s: %v", userID, circleID, err)
	}
}

func TestRepository_RoleInCircle_ReturnsRoleForMembersOnly(t *testing.T) {
	repo := newIntegrationRepository(t)
	ctx := context.Background()
	teacher := seedIntegrationUser(t, repo, "role-teacher")
	outsider := seedIntegrationUser(t, repo, "role-outsider")
	circle := seedIntegrationCircle(t, repo, "Role Circle", teacher, "HLQ-ROLE1")
	seedIntegrationMember(t, repo, circle, teacher, RoleTeacher)

	role, err := repo.RoleInCircle(ctx, circle, teacher)
	if err != nil || role != RoleTeacher {
		t.Fatalf("member role: got (%q, %v) want (%q, nil)", role, err, RoleTeacher)
	}

	role, err = repo.RoleInCircle(ctx, circle, outsider)
	if err != nil || role != "" {
		t.Fatalf("non-member role: got (%q, %v) want (\"\", nil)", role, err)
	}
}

func TestRepository_ListCircleIDs_ReturnsMembershipSet(t *testing.T) {
	repo := newIntegrationRepository(t)
	ctx := context.Background()
	member := seedIntegrationUser(t, repo, "list-member")
	loner := seedIntegrationUser(t, repo, "list-loner")
	first := seedIntegrationCircle(t, repo, "First Circle", member, "HLQ-LST01")
	second := seedIntegrationCircle(t, repo, "Second Circle", member, "HLQ-LST02")
	seedIntegrationMember(t, repo, first, member, RoleStudent)
	seedIntegrationMember(t, repo, second, member, RoleSupervisor)

	ids, err := repo.ListCircleIDs(ctx, member)
	if err != nil {
		t.Fatalf("ListCircleIDs: %v", err)
	}
	sort.Strings(ids)
	want := []string{first, second}
	sort.Strings(want)
	if len(ids) != 2 || ids[0] != want[0] || ids[1] != want[1] {
		t.Fatalf("circle IDs: got %v want %v", ids, want)
	}

	ids, err = repo.ListCircleIDs(ctx, loner)
	if err != nil {
		t.Fatalf("ListCircleIDs for non-member: %v", err)
	}
	if ids == nil || len(ids) != 0 {
		t.Fatalf("non-member circle IDs: got %v want empty non-nil slice", ids)
	}
}

func TestRepository_UpdateCircle_PersistsSettings(t *testing.T) {
	repo := newIntegrationRepository(t)
	ctx := context.Background()
	teacher := seedIntegrationUser(t, repo, "update-teacher")
	circle := seedIntegrationCircle(t, repo, "Update Circle", teacher, "HLQ-UPD01")
	description := "evening circle"
	rules := "mutual respect"

	updated, err := repo.UpdateCircle(ctx, circle, "Renamed Circle", CircleSettings{
		Description: &description, Rules: &rules, MaxCapacity: 40, IsPrivate: true,
		GenderRestriction: "female", Language: "en", GradingPolicy: "optional",
	})
	if err != nil {
		t.Fatalf("UpdateCircle: %v", err)
	}
	if updated.Name != "Renamed Circle" || updated.MaxCapacity != 40 || !updated.IsPrivate ||
		updated.GenderRestriction != "female" || updated.Language != "en" || updated.GradingPolicy != "optional" {
		t.Fatalf("returned projection not updated: %+v", updated)
	}
	if updated.Description == nil || *updated.Description != description {
		t.Fatalf("description: got %v want %q", updated.Description, description)
	}
	if updated.InviteCode != "HLQ-UPD01" {
		t.Fatalf("update must not rotate invite code: got %q", updated.InviteCode)
	}

	reread, err := repo.FindCircleByID(ctx, circle)
	if err != nil {
		t.Fatalf("FindCircleByID: %v", err)
	}
	if reread.Name != "Renamed Circle" || reread.MaxCapacity != 40 || !reread.IsPrivate {
		t.Fatalf("settings not persisted: %+v", reread)
	}
}

func TestRepository_RefreshInviteCode_ReplacesOnlyActiveCode(t *testing.T) {
	repo := newIntegrationRepository(t)
	ctx := context.Background()
	teacher := seedIntegrationUser(t, repo, "refresh-teacher")
	circle := seedIntegrationCircle(t, repo, "Refresh Circle", teacher, "HLQ-OLD01")

	if err := repo.RefreshInviteCode(ctx, circle, "HLQ-NEW09"); err != nil {
		t.Fatalf("RefreshInviteCode: %v", err)
	}
	if found, err := repo.FindCircleByInviteCode(ctx, "HLQ-NEW09"); err != nil || found.ID != circle {
		t.Fatalf("new code lookup: got (%q, %v)", found.ID, err)
	}
	if _, err := repo.FindCircleByInviteCode(ctx, "HLQ-OLD01"); err != ErrCircleNotFound {
		t.Fatalf("old code must stop resolving: got %v want %v", err, ErrCircleNotFound)
	}
}

func TestRepository_RemoveMember_DeletesOnlyTargetMembership(t *testing.T) {
	repo := newIntegrationRepository(t)
	ctx := context.Background()
	teacher := seedIntegrationUser(t, repo, "remove-teacher")
	leaver := seedIntegrationUser(t, repo, "remove-leaver")
	circle := seedIntegrationCircle(t, repo, "Remove Circle", teacher, "HLQ-RMV01")
	seedIntegrationMember(t, repo, circle, teacher, RoleTeacher)
	seedIntegrationMember(t, repo, circle, leaver, RoleStudent)

	if err := repo.RemoveMember(ctx, circle, leaver); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}

	if role, err := repo.RoleInCircle(ctx, circle, leaver); err != nil || role != "" {
		t.Fatalf("removed member still resolves: got (%q, %v)", role, err)
	}
	members, err := repo.ListMembers(ctx, circle)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(members) != 1 || members[0].UserID != teacher {
		t.Fatalf("remaining members: got %+v want only teacher", members)
	}
}

func TestRepository_ArchiveCircle_PreservesMembershipHistory(t *testing.T) {
	repo := newIntegrationRepository(t)
	ctx := context.Background()
	teacher := seedIntegrationUser(t, repo, "archive-teacher")
	student := seedIntegrationUser(t, repo, "archive-student")
	circle := seedIntegrationCircle(t, repo, "Archive Circle", teacher, "HLQ-ARC01")
	seedIntegrationMember(t, repo, circle, teacher, RoleTeacher)
	seedIntegrationMember(t, repo, circle, student, RoleStudent)

	if err := repo.ArchiveCircle(ctx, circle); err != nil {
		t.Fatalf("ArchiveCircle: %v", err)
	}

	archived, err := repo.FindCircleByID(ctx, circle)
	if err != nil {
		t.Fatalf("FindCircleByID: %v", err)
	}
	if !archived.IsArchived {
		t.Fatal("circle must be flagged archived")
	}
	if role, err := repo.RoleInCircle(ctx, circle, student); err != nil || role != RoleStudent {
		t.Fatalf("archive must retain membership history: got (%q, %v)", role, err)
	}
}

func TestRepository_SearchUsers_MatchesDisplayName(t *testing.T) {
	repo := newIntegrationRepository(t)
	ctx := context.Background()
	karim := seedIntegrationUser(t, repo, "search-karim")
	sara := seedIntegrationUser(t, repo, "search-sara")
	omar := seedIntegrationUser(t, repo, "search-omar")
	seedIntegrationProfile(t, repo, karim, strPtr("Karim Teacher"), nil)
	seedIntegrationProfile(t, repo, sara, strPtr("Sara Reciter"), nil)
	// Omar has no display name; search must fall back to full_name.
	seedIntegrationProfile(t, repo, omar, nil, strPtr("Omar Fallback"))

	users, err := repo.SearchUsers(ctx, "kar", 20)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if len(users) != 1 || users[0].ID != karim || users[0].DisplayName != "Karim Teacher" {
		t.Fatalf("display-name match: got %+v", users)
	}

	users, err = repo.SearchUsers(ctx, "omar fall", 20)
	if err != nil {
		t.Fatalf("SearchUsers fallback: %v", err)
	}
	if len(users) != 1 || users[0].ID != omar || users[0].DisplayName != "Omar Fallback" {
		t.Fatalf("full_name fallback match: got %+v", users)
	}

	users, err = repo.SearchUsers(ctx, "zzz-no-match", 20)
	if err != nil {
		t.Fatalf("SearchUsers no match: %v", err)
	}
	if len(users) != 0 {
		t.Fatalf("expected no matches, got %+v", users)
	}
}

func TestRepository_CircleExists_ReportsRowPresence(t *testing.T) {
	repo := newIntegrationRepository(t)
	ctx := context.Background()
	teacher := seedIntegrationUser(t, repo, "exists-teacher")
	circle := seedIntegrationCircle(t, repo, "Exists Circle", teacher, "HLQ-EXS01")

	if exists, err := repo.CircleExists(ctx, circle); err != nil || !exists {
		t.Fatalf("existing circle: got (%v, %v) want (true, nil)", exists, err)
	}
	if exists, err := repo.CircleExists(ctx, uuid.NewString()); err != nil || exists {
		t.Fatalf("unknown circle: got (%v, %v) want (false, nil)", exists, err)
	}
}

func TestRepository_UpdateMemberRole_PersistsChange(t *testing.T) {
	repo := newIntegrationRepository(t)
	ctx := context.Background()
	teacher := seedIntegrationUser(t, repo, "promote-teacher")
	student := seedIntegrationUser(t, repo, "promote-student")
	circle := seedIntegrationCircle(t, repo, "Promote Circle", teacher, "HLQ-PRM01")
	seedIntegrationMember(t, repo, circle, teacher, RoleTeacher)
	seedIntegrationMember(t, repo, circle, student, RoleStudent)

	if err := repo.UpdateMemberRole(ctx, circle, student, RoleSupervisor); err != nil {
		t.Fatalf("UpdateMemberRole: %v", err)
	}
	if role, err := repo.RoleInCircle(ctx, circle, student); err != nil || role != RoleSupervisor {
		t.Fatalf("role after update: got (%q, %v) want (%q, nil)", role, err, RoleSupervisor)
	}
}

func TestRepository_CancelledContext_WrapsOperationContext(t *testing.T) {
	repo := newIntegrationRepository(t)
	circle := seedIntegrationCircle(t, repo, "Cancel Circle", seedIntegrationUser(t, repo, "cancel-user"), "HLQ-CCL01")

	cases := []struct {
		name       string
		run        func(context.Context) error
		wantPrefix string
	}{
		{
			name: "role in circle",
			run: func(ctx context.Context) error {
				_, err := repo.RoleInCircle(ctx, circle, uuid.NewString())
				return err
			},
			wantPrefix: "read circle role",
		},
		{
			name:       "list circle ids",
			run:        func(ctx context.Context) error { _, err := repo.ListCircleIDs(ctx, uuid.NewString()); return err },
			wantPrefix: "list user circles",
		},
		{
			name: "update circle",
			run: func(ctx context.Context) error {
				_, err := repo.UpdateCircle(ctx, circle, "X", CircleSettings{MaxCapacity: 10})
				return err
			},
			wantPrefix: "find circle",
		},
		{
			name:       "refresh invite code",
			run:        func(ctx context.Context) error { return repo.RefreshInviteCode(ctx, circle, "HLQ-CCL02") },
			wantPrefix: "refresh invite code",
		},
		{
			name:       "remove member",
			run:        func(ctx context.Context) error { return repo.RemoveMember(ctx, circle, uuid.NewString()) },
			wantPrefix: "remove circle member",
		},
		{
			name:       "archive circle",
			run:        func(ctx context.Context) error { return repo.ArchiveCircle(ctx, circle) },
			wantPrefix: "archive circle",
		},
		{
			name:       "search users",
			run:        func(ctx context.Context) error { _, err := repo.SearchUsers(ctx, "kar", 20); return err },
			wantPrefix: "search users",
		},
		{
			name:       "circle exists",
			run:        func(ctx context.Context) error { _, err := repo.CircleExists(ctx, circle); return err },
			wantPrefix: "check circle existence",
		},
		{
			name: "update member role",
			run: func(ctx context.Context) error {
				return repo.UpdateMemberRole(ctx, circle, uuid.NewString(), RoleStudent)
			},
			wantPrefix: "update circle member role",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			err := tc.run(ctx)
			if err == nil {
				t.Fatal("expected error for cancelled context")
			}
			if !strings.HasPrefix(err.Error(), tc.wantPrefix) {
				t.Fatalf("error must carry operation context %q, got %v", tc.wantPrefix, err)
			}
		})
	}
}
