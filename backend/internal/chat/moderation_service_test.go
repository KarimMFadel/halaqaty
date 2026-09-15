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
	var content string
	var deletedAt *time.Time
	if err := repo.pool.QueryRow(ctx, `SELECT content, deleted_at FROM messages WHERE id = $1`, messageID).Scan(&content, &deletedAt); err != nil {
		t.Fatalf("load late message: %v", err)
	}
	if content != "keep me" || deletedAt != nil {
		t.Fatalf("late delete changed durable message: content=%q deleted_at=%v", content, deletedAt)
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

func TestModerationService_ConcurrentTeacherDeletesCreateOneAudit(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "moderation-race-teacher")
	sender := seedUser(t, repo, "moderation-race-sender")
	circle := seedCircle(t, repo, "Moderation race", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, teacher, "teacher", joined)
	seedMember(t, repo, circle, sender, "student", joined)
	messageID := seedMessage(t, repo, sender, &circle, nil, time.Now().UTC(), "race remove")

	service := NewModerationService(repo, nil)
	results := make(chan error, 2)
	for range 2 {
		go func() { results <- service.Delete(ctx, teacher, circle, messageID) }()
	}
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent teacher delete: %v", err)
		}
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

// Regression: an idempotent deletion replay must not bypass current authority.
func TestModerationService_DeletedMessageReplayStillRequiresAuthority(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "moderation-replay-teacher")
	sender := seedUser(t, repo, "moderation-replay-sender")
	stranger := seedUser(t, repo, "moderation-replay-stranger")
	circle := seedCircle(t, repo, "Moderation replay authority", teacher)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, teacher, "teacher", joined)
	seedMember(t, repo, circle, sender, "student", joined)
	seedMember(t, repo, circle, stranger, "student", joined)
	message := seedMessage(t, repo, sender, &circle, nil, time.Now().UTC(), "deleted")
	service := NewModerationService(repo, nil)
	if err := service.Delete(ctx, teacher, circle, message); err != nil {
		t.Fatalf("teacher deletion: %v", err)
	}
	for _, tc := range []struct {
		name          string
		actor, circle uuid.UUID
	}{
		{"unrelated member", stranger, circle},
		{"wrong conversation", teacher, uuid.New()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := service.Delete(ctx, tc.actor, tc.circle, message); !errors.Is(err, ErrMessageNotVisible) {
				t.Fatalf("unauthorized replay error=%v, want non-enumerating denial", err)
			}
		})
	}
}

func TestModerationService_AttachedMediaWithoutStoreFailsClosed(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "moderation-media-no-store-sender")
	peer := seedUser(t, repo, "moderation-media-no-store-peer")
	circle := seedCircle(t, repo, "Moderation media no store", sender)
	joined := time.Now().UTC().Add(-time.Hour)
	seedMember(t, repo, circle, sender, "teacher", joined)
	seedMember(t, repo, circle, peer, "student", joined)
	upload, err := repo.InsertUpload(ctx, Upload{
		UploaderID: sender, AuthorizationCircleID: circle, ObjectKey: chatObjectKey(uuid.New()),
		MIMEType: "image/png", OriginalFileName: "image.png", SizeBytes: 4,
	})
	if err != nil {
		t.Fatalf("stage upload: %v", err)
	}
	message, err := func() (Message, error) {
		var message Message
		err := repo.WithTx(ctx, func(tx *Tx) error {
			var err error
			message, _, err = tx.InsertMessage(ctx, MessageInput{SenderID: sender, CircleID: &circle, Type: MessageTypeImage, UploadID: &upload.ID, IdempotencyKey: "media-no-store"})
			if err != nil {
				return err
			}
			_, err = tx.AttachUpload(ctx, upload.ID)
			return err
		})
		return message, err
	}()
	if err != nil {
		t.Fatalf("attach fixture: %v", err)
	}

	if err := NewModerationService(repo, nil).Delete(ctx, sender, circle, message.ID); err == nil {
		t.Fatal("attached-media deletion must fail when the media store is unavailable")
	}
	var deletedAt *time.Time
	if err := repo.pool.QueryRow(ctx, `SELECT deleted_at FROM messages WHERE id = $1`, message.ID).Scan(&deletedAt); err != nil {
		t.Fatalf("load message: %v", err)
	}
	if deletedAt != nil {
		t.Fatal("fail-closed media deletion must not soft-delete the database row")
	}
}
