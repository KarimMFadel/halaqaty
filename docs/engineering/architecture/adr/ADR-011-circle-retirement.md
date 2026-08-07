# ADR-011: Circle Retirement Uses Archive-Only Semantics

**Status:** Accepted  
**Date:** 2026-08-07  
**Deciders:** Karim (product owner)

## Decision

Circle retirement is represented by the existing `circles.is_archived` state. No
hard-delete endpoint, repository method, SQL statement, or cascading deletion is
introduced for circles. Archived circles remain readable for retained history and
reporting, while new joins and other activity are rejected.

## Consequences

- Foreign keys and history remain intact after retirement.
- Archive operations must be teacher-authorized and idempotent.
- API documentation uses `DELETE /circles/{circleId}` only as the archive action;
  its description must explicitly prohibit hard deletion.

## References

- [ADR-010](ADR-010-circle-role-management.md)
- [Circle Management feature](../../../specs/002-circle-management/spec.md)
