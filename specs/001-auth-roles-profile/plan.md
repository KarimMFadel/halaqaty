# Implementation Plan: Authentication, Roles, and User Profile

**Branch**: `[001-auth-roles-profile]` | **Date**: 2026-07-25 | **Spec**: [`specs/001-auth-roles-profile/spec.md`](./spec.md)  
**Input**: Feature specification from `/specs/001-auth-roles-profile/spec.md`

## Summary

Deliver secure email/password authentication via Firebase Auth, backend token verification and 30-day inactivity session enforcement, profile create/read/update, and per-circle role authorization based on PostgreSQL `circle_members`. Keep API contract-first with backward-compatible changes in `docs/contracts`.

## Technical Context

**Language/Version**: Go 1.22+ (backend), Dart/Flutter 3.x (mobile)  
**Primary Dependencies**: Echo, pgx, Firebase Admin SDK, Riverpod (Flutter), http/dio client  
**Storage**: PostgreSQL (users, profiles, circle_members, session activity)  
**Testing**: Go unit + integration + contract tests; Flutter widget + integration tests  
**Target Platform**: Linux container backend, Android/iOS Flutter clients  
**Project Type**: Mobile + API modular monolith  
**Performance Goals**: Login p95 under 2s; 100% rejection for unauthorized protected access  
**Constraints**: Contract-first in `docs/contracts`; backward-compatible API changes; security and reliability baselines mandatory  
**Scale/Scope**: MVP scale (~50 concurrent users, <=10 simultaneous live sessions)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Spec-first gate**: PASS (spec exists at `specs/001-auth-roles-profile/spec.md`).
- **Stack gate**: PASS (Go + Flutter + PostgreSQL + LiveKit + Firebase preserved).
- **Identity gate**: PASS (Firebase handles token issuance and verification).
- **Authorization gate**: PASS (authorization derived from PostgreSQL `circle_members`).
- **Security invariants gate**: PASS with planned controls:
  - Authentication on protected routes with Firebase token verification
  - Per-circle authorization checks on restricted endpoints
  - Server-side validation for registration/profile/authorization inputs
  - Rate limits per IP and per user, plus WebSocket connection/message limits
  - Audit logs for auth/profile/role changes
- **Reliability gate**: PASS with planned controls:
  - Timeouts on HTTP/DB/Firebase boundaries
  - Retry policy only for idempotent external calls
  - Idempotency keys for registration and profile update writes
  - Observability via request IDs, metrics, and structured logs
- **Contract-first gate**: PASS (feature contracts in `specs/001-auth-roles-profile/contracts/`, then merged into `docs/contracts/` before implementation).

## Project Structure

### Documentation (this feature)

```text
specs/001-auth-roles-profile/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── auth-roles-profile.openapi.yaml
└── tasks.md
```

### Source Code (repository root)

```text
backend/
├── cmd/api/router.go
├── internal/auth/
│   ├── service.go
│   ├── handler.go
│   └── firebase_verifier.go
├── internal/profile/
│   ├── service.go
│   ├── handler.go
│   └── repository.go
├── internal/rbac/
│   ├── service.go
│   └── handler.go
├── internal/middleware/
│   ├── auth_middleware.go
│   ├── role_middleware.go
│   └── rate_limit_middleware.go
├── internal/platform/
│   ├── logging/audit_logger.go
│   └── metrics/auth_metrics.go
└── migrations/
    ├── 00000x_auth_roles_profile.up.sql
    └── 00000x_auth_roles_profile.down.sql

mobile/
├── lib/features/auth/
├── lib/features/profile/
├── lib/features/admin/
├── test/widget/
└── integration_test/

docs/
└── contracts/
    └── openapi.yaml
```

**Structure Decision**: Keep a single backend service and Flutter app, with clear auth/profile/rbac module separation, contract-first docs, and constitution-aligned identity/authorization boundaries.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| None | N/A | N/A |

## Phase 0 Research Output

- Completed: `specs/001-auth-roles-profile/research.md`
- Clarifications resolved in spec.

## Phase 1 Design Output

- Completed: `specs/001-auth-roles-profile/data-model.md`
- Completed: `specs/001-auth-roles-profile/contracts/auth-roles-profile.openapi.yaml`
- Completed: `specs/001-auth-roles-profile/quickstart.md`
- Agent context updated in `.github/copilot-instructions.md`

## Post-Design Constitution Check

- **Re-check result**: PASS
- Plan remains aligned to Firebase identity model, per-circle authorization model, sequential migration policy, and rate-limit invariants.
