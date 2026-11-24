package scheduling

// reminder_projection_queries.go holds the SQL of the F-008 reminder
// projection path: schedule enumeration, cancelled-exception narrowing and
// recipient fan-out. Per repo convention SQL never lives inside methods.

// listSchedulesForRemindersQuery reads every schedule: the projection work is
// per-occurrence (stop boundary, cancellation, no-longer-generated dates are
// all evaluated per date), so statements cannot prune schedules without
// duplicating F-006 engine rules in SQL.
// ponytail: loads all schedules each tick — fine at MVP scale (~50 concurrent
// users); move to a start-time-bounded statement if schedule counts grow.
const listSchedulesForRemindersQuery = `
SELECT id::text, circle_id::text, created_by::text, current_version,
       stopped_from_local_date, created_at, updated_at
FROM schedules
ORDER BY created_at, id
`

// listCancelledReminderExceptionsQuery narrows cancellation state to the
// candidate date window so each tick touches at most the rows it may exclude.
const listCancelledReminderExceptionsQuery = `
SELECT schedule_id::text, original_local_date
FROM schedule_occurrence_exceptions
WHERE schedule_id = ANY($1::uuid[])
  AND original_local_date >= $2
  AND original_local_date <= $3
  AND cancelled_at IS NOT NULL
`

// reminderCancellationQuery reads one occurrence's cancellation timestamp for
// the dispatch-time recheck; absent row means no exception was ever stored.
const reminderCancellationQuery = `
SELECT cancelled_at
FROM schedule_occurrence_exceptions
WHERE schedule_id = $1::uuid
  AND original_local_date = $2
`

// listReminderRecipientsQuery fans the reminder out to the circle's active
// members — the same recipient gate the started-session projection uses.
const listReminderRecipientsQuery = `
SELECT cm.user_id
FROM circle_members cm
JOIN users u ON u.id = cm.user_id
WHERE cm.circle_id = $1::uuid
  AND u.deleted_at IS NULL
ORDER BY cm.user_id
`
