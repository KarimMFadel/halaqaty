package sessions

// sessionColumns is the canonical session projection shared by reads and
// RETURNING clauses. Nullable enum-ish columns are coalesced so they scan
// into the zero values of the domain string types.
const sessionColumns = `id::text, circle_id::text, created_by::text, status, scheduled_at, actual_start, actual_end,
	 COALESCE(end_reason, ''), media_mode, COALESCE(media_room_ref, ''), is_locked, participant_count, created_at, updated_at`

// insertAdHocSessionQuery creates an F-005 ad-hoc session row relying on the
// column defaults: status scheduled, media_mode audio_only, count 0, and
// scheduled_at NULL until F-006 owns scheduling.
const insertAdHocSessionQuery = `
INSERT INTO sessions (circle_id, created_by)
VALUES ($1::uuid, $2::uuid)
RETURNING ` + sessionColumns

// findSessionByIDQuery loads one session without locking.
const findSessionByIDQuery = `
SELECT ` + sessionColumns + `
FROM sessions
WHERE id = $1::uuid
`

// lockSessionByIDQuery loads one session holding its row lock so join,
// reconnect, leave, and removal decisions serialize per session.
const lockSessionByIDQuery = `
SELECT ` + sessionColumns + `
FROM sessions
WHERE id = $1::uuid
FOR UPDATE
`

// startSessionQuery is the scheduled→active compare-and-set; it persists the
// opaque room reference exactly when the transition applies (ADR-015).
const startSessionQuery = `
UPDATE sessions
SET status = 'active', actual_start = NOW(), media_room_ref = $2, updated_at = NOW()
WHERE id = $1::uuid AND status = 'scheduled'
RETURNING ` + sessionColumns

// endSessionQuery is the active→ended compare-and-set with durable end
// attribution.
const endSessionQuery = `
UPDATE sessions
SET status = 'ended', actual_end = NOW(), end_reason = $2, updated_at = NOW()
WHERE id = $1::uuid AND status = 'active'
RETURNING ` + sessionColumns

// setSessionLockQuery applies the lock only while the session is active;
// replaying the current value converges to the same durable state.
const setSessionLockQuery = `
UPDATE sessions
SET is_locked = $2, updated_at = NOW()
WHERE id = $1::uuid AND status = 'active'
RETURNING ` + sessionColumns

// incrementParticipantCountQuery admits one more concurrently present
// participant; the 0..50 CHECK backstops the guarded decision (FR-009).
const incrementParticipantCountQuery = `
UPDATE sessions
SET participant_count = participant_count + 1, updated_at = NOW()
WHERE id = $1::uuid
RETURNING ` + sessionColumns

// decrementParticipantCountQuery releases one present participant slot.
const decrementParticipantCountQuery = `
UPDATE sessions
SET participant_count = participant_count - 1, updated_at = NOW()
WHERE id = $1::uuid
RETURNING ` + sessionColumns

// findPresenceQuery loads the eligibility facts for one participant: whether
// they are currently present and whether they were removed.
const findPresenceQuery = `
SELECT is_currently_present, removed_at
FROM session_participant_presence
WHERE session_id = $1::uuid AND user_id = $2::uuid
`

// insertPresenceQuery records the first join of a participant.
const insertPresenceQuery = `
INSERT INTO session_participant_presence (session_id, user_id, first_joined_at, last_joined_at, is_currently_present)
VALUES ($1::uuid, $2::uuid, NOW(), NOW(), TRUE)
`

// markPresencePresentQuery transitions an eligible absent participant back to
// present and counts the rejoin (FR-010).
const markPresencePresentQuery = `
UPDATE session_participant_presence
SET is_currently_present = TRUE, last_joined_at = NOW(), reconnect_count = reconnect_count + 1
WHERE session_id = $1::uuid AND user_id = $2::uuid
`

// markPresenceLeftQuery transitions a present participant out; the
// is_currently_present guard makes duplicate leaves no-ops.
const markPresenceLeftQuery = `
UPDATE session_participant_presence
SET is_currently_present = FALSE, last_left_at = NOW(), hand_raised_at = NULL
WHERE session_id = $1::uuid AND user_id = $2::uuid AND is_currently_present
`

// clearSessionPresenceQuery clears only the transient current-presence and
// hand state of a session that just ended; durable history is retained
// (FR-015, data-model invariant 5). Hands are cleared only on
// currently-present rows, so an absent participant's raised hand may survive
// session end — intentional, as no reader consumes hand state after the end.
const clearSessionPresenceQuery = `
UPDATE session_participant_presence
SET is_currently_present = FALSE, hand_raised_at = NULL
WHERE session_id = $1::uuid AND is_currently_present
`

// removePresenceQuery durably removes a participant; the removed_at guard
// makes duplicate removals converge. last_left_at records the forced leave
// only for a participant who was present, using pre-update column values.
const removePresenceQuery = `
UPDATE session_participant_presence
SET removed_at = COALESCE(removed_at, NOW()),
    is_currently_present = FALSE,
    hand_raised_at = NULL,
    last_left_at = CASE WHEN is_currently_present THEN NOW() ELSE last_left_at END
WHERE session_id = $1::uuid AND user_id = $2::uuid AND removed_at IS NULL
`

// raiseHandQuery records a raised hand for a currently present participant.
const raiseHandQuery = `
UPDATE session_participant_presence
SET hand_raised_at = COALESCE(hand_raised_at, NOW())
WHERE session_id = $1::uuid AND user_id = $2::uuid AND is_currently_present AND removed_at IS NULL
`

// lowerHandQuery clears the hand state; replaying on an already lowered hand
// converges to the same durable state (FR-016).
const lowerHandQuery = `
UPDATE session_participant_presence
SET hand_raised_at = NULL
WHERE session_id = $1::uuid AND user_id = $2::uuid AND is_currently_present AND removed_at IS NULL
`

// listSessionParticipantsQuery is the authoritative presence snapshot with the
// circle member display-name fallback used across the codebase and the
// participant's current circle role for the public snapshot projection.
const listSessionParticipantsQuery = `
SELECT p.session_id::text, p.user_id::text,
       COALESCE(NULLIF(BTRIM(pr.display_name), ''), NULLIF(BTRIM(pr.full_name), ''), 'Member'),
       COALESCE(cm.role, 'student'),
       p.first_joined_at, p.last_joined_at, p.last_left_at, p.reconnect_count,
       p.is_currently_present, p.removed_at, p.hand_raised_at
FROM session_participant_presence p
JOIN sessions s ON s.id = p.session_id
LEFT JOIN profiles pr ON pr.user_id = p.user_id
LEFT JOIN circle_members cm ON cm.circle_id = s.circle_id AND cm.user_id = p.user_id
WHERE p.session_id = $1::uuid
ORDER BY p.first_joined_at, p.user_id
`
