# ADR-014: Single-Server Docker Compose Deployment for MVP

**Status:** Accepted  
**Date:** 2026-08-09  
**Deciders:** Karim (product owner)

## Context

The MVP targets 10–50 users and is operated by one developer. The deployment must
keep cost and operational work low while running the Go API, PostgreSQL, MinIO, and
self-hosted LiveKit required by the constitution.

## Decision

Run the MVP services with Docker Compose on one Hetzner CX22. Do not introduce
Kubernetes, Redis, or multi-region infrastructure during MVP. Move services or adopt
orchestration only when the measured triggers in `DEPLOYMENT.md` are reached.

## Consequences

- Deployment and recovery remain understandable to one operator.
- The MVP accepts a single-server failure domain and mitigates it with monitoring and
  documented backups.
- LiveKit or the application stack can move to separate hosts without changing domain
  boundaries when capacity requires it.

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| Kubernetes for MVP | Operational cost and complexity exceed the current scale need. |
| Managed multi-service cloud stack | Higher recurring cost and more vendor coupling before product validation. |
| Separate hosts from launch | Adds deployment and networking work without measured load pressure. |

## References

- [Constitution — technology constraints](../../../../.specify/memory/constitution.md#i-technology-constraints)
- [Constitution — MVP scope discipline](../../../../.specify/memory/constitution.md#vii-mvp-scope-discipline-yagni)
- [Deployment Strategy](../../deployment/DEPLOYMENT.md)
- [ADR-008 — LiveKit](ADR-008-webrtc-solution.md)
