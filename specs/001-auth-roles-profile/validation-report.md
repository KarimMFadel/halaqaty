# Feature 001 Cross-Artifact Validation Report

**Feature**: Authentication, Roles, and User Profile  
**Date**: 2026-07-31  
**Artifacts analyzed**:
- `specs/001-auth-roles-profile/spec.md`
- `specs/001-auth-roles-profile/plan.md`
- `specs/001-auth-roles-profile/data-model.md`
- `specs/001-auth-roles-profile/contracts/auth-roles-profile.openapi.yaml`
- `specs/001-auth-roles-profile/tasks.md`
- `backend/migrations/000010_auth_roles_profile.up.sql`
- `backend/migrations/000011_auth_roles_profile_alignment.up.sql`
- `backend/migrations/000011_auth_roles_profile_alignment.down.sql`
- `backend/internal/middleware/auth_middleware.go`
- `backend/cmd/api/routes.go`
- `docs/contracts/openapi.yaml`

**Scope**: Verify consistency of dual-credential auth, session schema, profile fields, circle role endpoints, error envelope, audit constants, and migration alignment.

---

## Executive Summary

The Phase 1/2 foundation is largely aligned with the clarified plan and canonical REST contract. The core security model (Firebase bearer + `X-Halaqaty-Session-ID`), the 000011 alignment migration (UUID session ID, non-null expiry, `device_name`, conditional FK), and the route/middleware wiring are consistent.

There are **3 actionable drift findings** that should be resolved before `/speckit.implement`:
1. A **high-severity** contradiction between the spec (duplicate email → 409 conflict) and the plan/canonical contract (idempotent registration → returns existing session).
2. A **high-severity** implementation gap where `role_middleware.go` returns plain-text `http.Error` responses instead of the mandated JSON `ErrorEnvelope`.
3. A **medium-severity** schema drift where the `phone` profile field is present in `spec.md` and `data-model.md` but absent from both OpenAPI contracts.

The remaining findings are low/medium contract-style or documentation gaps that should be cleaned up during Phase 3 alignment.

---

## Findings

| ID | Severity | Focus Area | Location(s) | Summary | Recommendation |
|----|----------|------------|-------------|---------|----------------|
| F001 | **HIGH** | Registration idempotency | `spec.md`:FR-001, Edge Cases; `contracts/auth-roles-profile.openapi.yaml`:`/auth/register` 409; `docs/contracts/openapi.yaml`:`/auth/register` 409 | Spec says duplicate email registration is **rejected with a conflict error**; plan and canonical contract describe **idempotent registration that returns the current backend session** (200/201). The feature contract also uses a generic `409 Conflict` response instead of the canonical idempotent response. | Reconcile the spec with the canonical contract/plan: update `spec.md` FR-001 and the duplicate-email edge case to describe idempotent registration that returns the existing session; update the feature contract 409 response to match the canonical description (or remove 409 if idempotent behavior is chosen). |
| F002 | **HIGH** | Error envelope compliance | `backend/internal/middleware/role_middleware.go`:36, 42, 51, 57, 61 | `role_middleware.go` uses `http.Error` to write plain-text error responses, bypassing the standard `{error: {code, message, fields?}}` envelope. The auth middleware already uses `phttp.WriteError`. | Replace all `http.Error` calls in `role_middleware.go` with `phttp.WriteError` using the appropriate `httpconst` error code, message, and HTTP status. |
| F003 | **MEDIUM** | Profile schema drift | `spec.md`:Key Entities / Profile; `data-model.md`:User and Profile; `contracts/auth-roles-profile.openapi.yaml`:ProfileResponse, UpdateProfileRequest; `docs/contracts/openapi.yaml`:UserProfile, UpdateProfileRequest | `phone` is listed as a profile field in `spec.md` and `data-model.md`, but it is **not present** in either OpenAPI contract (RegisterRequest, UpdateProfileRequest, ProfileResponse, or UserProfile). | Either add `phone` to both OpenAPI profile schemas (RegisterRequest, UpdateProfileRequest, UserProfile/ProfileResponse) or remove `phone` from the spec/data-model if it is out of MVP scope. |
| F004 | **MEDIUM** | Register request schema drift | `contracts/auth-roles-profile.openapi.yaml`:RegisterRequest | The canonical `RegisterRequest` includes `preferred_language` (enum `[ar, en]`, default `ar`). The feature contract omits it. | Add `preferred_language` to the feature contract `RegisterRequest` so it stays a synchronized slice of the canonical contract. |
| F005 | **MEDIUM** | Circle creation response drift | `contracts/auth-roles-profile.openapi.yaml`:/circles POST 201; `docs/contracts/openapi.yaml`:/circles POST 201 | Canonical `/circles` POST returns `201` with a `Circle` body. The feature contract returns `201` with no content. | Add the `201` response content to the feature contract, referencing `#/components/schemas/Circle` (or the feature contract's equivalent), to match the canonical contract. |
| F006 | **MEDIUM** | Absolute session expiry undocumented | `backend/migrations/000011_auth_roles_profile_alignment.up.sql`:34-42; `spec.md`:FR-004; `plan.md`:Constraints; `data-model.md`:UserSession | The alignment migration sets a default absolute expiry of `NOW() + INTERVAL '90 days'`. The spec/plan only document a **30-day inactivity logout** and do not mention the absolute 90-day policy. | Document the absolute session expiry duration (90 days) in `spec.md`, `plan.md`, and `data-model.md` so the migration behavior is traceable to requirements. |
| F007 | **LOW** | Missing operationIds | `contracts/auth-roles-profile.openapi.yaml`:/auth/me GET, PUT | `GET /auth/me` and `PUT /auth/me` in the feature contract do not declare `operationId`. The canonical contract uses `getCurrentUser` and `updateCurrentUser`. | Add `operationId: getCurrentUser` and `operationId: updateCurrentUser` to the feature contract. |
| F008 | **LOW** | Error code naming convention | `docs/contracts/openapi.yaml`:ErrorResponse example | The canonical `ErrorResponse` example uses `code: FORBIDDEN`; the implementation uses `ERR_FORBIDDEN` (and the constitution example uses `ERR_CIRCLE_NOT_FOUND`). | Update the canonical example value to `ERR_FORBIDDEN` to match the `httpconst` convention, or document the canonical examples as non-binding placeholders. |
| F009 | **LOW** | Feature contract version drift | `contracts/auth-roles-profile.openapi.yaml`:info.version | Feature contract version is `0.3.0` while the canonical contract is `1.0.0`. | Align the feature contract version to `1.0.0` (as a synchronized slice) or add a comment explaining why the feature slice is versioned separately. |
| F010 | **LOW** | Single stub handler for both profile methods | `backend/cmd/api/router.go`:74-75 | `GET /auth/me` and `PUT /auth/me` are both wired to the same `authMeEndpoint` stub. This is acceptable for a stub but will need separate handlers during implementation. | During US2 implementation, split into separate GET and PUT handlers wired to the profile package. |
| F011 | **LOW** | Role middleware returns 403 for missing principal | `backend/internal/middleware/role_middleware.go`:40-43 | If the role middleware is invoked without a principal, it returns `403 Forbidden` in plain text. Since the auth middleware should run first, this path is defensive only. | When fixing F002, return `401 Unauthorized` for missing principal and `403 Forbidden` only for authenticated-but-unauthorized role checks. |
| F012 | **LOW** | Down migration data-loss caveat | `backend/migrations/000011_auth_roles_profile_alignment.down.sql`:5-13 | The down migration converts `session_id` back to `TEXT`, but existing non-UUID session IDs from before 000011 are not restored (they were replaced with `gen_random_uuid()` during up). | Accept as a documented rollback limitation, or add a note in the migration header that 000011 down is structural only and does not preserve original pre-UUID session values. |

---

## Per-Focus-Area Consistency Assessment

### 1. Dual-credential auth (Firebase bearer + `X-Halaqaty-Session-ID`)

**Status: Consistent**

- `spec.md` FR-007 and FR-003 require Firebase bearer verification and a matching backend session ID on protected routes; registration/session creation are bearer-only.
- `backend/cmd/api/routes.go` wires `RequireBearer` for `POST /auth/register` and `POST /auth/sessions`, and `Require` (bearer + session header) for `POST /auth/logout`, `GET /auth/me`, `PUT /auth/me`, and `PUT /circles/{circleId}/members/{userId}/role`.
- `backend/internal/middleware/auth_middleware.go` validates the bearer, resolves the local PostgreSQL UUID from the Firebase UID, and then validates the session ID against revocation, expiry, and user-ownership.
- Both `docs/contracts/openapi.yaml` and the feature contract declare the global security requirement as `BearerAuth + SessionId` and override the two bearer-only endpoints correctly.

### 2. User session schema (UUID session id, non-null `expires_at`, `device_name`)

**Status: Consistent after 000011**

- `000010` created `session_id TEXT`, nullable `expires_at`, and no `device_name`.
- `000011_auth_roles_profile_alignment.up.sql` converts `session_id` to `UUID` with a safe backfill, adds `device_name VARCHAR(100)`, makes `expires_at` non-null with a default, and adds supporting indexes.
- `000011.down.sql` reverses these structural changes (data type, default, nullability, column, indexes, conditional FK) without touching objects introduced by `000010`.
- `backend/internal/auth/models.go` `Session` struct matches the aligned schema: `ID string`, `UserID string`, `DeviceName *string`, `ExpiresAt time.Time`, `RevokedAt *time.Time`.
- The `auth_middleware.go` and `session_service.go` enforce both the absolute expiry and the 30-day inactivity policy.

### 3. Profile fields and required fields (`full_name`, `country` for first completion)

**Status: Mostly consistent; `phone` field drift**

- Both contracts define `full_name`, `country`, `bio`, `avatar_url`, `display_name`, and `preferred_language`.
- `RegisterRequest` requires `display_name` in both contracts.
- `UpdateProfileRequest` marks `full_name` and `country` as optional in the schema, with first-completion enforcement delegated to the service layer (per `spec.md` FR-012 and `data-model.md`).
- **Drift**: `phone` is described in the spec and data model but is absent from both OpenAPI contracts. See F003.
- **Drift**: `preferred_language` is in the canonical `RegisterRequest` but omitted from the feature contract. See F004.

### 4. Circle role schemas and role assignment endpoints (PUT vs POST, allowed roles, final-teacher protection)

**Status: Consistent**

- Both contracts define the role assignment endpoint as `PUT /circles/{circleId}/members/{userId}/role` with `operationId: updateCircleMemberRole`.
- Allowed roles are `student`, `supervisor`, `teacher` in both contracts and the migration `CHECK` constraint.
- Final-teacher protection and self-change rejection are described in `spec.md`, `plan.md`, the canonical contract, and `data-model.md`; the enforcement belongs in the transactional service layer (not the middleware).
- `routes.go` applies the role middleware with `RequireAny("supervisor", "teacher")` for the role assignment route, consistent with the spec.

### 5. Error envelope, error codes, and validation field names

**Status: Consistent in constants; implementation gap in role middleware**

- `backend/internal/platform/httpconst/error_codes.go` and `error_messages.go` define codes and messages for session-missing, session-not-found, session-expired, session-revoked, session-user-mismatch, unauthorized, forbidden, validation-failed, conflict, not-found, etc.
- `backend/internal/platform/http/errors.go` implements the standard `{error: {code, message, fields?}}` envelope.
- `auth_middleware.go` uses `phttp.WriteError` and the correct codes.
- **Gap**: `role_middleware.go` bypasses the standard envelope and writes plain text. See F002.
- **Minor**: canonical contract example uses `FORBIDDEN` without the `ERR_` prefix. See F008.

### 6. Audit event constants/actions

**Status: Consistent**

- `backend/internal/platform/logging/audit_logger.go` defines actions for `user.register`, `session.create`, `session.logout`, `profile.update`, `circle.create`, and `circle.role_change`.
- These map directly to the audit events listed in `tasks.md` T025 and the lifecycle events in `spec.md`/`plan.md`.
- Builder functions include actor ID, target user ID, circle ID, and relevant metadata.

### 7. Migration alignment (000010 → 000011, rollback)

**Status: Consistent**

- `000010` establishes the foundational tables with the pre-alignment session shape.
- `000011` is additive/standardizing only: it does not edit `000010` and introduces the UUID session ID, `device_name`, non-null expiry, new indexes, and the conditional `circle_members.circle_id → circles.id` FK.
- `000011.down.sql` drops only the objects introduced by `000011`, matching the plan's requirement.
- The conditional FK is correctly guarded by checking for the existence of the `circles` table.

---

## Conclusion

The cross-artifact analysis is **not clean**; there are actionable drift items.

Before proceeding to `/speckit.implement` (or continuing Phase 3 alignment), resolve in order:
1. **F001** — reconcile duplicate-email registration behavior between the spec and the canonical contract/plan.
2. **F002** — make `role_middleware.go` emit the standard JSON error envelope.
3. **F003** — decide whether `phone` is in the MVP profile and add it to both contracts or remove it from the spec/data model.
4. **F004** and **F005** — sync the feature contract with the canonical contract for `RegisterRequest.preferred_language` and `POST /circles` 201 response.
5. **F006** — document the 90-day absolute session expiry in the spec/plan/data-model.

After these fixes, the artifacts will be internally consistent and the implementation can proceed with confidence that the spec, plan, data model, contracts, migrations, and middleware wiring are aligned.

---

## Next Actions

- Run `/speckit.specify` or manually edit `spec.md` to resolve the registration idempotency contradiction and the `phone` field scope.
- Update `specs/001-auth-roles-profile/contracts/auth-roles-profile.openapi.yaml` to match the canonical contract for `RegisterRequest`, `POST /circles`, and `GET/PUT /auth/me` operationIds.
- Update `backend/internal/middleware/role_middleware.go` to use `phttp.WriteError` for all error responses.
- Document the 90-day absolute session expiry in `spec.md`, `plan.md`, and `data-model.md`.
- Re-run this analysis after edits to confirm the findings are closed.

## Phase 7 Hardening Evidence (T078 / T080)

**Date**: 2026-08-04  
**Branch**: `feat/hql-001-ph007-hardening-acceptance-gates`

### Unit test run (T078)

```
$ go test -short ./...

ok  github.com/KarimMFadel/halaqaty/backend/internal/auth       0.527s
ok  github.com/KarimMFadel/halaqaty/backend/internal/middleware  0.673s
ok  github.com/KarimMFadel/halaqaty/backend/internal/profile     0.805s
ok  github.com/KarimMFadel/halaqaty/backend/internal/rbac        1.017s
ok  github.com/KarimMFadel/halaqaty/backend/tests/performance    0.505s
```

All unit and performance tests pass.

### Contract test run

```
$ go test -tags=contract ./tests/contract/... -run "Auth|Profile|CircleAssignRole|ResponseSafety"

--- PASS: TestAuthCreateSessionContract
--- PASS: TestAuthLogoutContract
--- PASS: TestAuthRegisterContract
--- PASS: TestResponseSafety
--- PASS: TestAuthSessionContract
--- PASS: TestProfileValidationContract_ErrorEnvelopeAndFieldMap
--- PASS: TestProfileMeContract_RequiresBearerAndSession
--- PASS: TestCircleAssignRoleContract
ok  github.com/KarimMFadel/halaqaty/backend/tests/contract  0.487s
```

### SC-001 performance gate (T075)

`TestAuthSessionCreate_InProcessLatency_SC001` tests the login path (POST /auth/sessions)
and `TestAuthRegister_InProcessLatency` tests the registration path — both using in-process
mocks. p95 well under 2s gate. Real end-to-end SLO validation (Firebase round-trip +
PostgreSQL) requires separate live-infra load testing; this suite guards against
handler-layer hot-path regressions.

### Coverage gate (T080)

```
$ go test -short -tags=contract -coverpkg=./internal/... ./internal/... ./tests/contract/...
total: 45.3%
```

| Layer | Coverage | Notes |
|-------|----------|-------|
| Unit + contract | 45.3% | handlers, services, middleware, validation, rate limiters |
| Unit only | 24.4% | |
| Integration (DB required) | not run | repository methods, migrations, DB flows covered by T038/T052/T065 against live PostgreSQL |

**Status: PARTIAL — 80% gate not yet met.** The 80% target (constitution §VI) requires
integration tests running against a live PostgreSQL instance. `DATABASE_URL` is not set in
this environment. All non-DB paths are covered at 45.3%; the gap is exclusively in
repository/migration code that requires a real DB. Full gate verification must be run in
the CI environment with `make test-integration` before PR merge.

### Phase 7 review fixes (code-review findings)

| Finding | Fix |
|---------|-----|
| P1: `b.Loop()` requires Go 1.24 | Replaced with `for i := 0; i < b.N; i++` |
| P1: per-user rate limit bypassed at global layer | `LimitByIP` applied globally; full `Limit` applied per-route inside `requireWithUserLimit` after auth sets principal |
| P1: metrics not instrumented | `AuthMiddleware.SetMetrics` added; `RecordRequest`/`RecordRejection`/`RecordSessionExpiry` called inside `Require`; wired in `main.go` |
| P1: SC-001 gate measured register not login | Renamed to `TestAuthRegister_InProcessLatency`; added `TestAuthSessionCreate_InProcessLatency_SC001` for the session-creation (login) path |
| P2: WS plain-text error responses | `ws_rate_limit_middleware.go` now uses `phttp.WriteError` (JSON envelope) |
| P2: coverage gate marked complete at 45.3% | Status updated to PARTIAL above; gate requires live-DB integration run |

### Phase 7 tasks completed

| Task | Status | Description |
|------|--------|-------------|
| T073 | ✅ | `ErrorResponse.code` example → `ERR_FORBIDDEN`; `ValidationError.code` → `ERR_VALIDATION_FAILED`; error-codes table in `docs/contracts/README.md` |
| T074 | ✅ | `auth_metrics.go` — sync/atomic counters; wired into `AuthMiddleware.Require` via `SetMetrics`; live in `main.go` |
| T075 | ✅ | `TestAuthSessionCreate_InProcessLatency_SC001` for login path; `TestAuthRegister_InProcessLatency` for registration path |
| T076 | ✅ | Rate-limit integration tests: IP cap, user cap (after-auth ordering), LimitByIP isolation, WS conn/msg caps, 429 envelope shape |
| T077 | ✅ | `MaxBytesMiddleware` (1 MiB) in `server.go`; wired in `router.go`; `requireWithUserLimit` fixes per-user rate limit ordering |
| T078 | ✅ | Quickstart steps validated; OpenAPI manual validation passed; `go vet ./...` clean |
| T079 | ✅ | `password_storage_safety_test.go` — 5 assertions on request body, response, error envelope, stored session |
| T080 | ⚠️ | 45.3% unit+contract; 80% gate requires `DATABASE_URL` + `make test-integration`; must pass before PR merge |


- **F002 resolved**: `backend/internal/middleware/role_middleware.go` now emits the standard JSON `ErrorEnvelope` via `phttp.WriteError` / `phttp.WriteValidationError`.
- **F003 partially resolved**: `docs/contracts/openapi.yaml` updated to include `phone` in `UserProfile` and `UpdateProfileRequest`. The feature contract is generated by Spec-Kit and was not edited by hand.
- **Route method fix**: `backend/cmd/api/routes.go` `routeCircleAssignRole` changed from `POST` to `PUT` to match the canonical contract.
- **F012 documented**: The 000011 down migration is structural-only; pre-UUID session values are not restored because the up migration backfills non-UUID values with generated UUIDs.
- **F001 clarification**: The canonical contract and plan describe idempotent registration returning the existing backend session on `409`. The spec describes a "conflict error" for duplicate email. The canonical `409` response shape returns the existing session, so the implementation will follow the canonical contract.

## OpenAPI lint result (T031)

`make api-lint` was attempted from the repository root. It failed because the Spectral CLI (`spectral`) is not installed in this environment:

```text
process_begin: CreateProcess(NULL, spectral lint docs/contracts/openapi.yaml, ...) failed.
make (e=2): The system cannot find the file specified.
```

**Manual schema-reference validation performed instead**:
- Operation IDs `registerUser`, `createBackendSession`, `logoutUser`, `getCurrentUser`, `updateCurrentUser`, and `updateCircleMemberRole` are present in `docs/contracts/openapi.yaml`.
- `$ref` targets for `RegisterRequest`, `CreateBackendSessionRequest`, `BackendSessionResponse`, `UserProfile`, `UpdateProfileRequest`, `CreateCircleRequest`, and `AssignCircleRoleRequest` resolve within the canonical contract.
- Dual-credential security (`BearerAuth` + `SessionId`) is declared globally and correctly overridden to `BearerAuth` only for `POST /auth/register` and `POST /auth/sessions`.
- Profile fields include `full_name`, `display_name`, `country`, `bio`, `avatar_url`, `preferred_language`, and `phone` after the alignment update.
- Circle role schemas define roles `student`, `supervisor`, `teacher` and the role-assignment endpoint uses `PUT`.

**Recommendation**: Install/run Spectral (e.g. `npx @stoplight/spectral-cli lint docs/contracts/openapi.yaml`) before PR to satisfy the `make api-lint` gate.
