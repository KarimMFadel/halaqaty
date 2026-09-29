package scheduling

const calendarViewerQuery = `
SELECT p.timezone FROM profiles p WHERE p.user_id=$1::uuid`

const calendarCirclesQuery = `
SELECT c.id::text, c.name FROM circle_members cm
JOIN circles c ON c.id=cm.circle_id
WHERE cm.user_id=$1::uuid ORDER BY c.id`

const calendarOneOffsQuery = `
SELECT s.id::text, s.circle_id::text, c.name, COALESCE(d.title,'Circle Session'),
 s.status, s.scheduled_at, d.planned_end_at, d.planning_timezone, d.cancelled_at, d.version
FROM sessions s JOIN planned_session_details d ON d.session_id=s.id AND d.schedule_id IS NULL
JOIN circles c ON c.id=s.circle_id
WHERE s.circle_id=ANY($1::uuid[]) AND s.scheduled_at<$3 AND d.planned_end_at>$2
ORDER BY s.scheduled_at,s.id`

const calendarExceptionsQuery = `
SELECT e.schedule_id::text,e.original_local_date,e.replacement_local_date,
 to_char(e.replacement_start_local_time,'HH24:MI'),
 to_char(e.replacement_end_local_time,'HH24:MI'),e.replacement_duration_minutes,
 e.replacement_title,e.cancelled_at
FROM schedule_occurrence_exceptions e
WHERE e.schedule_id=ANY($1::uuid[])
 AND (e.original_local_date BETWEEN $2 AND $3 OR e.replacement_local_date BETWEEN $2 AND $3)`

const calendarMaterializedQuery = `
SELECT d.schedule_id::text,d.original_local_date,s.id::text,s.circle_id::text,c.name,
 COALESCE(d.title,'Circle Session'),s.status,s.scheduled_at,d.planned_end_at,
 d.planning_timezone,d.cancelled_at
FROM planned_session_details d JOIN sessions s ON s.id=d.session_id
JOIN circles c ON c.id=s.circle_id
WHERE d.schedule_id=ANY($1::uuid[]) AND s.scheduled_at<$3 AND d.planned_end_at>$2`
