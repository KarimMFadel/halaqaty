//go:build integration

package chat

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// seedChatUploadWithState inserts one chat upload row with an explicit state
// and created_at for cleanup-selection fixtures.
func seedChatUploadWithState(t *testing.T, repo *Repository, uploader, circleID uuid.UUID, objectKey, state string, createdAt time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO chat_uploads (uploader_id, authorization_circle_id, object_key, mime_type,
		                          original_file_name, size_bytes, state, created_at)
		VALUES ($1::uuid, $2::uuid, $3, 'audio/ogg', 'note.ogg', 1024, $4, $5)
		RETURNING id
	`, uploader, circleID, objectKey, state, createdAt).Scan(&id); err != nil {
		t.Fatalf("seed chat upload %s (%s): %v", objectKey, state, err)
	}
	return id
}

func TestMediaCleanup_ClaimExpiredStagedRevokesOnlyOldStagedRows(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	uploader := seedUser(t, repo, "cleanup-uploader")
	circleID := seedCircle(t, repo, "Cleanup Circle", uploader)
	source := NewPoolStagedUploadSource(repo.pool)

	old := time.Now().UTC().Add(-25 * time.Hour).Truncate(time.Microsecond)
	fresh := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	oldStaged := seedChatUploadWithState(t, repo, uploader, circleID, "chat/old-staged", "staged", old)
	seedChatUploadWithState(t, repo, uploader, circleID, "chat/fresh-staged", "staged", fresh)
	seedChatUploadWithState(t, repo, uploader, circleID, "chat/old-attached", "attached", old)
	seedChatUploadWithState(t, repo, uploader, circleID, "chat/old-revoked", "revoked", old)

	claimed, err := source.ClaimExpiredStaged(ctx, time.Now().UTC().Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("claim expired staged: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != oldStaged {
		t.Fatalf("claim must select exactly the old staged row, got %d rows", len(claimed))
	}
	// UPDATE ... RETURNING yields the post-transition projection.
	if claimed[0].State != UploadStateRevoked {
		t.Fatalf("claimed row state: got %q want revoked (post-transition projection)", claimed[0].State)
	}
	if claimed[0].ObjectKey != "chat/old-staged" {
		t.Fatalf("claimed object key: got %q", claimed[0].ObjectKey)
	}

	for key, wantState := range map[string]string{
		"chat/old-staged":   "revoked",
		"chat/fresh-staged": "staged",
		"chat/old-attached": "attached",
		"chat/old-revoked":  "revoked",
	} {
		var state string
		if err := repo.pool.QueryRow(ctx, `SELECT state FROM chat_uploads WHERE object_key = $1`, key).Scan(&state); err != nil {
			t.Fatalf("reread state for %s: %v", key, err)
		}
		if state != wantState {
			t.Fatalf("state for %s: got %q want %q", key, state, wantState)
		}
	}

	// The claim transitions rows out of staged, so an immediate re-claim must
	// find nothing: cleanup never reprocesses a claimed row.
	reclaimed, err := source.ClaimExpiredStaged(ctx, time.Now().UTC().Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("reclaim expired staged: %v", err)
	}
	if len(reclaimed) != 0 {
		t.Fatalf("immediate reclaim must be empty, got %d rows", len(reclaimed))
	}
}

func TestMediaCleanup_ClaimExpiredStaged_LimitsBatchSize(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	uploader := seedUser(t, repo, "cleanup-limited")
	circleID := seedCircle(t, repo, "Cleanup Limited Circle", uploader)
	source := NewPoolStagedUploadSource(repo.pool)

	old := time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond)
	for i := 0; i < 3; i++ {
		seedChatUploadWithState(t, repo, uploader, circleID, "chat/batch-"+uuid.NewString(), "staged", old)
	}

	claimed, err := source.ClaimExpiredStaged(ctx, time.Now().UTC().Add(-24*time.Hour), 2)
	if err != nil {
		t.Fatalf("claim with limit: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claim must respect the batch limit, got %d want 2", len(claimed))
	}
}
