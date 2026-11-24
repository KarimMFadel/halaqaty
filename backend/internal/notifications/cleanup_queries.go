package notifications

// deleteTerminalAttemptStateQuery removes a bounded batch of per-device
// attempt rows whose notification already reached a terminal delivery state.
// A terminal intent is never claimed again, so its attempt rows — including
// orphaned pending rows left behind by the expiry sweep — are dead dispatch
// state. The history row keeps the aggregate delivery_state and
// last_error_class for the retained 30-day window.
const deleteTerminalAttemptStateQuery = `
DELETE FROM notification_device_attempts
WHERE id IN (
    SELECT a.id
    FROM notification_device_attempts a
    JOIN notifications n ON n.id = a.notification_id
    WHERE n.delivery_state IN (
        'accepted_by_provider', 'suppressed_preference',
        'suppressed_quiet_hours', 'expired', 'exhausted'
    )
    ORDER BY a.id
    LIMIT $1
)
`

// deleteExpiredHistoryQuery removes a bounded batch of notification rows past
// the rolling 30-day history window; the notification_device_attempts FK
// cascade removes their remaining attempt rows in the same statement.
// Required manager-deletion intents (preference_override = TRUE) still in a
// pending delivery state are retained until terminal handling, even past the
// window (ADR-027 §6); once terminal, they are cleaned like any other row.
const deleteExpiredHistoryQuery = `
DELETE FROM notifications
WHERE id IN (
    SELECT id
    FROM notifications
    WHERE created_at < NOW() - INTERVAL '30 days'
      AND NOT (preference_override AND delivery_state IN ('pending_no_device', 'pending_attempt'))
    ORDER BY created_at, id
    LIMIT $1
)
`
