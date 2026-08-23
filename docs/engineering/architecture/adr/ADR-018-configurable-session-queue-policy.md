# ADR-018: Configurable Session Recitation Queue Policy

**Status:** Accepted
**Date:** 2026-08-23
**Deciders:** Karim (product owner)

## Context

F-003 needs predictable defaults for queue population, unfinished entries,
opt-out approval, grade visibility, and grade correction. Quran circles also
vary operationally, and a queue rule must not prevent a teacher or supervisor
from ending an F-005 session or adapting the current recitation workflow.

Making every rule optional would conflict with per-circle authorization,
PostgreSQL source-of-truth, one-position/one-reciter integrity, completed-only
progress, and the constitutional active-reciter media boundary. The design must
therefore separate configurable session policy from immutable platform safety.

## Decision

Each session has one F-003 queue-policy configuration. Its defaults are:

| Policy | Default | Other allowed behavior |
|---|---|---|
| Queue population | `present_at_activation` | `all_active_students` |
| Unfinished-entry finalization | `mark_unfinished_skipped` | `preserve_last_state` |
| Student opt-out | `approval_required` | `auto_approve` |
| Grade/note visibility | `managers_and_student` | `managers_only`, `all_participants` |
| Grade/note correction | `audited_any_time` | `before_round_finalization`, `immutable` |

An active teacher or supervisor may change these values while the session is
scheduled or active. Workflow changes apply prospectively and never rewrite
durable history. Visibility changes affect authorization projections, not stored
grades or notes. Every policy change and allowed grade/note correction emits a
redacted audit event after the business transaction commits.

`created_by` remains audit attribution, not a permanent session role. A session
creator may manage F-003 only while they remain an active teacher or supervisor
in the session's circle.

The following remain non-configurable:

- authenticated current-device session and active per-circle membership/RBAC;
- one position per student per round and at most one active reciter;
- canonical queue states, round types, grades, Quran-range validation, and
  idempotent/concurrency-safe transitions;
- PostgreSQL as source of truth and completed-only progress creation;
- completed `test` turns remain practice history but do not alter F-007
  Quran-map memorization/revision status;
- provider-neutral `ReciterAudioControl`, active-reciter-only student audio,
  video disabled, and complete media credential/room-reference secrecy; and
- retained finalized history, with prior correction values preserved by audit,
  and no student self-logging.

F-005 session lifecycle is independent of F-003 policy. A teacher/supervisor or
automatic F-005 rule may end the session without queue approval or synchronous
queue cleanup. The committed ended session is returned immediately. F-003 then
revokes any reciter entitlement and finalizes the active round idempotently;
failure is redacted telemetry plus retry and never rolls back or delays end.

The exact persistence fields and REST/event representations are decided during
F-003 planning and contract synchronization. They must use closed values above
and must not expose provider or credential material.

## Consequences

- Teachers and supervisors can adapt queue workflow per session without an
  unrestricted rules engine or arbitrary configuration data.
- Defaults preserve the accepted clarification answers and existing humane,
  predictable behavior.
- F-003 requires durable policy change, opt-out request, and grade-correction
  audit coverage; logging failure does not roll back committed business state.
- F-005 end remains reliable even when F-003 finalization temporarily fails.
- Canonical OpenAPI, WebSocket projections, architecture, and F-003 planning
  must represent the closed policy values before implementation.

## Alternatives Rejected

| Alternative | Reason Rejected |
|---|---|
| Make every validation and safety rule configurable | Allows unauthorized actions, duplicate reciters/progress, and media-secret or publishing violations. |
| Keep every queue rule globally fixed | Prevents managers from adapting legitimate live-circle workflows. |
| Give `created_by` permanent manager authority | Bypasses current per-circle role authorization after a role change. |
| Block session end until queue cleanup succeeds | Couples F-005 lifecycle availability to F-003 and can trap participants during failure. |
| Generic key/value rules engine | Adds speculative complexity and permits unsupported combinations. |

## References

- [Halaqaty Constitution](../../../../.specify/memory/constitution.md)
- [MVP Decision Register](../../../management/product/MVP_DECISION_REGISTER.md)
- [F-003 Specification](../../../../specs/003-recitation-queue-system/spec.md)
- [ADR-010](ADR-010-circle-role-management.md)
- [ADR-012](ADR-012-audit-logging-persistence.md)
- [ADR-015](ADR-015-session-media-provider-boundary.md)
- [ADR-017](ADR-017-session-recovery-and-reconciliation.md)
