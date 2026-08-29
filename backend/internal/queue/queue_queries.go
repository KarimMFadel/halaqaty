package queue

// This file is the ONLY home of runtime SQL for the queue package
// (constitution IV.7). Every statement is parameterized; repository methods
// reference these consts and never inline SQL.
//
// One deliberate deviation from the task letter: the per-session round
// allocation lock anchors on the sessions row, not on "rounds by session"
// FOR UPDATE — a session with zero rounds locks nothing under FOR UPDATE,
// so two concurrent first-round creations would both read MAX=0. The
// sessions row always exists, making it a stable per-session lock anchor
// (the sessions package uses an advisory lock for the same purpose).

// roundColumns is the canonical round projection shared by reads, RETURNING
// clauses, and locked claims. Nullable enum-ish values keep their NULL so
// the domain pointers (SelectedEntryID, ActivatedAt, FinalizedAt) round-trip.
const roundColumns = `id::text, session_id::text, round_number, round_type, surah_id, from_ayah, to_ayah,
	grading_required, lifecycle, selected_entry_id::text, version, created_by::text, created_at, activated_at, finalized_at`

// entryColumns is the canonical entry projection. grade and teacher_notes
// are selected as their own columns so the repository can drop them per
// viewer role when assembling visibility-filtered projections
// (data-model §Visibility projection).
const entryColumns = `id::text, queue_id::text, student_id::text, position, status, grade, teacher_notes,
	version, started_at, completed_at, resolved_by::text, created_at, updated_at`

// receiptColumns is the command-receipt projection; it structurally carries
// no grade, note, name, or media material (data-model §queue_command_receipts).
const receiptColumns = `session_id::text, actor_id::text, idempotency_key, command, resource_id::text, result_version, created_at`

// outboxColumns is the outbox-event projection; event_metadata is
// server-built transition facts only (data-model §queue_event_outbox).
const outboxColumns = `event_id::text, session_id::text, round_id::text, event_type, resource_id::text,
	round_version, event_metadata, available_at, delivered_at, attempt_count, parked_at`

// --- Authorization lookups -------------------------------------------------

// findSessionCircleRoleQuery returns the viewer's current circle role for a
// session ('teacher', 'supervisor', 'student') or ” when the session is
// unknown or the user is not an active member. Point query — never loads the
// member list.
const findSessionCircleRoleQuery = `
SELECT COALESCE(cm.role, '')
FROM sessions s
LEFT JOIN circle_members cm ON cm.circle_id = s.circle_id AND cm.user_id = $2::uuid
WHERE s.id = $1::uuid`

// findSessionQueuePolicyQuery reads the five policy dimensions with their
// version plus the session status and owning circle needed by authorization
// and policy-change guards.
const findSessionQueuePolicyQuery = `
SELECT queue_population_policy, queue_finalization_policy, queue_opt_out_policy,
       queue_grade_visibility, queue_grade_correction, queue_policy_version,
       status, circle_id::text
FROM sessions
WHERE id = $1::uuid`

const lockSessionQueuePolicyQuery = findSessionQueuePolicyQuery + `
FOR UPDATE`

// findSessionCircleIDQuery derives durable progress ownership from the
// authoritative session instead of a caller-provided field.
const findSessionCircleIDQuery = `
SELECT circle_id::text FROM sessions WHERE id = $1::uuid`

// updateSessionPolicyQuery is the optimistic policy patch: it rewrites the
// five dimensions and increments queue_policy_version only when the expected
// version still matches (data-model: increments only on effective change —
// the repository compares values first and skips this statement entirely on
// no-op patches).
const updateSessionPolicyQuery = `
UPDATE sessions
SET queue_population_policy = $2,
    queue_finalization_policy = $3,
    queue_opt_out_policy = $4,
    queue_grade_visibility = $5,
    queue_grade_correction = $6,
    queue_policy_version = queue_policy_version + 1
WHERE id = $1::uuid
  AND status IN ('scheduled', 'active')
  AND queue_policy_version = $7
RETURNING queue_population_policy, queue_finalization_policy, queue_opt_out_policy,
          queue_grade_visibility, queue_grade_correction, queue_policy_version`

// --- Locked claims ----------------------------------------------------------

// lockRoundAllocationQuery is the per-session round allocation lock: it
// takes the sessions row FOR UPDATE so round create/activate/reset
// transactions serialize per session and MAX(round_number)+1 stays stable
// (CHK036). The sessions row is the anchor because the round set may be
// empty, in which case a rounds-only FOR UPDATE would lock nothing.
const lockRoundAllocationQuery = `
SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))
FROM sessions
WHERE id = $1::uuid
FOR UPDATE`

const acquireRoundAllocationLockQuery = `
SELECT pg_advisory_lock(hashtextextended($1::text, 0))`

const releaseRoundAllocationLockQuery = `
SELECT pg_advisory_unlock(hashtextextended($1::text, 0))`

// lockRoundByIDQuery loads one round holding its row lock so advance, start,
// complete, move, and finalize decisions serialize per round.
const lockRoundByIDQuery = `
SELECT ` + roundColumns + `
FROM recitation_queue
WHERE id = $1::uuid
FOR UPDATE`

const lockActiveRoundQuery = `
SELECT ` + roundColumns + `
FROM recitation_queue
WHERE session_id = $1::uuid AND lifecycle = 'active'
FOR UPDATE`

const lockLowestPreparedRoundQuery = `
SELECT ` + roundColumns + `
FROM recitation_queue
WHERE session_id = $1::uuid AND lifecycle = 'prepared'
ORDER BY round_number
LIMIT 1
FOR UPDATE`

const activateRoundQuery = `
UPDATE recitation_queue
SET lifecycle = 'active', activated_at = NOW(), version = version + 1
WHERE id = $1::uuid AND lifecycle = 'prepared'
RETURNING ` + roundColumns

// lockEntryByIDQuery loads one entry holding its row lock so state
// transitions serialize per entry inside the round lock.
const lockEntryByIDQuery = `
SELECT ` + entryColumns + `
FROM recitation_queue_entries
WHERE id = $1::uuid
FOR UPDATE`

const findEntryQueueIDQuery = `SELECT queue_id::text FROM recitation_queue_entries WHERE id = $1::uuid`

// nextRoundNumberQuery allocates the next sequential round number as
// MAX+1; it must run under lockRoundAllocationQuery (CHK036 — no reuse,
// stable under retries and concurrent creation).
const nextRoundNumberQuery = `
SELECT COALESCE(MAX(round_number), 0) + 1 FROM recitation_queue WHERE session_id = $1::uuid`

// --- Round / entry mutations ------------------------------------------------

// insertRoundQuery creates a round in the requested lifecycle (prepared or
// active); activated_at is set exactly when the round is born active.
const insertRoundQuery = `
INSERT INTO recitation_queue (session_id, round_number, round_type, surah_id, from_ayah, to_ayah,
                              grading_required, lifecycle, created_by, activated_at)
VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9::uuid, CASE WHEN $8::varchar = 'active' THEN NOW() ELSE NULL END)
RETURNING ` + roundColumns

// updateRoundSelectionQuery replaces the round's selected entry (advance
// replaces any existing selection) and increments the version, guarded by
// the expected optimistic-concurrency version.
const updateRoundSelectionQuery = `
UPDATE recitation_queue
SET selected_entry_id = $2::uuid, version = version + 1
WHERE id = $1::uuid
  AND version = $3
  AND ($2::uuid IS NULL OR EXISTS (
      SELECT 1 FROM recitation_queue_entries e
      WHERE e.id = $2::uuid AND e.queue_id = recitation_queue.id
  ))
RETURNING ` + roundColumns

// clearRoundSelectionQuery clears a terminal entry's selection with the same
// round-version guard as selection replacement.
const clearRoundSelectionQuery = `
UPDATE recitation_queue
SET selected_entry_id = NULL, version = version + 1
WHERE id = $1::uuid AND selected_entry_id = $2::uuid AND version = $3
RETURNING ` + roundColumns

// bumpRoundVersionQuery increments the round version for queue-visible
// mutations that do not otherwise touch the round row (e.g. entry completion).
const bumpRoundVersionQuery = `
UPDATE recitation_queue SET version = version + 1 WHERE id = $1::uuid`

// finalizeRoundQuery applies the active→finalized (or prepared→finalized)
// transition, clears the selection, stamps finalized_at, and increments the
// version, guarded by the expected version. finalized is terminal.
const finalizeRoundQuery = `
UPDATE recitation_queue
SET lifecycle = 'finalized', selected_entry_id = NULL, finalized_at = NOW(), version = version + 1
WHERE id = $1::uuid AND version = $2
RETURNING ` + roundColumns

// markUnfinishedEntriesSkippedQuery applies the mark_unfinished_skipped
// finalization policy: every waiting or reciting entry becomes skipped with
// durable manager attribution. Runs in the finalization transaction.
const markUnfinishedEntriesSkippedQuery = `
UPDATE recitation_queue_entries
SET status = 'skipped', resolved_by = $2::uuid, version = version + 1, updated_at = NOW()
WHERE queue_id = $1::uuid AND status IN ('waiting', 'reciting')`

// selectNextWaitingEntryQuery claims the next waiting entry of a round in
// position order, holding its row lock for the advance transaction.
const selectNextWaitingEntryQuery = `
SELECT ` + entryColumns + `
FROM recitation_queue_entries
WHERE queue_id = $1::uuid
  AND status = 'waiting'
  AND id IS DISTINCT FROM (SELECT selected_entry_id FROM recitation_queue WHERE id = $1::uuid)
ORDER BY position
LIMIT 1
FOR UPDATE`

// insertQueueEntryQuery materializes one waiting entry at its activation
// position (data-model §Activation materialization).
const insertQueueEntryQuery = `
INSERT INTO recitation_queue_entries (queue_id, student_id, position, status)
VALUES ($1::uuid, $2::uuid, $3, 'waiting')
RETURNING ` + entryColumns

// updateEntryTransitionQuery applies an entry state transition guarded by the
// expected entry version; grade and notes ride along (non-NULL only for
// completion), started_at/completed_at are set by the repository per target
// status, and resolved_by carries terminal attribution.
const updateEntryTransitionQuery = `
UPDATE recitation_queue_entries
SET status = $4, grade = $5, teacher_notes = $6, started_at = $7, completed_at = $8,
    resolved_by = $9::uuid, version = version + 1, updated_at = NOW()
WHERE id = $1::uuid AND version = $2 AND status = $3
RETURNING ` + entryColumns

// updateEntryPositionQuery moves one entry to a new slot; the caller holds
// the round lock and rewrites the affected positions inside one transaction
// (move semantics A2a — reciting and terminal entries are immovable, a
// service-layer rule enforced before this primitive runs).
const updateEntryPositionQuery = `
UPDATE recitation_queue_entries
SET position = $2, version = version + 1, updated_at = NOW()
WHERE id = $1::uuid`

// updatePreorderPositionQuery rewrites one pre-activation candidate slot;
// full-list reorder composes these inside one transaction while the round is
// prepared.
const updatePreorderPositionQuery = `
UPDATE recitation_queue_preorder
SET position = $3
WHERE queue_id = $1::uuid AND student_id = $2::uuid`

const deletePreorderQuery = `DELETE FROM recitation_queue_preorder WHERE queue_id = $1::uuid`

const insertPreorderQuery = `
INSERT INTO recitation_queue_preorder (queue_id, student_id, position, added_by)
VALUES ($1::uuid, $2::uuid, $3, $4::uuid)`

const countActivePreorderStudentsQuery = `
SELECT COUNT(*)
FROM recitation_queue q
JOIN sessions s ON s.id = q.session_id
JOIN circle_members cm ON cm.circle_id = s.circle_id
WHERE q.id = $1::uuid
  AND cm.role = 'student'
  AND cm.user_id = ANY($2::uuid[])`

const lockQueueEntriesQuery = `
SELECT ` + entryColumns + `
FROM recitation_queue_entries
WHERE queue_id = $1::uuid
ORDER BY position
FOR UPDATE`

const setAllEntryPositionsQuery = `
UPDATE recitation_queue_entries SET position = position + 1000000 WHERE queue_id = $1::uuid`

// --- Population (activation materialization) --------------------------------

// listPresentAtActivationPopulationQuery yields the activation order under
// present_at_activation: pre-ordered students who are currently present keep
// their relative order first; then currently-present student members without
// a preorder row follow by F-005 first_joined_at with the user UUID
// tie-break (deterministic under concurrent equal timestamps).
const listPresentAtActivationPopulationQuery = `
SELECT student_id::text FROM (
    SELECT po.student_id, 0 AS seg, po.position::bigint AS seg_order, NULL::timestamptz AS joined_at, NULL::uuid AS tie_break
    FROM recitation_queue_preorder po
    JOIN sessions s ON s.id = $1::uuid
    JOIN circle_members pcm ON pcm.circle_id = s.circle_id AND pcm.user_id = po.student_id AND pcm.role = 'student'
    JOIN session_participant_presence p
      ON p.session_id = $1::uuid AND p.user_id = po.student_id AND p.is_currently_present
    WHERE po.queue_id = $2::uuid
    UNION ALL
    SELECT cm.user_id, 1, NULL::bigint, p.first_joined_at, cm.user_id
    FROM circle_members cm
    JOIN sessions s ON s.id = $1::uuid AND s.circle_id = cm.circle_id
    JOIN session_participant_presence p
      ON p.session_id = $1::uuid AND p.user_id = cm.user_id AND p.is_currently_present
    WHERE cm.role = 'student'
      AND NOT EXISTS (
          SELECT 1 FROM recitation_queue_preorder po
          WHERE po.queue_id = $2::uuid AND po.student_id = cm.user_id
      )
) population
ORDER BY seg, seg_order, joined_at, tie_break`

// listAllActiveStudentsPopulationQuery yields the activation order under
// all_active_students: pre-ordered students first (no presence gate), then
// every remaining student member by circle_members.joined_at with the user
// UUID tie-break.
const listAllActiveStudentsPopulationQuery = `
SELECT student_id::text FROM (
    SELECT po.student_id, 0 AS seg, po.position::bigint AS seg_order, NULL::timestamptz AS joined_at, NULL::uuid AS tie_break
    FROM recitation_queue_preorder po
    JOIN sessions s ON s.id = $1::uuid
    JOIN circle_members pcm ON pcm.circle_id = s.circle_id AND pcm.user_id = po.student_id AND pcm.role = 'student'
    WHERE po.queue_id = $2::uuid
    UNION ALL
    SELECT cm.user_id, 1, NULL::bigint, cm.joined_at, cm.user_id
    FROM circle_members cm
    JOIN sessions s ON s.id = $1::uuid AND s.circle_id = cm.circle_id
    WHERE cm.role = 'student'
      AND NOT EXISTS (
          SELECT 1 FROM recitation_queue_preorder po
          WHERE po.queue_id = $2::uuid AND po.student_id = cm.user_id
      )
) population
ORDER BY seg, seg_order, joined_at, tie_break`

// --- Command receipts -------------------------------------------------------

// insertCommandReceiptQuery reserves an idempotency key: it inserts only
// when the (session, actor, key) triple is free. Concurrent same-key
// inserts serialize on the primary key; the loser then reads the winner's
// committed row via findCommandReceiptQuery.
const insertCommandReceiptQuery = `
INSERT INTO queue_command_receipts (session_id, actor_id, idempotency_key, command)
VALUES ($1::uuid, $2::uuid, $3, $4)
ON CONFLICT (session_id, actor_id, idempotency_key) DO NOTHING`

// findCommandReceiptQuery loads the committed receipt for one idempotency
// key; it backs the replay path (same command) and the duplicate-command
// conflict (same key, another command).
const findCommandReceiptQuery = `
SELECT ` + receiptColumns + `
FROM queue_command_receipts
WHERE session_id = $1::uuid AND actor_id = $2::uuid AND idempotency_key = $3`

// updateCommandReceiptResultQuery records the committed resource and round
// version on the reserved receipt so replays return the committed outcome.
const updateCommandReceiptResultQuery = `
UPDATE queue_command_receipts
SET resource_id = $4::uuid, result_version = $5
WHERE session_id = $1::uuid AND actor_id = $2::uuid AND idempotency_key = $3`

// --- Transactional outbox ---------------------------------------------------

// insertOutboxEventQuery writes one client event or internal convergence
// intent in the same transaction as the queue mutation it describes.
const insertOutboxEventQuery = `
INSERT INTO queue_event_outbox (event_id, session_id, round_id, event_type, resource_id,
                                round_version, event_metadata, available_at, attempt_count)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::uuid, $6, $7, $8, $9)`

// claimDueOutboxEventsQuery claims due, undelivered, unparked events in due
// order. SKIP LOCKED lets parallel dispatchers partition the backlog without
// blocking each other.
const claimDueOutboxEventsQuery = `
SELECT ` + outboxColumns + `
FROM queue_event_outbox
WHERE available_at <= NOW() AND delivered_at IS NULL AND parked_at IS NULL
ORDER BY available_at, event_id
LIMIT $1
FOR UPDATE SKIP LOCKED`

// claimReplayOutboxEventsQuery is used at client dispatcher startup. It
// includes parked rows so restart recovery makes every undelivered event
// observable to the projector.
const claimReplayOutboxEventsQuery = `
SELECT ` + outboxColumns + `
FROM queue_event_outbox
WHERE delivered_at IS NULL
ORDER BY parked_at NULLS FIRST, available_at, event_id
LIMIT $1
FOR UPDATE SKIP LOCKED`

// markOutboxEventDeliveredQuery completes an event; the delivered guard makes
// duplicate marks converge and also permits successful operator replay of a
// parked event.
const markOutboxEventDeliveredQuery = `
UPDATE queue_event_outbox
SET delivered_at = NOW()
WHERE event_id = $1::uuid AND delivered_at IS NULL`

// retryOutboxEventQuery counts one failed delivery attempt and schedules the
// next try; the backoff interval is computed in Go and passed as parameter.
const retryOutboxEventQuery = `
UPDATE queue_event_outbox
SET attempt_count = attempt_count + 1, available_at = $2
WHERE event_id = $1::uuid AND delivered_at IS NULL AND parked_at IS NULL`

// parkOutboxEventQuery parks an event whose retries are exhausted; parked
// rows await operator replay and are never silently dropped.
const parkOutboxEventQuery = `
UPDATE queue_event_outbox
SET parked_at = NOW()
WHERE event_id = $1::uuid AND delivered_at IS NULL AND parked_at IS NULL`

// countOutboxEventsQuery reports the pending and parked outbox backlog for a
// session (operational counts; the dispatcher alert consumes them).
const countOutboxEventsQuery = `
SELECT COUNT(*) FILTER (WHERE delivered_at IS NULL AND parked_at IS NULL),
       COUNT(*) FILTER (WHERE delivered_at IS NULL AND parked_at IS NOT NULL)
FROM queue_event_outbox
WHERE session_id = $1::uuid`

// --- Progress ---------------------------------------------------------------

// upsertProgressQuery is the idempotent completion / re-grade upsert keyed
// by queue_entry_id: the first completion inserts, later corrections update
// grade/notes/updated_at on the same row (CHK035 — corrections converge to
// one record state; immutable round facts never change).
const upsertProgressQuery = `
INSERT INTO memorization_progress (student_id, circle_id, session_id, queue_entry_id, surah_id,
                                   surah_name, from_ayah, to_ayah, type, grade, notes, date)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, $8, $9, $10, $11, $12)
ON CONFLICT (queue_entry_id) DO UPDATE
SET grade = EXCLUDED.grade, notes = EXCLUDED.notes, updated_at = NOW()`

// --- Visibility projection --------------------------------------------------

// findQueueRoundQuery loads one round without locking (snapshot reads).
const findQueueRoundQuery = `
SELECT ` + roundColumns + `
FROM recitation_queue
WHERE id = $1::uuid`

// listQueueEntriesQuery loads a round's entries in position order; grade and
// teacher_notes are dropped per policy + viewer by the repository before the
// projection leaves the package.
const listQueueEntriesQuery = `
SELECT ` + entryColumns + `
FROM recitation_queue_entries
WHERE queue_id = $1::uuid
ORDER BY position`

// listPreorderCandidatesQuery loads the manager-only pre-activation
// candidate list (CHK008 — non-managers receive an empty array).
const listPreorderCandidatesQuery = `
SELECT queue_id::text, student_id::text, position, added_by::text, created_at
FROM recitation_queue_preorder
WHERE queue_id = $1::uuid
ORDER BY position`
