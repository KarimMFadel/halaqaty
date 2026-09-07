//go:build integration

package chat

import (
	"bytes"
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

// chatRepoMigrations is the full migration chain up to and including F-004,
// applied in production order so the repository runs against real schema.
var chatRepoMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
	"000013_create_circles.up.sql",
	"000014_circle_members_circle_fk.up.sql",
	"000015_circle_management.up.sql",
	"000016_live_sessions.up.sql",
	"000017_recitation_queue_system.up.sql",
	"000018_real_time_chat.up.sql",
}

// newChatRepo opens an isolated schema with the chat migration chain applied
// and returns a repository bound to that schema.
func newChatRepo(t *testing.T) *Repository {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_chat_repo_%d", time.Now().UnixNano())
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
	for _, name := range chatRepoMigrations {
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

func seedUser(t *testing.T, repo *Repository, label string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id
	`, "firebase-chat-"+label, label+"-chat@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	return id
}

func seedCircle(t *testing.T, repo *Repository, name string, teacherID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ($1, $2, $3)
		RETURNING id
	`, name, teacherID, "HLQ-"+uuid.NewString()[:8]).Scan(&id); err != nil {
		t.Fatalf("seed circle %s: %v", name, err)
	}
	return id
}

func seedMember(t *testing.T, repo *Repository, circleID, userID uuid.UUID, role string, joinedAt time.Time) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO circle_members (circle_id, user_id, role, joined_at)
		VALUES ($1, $2, $3, $4)
	`, circleID, userID, role, joinedAt); err != nil {
		t.Fatalf("seed member %s in %s: %v", userID, circleID, err)
	}
}

func seedMessage(t *testing.T, repo *Repository, senderID uuid.UUID, circleID, dmRecipientID *uuid.UUID, sentAt time.Time, content string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO messages (circle_id, dm_recipient_id, sender_id, idempotency_key, message_type, content, sent_at)
		VALUES ($1, $2, $3, $4, 'text', $5, $6)
		RETURNING id
	`, circleID, dmRecipientID, senderID, uuid.NewString(), content, sentAt).Scan(&id); err != nil {
		t.Fatalf("seed message %q: %v", content, err)
	}
	return id
}

func seedOutboxEvent(t *testing.T, repo *Repository, messageID uuid.UUID, eventType string, recipientID *uuid.UUID, availableAt time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO chat_event_outbox (message_id, event_type, recipient_id, available_at)
		VALUES ($1, $2, $3, $4)
		RETURNING event_id
	`, messageID, eventType, recipientID, availableAt).Scan(&id); err != nil {
		t.Fatalf("seed outbox event %s: %v", eventType, err)
	}
	return id
}

func countRows(t *testing.T, repo *Repository, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := repo.pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatalf("count rows (%s): %v", query, err)
	}
	return n
}

// assertStrictlyDescending verifies the (sent_at, id) DESC page contract:
// strictly decreasing, no duplicates.
func assertStrictlyDescending(t *testing.T, msgs []Message) {
	t.Helper()
	seen := make(map[uuid.UUID]bool, len(msgs))
	for i, msg := range msgs {
		if seen[msg.ID] {
			t.Fatalf("message %s appears more than once", msg.ID)
		}
		seen[msg.ID] = true
		if i == 0 {
			continue
		}
		prev := msgs[i-1]
		if prev.SentAt.After(msg.SentAt) {
			continue
		}
		if prev.SentAt.Before(msg.SentAt) {
			t.Fatalf("page order broken at %d: %v precedes %v", i, prev.SentAt, msg.SentAt)
		}
		if bytes.Compare(prev.ID[:], msg.ID[:]) <= 0 {
			t.Fatalf("equal-timestamp tie-break broken at %d: %s must precede %s", i, prev.ID, msg.ID)
		}
	}
}

func TestRepository_SendTransaction_CommitsMessageAndOutboxTogether(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "tx-sender")
	circle := seedCircle(t, repo, "Tx Circle", sender)
	seedMember(t, repo, circle, sender, "teacher", time.Now().UTC().Add(-time.Hour))

	in := MessageInput{
		SenderID:       sender,
		CircleID:       &circle,
		Type:           MessageTypeText,
		Content:        "transactional hello",
		IdempotencyKey: "tx-commit-1",
	}
	var sent Message
	if err := repo.WithTx(ctx, func(tx *Tx) error {
		msg, inserted, err := tx.InsertMessage(ctx, in)
		if err != nil {
			return err
		}
		if !inserted {
			return errors.New("first insert must report inserted=true")
		}
		sent = msg
		return tx.InsertOutboxEvent(ctx, msg.ID, "chat.message", nil)
	}); err != nil {
		t.Fatalf("send transaction: %v", err)
	}

	if sent.ID == uuid.Nil || sent.State != MessageStateActive || sent.SentAt.IsZero() {
		t.Fatalf("returned message projection incomplete: %+v", sent)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM messages WHERE sender_id = $1`, sender); n != 1 {
		t.Fatalf("committed message count: got %d want 1", n)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox WHERE message_id = $1`, sent.ID); n != 1 {
		t.Fatalf("committed outbox count: got %d want 1", n)
	}
}

func TestRepository_SendTransaction_ForcedFailureRollsBackBoth(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "rb-sender")
	circle := seedCircle(t, repo, "Rollback Circle", sender)
	seedMember(t, repo, circle, sender, "teacher", time.Now().UTC().Add(-time.Hour))

	forced := errors.New("forced failure")
	sendErr := repo.WithTx(ctx, func(tx *Tx) error {
		msg, _, err := tx.InsertMessage(ctx, MessageInput{
			SenderID:       sender,
			CircleID:       &circle,
			Type:           MessageTypeText,
			Content:        "doomed message",
			IdempotencyKey: "tx-rollback-1",
		})
		if err != nil {
			return err
		}
		if err := tx.InsertOutboxEvent(ctx, msg.ID, "chat.message", nil); err != nil {
			return err
		}
		return forced
	})
	if !errors.Is(sendErr, forced) {
		t.Fatalf("rollback transaction must surface forced error, got %v", sendErr)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM messages WHERE sender_id = $1`, sender); n != 0 {
		t.Fatalf("rolled-back message must not persist: got %d rows", n)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM chat_event_outbox`); n != 0 {
		t.Fatalf("rolled-back outbox event must not persist: got %d rows", n)
	}
}

func TestRepository_InsertMessage_IdempotentReplayReturnsExisting(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "idem-sender")
	circle := seedCircle(t, repo, "Idem Circle", sender)
	seedMember(t, repo, circle, sender, "teacher", time.Now().UTC().Add(-time.Hour))
	in := MessageInput{
		SenderID:       sender,
		CircleID:       &circle,
		Type:           MessageTypeText,
		Content:        "send me twice",
		IdempotencyKey: "idem-1",
	}

	var first, second Message
	var firstInserted, secondInserted bool
	send := func() error {
		return repo.WithTx(ctx, func(tx *Tx) error {
			msg, inserted, err := tx.InsertMessage(ctx, in)
			if err != nil {
				return err
			}
			if first.ID == uuid.Nil {
				first, firstInserted = msg, inserted
			} else {
				second, secondInserted = msg, inserted
			}
			return nil
		})
	}
	if err := send(); err != nil {
		t.Fatalf("first send: %v", err)
	}
	if err := send(); err != nil {
		t.Fatalf("retry send: %v", err)
	}

	if !firstInserted {
		t.Fatal("first insert must report inserted=true")
	}
	if secondInserted {
		t.Fatal("retry must report inserted=false")
	}
	if second.ID != first.ID {
		t.Fatalf("retry must return existing message id: got %s want %s", second.ID, first.ID)
	}
	if second.Content != first.Content {
		t.Fatalf("replayed message content: got %q want %q", second.Content, first.Content)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM messages WHERE sender_id = $1 AND idempotency_key = $2`, sender, in.IdempotencyKey); n != 1 {
		t.Fatalf("idempotent message count: got %d want 1", n)
	}
}

func TestRepository_GroupHistoryPage_KeysetPaginationIsDeterministic(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "keyset-teacher")
	circle := seedCircle(t, repo, "Keyset Circle", teacher)
	seedMember(t, repo, circle, teacher, "teacher", time.Now().UTC().Add(-time.Hour))

	// Five messages sharing one sent_at force the (sent_at, id) tie-break.
	sentAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Microsecond)
	all := make([]uuid.UUID, 0, 5)
	for i := 0; i < 5; i++ {
		all = append(all, seedMessage(t, repo, teacher, &circle, nil, sentAt, fmt.Sprintf("page-%d", i)))
	}

	var flattened []Message
	before := (*uuid.UUID)(nil)
	for page := 0; page < 3; page++ {
		msgs, err := repo.GroupHistoryPage(ctx, circle, teacher, before, 2)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		wantLen := 2
		if page == 2 {
			wantLen = 1
		}
		if len(msgs) != wantLen {
			t.Fatalf("page %d length: got %d want %d", page, len(msgs), wantLen)
		}
		flattened = append(flattened, msgs...)
		before = &msgs[len(msgs)-1].ID
	}

	assertStrictlyDescending(t, flattened)
	if len(flattened) != 5 {
		t.Fatalf("pagination must cover all messages exactly once: got %d", len(flattened))
	}

	// Exhausting the cursor must yield an empty terminal page.
	msgs, err := repo.GroupHistoryPage(ctx, circle, teacher, before, 2)
	if err != nil {
		t.Fatalf("terminal page: %v", err)
	}
	if len(msgs) != 0 {
		t.Fatalf("terminal page must be empty, got %d messages", len(msgs))
	}
}

func TestRepository_GroupHistoryPage_MembershipPeriodFilter(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "period-teacher")
	student := seedUser(t, repo, "period-student")
	outsider := seedUser(t, repo, "period-outsider")
	circle := seedCircle(t, repo, "Period Circle", teacher)

	joinedAt := time.Now().UTC().Add(-30 * time.Minute).Truncate(time.Microsecond)
	seedMember(t, repo, circle, teacher, "teacher", joinedAt.Add(-time.Hour))
	seedMember(t, repo, circle, student, "student", joinedAt)

	seedMessage(t, repo, teacher, &circle, nil, joinedAt.Add(-10*time.Minute), "before-join")
	seedMessage(t, repo, teacher, &circle, nil, joinedAt, "at-join")
	seedMessage(t, repo, teacher, &circle, nil, joinedAt.Add(10*time.Minute), "after-join")
	deleted := seedMessage(t, repo, teacher, &circle, nil, joinedAt.Add(11*time.Minute), "deleted-after-join")
	if _, err := repo.pool.Exec(ctx, `UPDATE messages SET deleted_at = NOW() WHERE id = $1`, deleted); err != nil {
		t.Fatalf("soft-delete fixture: %v", err)
	}

	studentPage, err := repo.GroupHistoryPage(ctx, circle, student, nil, 50)
	if err != nil {
		t.Fatalf("student page: %v", err)
	}
	if got := messageContents(studentPage); fmt.Sprint(got) != fmt.Sprint([]string{"after-join", "at-join"}) {
		t.Fatalf("student must see only messages at/after joined_at, excluding deleted: got %v", got)
	}

	teacherPage, err := repo.GroupHistoryPage(ctx, circle, teacher, nil, 50)
	if err != nil {
		t.Fatalf("teacher page: %v", err)
	}
	if got := messageContents(teacherPage); len(got) != 3 || got[0] != "after-join" || got[1] != "at-join" || got[2] != "before-join" {
		t.Fatalf("teacher joined before all messages and must see them DESC: got %v", got)
	}

	outsiderPage, err := repo.GroupHistoryPage(ctx, circle, outsider, nil, 50)
	if err != nil {
		t.Fatalf("outsider page: %v", err)
	}
	if len(outsiderPage) != 0 {
		t.Fatalf("non-member must see no messages, got %d", len(outsiderPage))
	}
}

func messageContents(msgs []Message) []string {
	contents := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		contents = append(contents, msg.Content)
	}
	return contents
}

func TestRepository_HistoryPages_UnknownCursor(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	a := seedUser(t, repo, "cursor-a")
	b := seedUser(t, repo, "cursor-b")
	teacher := seedUser(t, repo, "cursor-teacher")
	circle := seedCircle(t, repo, "Cursor Circle", teacher)
	seedMember(t, repo, circle, teacher, "teacher", time.Now().UTC().Add(-time.Hour))
	unknown := uuid.New()

	cases := []struct {
		name string
		run  func() error
	}{
		{
			name: "group page",
			run: func() error {
				_, err := repo.GroupHistoryPage(ctx, circle, teacher, &unknown, 10)
				return err
			},
		},
		{
			name: "dm page",
			run: func() error {
				_, err := repo.DMHistoryPage(ctx, a, b, &unknown, 10)
				return err
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("unknown cursor: got %v want ErrInvalidCursor", err)
			}
		})
	}
}

func TestRepository_DMHistoryPage_UnorderedPairBothDirections(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	a := seedUser(t, repo, "dm-a")
	b := seedUser(t, repo, "dm-b")
	c := seedUser(t, repo, "dm-c")
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)

	seedMessage(t, repo, a, nil, &b, base, "a-to-b")
	seedMessage(t, repo, b, nil, &a, base.Add(time.Minute), "b-to-a")
	seedMessage(t, repo, a, nil, &c, base.Add(2*time.Minute), "a-to-c-decoy")

	forward, err := repo.DMHistoryPage(ctx, a, b, nil, 50)
	if err != nil {
		t.Fatalf("forward pair page: %v", err)
	}
	if got := messageContents(forward); fmt.Sprint(got) != fmt.Sprint([]string{"b-to-a", "a-to-b"}) {
		t.Fatalf("pair history must contain both directions DESC: got %v", got)
	}

	reverse, err := repo.DMHistoryPage(ctx, b, a, nil, 50)
	if err != nil {
		t.Fatalf("reverse pair page: %v", err)
	}
	if got := messageContents(reverse); fmt.Sprint(got) != fmt.Sprint([]string{"b-to-a", "a-to-b"}) {
		t.Fatalf("pair history must be direction-agnostic: got %v", got)
	}
}

func TestRepository_InsertMessageRead_IdempotentFact(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "read-sender")
	reader := seedUser(t, repo, "read-reader")
	circle := seedCircle(t, repo, "Read Circle", sender)
	seedMember(t, repo, circle, sender, "teacher", time.Now().UTC().Add(-time.Hour))
	messageID := seedMessage(t, repo, sender, &circle, nil, time.Now().UTC().Add(-time.Minute), "read me")

	results := make([]bool, 0, 2)
	for i := 0; i < 2; i++ {
		if err := repo.WithTx(ctx, func(tx *Tx) error {
			inserted, err := tx.InsertMessageRead(ctx, messageID, reader)
			if err != nil {
				return err
			}
			results = append(results, inserted)
			return nil
		}); err != nil {
			t.Fatalf("read insert %d: %v", i+1, err)
		}
	}

	if len(results) != 2 || !results[0] || results[1] {
		t.Fatalf("read inserts must be [true false], got %v", results)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM message_reads WHERE message_id = $1 AND user_id = $2`, messageID, reader); n != 1 {
		t.Fatalf("unique read fact: got %d rows want 1", n)
	}
}

func TestRepository_Upload_AttachOnceOnly(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	uploader := seedUser(t, repo, "upload-uploader")
	circle := seedCircle(t, repo, "Upload Circle", uploader)

	staged, err := repo.InsertUpload(ctx, Upload{
		UploaderID:            uploader,
		AuthorizationCircleID: circle,
		ObjectKey:             "chat/upload-circle/photo.png",
		MIMEType:              "image/png",
		OriginalFileName:      "photo.png",
		SizeBytes:             2048,
	})
	if err != nil {
		t.Fatalf("stage upload: %v", err)
	}
	if staged.ID == uuid.Nil || staged.State != UploadStateStaged {
		t.Fatalf("staged upload projection: %+v", staged)
	}

	var attached Upload
	if err := repo.WithTx(ctx, func(tx *Tx) error {
		upload, err := tx.AttachUpload(ctx, staged.ID)
		if err != nil {
			return err
		}
		attached = upload
		return nil
	}); err != nil {
		t.Fatalf("attach upload: %v", err)
	}
	if attached.State != UploadStateAttached {
		t.Fatalf("attached state: got %q want attached", attached.State)
	}

	err = repo.WithTx(ctx, func(tx *Tx) error {
		_, err := tx.AttachUpload(ctx, staged.ID)
		return err
	})
	if !errors.Is(err, ErrUploadNotStaged) {
		t.Fatalf("second attach must be rejected, got %v", err)
	}

	var state string
	if err := repo.pool.QueryRow(ctx, `SELECT state FROM chat_uploads WHERE id = $1`, staged.ID).Scan(&state); err != nil {
		t.Fatalf("reread upload state: %v", err)
	}
	if state != "attached" {
		t.Fatalf("persisted state after double attach: got %q want attached", state)
	}
}

func TestRepository_ClaimOutboxEvents_ClaimsDueAndIncrementsAttempts(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	sender := seedUser(t, repo, "claim-sender")
	circle := seedCircle(t, repo, "Claim Circle", sender)
	seedMember(t, repo, circle, sender, "teacher", time.Now().UTC().Add(-time.Hour))
	due := time.Now().UTC().Add(-time.Minute)
	future := time.Now().UTC().Add(time.Minute)
	m1 := seedMessage(t, repo, sender, &circle, nil, due, "claim-1")
	m2 := seedMessage(t, repo, sender, &circle, nil, due, "claim-2")
	e1 := seedOutboxEvent(t, repo, m1, "chat.message", nil, due)
	e2 := seedOutboxEvent(t, repo, m2, "chat.message", nil, due)
	seedOutboxEvent(t, repo, m2, "chat.message_read", &sender, future)

	claimed, err := repo.ClaimOutboxEvents(ctx, 10)
	if err != nil {
		t.Fatalf("claim outbox: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claim must return the two due events, got %d", len(claimed))
	}
	claimedIDs := map[uuid.UUID]bool{}
	for _, event := range claimed {
		claimedIDs[event.EventID] = true
		if event.AttemptCount != 1 {
			t.Fatalf("claim must increment attempt_count to 1, got %d", event.AttemptCount)
		}
		if event.State != OutboxStatePending {
			t.Fatalf("claimed state: got %q want pending", event.State)
		}
	}
	if !claimedIDs[e1] || !claimedIDs[e2] {
		t.Fatalf("claim returned wrong events: got %v want %s and %s", claimedIDs, e1, e2)
	}

	// The claim leases rows by pushing available_at forward, so an immediate
	// reclaim must find nothing (SKIP LOCKED mutual exclusion, sequentially).
	reclaimed, err := repo.ClaimOutboxEvents(ctx, 10)
	if err != nil {
		t.Fatalf("reclaim outbox: %v", err)
	}
	if len(reclaimed) != 0 {
		t.Fatalf("immediate reclaim must be empty, got %d events", len(reclaimed))
	}
}

func TestRepository_InsertModerationAudit_AppendOnly(t *testing.T) {
	repo := newChatRepo(t)
	ctx := context.Background()
	teacher := seedUser(t, repo, "audit-teacher")
	student := seedUser(t, repo, "audit-student")
	circle := seedCircle(t, repo, "Audit Circle", teacher)
	seedMember(t, repo, circle, teacher, "teacher", time.Now().UTC().Add(-time.Hour))
	seedMember(t, repo, circle, student, "student", time.Now().UTC().Add(-time.Hour))
	messageID := seedMessage(t, repo, student, &circle, nil, time.Now().UTC().Add(-time.Minute), "to be moderated")

	var audit ModerationAudit
	if err := repo.WithTx(ctx, func(tx *Tx) error {
		recorded, err := tx.InsertModerationAudit(ctx, ModerationAudit{
			MessageID: messageID,
			CircleID:  circle,
			ActorID:   teacher,
			Action:    ModerationActionTeacherDelete,
		})
		if err != nil {
			return err
		}
		audit = recorded
		return nil
	}); err != nil {
		t.Fatalf("insert moderation audit: %v", err)
	}

	if audit.ID == uuid.Nil || audit.OccurredAt.IsZero() || audit.Action != ModerationActionTeacherDelete {
		t.Fatalf("audit projection incomplete: %+v", audit)
	}
	if n := countRows(t, repo, `SELECT COUNT(*) FROM message_moderation_audits WHERE message_id = $1`, messageID); n != 1 {
		t.Fatalf("moderation audit count: got %d want 1", n)
	}
}
