//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

type failingChatMediaClient struct {
	*minio.Client
	err error
}

func (c *failingChatMediaClient) RemoveObject(context.Context, string, string, minio.RemoveObjectOptions) error {
	return c.err
}

func TestChatMediaRevocation_ReconcilesLatestMarkerByMessageState(t *testing.T) {
	env := setupChatMediaEnv(t)
	key, err := env.store.Stage(context.Background(), chat.StageInput{UploadID: uuid.New(), Type: chat.MessageTypeImage, MIMEType: "image/png", SizeBytes: 4, Body: strings.NewReader("data")})
	if err != nil {
		t.Fatalf("stage object: %v", err)
	}
	if err := env.store.ApplyDeleteMarker(context.Background(), key); err != nil {
		t.Fatalf("apply marker: %v", err)
	}
	if err := env.store.RemoveLatestDeleteMarker(context.Background(), key); err != nil {
		t.Fatalf("remove latest marker: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := env.minio.StatObject(ctx, env.bucket, key, minio.StatObjectOptions{}); err != nil {
		t.Fatalf("restored versionless object: %v", err)
	}
}

func TestChatMediaRevocation_RepeatedMarkerApplicationIsIdempotent(t *testing.T) {
	env := setupChatMediaEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key, err := env.store.Stage(ctx, chat.StageInput{UploadID: uuid.New(), Type: chat.MessageTypeImage, MIMEType: "image/png", SizeBytes: 4, Body: strings.NewReader("data")})
	if err != nil {
		t.Fatalf("stage object: %v", err)
	}
	if err := env.store.ApplyDeleteMarker(ctx, key); err != nil {
		t.Fatalf("first marker: %v", err)
	}
	if err := env.store.ApplyDeleteMarker(ctx, key); err != nil {
		t.Fatalf("repeated marker: %v", err)
	}

	markers := 0
	for object := range env.minio.ListObjects(ctx, env.bucket, minio.ListObjectsOptions{Prefix: key, Recursive: true, WithVersions: true}) {
		if object.Err != nil {
			t.Fatalf("list versions: %v", object.Err)
		}
		if object.Key == key && object.IsDeleteMarker {
			markers++
		}
	}
	if markers != 1 {
		t.Fatalf("delete markers=%d, want exactly one after repeated reconciliation", markers)
	}
}

func TestChatMediaRevocation_RecoveryRemovesExactMarkerAndRetainsBytes(t *testing.T) {
	env := setupChatMediaEnv(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key, err := env.store.Stage(ctx, chat.StageInput{UploadID: uuid.New(), Type: chat.MessageTypeImage, MIMEType: "image/png", SizeBytes: 4, Body: strings.NewReader("data")})
	if err != nil {
		t.Fatalf("stage object: %v", err)
	}
	if err := env.store.ApplyDeleteMarker(ctx, key); err != nil {
		t.Fatalf("marker: %v", err)
	}
	var markerID string
	for object := range env.minio.ListObjects(ctx, env.bucket, minio.ListObjectsOptions{Prefix: key, Recursive: true, WithVersions: true}) {
		if object.Err != nil {
			t.Fatalf("list versions: %v", object.Err)
		}
		if object.Key == key && object.IsDeleteMarker {
			markerID = object.VersionID
		}
	}
	if markerID == "" {
		t.Fatal("missing internal marker version ID")
	}
	if err := env.store.RemoveDeleteMarker(ctx, key, markerID); err != nil {
		t.Fatalf("remove exact marker: %v", err)
	}
	if _, err := env.minio.StatObject(ctx, env.bucket, key, minio.StatObjectOptions{}); err != nil {
		t.Fatalf("retained prior bytes are not versionlessly accessible: %v", err)
	}
}

func TestChatMediaRevocation_DeletionMarkerFailureLeavesPostgresMessageActive(t *testing.T) {
	env := setupChatMediaEnv(t)
	ctx := context.Background()
	sender := seedChatMediaUser(t, env.pool, "marker-failure-sender")
	circle := seedChatMediaCircle(t, env.pool, sender)
	seedChatMediaMember(t, env.pool, circle, sender, "teacher", time.Now().Add(-time.Hour))
	_, message := stageAndSendChatMedia(t, env, ctx, sender, circle, chat.MessageTypeImage, chatMediaPNG(t), 0, "marker-failure.png", "marker-failure")

	mediaErr := errors.New("minio marker failure")
	failingStore := chat.NewMediaStore(&failingChatMediaClient{Client: env.minio, err: mediaErr}, env.bucket, 10*time.Second)
	if err := chat.NewModerationService(env.repo, failingStore).Delete(ctx, sender, circle, message.ID); !errors.Is(err, mediaErr) {
		t.Fatalf("delete error=%v, want wrapped MinIO error", err)
	}
	var state string
	if err := env.pool.QueryRow(ctx, `SELECT CASE WHEN deleted_at IS NULL THEN 'active' ELSE 'deleted' END FROM messages WHERE id = $1`, message.ID).Scan(&state); err != nil {
		t.Fatalf("read message state: %v", err)
	}
	if state != string(chat.MessageStateActive) {
		t.Fatalf("message state=%q, want active after marker failure", state)
	}
}

func TestChatMediaRevocation_UnauthorizedRenewalIsDeniedWithoutEnumeration(t *testing.T) {
	env := setupChatMediaEnv(t)
	ctx := context.Background()
	sender := seedChatMediaUser(t, env.pool, "revocation-renewal-sender")
	member := seedChatMediaUser(t, env.pool, "revocation-renewal-member")
	outsider := seedChatMediaUser(t, env.pool, "revocation-renewal-outsider")
	circle := seedChatMediaCircle(t, env.pool, sender)
	seedChatMediaMember(t, env.pool, circle, sender, "teacher", time.Now().Add(-time.Hour))
	seedChatMediaMember(t, env.pool, circle, member, "student", time.Now().Add(-time.Hour))
	_, message := stageAndSendChatMedia(t, env, ctx, sender, circle, chat.MessageTypeImage, chatMediaPNG(t), 0, "renewal-guard.png", "revocation-renewal-guard")

	if _, err := env.service.RenewMediaURL(ctx, outsider, message.ID); !errors.Is(err, chat.ErrMessageNotVisible) {
		t.Fatalf("outsider renewal err=%v, want ErrMessageNotVisible", err)
	}
	if _, err := env.service.RenewMediaURL(ctx, member, uuid.New()); !errors.Is(err, chat.ErrMessageNotVisible) {
		t.Fatalf("unknown message renewal err=%v, want ErrMessageNotVisible", err)
	}
}

func TestChatMediaRevocation_CrashAfterMarkerBeforeCommitIsRepaired(t *testing.T) {
	env := setupChatMediaEnv(t)
	ctx := context.Background()
	sender := seedChatMediaUser(t, env.pool, "revocation-crash-sender")
	circle := seedChatMediaCircle(t, env.pool, sender)
	seedChatMediaMember(t, env.pool, circle, sender, "teacher", time.Now().Add(-time.Hour))
	staged, message := stageAndSendChatMedia(t, env, ctx, sender, circle, chat.MessageTypeImage, chatMediaPNG(t), 0, "crash-window.png", "revocation-crash-window")

	// Simulate the process boundary after MinIO marker creation and before the
	// PostgreSQL soft-delete commit. The durable row must still be active and
	// the reconciler must remove only the interrupted marker.
	if err := env.store.ApplyDeleteMarker(ctx, staged.ObjectKey); err != nil {
		t.Fatalf("apply interrupted marker: %v", err)
	}
	var deletedAt *time.Time
	if err := env.pool.QueryRow(ctx, `SELECT deleted_at FROM messages WHERE id = $1`, message.ID).Scan(&deletedAt); err != nil {
		t.Fatalf("read crash-window message: %v", err)
	}
	if deletedAt != nil {
		t.Fatalf("crash-window message deleted_at=%v, want NULL", deletedAt)
	}

	if err := chat.NewMediaReconciler(env.repo, env.store).Reconcile(ctx, message.ID); err != nil {
		t.Fatalf("reconcile interrupted deletion: %v", err)
	}
	if status, body, _ := chatMediaGet(t, mustPresignChatMedia(t, env, ctx, staged.ObjectKey)); status != 200 || !bytes.Equal(body, chatMediaPNG(t)) {
		t.Fatalf("recovered media status=%d bytes=%d, want 200 and retained bytes", status, len(body))
	}
}

func TestChatMediaRevocation_ReconcilerRepairsActiveAndDeletedMessageStates(t *testing.T) {
	env := setupChatMediaEnv(t)
	ctx := context.Background()
	sender := seedChatMediaUser(t, env.pool, "reconciler-sender")
	circle := seedChatMediaCircle(t, env.pool, sender)
	seedChatMediaMember(t, env.pool, circle, sender, "teacher", time.Now().Add(-time.Hour))
	reconciler := chat.NewMediaReconciler(env.repo, env.store)

	activeUpload, activeMessage := stageAndSendChatMedia(t, env, ctx, sender, circle, chat.MessageTypeImage, chatMediaPNG(t), 0, "active-recovery.png", "active-recovery")
	if err := env.store.ApplyDeleteMarker(ctx, activeUpload.ObjectKey); err != nil {
		t.Fatalf("seed interrupted marker: %v", err)
	}
	if err := reconciler.Reconcile(ctx, activeMessage.ID); err != nil {
		t.Fatalf("reconcile active message: %v", err)
	}
	if status, _, _ := chatMediaGet(t, mustPresignChatMedia(t, env, ctx, activeUpload.ObjectKey)); status != 200 {
		t.Fatalf("active recovery GET status=%d, want 200", status)
	}

	deletedUpload, deletedMessage := stageAndSendChatMedia(t, env, ctx, sender, circle, chat.MessageTypeImage, chatMediaPNG(t), 0, "deleted-recovery.png", "deleted-recovery")
	if _, err := env.pool.Exec(ctx, `UPDATE messages SET deleted_at = NOW() WHERE id = $1`, deletedMessage.ID); err != nil {
		t.Fatalf("mark message deleted: %v", err)
	}
	if err := reconciler.Reconcile(ctx, deletedMessage.ID); err != nil {
		t.Fatalf("reconcile deleted message: %v", err)
	}
	if status, _, _ := chatMediaGet(t, mustPresignChatMedia(t, env, ctx, deletedUpload.ObjectKey)); status != 404 {
		t.Fatalf("deleted repair GET status=%d, want 404", status)
	}
}

func mustPresignChatMedia(t *testing.T, env *chatMediaEnv, ctx context.Context, key string) string {
	t.Helper()
	url, err := env.store.PresignGet(ctx, key, time.Minute)
	if err != nil {
		t.Fatalf("presign %s: %v", key, err)
	}
	return url.String()
}
