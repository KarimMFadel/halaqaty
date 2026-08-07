# ADR-012: Circle Audit Events Use Structured Application Logs

**Status:** Accepted  
**Date:** 2026-08-07  
**Deciders:** Karim (product owner)

## Decision

Circle lifecycle and governance transitions emit structured audit events through
the existing `platform/logging.AuditLogger`. Events include the action, actor,
circle, target user when applicable, UTC timestamp, and redacted metadata. Invite
codes, bearer tokens, session secrets, and request bodies are never logged.

Audit emission occurs after a successful database transaction. A logging failure
does not roll back a committed business operation; the structured logger remains
the operational sink for MVP and can be shipped to a durable log store without a
domain-schema change.

## Required events

`circle.create`, `circle.join`, `circle.role_change`, `circle.invite_refresh`,
`circle.member_remove`, and `circle.archive`.

## Consequences

Audit payload builders stay centralized in `platform/logging`, and tests assert
event names and redaction. Durable queryable audit storage is deferred until
reporting requirements justify an additional schema and retention decision.

## References

- [ADR-010](ADR-010-circle-role-management.md)
- [ADR-011](ADR-011-circle-retirement.md)
