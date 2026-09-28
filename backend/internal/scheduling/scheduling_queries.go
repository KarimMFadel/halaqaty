package scheduling

const getCircleAccessQuery = `
SELECT cm.role, c.is_archived
FROM circle_members cm
JOIN circles c ON c.id = cm.circle_id
JOIN users u ON u.id = cm.user_id
WHERE cm.circle_id = $1::uuid AND cm.user_id = $2::uuid AND u.deleted_at IS NULL
FOR UPDATE OF c, cm`

const getScheduleRequestReplayQuery = `
SELECT actor_id::text, idempotency_key, command_fingerprint, response_status, response_resource_id::text
FROM schedule_request_replays
WHERE actor_id = $1::uuid AND idempotency_key = $2`

const lockScheduleRequestReplayQuery = `
SELECT pg_advisory_xact_lock(hashtextextended($1::text || ':' || $2::text, 0))`

const insertScheduleRequestReplayQuery = `
INSERT INTO schedule_request_replays
  (actor_id, idempotency_key, command_fingerprint, response_status, response_resource_id)
VALUES ($1::uuid, $2, $3, $4, $5::uuid)
ON CONFLICT (actor_id, idempotency_key) DO NOTHING`
