package scheduling

// One-off planned sessions use the F-005 session lifecycle plus F-006 details.
const insertPlannedSessionQuery = `
INSERT INTO sessions (circle_id, created_by, scheduled_at)
VALUES ($1::uuid, $2::uuid, $3)
RETURNING id::text`

const insertOneOffPlannedDetailsQuery = `
INSERT INTO planned_session_details (
	session_id, title, start_local_time, end_local_time, duration_minutes,
	planned_end_at, planning_timezone, version
) VALUES ($1::uuid, $2, $3::time, $4::time, $5, $6, $7, 1)`

const getOneOffCircleQuery = `
SELECT s.circle_id::text
FROM sessions s JOIN planned_session_details d ON d.session_id = s.id
WHERE s.id = $1::uuid AND d.schedule_id IS NULL`

// One-off edits and starts share the F-005 per-session transaction lock.
const lockOneOffSessionAdvisoryQuery = `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`

const lockOneOffPlannedSessionQuery = `
SELECT s.circle_id::text, s.status, s.actual_start, d.version
FROM sessions s JOIN planned_session_details d ON d.session_id = s.id
WHERE s.id = $1::uuid AND d.schedule_id IS NULL
FOR UPDATE OF s, d`

const updateOneOffPlanQuery = `
UPDATE planned_session_details d
SET title=$2, start_local_time=$3::time, end_local_time=$4::time,
	duration_minutes=$5, planned_end_at=$6, planning_timezone=$7,
	version=d.version+1
FROM sessions s
WHERE d.session_id=$1::uuid AND d.schedule_id IS NULL
	AND s.id=d.session_id AND s.status='scheduled' AND s.actual_start IS NULL`

const updateOneOffStartQuery = `
UPDATE sessions SET scheduled_at=$2, updated_at=NOW()
WHERE id=$1::uuid AND status='scheduled' AND actual_start IS NULL`

const updateOneOffCancellationQuery = `
UPDATE planned_session_details SET cancelled_at=$2, version=version+1
WHERE session_id=$1::uuid AND schedule_id IS NULL`

const getOneOffPlannedSessionQuery = `
SELECT s.id::text, s.circle_id::text, c.name, COALESCE(d.title, 'Circle Session'),
	s.status, s.scheduled_at, d.planned_end_at,
	to_char(d.start_local_time, 'HH24:MI'), to_char(d.end_local_time, 'HH24:MI'),
	d.duration_minutes, d.planning_timezone, d.cancelled_at, d.version
FROM sessions s
JOIN planned_session_details d ON d.session_id=s.id AND d.schedule_id IS NULL
JOIN circles c ON c.id=s.circle_id
WHERE s.id=$1::uuid`
