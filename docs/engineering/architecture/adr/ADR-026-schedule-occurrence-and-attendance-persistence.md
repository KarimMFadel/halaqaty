# ADR-026: Schedule Occurrence and Attendance Persistence

**Status:** Accepted — approved by Karim on 2026-09-27; implemented as a partial F-006 pilot
**Date:** 2026-09-27
**Decider:** Karim (product owner)

## Context

ADR-025 approves broad recurrence, advisory overlap warnings, teacher/supervisor scheduling, and cancellation outside F-005's three-state session lifecycle. The existing `sessions` and `session_participant_presence` tables contain live lifecycle and raw presence, but no recurrence, exception or attendance model. An open-ended schedule cannot safely pre-create every future session row.

## Decision

- Add `profiles.timezone` as an IANA name, with `UTC` for existing profiles and an additive `/auth/me` read/update contract. Add `schedules` with stable identity/current version and `schedule_revisions` with mode-specific local recurrence rules, retained local start/end clock times, 1–44,640 minute planned duration, IANA planning zone, inclusive end date and effective local date; add versioned selected dates and occurrence exceptions. The 31-day duration ceiling bounds month retrieval without restricting permitted start dates/times or F-005 live-session policy. The end clock agrees with start plus duration by nominal local clock arithmetic; resolved ends preserve elapsed duration across DST. Compute open-ended occurrences for requested windows using duration-based lookback and arithmetic jumps from the anchor. `(schedule_id, original_local_date)` is the stable logical identity. Later series edits supersede unstarted exceptions but keep earlier revisions so past virtual and started history remain reproducible.
- Retain F-005 `sessions` as the sole live-session state. Factor its existing start transaction body for reuse so a recurring start locks the parent schedule, rechecks exception/detail cancellation, materializes at most one session and runs F-005 media/activation/admission in the same transaction. Recurring cancel/series edit takes the same parent lock, updates exception and materialized detail together, and cannot cancel an active session. One-offs materialize on creation and serialize start/cancel on the session row. Keep title, local clock inputs, planned end, timezone, series linkage and cancellation in `planned_session_details`. Cancellation never changes the F-005 status enum.
- Add `session_attendance` for the eligible roster and derived/final status, plus append-only `attendance_corrections` for teacher overrides. Use F-005 `first_joined_at` without altering presence. Snapshot active students at actual start; on an authorized later-enrolled student's join/presence transaction, persist their eligibility immediately, before membership can be removed. Finalization reads the durable roster, not current membership. Corrections survive recalculation.
- Add a scoped request-replay record for retry-safe mutations. Use unique occurrence and attendance keys, schedule version CAS and row locks for start/cancel races. Overlap warnings identify only authorized circles and UTC overlap intervals; a write recomputes warnings and requires renewed confirmation for any new/changed warning ID. Maintain current `circle_members` authorization on every read and write; archive is read-only.
- Implement through an additive paired migration after the current tip, with FK/check/index rules in the [F-006 data model](../../../../specs/006-schedule-calendar-attendance/data-model.md). Update `ARCHITECTURE.md` and canonical OpenAPI before code; do not add a library or service.

## Alternatives considered

| Alternative | Reason rejected |
|---|---|
| Materialize every future occurrence | Impossible for open-ended recurrence and creates an implicit future cap. |
| Add `cancelled` to `sessions.status` | Conflicts with ADR-025 and F-005's established three-state lifecycle. |
| Store attendance in F-005 presence | Conflates raw participation with policy and loses manual corrections. |
| Store recurrence as a generic RRULE string | Adds a wider grammar and parsing/validation surface than the approved modes require. |

## Consequences and implementation gates

Karim approved this technical design, including the 31-day planned-duration limit, on 2026-09-27. Paired migration 000020 and the F-006 code are present in this branch; full pilot acceptance still depends on current verification and F-008 reminder delivery. The decision satisfies constitution §III (PostgreSQL source of truth, existing stack), §IV (circle-role authorization) and §VI (test-first changes); it does not amend the constitution. F-008 retains push delivery ownership.

## References

- [ADR-025](ADR-025-schedule-policy-and-session-lifecycle-boundary.md)
- [ADR-016](ADR-016-session-realtime-and-presence-foundation.md)
- [F-006 plan](../../../../specs/006-schedule-calendar-attendance/plan.md)
- [Architecture](../ARCHITECTURE.md)
