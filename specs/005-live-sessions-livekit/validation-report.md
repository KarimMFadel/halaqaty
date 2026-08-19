# F-005 Validation Report

Updated: 2026-08-19

## Passed

- Backend short suite: `go test -short ./...`.
- Full backend integration suite: `DATABASE_URL=postgres://postgres:postgres@localhost:5432/halaqaty?sslmode=disable go test -tags=integration ./...` against Docker `halaqaty-pg` (`postgres:16-alpine`).
- T041/T043 focused recovery and atomic-start integration: `go test -tags=integration ./tests/integration -run 'TestRecoveryIntegration_|TestStartJoinIntegration_StartLockCoversRoomEnsureAndAdmission' -count=1` against Docker PostgreSQL.
- T043 focused reconciler unit tests: `go test -run 'TestStableMediaRoomRefIsDeterministicOpaqueAndKeyed|TestReconcilerSweepProcessesBoundedLifecycleCandidates' ./internal/sessions`.
- Full backend contract suite: `go test -tags=contract ./tests/contract`.
- Go lint: `golangci-lint run ./...`.
- Flutter unit/widget suite in Docker: `flutter test test` (80 tests passed).
- Flutter analyze in Docker: `flutter analyze` (no issues).
- Dart format in Docker: `dart format --set-exit-if-changed .` (53 files, 0 changed).
- Canonical OpenAPI Spectral lint via approved `node:22-alpine` Docker fallback.
- Secret scan: `make secrets` (no leaks found).
- T055 contract completeness: `go test -tags=contract ./tests/contract` (including error mappings, `ERR_MEDIA_UNAVAILABLE`, no-store, and standard envelopes).
- T056 signed webhook coverage: contract suite plus `go test -tags=integration ./tests/integration -run 'TestLiveKitWebhookIntegration_RateLimitAndSignedDelivery' -count=1` against Docker PostgreSQL.
- T057/T059 discovery coverage: backend contract suite and Docker Flutter `flutter test test/widget/sessions/session_discovery_test.dart`; canonical `GET /circles/{circleId}/sessions` is wired without scheduling/attendance behavior.
- T058 migration ownership: `go test -tags=integration ./tests/integration -run 'TestLiveSessionsMigration_F005OwnsSessionLifecycleTables' -count=1` against Docker PostgreSQL.
- T050 guard review: changed Go paths are gofmt-clean, `git diff --check` is clean for the batch, and focused tests contain no new TODO/FIXME or credential-bearing output.

## Blocked or open

- Flutter `integration_test/` in `halaqaty-flutter-ci:local` did not produce test output and the ephemeral containers remained running past the bounded wait; they were stopped. This gate remains blocked, not passed.
- Exact `make lint` remains blocked on this host because the host Flutter executable is unavailable; backend lint passed and Docker Flutter analyze/Spectral equivalents passed. `make secrets` passed.
- Flutter `integration_test/` remains blocked locally; the user-reported GitHub PR #18 unit/integration workflow is supporting CI evidence but cannot replace the unavailable local device gate.
- T054 remains approval-gated for Karim's manual auth, RBAC, media-credential, webhook, and response-safety review.
