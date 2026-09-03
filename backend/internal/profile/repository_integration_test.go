//go:build integration

package profile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
)

var profileRepoMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
}

// newProfileRepository opens an isolated schema with the auth/profile migration
// chain applied and returns a profile repository bound to that schema.
func newProfileRepository(t *testing.T) *Repository {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_profile_repo_%d", time.Now().UnixNano())
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
	for _, name := range profileRepoMigrations {
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

func seedProfileUser(t *testing.T, repo *Repository, label string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-profile-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	return id
}

func TestRepository_NewRepository_BindsPool(t *testing.T) {
	repo := newProfileRepository(t)
	if repo.pool == nil {
		t.Fatal("NewRepository must bind the pool")
	}
}

func TestRepository_GetByUserID_ReturnsProfileWhenPresent(t *testing.T) {
	repo := newProfileRepository(t)
	ctx := context.Background()
	userID := seedProfileUser(t, repo, "get-by-id")

	fullName := "Karim Fadel"
	displayName := "Karim"
	bio := "Teacher"
	country := "EG"
	avatarURL := "https://example.com/avatar.png"
	phone := "+123"
	preferredLanguage := "ar"
	completedAt := time.Now().UTC().Truncate(time.Second)
	if err := repo.UpdateByUserID(ctx, UpdateInput{
		UserID:            userID,
		FullName:          &fullName,
		DisplayName:       &displayName,
		Bio:               &bio,
		Country:           &country,
		AvatarURL:         &avatarURL,
		Phone:             &phone,
		PreferredLanguage: &preferredLanguage,
		CompletedAt:       &completedAt,
	}); err != nil {
		t.Fatalf("UpdateByUserID: %v", err)
	}

	record, err := repo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if record.Profile.ID != userID {
		t.Fatalf("profile user id: got %q want %q", record.Profile.ID, userID)
	}
	if record.Profile.FullName == nil || *record.Profile.FullName != fullName {
		t.Fatalf("full name: got %v", record.Profile.FullName)
	}
	if record.Profile.DisplayName == nil || *record.Profile.DisplayName != displayName {
		t.Fatalf("display name: got %v", record.Profile.DisplayName)
	}
	if record.Profile.PreferredLanguage != preferredLanguage {
		t.Fatalf("preferred language: got %q", record.Profile.PreferredLanguage)
	}
	if record.CompletedAt == nil || !record.CompletedAt.Equal(completedAt) {
		t.Fatalf("completed at: got %v", record.CompletedAt)
	}
}

func TestRepository_GetByUserID_ReturnsUserWithNoProfile(t *testing.T) {
	repo := newProfileRepository(t)
	ctx := context.Background()
	userID := seedProfileUser(t, repo, "no-profile")

	record, err := repo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if record.Profile.ID != userID {
		t.Fatalf("profile user id: got %q want %q", record.Profile.ID, userID)
	}
	if record.Profile.PreferredLanguage != "ar" {
		t.Fatalf("default preferred language: got %q", record.Profile.PreferredLanguage)
	}
	if record.Profile.FullName != nil {
		t.Fatalf("full name should be nil when no profile: got %v", record.Profile.FullName)
	}
}

func TestRepository_UpdateByUserID_PartialUpdate(t *testing.T) {
	repo := newProfileRepository(t)
	ctx := context.Background()
	userID := seedProfileUser(t, repo, "partial-update")

	displayName := "Reciter"
	if err := repo.UpdateByUserID(ctx, UpdateInput{
		UserID:      userID,
		DisplayName: &displayName,
	}); err != nil {
		t.Fatalf("UpdateByUserID partial: %v", err)
	}

	record, err := repo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID: %v", err)
	}
	if record.Profile.DisplayName == nil || *record.Profile.DisplayName != displayName {
		t.Fatalf("display name not updated: got %v", record.Profile.DisplayName)
	}
	if record.Profile.FullName != nil {
		t.Fatalf("full name should remain nil: got %v", record.Profile.FullName)
	}
}

func TestRepository_GetByUserID_UnknownUser(t *testing.T) {
	repo := newProfileRepository(t)
	ctx := context.Background()

	if _, err := repo.GetByUserID(ctx, "00000000-0000-0000-0000-000000000000"); !errors.Is(err, auth.ErrUserNotFound) {
		t.Fatalf("unknown user: got %v want %v", err, auth.ErrUserNotFound)
	}
}
