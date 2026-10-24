# F-008 Data Model

This is the proposed additive design for ADR-027. PostgreSQL is the authority; FCM is an external transport only. Names below are logical and finalized by the paired migration after review.

## Notification

One row represents one approved event for one recipient, regardless of the number of delivery channels.

- `id`: UUID primary key.
- `user_id`: recipient FK to `users.id`; access is always scoped to the authenticated recipient.
- `type`: stable allowlisted event type from the feature-local event catalog.
- `source_event_id`: stable source-owned event identifier; never a copied source payload.
- `source_ref`: typed opaque reference sufficient to navigate to authorized source details.
- `push_expires_at`: UTC deadline from source event time, capped at 24 hours for ordinary events or 30 days for mandatory manager-deletion notices, shortened by any earlier source-specific cutoff.
- `created_at`: UTC timestamp used for 30-day history ordering/visibility.
- `is_read`, `read_at`: read state. A preference-muted notice remains unread until an explicit read action.
- `delivery_state`: pending-no-device, pending-attempt, accepted-by-provider, suppressed-preference, suppressed-quiet-hours, expired, or exhausted; describes push state only and never claims handset receipt.
- `next_attempt_at`, `attempt_count`, `last_error_class`: bounded retry scheduling and operational diagnosis without sensitive provider payloads.
- `preference_override`: true only for the required manager-deletion notice, which bypasses ordinary circle-archive preference.
- Unique `(user_id, type, source_event_id)` constraint prevents duplicate history/intents on replay.

Message text and grades are not duplicated. The stored display projection uses generic lock-screen-safe text and typed navigation metadata. Authorized details load from their source after opening.

The allowlisted types follow the active notification matrix: `session_reminder`, `group_message`, `private_message`, `grade_posted`, `circle_invitation`, `queue_next_soon`, `queue_your_turn`, `student_joined_session`, `student_joined_circle`, `circle_archived`, `session_started`, and `session_ended`. `circle_invitation` is created when an authorized inviter sends a role-bound invitation to its named recipient.

## DeviceToken

Owned by authenticated account/device registration boundary.

- `id`: UUID; `user_id`: account FK; `token`: secret registration value; `platform`: allowlisted Android/iOS/web; optional non-identifying device label; created/last-seen timestamps.
- Unique `(user_id, token)`. Token is never returned in notification history and is redacted from logs.
- Invalid/unregistered FCM tokens are deleted through the same owner boundary. Authenticated clients can detach one token before logout/account switch; account closure removes all tokens.

## NotificationPreference

One user-level setting per configurable type, plus one quiet-hours configuration.

- `(user_id, type)` unique; `enabled` defaults true for configurable types.
- Always-on types are not represented as mutable false values and API validation rejects attempts to disable them.
- Quiet-hours start/end are local wall-clock times with default 22:00–07:00; interpret with the user's current stored profile IANA timezone at evaluation time. Reject invalid/equal interval values according to contract; overnight interval is supported.
- Missing preference rows resolve to the matrix default ON; avoid pre-seeding every user/type.

## NotificationDeviceAttempt

One row tracks one notification on one registered device, allowing independent outcomes for users with multiple tokens. It references the notification and `device_tokens.id`; it never duplicates the raw token. It stores provider state (pending, accepted, suppressed, exhausted), attempt count, next-attempt timestamp, and a safe error class. Attempts are bounded and use the existing retry/parking pattern.

## State and retention

- Creation transaction writes the history row and pending intent atomically with the source transition (or its existing durable outbox handoff). With no token, state remains pending-no-device until registration or expiry; this does not change history/unread or manager-deletion commit.
- On the first dispatch check, current preference and quiet hours determine whether the attempt is suppressed. A suppressed occurrence is terminal and is not replayed if settings later change.
- Transient send errors retry with bounded exponential backoff and a terminal exhausted state plus metric/log signal; accepted FCM sends terminate delivery attempts but do not set read state.
- Pending ordinary intents expire no later than 24 hours after source event; required manager-deletion push expires no later than 30 days. Earlier queue/session source cutoffs apply; F-006 reminder offsets expire when the next closer configured offset becomes due, and the final offset expires at occurrence start.
- History API includes only rows with `created_at` inside the rolling 30-day window. A periodic bounded cleanup deletes expired notification rows and related attempt state. Required deletion intents cannot be erased while pending; clean them only after terminal delivery state, preserving no more than the approved 30-day history requirement once delivery has settled.
- Unread count is derived from the recipient's retained unread rows, not FCM callbacks. Read-all and individual-read operations are transactional/idempotent.

## Authorization and privacy

- Every query filters by authenticated `user_id`; source eligibility is checked before creation and rechecked for volatile dispatch eligibility.
- `source_ref` is not authorization. Deep-link resolution performs the source feature's normal permission check.
- Private message bodies, grade details, credentials, media endpoints, room references, and raw FCM tokens are excluded from push and diagnostic logs.
