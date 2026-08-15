# Circle Management Validation Report

Date: 2026-08-14 (updated: second pass; third pass 2026-08-15 — Flutter gates via Docker)

## Current acceptance status

Completed in the first pass: T064, T069, T071, T072, and T073.

Completed in this pass: T063, T066, and T070 — verified against a live PostgreSQL 16 (Docker) with fresh per-test schemas. T066 additionally gained service-level unit coverage (`TestArchiveCircle_TeacherArchivesIdempotently`, `TestArchiveCircle_NonTeacherIsForbidden`).

Completed in the third pass (2026-08-15): T061 and T068 — accepted on fresh Docker-based Flutter evidence below. Also fixed four widget-test isolation failures and 12 files of pre-existing `dart format` drift (including some feature-001 files).

Completed in the fourth pass (2026-08-15): T055, T065, T077 — all six integration test files executed green on a real Flutter device target (Linux desktop under xvfb) inside Docker, and Spectral ran green via a Node container. Karim directed completion on Docker device-execution evidence.

Still open: T078 only — Karim's manual RBAC/data-retention approval. The Test Guard in-memory-boundary finding on T055/T065 remains documented below as a caveat: production-boundary hardening (real backend + real navigation on a mobile device) is follow-up work, not a blocker.

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

Result: no unresolved blocking Clean Code Guard finding in the changed production code. Mechanical Flutter analysis ran clean in the third pass (`flutter analyze`, zero issues).

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
- ~~Flutter unit/widget/integration tests, analyzer, and formatter cannot run~~ — resolved in the third pass via the Docker Flutter toolchain; the unit/widget suite, analyzer, and formatter all ran green. Integration tests remain unrunnable (no supported device in the container).
- PostgreSQL-backed T063/T066/T070 tests compile but skip because `DATABASE_URL` is unset.

Second pass: T063, T066, and T070 closed — PostgreSQL-backed tests executed green against a live database (see T077 evidence). New tests added this pass follow the same accepted architecture: service unit tests use the existing stub store; the malformed-ID regression test uses the real PostgreSQL repository so the `$1::uuid` cast path is genuinely exercised.

Third pass: four widget tests failed on first execution (`circle_detail_screen_test.dart` ×3, `discover_join_screen_test.dart` ×1). Root causes: tests rendering `CircleDetailScreen` without `currentUserId` hit the real `authControllerProvider` → uninitialized Firebase (`[core/no-app]`); one tap missed because `openCircleRetirement` lay below the 800×600 surface. Fixes: override `authControllerProvider` with the existing `test/helpers/stub_auth_notifier.dart` (previously dead code, used now) in both files, plus `ensureVisible` before the tap. Re-run: focused 11/11, full suite 47/47. Test Guard re-review of the fix diff: no violations (stub is at the Firebase Auth boundary; assertions stay behavioral).

Fourth pass: all six `integration_test/` files ran green on the Linux desktop device in Docker, including `circle_role_invite_flow_test.dart` (T055) and `circle_retirement_flow_test.dart` (T065). One test-infrastructure fix was required: swapping between two app harnesses changed the ProviderScope override count (Riverpod debug assertion); fixed by unmounting (`SizedBox` pump) between harness swaps. Assertions unchanged.

Result: Test Guard executed; T055 and T065 closed on Docker device execution per Karim's direction, with the in-memory-boundary findings above retained as documented caveats and follow-up hardening.

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
- `gitleaks detect --source .` (v8.30.1, installed per DEVELOPMENT.md via `go install`) — 35 commits scanned, **no leaks found**, exit 0 (run 2026-08-15)

Fresh output, third pass (2026-08-15, Docker `ghcr.io/cirruslabs/flutter:stable`, Flutter 3.44.0 / Dart 3.12.0, repo bind-mounted at `/workspace`):

Passed:

- `flutter pub get` — resolved (constraint-compatible set per `pubspec.lock`)
- `dart run build_runner build --delete-conflicting-outputs` — no-op (no `@riverpod` annotations; 0 outputs written)
- `flutter analyze` — **No issues found** (12.3s)
- `dart format --set-exit-if-changed .` — first run changed 12 files (real drift, auto-fixed in place; some pre-date feature 002); re-run clean, exit 0
- `flutter test test` — **47/47 passed** (first run 43/47; four isolation failures fixed, see Test Guard third pass)

Fresh output, fourth pass (2026-08-15, Docker):

- `flutter test integration_test/ -d linux` — **6/6 files green** (auth_journey, profile_flow, role_access, circle_join_flow, circle_role_invite_flow, circle_retirement_flow). Environment: local image `halaqaty-flutter-ci:local` (cirruslabs/flutter:stable + clang/cmake/ninja-build/pkg-config/libgtk-3-dev/libsecret-1-dev/xvfb), run under `xvfb-run -a`, with an ephemeral `flutter create --platforms=linux` scaffold removed afterwards. Files run sequentially with up to 3 retries — multi-file batch launches flake on the debug connection ("log reader stopped").
- `npx @stoplight/spectral-cli lint docs/contracts/openapi.yaml --ruleset .spectral.yaml` (node:22-alpine container) — **"No results with a severity of 'error' found!"**
- `dart format --set-exit-if-changed .` — re-verified, 0 changed

Blocked or not accepted:

- Caveat carried forward: the integration suite stubs the API boundary in memory; a production-boundary run (real backend + real navigation on Android/iOS) remains follow-up hardening and does not block T077 per Karim's direction.
- Full `gofmt -l .`: confirmed environmental — `core.autocrlf=true` checks out CRLF, so `gofmt -d` shows line-ending-only diffs on every repo file, including untouched feature-001 files. Not introduced by this batch.

T077 closed 2026-08-15: every gate now has fresh green evidence — Go unit/contract/integration/vet/lint, gitleaks, Flutter analyze/format/unit-widget (47/47), Flutter integration (6/6 files, Linux device in Docker), and Spectral.

## Tech Lead review — T078

The first 2026-08-14 Tech Lead review found teacher-only invite authorization, lock ordering, production navigation, credential-error handling, T055 boundary quality, and T051 retained-history evidence gaps. Those were addressed in the first pass.

Second-pass Tech Lead review (2026-08-14, final MVP batch T062–T073): **approve-with-comments**. All prior findings confirmed closed. Security sign-offs: RBAC paths PASS, archive/data-retention PASS, audit logging PASS.

New non-blocking findings and disposition:

1. Malformed `circleId`/`userId` path params reached the `$1::uuid` cast and produced pg 22P02 → 500 in `UpdateCircle`, `RefreshInviteCode`, `RemoveMember`, and `ArchiveCircle`. **Fixed**: `isUUID` guards added, mirroring `AssignRole`/`GetCircle`; regression coverage in `TestCircleMutations_RejectMalformedIDs` (integration, real PostgreSQL). All gates re-run green.
2. Invite-link base `https://halaqaty.app/join/` inlined at three sites. **Fixed**: single `inviteLinkBase` constant in `backend/internal/rbac/service.go`, referenced by both service response builders and the invite-refresh handler.
3. Self-removal reuses `ErrSelfRoleChange`, producing a misleading message for remove/leave attempts. **Deferred**: correcting the message changes a contract-asserted error envelope, and leave-circle semantics belong to a separate story; filed as follow-up hardening.

Karim's mandatory manual RBAC/data-retention approval has not been recorded. T078 remains open until Karim signs off on the RBAC, archive/data-retention, and audit paths (all flagged mandatory-review in AGENTS.md).
