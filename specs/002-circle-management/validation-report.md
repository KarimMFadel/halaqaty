# Circle Management Validation Report

Date: 2026-08-14 (updated: second pass)

## Current acceptance status

Completed in the first pass: T064, T069, T071, T072, and T073.

Completed in this pass: T063, T066, and T070 — verified against a live PostgreSQL 16 (Docker) with fresh per-test schemas. T066 additionally gained service-level unit coverage (`TestArchiveCircle_TeacherArchivesIdempotently`, `TestArchiveCircle_NonTeacherIsForbidden`).

Still open: T055, T061, T065, T068, T077, and T078. T061 and T068 are implemented but cannot be accepted until the mandatory Flutter gates run (`flutter`/`dart` are not installed in this environment). T055 and T065 also carry the Test Guard boundary finding below. T077 records fresh gate output but stays open until the Flutter/Spectral/gitleaks gates can execute. T078 requires Karim's manual RBAC/data-retention approval.

## Clean Code Guard — T074

Scope: `backend/internal/rbac/` and `mobile/lib/features/circles/`.

Resolved findings:

- Invite refresh now authorizes teachers only in both the service and production router.
- Invite refresh and member removal now lock the circle row before memberships, matching archive lock order.
- Missing circle credentials now use a typed `StateError` and are handled inside Flutter mutation boundaries; Firebase failures remain typed `FirebaseAuthException` handling.
- The update request now distinguishes omitted nullable fields from explicit JSON `null`, so description/rules can be cleared as documented.
- Management and retirement screens are reachable through role-aware detail-screen navigation.
- The circle-members foreign key now uses `ON DELETE RESTRICT`; T064 scans all three circle feature migrations so cascade deletion cannot hide behind an incomplete test scope.

Non-blocking reduction: the obsolete archive-race wrapper was deleted after row locking became the real synchronization mechanism. The separate one-method retirement controller still duplicates part of the management mutation pipeline and is a future simplification candidate.

Result: no unresolved blocking Clean Code Guard finding in the changed production code. Mechanical Flutter analysis remains blocked under T077.

## Test Guard — T075

Scope: changed Go and Dart tests.

Accepted test architecture:

- Database retention, audit, and concurrency coverage uses the real PostgreSQL repository and fresh migrated schemas.
- HTTP contracts exercise real handlers/middleware and observable response envelopes.
- The hard-delete safety test intentionally inspects routes/repository symbols/SQL because T064 explicitly requires proving their absence.
- Duplicate invite contract branching and the obsolete archive race wrapper were removed.

Blocking findings:

- `mobile/integration_test/circle_role_invite_flow_test.dart` uses an in-memory API boundary and direct screen construction. It is useful component-flow coverage but does not satisfy T055's production navigation/auth/RBAC acceptance boundary.
- `mobile/integration_test/circle_retirement_flow_test.dart` similarly uses an in-memory API boundary. It does not replace a configured backend/device integration run for T065.
- Flutter unit/widget/integration tests, analyzer, and formatter cannot run because neither `flutter` nor `dart` is installed or available on PATH.
- PostgreSQL-backed T063/T066/T070 tests compile but skip because `DATABASE_URL` is unset.

Second pass: T063, T066, and T070 closed — PostgreSQL-backed tests executed green against a live database (see T077 evidence). New tests added this pass follow the same accepted architecture: service unit tests use the existing stub store; the malformed-ID regression test uses the real PostgreSQL repository so the `$1::uuid` cast path is genuinely exercised.

Result: Test Guard executed; T055 and T065 remain open (Flutter environment unavailable; boundary findings above stand).

## Docs Guard — T076

Verified the canonical OpenAPI paths, operation IDs, security schemes, request fields, and local references against the Go routes, handlers, and models. The canonical contract was corrected to include nullable update fields, the shared conflict response, and archived-mutation conflict responses.

Unresolved generated-contract drift in `specs/002-circle-management/contracts/circle-management.openapi.yaml`:

- `UpdateCircleRequest` omits `grading_policy` although the Go and canonical models support it.
- Several archived mutation paths omit their `409 Conflict` response; archive also omits its possible `404 Not Found` response.

The feature-local file is Spec-Kit generated and was not hand-edited. Regenerate/synchronize it through the Spec-Kit workflow.

Spectral could not run because the `spectral` executable is not installed. The dedicated Go OpenAPI contract test passed for local references, required operation IDs, uniqueness, and bearer/session security.

## Verification evidence — T077

Fresh output, second pass (2026-08-14, PostgreSQL 16 via Docker container `halaqaty-pg`, `DATABASE_URL=postgres://postgres:postgres@localhost:5432/halaqaty?sslmode=disable`):

Passed:

- `go test -short ./... -count=1` — all packages ok, zero FAIL
- `go test -tags=contract ./tests/contract -count=1` — ok (includes updated retirement contract with empty-204-body assertion)
- `go test -tags=integration ./... -count=1` — all packages ok against live PostgreSQL, zero FAIL; includes `TestCircleRetirement_RetainsHistoryAndBlocksMutations` (T063), `TestCircleAudit_RecordsCompleteMutationLifecycle` (T070), `TestCircleManagementMigration_PreservesLegacyRowsAndRollsBack` with FK RESTRICT + hard-delete rejection (T064), and new `TestCircleMutations_RejectMalformedIDs`
- `go vet ./...` — clean
- `golangci-lint run ./...` — zero violations
- Focused: `go test -short -run ArchiveCircle ./internal/rbac -count=1 -v` — 2/2 PASS

Blocked or not accepted:

- Flutter tests/integration/analyze/format: `flutter` and `dart` executables are not installed in this environment. Per AGENTS.md, T061/T068 and the Flutter portions of T055/T065 cannot be marked complete, and no commit of Flutter changes may proceed, until these gates run with fresh successful output.
- Spectral: executable unavailable. Mitigation unchanged: the Go OpenAPI contract test (T072) passes for local references, required operation IDs, uniqueness, and bearer/session security.
- Gitleaks: executable unavailable.
- Full `gofmt -l .`: confirmed environmental — `core.autocrlf=true` checks out CRLF, so `gofmt -d` shows line-ending-only diffs on every repo file, including untouched feature-001 files. Not introduced by this batch.

T077 remains open because the Flutter, Spectral, and gitleaks gates cannot execute in this environment.

## Tech Lead review — T078

The first 2026-08-14 Tech Lead review found teacher-only invite authorization, lock ordering, production navigation, credential-error handling, T055 boundary quality, and T051 retained-history evidence gaps. Those were addressed in the first pass.

Second-pass Tech Lead review (2026-08-14, final MVP batch T062–T073): **approve-with-comments**. All prior findings confirmed closed. Security sign-offs: RBAC paths PASS, archive/data-retention PASS, audit logging PASS.

New non-blocking findings and disposition:

1. Malformed `circleId`/`userId` path params reached the `$1::uuid` cast and produced pg 22P02 → 500 in `UpdateCircle`, `RefreshInviteCode`, `RemoveMember`, and `ArchiveCircle`. **Fixed**: `isUUID` guards added, mirroring `AssignRole`/`GetCircle`; regression coverage in `TestCircleMutations_RejectMalformedIDs` (integration, real PostgreSQL). All gates re-run green.
2. Invite-link base `https://halaqaty.app/join/` inlined at three sites. **Fixed**: single `inviteLinkBase` constant in `backend/internal/rbac/service.go`, referenced by both service response builders and the invite-refresh handler.
3. Self-removal reuses `ErrSelfRoleChange`, producing a misleading message for remove/leave attempts. **Deferred**: correcting the message changes a contract-asserted error envelope, and leave-circle semantics belong to a separate story; filed as follow-up hardening.

Karim's mandatory manual RBAC/data-retention approval has not been recorded. T078 remains open until Karim signs off on the RBAC, archive/data-retention, and audit paths (all flagged mandatory-review in AGENTS.md).
