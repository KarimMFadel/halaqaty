# F-008 Research and Design Inputs

**Branch**: `008-notification-system`  
**Date**: 2026-10-17

## Existing Project Decisions and Patterns

### Persistence and source ownership

- PostgreSQL is the only authoritative persistent store in MVP. Schema changes require a new ADR and a paired golang-migrate migration; do not modify applied migrations. Sources: `.specify/memory/constitution.md` §§III, Technology Constraints; `AGENTS.md`.
- `ARCHITECTURE.md` already sketches `device_tokens` and `notifications` tables and lists the planned F-008 history/read/preferences REST paths. These are approved architectural starting points to reconcile in ADR-027, not implemented infrastructure. The current notification schema has no source-event idempotency or delivery state. Sources: `docs/engineering/architecture/ARCHITECTURE.md`, `device_tokens`, `notifications`, and `/notifications` sections; `backend/migrations/` currently ends at `000020`.
- Queue events have stable IDs and a domain-owned outbox with retry/parking semantics. F-008 must not claim or mutate queue-owned delivery rows. Sources: `specs/003-recitation-queue-system/spec.md`; `backend/internal/queue/outbox.go`; `backend/migrations/000017_recitation_queue_system.up.sql`; `docs/contracts/ws_events.md`.
- Chat messages have durable IDs and F-004 owns `chat_event_outbox` and foreground WebSocket delivery. ADR-021 explicitly excludes F-004 FCM triggers and assigns background notification delivery to F-008. F-008 must consume a narrow approved source handoff without duplicating chat content or taking over the chat outbox. Sources: `docs/engineering/architecture/adr/ADR-021-chat-persistence-media-and-delivery.md`; `backend/internal/chat/outbox.go`; `backend/migrations/000018_real_time_chat.up.sql`.
- F-005 owns lifecycle/presence and emits metadata-only `session.started`/`session.ended`; no media credentials, endpoint, room reference, or provider data may enter notification projections. Source: `docs/contracts/ws_events.md`; `docs/engineering/architecture/adr/ADR-016-session-realtime-and-presence-foundation.md`; F-008 FR-019.
- F-006 owns current occurrence/revision eligibility. Reminder delivery must consult that boundary and reject cancelled/superseded occurrences. Source: `specs/006-schedule-calendar-attendance/` and ADR-025/ADR-026.
- `realtime.Hub.SendToUsers` already supports authenticated user-targeted foreground events with a just-in-time authorization callback; use this instead of adding notification topics. Source: `backend/internal/realtime/hub.go`.
- ADR-024 leaves manager deletion blocked until F-008 can atomically archive circles and enqueue member notices after designated-supervisor validation. FCM acceptance or handset receipt is not a safe transaction gate. Source: `docs/engineering/architecture/adr/ADR-024-account-deletion-tombstone-and-firebase-cleanup.md`.

### Firebase and device-token implementation gap

- The Go Firebase Admin dependency is already in `backend/go.mod`; no additional Go FCM SDK dependency is needed.
- `docs/contracts/openapi.yaml` documents `POST /auth/fcm-token`, and `ARCHITECTURE.md` specifies the intended `device_tokens` table, but the current backend has no token handler, token table, or migration. F-008 planning must close this existing F-001 auth/device-registration boundary gap; it must not assume token persistence already works.
- `ARCHITECTURE.md` already lists `firebase_messaging` in its Flutter dependency matrix, but `mobile/pubspec.yaml`/`pubspec.lock` do not include it yet. The architectural direction exists; ADR-027 must formally approve the dependency and platform setup before implementation.
- Firebase's official Flutter guide distinguishes foreground/background/terminated handling, requires a top-level background handler for Apple/Android, and uses `getInitialMessage` / `onMessageOpenedApp` for notification-open routing: [Receive messages in Flutter apps](https://firebase.google.com/docs/cloud-messaging/flutter/receive-messages).
- Firebase's official guidance says invalid/unregistered registration tokens should be removed from server storage; this cleanup must use the established auth/token owner boundary: [Manage FCM registration tokens](https://firebase.google.com/docs/cloud-messaging/manage-tokens).
- FCM acceptance is not handset receipt or user read. F-008 requirements already prohibit claiming either; Firebase delivery reporting also distinguishes acceptance and device SDK delivery: [Understand message delivery](https://firebase.google.com/docs/cloud-messaging/understand-delivery).
- Microsoft Teams documentation presents Activity feed as a 30-day timeline with an Unread filter and separately describes notification settings as controls over when/where alerts appear. This suggests (but does not explicitly prove) that muted alerts can remain reviewable/unread in the feed; Karim's accepted history/unread policy remains the authority: [Activity feed](https://support.microsoft.com/en-us/teams/notifications-settings/explore-the-activity-feed-in-microsoft-teams), [Notification settings](https://support.microsoft.com/en-us/teams/notifications-settings/manage-notifications-in-microsoft-teams).

## Planning Decisions Already Grounded by Karim's Clarifications

- History/unread records are recipient-scoped and independent of push delivery.
- Circle join/archive, session start/end preferences are global per-user event types; configurable types default ON.
- Quiet hours default to 10pm–7am; suppressed push is dropped for that occurrence and does not affect history/unread.
- Manager-deletion notices bypass the ordinary circle-archive preference, remain subject to quiet hours, and must be durably handed off atomically with archival/deletion.
- F-006 remains the source of reminder eligibility; stale/cancelled/superseded reminders are suppressed.

## Agent Recommendations for Plan-Level Resolution

- Architect and Flutter recommend checking the current global event-type preference immediately before first push dispatch, so an unsent queued push is suppressed after the user turns its type off; the history item remains.
- Karim accepted Architect's recommendation to use the existing profile IANA timezone as the account-wide quiet-hours clock; travel quiet hours follow that stored profile timezone.
- Karim accepted the Flutter/Architect recommendation to check the current global event-type preference before first push dispatch; an OFF setting suppresses an unsent push while history remains.
- Flutter and Architect agree that a configurable type's OFF state mutes F-008 push and foreground alert presentation while preserving history and leaving source content delivery intact. They differed on unread counting; Karim chose Flutter's recommendation: preference-muted items remain unread. Teams documentation was used only as contextual UX reference, not as authority for this decision.
- Architect and Tech Lead agree that manager deletion is blocked when required durable history/dispatch intents cannot be committed, but a transient FCM outage after that atomic commit does not roll back the archive/account closure. The spec's Story 5 wording has been updated to express this distinction.
- Architect and Go recommend narrow source-specific durable handoffs, no generic cross-feature event bus, no duplicate source state, and recipient/event idempotency. Exact handoff interfaces, retry/expiry, and health signals belong in ADR-027/contracts.
- Senior Go and Architect initially recommended terminal suppression for zero registered tokens. Karim chose to hold the intent with bounded expiry and recheck: 24 hours for ordinary notices, 30 days for manager-deletion notices, with earlier source cutoffs. Architect recommends expiring each F-006 reminder when the next closer configured offset becomes due and the final offset at occurrence start; Karim accepted this rule. Recheck current preference, quiet hours, and source eligibility before dispatch.
- OQ-036/ADR-010 approves role-bound invitations, while current F-002 contract/spec artifacts focus on reusable invite-code/link flows and do not describe a notification handoff for a particular recipient; the distinct “student joined circle” event is already specified. Karim accepted the trigger: notify the named recipient when an authorized inviter sends a role-bound invitation. The history/delivery and source integration remain F-008 work under F-002 authorization rules; do not infer a recipient event from code sharing or join.

## Alternatives Rejected or Deferred

- A new generic event bus/broker: rejected for MVP scope; source-owned transitions/outboxes remain authoritative.
- Reusing queue/chat outbox rows as F-008 delivery state: rejected because those outboxes have distinct owners, semantics, and consumers.
- Treating FCM send acceptance or notification open as proof of delivery/read: prohibited by the accepted specification.
- Cross-platform badge behavior: Flutter, Architect, and Tech Lead agree on an exact server/in-app unread count with native app-icon synchronization where the OS/launcher supports it and badge display is enabled. Android launcher numeric behavior and user settings are not universally controllable; no app dependency is justified solely to promise unsupported badge behavior. The plan adopts this aligned recommendation.
