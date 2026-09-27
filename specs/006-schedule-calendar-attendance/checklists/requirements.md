# Specification Quality Checklist: F-006 Schedule, Calendar & Attendance

**Purpose**: Validate specification completeness and quality before proceeding to planning

**Created**: 2026-09-26

**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain in the spec (verified after the 2026-09-27 clarification update).
- [x] Requirements are testable and unambiguous; expanded recurrence coverage was revalidated in `checklists/scheduling.md` (24/24), with the 31-day planned-duration limit approved on 2026-09-27.
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria; expanded recurrence scenarios were revalidated in `checklists/scheduling.md` (24/24).
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria once unresolved policies are decided
- [x] No implementation details leak into specification

## Notes

- Specification validation completed on 2026-09-26 and refreshed after 2026-09-27 clarification, scheduling checklist, and Karim's approval of the 31-day planned-duration bound. F-008 reminder delivery remains a separate feature dependency for full F-006 completion.
- F-005 participant presence is an input only. This feature must not modify presence facts or reassign F-005 ad-hoc-session ownership.
- FR-007 includes cancelled planned sessions, while the current F-005 session status contract accepts only `scheduled`, `active`, and `ended`. Planning must explicitly reconcile cancellation with the existing session lifecycle and contract; it cannot silently expand the F-005 enum.
- Clarification update 2026-09-27: Karim answered all seven policy groups. FR-008, FR-010, and FR-015 markers are resolved; expanded recurrence, separate cancellation, supervisor attendance read access, correction audit, and partial-pilot dependency are recorded in the spec. Follow-ups resolved DST handling (shift forward/first occurrence) and rejected past-time creation. The initial calendar view is the current month with navigation to any later month. Custom recurrence supports both selected weekdays on weekly/biweekly patterns and arbitrary repeat intervals or explicitly selected dates; FR-001 and its acceptance scenarios now reflect both. No unanswered clarification markers remain. Earlier checked items are historical specification checks, not fresh checklist-phase approval.
- Work now uses `006-schedule-calendar-attendance` from `main`; the shared gap batch is closed. Canonical recurrence/lifecycle reconciliation and architecture approval remain prerequisites for an approved implementation plan. No checklist, planning, implementation, or delivery gate was run by this clarification update.
- Checklist assessment 2026-09-27: [scheduling.md](scheduling.md) contains 24 requirements-quality checks. Ten remain open, including interval/date semantics, overnight DST and end-date boundaries, exception precedence, recurrence-overlap scope, retained-history access, acceptance traceability, and product/architecture consistency. The previously checked items above are historical; the current specification is not yet cleared for `/speckit.plan`. Resolve material requirement decisions through `/speckit.clarify`, then rerun this checklist.
- Reassessment 2026-09-27: [scheduling.md](scheduling.md) now has 23/24 checks satisfied after the accepted clarification. CHK024 remains open because canonical product, architecture, and REST contract wording has not been reconciled with expanded recurrence, supervisor scheduling, warning-only overlaps, and separate cancellation. The spec is internally clarified; this cross-document conflict still blocks declaring the checklist fully clear for planning. No technical design or implementation approval is implied.
- Reconciliation 2026-09-27: CHK024 in [scheduling.md](scheduling.md) is closed, yielding 24/24 requirements-quality checks satisfied. ADR-025 and OQ-060 record the approved policy; product and journey wording is synchronized; architecture and OpenAPI label their weekly schedule shapes as unimplemented drafts pending `/speckit.plan`. This checklist permits planning but does not approve fields, migrations, API/event shapes, implementation, or F-008 reminder delivery.
