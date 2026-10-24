# Implementation Plan: F-008 Notification System

**Branch**: `008-notification-system` | **Date**: 2026-10-17 | **Spec**: [spec.md](spec.md)

## Summary

Implement one recipient-scoped notification history record per approved source event and recipient, with durable idempotent push work, global event preferences, quiet-hours gating, read state, and an authoritative unread count. PostgreSQL remains authoritative. Existing feature transitions and eligibility remain owned by F-001–F-007; F-008 adds narrow durable handoffs and uses FCM only for background/closed-app projections. Manager deletion must atomically persist required member history and delivery intents with archive and closure.

## Technical Context

**Language/Version**: Go 1.26; Dart/Flutter 3.47.4 (repo currently targets CI Flutter 3.44.0)  
**Primary Dependencies**: pgx, existing Firebase Admin Go SDK, Firebase Auth client; add `firebase_messaging` per ADR-027 and lock compatible platform versions  
**Storage**: PostgreSQL only; paired golang-migrate migrations  
**Testing**: Go unit, contract, integration, migration-up/down; Flutter unit/widget and platform/integration flows  
**Target Platform**: Go modular monolith; Android, iOS, and web FCM clients  
**Project Type**: Existing backend + mobile monorepo  
**Performance Goals**: Match MVP scale in constitution (50 concurrent users, ≤10 simultaneous live sessions); page history and bound dispatcher batches  
**Constraints**: FCM acceptance is not receipt; source eligibility rechecked; UTC API timestamps; no message-body/grade/private content in lock-screen payloads; 30-day display retention; user profile IANA timezone governs quiet hours; deletion requires transactional durable intent  
**Scale/Scope**: Existing Halaqaty MVP; no new broker, Redis, or standalone service

## Constitution Check

- Spec-first: approved F-008 scope exists and all seven lifecycle phases are being followed. PASS.
- PostgreSQL-only persistent state, Go/Flutter stack, FCM: preserved. PASS.
- New library/platform integration requires ADR: ADR-027 authorizes Firebase Messaging dependency/platform setup. PASS after ADR approval within this plan.
- AuthZ from source membership and authenticated user identity; payload privacy and no media secrets: preserved. PASS.
- Paired migrations and tests: required; no applied migration edits. PASS.
- Test-first and CI gates: required in tasks. PASS.
- MVP restraint: no generic broker, no new service, no duplicate source event owner. PASS.

## Phase 0 and 1 Decisions

See [research.md](research.md), [data-model.md](data-model.md), [contracts/](contracts/), [quickstart.md](quickstart.md), and [ADR-027](../../docs/engineering/architecture/adr/ADR-027-notification-history-and-fcm-delivery.md).

- Reconcile the existing architecture sketches with implementation reality: device-token registration is documented but not implemented; migration 000021 provides it under the F-001 auth boundary.
- Use `notifications` as both recipient history/idempotency ledger and durable delivery state; keep one row per logical notification and per-device attempt rows only for device-level outcomes.
- Use source-event ID + recipient + notification type uniqueness. Source transitions create the durable F-008 row in their own transaction or through the existing source-owned outbox; F-008 does not take ownership of queue/chat outboxes.
- Persist only approved generic content, typed source reference, timestamps, read state, preference override marker, and bounded dispatch state. Fetch authorized detail from source feature on open.
- Check recipient/source eligibility at creation and recheck volatile eligibility at dispatch, especially F-006 occurrence/revision eligibility and circle membership.
- Preference OFF is checked for foreground alert presentation and immediately before first push attempt; history/unread stays. Once suppressed, that occurrence is terminal and re-enabling does not replay it. Quiet hours use profile IANA timezone and suppress/drop push only.
- F-008 owns realtime notification-history invalidation/update using existing authenticated user-targeted hub. In-app unread count is server authoritative. Native badge synchronization is best-effort on supported/enabled platforms.
- Required manager-deletion notices bypass ordinary circle-archive preference, remain subject to quiet hours, and must be transactionally durable before deletion/archive commits. Post-commit FCM failure retries and does not roll back closure. Enablement depends on operational readiness evidence; Karim must manually deep-review deletion security.
- When no token exists, retain the intent pending registration. Ordinary events expire after 24 hours from source event; required manager-deletion push expires after 30 days. Earlier source-validity cutoffs apply. Recheck current preference, quiet hours, and source eligibility at registration and immediately before dispatch. For F-006, each reminder expires when the next closer configured offset becomes due; the final offset expires at occurrence start.
- Circle invitation notification projection is in F-008 scope and fires when an authorized inviter sends a role-bound invitation to a named recipient. T023 integrates the committed invitation transition under F-002's role/authorization rules; the notification adapter must not infer an event from code sharing or joining.

## Project Structure

```text
backend/
  cmd/api/                         routes and wiring
  internal/notifications/          repository, policy, dispatcher, handlers
  internal/auth/                   FCM token registration and closure cleanup
  internal/{chat,queue,sessions,scheduling,circles,grades}/ source handoff seams only
  migrations/000021_notification_system.up.sql
  migrations/000021_notification_system.down.sql
  tests/contract/ and integration tests in owning packages
mobile/lib/features/notifications/ API, providers, history/settings screens, FCM routing
mobile/test/                       unit/widget coverage
mobile/integration_test/           end-to-end notification journeys
specs/008-notification-system/      this plan and contracts
```

**Structure Decision**: Extend the existing Go modular monolith and Flutter feature-first layout. Reuse auth, realtime, source repositories/outboxes, scheduling eligibility, route/deep-link patterns, logging, and migration/test conventions. No new service or event bus.

## Rollout and Safety

1. Add schema/contracts and source handoffs; deploy backend with dispatcher and token persistence while manager deletion remains disabled.
2. Add mobile permission/token lifecycle, history/settings, and deep-link reconciliation.
3. Verify source event coverage, retry/parking operations, retention, quiet-time behavior, invalid-token cleanup, and deletion transaction rollback/readiness in the target environment.
4. Enable manager deletion only after durable dispatcher operation and Karim's mandatory security review; do not claim push receipt.

## Complexity Tracking

No constitution violations. Per-device attempt records are justified by users having multiple devices and independent invalid tokens/partial FCM outcomes; no generalized delivery framework is introduced.
