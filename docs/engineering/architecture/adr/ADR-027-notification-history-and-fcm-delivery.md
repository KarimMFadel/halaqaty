# ADR-027: F-008 Notification History and FCM Delivery

**Status:** Accepted

**Date:** 2026-10-17  
**Decider:** Karim (product policy); Architect, Senior Go, Senior Flutter, Tech Lead (technical recommendations)

## Context

F-008 consolidates user-visible notification history, preferences, quiet hours, unread state, and FCM projections for already-approved source events. PostgreSQL is the sole persistent authority. Existing architecture sketches `device_tokens`, `notifications`, and notification REST paths, but the token table/handler and notification implementation do not exist. F-003 owns queue state and its outbox; F-004 owns durable chat and foreground realtime; F-005 owns session lifecycle; F-006 owns reminder eligibility; F-007 owns grades; F-002 owns circle/invitation eligibility. F-001 owns identity and account/token lifecycle.

F-008 must also enable the already-approved manager-deletion path in ADR-024: designated-supervisor validation and circle archival plus durable member notices must commit atomically. A provider outage after commit cannot safely reverse closure.

## Decision

1. **Persistence and idempotency.** Use PostgreSQL `notifications` as recipient history, read state, and durable dispatch intent. A unique recipient/type/source-event key makes source replays idempotent. Retain only an allowlisted event type, generic display projection, stable source reference, timestamps, read state, and dispatch state; load sensitive source details only after normal source authorization.
2. **Device delivery.** Complete device-token registration under the existing F-001 authenticated boundary for Android, iOS, and web clients. Keep only the minimum token/platform/lifecycle fields. Track independent per-device attempts referencing token rows, so one invalid device or partial FCM failure does not hide another device's outcome. Delete invalid/unregistered tokens and all account tokens on closure. Never log or return raw tokens.
3. **Source ownership.** Use narrow source-owned transaction handoffs for approved events. Do not add a generic broker and do not consume or mutate F-003/F-004 outbox tables. F-003 transitions hand off stable queue event IDs. F-004 message commit exposes only an approved notification projection while preserving its outbox and foreground chat. F-005 lifecycle emits metadata-only start/end projections. F-006 eligibility is checked at creation and again immediately before reminder dispatch. F-002 owns membership, archive, and invitation eligibility. Per OQ-066, F-008 creates the named recipient's invitation notice when an authorized inviter's role-bound invitation commits; reusable invite-code sharing and circle joining are not invitation-notification triggers. Grade visibility remains F-007-owned.
4. **Delivery semantics.** Persist history and intent before delivery. Use the existing bounded retry/parking pattern (maximum five attempts, exponential backoff with jitter, operational metric/log on parking); FCM acceptance means provider acceptance only, not handset receipt. With no token, retain intent pending registration: ordinary events expire after 24 hours from source event, and required manager-deletion pushes expire after 30 days, shortened by earlier source-validity cutoffs. Recheck current preference, quiet hours, and source eligibility at registration and immediately before dispatch. F-006 reminder offsets expire when the next closer configured offset becomes due; the last expires at occurrence start. Preference-off and quiet-hour suppression are terminal for that occurrence; re-enabling later does not replay it. Quiet hours use profile IANA timezone and default 22:00–07:00.
5. **Read and alert behavior.** A configurable type OFF mutes F-008 push and foreground alert presentation, but history/unread remains; source content (including F-004 messages) is unaffected. Opening an item marks it read; opening the list does not. Read-all and count updates are server-authoritative. Native icon badge is synchronized to the exact count where OS and launcher capabilities and user settings support it; otherwise exact in-app count remains authoritative. Karim accepted this platform-supported scope after review.
6. **Retention.** Expose only the rolling 30-day history window. Cleanup removes expired history and terminal attempt state in bounded batches. Pending required manager-deletion dispatch intents are retained until terminal handling; user-facing history still follows the approved 30-day window. FCM delivery state is never read state.
7. **Manager deletion.** For every owned active circle, require the designated supervisor and atomically archive, close the account, and insert one mandatory history/dispatch intent per eligible member in the same PostgreSQL transaction. This event bypasses the member's ordinary circle-archive preference, but remains subject to quiet hours. Any required database write failure aborts the transaction. After commit, transient FCM failure is retried and cannot roll back closure. Keep manager deletion disabled until migration, dispatcher, monitoring/operations, and Karim's mandatory security review are complete.
8. **Contracts and mobile.** Use the existing planned REST paths. Add an authenticated user-targeted WebSocket notification update/invalidation via `realtime.Hub.SendToUsers`; clients reconcile REST state on reconnect/resume/account switch. Add Flutter `firebase_messaging` and native setup only as specified here; handle foreground/background/terminated opens with generic payloads and authorized deep-link resolution.

## Consequences

- Migration 000021 adds notification/preference/device-token storage and necessary constraints/indexes; a paired down migration is required and must not destructively remove live state without explicit safeguards.
- OpenAPI and WebSocket catalog changes precede implementation. Source-specific interfaces must not silently widen source ownership.
- The dispatcher needs bounded batches, retries, parking telemetry, 30-day cleanup, token invalidation, and a documented operational readiness check before deletion is enabled.
- F-008 must not claim successful delivery or read based on FCM provider responses.
- Karim accepted best-effort native badge synchronization where platform and user settings permit it; exact in-app count remains required.
- No-token intents wait for registration under the approved 24-hour/30-day bounds and source cutoffs; history/read-state and manager-deletion transaction do not depend on handset receipt.

## Alternatives considered

| Alternative | Decision |
|---|---|
| New generic event bus/broker | Rejected; adds infrastructure and duplicates source ownership at MVP scale. |
| Reuse F-003 or F-004 outbox as F-008 state | Rejected; different owner, retention, and delivery semantics. |
| One push row per device | Rejected; history is per recipient/event; device attempts are subordinate delivery records. |
| FCM callback as read/delivery proof | Rejected; provider acceptance does not prove device receipt or user action. |
| Delete manager account only after every device accepts FCM | Rejected; external provider state cannot participate in PostgreSQL transaction and acceptance is not receipt. |

## References

- [F-008 plan](../../../../specs/008-notification-system/plan.md), [data model](../../../../specs/008-notification-system/data-model.md), [contracts](../../../../specs/008-notification-system/contracts/service.md)
- [ADR-021: Chat persistence, media, and delivery](ADR-021-chat-persistence-media-and-delivery.md)
- [ADR-024: Account closure tombstone and Firebase cleanup](ADR-024-account-deletion-tombstone-and-firebase-cleanup.md)
- [ADR-025/026: F-006 scheduling policy and occurrences](ADR-025-schedule-policy-and-session-lifecycle-boundary.md), [persistence](ADR-026-schedule-occurrence-and-attendance-persistence.md)
- [Firebase Flutter message handling](https://firebase.google.com/docs/cloud-messaging/flutter/receive-messages)
- [Firebase registration token management](https://firebase.google.com/docs/cloud-messaging/manage-tokens)
