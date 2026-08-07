# Implementation Plan: Circle Management

**Branch**: `[002-circle-management]` | **Date**: 2026-08-07 | **Spec**: [`spec.md`](./spec.md)

## Summary

Extend the existing Feature 001 circle foundation into a complete Circle Management slice: transactional creation and role assignment, public discovery and joining, invite-link joining and rotation, member/role management, and archive retirement. Reuse Firebase/backend-session authentication, PostgreSQL `circle_members` authorization, existing REST conventions, and Flutter Riverpod patterns. No circle hard deletion is designed or implemented.

## Technical Context

**Language/Version**: Go 1.22+, Dart/Flutter 3.x  
**Primary Dependencies**: Echo, pgx, Firebase Admin SDK, Riverpod, Dio  
**Storage**: PostgreSQL (`users`, `circles`, `circle_members`, audit records)  
**Testing**: Go unit, integration, contract; Flutter widget/integration  
**Target Platform**: Linux API container; Android/iOS Flutter  
**Project Type**: Mobile plus API modular monolith  
**Performance Goals**: Circle reads p95 under 500ms at MVP scale; public discovery p95 under 1s; mutation responses under 2s; no duplicate membership under concurrent joins  
**Constraints**: Contract-first, additive v1 API changes, dual credentials on protected routes, Arabic-first RTL, no hard deletion, no queue/session/chat implementation  
**Scale/Scope**: MVP, approximately 50 concurrent users and 10 live sessions; optimize only measured query bottlenecks

## Constitution Check

*GATE: Must pass before implementation planning is accepted.*

- **Spec-first**: PASS — Feature 002 spec and checklist exist; three source-document alignment actions are included below.
- **Stack**: PASS — Go, Flutter, PostgreSQL, Firebase, and existing LiveKit boundary remain unchanged; LiveKit is out of scope.
- **Identity and authorization**: PASS — Firebase verifies identity; backend sessions authenticate protected calls; `circle_members` owns per-circle authorization.
- **Security**: PASS — server validation, role checks, rate limits, audit events, request IDs, safe error responses, and hard-delete prohibition are planned.
- **Reliability**: PASS — transactions, row locks, uniqueness constraints, idempotency, timeouts, structured logs, and safe retry policy are planned.
- **Contract-first**: CONDITIONAL — feature contract is supplied; canonical OpenAPI and stale product/journey wording must be synchronized before implementation.

## Existing baseline

- Reuse migrations `000013_create_circles` and `000014_circle_members_circle_fk`; do not edit applied migrations.
- Add the next sequential migration for missing F-002 fields and constraints.
- Reuse `backend/internal/middleware`, `backend/internal/rbac`, centralized HTTP constants, route constants, and package-level SQL query files.
- Add `backend/internal/circle` (or the repository's established equivalent after checking current ownership) only if no existing circle package is present; keep one implementation and avoid speculative interfaces.
- Add `mobile/lib/features/circles/` with Riverpod providers/notifiers and Dio client methods; reuse existing auth/session interceptors and UI primitives.

## Phase 0 — Canonical alignment gate

1. Amend `docs/management/product/FEATURES.md` F-002 so “delete” means archive/retirement and explicitly prohibits hard deletion.
2. Amend `docs/management/product/JOURNEY.md` T-05 to reflect the approved public/private behavior: public circles are discoverable/joinable and all circles retain invite links; private circles are invite-only.
3. Add the retirement decision to the required ADR/amendment trail, preserving ADR-010's role decision.
4. Align product docs, architecture, schema, and contracts on circle gender values `male`, `female`, `mixed`, and `unspecified`, defaulting to `unspecified`; keep this independent of personal gender.
5. Create and accept `docs/engineering/architecture/adr/ADR-012-audit-logging-persistence.md` to define audit sink/schema, retention, redaction, transaction/failure behavior, indexing, and access policy before audit implementation.
6. Synchronize `docs/contracts/openapi.yaml` with `contracts/circle-management.openapi.yaml`, including public discovery/direct join, redacted public summaries, invite refresh, the 8-character code pattern, and archive-only `DELETE /circles/{circleId}`.
7. Run `$docs-guard` or apply its checklist manually, then `make api-lint`.

## Phase 1 — Database and domain invariants

1. Add an additive paired migration after the current repository head for description, rules, capacity, privacy, language, gender, and any required indexes/defaults.
2. Preserve existing rows and validate/backfill values before constraints; provide a rollback that removes only new objects and does not touch unrelated Feature 001 data.
3. Implement repository queries in `*_queries.go`: list own circles, public discovery, read details, create, update, archive, join by public/direct or invite code, refresh invite, list/remove members, and update role.
4. Implement transactional service methods for creation, join, role mutation, invite rotation, membership limit, capacity, archive state, and final-teacher protection.
5. Implement the accepted ADR-012 audit design for create, join, role change, invite refresh, member removal, and archive; never emit or implement hard-delete SQL.

## Phase 2 — Backend API and contract

1. Centralize route patterns in `backend/cmd/api/routes.go` and wire handlers through the existing router/middleware stack.
2. Apply Firebase bearer plus `X-Halaqaty-Session-ID` protection to all protected circle endpoints; public discovery still requires authenticated backend session unless the final product decision explicitly allows anonymous browsing.
3. Return the standard `{ "error": { "code", "message", "fields?" } }` envelope and documented `400/401/403/404/409` responses.
4. Enforce per-IP/user rate limits, request timeouts, request IDs, structured logs, and safe retry semantics at the existing platform boundaries.
5. Synchronize feature and canonical OpenAPI contracts; validate operation IDs, references, response bodies, code pattern, archive semantics, and backward compatibility.

## Phase 3 — Flutter circle experience

1. Add Riverpod state/providers for the authenticated user's circles, public discovery, circle details/members, create/edit, invite sharing/refresh, join confirmation, role management, and archived read-only state.
2. Reuse Dio/session behavior from Feature 001; surface standard validation/auth/conflict errors at the nearest UI boundary.
3. Implement Arabic-first, RTL-aware layouts and accessibility labels for create, discover, join, member, invite, role, and archive flows.
4. Ensure public cards never show private/member data and archived circles cannot expose active mutation controls.

## Phase 4 — Verification and review

1. Go unit tests: validation, invite format/rotation, role policy, membership limit, capacity, archive state, and final-teacher invariant.
2. Go contract tests: all Feature 002 operations, standard error envelopes, backward-compatible existing circle routes, and hard-delete absence.
3. Go integration tests: fresh/upgrade/rollback migrations, concurrent joins, concurrent role changes, invite refresh race, archive behavior, and audit events.
4. Flutter widget/integration tests: create, public discovery, invite join, member list, role denial, invite refresh, archive read-only state, RTL/error rendering.
5. Run `$clean-code-guard`, `$test-guard`, `$docs-guard`, focused suites, then full applicable repository gates.
6. Send one coherent batch to Tech Lead review; Karim performs the required manual review for RBAC and data-retention/deletion safety.

## Design outputs

- [`research.md`](./research.md) — decisions and alternatives.
- [`data-model.md`](./data-model.md) — schema/invariants/migration approach.
- [`contracts/circle-management.openapi.yaml`](./contracts/circle-management.openapi.yaml) — feature contract slice.
- [`quickstart.md`](./quickstart.md) — implementation and validation sequence.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Concurrent joins exceed capacity or five-circle limit | Transaction plus row/advisory locking and database uniqueness; integration race tests |
| Role mutation creates teacherless circle | Lock membership set, validate resulting teacher count, mutate atomically |
| Old invite remains usable after refresh | Single transaction/code uniqueness and immediate old-code invalidation |
| Public discovery leaks private data | Separate public summary projection and contract tests for field omission |
| Archive accidentally deletes history | No hard-delete route/query/cascade; retention tests and manual Karim review |
| Existing clients break | Additive endpoints/fields, preserve existing operation IDs and response shapes, contract diff review |

## Post-design constitution check

**CONDITIONAL PASS.** The technical design follows the constitution and harness. Implementation is gated on canonical product/ADR/OpenAPI alignment for retirement, public discovery, and invite refresh.
