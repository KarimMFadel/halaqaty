# Implementation Plan: F-006 Schedule, Calendar & Attendance

**Branch**: `006-schedule-calendar-attendance` | **Date**: 2026-09-27 | **Spec**: [spec.md](spec.md)

## Summary

Add circle-scoped recurring and one-off planning, a member calendar, and attendance derived from F-005 presence. Generate open-ended recurrence only for requested date ranges; retain durable exceptions and started history. Extend the existing `sessions` lifecycle without adding a fourth status. The F-006 migration and feature handlers are implemented; current fix-wave verification and the F-008 push dependency remain open.

## Technical Context

- **Stack**: existing Go service, Flutter/Riverpod client, PostgreSQL via pgx, Firebase identity, F-005 LiveKit media and WebSocket transport. No new dependency or service.
- **Source of truth**: PostgreSQL for plans, exceptions, materialized planned sessions, attendance, correction audit and idempotency. F-005 presence remains read-only input.
- **Scale**: MVP pilot at most 50 concurrent users and 10 live sessions (existing architecture); calendar reads are bounded by requested month, while creation has no future-date cap.
- **Tests**: Go unit/contract/integration; PostgreSQL migration up/down on disposable DB; Flutter widget/integration in Arabic RTL and LTR; timezone fixtures and concurrency/retry tests.
- **Contract**: `docs/contracts/openapi.yaml` contains the approved F-006 REST surface, synchronized with [contracts/schedule-calendar.openapi.yaml](contracts/schedule-calendar.openapi.yaml) and [contracts/profile-timezone.md](contracts/profile-timezone.md). The prior weekly-only schedule body was an unimplemented draft. Existing schedule path/method/operation IDs and the GET `data` wrapper remain; approved create semantics replace the draft request. Existing implemented F-005 and profile fields remain compatible. F-006 handlers and migration are implemented.

## Constitution Check

| Gate | Result |
|---|---|
| Spec and approved product scope | Pass: F-006 is Approved; clarified spec and 24/24 scheduling checklist; ADR-025/OQ-060 record policy. |
| Stack and ownership | Pass: reuse existing Go/Flutter/PostgreSQL/Firebase and F-005 media/presence; no new infrastructure. |
| Auth/privacy | Design: enforce both Firebase/backend session and current `circle_members` role for every read/write; no former-member history access or cross-member calendar inspection. |
| Contract-first | Approved feature contract is synchronized into canonical OpenAPI and API lint passes; implementation must still meet contract tests. |
| Schema governance | [ADR-026](../../docs/engineering/architecture/adr/ADR-026-schedule-occurrence-and-attendance-persistence.md) is accepted and architecture reflects it; the paired migration and up/down tests are recorded in [quickstart.md](quickstart.md). |
| F-008 push | Partial pilot only; full SC-009 remains pending F-008 approval and real delivery evidence. |

## Design and sequencing

1. **Use approved boundaries.** ADR-026, the data model, the F-006 REST contract, and architecture are aligned; approved paths/schemas are in `docs/contracts/openapi.yaml` before route code. Existing v1 F-005 routes and three-state status remain compatible. Add any new event to `docs/contracts/ws_events.md` before use; none is currently needed because authenticated refresh after mutation and on app resume covers the pilot.
2. **Persist plans and viewer timezone.** Add a paired F-006 migration after the current tip for `profiles.timezone` (valid IANA value, default `UTC` for existing profiles), `schedules`, effective-dated revisions, selected dates, exceptions, planned details, attendance, correction audit and request replay keys. Extend the existing `/auth/me` profile read/update contract additively so a viewer can store their timezone; validate it server-side. Keep planned metadata separate from F-005 `sessions`. Add foreign keys, checks, unique occurrence identity, indexes and rollback. Do not run migration in this phase.
3. **Generate occurrences on demand.** Store the entered local start and end clock times, elapsed `duration_minutes`, IANA zone and anchor local date. The end clock is retained as input; the resolved UTC end is start plus elapsed duration, and the displayed local end may differ across DST. Validate duration from 1 minute through 31 days (44,640 minutes) and require entered end clock to equal start clock plus duration by nominal 24-hour arithmetic. This caps only planned length, not permitted dates or times; F-005 still ends live sessions under its existing limit. Weekly/biweekly weekday sets use week cadence; arbitrary day/week intervals step from the anchor; selected dates are deduplicated. The inclusive end date applies to local starts. Resolve DST gaps by advancing by the clock change and repeated starts to the first offset. Query one calendar month at a time: for each rule, calculate a lower bound from its stored duration and jump arithmetically to the first candidate at or after that bound; also include persisted history and moved exceptions. Do not iterate from a distant anchor or impose a future planning horizon. Stable identity is `(schedule_id, original_local_date)`. A new series revision supersedes unstarted exceptions but retains past virtual and started/completed history.
4. **Integrate F-005 lifecycle.** A recurring occurrence is virtual until edited/cancelled or started. Start, cancel and series edit serialize on the parent `schedules` row for a recurring occurrence; one-off start/cancel serialize on its `sessions` row. Refactor the existing F-005 start repository transaction into a shared transactional body so planned start can lock the schedule, recheck current revision/exception/details, materialize at most one row, and perform the existing F-005 media/activation/admission flow in **one transaction**. Both the ordinary F-005 start route and planned route use that body; no nested or intervening transaction may release the lock. F-005 checks planned cancellation immediately before scheduled-to-active transition. Cancellation sets the authoritative exception and, if already materialized, the detail cancellation in the same transaction; it cannot succeed after activation. For a virtual cancellation, no session row is needed. A cancelled row or exception cannot start or appear as startable in F-005 discovery, but remains in F-006 calendar history. F-005's ad-hoc route and four-hour/idle limits remain intact; planned end is informational.
5. **Provide overlap warnings.** A read-only preview evaluates the first 31 local days (or finite selected dates) against current authorized commitments, using half-open UTC intervals; touching endpoints are not overlaps. Each warning includes authorized circle identity and the overlapping UTC interval, without another member's private schedule. On write, recompute inside a serializable scheduling transaction and compare the current warning IDs (derived from occurrence identities and overlap instants) with the IDs the manager reviewed; retry serialization failures within the existing bounded timeout. If new or changed warnings exist, return `409` with the current safe warnings and require a fresh explicit confirmation; a bare `confirm_overlaps: true` cannot silently confirm a new conflict. This bounds preview without limiting future dates. Calendar reads recompute member-only warnings for their requested window.
6. **Classify attendance.** At actual start, snapshot active students in the same database transition. When a newly enrolled student makes an authorized F-005 join/presence transition, durably upsert their `later_participant` roster eligibility before membership can be removed; do not infer it later from the current `circle_members` row. On end, classify one row per eligible student: first presence through `actual_start + 10m` is Present, later is Late, absent if none. Manual teacher corrections are separate audit rows and take precedence on replay/recalculation. No attendance for never-started/cancelled sessions. F-005 raw presence is never rewritten.
7. **Mobile delivery.** Reuse the existing circle detail and session screens. Add schedule manager, one-off form, current-month unified calendar, attendance review/correction and clear warning confirmation. Show planned, live, completed and cancelled states, circle text plus non-color cue, timezone and resolved overnight end. Support Arabic RTL/LTR and loading/empty/error/retry/success/offline states. Offline viewing may use last fetched data with a stale label; offline mutation is not queued.
8. **F-008 boundary.** Expose eligible occurrence identifiers/times for later F-008 reminder integration, with generation/version checks so cancelled or superseded items are ineligible. Do not add a push job, preference store, FCM send or placeholder claim to F-006. Label pilot incomplete until F-008's four intervals and foreground/background/closed-app evidence pass.

## Reliability and security

- Every mutating command checks authenticated backend session, current circle role and active circle state inside the transaction; readers check current membership even for archived history. Teachers and supervisors manage plans; only teachers correct attendance. Deny cross-circle IDs without disclosing existence.
- Use request idempotency keys and a replay table for create/cancel/correct operations, plus schedule `version` compare-and-swap on edits. Unique occurrence identity and `session_attendance(session_id,user_id)` prevent duplicate materialization/classification. Serialize start/cancel on the same occurrence key; after a conflict, return current state rather than silently overwrite.
- Use the existing global per-IP/per-user REST rate limits and validation/error envelope. Validate IANA profile/planning zones, local dates/start/end times, 1–44,640 minute planned duration, positive interval, mode-specific fields, bounded request body and one-month query window; never validate by limiting how far in the future a date lies. For month reads, use each rule's validated duration to include every occurrence intersecting the month, including starts in earlier months.
- Database transactions and retry-safe upserts handle transient failures. Reuse the existing F-005 start/reconciliation path, which currently ensures the LiveKit room and issues the connection inside its serialized start transaction; do not introduce another media path or weaken its rollback/recovery behavior. Use request IDs and structured redacted logs/metrics for warning, start, classification and correction outcomes; no student calendar details in logs. Existing F-005 timeout/recovery remains authoritative for live sessions.

## Project Structure

```text
specs/006-schedule-calendar-attendance/{plan.md,research.md,data-model.md,quickstart.md,contracts/}
backend/migrations/                         # paired F-006 migration
backend/internal/scheduling/                # recurrence, calendar and warnings
backend/internal/attendance/                # classification/correction
backend/internal/sessions/                  # planned-start guard and snapshot integration
backend/cmd/api/routes.go                   # centralized route patterns
mobile/lib/features/scheduling/            # Riverpod data/application/presentation
mobile/lib/features/attendance/            # review/correction UI
docs/contracts/openapi.yaml                # canonical REST source of truth
docs/engineering/architecture/ARCHITECTURE.md
```

## Verification and gates

- Recurrence tests: every mode, duplicate dates, inclusive end, selected past rejection, retained local end versus resolved DST end, distant future, long-duration cross-month inclusion, exception supersession, stable identity and concurrency.
- API/DB tests: stored viewer-zone read/update and invalid zone rejection, role matrix, archived/revoked access, actionable privacy-safe warnings and new-conflict reconfirmation, F-005 start response/media failure compatibility, virtual/materialized cancellation/start races, departure after later-enrolled participation, duplicate requests, correction audit/replay, migration up/down, and query plans for distant month retrieval.
- Mobile tests: manager flow, calendar current month and later navigation, conflict confirmation, attendance permissions, RTL/LTR, non-color cues and all required states. Device/backend integration is separate from widget tests.
- Before implementation: accepted ADR-026, architecture and canonical OpenAPI sync, API lint, read-only analysis, applicable review. Before PR: unfiltered Go unit/contract/integration/combined coverage and lint; Flutter test/integration/analyze/format; secret scan; Tech Lead review and Karim's required manual review where applicable. Report unrun gates as unverified.

## Open items

- ADR-026 and the spec were approved by Karim on 2026-09-27. The corresponding migration and feature handlers are implemented; current fix-wave gates remain to be recorded in [quickstart.md](quickstart.md).
- Canonical and feature-local REST shapes are synchronized; repeat contract verification after the current fixes. No new WebSocket event is proposed.
- F-008 is Proposed; SC-009 blocks full F-006 completion, but not a clearly labelled partial pilot.
