# Implementation Plan: Recitation Queue System

**Branch**: `003-recitation-queue-system` | **Date**: 2026-08-23 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specs/003-recitation-queue-system/spec.md`

## Summary

Deliver durable, sequential recitation rounds for scheduled or active F-005 sessions. A new Go `queue` domain owns round preparation/activation/finalization, pre-set ordering, entry transitions, opt-out requests, grading, completed-turn practice records, policy, and redacted queue audit events. PostgreSQL transactions, row locks, unique/partial indexes, optimistic versions, and idempotency receipts enforce one position, one reciter, one current round, and one practice record. Flutter adds an Arabic-first queue panel over the existing session shell. F-003 publishes redacted events through F-005 realtime and calls only its provider-neutral `ReciterAudioControl`; it never owns session/media lifecycle or credentials.

## Technical Context

**Language/Version**: Go 1.22; Dart/Flutter 3.x  
**Primary Dependencies**: `net/http`, existing Go HTTP stack, `pgx`, Firebase-auth middleware, Riverpod, Dio, existing WebSocket and `livekit_client` adapters  
**Storage**: PostgreSQL 16; no queue state in LiveKit, Firebase, caches, or process memory  
**Testing**: Go unit/contract/integration and race-focused concurrency tests; Flutter unit/widget/integration tests; migration up/down and OpenAPI/WebSocket contract checks  
**Target Platform**: Single Docker Compose Go service and Android/iOS Flutter client  
**Project Type**: Mobile application plus REST/WebSocket backend  
**Performance Goals**: Queue reads and mutations under 200 ms p95; 95% of committed queue changes visible to connected clients within 500 ms, excluding FCM latency  
**Constraints**: 50 participants per session, at most 10 live sessions, additive `/api/v1` evolution, Arabic-first RTL, no timers/video/recording/AI, F-005 end never waits for F-003  
**Scale/Scope**: One current prepared/active round per session; retained finalized history; one entry per student per round; at most one reciting entry

## Constitution Check

*GATE: Passed before research and passed again after design.*

- **Spec-first**: PASS — the approved F-003 scope and five clarification decisions are recorded in `spec.md`; this command creates design artifacts only.
- **Stack/source of truth**: PASS — Go, Flutter, PostgreSQL, Firebase identity, existing WebSocket transport, and self-hosted LiveKit remain unchanged.
- **Authorization/security**: PASS — every operation revalidates the current device session and active `circle_members` role; immutable queue/media invariants cannot be configured.
- **Media boundary**: PASS — queue code depends only on `sessions.ReciterAudioControl`; no provider SDK type, credential, endpoint, or room reference appears in queue contracts, persistence, logs, caches, or URLs.
- **Contract-first/backward compatibility**: PASS — canonical REST and WebSocket contracts are synchronized before implementation; additions remain under `/api/v1`. The pre-implementation queue placeholders are completed before any F-003 client ships.
- **Reliability**: PASS — transactions, row locks, constraints, optimistic versions, idempotency receipts, bounded media reconciliation, at-least-once events, and REST recovery are specified.
- **Test-first and scope**: PASS — tests precede implementation; no scheduling, chat, payments, dashboards, FCM infrastructure, session/media rebuild, timers, video, recording, or AI work is introduced.

## Existing Baseline

- Reuse F-001 authentication/current-device middleware, request IDs, standard errors, rate limiting, and structured redacted audit logging.
- Reuse F-002 `circle_members` as the only authorization source; `created_by` is never an authorization shortcut.
- Reuse F-005 `sessions`, `session_participant_presence`, session-topic authorization, realtime hub, session-end reconciliation, and `ReciterAudioControl`.
- Add `backend/internal/queue/`; SQL remains in `queue_queries.go`, route patterns remain in `backend/cmd/api/routes.go`, and only the sessions package resolves its private media room reference.
- Add queue models/controller/widgets under `mobile/lib/features/sessions/` so the existing room lifecycle and media connection remain intact.

## Phase 0 — Research and canonical alignment

1. Apply the decisions in [research.md](./research.md), including closed session policy, round lifecycle, pre-set candidates, atomic completion/progress, optimistic concurrency, media convergence, and F-008 notification ownership.
2. Synchronize the additive queue surface in `docs/contracts/openapi.yaml` with [contracts/recitation-queue.openapi.yaml](./contracts/recitation-queue.openapi.yaml). Retain the existing queue paths and complete them with reset, advance, state transition, order, opt-out decision, and policy controls.
3. Synchronize `docs/contracts/ws_events.md` with [contracts/recitation-queue.ws_events.md](./contracts/recitation-queue.ws_events.md). Every queue event carries `event_id`, `session_id`, `round_id`, and `version`; visibility-filtered grade fields never leak to unauthorized participants.
4. Update `docs/engineering/architecture/ARCHITECTURE.md` with the exact additive schema and remove placeholder queue behavior that conflicts with this design. ADR-018 already authorizes the closed policy and non-blocking session-end integration; no new framework or infrastructure ADR is required.

## Phase 1 — Data and backend design

1. Add paired migration `000017_recitation_queue_system` for the 114-Surah Quran reference seed, session policy columns, rounds, pre-set candidates, entries, opt-out requests, command receipts, a redacted-metadata event outbox, completed-turn `memorization_progress`, indexes, and constraints from [data-model.md](./data-model.md). The down migration removes only F-003-owned objects/columns.
2. Implement queue repository transactions using `SELECT ... FOR UPDATE`, a partial unique current-round index, a partial unique reciter index, unique round/student and progress/entry constraints, contiguous position rewrites, expected versions, and optional idempotency receipts.
3. Implement services for prepare/activate/reset, derived late-join append, reorder/move, advance selection, entry transitions, opt-out decision, correction, visibility projection, and ended-session convergence. Authentication/membership/role checks precede mutation and are repeated inside the locked transaction where staleness matters.
4. Complete an entry and insert/upsert its one `memorization_progress` row in the same transaction. Grade-required completion is rejected without an allowed grade; skipped and opted-out paths cannot write progress.
5. Commit queue truth and a redacted-metadata event intent in one transaction before at-least-once WebSocket/in-app projection. The outbox worker reconstructs visibility-sensitive fields from PostgreSQL and retries failed delivery with bounded exponential backoff; clients deduplicate by `event_id` and version and re-fetch after reconnect or a version gap.
6. For turn start, commit the sole `reciting` entry then request its grant; a grant failure leaves authoritative state intact, reports recoverable media unavailability, and is retried. Turn end attempts revoke before another turn can start. F-005 session end returns independently; F-003 later revokes/finalizes idempotently.

## Phase 1 — Mobile design

1. Extend the existing session API/realtime clients with typed queue contracts, event IDs, round versions, visibility-safe nullable grade fields, reconnect re-fetch, and stale-event rejection.
2. Add Riverpod queue state/controller integrated with `SessionRoomController`; REST snapshots replace local state, while events only trigger version-aware updates or re-fetch.
3. Add Arabic-first RTL student position/opt-out views and manager preparation, ordering, advance, skip, complete/grade, reset, policy, correction, and end-session controls. Queue/media errors never disable the existing F-005 end or general moderation actions.
4. FCM/device-token registration and general notification delivery remain F-008-owned. F-003 defines stable turn event identifiers so a later F-008 adapter can deliver the same at-least-once notification without changing queue truth or contracts.

## Phase 2 — Verification strategy

1. Unit-test validation, role/visibility matrices, all policy values, state transitions, note boundary, terminal states, and mobile reconnection reducers.
2. Integration-test duplicate/concurrent reset, advance/start, reorder, late join, opt-out, completion, correction, session end, and media failures against PostgreSQL; assert all uniqueness and completed-only progress invariants.
3. Contract-test every REST path/status/schema and WebSocket projection, including grade redaction, standard errors, rate limiting, event deduplication, and media-secret scanning.
4. Run migration fresh/up/down/up, Go unit/integration/race/fmt/lint, Flutter unit/integration/analyze/format, Spectral, gitleaks, and repository architecture/quality reviews before completion.

## Project Structure

```text
backend/
├── internal/queue/                 # domain, repository, service, handler, queries, tests
├── internal/sessions/              # existing ReciterAudioControl and end hook only
├── internal/realtime/              # existing redacted session-topic delivery
├── internal/platform/logging/      # queue audit event builders
├── cmd/api/routes.go               # centralized queue route patterns
└── migrations/000017_recitation_queue_system.{up,down}.sql

mobile/lib/features/sessions/
├── application/                    # queue controller integrated with session shell
├── data/                           # queue REST/WebSocket DTOs and constants
├── domain/                         # queue/policy models
└── presentation/                   # Arabic-first RTL queue UI

backend/tests/{contract,integration}/
mobile/test/{unit,widget}/sessions/
mobile/integration_test/
docs/contracts/{openapi.yaml,ws_events.md}
specs/003-recitation-queue-system/{research.md,data-model.md,quickstart.md,contracts/}
```

**Structure Decision**: Extend the existing modular monolith and session feature; add one focused backend queue package and no new service, provider abstraction, cache, scheduler, or notification subsystem.

## Complexity Tracking

No constitution violations require justification.
