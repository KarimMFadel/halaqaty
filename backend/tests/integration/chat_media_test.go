//go:build integration

// T047 — behavioral integration tests for F-004 US3 chat media against real
// MinIO and real PostgreSQL: private objects with no public access, supported
// media families staged end-to-end, expired versus renewed versionless
// presigned URLs, delete-marker revocation of previously issued versionless
// URLs, non-enumerating unauthorized renewal, and object cleanup after failed
// database finalization. The REST surface itself is pinned by the contract
// suite (tests/contract/chat_media_contract_test.go); this file proves the
// service and object-store behaviors the stubs cannot.
//
// Infrastructure is discovered through the CHAT_MEDIA_* environment variables
// (same names as production configuration) with local compose defaults, and
// every test skips cleanly when PostgreSQL or MinIO is unreachable so CI
// without object storage stays green.

package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// Real-media fixtures for the three supported families (ADR-022: valid
// generated media, never header-only placeholders). Audio and PDF bytes come
// from the chat package's committed testdata; images are encoded in-process
// with the standard library. Staging runs the real parsers, so voice and
// file subtests skip cleanly when ffprobe/qpdf are not installed, mirroring
// the clean-skip posture for unreachable infrastructure.

// chatMediaFixture loads one committed binary fixture from the chat
// package's testdata directory.
func chatMediaFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "internal", "chat", "testdata", name))
	if err != nil {
		t.Fatalf("read chat media fixture %s: %v", name, err)
	}
	return data
}

// chatMediaVoice loads a real OGG container (mono 440 Hz FLAC tone, parsed
// duration 2.000000s). Voice staging requires ffprobe.
func chatMediaVoice(t *testing.T) []byte {
	t.Helper()
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not installed; real-audio staging requires the ADR-022 tools")
	}
	return chatMediaFixture(t, "voice_2s.ogg")
}

// chatMediaPDF loads a real one-page PDF with a correct xref table that
// passes qpdf --check. PDF staging requires qpdf.
func chatMediaPDF(t *testing.T) []byte {
	t.Helper()
	if _, err := exec.LookPath("qpdf"); err != nil {
		t.Skip("qpdf not installed; real-PDF staging requires the ADR-022 tools")
	}
	return chatMediaFixture(t, "valid.pdf")
}

// chatMediaPNG encodes a genuinely decodable PNG; no external parser is
// involved, so image staging runs everywhere.
func chatMediaPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatalf("encode PNG fixture: %v", err)
	}
	return buf.Bytes()
}

// chatMediaEnv bundles the real fixtures one chat-media integration test runs
// against: an isolated PostgreSQL schema with the full migration chain, and an
// isolated versioned MinIO bucket behind the real chat.MediaStore, cleaner,
// and upload service.
type chatMediaEnv struct {
	pool    *pgxpool.Pool
	repo    *chat.Repository
	members *rbac.Repository
	store   *chat.MediaStore
	cleaner *chat.Cleaner
	service *chat.UploadService
	minio   *minio.Client
	bucket  string
}

// setupChatMediaEnv builds the isolated schema and bucket. It skips with a
// clear message when DATABASE_URL is unset or MinIO is unreachable, but fails
// hard once infrastructure is reachable: a reachable-but-broken bucket is a
// real defect, not an environment gap.
func setupChatMediaEnv(t *testing.T) *chatMediaEnv {
	t.Helper()
	ctx := context.Background()

	admin := openPool(t, ctx)
	t.Cleanup(admin.Close)
	schema := uniqueSchemaName(t)
	createConn := acquireConn(t, admin, ctx)
	createSchema(t, createConn, ctx, schema)
	createConn.Release()
	t.Cleanup(func() { dropSchema(t, admin, ctx, schema) })
	applyConn := acquireConn(t, admin, ctx)
	defer applyConn.Release()
	if _, err := applyConn.Exec(ctx, "SET search_path TO "+schema); err != nil {
		t.Fatalf("set search_path to %s: %v", schema, err)
	}
	applyRealTimeChatMigrations(t, applyConn, ctx)
	pool := openSchemaPool(t, ctx, schema)
	t.Cleanup(pool.Close)

	client, bucket := setupChatMediaMinio(t)
	store := chat.NewMediaStore(client, bucket, 10*time.Second)
	// The same startup gate main.go fail-fasts on must pass for the fresh
	// bucket: versioned, reachable, private by default.
	if err := store.EnsureChatBucketVersioned(ctx); err != nil {
		t.Fatalf("verify chat media bucket versioned: %v", err)
	}

	repo := chat.NewRepository(pool)
	members := rbac.NewRepository(pool)
	cleaner := chat.NewCleaner(chat.NewPoolStagedUploadSource(pool), store)
	return &chatMediaEnv{
		pool:    pool,
		repo:    repo,
		members: members,
		store:   store,
		cleaner: cleaner,
		service: chat.NewUploadService(repo, members, store, cleaner, new(metrics.ChatMetrics), nil),
		minio:   client,
		bucket:  bucket,
	}
}

// setupChatMediaMinio creates one isolated versioned bucket per test and
// returns the raw client and bucket name. Unreachable object storage skips the
// test instead of failing CI.
func setupChatMediaMinio(t *testing.T) (*minio.Client, string) {
	t.Helper()
	endpoint := os.Getenv("CHAT_MEDIA_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}
	accessKeyID := os.Getenv("CHAT_MEDIA_ACCESS_KEY_ID")
	if accessKeyID == "" {
		accessKeyID = "minioadmin"
	}
	secret := os.Getenv("CHAT_MEDIA_SECRET_ACCESS_KEY")
	if secret == "" {
		secret = "minioadmin"
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secret, ""),
		Secure: strings.EqualFold(os.Getenv("CHAT_MEDIA_USE_SSL"), "true"),
	})
	if err != nil {
		t.Fatalf("init MinIO client for %s: %v", endpoint, err)
	}
	bucket := fmt.Sprintf("halaqaty-chat-it-%d", time.Now().UnixNano())

	probeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := client.BucketExists(probeCtx, bucket); err != nil {
		t.Skipf("MinIO unreachable at %s (%v); start the compose minio service or set CHAT_MEDIA_ENDPOINT", endpoint, err)
	}

	ctx := context.Background()
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatalf("create integration bucket %s: %v", bucket, err)
	}
	if err := client.EnableVersioning(ctx, bucket); err != nil {
		t.Fatalf("enable versioning on %s: %v", bucket, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_ = client.RemoveBucketWithOptions(cleanupCtx, bucket, minio.RemoveBucketOptions{ForceDelete: true})
	})
	return client, bucket
}

// seedChatMediaUser inserts one user row and returns its id.
func seedChatMediaUser(t *testing.T, pool *pgxpool.Pool, label string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id
	`, "firebase-chatmedia-"+label, label+"-chatmedia@example.com").Scan(&id); err != nil {
		t.Fatalf("seed chat media user %s: %v", label, err)
	}
	return id
}

// seedChatMediaCircle inserts one circle owned by teacherID.
func seedChatMediaCircle(t *testing.T, pool *pgxpool.Pool, teacherID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('Chat Media Circle', $1, $2)
		RETURNING id
	`, teacherID, "HLQ-"+uuid.NewString()[:8]).Scan(&id); err != nil {
		t.Fatalf("seed chat media circle: %v", err)
	}
	return id
}

// seedChatMediaMember inserts one circle membership with an explicit joined_at
// so membership-period visibility can be exercised.
func seedChatMediaMember(t *testing.T, pool *pgxpool.Pool, circleID, userID uuid.UUID, role string, joinedAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO circle_members (circle_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, $4)
	`, circleID, userID, role, joinedAt); err != nil {
		t.Fatalf("seed chat media member %s: %v", userID, err)
	}
}

// stageChatMedia stages one upload through the real service and fails the test
// on the first unexpected error.
func stageChatMedia(t *testing.T, env *chatMediaEnv, ctx context.Context, uploaderID, circleID uuid.UUID, mediaType chat.MessageType, payload []byte, duration int, fileName string) chat.StagedUpload {
	t.Helper()
	staged, err := env.service.Stage(ctx, chat.StageUploadInput{
		UploaderID:       uploaderID,
		Target:           chat.UploadTarget{CircleID: &circleID},
		MediaType:        mediaType,
		Data:             payload,
		DeclaredFileName: fileName,
		DurationSeconds:  duration,
	})
	if err != nil {
		t.Fatalf("stage %s upload: %v", mediaType, err)
	}
	return staged
}

// stageAndSendChatMedia runs the full US3 send pipeline — stage then attach as
// one group media message — against real infrastructure.
func stageAndSendChatMedia(t *testing.T, env *chatMediaEnv, ctx context.Context, senderID, circleID uuid.UUID, mediaType chat.MessageType, payload []byte, duration int, fileName, idempotencyKey string) (chat.StagedUpload, chat.Message) {
	t.Helper()
	staged := stageChatMedia(t, env, ctx, senderID, circleID, mediaType, payload, duration, fileName)
	sent, err := env.service.SendGroupMedia(ctx, chat.SendGroupMediaInput{
		SenderID:       senderID,
		CircleID:       circleID,
		UploadID:       staged.UploadID,
		MessageType:    mediaType,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		t.Fatalf("send %s media message: %v", mediaType, err)
	}
	return staged, sent
}

// chatMediaGet fetches one object URL over plain HTTP and returns the status,
// body, and content type — the same client path a mobile consumer uses.
func chatMediaGet(t *testing.T, objectURL string) (int, []byte, string) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(objectURL)
	if err != nil {
		t.Fatalf("GET %s: %v", objectURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s body: %v", objectURL, err)
	}
	return resp.StatusCode, body, resp.Header.Get("Content-Type")
}

// chatMediaPlainURL strips the signature query from a presigned URL, yielding
// the unsigned object address an anonymous client would try.
func chatMediaPlainURL(signed *url.URL) string {
	plain := *signed
	plain.RawQuery = ""
	plain.Fragment = ""
	return plain.String()
}

// assertChatMediaSevenDayTTL fails unless the expiry is within one minute of
// the documented seven-day presigned lifetime (FR-024).
func assertChatMediaSevenDayTTL(t *testing.T, field string, expiresAt time.Time) {
	t.Helper()
	ttl := time.Until(expiresAt)
	if ttl < chat.MediaURLTTL-time.Minute || ttl > chat.MediaURLTTL+time.Minute {
		t.Fatalf("%s lifetime=%s, want %s±1m", field, ttl, chat.MediaURLTTL)
	}
}

// chatMediaLatestDeleteMarkerKey lists every object version under chat/ and
// returns the key of the single latest delete marker, proving an object was
// revoked by marker rather than physically destroyed (ADR-021).
func chatMediaLatestDeleteMarkerKey(t *testing.T, env *chatMediaEnv, ctx context.Context) string {
	t.Helper()
	found := ""
	for object := range env.minio.ListObjects(ctx, env.bucket, minio.ListObjectsOptions{
		Prefix:       "chat/",
		Recursive:    true,
		WithVersions: true,
	}) {
		if object.Err != nil {
			t.Fatalf("list object versions: %v", object.Err)
		}
		if object.IsLatest && object.IsDeleteMarker {
			if found != "" {
				t.Fatalf("multiple latest delete markers, want exactly one: %s and %s", found, object.Key)
			}
			found = object.Key
		}
	}
	if found == "" {
		t.Fatal("no latest delete marker: the staged object was not revoked by a delete marker")
	}
	return found
}

// chatMediaFailingUploadRepo delegates every operation to the real repository
// except InsertUploadWithinBudget, which fails — the exact database-finalization
// seam Stage calls, whose cleanup path must revoke the already-written object.
// Infrastructure-level DB failures cannot be timed mid-call, so the failure is
// injected at this one seam while PostgreSQL and MinIO stay real everywhere
// else.
type chatMediaFailingUploadRepo struct {
	*chat.Repository
	failErr error
}

// InsertUploadWithinBudget simulates the failed database finalization of a
// staged object.
func (r chatMediaFailingUploadRepo) InsertUploadWithinBudget(context.Context, chat.Upload, time.Time) (chat.Upload, error) {
	return chat.Upload{}, r.failErr
}

// TestChatMediaStagingAndAccess proves the object-store contract of staged
// chat media end-to-end: every supported family stages with correct database
// metadata and readable presigned content, objects stay private without a
// signature, and a delete marker revokes previously issued versionless URLs.
func TestChatMediaStagingAndAccess(t *testing.T) {
	env := setupChatMediaEnv(t)
	ctx := context.Background()
	uploader := seedChatMediaUser(t, env.pool, "stager")
	circle := seedChatMediaCircle(t, env.pool, uploader)
	seedChatMediaMember(t, env.pool, circle, uploader, "teacher", time.Now().Add(-time.Hour))

	t.Run("supported families stage end-to-end", func(t *testing.T) {
		cases := []struct {
			name         string
			mediaType    chat.MessageType
			payload      func(*testing.T) []byte
			mime         string
			duration     int
			declaredName string
			storedName   string
		}{
			{name: "voice", mediaType: chat.MessageTypeVoice, payload: chatMediaVoice, mime: "audio/ogg", duration: 42,
				declaredName: `..\..\recitation note.ogg`, storedName: "recitation note.ogg"},
			{name: "image", mediaType: chat.MessageTypeImage, payload: chatMediaPNG, mime: "image/png", duration: 0,
				declaredName: "whiteboard.png", storedName: "whiteboard.png"},
			{name: "file", mediaType: chat.MessageTypeFile, payload: chatMediaPDF, mime: "application/pdf", duration: 0,
				declaredName: "tajweed-rules.pdf", storedName: "tajweed-rules.pdf"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				payload := tc.payload(t)
				staged := stageChatMedia(t, env, ctx, uploader, circle, tc.mediaType, payload, tc.duration, tc.declaredName)

				if staged.ObjectKey != "chat/"+staged.UploadID.String() {
					t.Fatalf("object key=%q, want chat/%s (server-derived, filename-free)", staged.ObjectKey, staged.UploadID)
				}
				if got, want := staged.URL.Path, "/"+env.bucket+"/"+staged.ObjectKey; got != want {
					t.Fatalf("presigned path=%q, want %q", got, want)
				}
				assertChatMediaSevenDayTTL(t, "staged preview", staged.URLExpiresAt)

				var state, mime, storedName string
				var storedUploader, storedCircle uuid.UUID
				var sizeBytes int64
				if err := env.pool.QueryRow(ctx, `
					SELECT state, mime_type, original_file_name, size_bytes, uploader_id, authorization_circle_id
					FROM chat_uploads WHERE id = $1
				`, staged.UploadID).Scan(&state, &mime, &storedName, &sizeBytes, &storedUploader, &storedCircle); err != nil {
					t.Fatalf("load staged upload row: %v", err)
				}
				if state != string(chat.UploadStateStaged) || mime != tc.mime || storedName != tc.storedName ||
					sizeBytes != int64(len(payload)) || storedUploader != uploader || storedCircle != circle {
					t.Fatalf("staged row state=%q mime=%q name=%q size=%d uploader=%s circle=%s", state, mime, storedName, sizeBytes, storedUploader, storedCircle)
				}

				status, body, contentType := chatMediaGet(t, staged.URL.String())
				if status != http.StatusOK {
					t.Fatalf("presigned GET status=%d, want 200", status)
				}
				if !bytes.Equal(body, payload) {
					t.Fatalf("presigned GET returned %d bytes, want the original %d bytes", len(body), len(payload))
				}
				if contentType != tc.mime {
					t.Fatalf("presigned GET content type=%q, want %q", contentType, tc.mime)
				}
			})
		}
	})

	t.Run("objects are private without a signature", func(t *testing.T) {
		staged := stageChatMedia(t, env, ctx, uploader, circle, chat.MessageTypeImage, chatMediaPNG(t), 0, "private.png")
		status, _, _ := chatMediaGet(t, chatMediaPlainURL(staged.URL))
		if status != http.StatusForbidden {
			t.Fatalf("unsigned GET status=%d, want 403 (private bucket denies anonymous access)", status)
		}
	})

	t.Run("delete marker revokes previously issued versionless url", func(t *testing.T) {
		staged := stageChatMedia(t, env, ctx, uploader, circle, chat.MessageTypeFile, chatMediaPDF(t), 0, "revoke.pdf")
		live, err := env.store.PresignGet(ctx, staged.ObjectKey, chat.MediaURLTTL)
		if err != nil {
			t.Fatalf("presign live url: %v", err)
		}
		if status, _, _ := chatMediaGet(t, live.String()); status != http.StatusOK {
			t.Fatalf("live presigned GET status=%d, want 200 before revocation", status)
		}

		if err := env.store.ApplyDeleteMarker(ctx, staged.ObjectKey); err != nil {
			t.Fatalf("apply delete marker: %v", err)
		}
		// The previously issued long-lived URL must die immediately: no
		// versionId was ever embedded, so the marker shadows the object.
		if status, _, _ := chatMediaGet(t, live.String()); status != http.StatusNotFound {
			t.Fatalf("revoked presigned GET status=%d, want 404 after delete marker", status)
		}
		if key := chatMediaLatestDeleteMarkerKey(t, env, ctx); key != staged.ObjectKey {
			t.Fatalf("delete marker key=%q, want %q (bytes retained as an older version)", key, staged.ObjectKey)
		}
	})
}

// TestChatMediaSendRenewAndCleanup proves the US3 message lifecycle against
// real infrastructure: media sends attach exactly once and replay
// idempotently, renewal reauthorizes viewers non-enumeratingly, expired
// versionless URLs are denied until renewed, and an upload whose database
// finalization fails leaves no consumable object behind.
func TestChatMediaSendRenewAndCleanup(t *testing.T) {
	env := setupChatMediaEnv(t)
	ctx := context.Background()
	uploader := seedChatMediaUser(t, env.pool, "sender")
	member := seedChatMediaUser(t, env.pool, "member")
	outsider := seedChatMediaUser(t, env.pool, "outsider")
	circle := seedChatMediaCircle(t, env.pool, uploader)
	seedChatMediaMember(t, env.pool, circle, uploader, "teacher", time.Now().Add(-2*time.Hour))
	seedChatMediaMember(t, env.pool, circle, member, "student", time.Now().Add(-time.Hour))

	t.Run("media send attaches once and replays idempotently", func(t *testing.T) {
		staged, sent := stageAndSendChatMedia(t, env, ctx, uploader, circle, chat.MessageTypeImage, chatMediaPNG(t), 0, "board.png", "send-image-once")

		if sent.UploadID == nil || *sent.UploadID != staged.UploadID {
			t.Fatalf("sent message upload=%v, want %s", sent.UploadID, staged.UploadID)
		}
		if sent.CircleID == nil || *sent.CircleID != circle || sent.Type != chat.MessageTypeImage {
			t.Fatalf("sent message circle=%v type=%q, want %s/image", sent.CircleID, sent.Type, circle)
		}

		var uploadState string
		var messages, outbox int
		if err := env.pool.QueryRow(ctx, `SELECT state FROM chat_uploads WHERE id = $1`, staged.UploadID).Scan(&uploadState); err != nil {
			t.Fatalf("load attached upload: %v", err)
		}
		if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE sender_id = $1 AND idempotency_key = 'send-image-once'`, uploader).Scan(&messages); err != nil {
			t.Fatalf("count sent messages: %v", err)
		}
		if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM chat_event_outbox WHERE message_id = $1`, sent.ID).Scan(&outbox); err != nil {
			t.Fatalf("count outbox events: %v", err)
		}
		if uploadState != string(chat.UploadStateAttached) || messages != 1 || outbox != 1 {
			t.Fatalf("after send upload state=%q messages=%d outbox=%d, want attached/1/1", uploadState, messages, outbox)
		}

		replayed, err := env.service.SendGroupMedia(ctx, chat.SendGroupMediaInput{
			SenderID: uploader, CircleID: circle, UploadID: staged.UploadID,
			MessageType: chat.MessageTypeImage, IdempotencyKey: "send-image-once",
		})
		if err != nil || replayed.ID != sent.ID {
			t.Fatalf("idempotent replay ID=%s err=%v, want the committed message %s", replayed.ID, err, sent.ID)
		}
		if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM chat_event_outbox WHERE message_id = $1`, sent.ID).Scan(&outbox); err != nil {
			t.Fatalf("recount outbox events: %v", err)
		}
		if outbox != 1 {
			t.Fatalf("replay duplicated outbox events: %d, want 1", outbox)
		}

		// Regression: a fresh-key re-attach once surfaced as a raw
		// uq_messages_upload_id 23505 (mapped to 500) because the message
		// insert preceded the attachability check; real PostgreSQL catches
		// what the unit fakes cannot.
		if _, err := env.service.SendGroupMedia(ctx, chat.SendGroupMediaInput{
			SenderID: uploader, CircleID: circle, UploadID: staged.UploadID,
			MessageType: chat.MessageTypeImage, IdempotencyKey: "send-image-again",
		}); !errors.Is(err, chat.ErrUploadNotStaged) {
			t.Fatalf("double attach err=%v, want ErrUploadNotStaged (single-use attach)", err)
		}

		voiceStaged := stageChatMedia(t, env, ctx, uploader, circle, chat.MessageTypeVoice, chatMediaVoice(t), 30, "note.ogg")
		if _, err := env.service.SendGroupMedia(ctx, chat.SendGroupMediaInput{
			SenderID: uploader, CircleID: circle, UploadID: voiceStaged.UploadID,
			MessageType: chat.MessageTypeImage, IdempotencyKey: "send-wrong-family",
		}); !errors.Is(err, chat.ErrUploadNotAttachable) {
			t.Fatalf("wrong-family attach err=%v, want ErrUploadNotAttachable", err)
		}
		var wrongFamilyMessages int
		if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM messages WHERE idempotency_key = 'send-wrong-family'`).Scan(&wrongFamilyMessages); err != nil {
			t.Fatalf("count wrong-family messages: %v", err)
		}
		if wrongFamilyMessages != 0 {
			t.Fatalf("wrong-family send committed %d messages, want rolled back to 0", wrongFamilyMessages)
		}
	})

	t.Run("renewal serves members a fresh seven day url", func(t *testing.T) {
		_, sent := stageAndSendChatMedia(t, env, ctx, uploader, circle, chat.MessageTypeVoice, chatMediaVoice(t), 60, "lesson.ogg", "send-voice-renew")

		access, err := env.service.RenewMediaURL(ctx, member, sent.ID)
		if err != nil {
			t.Fatalf("member renewal: %v", err)
		}
		assertChatMediaSevenDayTTL(t, "renewed url", access.ExpiresAt)
		status, body, _ := chatMediaGet(t, access.URL.String())
		if status != http.StatusOK || !bytes.Equal(body, chatMediaVoice(t)) {
			t.Fatalf("renewed GET status=%d bytes=%d, want 200 and the staged payload", status, len(body))
		}
	})

	t.Run("expired presigned url is denied and renewal restores access", func(t *testing.T) {
		staged, sent := stageAndSendChatMedia(t, env, ctx, uploader, circle, chat.MessageTypeVoice, chatMediaVoice(t), 15, "expiry.ogg", "send-voice-expiry")

		// A short-TTL versionless URL stands in for a seven-day URL that has
		// aged past its lifetime, without waiting a week.
		short, err := env.store.PresignGet(ctx, staged.ObjectKey, 2*time.Second)
		if err != nil {
			t.Fatalf("presign short url: %v", err)
		}
		if status, _, _ := chatMediaGet(t, short.String()); status != http.StatusOK {
			t.Fatalf("short url GET before expiry status=%d, want 200", status)
		}
		time.Sleep(3 * time.Second)
		if status, _, _ := chatMediaGet(t, short.String()); status != http.StatusForbidden {
			t.Fatalf("expired url GET status=%d, want 403 (expired signature denied)", status)
		}

		access, err := env.service.RenewMediaURL(ctx, member, sent.ID)
		if err != nil {
			t.Fatalf("renewal after expiry: %v", err)
		}
		if status, body, _ := chatMediaGet(t, access.URL.String()); status != http.StatusOK || !bytes.Equal(body, chatMediaVoice(t)) {
			t.Fatalf("renewed GET status=%d bytes=%d, want 200 and the staged payload", status, len(body))
		}
	})

	t.Run("unauthorized renewal is non-enumerating", func(t *testing.T) {
		_, sent := stageAndSendChatMedia(t, env, ctx, uploader, circle, chat.MessageTypeImage, chatMediaPNG(t), 0, "guard.png", "send-image-guard")

		if _, err := env.service.RenewMediaURL(ctx, outsider, sent.ID); !errors.Is(err, chat.ErrMessageNotVisible) {
			t.Fatalf("outsider renewal err=%v, want ErrMessageNotVisible", err)
		}
		if _, err := env.service.RenewMediaURL(ctx, member, uuid.New()); !errors.Is(err, chat.ErrMessageNotVisible) {
			t.Fatalf("unknown message renewal err=%v, want ErrMessageNotVisible", err)
		}

		// A member who joined after the message was sent renews nothing: the
		// current membership period bounds visibility (FR-023/SR-002).
		lateMember := seedChatMediaUser(t, env.pool, "late-joiner")
		seedChatMediaMember(t, env.pool, circle, lateMember, "student", time.Now().Add(time.Minute))
		if _, err := env.service.RenewMediaURL(ctx, lateMember, sent.ID); !errors.Is(err, chat.ErrMessageNotVisible) {
			t.Fatalf("late-joiner renewal err=%v, want ErrMessageNotVisible", err)
		}
	})

	t.Run("failed database finalization cleans the staged object", func(t *testing.T) {
		failing := chat.NewUploadService(
			chatMediaFailingUploadRepo{Repository: env.repo, failErr: errors.New("simulated finalization failure")},
			env.members, env.store, env.cleaner, new(metrics.ChatMetrics), nil,
		)
		if _, err := failing.Stage(ctx, chat.StageUploadInput{
			UploaderID:       uploader,
			Target:           chat.UploadTarget{CircleID: &circle},
			MediaType:        chat.MessageTypeVoice,
			Data:             chatMediaVoice(t),
			DeclaredFileName: "orphan.ogg",
			DurationSeconds:  20,
		}); err == nil {
			t.Fatal("stage succeeded despite database finalization failure")
		}

		orphanKey := chatMediaLatestDeleteMarkerKey(t, env, ctx)
		orphanID, err := uuid.Parse(strings.TrimPrefix(orphanKey, "chat/"))
		if err != nil {
			t.Fatalf("orphan key %q hides no upload id: %v", orphanKey, err)
		}
		revoked, err := env.store.PresignGet(ctx, orphanKey, time.Minute)
		if err != nil {
			t.Fatalf("presign orphan key: %v", err)
		}
		if status, _, _ := chatMediaGet(t, revoked.String()); status != http.StatusNotFound {
			t.Fatalf("orphan presigned GET status=%d, want 404 (cleaned object is unconsumable)", status)
		}

		var rows int
		if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM chat_uploads WHERE uploader_id = $1 AND object_key = $2`, uploader, orphanKey).Scan(&rows); err != nil {
			t.Fatalf("count orphan upload rows: %v", err)
		}
		if rows != 0 {
			t.Fatalf("failed finalization left %d upload rows, want 0", rows)
		}
		if _, err := env.service.SendGroupMedia(ctx, chat.SendGroupMediaInput{
			SenderID: uploader, CircleID: circle, UploadID: orphanID,
			MessageType: chat.MessageTypeVoice, IdempotencyKey: "send-orphan",
		}); !errors.Is(err, chat.ErrUploadNotAttachable) {
			t.Fatalf("orphan attach err=%v, want ErrUploadNotAttachable (upload not consumable)", err)
		}
	})
}
