# ADR-019: No Cascading Deletes on Student History Tables

**Status:** Accepted  
**Date:** 2026-08-23  
**Deciders:** Karim (product owner; schema/deletion policy delegated to Architect)

## Context

The `memorization_progress` table definition carried
`student_id FK → users.id NOT NULL ON DELETE CASCADE`, while the F-003
feature data model specified plain FKs. Existing repo precedent is split:
`firebase_devices` and `firebase_auth_sessions` cascade on `user_id`, but no
business/history table cascades, and ADR-011 prohibits cascading deletion for
circle retirement. The constitution freezes "full history kept" for recitation
progress and defines account deletion as archive semantics, not destruction.

## Decision

Business/history tables — `memorization_progress` now and any future
educational-history table — use plain FKs with default `NO ACTION`
(restrict) behavior. `ON DELETE CASCADE` is reserved for user-owned
auth/session artifacts (`firebase_devices`, `firebase_auth_sessions`) whose
rows are meaningless without the owner account.

Removing a student's educational history requires an explicit, audited
erasure operation (a future feature with its own ADR); it is never a
side effect of a SQL delete on `users`.

## Consequences

- A hard `DELETE FROM users` fails loudly while progress rows exist, instead
  of silently destroying years of memorization history.
- A future compliance/erasure feature must delete progress explicitly (and
  decide its audit-trail interaction with ADR-012) before removing the user.
- No MVP behavior changes: MVP has no user hard-delete endpoint, and account
  deletion uses archive semantics.

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| Keep `ON DELETE CASCADE` on `student_id` | Silent mass-destruction path on an educational-history table; contradicts ADR-011's no-cascade principle and constitution "full history kept". |
| `ON DELETE SET NULL` | Breaks the NOT NULL provenance invariant; orphan history with no student is useless for F-007 analytics. |
| Soft-delete flag on progress now | YAGNI — no deletion flow exists in MVP; add when an erasure feature is specified. |

## References

- [ADR-011 — Circle Retirement Uses Archive-Only Semantics](ADR-011-circle-retirement.md)
- [ADR-012 — Circle Audit Events Use Structured Application Logs](ADR-012-audit-logging-persistence.md)
- [Constitution — frozen MVP business rules ("Full history kept")](../../../../.specify/memory/constitution.md)
- [Architecture — `memorization_progress`](../ARCHITECTURE.md#memorization_progress)
