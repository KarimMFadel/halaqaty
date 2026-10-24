package notifications

const insertNotificationQuery = `
INSERT INTO notifications (
    user_id, type, source_event_id, source_ref_type, source_ref_id,
    title, body, created_at, push_expires_at, preference_override, delivery_state
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, COALESCE($8::timestamptz, NOW()), $9, $10,
    CASE WHEN $9 <= NOW() THEN 'expired' ELSE 'pending_no_device' END
)
ON CONFLICT (user_id, type, source_event_id) DO NOTHING
RETURNING id
`

const findNotificationIDBySourceQuery = `
SELECT id
FROM notifications
WHERE user_id = $1 AND type = $2 AND source_event_id = $3
`
