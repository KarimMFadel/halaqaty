package notifications

const getNotificationPreferencesQuery = `
SELECT COALESCE(p.timezone, 'UTC'),
       COALESCE(q.enabled, TRUE),
       COALESCE(to_char(q.quiet_hours_start, 'HH24:MI'), '22:00'),
       COALESCE(to_char(q.quiet_hours_end, 'HH24:MI'), '07:00')
FROM users u
LEFT JOIN profiles p ON p.user_id = u.id
LEFT JOIN notification_preferences q
       ON q.user_id = u.id AND q.notification_type IS NULL
WHERE u.id = $1 AND u.deleted_at IS NULL
`

const listNotificationTypePreferencesQuery = `
SELECT notification_type, enabled
FROM notification_preferences
WHERE user_id = $1 AND notification_type IS NOT NULL
`

const upsertNotificationTypePreferenceQuery = `
INSERT INTO notification_preferences (user_id, notification_type, enabled)
VALUES ($1, $2, $3)
ON CONFLICT (user_id, notification_type) WHERE notification_type IS NOT NULL
DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = NOW()
`

const upsertNotificationQuietHoursQuery = `
INSERT INTO notification_preferences (
    user_id, notification_type, enabled, quiet_hours_start, quiet_hours_end
)
VALUES ($1, NULL, $2, $3::time, $4::time)
ON CONFLICT (user_id) WHERE notification_type IS NULL
DO UPDATE SET enabled = EXCLUDED.enabled,
              quiet_hours_start = EXCLUDED.quiet_hours_start,
              quiet_hours_end = EXCLUDED.quiet_hours_end,
              updated_at = NOW()
`
