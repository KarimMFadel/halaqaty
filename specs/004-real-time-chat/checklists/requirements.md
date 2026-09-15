# Requirements Quality Checklist: Real-time Chat

**Purpose**: Validate F-004 requirements for completeness, clarity, consistency, measurability, and readiness for implementation planning
**Created**: 2026-09-03
**Feature**: [spec.md](../spec.md)

**Note**: This checklist evaluates the written requirements, not the implementation.

## Requirement Completeness

- [x] CHK001 Are permissions specified for every group-chat and DM operation, including history, send, upload, reply, read, typing, pin, delete, search, realtime subscription, and media renewal? [Completeness, Spec §FR-003–FR-004, §FR-016–FR-018]
- [x] CHK002 Are group-history visibility requirements defined consistently for first-time members, removed members, rejoining members, and retained archived-circle members? [Completeness, Spec §FR-001, §FR-026, §FR-032–FR-033]
- [x] CHK003 Are DM lifecycle requirements complete for eligibility gain, loss, restoration, role changes, membership removal, and archived-only relationships? [Completeness, Spec §FR-016–FR-018]
- [x] CHK004 Are requirements present for empty, loading, retrying, terminal-failure, and permission-denied states in each critical mobile chat flow? [Gap, Spec §User Stories 1–5]
- [x] CHK005 Are offline-send requirements complete for validation failures, authorization loss before reconnect, user cancellation, app restart, and retry exhaustion? [Gap, Spec §FR-008]
- [x] CHK006 Are upload-staging requirements defined for abandoned uploads, failed message attachment, idempotent retry, and objects that never become part of a message? [Gap, Spec §FR-022–FR-024]
- [x] CHK007 Are read-receipt requirements defined for messages sent before a member joined, messages sent after a member left, sender self-reads, and later eligibility restoration? [Completeness, Spec §FR-012–FR-014]
- [x] CHK008 Are teacher-deletion audit requirements accompanied by an approved durable data-ownership and retention requirement? [Gap, Spec §FR-030, §SR-008]

## Requirement Clarity

- [x] CHK009 Is deterministic newest-page ordering defined with an unambiguous tie-break rule for equal acceptance timestamps? [Ambiguity, Spec §User Story 1, Scenario 1]
- [x] CHK010 Is the start and end of a “current active membership period” defined precisely enough to handle removal, rejoin, role change, and archival? [Clarity, Spec §Clarifications, §FR-001, §FR-026]
- [x] CHK011 Is “eligible non-sender” defined for group read status when membership changes after a message is accepted? [Ambiguity, Spec §FR-014]
- [x] CHK012 Is the automatic typing-expiry interval or an objectively selectable planning constraint specified? [Ambiguity, Spec §FR-015]
- [x] CHK013 Are the exact transitions and failure outcomes for pending, sent, delivered, and read states unambiguous across REST failure, timeout, and reconnect? [Clarity, Spec §FR-008, §FR-012]
- [x] CHK014 Is the meaning of “actual MIME type” sufficiently defined for JPEG, PNG, PDF, and voice validation without relying on filename extensions? [Clarity, Spec §FR-021–FR-022]
- [x] CHK015 Is Arabic full-text search behavior defined for normalization, diacritics, partial terms, and result ordering? [Gap, Spec §FR-026]
- [x] CHK016 Is “normal circle or account data cleanup” tied to a defined retention event or explicitly documented as indefinite storage until another feature governs deletion? [Ambiguity, Spec §FR-031]

## Requirement Consistency

- [x] CHK017 Are immediate attachment-access revocation and renewable seven-day presigned links reconciled, including links issued before soft deletion? [Conflict, Spec §FR-024, §FR-031]
- [x] CHK018 Is pair-owned DM media consistent with the requirement that every upload carries target-circle context when multiple qualifying circles exist? [Consistency, Spec §FR-023, §FR-035]
- [x] CHK019 Is the one-conversation-per-pair rule consistent with the directional `dm_recipient_id` architecture and is pair identity/uniqueness stated explicitly? [Consistency, Spec §FR-016, §Key Entities: Message]
- [x] CHK020 Is excluding F-008 trigger creation consistent across the scope, requirements, reliability model, success criteria, and the F-004 product acceptance criterion for background FCM delivery? [Conflict, Spec §FR-034, §SR-001, §SR-007, §SC-001]
- [x] CHK021 Are supervisor permissions consistent across pinning, deletion, DM eligibility, and archived-circle behavior without implying teacher-equivalent moderation rights? [Consistency, Spec §FR-016, §FR-027, §FR-030, §FR-032]
- [x] CHK022 Are the 5 MB image, 10 MB PDF, and 20 MB voice limits consistent across clarifications, scenarios, functional requirements, architecture, and contracts? [Consistency, Spec §Clarifications, §FR-020–FR-022]
- [x] CHK023 Is the requirement for a durable teacher-deletion audit record consistent with the scope restriction to the approved `messages` and `message_reads` architecture? [Conflict, Spec §FR-030, §FR-037]

## Acceptance Criteria Quality

- [x] CHK024 Does every functional and safety requirement map to at least one independently testable acceptance scenario or measurable outcome? [Traceability, Spec §User Scenarios, §FR-001–FR-039, §SR-001–SR-008]
- [x] CHK025 Can the “MVP reference load” used by the two-second history/search and realtime targets be objectively reproduced from stated concurrency and data-volume assumptions? [Measurability, Spec §SC-005–SC-006]
- [x] CHK026 Are success criteria defined for DM history restoration, membership-period history filtering, and multi-circle DM media authorization? [Coverage, Spec §Clarifications, §SC-002–SC-004]
- [x] CHK027 Are acceptance outcomes explicit for every terminal offline-send failure rather than only successful retry and reconciliation? [Gap, Spec §User Story 2, §FR-008]
- [x] CHK028 Can Arabic-first, RTL, accessibility, and non-color-only status requirements be measured using explicit states and assistive-technology expectations? [Measurability, Spec §FR-038, §SC-009]

## Scenario and Edge-Case Coverage

- [x] CHK029 Are concurrent send, read, pin, delete, and authorization-change requirements complete for both group and direct conversations? [Coverage, Spec §Edge Cases, §SR-004]
- [x] CHK030 Are recovery requirements specified for REST success followed by missed realtime delivery, realtime delivery before REST response, reconnect gaps, and duplicate events? [Coverage, Spec §FR-007–FR-011]
- [x] CHK031 Are requirements defined for microphone denial, recording interruption, playback failure, expired links, and media-renewal denial? [Gap, Spec §User Story 3, §FR-019–FR-024]
- [x] CHK032 Are reply requirements complete when the original becomes deleted, unauthorized, outside the current membership period, or inaccessible after DM eligibility loss? [Coverage, Spec §FR-025, §FR-031]
- [x] CHK033 Are pagination and search requirements defined for empty histories, page-boundary insertions/deletions, and messages becoming unauthorized between requests? [Gap, Spec §FR-011, §FR-026]
- [x] CHK034 Are rate-limit requirements explicit for idempotent retries, multiple devices, multiple qualifying DM circles, and uploads that do not produce messages? [Coverage, Spec §FR-006, §FR-022, §Edge Cases]

## Non-Functional Requirements

- [x] CHK035 Are privacy requirements complete for minors, DM history restoration, filenames, quoted previews, audit records, and retained soft-deleted media? [Completeness, Spec §SR-005–SR-008]
- [x] CHK036 Are observability requirements specified for authorization denial, realtime gaps, upload rejection, idempotency conflicts, and storage/realtime dependency failures without leaking sensitive data? [Gap, Spec §SR-001, §SR-006]
- [x] CHK037 Are availability, timeout, retry-bound, and graceful-degradation requirements documented for PostgreSQL, MinIO, and the shared realtime transport? [Gap, Spec §SR-001, §FR-008–FR-011, §FR-024]
- [x] CHK038 Are the 50-concurrent-user and ten-live-session constitutional limits translated into relevant chat load and storage assumptions without coupling chat to live sessions? [Assumption, Spec §FR-002, §SC-005–SC-006]

## Dependencies and Scope Boundaries

- [x] CHK039 Is the F-002 role-bound invitation amendment identified as a planning dependency rather than an F-004 implementation responsibility? [Dependency, Spec §SR-003, §Scope Boundaries, §Assumptions]
- [x] CHK040 Is reliance on F-001 current-device sessions and F-005 realtime tickets stated for every affected authorization flow without duplicating their responsibilities? [Dependency, Spec §FR-003, §FR-009–FR-010, §Assumptions]
- [x] CHK041 Are required corrections to the existing upload and realtime contracts explicitly bounded to F-004 planning artifacts and canonical contract updates? [Dependency, Spec §Clarifications, §FR-035–FR-036]
- [x] CHK042 Are voice codec/container selection, physical media purge, push delivery, and live-session behavior clearly deferred without weakening the fixed product outcomes? [Scope, Spec §FR-020, §FR-031, §FR-034, §Scope Boundaries]

## Notes

- Resolved 2026-09-06 through the approved clarification, ADR-021/Constitution amendment, contract reconciliation, and replanning pass. Checked items confirm requirement quality only; implementation and manual-review evidence remain tracked in `tasks.md` and release gates.

- Check items off as the specification resolves each requirement-quality question: `[x]`.
- Record gaps or decisions inline and link the corresponding specification change.
- Do not use this checklist as an implementation test plan.
- Planning review 2026-09-03: all 12 accepted clarification statements in `spec.md` are individually traced in `plan.md` and reflected in the feature/canonical contracts. All 42 checks resolved 2026-09-06; no unchecked requirements-quality questions remain.
