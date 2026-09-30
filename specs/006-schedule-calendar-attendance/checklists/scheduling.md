# Scheduling Requirements Checklist: F-006

**Purpose**: Assess the clarity, consistency, and coverage of F-006 requirements before planning; these items assess written requirements, not implementation.
**Created**: 2026-09-27
**Feature**: [spec.md](../spec.md)
**Audience**: Specification author and reviewer
**Depth**: Full requirements quality assessment for scheduling, attendance, permissions, and dependencies

## Requirement completeness and clarity

- [x] CHK001 Are both selected weekdays on weekly/biweekly patterns and arbitrary intervals or explicit dates included in the required scope? [Completeness, Spec §FR-001]
- [x] CHK002 Are the allowed units, anchor date, and interval-count rules for arbitrary repeats defined so two readers derive the same occurrences? [Ambiguity, Spec §FR-001]
- [x] CHK003 Are the selection, deduplication, past-date, and optional end-date rules for explicitly selected dates defined? [Gap, Spec §FR-001–FR-002]
- [x] CHK004 Are the IANA planning timezone, UTC occurrence instant, DST gap/repetition rule, and viewer timezone explicit? [Clarity, Spec §FR-002/FR-006]
- [x] CHK005 Is the treatment of overnight occurrences across a DST change specified for both start and end instants? [Gap, Spec §FR-001–FR-002]
- [x] CHK006 Are present/future creation, no future scheduling cap, current-month initial display, and navigation to later months stated separately? [Clarity, Spec §FR-002]
- [x] CHK007 Are one-off sessions, optional title, and informational duration separated from F-005's automatic end limits? [Consistency, Spec §FR-003]
- [x] CHK008 Are the semantics of a recurrence end date defined, including whether its local date is inclusive and how an overnight final occurrence is treated? [Ambiguity, Spec §FR-001]

## Scenario and edge-case coverage

- [x] CHK009 Are one-occurrence edits/cancellations, future-series changes that supersede prior unstarted exceptions, and immutable completed history addressed? [Coverage, Spec §FR-007/FR-013; amended after clarification]
- [x] CHK010 Is the precedence between a one-occurrence exception and a later series change specified for unstarted occurrences? [Gap, Spec §FR-013]
- [x] CHK011 Are occurrence identity, duplicate-request safety, and concurrent-edit outcomes included as requirements? [Coverage, Spec §FR-013]
- [x] CHK012 Is the overlap policy defined for open-ended recurrences without implying an infinite search or silently limiting future scheduling? [Gap, Spec §FR-002/FR-008]
- [x] CHK013 Are non-blocking same-/cross-circle manager warnings, student calendar warnings, and authorized disclosure distinguished? [Clarity, Spec §FR-008; amended to reflect the later accepted manager-choice policy]
- [x] CHK014 Are cancellation eligibility and F-005's existing three-state lifecycle distinguished without claiming an approved cancellation field or status? [Consistency, Spec §FR-007]
- [x] CHK015 Is the calendar treatment of past completed sessions, archived-circle history, and retained cancelled occurrences defined sufficiently to support the stated user journeys? [Gap, Spec §FR-004/FR-007/FR-012]

## Attendance and access requirements

- [x] CHK016 Are the actual-start time anchor, 10-minute boundary, no minimum duration, reconnect handling, Absent finalization, and manual Excused classification explicit? [Clarity, Spec §FR-009–FR-010]
- [x] CHK017 Are start-time roster eligibility, newly enrolled participants, ad-hoc sessions, and never-started/cancelled exclusions documented? [Coverage, Spec §FR-009–FR-010]
- [x] CHK018 Are correction audit details, persistence through recalculation, and preservation of raw presence facts stated? [Completeness, Spec §FR-011]
- [x] CHK019 Is the read-access outcome for a student whose membership is later revoked or whose circle is archived stated consistently with retained-history access? [Ambiguity, Spec §FR-012 and Edge Cases]
- [x] CHK020 Are teacher/supervisor/student permissions, non-member denial, and archived read-only behavior separated from F-005 moderation rights? [Consistency, Spec §FR-012]

## Acceptance, accessibility, and dependencies

- [x] CHK021 Are Arabic RTL/LTR, non-color identification, and loading/empty/error/retry/success/offline state requirements recorded? [Coverage, Spec §FR-005/FR-014]
- [x] CHK022 Are F-006 reminder eligibility, F-008 delivery ownership, the partial pilot, and the full-completion condition distinguished? [Dependency, Spec §FR-015/SC-009]
- [x] CHK023 Does each F-006 product acceptance criterion have a specific requirement and measurable scenario, including reminder intervals, manual correction, lifecycle, and expanded recurrence? [Gap, Spec §FR-001–FR-015/SC-001–SC-009; FEATURES F-006]
- [x] CHK024 Are the accepted broader recurrence, teacher/supervisor scheduling, non-blocking overlaps, and cancellation decisions reconciled in the canonical product, architecture, and REST contract text through the required approval process? [Conflict, Spec §Assumptions; FEATURES F-006; ARCHITECTURE §schedules/sessions; OpenAPI §Schedules]

## Assessment

- Reassessment 2026-09-27: 23/24 items satisfied; CHK024 remains open. The accepted decisions now make the interval/date, DST, end-date, series-change, overlap-warning, calendar-history, and revoked-member requirements explicit in the spec. The CHK013 wording was updated because the earlier blocking policy was superseded by Karim's manager-choice clarification.
- Existing FEATURES acceptance criteria map to FR-001/SC-006 (weekly and multiple entries), FR-015/SC-009 (push intervals), FR-009–FR-011/SC-004/SC-007 (attendance and manual corrections), FR-004–FR-005/SC-003 (unified calendar), FR-008/SC-008 (conflicts), and FR-007/SC-006 (lifecycle/cancellation). The accepted expanded recurrence and supervisor scheduling rights also have explicit scenarios, but canonical wording remains stale.
- CHK024 is a blocking cross-document conflict: FEATURES and JOURNEY still imply teacher-only weekly scheduling and a Completed → Cancelled sequence; ARCHITECTURE and OpenAPI still show teacher-only weekly scheduling and the F-005 three-state lifecycle. Product wording must be reconciled with Karim's accepted decisions through the governing process; architecture/ADR/schema/contract work is still required before implementation. This checklist does not approve a technical design.
- The `.specify/scripts/powershell/check-prerequisites.ps1 -Json` mode stops because `plan.md` does not yet exist. `-Json -PathsOnly` resolved this feature and branch. The Halaqaty phase order puts checklist before plan, so this assessment uses the verified feature path and existing spec directly. No tests or implementation gates were run.
- Reconciliation 2026-09-27: 24/24 requirements-quality items now satisfied. CHK024 closed against ADR-025, OQ-060 and its amendment, the mirrored product/journey descriptions, architecture's explicit F-006 boundary and unimplemented schedule status, and OpenAPI's explicit draft weekly shape. This closes the policy/document contradiction; it does not approve a final schema, an implemented endpoint, migration, or completed F-008 reminders. `/speckit.plan` owns those design deliverables and any additional ADR approval.
