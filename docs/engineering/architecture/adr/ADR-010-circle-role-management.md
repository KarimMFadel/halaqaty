# ADR-010: Multi-Teacher Circle Role Management

**Status:** Accepted  
**Date:** 2026-07-31  
**Last amended:** 2026-11-22 following F-002 teacher-consent clarification
**Deciders:** Karim (product owner)

---

## Context

Circles need a required teacher, optional shared teaching, and delegated management.
The previous creation flow could make another user an active teacher without that
person accepting the responsibility. Named, role-bound invitations are not yet
implemented and must be designed in the reopened F-002 plan.

## Decision

1. Roles remain scoped exclusively to `circle_members`; no self-registration action
   creates a circle role.
2. The creator initially accepts `teacher` responsibility by creating the circle.
   After another named person accepts a teacher invitation, an authorized other
   manager may change the creator to supervisor (organizer); self-role changes remain
   prohibited. Every active circle must retain at least one accepted teacher; a pending
   invitation does not satisfy this requirement. Additional teachers are optional and
   have the ordinary `teacher` role.
3. Supervisors and backups are optional and use ordinary `supervisor` or `teacher`
   roles; no separate primary or backup permission role is introduced.
4. Assigning another person as `teacher`, including promoting an existing member,
   requires a named, role-bound invitation and that recipient's acceptance. A pending
   invitation creates no new membership or teacher permissions; an existing member
   retains their prior role until acceptance. Acceptance creates a
   teacher membership or promotes that recipient's existing membership to `teacher`,
   subject to archive, capacity, duplicate, and membership-limit safeguards.
5. Invitation authority remains circle-scoped: an active teacher or supervisor may
   invite a person as `teacher` or `student`; an active student may invite a person only
   as `student`. The intended role is bound by the backend and cannot be changed by the
   inviter or recipient during acceptance. Reusable circle invite-code behavior remains
   student-only.
6. Any active teacher or supervisor may change another member between `student` and
   `supervisor`. Assigning `teacher` requires invitation acceptance. A manager cannot
   change their own role, and a change that would leave the circle without a teacher is
   rejected.
7. Named invitations have no expiry. An active teacher or supervisor may revoke a
   pending invitation; a new invitation to the same recipient in the same circle
   replaces the prior pending invitation. Each invitation has an immutable role and
   is accepted at most once by its named recipient. Acceptance atomically rechecks
   archive state, current authority, capacity and membership eligibility.

### Invitation persistence

Add `circle_invitations` in a new paired migration. Each row has UUID `id`,
`circle_id`, `inviter_user_id`, `recipient_user_id`, immutable `role` (`teacher`
or `student`), `state` (`pending`, `accepted`, `revoked`, or `superseded`),
`created_at`, and nullable `resolved_at`. Foreign keys reference existing circles
and users; a partial unique index on `(circle_id, recipient_user_id)` where state
is `pending` enforces at most one outstanding invitation. State and resolved time
must agree. No expiration column or new global role is introduced.

All send/replace, revoke, and acceptance operations serialize with circle
membership mutations. Replacement changes the prior pending row to superseded
before inserting the new row in the same transaction; notification intent creation
shares that transaction. Acceptance locks the invitation, verifies the named
recipient and pending state, rechecks the inviter's current authority, circle and
membership eligibility, and writes the membership or promotion and accepted state
atomically. A stale, revoked, superseded or already accepted invitation grants no
new access. Archives and removals cannot race past these checks.

## Consequences

- The creator is initially teacher and can become supervisor after another named
  teacher accepts responsibility and an authorized other manager changes that role.
  Every active circle must retain at least one accepted teacher.
- Additional teachers are optional and accept before the role takes effect. Managers
  can add or replace teachers later, but cannot demote/remove the final teacher.
- Teachers and supervisors can invite teachers or students; students can extend a
  student-only invitation. No invitation creates a global role or grants access to a
  different circle.
- Role-bound invitations prevent clients from escalating or altering the assigned role
  during acceptance; teacher consent is explicit rather than inferred from circle
  creation or a reusable join code.
- Role-management authorization checks both actor membership and target membership;
  cross-circle changes and self-changes are forbidden.
- F-002 must add the named invitation lifecycle and define its REST contract, persistence,
  notification handoff, and acceptance behavior before implementation. The canonical
  OpenAPI contract, role documentation, feature contract, and tests must cover creator role selection, accepted teacher responsibility, invitation acceptance, manager authorization, self-change
  rejection, and final-teacher protection.

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| Single creator-teacher with teacher-only role management | Does not support the agreed multiple-teacher and delegated-management workflow. |
| Directly assign a selected teacher during circle creation | Makes another person responsible for a circle without their acceptance. |
| Global account role | Breaks the per-circle authorization invariant. |
| Allow managers to alter their own role | Can create accidental lockout and weakens role-management safeguards. |

## Amendment History

| Date | Context | Change | Approved by |
|---|---|---|---|
| 2026-09-03 | F-004 Real-time Chat specification kickoff | Retained invite-code/link generation and sharing, and extended acceptance with circle-scoped, role-bound invitations: teachers and supervisors may invite teachers or students, while students may invite students only. | Karim |
| 2026-11-22 | F-002 teacher-consent clarification | Creator defaults to teacher absent another accepted teacher; the creator may instead be supervisor (organizer) when a named person accepts responsibility. Every active circle retains an accepted teacher. Assignment and promotion require acceptance; additional teachers and supervisors are optional ordinary roles. | Karim |
| 2026-11-22 | Named invitation lifecycle | No expiry; active teachers or supervisors may revoke pending invitations, and a new invitation to the same circle and recipient replaces the previous pending invitation. Acceptance is single-use and rechecks eligibility. | Karim |

## References

- [ADR-009](ADR-009-firebase-device-sessions.md)
- [Architecture](../ARCHITECTURE.md)
- [MVP Decision Register](../../../management/product/MVP_DECISION_REGISTER.md)
- [Canonical OpenAPI contract](../../../contracts/openapi.yaml)
