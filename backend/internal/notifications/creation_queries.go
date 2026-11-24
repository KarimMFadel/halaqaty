package notifications

// insertNotificationQuery seeds the durable delivery-state: intents whose
// push deadline already passed are born expired, and per the approved
// data-model semantics only an intent with NO registered token holds in
// pending_no_device until registration — an intent created while a token is
// registered (the everyday case) is immediately dispatchable. Like the
// dispatch targeting queries, this only READS the F-001-owned device_tokens
// table; token mutations stay behind the DeviceTokenRemover seam.
const insertNotificationQuery = `
INSERT INTO notifications (
    user_id, type, source_event_id, source_ref_type, source_ref_id,
    title, body, created_at, push_expires_at, preference_override, delivery_state
)
VALUES (
    $1, $2, $3, $4, $5, $6, $7, COALESCE($8::timestamptz, NOW()), $9, $10,
    CASE WHEN $9 <= NOW() THEN 'expired'
         WHEN EXISTS (SELECT 1 FROM device_tokens dt WHERE dt.user_id = $1) THEN 'pending_attempt'
         ELSE 'pending_no_device' END
)
ON CONFLICT (user_id, type, source_event_id) DO NOTHING
RETURNING id
`

const findNotificationIDBySourceQuery = `
SELECT id
FROM notifications
WHERE user_id = $1 AND type = $2 AND source_event_id = $3
`
