package notifications

// expireDueIntentsQuery terminally expires a bounded batch of pending intents
// whose push deadline passed; history and unread state are untouched.
const expireDueIntentsQuery = `
UPDATE notifications
SET delivery_state = 'expired'
WHERE id IN (
    SELECT id FROM notifications
    WHERE delivery_state IN ('pending_no_device', 'pending_attempt')
      AND push_expires_at <= $1
    ORDER BY push_expires_at
    LIMIT $2
)
`

// claimDueIntentsQuery leases due dispatch intents: the claim updates the
// selected rows' next_attempt_at to the lease deadline ($3) inside the claim
// transaction, so a claimed row is not re-claimable until the lease expires.
// FOR UPDATE SKIP LOCKED lets concurrent dispatchers claim disjoint rows of
// the same batch; lease expiry provides crash recovery for a dispatcher that
// claimed rows and died before settling them.
const claimDueIntentsQuery = `
WITH due AS (
    SELECT id
    FROM notifications
    WHERE delivery_state = 'pending_attempt'
      AND next_attempt_at <= $1
      AND push_expires_at > $1
    ORDER BY next_attempt_at, created_at
    LIMIT $2
    FOR UPDATE SKIP LOCKED
), leased AS (
    UPDATE notifications n
    SET next_attempt_at = $3
    FROM due
    WHERE n.id = due.id
    RETURNING n.id, n.user_id, n.type, n.source_ref_type, n.source_ref_id,
              n.title, n.body, n.push_expires_at, n.preference_override
)
SELECT id, user_id, type, source_ref_type, source_ref_id, title, body,
       push_expires_at, preference_override
FROM leased
ORDER BY id
`

const hasDeviceAttemptsQuery = `
SELECT EXISTS(
    SELECT 1 FROM notification_device_attempts WHERE notification_id = $1
)
`

// listDueDeviceTargetsQuery returns registered devices whose per-device
// attempt is due now: no attempt row yet, or a pending row whose backoff
// elapsed. Terminal per-device states are never returned.
//
// Deliberate read/write asymmetry: device_tokens is owned by F-001 auth.
// Reads of token rows for dispatch targeting are local to this package; all
// mutations (invalid-token deletion) go through the DeviceTokenRemover seam
// that auth.SessionRepository satisfies — this package never writes
// device_tokens directly.
const listDueDeviceTargetsQuery = `
SELECT dt.id, dt.token, COALESCE(a.attempt_count, 0)
FROM device_tokens dt
LEFT JOIN notification_device_attempts a
       ON a.device_token_id = dt.id AND a.notification_id = $1
WHERE dt.user_id = $2
  AND (a.id IS NULL OR (a.state = 'pending' AND a.next_attempt_at <= $3))
ORDER BY dt.created_at, dt.id
`

// suppressNotificationQuery terminally suppresses one occurrence; the guard
// keeps already-terminal states (never replay a settled occurrence).
const suppressNotificationQuery = `
UPDATE notifications
SET delivery_state = $2
WHERE id = $1 AND delivery_state IN ('pending_no_device', 'pending_attempt')
`

// suppressDeviceAttemptsQuery records the per-device suppression outcome for
// every currently registered device of the recipient. Like the targeting
// query above it only reads the F-001-owned device_tokens rows; token
// mutations stay behind the auth-owned DeviceTokenRemover seam.
const suppressDeviceAttemptsQuery = `
INSERT INTO notification_device_attempts (notification_id, device_token_id, state)
SELECT $1, id, $2 FROM device_tokens WHERE user_id = $3
ON CONFLICT (notification_id, device_token_id) DO NOTHING
`

// recordAcceptedAttemptQuery marks one device attempt provider-accepted. It
// never implies handset receipt or read state.
const recordAcceptedAttemptQuery = `
INSERT INTO notification_device_attempts (
    notification_id, device_token_id, state, attempt_count, next_attempt_at
)
VALUES ($1, $2, 'accepted_by_provider', 1, NOW())
ON CONFLICT (notification_id, device_token_id) DO UPDATE
SET state = 'accepted_by_provider',
    attempt_count = notification_device_attempts.attempt_count + 1,
    last_error_class = NULL,
    updated_at = NOW()
`

// recordRetryAttemptQuery reschedules one failed attempt ($5 = 'pending') or
// parks it at the five-attempt ceiling ($5 = 'exhausted').
const recordRetryAttemptQuery = `
INSERT INTO notification_device_attempts (
    notification_id, device_token_id, state, attempt_count, next_attempt_at, last_error_class
)
VALUES ($1, $2, $5, 1, $3, $4)
ON CONFLICT (notification_id, device_token_id) DO UPDATE
SET state = $5,
    attempt_count = notification_device_attempts.attempt_count + 1,
    next_attempt_at = $3,
    last_error_class = $4,
    updated_at = NOW()
`

// settleNotificationQuery recomputes the aggregate delivery state from the
// per-device rows: any pending row keeps the intent pending; otherwise any
// accepted device settles provider acceptance; no remaining rows means every
// token was deleted as invalid and the intent waits for re-registration;
// anything left is parked exhaustion.
const settleNotificationQuery = `
UPDATE notifications n
SET delivery_state = CASE
        WHEN NOT EXISTS (
            SELECT 1 FROM notification_device_attempts a
            WHERE a.notification_id = n.id
        ) THEN 'pending_no_device'
        WHEN EXISTS (
            SELECT 1 FROM notification_device_attempts a
            WHERE a.notification_id = n.id AND a.state = 'pending'
        ) THEN 'pending_attempt'
        WHEN EXISTS (
            SELECT 1 FROM notification_device_attempts a
            WHERE a.notification_id = n.id AND a.state = 'accepted_by_provider'
        ) THEN 'accepted_by_provider'
        ELSE 'exhausted'
    END,
    next_attempt_at = COALESCE(
        (SELECT MIN(a.next_attempt_at) FROM notification_device_attempts a
         WHERE a.notification_id = n.id AND a.state = 'pending'),
        n.next_attempt_at),
    attempt_count = COALESCE(
        (SELECT MAX(a.attempt_count) FROM notification_device_attempts a
         WHERE a.notification_id = n.id),
        n.attempt_count),
    last_error_class = COALESCE(
        (SELECT a.last_error_class FROM notification_device_attempts a
         WHERE a.notification_id = n.id AND a.last_error_class IS NOT NULL
         ORDER BY a.updated_at DESC, a.id LIMIT 1),
        n.last_error_class)
WHERE n.id = $1
  AND n.delivery_state IN ('pending_no_device', 'pending_attempt')
RETURNING n.delivery_state
`

const getDeliveryStateQuery = `
SELECT delivery_state FROM notifications WHERE id = $1
`

const getTypePreferenceEnabledQuery = `
SELECT enabled
FROM notification_preferences
WHERE user_id = $1 AND notification_type = $2
`
