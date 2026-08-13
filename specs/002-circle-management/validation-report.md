# Circle Management Validation Report

Date: 2026-08-14

## Current acceptance status

Completed in this pass: T064, T069, T071, T072, and T073.

Still open: T055, T061, T063, T065, T066, T068, T070, T077, and T078. T061 and T068 are implemented but cannot be accepted until the mandatory Flutter gates run. T063, T066, and T070 require a configured PostgreSQL `DATABASE_URL`. T078 also requires Karim's manual RBAC/data-retention approval.

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

Result: Test Guard executed; T055, T063, T065, T066, and T070 remain open.

## Docs Guard — T076

Verified the canonical OpenAPI paths, operation IDs, security schemes, request fields, and local references against the Go routes, handlers, and models. The canonical contract was corrected to include nullable update fields, the shared conflict response, and archived-mutation conflict responses.

Unresolved generated-contract drift in `specs/002-circle-management/contracts/circle-management.openapi.yaml`:

- `UpdateCircleRequest` omits `grading_policy` although the Go and canonical models support it.
- Several archived mutation paths omit their `409 Conflict` response; archive also omits its possible `404 Not Found` response.

The feature-local file is Spec-Kit generated and was not hand-edited. Regenerate/synchronize it through the Spec-Kit workflow.

Spectral could not run because the `spectral` executable is not installed. The dedicated Go OpenAPI contract test passed for local references, required operation IDs, uniqueness, and bearer/session security.

## Verification evidence — T077

Passed:

- `go test -short ./...`
- `go test -tags=contract ./tests/contract -count=1`
- Focused circle service and production-router authorization tests
- Focused rate-limit/timeout and observability integration-tagged tests that do not require PostgreSQL
- `go vet ./...`
- `golangci-lint run ./...`
- `git diff --check`

Blocked or not accepted:

- PostgreSQL integration tests: skipped with `DATABASE_URL is not set`.
- Flutter tests/integration/analyze/format: Flutter and Dart executables unavailable.
- Spectral: executable unavailable.
- Gitleaks: executable unavailable.
- Full `gofmt -l .`: reports existing repository files outside this feature batch; touched Go files were formatted directly.

T077 remains open because all required gates are not green.

## Tech Lead review — T078

The 2026-08-14 Tech Lead review found teacher-only invite authorization, lock ordering, production navigation, credential-error handling, T055 boundary quality, and T051 retained-history evidence gaps. This pass addressed the production authorization, lock ordering, navigation, credential handling, and retained member-removal audit evidence. T055 remains open because its current integration test does not cross the production backend boundary.

Karim's mandatory manual RBAC/data-retention approval has not been recorded. T078 remains open.
