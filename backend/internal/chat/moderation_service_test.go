//go:build integration

package chat

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestModerationService_SelfDeleteUsesAuthoritativeDatabaseTime(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "moderation-self-delete")
	circle := seedCircle(t, repo, "Moderation self-delete", sender)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, sender, "student", joined)

	messageID := seedMessage(t, repo, sender, &circle, nil, time.Now().UTC(), "remove me")
	service := NewModerationService(repo, nil)
	if err := service.Delete(ctx, sender, circle, messageID); err != nil {
		t.Fatalf("timely self-delete: %v", err)
	}

	var deletedAt *time.Time
	if err := repo.pool.QueryRow(ctx, `SELECT deleted_at FROM messages WHERE id = $1`, messageID).Scan(&deletedAt); err != nil {
		t.Fatalf("load deleted message: %v", err)
	}
	if deletedAt == nil {
		t.Fatal("deleted_at is nil")
	}
}

func TestModerationService_LateSelfDeleteReturnsConflictAndKeepsContent(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "moderation-late-delete")
	circle := seedCircle(t, repo, "Moderation late-delete", sender)
	seedMember(t, repo, circle, sender, "student", time.Now().UTC().Add(-2*time.Hour))
	messageID := seedMessage(t, repo, sender, &circle, nil, time.Now().UTC().Add(-11*time.Minute), "keep me")

	err := NewModerationService(repo, nil).Delete(ctx, sender, circle, messageID)
	if !errors.Is(err, ErrDirectDeleteConflict) {
		t.Fatalf("late self-delete error = %v, want conflict", err)
	}
}

func TestModerationService_TeacherDeleteIsIdempotentAndAuditedOnce(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "moderation-teacher")
	sender := seedUser(t, repo, "moderation-sender")
	circle := seedCircle(t, repo, "Moderation teacher-delete", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, teacher, "teacher", joined)
	seedMember(t, repo, circle, sender, "student", joined)
	messageID := seedMessage(t, repo, sender, &circle, nil, time.Now().UTC().Add(-time.Hour), "teacher remove")
	service := NewModerationService(repo, nil)
	if err := service.Delete(ctx, teacher, circle, messageID); err != nil {
		t.Fatalf("teacher delete: %v", err)
	}
	if err := service.Delete(ctx, teacher, circle, messageID); err != nil {
		t.Fatalf("teacher delete replay: %v", err)
	}
	var count int
	if err := repo.pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_moderation_audits WHERE message_id = $1`, messageID).Scan(&count); err != nil {
		t.Fatalf("count moderation audits: %v", err)
	}
	if count != 1 {
		t.Fatalf("audit count = %d, want 1", count)
	}
}

func TestModerationService_DMOwnDelete(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "moderation-dm-teacher")
	student := seedUser(t, repo, "moderation-dm-student")
	circle := seedCircle(t, repo, "Moderation DM", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, teacher, "teacher", joined)
	seedMember(t, repo, circle, student, "student", joined)
	messageID := seedMessage(t, repo, student, nil, &teacher, time.Now().UTC(), "dm remove")
	if err := NewModerationService(repo, nil).Delete(ctx, student, uuid.Nil, messageID); err != nil {
		t.Fatalf("DM own delete: %v", err)
	}
}
