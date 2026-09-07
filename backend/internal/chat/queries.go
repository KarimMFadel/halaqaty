package chat

// This file is the ONLY home of runtime SQL for the chat package
// (constitution IV.7). Every statement is parameterized; repository methods
// reference these consts and never inline SQL.
//
// Column projections are alias-qualified (m./u.) so one const serves both
// single-table RETURNING clauses (INSERT INTO messages AS m ...) and joined
// history queries without ambiguous column references.

// messageColumns is the canonical message projection; state is derived from
// deleted_at so callers never interpret timestamps to classify visibility.
const messageColumns = `m.id, m.circle_id, m.dm_recipient_id, m.sender_id, m.message_type, m.content,
	m.upload_id, m.reply_to_id,
	CASE WHEN m.deleted_at IS NULL THEN 'active' ELSE 'deleted' END AS state,
	m.sent_at, m.deleted_at`

// uploadColumns is the canonical chat-upload projection.
const uploadColumns = `u.id, u.uploader_id, u.authorization_circle_id, u.dm_peer_id, u.object_key, u.mime_type,
	u.original_file_name, u.size_bytes, u.duration_seconds, u.state, u.created_at, u.updated_at`

// outboxColumns is the identifier-only chat event projection; no message
// bodies, object keys, or credentials are ever selected.
const outboxColumns = `event_id, message_id, event_type, recipient_id, available_at, delivered_at, attempt_count, parked_at`

// --- Messages ----------------------------------------------------------------

// insertMessageQuery inserts one message idempotently: a replayed
// (sender_id, idempotency_key) yields no row so the repository loads the
// committed original and reports inserted=false.
const insertMessageQuery = `
INSERT INTO messages AS m (circle_id, dm_recipient_id, sender_id, idempotency_key, message_type, content, upload_id, reply_to_id)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7::uuid, $8::uuid)
ON CONFLICT (sender_id, idempotency_key) DO NOTHING
RETURNING ` + messageColumns

// findMessageByIdempotencyQuery loads the committed original for a replayed
// send in the same transaction as the conflicted insert.
const findMessageByIdempotencyQuery = `
SELECT ` + messageColumns + `
FROM messages m
WHERE m.sender_id = $1::uuid AND m.idempotency_key = $2`

// --- History pages -----------------------------------------------------------

// findMessageCursorQuery resolves an anchor cursor to its (sent_at, id) pair.
// Soft-deleted anchors stay valid so pagination over a just-deleted message
// does not break; unknown ids are rejected by the repository.
const findMessageCursorQuery = `
SELECT sent_at, id FROM messages WHERE id = $1::uuid`

// groupHistoryPageQuery returns one circle history page for a current member.
// The circle_members join is the authorization boundary: non-members match no
// rows, and membership-period filtering hides messages sent before joined_at.
// The keyset predicate uses a row comparison on the (sent_at, id) DESC order.
const groupHistoryPageQuery = `
SELECT ` + messageColumns + `
FROM messages m
JOIN circle_members cm ON cm.circle_id = m.circle_id AND cm.user_id = $2::uuid
WHERE m.circle_id = $1::uuid
  AND m.deleted_at IS NULL
  AND m.sent_at >= cm.joined_at
  AND ($3::timestamptz IS NULL OR (m.sent_at, m.id) < ($3::timestamptz, $4::uuid))
ORDER BY m.sent_at DESC, m.id DESC
LIMIT $5`

// dmHistoryPageQuery returns one unordered-pair DM history page in
// (sent_at, id) DESC order, excluding deleted messages. Pair eligibility is
// recalculated from current shared circles by the service layer.
const dmHistoryPageQuery = `
SELECT ` + messageColumns + `
FROM messages m
WHERE m.deleted_at IS NULL
  AND ((m.sender_id = $1::uuid AND m.dm_recipient_id = $2::uuid)
    OR (m.sender_id = $2::uuid AND m.dm_recipient_id = $1::uuid))
  AND ($3::timestamptz IS NULL OR (m.sent_at, m.id) < ($3::timestamptz, $4::uuid))
ORDER BY m.sent_at DESC, m.id DESC
LIMIT $5`

// --- Read receipts -----------------------------------------------------------

// insertMessageReadQuery records one read fact idempotently: a repeated
// (message_id, user_id) insert affects zero rows and reports inserted=false.
const insertMessageReadQuery = `
INSERT INTO message_reads (message_id, user_id)
VALUES ($1::uuid, $2::uuid)
ON CONFLICT (message_id, user_id) DO NOTHING`

// --- Transactional outbox ----------------------------------------------------

// insertChatOutboxEventQuery writes one identifier-only chat event in the
// same transaction as the mutation it describes.
const insertChatOutboxEventQuery = `
INSERT INTO chat_event_outbox (message_id, event_type, recipient_id)
VALUES ($1::uuid, $2, $3::uuid)`

// claimOutboxEventsQuery atomically leases due, undelivered, unparked events:
// FOR UPDATE SKIP LOCKED keeps concurrent dispatchers disjoint, attempt_count
// increments per lease, and the 30-second available_at lease window blocks an
// immediate reclaim by another (or the same) dispatcher. Rows at the
// five-attempt ceiling are skipped so the check constraint can never be
// violated by a claim; parking them is the dispatcher's job.
const claimOutboxEventsQuery = `
WITH claimed AS (
    SELECT event_id AS claimed_event_id
    FROM chat_event_outbox
    WHERE available_at <= NOW()
      AND delivered_at IS NULL
      AND parked_at IS NULL
      AND attempt_count < 5
    ORDER BY available_at, event_id
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE chat_event_outbox AS outbox
SET attempt_count = attempt_count + 1,
    available_at = NOW() + INTERVAL '30 seconds'
FROM claimed
WHERE outbox.event_id = claimed.claimed_event_id
RETURNING ` + outboxColumns

// --- Uploads -----------------------------------------------------------------

// insertUploadQuery stages one private attachment row; state defaults to
// 'staged' until a message attach transitions it exactly once.
const insertUploadQuery = `
INSERT INTO chat_uploads AS u (uploader_id, authorization_circle_id, dm_peer_id, object_key,
                               mime_type, original_file_name, size_bytes, duration_seconds)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6, $7, $8)
RETURNING ` + uploadColumns

// attachUploadQuery applies the single-use staged→attached transition; a row
// already attached (or revoked) matches zero rows, which the repository maps
// to ErrUploadNotStaged so double-attach is detectable.
const attachUploadQuery = `
UPDATE chat_uploads AS u
SET state = 'attached', updated_at = NOW()
WHERE u.id = $1::uuid AND u.state = 'staged'
RETURNING ` + uploadColumns

// claimExpiredStagedUploadsQuery atomically claims staged uploads past the
// media-cleaner cutoff: matched rows transition to 'revoked' exactly once
// (FOR UPDATE SKIP LOCKED keeps concurrent cleaners disjoint and prevents
// reprocessing even when object deletion later fails) and are returned for
// best-effort object deletion. Only ever-staged rows match, so attached and
// revoked uploads are never claimed.
const claimExpiredStagedUploadsQuery = `
UPDATE chat_uploads AS u
SET state = 'revoked', updated_at = NOW()
WHERE u.id IN (
    SELECT id FROM chat_uploads
    WHERE state = 'staged' AND created_at < $1::timestamptz
    ORDER BY created_at
    LIMIT $2
    FOR UPDATE SKIP LOCKED
)
RETURNING ` + uploadColumns

// --- Moderation audits -------------------------------------------------------

// insertModerationAuditQuery appends one content-free moderation fact.
const insertModerationAuditQuery = `
INSERT INTO message_moderation_audits (message_id, circle_id, actor_id, action)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4)
RETURNING id, occurred_at`
