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

// lockActiveCircleMemberQuery locks the circle and membership rows in the same
// transaction as a chat mutation, closing archive and removal races before
// persistence.
const lockActiveCircleMemberQuery = `
SELECT c.is_archived
FROM circles c
JOIN circle_members cm ON cm.circle_id = c.id AND cm.user_id = $2::uuid
WHERE c.id = $1::uuid
FOR UPDATE OF c, cm`

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

// groupSearchPageQuery searches only retained, non-deleted group messages.
const groupSearchPageQuery = `
SELECT ` + messageColumns + `
FROM messages m
JOIN circle_members cm ON cm.circle_id = m.circle_id AND cm.user_id = $2::uuid
WHERE m.circle_id = $1::uuid
  AND m.deleted_at IS NULL
  AND m.sent_at >= cm.joined_at
  AND m.search_vector @@ websearch_to_tsquery('simple', halaqaty_normalize_arabic($3))
  AND ($4::timestamptz IS NULL OR (m.sent_at, m.id) < ($4::timestamptz, $5::uuid))
ORDER BY m.sent_at DESC, m.id DESC
LIMIT $6`

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

const softDeleteOwnDirectMessageQuery = `
UPDATE messages
SET deleted_at = NOW()
WHERE id = $1::uuid
  AND sender_id = $2::uuid
  AND dm_recipient_id = $3::uuid
  AND circle_id IS NULL
  AND deleted_at IS NULL
  AND sent_at >= NOW() - INTERVAL '10 minutes'
RETURNING id`

// --- Read receipts -----------------------------------------------------------

// lockVisibleGroupMessageForReadQuery verifies and locks one active group
// message that is visible in the reader's current membership period. The
// sender exclusion prevents users from recording read facts for their own
// messages.
const lockVisibleGroupMessageForReadQuery = `
SELECT m.sender_id
FROM messages m
JOIN circle_members cm ON cm.circle_id = m.circle_id AND cm.user_id = $2::uuid
WHERE m.circle_id = $1::uuid
  AND m.id = $3::uuid
  AND m.deleted_at IS NULL
  AND m.sent_at >= cm.joined_at
  AND m.sender_id <> $2::uuid
FOR SHARE OF m`

// lockEligibleDirectMessageForReadQuery verifies the reader is the recipient
// of a visible DM and that the unordered pair still has an active qualifying
// circle relationship. The sender is returned for targeted delivery.
const lockEligibleDirectMessageForReadQuery = `
SELECT m.sender_id
FROM messages m
JOIN circle_members cm_a ON cm_a.user_id = m.sender_id
JOIN circle_members cm_b ON cm_b.circle_id = cm_a.circle_id AND cm_b.user_id = $1::uuid
JOIN circles c ON c.id = cm_a.circle_id AND c.is_archived = FALSE
WHERE m.id = $2::uuid
  AND m.circle_id IS NULL
  AND m.deleted_at IS NULL
  AND m.dm_recipient_id = $1::uuid
  AND (
       (cm_a.role = 'teacher' AND cm_b.role = 'student')
    OR (cm_a.role = 'student' AND cm_b.role = 'teacher')
    OR (cm_a.role = 'supervisor' AND cm_b.role = 'student')
    OR (cm_a.role = 'student' AND cm_b.role = 'supervisor')
  )
FOR SHARE OF m`

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
// 'staged' until a message attach transitions it exactly once. The id is
// caller-pinned when provided (the object key derives from it) and
// database-generated otherwise.
const insertUploadQuery = `
INSERT INTO chat_uploads AS u (id, uploader_id, authorization_circle_id, dm_peer_id, object_key,
                               mime_type, original_file_name, size_bytes, duration_seconds)
VALUES (COALESCE($1::uuid, gen_random_uuid()), $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, $8, $9)
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

// countRecentUploadsQuery counts the uploader's successfully staged uploads
// (staged or attached) created since the cutoff; only cleaned-up revoked rows
// are excluded, matching the rolling-hour budget accounting that counts
// staged-but-unattached uploads (FR-022).
const countRecentUploadsQuery = `
SELECT COUNT(*) FROM chat_uploads
WHERE uploader_id = $1::uuid
  AND created_at >= $2::timestamptz
  AND state IN ('staged', 'attached')`

// Serialize only finalization for this uploader across backend processes.
const lockUploadBudgetQuery = `SELECT pg_advisory_xact_lock(hashtextextended('chat-upload:' || $1::text, 0))`

// findQualifyingDMCircleQuery resolves one currently qualifying shared
// active circle for an unordered pair: both users are current members of the
// same non-archived circle and their roles form a teacher-student or
// supervisor-student pair in either direction (FR-016). It returns only one
// witness id and never discloses which circle matched.
const findQualifyingDMCircleQuery = `
SELECT cm_a.circle_id
FROM circle_members cm_a
JOIN circle_members cm_b ON cm_b.circle_id = cm_a.circle_id AND cm_b.user_id = $2::uuid
JOIN circles c ON c.id = cm_a.circle_id
WHERE cm_a.user_id = $1::uuid
  AND c.is_archived = FALSE
  AND (
       (cm_a.role = 'teacher'    AND cm_b.role = 'student')
    OR (cm_a.role = 'student'    AND cm_b.role = 'teacher')
    OR (cm_a.role = 'supervisor' AND cm_b.role = 'student')
    OR (cm_a.role = 'student'    AND cm_b.role = 'supervisor')
  )
 LIMIT 1`

// lockQualifyingDMCircleQuery serializes a direct mutation with membership,
// role, and archive changes by locking the qualifying circle and both member
// rows in the caller's transaction.
const lockQualifyingDMCircleQuery = `
SELECT cm_a.circle_id
FROM circle_members cm_a
JOIN circle_members cm_b ON cm_b.circle_id = cm_a.circle_id AND cm_b.user_id = $2::uuid
JOIN circles c ON c.id = cm_a.circle_id
WHERE cm_a.user_id = $1::uuid
  AND c.is_archived = FALSE
  AND (
       (cm_a.role = 'teacher'    AND cm_b.role = 'student')
    OR (cm_a.role = 'student'    AND cm_b.role = 'teacher')
    OR (cm_a.role = 'supervisor' AND cm_b.role = 'student')
    OR (cm_a.role = 'student'    AND cm_b.role = 'supervisor')
  )
ORDER BY cm_a.circle_id
LIMIT 1
FOR UPDATE OF c, cm_a, cm_b`

const lockOwnDirectMessageForDeleteQuery = `
SELECT deleted_at, sent_at
FROM messages
WHERE id = $1::uuid
  AND sender_id = $2::uuid
  AND dm_recipient_id = $3::uuid
  AND circle_id IS NULL
FOR UPDATE`

const findDirectMessageUploadForDeleteQuery = `
SELECT ` + messageColumns + `, ` + uploadColumns + `
FROM messages m
JOIN chat_uploads u ON u.id = m.upload_id
WHERE m.id = $1::uuid
  AND m.sender_id = $2::uuid
  AND m.dm_recipient_id = $3::uuid
  AND m.circle_id IS NULL
FOR UPDATE OF m, u`

// findMessageUploadQuery loads one message together with its attached upload
// for media renewal. Text messages carry no upload row and match nothing, so
// the caller can deny them non-enumeratingly.
const findMessageUploadQuery = `
SELECT ` + messageColumns + `, ` + uploadColumns + `
FROM messages m
JOIN chat_uploads u ON u.id = m.upload_id
WHERE m.id = $1::uuid`

// findUploadForUpdateQuery loads one upload row locked by the enclosing
// transaction so concurrent attach attempts serialize on the row before the
// guarded staged→attached transition.
const findUploadForUpdateQuery = `
SELECT ` + uploadColumns + `
FROM chat_uploads u
WHERE u.id = $1::uuid
FOR UPDATE`

// --- Moderation audits -------------------------------------------------------

// insertModerationAuditQuery appends one content-free moderation fact.
const insertModerationAuditQuery = `
INSERT INTO message_moderation_audits (message_id, circle_id, actor_id, action)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4)
RETURNING id, occurred_at`
