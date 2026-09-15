# ADR-010: Multi-Teacher Circle Role Management

**Status:** Accepted  
**Date:** 2026-07-31  
**Last amended:** 2026-09-03 during F-004 Real-time Chat specification kickoff
**Deciders:** Karim (product owner)

---

## Context

Circles need to support multiple teachers and a designated backup supervisor from
creation. The previous lifecycle allowed one teacher only and prevented supervisors
from managing role assignments, which does not support the agreed circle-management
workflow.

## Decision

1. Roles remain scoped exclusively to `circle_members`; no self-registration action
   creates a circle role.
2. A creator may select existing registered users as one or more teachers and one
   optional backup supervisor during circle creation. These assignments immediately
   create active memberships. If no teacher is selected, the creator becomes a teacher;
   otherwise the creator is an active supervisor.
3. Existing invite-code/link generation, regeneration, and sharing behavior is
   retained. Invitation authority remains circle-scoped: an active teacher or
   supervisor may invite a person as either `teacher` or `student`, while an active
   student may invite a person only as `student`. The intended role is bound to the
   invitation by the backend and cannot be changed by the inviter or invitee during
   acceptance.
4. Accepting a valid invitation creates an active membership with its bound role,
   subject to the circle's archive, capacity, duplicate-membership, and membership-limit
   safeguards.
5. Any active teacher or supervisor may change another member between `student`,
   `supervisor`, and `teacher`. A manager cannot change their own role, and a change
   that would leave the circle without a teacher is rejected.

## Consequences

- A circle can have multiple teachers, and every circle must retain at least one.
- Teachers and supervisors can invite teachers or students; students can extend a
  student-only invitation. No invitation creates a global role or grants access to a
  different circle.
- Role-bound invitations prevent clients from escalating or altering the assigned role
  during acceptance.
- Role-management authorization checks both actor membership and target membership;
  cross-circle changes and self-changes are forbidden.
- The canonical OpenAPI contract, role documentation, feature contract, and tests
  must cover creation assignments, fallback teacher assignment, manager authorization,
  self-change rejection, and final-teacher protection.

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| Single creator-teacher with teacher-only role management | Does not support the agreed multiple-teacher and delegated-management workflow. |
| Global account role | Breaks the per-circle authorization invariant. |
| Allow managers to alter their own role | Can create accidental lockout and weakens role-management safeguards. |

## Amendment History

| Date | Context | Change | Approved by |
|---|---|---|---|
| 2026-09-03 | F-004 Real-time Chat specification kickoff | Retained invite-code/link generation and sharing, and extended acceptance with circle-scoped, role-bound invitations: teachers and supervisors may invite teachers or students, while students may invite students only. | Karim |

## References

- [ADR-009](ADR-009-firebase-device-sessions.md)
- [Architecture](../ARCHITECTURE.md)
- [MVP Decision Register](../../../management/product/MVP_DECISION_REGISTER.md)
- [Canonical OpenAPI contract](../../../contracts/openapi.yaml)
