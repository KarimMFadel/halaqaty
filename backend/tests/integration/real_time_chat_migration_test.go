//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	migration018Up   = "000018_real_time_chat.up.sql"
	migration018Down = "000018_real_time_chat.down.sql"
)

var realTimeChatTables = []string{
	"messages",
	"chat_uploads",
	"message_reads",
	"chat_event_outbox",
	"message_moderation_audits",
}

func TestRealTimeChatMigration_FreshSchema(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	applyRealTimeChatMigrations(t, conn, ctx)

	assertRealTimeChatTables(t, conn, ctx, true)
	for _, index := range []string{
		"uq_messages_sender_idempotency_key",
		"idx_messages_circle_history",
		"idx_messages_dm_history",
		"idx_messages_reply_to_id",
		"idx_messages_pinned",
		"idx_messages_search_vector",
		"idx_chat_uploads_uploader_created_at",
		"idx_chat_uploads_staged_created_at",
		"idx_message_reads_user_id",
		"idx_chat_event_outbox_dispatch",
		"idx_message_moderation_audits_message_id",
	} {
		assertIndexExists(t, conn, ctx, schema, index)
	}

	var generated, dataType string
	if err := conn.QueryRow(ctx, `
		SELECT is_generated, data_type
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'messages'
		  AND column_name = 'search_vector'
	`).Scan(&generated, &dataType); err != nil {
		t.Fatalf("inspect messages.search_vector: %v", err)
	}
	if generated != "ALWAYS" || dataType != "tsvector" {
		t.Fatalf("messages.search_vector: got generated=%q type=%q, want ALWAYS/tsvector", generated, dataType)
	}

	var volatility string
	if err := conn.QueryRow(ctx, `
		SELECT p.provolatile::text
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = current_schema()
		  AND p.proname = 'halaqaty_normalize_arabic'
	`).Scan(&volatility); err != nil {
		t.Fatalf("inspect halaqaty_normalize_arabic: %v", err)
	}
	if volatility != "i" {
		t.Fatalf("halaqaty_normalize_arabic volatility: got %q, want immutable", volatility)
	}

	var fkCount, noActionCount int
	if err := conn.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE c.confdeltype = 'a' AND c.confupdtype = 'a')
		FROM pg_constraint c
		JOIN pg_class r ON r.oid = c.conrelid
		JOIN pg_namespace n ON n.oid = r.relnamespace
		WHERE n.nspname = current_schema()
		  AND r.relname = ANY($1::text[])
		  AND c.contype = 'f'
	`, realTimeChatTables).Scan(&fkCount, &noActionCount); err != nil {
		t.Fatalf("inspect F-004 foreign keys: %v", err)
	}
	if fkCount != 16 || noActionCount != fkCount {
		t.Fatalf("F-004 NO ACTION foreign keys: got %d of %d, want 16 of 16", noActionCount, fkCount)
	}

	assertTableColumns(t, conn, ctx, "chat_event_outbox",
		"event_id,message_id,event_type,recipient_id,available_at,delivered_at,attempt_count,parked_at")
	assertTableColumns(t, conn, ctx, "message_moderation_audits",
		"id,message_id,circle_id,actor_id,action,occurred_at")
}

func TestRealTimeChatMigration_ConstraintsAndArabicSearch(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	applyRealTimeChatMigrations(t, conn, ctx)

	senderID := seedLiveSessionUser(t, conn, ctx, "chat-sender")
	recipientID := seedLiveSessionUser(t, conn, ctx, "chat-recipient")
	actorID := seedLiveSessionUser(t, conn, ctx, "chat-actor")
	circleID := seedLiveSessionCircle(t, conn, ctx, senderID)

	var uploadID string
	var uploadState string
	if err := conn.QueryRow(ctx, `
		INSERT INTO chat_uploads (
			uploader_id, authorization_circle_id, object_key, mime_type,
			original_file_name, size_bytes
		)
		VALUES ($1::uuid, $2::uuid, 'chat/group/image.png', 'image/png', 'image.png', 1024)
		RETURNING id::text, state
	`, senderID, circleID).Scan(&uploadID, &uploadState); err != nil {
		t.Fatalf("insert valid chat upload: %v", err)
	}
	if uploadState != "staged" {
		t.Fatalf("chat upload state: got %q, want staged", uploadState)
	}

	var textMessageID string
	var normalized string
	var searchMatches bool
	if err := conn.QueryRow(ctx, `
		INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, content)
		VALUES ($1::uuid, $2::uuid, 'text-1', 'text', 'إِلَـى آلاء HELLO')
		RETURNING id::text,
		          halaqaty_normalize_arabic(content),
		          search_vector @@ websearch_to_tsquery('simple', halaqaty_normalize_arabic('الى hello'))
	`, circleID, senderID).Scan(&textMessageID, &normalized, &searchMatches); err != nil {
		t.Fatalf("insert searchable text message: %v", err)
	}
	if normalized != "الي الاء hello" {
		t.Fatalf("normalized chat text: got %q, want %q", normalized, "الي الاء hello")
	}
	if !searchMatches {
		t.Fatal("generated Arabic search vector did not match normalized query")
	}

	var mediaMessageID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, upload_id)
		VALUES ($1::uuid, $2::uuid, 'media-1', 'image', $3::uuid)
		RETURNING id::text
	`, circleID, senderID, uploadID).Scan(&mediaMessageID); err != nil {
		t.Fatalf("insert valid media message: %v", err)
	}

	if _, err := conn.Exec(ctx, `
		INSERT INTO message_reads (message_id, user_id)
		VALUES ($1::uuid, $2::uuid)
	`, textMessageID, recipientID); err != nil {
		t.Fatalf("insert valid message read: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO chat_event_outbox (message_id, event_type, recipient_id)
		VALUES ($1::uuid, 'chat.message_read', $2::uuid)
	`, textMessageID, senderID); err != nil {
		t.Fatalf("insert valid chat outbox row: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO message_moderation_audits (message_id, circle_id, actor_id, action)
		VALUES ($1::uuid, $2::uuid, $3::uuid, 'teacher_delete')
	`, textMessageID, circleID, actorID); err != nil {
		t.Fatalf("insert valid moderation audit: %v", err)
	}

	expectViolations(t, conn, ctx, []sqlViolationCase{
		{
			name: "message without context",
			sql: `INSERT INTO messages (sender_id, idempotency_key, message_type, content)
			      VALUES ($1::uuid, 'bad-context-none', 'text', 'hello')`,
			code: "23514", args: []any{senderID},
		},
		{
			name: "message with group and DM contexts",
			sql: `INSERT INTO messages (circle_id, dm_recipient_id, sender_id, idempotency_key, message_type, content)
			      VALUES ($1::uuid, $2::uuid, $3::uuid, 'bad-context-both', 'text', 'hello')`,
			code: "23514", args: []any{circleID, recipientID, senderID},
		},
		{
			name: "self DM",
			sql: `INSERT INTO messages (dm_recipient_id, sender_id, idempotency_key, message_type, content)
			      VALUES ($1::uuid, $1::uuid, 'bad-self-dm', 'text', 'hello')`,
			code: "23514", args: []any{senderID},
		},
		{
			name: "untrimmed text",
			sql: `INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, content)
			      VALUES ($1::uuid, $2::uuid, 'bad-untrimmed', 'text', ' hello ')`,
			code: "23514", args: []any{circleID, senderID},
		},
		{
			name: "empty text",
			sql: `INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, content)
			      VALUES ($1::uuid, $2::uuid, 'bad-empty', 'text', '')`,
			code: "23514", args: []any{circleID, senderID},
		},
		{
			name: "oversized text",
			sql: `INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, content)
			      VALUES ($1::uuid, $2::uuid, 'bad-long', 'text', repeat('x', 4001))`,
			code: "23514", args: []any{circleID, senderID},
		},
		{
			name: "text with upload",
			sql: `INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, content, upload_id)
			      VALUES ($1::uuid, $2::uuid, 'bad-text-upload', 'text', 'hello', $3::uuid)`,
			code: "23514", args: []any{circleID, senderID, uploadID},
		},
		{
			name: "media without upload",
			sql: `INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type)
			      VALUES ($1::uuid, $2::uuid, 'bad-media-empty', 'voice')`,
			code: "23514", args: []any{circleID, senderID},
		},
		{
			name: "duplicate sender idempotency key",
			sql: `INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, content)
			      VALUES ($1::uuid, $2::uuid, 'text-1', 'text', 'retry')`,
			code: "23505", args: []any{circleID, senderID},
		},
		{
			name: "reused upload",
			sql: `INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, upload_id)
			      VALUES ($1::uuid, $2::uuid, 'media-2', 'image', $3::uuid)`,
			code: "23505", args: []any{circleID, senderID, uploadID},
		},
		{
			name: "missing reply target",
			sql: `INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, content, reply_to_id)
			      VALUES ($1::uuid, $2::uuid, 'bad-reply', 'text', 'reply', '99999999-9999-9999-9999-999999999999'::uuid)`,
			code: "23503", args: []any{circleID, senderID},
		},
		{
			name: "DM pin",
			sql: `INSERT INTO messages (dm_recipient_id, sender_id, idempotency_key, message_type, content, is_pinned, pinned_by, pinned_at)
			      VALUES ($1::uuid, $2::uuid, 'bad-dm-pin', 'text', 'hello', TRUE, $3::uuid, NOW())`,
			code: "23514", args: []any{recipientID, senderID, actorID},
		},
		{
			name: "incomplete pin metadata",
			sql: `INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, content, is_pinned, pinned_by)
			      VALUES ($1::uuid, $2::uuid, 'bad-pin-shape', 'text', 'hello', TRUE, $3::uuid)`,
			code: "23514", args: []any{circleID, senderID, actorID},
		},
		{
			name: "deleted pinned message",
			sql: `UPDATE messages SET is_pinned = TRUE, pinned_by = $2::uuid, pinned_at = NOW(), deleted_at = NOW()
			      WHERE id = $1::uuid`,
			code: "23514", args: []any{textMessageID, actorID},
		},
		{
			name: "bad upload media limits",
			sql: `INSERT INTO chat_uploads (uploader_id, authorization_circle_id, object_key, mime_type, original_file_name, size_bytes)
			      VALUES ($1::uuid, $2::uuid, 'chat/group/large.png', 'image/png', 'large.png', 5242881)`,
			code: "23514", args: []any{senderID, circleID},
		},
		{
			name: "bad voice duration",
			sql: `INSERT INTO chat_uploads (uploader_id, authorization_circle_id, object_key, mime_type, original_file_name, size_bytes, duration_seconds)
			      VALUES ($1::uuid, $2::uuid, 'chat/group/long.ogg', 'audio/ogg', 'long.ogg', 1024, 301)`,
			code: "23514", args: []any{senderID, circleID},
		},
		{
			name: "bad upload state",
			sql: `INSERT INTO chat_uploads (uploader_id, authorization_circle_id, object_key, mime_type, original_file_name, size_bytes, state)
			      VALUES ($1::uuid, $2::uuid, 'chat/group/bad.pdf', 'application/pdf', 'bad.pdf', 1024, 'deleted')`,
			code: "23514", args: []any{senderID, circleID},
		},
		{
			name: "negative outbox attempts",
			sql: `INSERT INTO chat_event_outbox (message_id, event_type, attempt_count)
			      VALUES ($1::uuid, 'chat.message', -1)`,
			code: "23514", args: []any{textMessageID},
		},
		{
			name: "outbox attempts above retry limit",
			sql: `INSERT INTO chat_event_outbox (message_id, event_type, attempt_count)
			      VALUES ($1::uuid, 'chat.message', 6)`,
			code: "23514", args: []any{textMessageID},
		},
		{
			name: "bad moderation action",
			sql: `INSERT INTO message_moderation_audits (message_id, circle_id, actor_id, action)
			      VALUES ($1::uuid, $2::uuid, $3::uuid, 'edit')`,
			code: "23514", args: []any{mediaMessageID, circleID, actorID},
		},
	})

	var indexDefinition string
	if err := conn.QueryRow(ctx, `
		SELECT indexdef
		FROM pg_indexes
		WHERE schemaname = current_schema()
		  AND indexname = 'idx_messages_search_vector'
	`).Scan(&indexDefinition); err != nil {
		t.Fatalf("inspect chat search index: %v", err)
	}
	for _, fragment := range []string{"USING gin", "deleted_at IS NULL", "message_type", "circle_id IS NOT NULL"} {
		if !strings.Contains(indexDefinition, fragment) {
			t.Fatalf("chat search index %q does not contain %q", indexDefinition, fragment)
		}
	}
}

func TestRealTimeChatMigration_UpgradeDownAndReapplyPreservePriorData(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}
	userID := seedLiveSessionUser(t, conn, ctx, "down-up")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, userID, "")

	runMigrationFile(t, conn, ctx, migration018Up)
	assertRealTimeChatTables(t, conn, ctx, true)
	assertLegacyRowsIntact(t, conn, ctx, userID, circleID, sessionID)

	runMigrationFile(t, conn, ctx, migration018Down)
	assertRealTimeChatTables(t, conn, ctx, false)
	assertLegacyRowsIntact(t, conn, ctx, userID, circleID, sessionID)

	var functionCount int
	if err := conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = current_schema()
		  AND p.proname = 'halaqaty_normalize_arabic'
	`).Scan(&functionCount); err != nil {
		t.Fatalf("inspect normalization function after rollback: %v", err)
	}
	if functionCount != 0 {
		t.Fatalf("normalization function still exists after rollback: count=%d", functionCount)
	}

	runMigrationFile(t, conn, ctx, migration018Up)
	assertRealTimeChatTables(t, conn, ctx, true)
	assertLegacyRowsIntact(t, conn, ctx, userID, circleID, sessionID)
}

func applyRealTimeChatMigrations(t *testing.T, conn *pgxpool.Conn, ctx context.Context) {
	t.Helper()
	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}
	runMigrationFile(t, conn, ctx, migration018Up)
}

func assertRealTimeChatTables(t *testing.T, conn *pgxpool.Conn, ctx context.Context, wantExist bool) {
	t.Helper()
	for _, table := range realTimeChatTables {
		var regclass *string
		if err := conn.QueryRow(ctx, `SELECT to_regclass($1)`, table).Scan(&regclass); err != nil {
			t.Fatalf("lookup table %s: %v", table, err)
		}
		if (regclass != nil) != wantExist {
			t.Fatalf("table %s existence: got %v, want %v", table, regclass != nil, wantExist)
		}
	}
}

func assertTableColumns(t *testing.T, conn *pgxpool.Conn, ctx context.Context, table, want string) {
	t.Helper()
	var got string
	if err := conn.QueryRow(ctx, `
		SELECT string_agg(column_name, ',' ORDER BY ordinal_position)
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = $1
	`, table).Scan(&got); err != nil {
		t.Fatalf("inspect columns for %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("columns for %s: got %q, want %q", table, got, want)
	}
}
