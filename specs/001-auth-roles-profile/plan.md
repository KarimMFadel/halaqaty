# Implementation Plan: Authentication, Roles, and User Profile

**Branch**: `[001-auth-roles-profile]` | **Date**: 2026-07-31 | **Spec**: [`spec.md`](./spec.md)

## Summary

Deliver Firebase-owned identity flows, backend per-device sessions, profile management, and safe multi-teacher per-circle authorization. Go never receives passwords: it verifies Firebase ID tokens and requires a matching backend session for protected routes. `docs/contracts/openapi.yaml` remains the canonical REST contract.

## Technical Context

**Language/Version**: Go 1.22+, Dart/Flutter 3.x
**Primary Dependencies**: Echo, pgx, Firebase Admin SDK, Riverpod, Dio
**Storage**: PostgreSQL (`users`, `profiles`, `user_sessions`, `circles`, `circle_members`)
**Testing**: Go unit, integration, contract; Flutter widget, integration
**Target Platform**: Linux API container; Android/iOS Flutter
**Project Type**: Mobile plus API modular monolith
**Performance Goals**: Auth/session p95 under 2s; unauthorized protected access rejected; role changes transactionally consistent
**Constraints**: Contract-first, backwards-compatible v1 API; Firebase identity boundary; dual credentials after session creation; security/reliability baseline
**Scale/Scope**: MVP, approximately 50 concurrent users and 10 live sessions

## Constitution Check

- **Spec-first**: PASS — specification and clarification checklist are complete.
- **Stack**: PASS — Go, Flutter, PostgreSQL, Firebase, and LiveKit boundaries remain unchanged.
- **Identity and authorization**: PASS — Firebase owns identity; PostgreSQL `circle_members` owns per-circle authorization.
- **Security**: PASS — Firebase/session authentication, validation, per-IP/user rate limits, audit events, and role checks are planned.
- **Reliability**: PASS — boundary timeouts, idempotent provisioning, transactional writes, request IDs, structured logs, and safe retry policy are planned.
- **Contract-first**: PASS — canonical OpenAPI is aligned; the feature contract is a synchronized feature slice.

## Project Structure

```text
backend/
├── cmd/api/
├── internal/auth/                 # Firebase verification and sessions
├── internal/profile/              # profile use cases/repository
├── internal/rbac/                 # circle role policy and repository
├── internal/middleware/           # bearer + session + role + rate limits
├── internal/platform/             # logging, metrics, HTTP constants
└── migrations/
    ├── 000010_auth_roles_profile.{up,down}.sql
    └── 000011_auth_roles_profile_alignment.{up,down}.sql

mobile/
├── lib/features/auth/
├── lib/features/profile/
├── lib/core/                      # Dio/session storage/interceptors
├── test/widget/
└── integration_test/

specs/001-auth-roles-profile/
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/auth-roles-profile.openapi.yaml
```

## Implementation and Migration Plan

1. Keep `docs/contracts/openapi.yaml` canonical. The new creation fields are additive; preserve existing v1 endpoints and response shapes.
2. Do not edit applied `000010`. Add `000011_auth_roles_profile_alignment`: backfill/standardize opaque UUID sessions, expiry, indexes, and `circle_members.circle_id → circles.id` once the parent table exists; validate rows before enforcing constraints. Its down migration reverses only objects introduced by `000011`.
3. Implement middleware in this order: bearer extraction/Firebase verification; Firebase UID-to-local user lookup; registration/session exemptions; then session-header lookup, revocation/expiry/owner match, and activity touch on all remaining protected routes.
4. Make registration and session provisioning idempotent. Implement circle creation and role updates as transactions with membership row locks, actor/target validation, final-teacher protection, audit logging, and standard error envelopes.
5. Add Flutter Firebase flows and a session-aware Dio interceptor. Store only the opaque backend session ID securely; refresh Firebase ID tokens for protected calls; clear application session state on logout or `401`.
6. Test multi-teacher creation, backup supervisor, creator-teacher fallback, invitee student role, manager permissions, self-change rejection, final-teacher protection, and missing/revoked/mismatched session IDs across Go unit/integration/contract and Flutter widget/integration suites.

## Design Outputs

- `research.md` records the decided boundaries and safety choices.
- `data-model.md` defines the implementation model and additive migration approach.
- `contracts/auth-roles-profile.openapi.yaml` is synchronized with the canonical contract.
- `quickstart.md` provides the implementation and validation sequence.

## Post-Design Constitution Check

**PASS.** ADR-010 and the amended constitution reconcile the multi-teacher model. The spec, architecture, decision register, canonical OpenAPI contract, migration approach, and this implementation plan agree.
