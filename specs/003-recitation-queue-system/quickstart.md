# Quickstart: Recitation Queue System

## Prerequisites

- Branch: `003-recitation-queue-system`
- PostgreSQL container `halaqaty-pg` available for integration tests
- F-001/F-002/F-005 migrations through `000016` applied
- Read `spec.md`, `plan.md`, `data-model.md`, ADR-010, ADR-013, ADR-015, ADR-018, ADR-019, and both feature-local contracts

## Implementation order

1. Apply the pending canonical-sync deltas from `plan.md` §Canonical sync to `docs/contracts/openapi.yaml`, `docs/contracts/ws_events.md`, and the one ARCHITECTURE.md constraint line; run `$docs-guard` and `make api-lint` before writing handlers or mobile DTOs.
2. Add the paired `000017_recitation_queue_system` migration and verify fresh up/down/up behavior plus all CHECK, FK, unique, and partial indexes.
3. Write failing Go tests for policy, round population, controls (including single-entry move), opt-out, completion/progress, correction, concurrency, media failure, and session-end convergence.
4. Implement `backend/internal/queue` and wire centralized routes/middleware; extend F-005 only at its existing provider-neutral hook points.
5. Write failing Flutter tests for Arabic/RTL queue state, manager controls, grade visibility, reconnect/version gaps, and non-blocking session end; implement inside the existing sessions feature.
6. Add contract/security tests and verify no queue artifact contains media credentials, room references, provider identifiers, or URLs.

## Local verification

From `backend/`:

```powershell
$env:DATABASE_URL='postgres://postgres:postgres@localhost:5432/halaqaty?sslmode=disable'
go test -short ./...
go test -tags=contract ./...
go test -tags=integration ./...
go test -race ./internal/queue ./internal/sessions ./internal/realtime
golangci-lint run ./...
gofmt -l .
```

From `mobile/`:

```powershell
flutter test test
flutter test integration_test/
flutter analyze
dart format --set-exit-if-changed .
```

From the repository root:

```powershell
make api-lint
gitleaks detect --source .
git diff --check
```

Use the documented Docker Flutter/Spectral fallbacks when host SDKs are unavailable. A missing device, SDK, database, or backend environment is a blocked gate, never a pass.

## Acceptance smoke flow

1. Authenticate a teacher and two students with current device sessions; ensure active circle roles.
2. Create a scheduled F-005 session; prepare a round with a manual pre-set order; confirm the prepared order is manager-surface only.
3. Start the session; confirm the first prepared round activates automatically and materializes exactly one entry per eligible student under the population policy; prepare a second round while the first is active and confirm it stacks prepared.
4. Advance and start one entry; confirm it alone is `reciting` and only its audio publishing is granted; move another waiting entry while recitation is in progress.
5. Complete it atomically with a grade and 500-character note; confirm one progress row; complete a grading-optional entry with no grade.
6. Retry the same command, concurrently skip another entry, and approve/auto-approve an opt-out; confirm convergence and no non-completed progress rows.
7. Reset the round; confirm prior history is retained and the next prepared round activates automatically in round-number order; reconnect a client and confirm it re-fetches the queue and ignores stale/duplicate events.
8. End F-005 while forcing queue cleanup/media failure; confirm end returns committed, the active round and never-activated prepared rounds finalize (inert), and reconciliation converges within the 10-second target.

## Required review evidence

- REST/WebSocket canonical and feature-local contracts agree.
- Every new route uses authentication, current membership/role authorization, validation, global rate limiting, standard errors, and audit instrumentation.
- PostgreSQL proves one active round, one entry/student/round, one reciter/round, and one progress row/completed entry under concurrency.
- Queue UI remains usable in Arabic RTL, and queue failures never disable F-005 end/general moderation.
- No LiveKit import appears in F-003; no credential or room reference appears in contracts, queue persistence, events, logs, caches, or URLs.
