package chat

// This file holds the runtime SQL added by the outbox dispatcher (T026). Like
// queries.go, every statement is parameterized and referenced from methods —
// SQL never lives inside method bodies.

// claimReplayChatOutboxEventsQuery leases due pending rows and parked rows for
// startup/operator replay. A parked row restarts with a fresh five-attempt
// budget (attempt_count = 1 counts this replay attempt); a pending row keeps
// counting attempts like the normal due-claim. Parked rows must reset their
// budget because the due-claim skips rows at the attempt ceiling. The claimed
// CTE also captures was_parked from the pre-update row: Postgres RETURNING
// yields post-update values, so the cleared parked_at cannot express the
// pre-claim parked state the recovered audit needs.
const claimReplayChatOutboxEventsQuery = `
WITH claimed AS (
    SELECT event_id AS claimed_event_id, parked_at IS NOT NULL AS was_parked
    FROM chat_event_outbox
    WHERE delivered_at IS NULL AND (parked_at IS NOT NULL OR (available_at <= NOW() AND attempt_count < 5))
    ORDER BY parked_at NULLS FIRST, available_at, event_id
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
UPDATE chat_event_outbox AS outbox
SET attempt_count = CASE WHEN outbox.parked_at IS NOT NULL THEN 1 ELSE outbox.attempt_count + 1 END,
    parked_at = NULL,
    available_at = NOW() + INTERVAL '30 seconds'
FROM claimed
WHERE outbox.event_id = claimed.claimed_event_id
RETURNING ` + outboxColumns + `, claimed.was_parked`

// markChatOutboxEventDeliveredQuery completes an event; the delivered guard
// makes duplicate marks converge and permits successful operator replay of a
// previously parked event.
const markChatOutboxEventDeliveredQuery = `
UPDATE chat_event_outbox
SET delivered_at = NOW()
WHERE event_id = $1::uuid AND delivered_at IS NULL`

// retryChatOutboxEventQuery reschedules one failed delivery. Unlike the queue
// outbox, attempts are already counted by the claim statements, so a retry
// only moves the availability horizon.
const retryChatOutboxEventQuery = `
UPDATE chat_event_outbox
SET available_at = $2
WHERE event_id = $1::uuid AND delivered_at IS NULL AND parked_at IS NULL`

// parkChatOutboxEventQuery parks an event whose five attempts are exhausted;
// the table constraint enforces attempt_count = 5 for parked rows. Parked rows
// await explicit replay and are never silently dropped.
const parkChatOutboxEventQuery = `
UPDATE chat_event_outbox
SET parked_at = NOW()
WHERE event_id = $1::uuid AND delivered_at IS NULL AND parked_at IS NULL`

// findMessageForProjectionQuery reloads the durable message that backs one
// outbox event so delivery payloads are rebuilt from current database state
// and stored bodies are never trusted. Soft-deleted messages match no row.
const findMessageForProjectionQuery = `
SELECT ` + messageColumns + `
FROM messages m
WHERE m.id = $1::uuid AND m.deleted_at IS NULL`

const findDeletedMessageForProjectionQuery = `
SELECT ` + messageColumns + `
FROM messages m
WHERE m.id = $1::uuid AND m.deleted_at IS NOT NULL`

const releaseExpiredStagedUploadQuery = `UPDATE chat_uploads SET state = 'staged', updated_at = NOW() WHERE id = $1::uuid AND state = 'revoked'`
const finalizeExpiredStagedUploadQuery = `DELETE FROM chat_uploads WHERE id = $1::uuid AND state = 'revoked'`
