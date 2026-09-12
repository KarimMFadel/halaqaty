//go:build integration

package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// TestPresenceService_MarkGroupMessageReadRejectsArchivedCircle proves retained
// archived-circle history stays read-only: marking a visible message read is a
// mutation and must fail before any persistence is attempted.
func TestPresenceService_MarkGroupMessageReadRejectsArchivedCircle(t *testing.T) {
	repo := newChatRepo(t)
	service := NewPresenceService(repo, rbac.NewRepository(repo.pool))
	ctx := context.Background()
	senderID := seedUser(t, repo, "presence-archived-sender")
	readerID := seedUser(t, repo, "presence-archived-reader")
	circleID := seedCircle(t, repo, "Presence Archived Circle", senderID)
	joinedAt := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circleID, senderID, "teacher", joinedAt)
	seedMember(t, repo, circleID, readerID, "student", joinedAt)
	messageID := seedMessage(t, repo, senderID, &circleID, nil, joinedAt.Add(time.Minute), "retained history")
	if _, err := repo.pool.Exec(ctx, `UPDATE circles SET is_archived = TRUE WHERE id = $1`, circleID); err != nil {
		t.Fatalf("archive circle: %v", err)
	}

	err := service.MarkGroupMessageRead(ctx, readerID, circleID, messageID)
	if !errors.Is(err, ErrCircleArchived) {
		t.Fatalf("MarkGroupMessageRead() error = %v, want ErrCircleArchived", err)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM message_reads`); n != 0 {
		t.Fatalf("archived mark-read must persist nothing: got %d read facts", n)
	}
}
