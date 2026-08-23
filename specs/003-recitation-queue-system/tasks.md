# Tasks: Recitation Queue System

**Input**: Design documents from `specs/003-recitation-queue-system/` — regenerated 2026-08-23 from the approved plan (post-clarification), `data-model.md`, and both feature-local contracts. The prior pre-clarification task list is fully replaced.

**Prerequisites**: `plan.md`, `spec.md` (Approved 2026-08-23), `research.md`, `data-model.md`, `contracts/recitation-queue.openapi.yaml`, `contracts/recitation-queue.ws_events.md`, `quickstart.md`
**MVP scope**: All three P1 user stories (US1, US2, US3). F-003 is not complete if any is omitted. No P2/P3 stories exist for this feature. No history UI/REST surface and no FCM work anywhere (I3/I4 — F-007/F-008 scope).

**Task format**: `- [ ] [ID] [P?] [US?] (owner) Description with file path — Dep: … Verify: …`
- **[P]**: parallelizable — no incomplete dependency, no file shared with another in-flight parallel task.
- **(owner)**: exactly ONE owner agent per task. Conventions: Go backend → `senior-golang-developer`; Flutter → `senior-flutter-mobile-engineer`; canonical contracts/schema sync + contract-catalog parity → `architect`; migration SQL authoring → `database-optimizer`; review gates → `tech-lead` (Karim's mandatory deep review).
- **Dep**: explicit task dependencies. **Verify**: required evidence before the task may be marked `[X]` (unavailable tooling = blocked, never passed).

**Reliability parameters (A1, governing several tasks)**: outbox delivery = initial attempt + 5 retries (+1/+2/+4/+8/+16 s, jittered) then parked with alert + operator replay; `ReciterAudioControl` calls = 5 s timeout, non-blocking, never rolls back queue commit; session-end convergence = idempotent retry, ≤ 10 s target after observing end. SC-008 = p95 ≤ 500 ms PG-commit→dispatch, ≥ 100 committed actions/scenario, local-network env, disconnected clients excluded.

---

## Phase 1: Canonical Sync (blocks migration and all code)

**Purpose**: Apply plan §Canonical sync deltas CS-1..CS-7 to the canonical contracts before any handler, migration, or Flutter DTO work (D13, FR-015). Feature-local contracts in `specs/003-recitation-queue-system/contracts/` are already authoritative for implementation shape.

- [ ] T001 [P] (architect) Apply CS-1..CS-4 to `docs/contracts/openapi.yaml`: B1 stacking/automatic-activation description on `POST /sessions/{sessionId}/queue/rounds`; add `POST /sessions/{sessionId}/queue/entries/{entryId}/move` with `MoveEntryRequest` (200 QueueState, 400/401/403/404/409/422/429) exactly per `specs/003-recitation-queue-system/contracts/recitation-queue.openapi.yaml`; restrict `PUT /sessions/{sessionId}/queue/order` + `ReorderQueueRequest` to prepared-round pre-set candidates and remove `order_kind`/`queue_entries` from the request; grading-optional completion + reset→auto-activation wording on `EntryStatusRequest` and `POST .../reset` — Dep: —. Verify: `$docs-guard` clean + `make api-lint` green (Spectral via Docker fallback is acceptable).
- [ ] T002 [P] (architect) Apply CS-5..CS-6 to `docs/contracts/ws_events.md`: `queue.reordered` order_kind ∈ {`preorder_students`, `entry_move`} with `ordered_ids` = resulting complete order + updated example; `queue.round_started` automatic-activation triggers; `queue.round_finalized` session-end finalizes never-activated prepared rounds as inert (`reason: session_ended`) — Dep: —. Verify: `$docs-guard` clean; enum/event names byte-identical to `specs/003-recitation-queue-system/contracts/recitation-queue.ws_events.md`.
- [ ] T003 [P] (architect) Apply CS-7 one-line fix to `docs/engineering/architecture/ARCHITECTURE.md`: `recitation_queue` partial unique `WHERE lifecycle IN ('prepared','active')` → `WHERE lifecycle = 'active'` ("One active round") — Dep: —. Verify: `$docs-guard` clean; line matches `specs/003-recitation-queue-system/data-model.md` §recitation_queue constraints exactly.

**Checkpoint**: Canonical and feature-local contracts agree; implementation truth is frozen. Migration and Flutter DTOs may now start.

---

## Phase 2: Foundational (blocking prerequisites for every user story)

**Purpose**: PostgreSQL schema (ADR-019/ADR-018, CS-7 constraint), queue domain types/validation, shared persistence, error constants, audit, metrics, media-control boundary, session observer, and the outbox delivery core.

**⚠️ CRITICAL**: No user-story implementation starts until this phase (including Gate A) is complete. Canonical sync (Phase 1) → migration → domain/validation → … per plan §Risks 3.

- [ ] T004 (database-optimizer) Write failing fresh-schema migration tests for all F-003 objects in `backend/tests/integration/recitation_queue_migration_test.go`: up/down/up cycle; `quran_surahs` (114 rows, positive `ayah_count`), `recitation_queue` (round CHECKs, `UNIQUE (session_id, round_number)`, partial unique `(session_id) WHERE lifecycle = 'active'` per CS-7), `recitation_queue_preorder`, `recitation_queue_entries` (5-state CHECK, grade CHECK, `(queue_id, student_id)` + `(queue_id, position)` unique, partial unique reciter index), `queue_opt_out_requests` (one-pending partial unique), `queue_command_receipts` (composite PK), `queue_event_outbox`, `memorization_progress` (`queue_entry_id NOT NULL UNIQUE`, no-cascade FKs), `sessions` policy columns with ADR-018 defaults + CHECKs; down removes only F-003 objects and F-001/F-002/F-005 data survives — Dep: T001, T002, T003. Verify: `go test -tags=integration -run RecitationQueueMigration ./tests/integration/` red (migration absent), with `DATABASE_URL` set.
- [ ] T005 (database-optimizer) Author paired migration `backend/migrations/000017_recitation_queue_system.up.sql` + `.down.sql` exactly per `specs/003-recitation-queue-system/data-model.md` (re-check 000017 is still the next sequence number immediately before creating): all FKs plain `NO ACTION` (ADR-019 — no cascade anywhere), immutable 114-row Surah seed, least-privilege app-role read grants on `quran_surahs`, partial unique `WHERE lifecycle = 'active'` — Dep: T004. Verify: T004 green end-to-end (fresh up/down/up).
- [ ] T006 **GATE A — Tech Lead migration review** (tech-lead) Mandatory Karim deep-review of the F-003 schema diff: partial-unique constraints (one active round, one reciter, one pending opt-out), `queue_entry_id NOT NULL UNIQUE` idempotency target, no-cascade FK policy, CHECK enums vs ADR-013/data-model, policy columns vs ADR-018, CS-7 ARCHITECTURE.md line — Dep: T003, T005. Verify: recorded sign-off on the T005 diff; findings fixed and re-reviewed before any repository code builds on the schema. **Blocking**: nothing in T007+ starts before this passes.
- [ ] T007 (senior-golang-developer) Define round, entry, preorder, policy, opt-out request, receipt, outbox, progress, and domain-error types with closed enums (5 entry states · 3 round lifecycle values · 4 round types · 5 grades · 5 policy dimensions) and zero provider-specific media types in `backend/internal/queue/queue_types.go` — Dep: T006. Verify: `go build ./internal/queue` + `gofmt -l .` empty; enums byte-match data-model.md.
- [ ] T008 (senior-golang-developer) Write table-driven validation tests in `backend/internal/queue/queue_validation_test.go`: closed enums, Quran range vs Surah `ayah_count`, `from_ayah <= to_ayah`, note ≤ 500 chars, entry transition table (`waiting → reciting → completed`; `waiting|reciting → skipped|opted_out`; terminal states), positive expected versions, Idempotency-Key 1–128 — Dep: T007. Verify: red run evidence (`go test -short ./internal/queue`).
- [ ] T009 (senior-golang-developer) Implement `backend/internal/queue/queue_validation.go` making T008 green (pure functions; SQL bounds injected) — Dep: T008. Verify: `go test -short ./internal/queue` green.
- [ ] T010 (senior-golang-developer) Define all package-level parameterized SQL statements (authorization lookups, policy reads, locked round/entry claims, population queries, receipts, outbox insert/claim/park, visibility projections, progress upsert) in `backend/internal/queue/queue_queries.go` — no inline SQL will exist in repository/service bodies — Dep: T007, T005. Verify: `go vet ./internal/queue`; review confirms every statement parameterized (constitution IV.7).
- [ ] T011 (senior-golang-developer) Write repository tests in `backend/internal/queue/queue_repository_test.go` (integration, real PG): transaction rollback, `SELECT … FOR UPDATE` round locks, optimistic-version conflicts, reused idempotency-key-with-another-command conflict, duplicate replay returns committed resource, outbox row inserted in the same transaction, receipt/outbox metadata redaction (no grade/note/name/media) — Dep: T010. Verify: red run evidence with `DATABASE_URL`.
- [ ] T012 (senior-golang-developer) Implement `backend/internal/queue/queue_repository.go` making T011 green (transactions, locks, receipts, visibility-filtered projections) — Dep: T011. Verify: `go test -tags=integration ./internal/queue` green.
- [ ] T013 [P] (senior-golang-developer) Add centralized queue error codes/messages to `backend/internal/platform/httpconst/error_codes.go` and `error_messages.go`: 409 conflict cases (stale version, invalid transition, no waiting entry, entry reciting/terminal, finalized/inert round, duplicate command), 422 enum/range/order/grade/note, 503 audio-convergence-pending with committed truth intact — no internal or media detail leaks — Dep: T001. Verify: `go build ./internal/platform/httpconst`; constants are the only allowed source of these literals in handlers.
- [ ] T014 [P] (senior-golang-developer) Write red tests then implement redacted queue audit actions (policy changes, opt-out request/decision, grade/note correction with prior/current values and no note text, manager attribution on every mutation) in `backend/internal/platform/logging/queue_audit_test.go` and `backend/internal/platform/logging/audit_logger.go` — PG-persisted business facts per plan §Audit model; ops logs non-authoritative — Dep: T007. Verify: `go test -short ./internal/platform/logging` green after implementation; tests prove absence of notes/grades/media in audit payloads.
- [ ] T015 [P] (senior-golang-developer) Write red tests then implement bounded-cardinality F-003 metrics in `backend/internal/platform/metrics/queue_metrics_test.go` and `backend/internal/platform/metrics/queue_metrics.go`: `queue_command_duration`, `queue_command_conflicts_total`, `outbox_pending`, `outbox_parked_total`, `event_delivery_lag` (SC-008 commit→dispatch p95), `audio_convergence_lag`, `session_end_finalization_lag`, `invariant_violations_total`, queue rate-limit counters — UUID/closed-enum labels only, no PII — Dep: T007. Verify: `go test -short ./internal/platform/metrics` green; label cardinality bounded in tests.
- [ ] T016 [P] (senior-golang-developer) Write red tests then implement the provider-neutral `ReciterAudioControl` boundary — `GrantReciterAudio(ctx, sessionID, roundID, queueEntryID, userID)` / `RevokeReciterAudio(...)` declared in `backend/internal/sessions/` and implemented ONLY in `backend/internal/sessions/livekit/adapter.go`: 5 s per-call timeout, idempotent grant/revoke, `CanPublishVideo` always false, secret-safe errors, neutral `MediaConnection` identifiers only (ADR-015) — tests in `backend/internal/sessions/livekit/reciter_audio_control_test.go` — Dep: —. Verify: `go test -short ./internal/sessions/livekit` green; no LiveKit import outside `internal/sessions/livekit/`.
- [ ] T017 [P] (senior-golang-developer) Write red tests then implement the narrow optional queue observer in `backend/internal/sessions/queue_observer.go` + `backend/internal/sessions/queue_observer_test.go`: committed session-start / participant-join / session-end facts only; bounded callbacks that can never block or roll back F-005 lifecycle transactions — Dep: —. Verify: `go test -short ./internal/sessions` green.
- [ ] T018 (senior-golang-developer) Write red tests then implement the transactional-outbox core in `backend/internal/queue/outbox_test.go` and `backend/internal/queue/outbox.go`: dispatcher with initial attempt + 5 exponential-backoff retries (+1/+2/+4/+8/+16 s, jittered) then parked (`parked_at`, `outbox_parked_total` alert, operator replay — never silent drop); startup replay of pending + parked rows; visibility-aware projector framework reconstructing grade/note/name fields from PostgreSQL at send time — Dep: T012, T015. Verify: `go test -short ./internal/queue` green (fake clock for backoff); parking path asserts metric+alert.

**Checkpoint**: Schema reviewed and frozen (Gate A passed); validation, persistence, idempotency, audit, metrics, error constants, media boundary, observer, and outbox delivery core ready. User-story work may begin (US1 first).

---

## Phase 3: User Story 1 — Prepare and run a recitation round (Priority: P1) 🎯 MVP

**Goal**: Teachers/supervisors prepare stacked rounds that activate automatically in round-number order, populate under either policy, and run one-reciter-at-a-time turns with durable ordering, advance/start separation, move, skip, reset, and prospective policy — audio granted/revoked only via `ReciterAudioControl`.

**Independent Test**: Prepare rounds with join and manual order, start a live session, verify automatic activation and one entry per eligible student under both population policies, reorder pre-activation, move mid-round, advance/start/skip successive turns, reset, and prove PostgreSQL and audio entitlement never identify more than one reciter.

### Tests for User Story 1 (write first — red)

- [ ] T019 [P] [US1] (senior-golang-developer) Write unit tests in `backend/internal/queue/round_service_test.go`: prepare while scheduled/live, several prepared rounds stack with max+1 numbering under lock (CHK036), activation invariant in round-number order (B1), `present_at_activation` (pre-set present first, then present members by `first_joined_at`, UUID tie-break) and `all_active_students` (pre-set first, then `circle_members.joined_at`, UUID tie-break), empty-population round activates empty (CHK031), teacher/supervisor exclusion, pre-activation full-list reorder, post-activation single-entry move (allowed while reciting; reciting/terminal entries immovable), reset finalizes + creates next round + invariant activates next prepared — Dep: T009, T012. Verify: red run evidence.
- [ ] T020 [P] [US1] (senior-golang-developer) Write unit tests in `backend/internal/queue/turn_service_test.go`: advance selects next waiting durably and replaces an existing selection without duplicate; zero-waiting → clean rejection no mutation; advance-while-reciting rejected; start applies only to the selected waiting entry; one-reciter invariant (partial unique as final guard); skip from `waiting|reciting` with audio revoked when reciting; audio grant after commit, revoke before next reciter, 5 s timeout non-blocking, 503 audio-convergence-pending with committed truth intact — Dep: T009, T012, T016. Verify: red run evidence.
- [ ] T021 [P] [US1] (senior-golang-developer) Write unit tests in `backend/internal/queue/policy_service_test.go`: all values of the five policy dimensions, manager-only + scheduled-or-active guard, `queue_policy_version` increments only on effective change, workflow policies prospective, grade/note visibility immediate and prospective, every change emits a redacted durable audit record — Dep: T009, T012, T014. Verify: red run evidence.
- [ ] T022 [P] [US1] (senior-golang-developer) Write PostgreSQL acceptance tests in `backend/tests/integration/recitation_queue_round_flow_test.go`: prepare→stack→session-start auto-activation→population under both policies→reorder/move durability→advance/start/skip→reset chain with retained immutable history (quickstart smoke steps 1–4, 7) — Dep: T012. Verify: red run evidence with `DATABASE_URL`.
- [ ] T023 [P] [US1] (senior-golang-developer) Write concurrency tests in `backend/tests/integration/recitation_queue_concurrency_test.go` for every racing pair (CHK032): advance/start, reset/complete, reorder|move vs late-join append, opt-out/skip first-terminal-wins, plus stale-version replays converging to one durable outcome (CHK025) — Dep: T012. Verify: red run evidence.
- [ ] T024 [P] [US1] (senior-golang-developer) Write BEHAVIORAL REST contract tests in `backend/tests/contract/recitation_queue_management_contract_test.go` (runtime requests/responses pinned to the feature-local contract — NOT document parity, which is T070): GET snapshot/POST rounds/PUT order/POST move/POST advance/PUT status (start|skip)/POST reset/PATCH policy — schemas, status codes incl. 409/422/429 variants, `Idempotency-Key` replay semantics, standard error envelope — Dep: T001, T013. Verify: red run evidence (`go test -tags=contract -run RecitationQueueManagement`).
- [ ] T025 [P] [US1] (senior-golang-developer) Write RBAC denial tests in `backend/tests/integration/recitation_queue_rbac_test.go`: unauthenticated, inactive/non-member, student attempting every manager operation, session creator without current teacher/supervisor role — every case asserts zero state mutation — Dep: T012. Verify: red run evidence.

### Implementation for User Story 1

- [ ] T026 [US1] (senior-golang-developer) Implement `backend/internal/queue/round_service.go` making T019/T022 green: prepare, stacking, activation invariant restoration (session start, round creation on live session, finalization-via-reset), population materialization in one locked transaction, pre-activation reorder, move, reset — Dep: T019, T022. Verify: `go test -short ./internal/queue` + integration round-flow green.
- [ ] T027 [US1] (senior-golang-developer) Implement `backend/internal/queue/turn_service.go` making T020 green: advance/start/skip with selection lifecycle, one-reciter enforcement, `ReciterAudioControl` grant-after-commit/revoke-before-next with bounded retry, outbox row per mutation — Dep: T020, T026, T016. Verify: unit green; concurrency test T023 green.
- [ ] T028 [US1] (senior-golang-developer) Implement `backend/internal/queue/policy_service.go` making T021 green: five dimensions, versioning, prospective/immediate split, durable redacted audit via T014 — Dep: T021. Verify: unit green.
- [ ] T029 [US1] (senior-golang-developer) Implement US1 REST handlers + wiring: request decoding/validation, idempotency receipts, role middleware, rate limits, `httpconst` errors, visibility-filtered `QueueState` responses for GET queue, POST rounds, PUT order, POST move, POST advance, PUT status (start|skip), POST reset, PATCH policy in `backend/internal/queue/handler.go`; centralized route patterns in `backend/cmd/api/routes.go` + router/main wiring; session-start activation wiring via the T017 observer in `backend/internal/sessions/session_service.go` and `backend/cmd/api/main.go` (observer failure never changes F-005 results) — Dep: T024, T025, T026, T027, T028, T013, T017, T018. Verify: `go test -tags=contract -run RecitationQueueManagement ./tests/contract/` + RBAC integration green.
- [ ] T030 [P] [US1] (senior-golang-developer) Project US1 events from committed outbox rows through the existing hub in `backend/internal/queue/outbox.go` (+ `backend/internal/realtime/types.go` registration only): `queue.state`, `queue.round_started`, `queue.reordered` (both order kinds), `queue.advanced`, `queue.entry_updated`, `queue.policy_changed`, targeted `queue.your_turn`/`queue.next_soon` — server-built payloads, send-time visibility filtering, stable `event_id` + round `version` — Dep: T018, T026, T027, T028. Verify: integration test asserting authorized clients receive events in round-version order; no grade/note/media in broadcast payloads.

### Mobile for User Story 1 (starts only after T001/T002 canonical sync landed)

- [ ] T031 [P] [US1] (senior-flutter-mobile-engineer) Write red DTO/REST-client tests in `mobile/test/features/sessions/data/queue_api_client_test.dart`: `QueueState`/`QueuePolicy`/`QueueEntry`/preorder decode (empty `preorder` for non-managers), manager command payloads, standard error envelope, 409/422/503 parsing, `Idempotency-Key` header — current-queue surface ONLY, no history endpoints (I3) — Dep: T001, T002. Verify: red run evidence (`flutter test test/features/sessions/data` via Docker fallback if needed).
- [ ] T032 [P] [US1] (senior-flutter-mobile-engineer) Write red Riverpod controller tests in `mobile/test/features/sessions/application/queue_controller_test.dart`: authoritative snapshot state, manager commands with action-local failures, event dedup by `event_id`, stale-version ignore, version-gap → re-fetch `GET /sessions/{id}/queue` (FR-009), `queue.policy_changed` re-fetch self-heal (CHK034 pattern), session-end controls stay available after queue failures (FR-016) — Dep: T031. Verify: red run evidence.
- [ ] T033 [P] [US1] (senior-flutter-mobile-engineer) Write red Arabic-first RTL+LTR widget tests in `mobile/test/widget/sessions/queue/queue_manager_panel_test.dart`: prepare/reorder/move/advance/start/skip/policy controls, loading/empty/reconnecting/recoverable/terminal UI states, minimum touch targets, Arabic semantic labels for positions/statuses, screen-reader-friendly turn announcements — Dep: T032. Verify: red run evidence in BOTH text directions.
- [ ] T034 [US1] (senior-flutter-mobile-engineer) Implement `mobile/lib/features/sessions/data/queue_api_client.dart` making T031 green (dio, existing auth/session headers, current-queue surface only) — Dep: T031. Verify: `flutter test test/features/sessions/data` green.
- [ ] T035 [US1] (senior-flutter-mobile-engineer) Add `queue.*` event constants, decoding, `event_id` dedup, and version-gap signaling with zero media fields to `mobile/lib/features/sessions/data/session_protocol_constants.dart` and `mobile/lib/features/sessions/data/realtime_session_client.dart` — Dep: T032, T002. Verify: `flutter test test/features/sessions/data` green.
- [ ] T036 [US1] (senior-flutter-mobile-engineer) Implement `mobile/lib/features/sessions/application/queue_controller.dart` making T032 green (Riverpod, no `setState`) and integrate with existing connect/reconnect/leave/end lifecycle in `mobile/lib/features/sessions/application/session_room_controller.dart` preserving F-005 ownership — Dep: T032, T034, T035. Verify: `flutter test test/features/sessions/application` green.
- [ ] T037 [US1] (senior-flutter-mobile-engineer) Implement Arabic-first RTL queue screens in `mobile/lib/features/sessions/presentation/queue/` making T033 green and embed in `mobile/lib/features/sessions/presentation/session_room_screen.dart` with labels in `session_ui_labels.dart` without hiding F-005 moderation/end controls — Dep: T033, T036. Verify: `flutter test test/widget/sessions/queue` green both directions.
- [ ] T038 [US1] (senior-flutter-mobile-engineer) Write the end-to-end manager journey in `mobile/integration_test/queue_flow_test.dart`: prepare→auto-activation→advance→start→move while reciting→skip→reset→reconnect re-fetch (Docker `halaqaty-flutter-ci:local` image, sequential per-file runs) — Dep: T037, T029. Verify: `flutter test integration_test/queue_flow_test.dart -d linux` green (blocked, not passed, if no device).

**Checkpoint**: US1 independently delivers a durable, automatically-activating, one-reciter-at-a-time queue with audio entitlement through the neutral boundary, plus the Arabic-first manager UI.

---

## Phase 4: User Story 2 — Join late and opt out humanely (Priority: P1)

**Goal**: Late joiners converge to one fair appended position under both population policies; students opt out under either policy with durable logging, no penalty, no progress record; reconnect always returns to PostgreSQL truth.

**Independent Test**: Admit a late student under both policies, exercise approval-required (approve + decline) and auto-approved opt-out, replay/miss/reorder events, and verify one position, zero progress records, and authoritative REST recovery.

### Tests for User Story 2 (write first — red)

- [ ] T039 [P] [US2] (senior-golang-developer) Write unit tests in `backend/internal/queue/optout_service_test.go`: late-join appends exactly once at the end under both policies (`all_active_students` fires only for members added after activation), pending-request uniqueness per entry, approve → `opted_out` with audio revoked when reciting and durable audit, decline → request terminally closed + entry stays `waiting` (CHK005), auto-approve idempotent direct transition with no pending state, pending-at-finalization → later decision clean 409, zero progress/penalty records — Dep: T009, T012. Verify: red run evidence.
- [ ] T040 [P] [US2] (senior-golang-developer) Write integration tests in `backend/tests/integration/recitation_queue_late_join_test.go` and `backend/tests/integration/recitation_queue_opt_out_test.go`: admitted late joiners without duplicate entries/positions under concurrency, opt-out acceptance matrix with audit attribution and revoked active-reciter audio — Dep: T012. Verify: red run evidence with `DATABASE_URL`.
- [ ] T041 [P] [US2] (senior-golang-developer) Write BEHAVIORAL contract tests in `backend/tests/contract/recitation_queue_opt_out_contract_test.go` (runtime behavior — document parity is T070): POST opt-out (200 idempotent/201) and POST opt-out-requests/{id}/decision shapes, student-self vs manager authorization, targeted manager-only `queue.opt_out_requested` delivery, redacted versioned `queue.entry_updated` — Dep: T024. Verify: red run evidence.

### Implementation for User Story 2

- [ ] T042 [US2] (senior-golang-developer) Implement `backend/internal/queue/optout_service.go` and the late-join append hook in `backend/internal/queue/round_service.go` making T039/T040 green; wire the T017 participant-join observer in `backend/internal/sessions/session_service.go` + `backend/cmd/api/main.go` (join success never depends on the callback) — Dep: T039, T040, T026, T017. Verify: unit + integration green.
- [ ] T043 [US2] (senior-golang-developer) Add opt-out request and decision handlers with student-self and manager authorization to `backend/internal/queue/handler.go` + route patterns in `backend/cmd/api/routes.go`, making T041 green — Dep: T041, T042, T029. Verify: `go test -tags=contract -run RecitationQueueOptOut ./tests/contract/` green.
- [ ] T044 [US2] (senior-golang-developer) Project targeted manager-only `queue.opt_out_requested` from outbox rows in `backend/internal/queue/outbox.go` (auto-approved opt-outs emit nothing) — Dep: T042, T030. Verify: integration assertion that only current teachers/supervisors receive it.

### Mobile for User Story 2

- [ ] T045 [P] [US2] (senior-flutter-mobile-engineer) Extend `mobile/test/features/sessions/application/queue_controller_test.dart` (red first): duplicate/out-of-order/unknown event handling, reconnect snapshot replacement, student-only opt-out command with pending/declined/approved/auto-approved feedback states — Dep: T032. Verify: red run evidence.
- [ ] T046 [P] [US2] (senior-flutter-mobile-engineer) Write red Arabic-first RTL+LTR widget tests in `mobile/test/widget/sessions/queue/queue_student_panel_test.dart`: durable student position, opt-out request/feedback states, reconnecting banner + re-fetch, empty-state student guidance — Dep: T045. Verify: red run evidence both directions.
- [ ] T047 [US2] (senior-flutter-mobile-engineer) Implement the student surface: opt-out command + policy-aware Arabic feedback in `mobile/lib/features/sessions/application/queue_controller.dart` and student queue screen under `mobile/lib/features/sessions/presentation/queue/` making T045/T046 green — Dep: T045, T046, T036, T037. Verify: `flutter test` on the two files green.
- [ ] T048 [US2] (senior-flutter-mobile-engineer) Write the late-join + opt-out + duplicate-event + reconnect journey in `mobile/integration_test/queue_late_join_opt_out_test.dart` — Dep: T047, T043. Verify: integration test green (blocked, not passed, without device/backend).

**Checkpoint**: US2 independently handles fair late admission, humane opt-out under both policies, duplicate delivery, and reconnect recovery.

---

## Phase 5: User Story 3 — Grade completed recitations and preserve history (Priority: P1)

**Goal**: Completion, grading, exactly-one progress record, audited correction, reset, and session-end convergence remain atomic, auditable, and independent of F-005 session end.

**Independent Test**: Complete turns with every grade and with no grade (grading-optional), retry/concurrently repeat commands, correct grades under all three policies, reset, end the session under forced failures — prove one progress row per completion, none for skipped/opted-out, preserved immutable history, inert never-activated rounds, and non-blocking F-005 end. (No history UI — mobile shows the current/latest round snapshot only, read-only when finalized; I3.)

### Tests for User Story 3 (write first — red)

- [ ] T049 [P] [US3] (senior-golang-developer) Write unit tests extending `backend/internal/queue/turn_service_test.go`: all five grades, 500/501-char note boundary, grading-required atomic completion (no `completed` without grade), grading-optional completion carries no grade/note, correction under `audited_any_time`/`before_round_finalization`/`immutable` incl. repeated-identical convergence and note-clear-to-NULL (CHK035), `test`-type record retained, skipped/opted-out → zero progress rows — Dep: T020, T026. Verify: red run evidence.
- [ ] T050 [P] [US3] (senior-golang-developer) Write integration tests in `backend/tests/integration/recitation_queue_completion_test.go`: exactly one `memorization_progress` row per completed entry under retries and conflicting concurrent completions (`queue_entry_id UNIQUE` upsert), zero for skipped/opted-out (SC-004) — Dep: T012. Verify: red run evidence.
- [ ] T051 [P] [US3] (senior-golang-developer) Write integration tests in `backend/tests/integration/recitation_queue_reset_history_test.go`: both finalization policies (`mark_unfinished_skipped`/`preserve_last_state`), next-round numbering, immutable prior rounds with no reused positions/states, never-activated prepared rounds finalized inert at session end, selection cleared, audio revoked — Dep: T012. Verify: red run evidence.
- [ ] T052 [P] [US3] (senior-golang-developer) Write integration tests in `backend/tests/integration/recitation_queue_session_end_test.go` (SC-007): F-005 end commits and returns before/independent of queue cleanup, idempotent convergence retry to finalized non-actionable rounds within the 10 s target, parked-retry exhaustion is the observable terminal outcome, ended-session result never modified — Dep: T012. Verify: red run evidence.
- [ ] T053 [P] [US3] (senior-golang-developer) Write BEHAVIORAL contract tests in `backend/tests/contract/recitation_queue_grading_contract_test.go` (runtime behavior — document parity is T070): PUT status `completed` conditional grade/note rules (422 otherwise), POST grade correction shapes + policy boundaries (409/422), reset snapshot, visibility-filtered grade projections for all three `queue_grade_visibility` values, redacted audit events — Dep: T024. Verify: red run evidence.

### Implementation for User Story 3

- [ ] T054 [US3] (senior-golang-developer) Implement completion and correction in `backend/internal/queue/turn_service.go` + progress insert/correction statements in `backend/internal/queue/queue_queries.go`/`queue_repository.go` making T049/T050 green: atomic completion+progress, correction updating entry and the same one progress row with redacted audit — Dep: T049, T050, T027. Verify: unit + integration green.
- [ ] T055 [US3] (senior-golang-developer) Implement reset finalization + next-round creation in `backend/internal/queue/round_service.go` making T051 green (finalization policy, history immutability, invariant activates next prepared round) — Dep: T051, T026. Verify: integration green.
- [ ] T056 [US3] (senior-golang-developer) Implement `backend/internal/queue/convergence.go` making T052 green: session-end finalization (revoke reciter audio, finalize active + every never-activated prepared round as permanently inert, clear selection, idempotent retry, ≤ 10 s target) + restart reconciliation pass (re-apply activation invariant, finalize ended sessions, replay pending/parked outbox, reconcile audio to PG — CHK033); wire the T017 session-end observer in `backend/internal/sessions/session_service.go` + startup in `backend/cmd/api/main.go` — Dep: T052, T018, T017, T055. Verify: session-end integration green; convergence lag metric recorded.
- [ ] T057 [US3] (senior-golang-developer) Add grading handlers making T053 green: PUT status `completed` conditional-grade semantics and POST `.../entries/{entryId}/grade` correction with policy boundaries and idempotency in `backend/internal/queue/handler.go` + routes in `backend/cmd/api/routes.go` — Dep: T053, T054, T029. Verify: `go test -tags=contract -run RecitationQueueGrading ./tests/contract/` green.
- [ ] T058 [US3] (senior-golang-developer) Project visibility-filtered `queue.grade_submitted` (send-time recipient filtering, omitted `graded_by`) and `queue.round_finalized` (`reason` ∈ {`reset`,`session_ended`}) from outbox rows in `backend/internal/queue/outbox.go` — Dep: T054, T056, T030. Verify: integration assertions per visibility policy; notes never in delivery logs.

### Mobile for User Story 3 (current-round surface only — no history navigation, I3)

- [ ] T059 [P] [US3] (senior-flutter-mobile-engineer) Extend `mobile/test/features/sessions/application/queue_controller_test.dart` (red first): atomic completion payloads, grade visibility per policy, audited correction flows, reset snapshot replacement, terminal read-only finalized state, queue-failure isolation from session end — Dep: T032. Verify: red run evidence.
- [ ] T060 [P] [US3] (senior-flutter-mobile-engineer) Write red Arabic-first RTL+LTR widget tests in `mobile/test/widget/sessions/queue/queue_grading_panel_test.dart`: five-grade selector, note 500-char boundary feedback, visibility-aware grade display, correction under policy, reset confirmation, terminal read-only round state with session-end control still available — Dep: T059. Verify: red run evidence both directions.
- [ ] T061 [US3] (senior-flutter-mobile-engineer) Implement grading/correction/reset UI and controller commands in `mobile/lib/features/sessions/application/queue_controller.dart`, `mobile/lib/features/sessions/data/queue_api_client.dart`, and `mobile/lib/features/sessions/presentation/queue/` making T059/T060 green (read-only latest-round view = current surface only) — Dep: T059, T060, T047. Verify: `flutter test` on affected files green.
- [ ] T062 [US3] (senior-flutter-mobile-engineer) Write the complete→correct→reset→session-end journey in `mobile/integration_test/queue_grading_history_test.dart` (session end stays usable under forced queue/media failure) — Dep: T061, T057. Verify: integration test green (blocked, not passed, without device/backend).

**Checkpoint**: US3 independently produces trustworthy exactly-once progress history, audited correction, reset preservation, and non-blocking session-end convergence.

---

## Phase 6: Cross-Cutting Verification, Reliability, Coverage & Review Gates

**Purpose**: Enforce the A1 reliability parameters, SC-008 latency, SC-005 safety, contract-catalog parity, the constitution's ≥80% coverage floor, full quality gates, and the consolidated Tech Lead deep review. All tasks here are part of MVP completion.

- [ ] T063 [P] (senior-golang-developer) Write A1 reliability-parameter tests in `backend/tests/integration/recitation_queue_reliability_test.go`: forced delivery failure → exactly initial + 5 exponential-backoff retries (+1/+2/+4/+8/+16 s, jittered) → parked with `parked_at` + alert + operator replay (never silent); `ReciterAudioControl` 5 s timeout non-blocking with queue truth committed; session-end convergence ≤ 10 s with idempotent retry; app-restart replay of pending + parked outbox and closed/missing media room convergence (CHK033) — Dep: T056, T018. Verify: green with `DATABASE_URL` (fake clock / injected timers where determinism requires).
- [ ] T064 [P] (senior-golang-developer) Write the SC-008 performance verification in `backend/tests/performance/recitation_queue_delivery_performance_test.go` using the plan protocol ONLY: p95 ≤ 500 ms from PG queue-mutation commit to dispatch to connected authorized clients, ≥ 100 committed actions per scenario, standard local-network environment, disconnected clients excluded (they recover via FR-009 re-fetch); metrics-based (`event_delivery_lag`), no production-path tracing code — Dep: T030, T029, T015. Verify: green run with recorded p95 per scenario.
- [ ] T065 [P] (senior-golang-developer) Write response-safety tests in `backend/tests/contract/recitation_queue_response_safety_test.go` (SC-005 extended surface): REST bodies, events, errors, headers, redirects, logs, audit payloads, metrics labels, receipts, outbox metadata, and client-visible URLs contain no media credential, endpoint, room reference, provider identifier, stack trace, SQL, or internal detail — Dep: T058, T044. Verify: green.
- [ ] T066 [P] (senior-golang-developer) Write media-boundary/architecture tests in `backend/tests/contract/recitation_queue_media_boundary_test.go`: `backend/internal/queue` imports no LiveKit/provider SDK types and calls only `sessions.ReciterAudioControl`; provider imports confined to `backend/internal/sessions/livekit/` (+ mobile `livekit_media_session.dart` untouched by queue code); video grants always disabled — Dep: T058, T016. Verify: green.
- [ ] T067 [P] (senior-golang-developer) Write rate-limit tests in `backend/tests/integration/recitation_queue_rate_limit_test.go`: per-IP and per-user limits on queue reads and mutations → standard 429 with zero state mutation — Dep: T043, T057. Verify: green.
- [ ] T068 [P] (senior-golang-developer) Write adversarial-input tests in `backend/tests/integration/recitation_queue_security_test.go` (SC-003 full matrix): malformed IDs/bodies, invalid enums/Quran ranges/notes/transitions/policy values, order membership+uniqueness violations, stale versions, reused idempotency keys, unauthenticated/current-device failures, non-members, unsupported methods — all with zero mutation — Dep: T057. Verify: green.
- [ ] T069 [P] (senior-flutter-mobile-engineer) Write cross-story Flutter resilience tests in `mobile/test/widget/sessions/session_room_queue_resilience_test.dart`: event dedup, reconnect REST snapshot replacement, Arabic RTL both directions, unauthorized controls hidden per role, queue-failure isolation from leave/end controls (FR-016) — Dep: T062. Verify: green.
- [ ] T070 [P] (architect) CONTRACT-CATALOG PARITY CHECK (documents only — runtime behavior is owned by T024/T041/T053; no overlap): verify canonical `docs/contracts/openapi.yaml` + `docs/contracts/ws_events.md` match `specs/003-recitation-queue-system/contracts/recitation-queue.openapi.yaml` + `recitation-queue.ws_events.md` on every F-003 operation, schema, status code, `Idempotency-Key`, event name/payload/audience, with no duplicate keys and no history/FCM/timer surface anywhere (CHK044); fix drift if found — Dep: T057, T058. Verify: `$docs-guard` clean + `make api-lint` green + parity report appended to the PR.
- [ ] T071 (senior-golang-developer) **Coverage gate (constitution §VI — ≥80%)**: add `test-feature-003-coverage` target to `backend/Makefile` (mirroring the feature-001 targets) that runs `go test -short -coverprofile` across `backend/internal/` packages, computes the AGGREGATE coverage over `backend/internal/` (queue, sessions, platform additions), and FAILS below 80%; run it and record the number — Dep: T063, T064, T065, T066, T067, T068. Verify: target green with aggregate ≥ 80% printed; this task BLOCKS T072/T074 — a lower number is a failure, not a waiver.
- [ ] T072 (senior-golang-developer) Run and document the full backend gate suite from `specs/003-recitation-queue-system/quickstart.md`: fresh migration up/down/up, `go test -short ./...`, `-tags=contract`, `-tags=integration` (needs `DATABASE_URL`), `-race ./internal/queue ./internal/sessions ./internal/realtime`, `golangci-lint run ./...`, `gofmt -l .`, `make api-lint`, `gitleaks detect --source .`, plus the T071 coverage target — Dep: T070, T071. Verify: all green; evidence recorded; unavailable environment = blocked, never passed.
- [ ] T073 (senior-flutter-mobile-engineer) Run and document the full mobile gate suite from `specs/003-recitation-queue-system/quickstart.md` (Docker fallback per AGENTS.md when host SDK is absent): `flutter test test`, `flutter test integration_test/` (sequentially, up to 3 retries per file), `flutter analyze`, `dart format --set-exit-if-changed .` — Dep: T069. Verify: all green with fresh output; missing SDK/device/backend = blocked, never passed.
- [ ] T074 **GATE B — Tech Lead consolidated deep review** (tech-lead) Karim's mandatory security/correctness review over the complete F-003 services/handlers/outbox/convergence diff, plus the Ponytail over-engineering lens and FR-011 scope check (no history UI/REST, no FCM, no timers, no dashboards): (1) role checks on ALL queue mutation operations incl. opt-out student-self rule; (2) one-reciter invariant + audio grant/revoke paths incl. timeout/retry/convergence; (3) correction + audit redaction (no note/grade leakage); (4) session-end independence (F-005 result unmodified, ≤ 10 s convergence, parked exhaustion observable) — Dep: T072, T073, T070. Verify: recorded sign-off; every actionable finding fixed and re-verified in one bounded wave before merge. **Feature is not complete without this gate.**

---

## Acceptance-Criterion Traceability

| Acceptance scenario | Test tasks | Implementation tasks |
|---|---|---|
| US1-1 prepare, order, auto-activate, one position | T019, T022, T025, T031, T033 | T026, T029, T034, T037 |
| US1-2 reorder/move durably without duplicates | T019, T022, T023, T024 | T026, T029 |
| US1-3 advance selects only; no audio | T020, T023, T024 | T027, T029 |
| US1-4 start = sole reciter + audio grant | T020, T022, T023 | T016, T027, T029 |
| US1-5 revoke before next reciter | T020, T022, T023 | T016, T027 |
| US2-1 late join appends/retains exactly once | T039, T040 | T042 |
| US2-2 approved opt-out, logged, no progress | T039, T040, T041 | T042, T043, T044 |
| US2-3 auto-approve idempotent, no pending/progress | T039, T040, T041 | T042, T043 |
| US2-4 dedup + re-fetch after reconnect | T032, T041, T045, T069 | T030, T035, T036, T047 |
| US3-1 atomic required grade, note boundary | T049, T050, T053 | T054, T057 |
| US3-2 exactly one progress record under retry/concurrency | T050 | T054 |
| US3-3 skipped/opted-out create no progress | T049, T050 | T054 |
| US3-4 reset finalizes, preserves history, next round activates | T051, T053 | T055, T057 |
| US3-5 audited correction updates the same record | T049, T053 | T054, T057 |
| FR-014/SC-007 session-end independence + inert rounds | T052, T063 | T056, T058 |
| SC-008 latency / A1 parameters | T063, T064 | T015, T018, T030 |

## Dependencies and Execution Order

### Phase dependencies

- Phase 1 (canonical sync) has no prerequisite and BLOCKS the migration (T004/T005) and all Flutter DTO work (T031+).
- Phase 2 blocks every user story. Gate A (T006) blocks all queue code (T007+).
- All three stories are P1; execute US1 → US2 → US3 (US2 consumes the active-round core; US3 consumes terminal-entry/history behavior); each retains its independent acceptance test.
- Phase 6 depends on all three stories and is part of MVP completion; T071 (coverage) and T074 (Gate B) are blocking.

### Critical dependency chain (one line)

`T001→T003 → T004→T005 → T006(Gate A) → T007→T008→T009→T010→T011→T012 → T015→T018 → T019→T026→T027→T029 → T039→T042→T043 → T049→T054→T057→T058 → T063→T071→T072 → T074(Gate B)` — with the mobile branch `T031→T034→T035→T036→T037→T038 → …T047→T048 → …T061→T062→T069→T073` merging at T074.

Audit (T014), metrics (T015), and error-handling (T013) are inside the blocking chain: T014 gates T021 (policy audit tests), T015 gates T018 (outbox alerting/lag metrics), and T013 gates every handler task — implementation cannot complete without them.

### Parallel opportunities

- Phase 1: T001 ∥ T002 ∥ T003 (different canonical files).
- Phase 2: T013 ∥ T014 ∥ T015 ∥ T016 ∥ T017 beside the T007→T012 chain; T018 joins after T012+T015.
- Per story: all red test tasks run in parallel (US1: T019–T025, T031–T033; US2: T039–T041, T045–T046; US3: T049–T053, T059–T060).
- Backend and mobile tracks run in parallel per story once the story's contracts/services land (mobile DTOs require only T001/T002).
- Phase 6: T063–T070 all parallel; then T071 → T072 ∥ T073 → T074.

### Parallel example: User Story 1

```text
Wave 1 (red tests): T019 ∥ T020 ∥ T021 ∥ T022 ∥ T023 ∥ T024 ∥ T025 ∥ T031 ∥ T032 ∥ T033
Wave 2 (backend):   T026 → (T027 ∥ T028) → T029 → T030
Wave 3 (mobile):    T034 → (T035 ∥ T037-prep) → T036 → T037 → T038
```

## Implementation Strategy

### MVP First (User Story 1 only is a valid demo checkpoint)

1. Phase 1 canonical sync + Phase 2 foundation incl. Gate A.
2. US1 complete → STOP and VALIDATE the independent test (auto-activation + one reciter).
3. US2 → US3 → Phase 6 verification + coverage gate + Gate B.

### Scope guards (violations reopen the plan)

- No history-list REST endpoints, no mobile history UI/operations (I3 — F-007); the read-only finalized latest round via `GET /sessions/{id}/queue` is the current surface (D12).
- No FCM/device-token work (I4 — F-008); no timers, dashboards, activate endpoint, or video.
- No provider types outside `backend/internal/sessions/livekit/` and `mobile/lib/features/sessions/data/livekit_media_session.dart` (ADR-015).
- Unavailable SDK/device/database = blocked gate, never a pass; mark `[X]` only with current verification evidence.

## Prior /speckit.analyze Findings — Structural Resolution (verify here)

| Finding | Resolution in this task list |
|---|---|
| C2 (coverage never enforced) | T071: dedicated blocking coverage gate — aggregate ≥ 80% over `backend/internal/` via new `backend/Makefile` target, fails below, gates T072/T074. |
| T1a (audit/metrics/error tasks off critical path) | T013/T014/T015 sit in Phase 2 and gate T018/T021/all handlers; re-verified in T072. |
| T1b (multiple reviewers per task) | Every task above has exactly ONE owner agent; reviews consolidated into Gate A (T006) and Gate B (T074), both tech-lead-only. |
| T1c (duplicate contract verification) | Behavioral runtime contract tests T024/T041/T053 (senior-golang-developer) are separate from the document-parity check T070 (architect); one owner each, no overlap. |
| I3 (mobile history tasks must not exist) | No history UI/operation tasks; mobile read surface is the current-queue snapshot only (T031 note, T061 read-only latest round). |

## Next Command

Run `/speckit.analyze` to check `spec.md`, `plan.md`, `data-model.md`, contracts, and this task list for consistency before `/speckit.implement`.
