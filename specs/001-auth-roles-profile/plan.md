# Implementation Plan: Authentication, Roles, and User Profile

**Branch**: `[001-auth-roles-profile]` | **Date**: 2026-08-04 | **Spec**: [`spec.md`](./spec.md)

## Summary

Deliver Firebase-owned identity flows, backend per-device sessions, profile management, and safe multi-teacher per-circle authorization. Go never receives passwords: it verifies Firebase ID tokens and requires a matching backend session for protected routes. `docs/contracts/openapi.yaml` remains the canonical REST contract. This planning update resolves one feature-contract mismatch: `UpdateProfileRequest.display_name` now enforces `minLength: 2` and `maxLength: 100`.

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

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Spec-first**: PASS — specification and clarification checklist are complete.
- **Stack**: PASS — Go, Flutter, PostgreSQL, Firebase, and LiveKit boundaries remain unchanged.
- **Identity and authorization**: PASS — Firebase owns identity; PostgreSQL `circle_members` owns per-circle authorization.
- **Security**: PASS — Firebase/session authentication, validation, per-IP/user rate limits, audit events, and role checks are planned.
- **Reliability**: PASS — boundary timeouts, idempotent provisioning, transactional writes, request IDs, structured logs, and safe retry policy are planned.
- **Contract-first**: PASS — canonical OpenAPI is aligned; the feature contract is a synchronized feature slice with corrected profile validation.

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
5. Align profile validation contracts so `display_name` is consistently constrained to 2..100 characters across registration and profile update payloads.
6. Add Flutter Firebase flows and a session-aware Dio interceptor. Store only the opaque backend session ID securely; refresh Firebase ID tokens for protected calls; clear application session state on logout or `401`.
7. Test multi-teacher creation, backup supervisor, creator-teacher fallback, invitee student role, manager permissions, self-change rejection, final-teacher protection, and missing/revoked/mismatched session IDs across Go unit/integration/contract and Flutter widget/integration suites.

## Design Outputs

- `research.md` records the decided boundaries and safety choices.
- `data-model.md` defines the implementation model and additive migration approach.
- `contracts/auth-roles-profile.openapi.yaml` is synchronized with the canonical contract.
- `quickstart.md` provides the implementation and validation sequence.

## Post-Design Constitution Check

**PASS.** ADR-010 and the amended constitution reconcile the multi-teacher model. The spec, architecture, decision register, canonical OpenAPI contract, migration approach, and this implementation plan agree.

## Account-deletion amendment (2026-09-26)

The original plan above remains the record for US1–US3. This additive design implements US4/FR-014–FR-021, with student deletion first. Teacher deletion must return a conflict without mutation until F-008 can deliver member notifications and every owned active circle has a designated supervisor; no teacher archive/notification behavior is enabled in this batch.

### Boundaries and sequence

1. Flutter asks for explicit irreversible confirmation after Firebase SDK reauthentication. `DELETE /auth/me` sends `{ "confirm": true }` with the current Firebase bearer and matching backend session. The API reads the verified Go SDK token's `AuthTime` field (`auth_time` claim), requires it to be no more than five minutes old (and not in the future), and never substitutes token `IssuedAt` or a client-provided timestamp.
2. Before mutation, reject a user with any active teacher or supervisor membership or owned active circle; this batch supports student-only deletion. Also reject a user with an active participant record in an active live session. Account closure and session admission serialize on the user row so no media credential can be issued after closure commits. The future F-008-dependent manager path will lock all affected circles, check supervisors, archive atomically, and enqueue durable member notifications in that same transaction; it is not implemented now.
3. Lock the local user row. In one PostgreSQL transaction, set `users.deleted_at`, null email, scrub non-retained profile fields while keeping only `profiles.display_name`, delete all `user_sessions` rows (invalidating every session ID and erasing device/session metadata), and delete device tokens if present. Keep referenced `users.id`, historical rows, and circle memberships as provenance. While Firebase cleanup is pending, keep `users.firebase_uid` only as the worker's retry key. Every authenticated read/write, registration, and backend-session creation must reject `deleted_at IS NOT NULL` before creating any session or returning personal data. Session creation must serialize with closure through the same user-row lock or an atomic `INSERT ... SELECT ... WHERE deleted_at IS NULL` in its transaction, so a pre-closure read cannot insert a post-closure session.
4. Commit the closure before contacting Firebase. The backend Admin SDK calls `DeleteUser(uid)` with a bounded timeout. Success or `IsUserNotFound` means cleanup is complete; then clear `firebase_uid` in the tombstone. On timeout/failure, return HTTP 202 with `status: pending_identity_removal`, clear mobile credentials, and let a backend worker retry rows where `deleted_at IS NOT NULL AND firebase_uid IS NOT NULL`. No client retry is needed or possible after access closure. Once cleanup succeeds in-request, return 204; never return 204 while Firebase cleanup is pending.
5. `/auth/register` and `/auth/sessions` use Firebase's revocation-aware token verification before provisioning. This prevents an old, still cryptographically valid ID token from creating a new local account after Firebase user deletion and `firebase_uid` scrubbing. Pending tombstones also reject their retained UID. Firebase network failure on these entry points fails closed. Normal protected requests additionally reject deleted local users regardless of Firebase state.

### Persistence and migration

- Add a new sequential migration after the current maximum migration version; never modify applied migrations. `users.deleted_at TIMESTAMPTZ NULL` marks irreversible closure, and `users.firebase_uid` and `users.email` become nullable while retaining their existing uniqueness for active values. No new table, queue, or provider abstraction is needed. `deleted_at` plus retained pending UID is the durable retry queue. Add a partial index for pending cleanup only if query evidence needs it at MVP scale.
- `profiles` remains for the historical display name; null `full_name`, `country`, `phone`, `bio`, `avatar_url`, `completed_at`, and reset `preferred_language` to its non-identifying default. Scrub any avatar object or device token that the actual schema/storage audit finds; never claim external media removal solely from nulling a URL.
- Rollback may remove the new schema only before any deletion has occurred. After closure, a down migration must refuse to reimpose `NOT NULL` on tombstone rows or restore erased personal data. Do not re-enable a deleted account.

### Contract and verification

- Replace the canonical OpenAPI's false hard-delete wording. Document 204 complete, 202 pending identity cleanup, 400 missing confirmation, 401 missing/stale reauthentication or credential, 409 manager deletion blocked, and 503 closure/storage failure. Keep the feature contract synchronized.
- Test transaction rollback before closure, concurrent deletion/session creation, zero surviving session rows and rejection of every old session ID, rejection at every auth entry point, five-minute boundary and forged clock claims, Firebase timeout and retry, Firebase user-not-found replay, and history/display-name retention without private profile fields. Add a Flutter journey for explicit confirmation, fresh reauthentication, pending/completed result, and credential clearing. Run full gates and Karim's manual deletion/security review before commit/merge.

**Design gate:** ADR, architecture, feature contract, canonical OpenAPI, and the approved deletion decisions are aligned. The student-only tasks are generated below Phase 8 and mapped to US4/FR-014–FR-021. This does not mark implementation complete or reopen original US1–US3 tasks.
