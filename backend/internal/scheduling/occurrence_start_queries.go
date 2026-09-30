package scheduling

const getOccurrenceExceptionForStartQuery = `
SELECT replacement_local_date,
       to_char(replacement_start_local_time, 'HH24:MI'),
       to_char(replacement_end_local_time, 'HH24:MI'),
       replacement_duration_minutes, replacement_title, cancelled_at
FROM schedule_occurrence_exceptions
WHERE schedule_id=$1::uuid AND original_local_date=$2
`

const getOccurrenceDetailForStartQuery = `
SELECT session_id::text, cancelled_at
FROM planned_session_details
WHERE schedule_id=$1::uuid AND original_local_date=$2
`

const insertOccurrenceSessionQuery = `
INSERT INTO sessions (circle_id, created_by, scheduled_at)
VALUES ($1::uuid, $2::uuid, $3)
RETURNING id::text
`

const insertOccurrenceDetailsQuery = `
INSERT INTO planned_session_details (
    session_id, schedule_id, original_local_date, title, start_local_time,
    end_local_time, duration_minutes, planned_end_at, planning_timezone, version
) VALUES ($1::uuid,$2::uuid,$3,$4,$5::time,$6::time,$7,$8,$9,1)
`
