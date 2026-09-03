//go:build integration

package auth

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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var authRepoMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
}

// newSessionRepo opens an isolated schema with the auth migration chain
// applied and returns a session repository bound to that schema.
func newSessionRepo(t *testing.T) *SessionRepository {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_auth_repo_%d", time.Now().UnixNano())
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
	for _, name := range authRepoMigrations {
		sql, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	return NewSessionRepository(pool)
}

// seedAuthUser inserts one users row and returns its UUID.
func seedAuthUser(t *testing.T, repo *SessionRepository, label string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-auth-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	return id
}

// createRepoSession persists one session for userID with whole-second
// timestamps so round-trips compare exactly.
func createRepoSession(t *testing.T, repo *SessionRepository, userID string, deviceName *string) Session {
	t.Helper()
	session := Session{
		ID:             uuid.NewString(),
		UserID:         userID,
		DeviceName:     deviceName,
		LastActivityAt: time.Now().UTC().Truncate(time.Second),
		ExpiresAt:      time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second),
	}
	if err := repo.CreateSession(context.Background(), session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return session
}

func TestSessionRepository_UpsertUserByFirebaseUID(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()

	user, inserted, err := repo.UpsertUserByFirebaseUID(ctx, "uid-upsert-1", "upsert1@example.com")
	if err != nil || !inserted {
		t.Fatalf("first upsert: got (%v, %v) want (nil, true)", err, inserted)
	}
	if user.ID == "" || user.FirebaseUID != "uid-upsert-1" || user.Email != "upsert1@example.com" {
		t.Fatalf("inserted user projection: %+v", user)
	}

	// Replay of the same Firebase identity must be idempotent and keep the ID.
	replayed, inserted, err := repo.UpsertUserByFirebaseUID(ctx, "uid-upsert-1", "upsert1@example.com")
	if err != nil || inserted {
		t.Fatalf("replay upsert: got (%v, %v) want (nil, false)", err, inserted)
	}
	if replayed.ID != user.ID {
		t.Fatalf("replay must keep user id: got %q want %q", replayed.ID, user.ID)
	}

	// Replay with a changed email refreshes the stored email.
	refreshed, _, err := repo.UpsertUserByFirebaseUID(ctx, "uid-upsert-1", "renamed@example.com")
	if err != nil {
		t.Fatalf("email refresh upsert: %v", err)
	}
	if refreshed.Email != "renamed@example.com" {
		t.Fatalf("email not refreshed: got %q", refreshed.Email)
	}

	// The same email bound to a different Firebase UID must be rejected.
	_, _, err = repo.UpsertUserByFirebaseUID(ctx, "uid-upsert-2", "renamed@example.com")
	if !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("duplicate email: got %v want %v", err, ErrDuplicateEmail)
	}
}

func TestSessionRepository_GetUserByFirebaseUID(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	seeded := seedAuthUser(t, repo, "by-uid")

	user, err := repo.GetUserByFirebaseUID(ctx, "firebase-auth-by-uid")
	if err != nil {
		t.Fatalf("GetUserByFirebaseUID: %v", err)
	}
	if user.ID != seeded || user.Email != "by-uid@example.com" {
		t.Fatalf("user projection: got %+v want id %s", user, seeded)
	}

	if _, err := repo.GetUserByFirebaseUID(ctx, "firebase-auth-unknown"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown uid: got %v want %v", err, ErrUserNotFound)
	}
}

func TestSessionRepository_GetUserByEmail(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	seeded := seedAuthUser(t, repo, "by-email")

	user, err := repo.GetUserByEmail(ctx, "by-email@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if user.ID != seeded || user.FirebaseUID != "firebase-auth-by-email" {
		t.Fatalf("user projection: got %+v want id %s", user, seeded)
	}

	if _, err := repo.GetUserByEmail(ctx, "ghost@example.com"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown email: got %v want %v", err, ErrUserNotFound)
	}
}

func TestSessionRepository_CreateEmptyProfile(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	userID := seedAuthUser(t, repo, "empty-profile")

	if err := repo.CreateEmptyProfile(ctx, userID); err != nil {
		t.Fatalf("CreateEmptyProfile: %v", err)
	}
	if err := repo.CreateEmptyProfile(ctx, userID); err != nil {
		t.Fatalf("CreateEmptyProfile replay must be idempotent: %v", err)
	}

	profile, err := repo.GetUserProfileByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserProfileByUserID: %v", err)
	}
	if profile.ID != userID || profile.DisplayName != nil || profile.PreferredLanguage != "ar" {
		t.Fatalf("empty profile projection: %+v", profile)
	}
}

func TestSessionRepository_GetByIDAndUserID_RequiresOwnership(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	owner := seedAuthUser(t, repo, "session-owner")
	intruder := seedAuthUser(t, repo, "session-intruder")
	device := "Pixel 9"
	session := createRepoSession(t, repo, owner, &device)

	fetched, err := repo.GetByIDAndUserID(ctx, session.ID, owner)
	if err != nil {
		t.Fatalf("owner lookup: %v", err)
	}
	if fetched.UserID != owner || fetched.DeviceName == nil || *fetched.DeviceName != device {
		t.Fatalf("owned session projection: %+v", fetched)
	}

	if _, err := repo.GetByIDAndUserID(ctx, session.ID, intruder); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("foreign session must not resolve: got %v want %v", err, ErrSessionNotFound)
	}
	if _, err := repo.GetByIDAndUserID(ctx, uuid.NewString(), owner); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown session: got %v want %v", err, ErrSessionNotFound)
	}
}

func TestSessionRepository_GetLocalUserIDByFirebaseUID(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	seeded := seedAuthUser(t, repo, "local-id")

	id, err := repo.GetLocalUserIDByFirebaseUID(ctx, "firebase-auth-local-id")
	if err != nil || id != seeded {
		t.Fatalf("local user id: got (%q, %v) want (%q, nil)", id, err, seeded)
	}

	if _, err := repo.GetLocalUserIDByFirebaseUID(ctx, "firebase-auth-missing"); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("unknown uid: got %v want %v", err, ErrUserNotFound)
	}
}

func TestSessionRepository_Touch_UpdatesActivityAndRequiresRow(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	userID := seedAuthUser(t, repo, "touch-user")
	session := createRepoSession(t, repo, userID, nil)

	touched := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if err := repo.Touch(ctx, session.ID, touched); err != nil {
		t.Fatalf("Touch: %v", err)
	}
	fetched, err := repo.GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !fetched.LastActivityAt.Equal(touched) {
		t.Fatalf("last activity: got %v want %v", fetched.LastActivityAt, touched)
	}

	if err := repo.Touch(ctx, uuid.NewString(), touched); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown session touch: got %v want %v", err, ErrSessionNotFound)
	}
}

func TestSessionRepository_Revoke_MarksSessionAndRequiresRow(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	userID := seedAuthUser(t, repo, "revoke-user")
	session := createRepoSession(t, repo, userID, nil)

	revokedAt := time.Now().UTC().Truncate(time.Second)
	if err := repo.Revoke(ctx, session.ID, revokedAt); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	fetched, err := repo.GetByID(ctx, session.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if fetched.RevokedAt == nil || !fetched.RevokedAt.Equal(revokedAt) {
		t.Fatalf("revoked_at: got %v want %v", fetched.RevokedAt, revokedAt)
	}
	if !fetched.IsRevoked() {
		t.Fatal("session must report revoked")
	}

	if err := repo.Revoke(ctx, uuid.NewString(), revokedAt); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown session revoke: got %v want %v", err, ErrSessionNotFound)
	}
}

func TestSessionRepository_DeviceNameRoundTrip(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()
	userID := seedAuthUser(t, repo, "device-user")

	device := "Samsung Tab"
	named := createRepoSession(t, repo, userID, &device)
	fetched, err := repo.GetByID(ctx, named.ID)
	if err != nil {
		t.Fatalf("GetByID named device: %v", err)
	}
	if fetched.DeviceName == nil || *fetched.DeviceName != device {
		t.Fatalf("device name: got %v want %q", fetched.DeviceName, device)
	}

	anonymous := createRepoSession(t, repo, userID, nil)
	fetched, err = repo.GetByID(ctx, anonymous.ID)
	if err != nil {
		t.Fatalf("GetByID anonymous device: %v", err)
	}
	if fetched.DeviceName != nil {
		t.Fatalf("nil device name must stay nil: got %q", *fetched.DeviceName)
	}

	if _, err := repo.GetByID(ctx, uuid.NewString()); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("unknown session: got %v want %v", err, ErrSessionNotFound)
	}
}

func TestSessionRepository_CancelledContext_WrapsOperationContext(t *testing.T) {
	repo := newSessionRepo(t)
	userID := seedAuthUser(t, repo, "cancel-user")

	cases := []struct {
		name       string
		run        func(context.Context) error
		wantPrefix string
	}{
		{
			name: "upsert user",
			run: func(ctx context.Context) error {
				_, _, err := repo.UpsertUserByFirebaseUID(ctx, "uid-cancel", "cancel@example.com")
				return err
			},
			wantPrefix: "upsert user by firebase uid",
		},
		{
			name:       "get user by firebase uid",
			run:        func(ctx context.Context) error { _, err := repo.GetUserByFirebaseUID(ctx, "uid-cancel"); return err },
			wantPrefix: "get user by firebase uid",
		},
		{
			name:       "get user by email",
			run:        func(ctx context.Context) error { _, err := repo.GetUserByEmail(ctx, "cancel@example.com"); return err },
			wantPrefix: "get user by email",
		},
		{
			name:       "create empty profile",
			run:        func(ctx context.Context) error { return repo.CreateEmptyProfile(ctx, userID) },
			wantPrefix: "create empty profile",
		},
		{
			name: "create session",
			run: func(ctx context.Context) error {
				return repo.CreateSession(ctx, Session{ID: uuid.NewString(), UserID: userID, ExpiresAt: time.Now()})
			},
			wantPrefix: "create session",
		},
		{
			name: "get by id and user",
			run: func(ctx context.Context) error {
				_, err := repo.GetByIDAndUserID(ctx, uuid.NewString(), userID)
				return err
			},
			wantPrefix: "scan session",
		},
		{
			name: "get local user id",
			run: func(ctx context.Context) error {
				_, err := repo.GetLocalUserIDByFirebaseUID(ctx, "uid-cancel")
				return err
			},
			wantPrefix: "get local user id",
		},
		{
			name:       "touch session",
			run:        func(ctx context.Context) error { return repo.Touch(ctx, uuid.NewString(), time.Now()) },
			wantPrefix: "touch session",
		},
		{
			name:       "revoke session",
			run:        func(ctx context.Context) error { return repo.Revoke(ctx, uuid.NewString(), time.Now()) },
			wantPrefix: "revoke session",
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
