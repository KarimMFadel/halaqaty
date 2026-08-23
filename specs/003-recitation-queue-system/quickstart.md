# Quickstart: Recitation Queue System

## Prerequisites

- Branch: `003-recitation-queue-system`
- PostgreSQL container `halaqaty-pg` available for integration tests
- F-001/F-002/F-005 migrations through `000016` applied
- Read `spec.md`, `plan.md`, `data-model.md`, ADR-010, ADR-015, ADR-018, and both feature-local contracts

## Implementation order

1. Synchronize and lint `docs/contracts/openapi.yaml` and `docs/contracts/ws_events.md` before writing handlers or mobile DTOs.
2. Add the paired F-003 migration and verify fresh up/down/up behavior plus all CHECK, FK, unique, and partial indexes.
3. Write failing Go tests for policy, round population, controls, opt-out, completion/progress, correction, concurrency, media failure, and session-end convergence.
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
2. Create a scheduled/active F-005 session and prepare a round with a manual student order.
3. Activate the session/round and confirm exactly one entry per eligible student.
4. Advance and start one entry; confirm it alone is `reciting` and only its audio publishing is granted.
5. Complete it atomically with a grade and 500-character note; confirm one progress row.
6. Retry the same command, concurrently skip another entry, and approve/auto-approve an opt-out; confirm convergence and no non-completed progress rows.
7. Reset the round, reconnect a client, and confirm it re-fetches the queue and ignores stale/duplicate events.
8. End F-005 while forcing queue cleanup/media failure; confirm end returns committed and reconciliation later finalizes the queue.

## Required review evidence

- REST/WebSocket canonical and feature-local contracts agree.
- Every new route uses authentication, current membership/role authorization, validation, global rate limiting, standard errors, and audit instrumentation.
- PostgreSQL proves one current round, one entry/student/round, one reciter/round, and one progress row/completed entry under concurrency.
- Queue UI remains usable in Arabic RTL, and queue failures never disable F-005 end/general moderation.
- No LiveKit import appears in F-003; no credential or room reference appears in contracts, queue persistence, events, logs, caches, or URLs.
