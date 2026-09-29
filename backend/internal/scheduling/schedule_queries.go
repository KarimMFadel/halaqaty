package scheduling

const lockScheduleQuery = getScheduleByIDQuery + ` FOR UPDATE`

const lockOccurrenceSessionQuery = `
SELECT s.actual_start IS NOT NULL OR s.status <> 'scheduled'
FROM planned_session_details d JOIN sessions s ON s.id=d.session_id
WHERE d.schedule_id=$1::uuid AND d.original_local_date=$2 FOR UPDATE OF s,d`

const listUnstartedDetailsQuery = `
SELECT d.session_id::text, d.original_local_date
FROM planned_session_details d JOIN sessions s ON s.id=d.session_id
WHERE d.schedule_id=$1::uuid
AND s.actual_start IS NULL AND s.status='scheduled'
FOR UPDATE OF s,d`

const updatePlannedDetailQuery = `
UPDATE planned_session_details SET title=$3,start_local_time=$4::time,end_local_time=$5::time,
duration_minutes=$6,planned_end_at=$7,planning_timezone=$8,cancelled_at=$9,version=version+1
WHERE schedule_id=$1::uuid AND original_local_date=$2`

const updatePlannedStartQuery = `
UPDATE sessions s SET scheduled_at=$3 FROM planned_session_details d
WHERE d.session_id=s.id AND d.schedule_id=$1::uuid AND d.original_local_date=$2
AND s.actual_start IS NULL AND s.status='scheduled'`

const cancelPlannedDetailQuery = `
UPDATE planned_session_details d SET cancelled_at=$3,version=version+1
FROM sessions s WHERE s.id=d.session_id AND d.schedule_id=$1::uuid
AND d.original_local_date=$2 AND s.actual_start IS NULL AND s.status='scheduled'`

// F-006 schedule persistence statements (migration 000020, ADR-026). All
// statements are parameterized; repository methods reference them instead of
// inlining SQL.

const insertScheduleQuery = `
INSERT INTO schedules (circle_id, created_by, current_version)
VALUES ($1::uuid, $2::uuid, 1)
RETURNING id::text, circle_id::text, created_by::text, current_version,
	stopped_from_local_date, created_at, updated_at`

const getScheduleByIDQuery = `
SELECT id::text, circle_id::text, created_by::text, current_version,
	stopped_from_local_date, created_at, updated_at
FROM schedules
WHERE id = $1::uuid`

const listSchedulesByCircleQuery = `
SELECT id::text, circle_id::text, created_by::text, current_version,
	stopped_from_local_date, created_at, updated_at
FROM schedules
WHERE circle_id = $1::uuid
ORDER BY created_at, id`

const bumpScheduleVersionQuery = `
UPDATE schedules
SET current_version = current_version + 1, updated_at = NOW()
WHERE id = $1::uuid AND current_version = $2`

const stopScheduleQuery = `
UPDATE schedules
SET stopped_from_local_date = $2, current_version = current_version + 1,
	updated_at = NOW()
WHERE id = $1::uuid AND current_version = $3`

const insertScheduleRevisionQuery = `
INSERT INTO schedule_revisions (
	schedule_id, version, effective_local_date, mode, title, anchor_local_date,
	start_local_time, end_local_time, duration_minutes, timezone,
	end_local_date, week_cadence, weekdays, interval_count, interval_unit
) VALUES ($1::uuid, $2, $3, $4, $5, $6, $7::time, $8::time, $9, $10,
	$11, $12, $13, $14, $15)`

const insertScheduleSelectedDateQuery = `
INSERT INTO schedule_selected_dates (schedule_id, revision, local_date)
VALUES ($1::uuid, $2, $3)`

const listScheduleRevisionsQuery = `
SELECT schedule_id::text, version, effective_local_date, mode, title,
	anchor_local_date, to_char(start_local_time, 'HH24:MI'),
	to_char(end_local_time, 'HH24:MI'), duration_minutes, timezone,
	end_local_date, week_cadence, weekdays, interval_count, interval_unit
FROM schedule_revisions
WHERE schedule_id = ANY($1::uuid[])
ORDER BY schedule_id, version`

const listScheduleSelectedDatesQuery = `
SELECT schedule_id::text, revision, local_date
FROM schedule_selected_dates
WHERE schedule_id = ANY($1::uuid[])
ORDER BY schedule_id, revision, local_date`

// supersedeUnstartedExceptionsQuery removes the individual edits that a later
// series change (or stop) replaces: exceptions whose occurrence — moved or not —
// currently lives at/after the boundary local date and has no started
// materialization. The boundary compares the effective date (replacement when
// the occurrence was moved, otherwise the original), so a move into the
// superseded range is removed and a move out of it is retained. Started
// history stays in planned_session_details and its exception rows.
const supersedeUnstartedExceptionsQuery = `
DELETE FROM schedule_occurrence_exceptions e
WHERE e.schedule_id = $1::uuid
	AND COALESCE(e.replacement_local_date, e.original_local_date) >= $2
	AND NOT EXISTS (
		SELECT 1
		FROM planned_session_details d
		JOIN sessions s ON s.id = d.session_id
		WHERE d.schedule_id = e.schedule_id
			AND d.original_local_date = e.original_local_date
			AND s.actual_start IS NOT NULL
	)`

const upsertOccurrenceExceptionQuery = `
INSERT INTO schedule_occurrence_exceptions (
	schedule_id, original_local_date, series_version, version,
	replacement_local_date, replacement_start_local_time,
	replacement_end_local_time, replacement_duration_minutes,
	replacement_title, cancelled_at, updated_by
) VALUES (
	$1::uuid, $2,
	(SELECT current_version FROM schedules WHERE id = $1::uuid), 1,
	$3, $4::time, $5::time, $6, $7, $8, $9::uuid
)
ON CONFLICT (schedule_id, original_local_date) DO UPDATE SET
	series_version = EXCLUDED.series_version,
	version = schedule_occurrence_exceptions.version + 1,
	replacement_local_date = EXCLUDED.replacement_local_date,
	replacement_start_local_time = EXCLUDED.replacement_start_local_time,
	replacement_end_local_time = EXCLUDED.replacement_end_local_time,
	replacement_duration_minutes = EXCLUDED.replacement_duration_minutes,
	replacement_title = EXCLUDED.replacement_title,
	cancelled_at = EXCLUDED.cancelled_at,
	updated_by = EXCLUDED.updated_by,
	updated_at = NOW()
RETURNING series_version, version, created_at, updated_at`

const listOccurrenceExceptionsQuery = `
SELECT schedule_id::text, original_local_date, series_version, version,
	replacement_local_date, to_char(replacement_start_local_time, 'HH24:MI'),
	to_char(replacement_end_local_time, 'HH24:MI'),
	replacement_duration_minutes, replacement_title, cancelled_at,
	updated_by::text, created_at, updated_at
FROM schedule_occurrence_exceptions
WHERE schedule_id = $1::uuid
ORDER BY original_local_date`

// getCircleNameQuery reads the caller-visible circle name for calendar
// projections; the caller has already proven current membership.
const getCircleNameQuery = `
SELECT name FROM circles WHERE id = $1::uuid`

// getOccurrenceSessionIDQuery returns the materialized F-005 session id of one
// logical occurrence, or no row while the occurrence is still virtual.
const getOccurrenceSessionIDQuery = `
SELECT d.session_id::text
FROM planned_session_details d
WHERE d.schedule_id = $1::uuid AND d.original_local_date = $2`
