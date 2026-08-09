# Architecture Documentation

System design, database schema, technical decisions, and architectural decision records (ADRs).

## Key Documents

- [`ARCHITECTURE.md`](ARCHITECTURE.md) — Technical architecture, system overview, database schema, security model
- [`adr/`](adr/) — Architecture Decision Records (one markdown file per decision, `ADR-NNN-title.md`)

---

## Architecture Decision Records (ADRs)

The `adr/` subdirectory records the key architectural decisions made for Halaqaty. Each ADR documents the context, the decision, and the alternatives considered.

**Format:** `ADR-NNN-title.md` (inside `adr/`)
**Status values:** `Proposed` | `Accepted` | `Deprecated` | `Superseded by ADR-NNN`

### Index

| ADR | Title | Status | Date |
|---|---|---|---|
| [ADR-001](adr/ADR-001-modular-monolith.md) | Modular Monolith Architecture for MVP | Accepted | 2026-04-26 |
| [ADR-002](adr/ADR-002-go-framework.md) | Go Web Framework — Echo v4 | Accepted | 2026-04-26 |
| [ADR-003](adr/ADR-003-flutter-state-management.md) | Flutter State Management — Riverpod 2.x | Accepted | 2026-04-26 |
| [ADR-004](adr/ADR-004-auth-boundary.md) | Authentication and Authorization Boundary | Accepted | 2026-04-26 |
| [ADR-005](adr/ADR-005-feature-flags.md) | Feature Flag Strategy | Accepted | 2026-04-26 |
| [ADR-006](adr/ADR-006-db-migrations.md) | Database Migration Tool — golang-migrate | Accepted | 2026-04-26 |
| [ADR-007](adr/ADR-007-monorepo-structure.md) | Monorepo Structure and CI Strategy | Accepted | 2026-04-26 |
| [ADR-008](adr/ADR-008-webrtc-solution.md) | WebRTC Solution for Live Audio Sessions — LiveKit | Accepted | 2026-07-25 |
| [ADR-009](adr/ADR-009-firebase-device-sessions.md) | Firebase Identity and Backend Device Sessions | Accepted | 2026-07-31 |
| [ADR-010](adr/ADR-010-circle-role-management.md) | Multi-Teacher Circle Role Management | Accepted | 2026-07-31 |
| [ADR-011](adr/ADR-011-circle-retirement.md) | Circle Retirement Uses Archive-Only Semantics | Accepted | 2026-08-07 |
| [ADR-012](adr/ADR-012-audit-logging-persistence.md) | Circle Audit Events Use Structured Application Logs | Accepted | 2026-08-07 |
| [ADR-013](adr/ADR-013-recitation-grade-scale.md) | Canonical Five-Grade Recitation Scale | Accepted | 2026-08-09 |
| [ADR-014](adr/ADR-014-mvp-deployment.md) | Single-Server Docker Compose Deployment for MVP | Accepted | 2026-08-09 |

---

### How to add a new ADR

1. Create `ADR-NNN-short-title.md` inside the [`adr/`](adr/) subdirectory (next sequential number).
2. Use the structure: **Status**, **Date**, **Deciders**, **Context**, **Decision**, **Consequences**, **Alternatives Considered**.
3. Add a row to the index table above.
4. Reference the ADR from the relevant section of [`ARCHITECTURE.md`](ARCHITECTURE.md) or [`../../management/product/MVP_DECISION_REGISTER.md`](../../management/product/MVP_DECISION_REGISTER.md).
5. If the ADR changes an existing decision, mark the superseded ADR as `Superseded by ADR-NNN`.

---

*For the governing principles behind these decisions, see [`../../../.specify/memory/constitution.md`](../../../.specify/memory/constitution.md). For deployment decisions, see [`../deployment/DEPLOYMENT.md`](../deployment/DEPLOYMENT.md).*
