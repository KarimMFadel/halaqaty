package attendance

const snapshotStartRosterQuery = `
INSERT INTO session_attendance (session_id, user_id, roster_source)
SELECT $1::uuid, cm.user_id, 'start_snapshot'
FROM sessions s
JOIN circles c ON c.id = s.circle_id AND NOT c.is_archived
JOIN circle_members cm ON cm.circle_id = s.circle_id AND cm.role = 'student'
WHERE s.id = $1::uuid AND s.status = 'active' AND s.actual_start IS NOT NULL
ON CONFLICT (session_id, user_id) DO NOTHING
`

const lockStudentMembershipQuery = `
SELECT cm.role
FROM sessions s
JOIN circles c ON c.id = s.circle_id AND NOT c.is_archived
JOIN circle_members cm ON cm.circle_id = s.circle_id AND cm.user_id = $2::uuid
WHERE s.id = $1::uuid AND s.status = 'active'
FOR KEY SHARE OF cm
`

const addLaterParticipantQuery = `
INSERT INTO session_attendance (session_id, user_id, first_presence_at, roster_source)
SELECT $1::uuid, p.user_id, p.first_joined_at, 'later_participant'
FROM session_participant_presence p
JOIN sessions s ON s.id = p.session_id AND s.status = 'active'
JOIN circle_members cm ON cm.circle_id = s.circle_id AND cm.user_id = p.user_id AND cm.role = 'student'
WHERE p.session_id = $1::uuid AND p.user_id = $2::uuid
  AND p.first_joined_at IS NOT NULL
ON CONFLICT (session_id, user_id) DO NOTHING
`

const attendanceSessionFactsQuery = `
SELECT actual_start, actual_end, status
FROM sessions
WHERE id = $1::uuid
FOR UPDATE
`

const attendanceRosterQuery = `
SELECT a.user_id::text, COALESCE(a.first_presence_at, p.first_joined_at)
FROM session_attendance a
LEFT JOIN session_participant_presence p ON p.session_id = a.session_id AND p.user_id = a.user_id
WHERE a.session_id = $1::uuid
ORDER BY a.user_id
`

const finalizeAttendanceRowQuery = `
UPDATE session_attendance a
SET first_presence_at = $3,
    base_status = $4,
    effective_status = COALESCE((
        SELECT c.new_status
        FROM attendance_corrections c
        WHERE c.session_id = a.session_id AND c.user_id = a.user_id
        ORDER BY c.at DESC, c.id DESC
        LIMIT 1
    ), $4),
    finalized_at = COALESCE(a.finalized_at, $5),
    updated_at = NOW()
WHERE a.session_id = $1::uuid AND a.user_id = $2::uuid
`

const refreshEndedSessionRecoveryOrderQuery = `
UPDATE sessions SET updated_at = NOW()
WHERE id = $1::uuid AND status = 'ended'
`

const lockAttendanceCorrectionScopeQuery = `
SELECT s.circle_id::text, s.status, c.is_archived
FROM sessions s
JOIN circles c ON c.id = s.circle_id
WHERE s.id = $1::uuid
FOR UPDATE OF s, c
`

const lockAttendanceCorrectionMemberQuery = `
SELECT role FROM circle_members
WHERE circle_id = $1::uuid AND user_id = $2::uuid
FOR KEY SHARE
`

const lockAttendanceCorrectionReplayQuery = `
SELECT pg_advisory_xact_lock(hashtextextended($1::text || ':' || $2::text, 0))
`

const loadAttendanceCorrectionReplayQuery = `
SELECT command_fingerprint, response_resource_id::text
FROM schedule_request_replays
WHERE actor_id = $1::uuid AND idempotency_key = $2
`

const insertAttendanceCorrectionReplayQuery = `
INSERT INTO schedule_request_replays (actor_id, idempotency_key, command_fingerprint, response_status, response_resource_id)
VALUES ($1::uuid, $2, $3, 200, $4::uuid)
`

const lockEffectiveAttendanceQuery = `
SELECT effective_status FROM session_attendance
WHERE session_id = $1::uuid AND user_id = $2::uuid AND finalized_at IS NOT NULL
FOR UPDATE
`

const insertAttendanceCorrectionQuery = `
INSERT INTO attendance_corrections (session_id, user_id, actor_id, reason, previous_status, new_status)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5, $6)
RETURNING id::text, at
`

const updateEffectiveAttendanceQuery = `
UPDATE session_attendance SET effective_status = $3, updated_at = NOW()
WHERE session_id = $1::uuid AND user_id = $2::uuid AND finalized_at IS NOT NULL
`

const getAttendanceCorrectionByIDQuery = `
SELECT a.session_id::text, a.user_id::text, a.effective_status,
       'manual', a.first_presence_at, c.id::text, c.actor_id::text, c.at,
       c.reason, c.previous_status, c.new_status
FROM attendance_corrections c
JOIN session_attendance a ON a.session_id = c.session_id AND a.user_id = c.user_id
WHERE c.id = $1::uuid
`

const attendanceReadScopeQuery = `
SELECT s.status, c.is_archived, cm.role
FROM sessions s
JOIN circles c ON c.id = s.circle_id
JOIN circle_members cm ON cm.circle_id = s.circle_id AND cm.user_id = $2::uuid
WHERE s.id = $1::uuid
FOR KEY SHARE OF cm
`

const listAttendanceQuery = `
SELECT a.session_id::text, a.user_id::text, a.effective_status,
       CASE WHEN correction.id IS NULL THEN 'automatic' ELSE 'manual' END,
       a.first_presence_at, correction.id::text, correction.actor_id::text,
       correction.at, correction.reason, correction.previous_status, correction.new_status
FROM session_attendance a
LEFT JOIN LATERAL (
    SELECT id, actor_id, at, reason, previous_status, new_status
    FROM attendance_corrections
    WHERE session_id = a.session_id AND user_id = a.user_id
    ORDER BY at DESC, id DESC LIMIT 1
) correction ON TRUE
WHERE a.session_id = $1::uuid AND a.finalized_at IS NOT NULL
  AND ($2::text IN ('teacher', 'supervisor') OR a.user_id = $3::uuid)
ORDER BY a.user_id
`
