# ADR-001: Modular Monolith Architecture for MVP

**Status:** Accepted  
**Date:** 2026-04-26  
**Deciders:** Karim (product owner)

---

## Context

Halaqaty needs a Go backend that supports the MVP (~50 concurrent users, single deployment) but must not become a tangle of spaghetti code that is hard to extract into microservices later if the product scales. We need to decide on the macro-architecture before any domain code is written.

The team is currently one developer (Karim) using Copilot as the primary implementation assistant. Operational complexity must be kept low. The MVP runs on a single server (Docker Compose).

---

## Decision

We will build a **modular monolith**: a single Go binary with a strict internal package structure that enforces domain boundaries at the code level.

> The Go module and all backend code live under `backend/` in the monorepo root. See ADR-007 for the full repository layout.

**Package structure (`backend/internal/`):**
```
backend/internal/
├── auth/          ← user identity, token validation
├── circles/       ← circle lifecycle, membership, roles
├── sessions/      ← live session management, LiveKit coordination
├── queue/         ← per-session queue state machine
├── chat/          ← persistent messages, voice notes
├── progress/      ← recitation records, Juz tracking
├── schedule/      ← recurring and one-off session scheduling
├── notifications/ ← push notification dispatch
└── shared/        ← cross-cutting utilities (DB, logging, config)
```

**Enforcement rules:**
1. Cross-package calls must go through a **service interface** — never import concrete types from another domain's internal sub-packages.
2. Each package owns its own DB queries. No package reaches into another's table via raw SQL.
3. Database transactions that span domains must be orchestrated by the calling package, not by a shared "god" service.
4. Entry points (`backend/cmd/api/`) compose packages together via dependency injection.

---

## Consequences

**Positive:**
- Single binary, single Dockerfile, single Docker Compose service. Simple to deploy and monitor.
- Service boundaries are enforced by code review and linters from day one — no architectural debt to repay before scaling.
- Each domain can be extracted into a separate service with minimum code change if needed. The interface contracts are already there.
- Copilot can generate a new domain package without knowledge of other packages' internals.

**Negative:**
- Teams cannot deploy domain packages independently. Not relevant in MVP with a one-person team.
- Shared DB means all domains contend on the same PostgreSQL instance. Acceptable for 50-user pilot.
- Interface overhead adds boilerplate. Mitigated by code generation.

---

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| **Microservices from day 1** | Operational complexity far exceeds team size. Network overhead would add latency to session-critical paths (queue, LiveKit token issuance). |
| **Pure flat monolith** (`internal/` as one flat package) | No domain isolation. Copilot-generated code would create implicit cross-domain coupling within weeks. |
| **Domain-Driven Design full aggregate pattern** | Too much ceremony for a 1-developer MVP. The interface-at-boundary rule achieves the same isolation with less overhead. |

---

## References

- `../ARCHITECTURE.md` — full domain model and DB schema
- `.specify/memory/constitution.md` — "Build as modular monolith" principle
