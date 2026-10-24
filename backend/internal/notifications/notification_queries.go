package notifications

const listNotificationsQuery = `
SELECT id, type, source_ref_type, source_ref_id, title, body, created_at, is_read, read_at
FROM notifications
WHERE user_id = $1
  AND created_at >= NOW() - INTERVAL '30 days'
  AND ($2::timestamptz IS NULL OR (created_at, id) < ($2, $3::uuid))
ORDER BY created_at DESC, id DESC
LIMIT $4
`

const countUnreadNotificationsQuery = `
SELECT COUNT(*)
FROM notifications
WHERE user_id = $1
  AND NOT is_read
  AND created_at >= NOW() - INTERVAL '30 days'
`

const markNotificationReadQuery = `
UPDATE notifications
SET is_read = TRUE, read_at = NOW()
WHERE id = $1
  AND user_id = $2
  AND NOT is_read
`

const notificationExistsForUserQuery = `
SELECT EXISTS(
    SELECT 1
    FROM notifications
    WHERE id = $1
      AND user_id = $2
)
`

// markAllNotificationsReadQuery shares the 30-day window with
// countUnreadNotificationsQuery so the bulk update and the authoritative
// unread count never disagree about which rows count.
const markAllNotificationsReadQuery = `
UPDATE notifications
SET is_read = TRUE, read_at = NOW()
WHERE user_id = $1
  AND NOT is_read
  AND created_at >= NOW() - INTERVAL '30 days'
`
