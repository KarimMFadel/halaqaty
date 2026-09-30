# ADR-025: F-006 Schedule Policy and Session Lifecycle Boundary

**Status:** Accepted
**Date:** 2026-09-27
**Decider:** Karim (product owner)

## Context

The approved F-006 clarification extends the older weekly, teacher-only schedule description. The current architecture sketch and OpenAPI request describe one weekly weekday, while the F-005 session contract deliberately has only `scheduled`, `active`, and `ended` states. Treating those older shapes as the complete F-006 design would lose accepted recurrence, supervisor scheduling, and cancellation behavior.

## Decision

- F-006 planning must cover selected weekdays on weekly/biweekly patterns, positive whole-number day/week intervals, explicitly selected dates, optional inclusive local end dates, and one-off planned sessions. Schedule entries retain local clock time and an IANA planning timezone; occurrence instants use UTC. Existing weekly contract/schema sketches are not the final F-006 model.
- Active circle teachers and supervisors may manage schedules and one-off planned sessions. Same- and cross-circle overlaps produce privacy-safe warnings; the manager may proceed. Student overlap warnings do not restrict participation. Only teachers may correct attendance; supervisors may read it.
- A teacher or supervisor may cancel an unstarted planned occurrence or change/stop future occurrences. A later series change replaces prior individual edits to affected unstarted occurrences; started/completed history remains intact. Cancellation is separate from F-005's `scheduled → active → ended` status. Active sessions use F-005 End behavior.
- F-005 keeps ownership of the existing ad-hoc creation, live-session moderation, media, presence facts, and three-state lifecycle. F-006 consumes presence facts for attendance without rewriting them. F-008 owns push delivery and preferences; its verified reminders remain a dependency for full F-006 completion.
- F-006 planning must define any additional persistence and REST contract shape through Spec-Kit. New tables/columns require the architecture update and a paired migration under the repository's ADR process before implementation. This ADR approves the product boundary, not unspecified fields, endpoints, or a new persisted session status.

## Alternatives considered

| Alternative | Reason rejected |
|---|---|
| Limit F-006 to one weekly weekday and teachers only | Conflicts with Karim's accepted recurrence and supervisor scheduling decisions. |
| Block overlapping circle sessions | Karim chose manager discretion after a warning. |
| Add `cancelled` to F-005 session status | Changes the established lifecycle instead of keeping cancellation separate from live state. |
| Rebuild F-005 sessions or F-008 delivery within F-006 | Crosses their approved ownership boundaries. |

## Governance

This records Karim's F-006 clarification decisions on 2026-09-27 under the constitution's spec-first and circle-scoped authorization rules. It does not amend the constitution or approve a schema/library change. Any later schema change still needs an approved design and migration before implementation.

## References

- [F-006 specification](../../../../specs/006-schedule-calendar-attendance/spec.md)
- [MVP Decision Register](../../../management/product/MVP_DECISION_REGISTER.md)
- [ADR-016: Session presence boundary](ADR-016-session-realtime-and-presence-foundation.md)
- [Architecture](../ARCHITECTURE.md)
