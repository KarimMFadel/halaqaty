# Architecture Decision Records (ADRs)

This directory records the key architectural decisions made for Halaqaty. Each ADR documents the context, the decision, and the alternatives considered.

**Format:** `ADR-NNN-title.md`  
**Status values:** `Proposed` | `Accepted` | `Deprecated` | `Superseded by ADR-NNN`

---

## Index

| ADR | Title | Status | Date |
|---|---|---|---|
| [ADR-001](ADR-001-modular-monolith.md) | Modular Monolith Architecture for MVP | Accepted | 2026-04-26 |
| [ADR-002](ADR-002-go-framework.md) | Go Web Framework — Echo v4 | Accepted | 2026-04-26 |
| [ADR-003](ADR-003-flutter-state-management.md) | Flutter State Management — Riverpod 2.x | Accepted | 2026-04-26 |
| [ADR-004](ADR-004-auth-boundary.md) | Authentication and Authorization Boundary | Accepted | 2026-04-26 |
| [ADR-005](ADR-005-feature-flags.md) | Feature Flag Strategy | Accepted | 2026-04-26 |
| [ADR-006](ADR-006-db-migrations.md) | Database Migration Tool — golang-migrate | Accepted | 2026-04-26 |
| [ADR-007](ADR-007-monorepo-structure.md) | Monorepo Structure and CI Strategy | Accepted | 2026-04-26 |

---

## How to add a new ADR

1. Create `ADR-NNN-short-title.md` in this directory (next sequential number).
2. Use the structure: **Status**, **Date**, **Deciders**, **Context**, **Decision**, **Consequences**, **Alternatives Considered**.
3. Add a row to the index table above.
4. Reference the ADR from the relevant section of `../ARCHITECTURE.md` or `../../management/product/MVP_DECISION_REGISTER.md`.
5. If the ADR changes an existing decision, mark the superseded ADR as `Superseded by ADR-NNN`.
