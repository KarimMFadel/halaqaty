# ADR-010: Multi-Teacher Circle Role Management

**Status:** Accepted  
**Date:** 2026-07-31  
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
3. Invite acceptance creates an active student membership.
4. Any active teacher or supervisor may change another member between `student`,
   `supervisor`, and `teacher`. A manager cannot change their own role, and a change
   that would leave the circle without a teacher is rejected.

## Consequences

- A circle can have multiple teachers, and every circle must retain at least one.
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

## References

- [ADR-009](ADR-009-firebase-device-sessions.md)
- [Architecture](../ARCHITECTURE.md)
- [MVP Decision Register](../../../management/product/MVP_DECISION_REGISTER.md)
- [Canonical OpenAPI contract](../../../contracts/openapi.yaml)
