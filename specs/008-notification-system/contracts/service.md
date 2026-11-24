# F-008 Service and REST Contract

Canonical public contract changes are applied to `docs/contracts/openapi.yaml` during implementation after this plan/ADR is reviewed. These paths align with the existing architecture sketch. All endpoints require the existing Firebase-authenticated backend identity and enforce recipient scoping.

## History and read state

- `GET /notifications`: cursor-paginated newest-first history, limited to the current 30-day window; `limit` defaults to 20 and is capped at 50. Response includes `items`, `next_cursor`, and authoritative `unread_count`. Each item exposes `id`, allowlisted `type`, generic `title`/`body`, typed `source_ref`, `created_at`, and `is_read`. It never returns device tokens or private source payloads.
- `PUT /notifications/{id}/read`: marks only the authenticated recipient's item read; idempotent. Return updated unread count. Unknown or another user's IDs do not disclose existence.
- `PUT /notifications/read-all`: marks current retained unread items read atomically and returns updated unread count. Idempotent.

## Preferences

- `GET /notifications/preferences`: returns global per-type values, mutability/default metadata, and quiet-hours configuration.
- `PUT /notifications/preferences`: updates only allowlisted configurable types and quiet-hours values. Reject disabling always-on types, unknown types, malformed wall-clock values, or unsupported timezone override fields. Quiet-hours timezone is read from the existing profile; this API does not duplicate it.

## Token lifecycle

- `POST /auth/fcm-token` registers or refreshes an Android, iOS, or web token under the authenticated F-001 boundary. `DELETE /auth/fcm-token` accepts the raw token in its JSON body and removes only that token for the authenticated account; clients call it before logout/account switch for immediate server detachment. Neither operation returns or logs the raw token.
- Token values are write-only and never returned. Invalid provider tokens are removed server-side. Account closure deletes all associated tokens.
- When a token registers, the dispatcher reconsiders pending intents that have not expired and whose source remains eligible; it does not replay terminally suppressed or completed intents to later devices.

## Realtime contract

See [realtime.md](realtime.md) for the exact `notification.updated` envelope and payload. Use the existing `realtime.Hub.SendToUsers` path and authorization callback. Clients reconcile via REST after reconnect, missed event, resume, or account switch.

## FCM projection

FCM data contains a stable notification ID and typed source reference only; generic notification display text is safe on a lock screen. No source message body, grade detail, media data, authorization token, or arbitrary URL. App open handling uses the existing route authorization process and marks the selected notice read. Foreground F-008 alert presentation is muted by OFF preference; underlying F-004 chat delivery remains unchanged.

## Error and delivery semantics

- REST errors use the standard API envelope and status conventions.
- Source handoff and history insertion are idempotent on `(recipient, type, source event)`.
- FCM provider acceptance is recorded as accepted by FCM, not delivered/read.
- Quiet hours and preference suppression are terminal for that occurrence. Transient provider failures retry under ADR-027 bounded retry policy.
- If no token is registered, retain the push intent pending registration. Ordinary notices expire no later than 24 hours after source event; mandatory manager-deletion notices expire no later than 30 days, shortened by earlier source cutoffs. Recheck preference, quiet hours, and current source eligibility at registration and dispatch. F-006 reminder offsets expire when the next closer configured offset becomes due, with the final offset expiring at occurrence start; history/unread remains independent.
- A manager-deletion database transaction fails closed if the designated supervisor or required durable history/intent write is unavailable. Post-commit FCM failure does not reverse account/circle mutation.
