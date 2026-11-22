//go:build integration

package chat

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
)

// perKeyFailStore fails ApplyDeleteMarker only for configured object keys so a
// test can model rows whose reconciliation never completes.
type perKeyFailStore struct {
	failing map[string]error
	applied []string
}

func (s *perKeyFailStore) Put(context.Context, ObjectPutInput) error { return nil }
func (s *perKeyFailStore) PresignGet(context.Context, string, time.Duration) (*url.URL, error) {
	return nil, nil
}
func (s *perKeyFailStore) ApplyDeleteMarker(_ context.Context, key string) error {
	s.applied = append(s.applied, key)
	return s.failing[key]
}
func (s *perKeyFailStore) RemoveLatestDeleteMarker(context.Context, string) error { return nil }
func (s *perKeyFailStore) EnsureChatBucketVersioned(context.Context) error        { return nil }

func seedReconcilableMedia(t *testing.T, repo *Repository, circle, sender, messageID, uploadID uuid.UUID) string {
	t.Helper()
	ctx := context.Background()
	upload, err := repo.InsertUpload(ctx, Upload{
		ID:                    uploadID,
		UploaderID:            sender,
		AuthorizationCircleID: circle,
		ObjectKey:             "chat/" + uploadID.String(),
		MIMEType:              "image/png",
		OriginalFileName:      "reconcile.png",
		SizeBytes:             4,
	})
	if err != nil {
		t.Fatalf("stage reconcile upload: %v", err)
	}
	if err := repo.WithTx(ctx, func(tx *Tx) error {
		_, err := tx.AttachUpload(ctx, upload.ID)
		return err
	}); err != nil {
		t.Fatalf("attach reconcile upload: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `
		INSERT INTO messages (id, circle_id, sender_id, idempotency_key, message_type, upload_id)
		VALUES ($1, $2, $3, $4, 'image', $5)
	`, messageID, circle, sender, "reconcile-"+messageID.String(), upload.ID); err != nil {
		t.Fatalf("seed reconcile message: %v", err)
	}
	return upload.ObjectKey
}

// TestMediaReconciler_SweepPaginatesPastStuckFirstPage proves the sweep keeps
// advancing: rows on the first page whose reconciliation always fails must not
// starve later pages, and the bounded loop must terminate.
func TestMediaReconciler_SweepPaginatesPastStuckFirstPage(t *testing.T) {
	repo := newChatRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sender := seedUser(t, repo, "sweep-sender")
	circle := seedCircle(t, repo, "Sweep Circle", sender)
	seedMember(t, repo, circle, sender, "teacher", time.Now().UTC().Add(-time.Hour))

	// Explicit ascending ids make the page boundary deterministic.
	messageIDs := []uuid.UUID{
		uuid.MustParse("00000000-0000-0000-0000-000000000001"),
		uuid.MustParse("00000000-0000-0000-0000-000000000002"),
		uuid.MustParse("00000000-0000-0000-0000-000000000003"),
		uuid.MustParse("00000000-0000-0000-0000-000000000004"),
		uuid.MustParse("00000000-0000-0000-0000-000000000005"),
	}
	keys := make([]string, 0, len(messageIDs))
	for i, messageID := range messageIDs {
		uploadID := uuid.MustParse(fmt.Sprintf("10000000-0000-0000-0000-%012d", i+1))
		keys = append(keys, seedReconcilableMedia(t, repo, circle, sender, messageID, uploadID))
	}
	if _, err := repo.pool.Exec(ctx, `UPDATE messages SET deleted_at = NOW() WHERE circle_id = $1`, circle); err != nil {
		t.Fatalf("mark reconcile messages deleted: %v", err)
	}

	stuck := errors.New("object store stuck")
	fake := &perKeyFailStore{failing: map[string]error{keys[0]: stuck, keys[1]: stuck}}
	reconciler := NewMediaReconciler(repo, NewMediaStore(fake, 5*time.Second))

	err := reconciler.Sweep(ctx, 2)
	if !errors.Is(err, stuck) {
		t.Fatalf("sweep error: got %v want it to wrap the stuck-store failure", err)
	}
	if len(fake.applied) != len(keys) {
		t.Fatalf("sweep processed %d keys, want all %d (later pages must not be starved)", len(fake.applied), len(keys))
	}
}
