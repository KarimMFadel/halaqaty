# Tasks: Live Sessions (LiveKit)

**Input**: `spec.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/`  
**Prerequisites**: F-001 auth/device sessions, F-002 circle membership/roles, ADR-015, ADR-016  
**Tests**: Required by the F-005 specification and constitution

## MVP Scope

F-005 implements audio-only ad-hoc sessions, presence, hand state, shared realtime transport, and the single LiveKit adapter. Do **not** implement F-003 queue/reciter behavior, F-004 chat, F-006 scheduling/attendance policy, F-008 push delivery, video, screenshare, recording, transcription, or multi-provider selection.

## Phase 0: Canonical Contract Gate (Blocks all implementation)

- [x] T001 [US1] Synchronize F-005 REST operations, schemas, errors, no-store responses, and signed webhook callback from `specs/005-live-sessions-livekit/contracts/live-sessions.openapi.yaml` into `docs/contracts/openapi.yaml`.
- [x] T002 [US1] Synchronize topic authorization, snapshots, session lifecycle, hand, moderation, deduplication, and reconnect semantics from `specs/005-live-sessions-livekit/contracts/live-sessions.ws_events.md` into `docs/contracts/ws_events.md`.
- [x] T003 [US1] Reconcile teacher/supervisor lifecycle wording and F-005/F-006 boundaries in `docs/management/product/FEATURES.md` and `docs/management/product/JOURNEY.md`.
- [x] T004 [US1] Apply the docs-guard checklist manually against `docs/contracts/openapi.yaml`, `docs/contracts/ws_events.md`, `docs/engineering/architecture/ADR-015-session-media-provider-boundary.md`, and `docs/engineering/architecture/adr/ADR-016-session-realtime-and-presence-foundation.md`; run `make api-lint` and resolve findings.

## Phase 1: Shared Foundation (Blocks all stories)

- [x] T005 [US1] Create the complete F-005-owned `sessions` table, its lifecycle/media/lock/end-reason constraints, and `session_participant_presence` in paired `backend/migrations/000016_live_sessions.up.sql` and `backend/migrations/000016_live_sessions.down.sql`; F-003 and F-006 must extend it only through later paired migrations.
- [x] T006 [US1] Add fresh-schema, upgrade, rollback, rerun-safety, constraint, and 51st-participant race coverage for the complete F-005 migration in `backend/tests/integration/live_sessions_migration_test.go`.
- [x] T007 [US1] Define provider-neutral session entities, lifecycle errors, `SessionMediaGateway`, and `ReciterAudioControl` in `backend/internal/sessions/session_types.go` and `backend/internal/sessions/media_gateway.go`.
- [x] T008 [P] [US1] Add validated LiveKit endpoint/API-key/secret and audio-policy configuration without logging secrets in `backend/internal/platform/config/livekit_config.go` and `backend/internal/platform/config/livekit_config_test.go` (package follows per-domain file naming).
- [x] T009 [P] [US1] Add generic realtime ticket/topic and connection state types in `backend/internal/realtime/types.go` and `backend/internal/realtime/types_test.go`.
- [x] T010 [US1] Add package-level session SQL and repository operations for CAS lifecycle, capacity, presence, reconnect, lock, removal, hand state, and snapshots in `backend/internal/sessions/session_queries.go` and `backend/internal/sessions/session_repository.go`.
- [x] T011 [US1] Add session repository transaction/idempotency tests in `backend/internal/sessions/session_repository_test.go`.
- [x] T012 [US1] Add centralized F-005 route patterns in `backend/cmd/api/routes.go` and route-constant coverage in `backend/cmd/api/routes_test.go`.

**Checkpoint**: T001–T012 complete; schema, contracts, provider-neutral types, and route names are ready.

## Phase 2: User Story 1 — Start and Join an Audio Session (P1) 🎯 MVP

**Goal**: A teacher/supervisor starts one audio-only room and a member joins with only their own least-privilege connection.

- [x] T013 [P] [US1] Write lifecycle, authorization, capacity, idempotent-start, and student-publish-denial tests in `backend/internal/sessions/session_service_test.go`.
- [x] T014 [P] [US1] Write start/join response, no-store header, error-envelope, credential-isolation, and response-safety contract tests in `backend/tests/contract/live_sessions_start_join_contract_test.go`.
- [x] T015 [P] [US1] Write concurrent start/join, cross-circle denial, stale membership, full room, and backend-session revocation integration tests in `backend/tests/integration/live_sessions_start_join_test.go`.
- [x] T016 [P] [US1] Write Arabic/RTL room loading, student listen-only, and start/join error widget tests in `mobile/test/widget/sessions/session_room_start_join_test.dart`.
- [x] T017 [US1] Implement session creation/start/join policy, current-device/session/membership checks, CAS transitions, capacity, and idempotent connection issuance in `backend/internal/sessions/session_service.go`.
- [x] T018 [US1] Implement the typed audio-only LiveKit adapter with one-hour identity-specific connections, `CanPublishVideo=false`, Quran audio settings, and no recording path in `backend/internal/sessions/livekit/adapter.go` and `backend/internal/sessions/livekit/adapter_test.go`.
- [x] T019 [US1] Implement signed LiveKit webhook verification and neutral duplicate-safe translation in `backend/internal/sessions/livekit/webhook.go` and `backend/internal/sessions/livekit/webhook_test.go`.
- [x] T020 [US1] Implement F-005 REST request validation, standard error mapping, no-store responses, and audit-safe handlers in `backend/internal/sessions/handler.go` and `backend/internal/sessions/handler_test.go`.
- [x] T021 [US1] Wire session handlers, auth/session middleware, request limits, deadlines, and signed webhook route in `backend/cmd/api/router.go` and `backend/cmd/api/router_test.go`.
- [x] T022 [US1] Add session API models and authenticated Dio calls that retain `MediaConnection` only in memory in `mobile/lib/features/sessions/data/session_api_client.dart` and `mobile/lib/features/sessions/domain/session_models.dart`.
- [x] T023 [US1] Define provider-neutral `MediaSession` and the sole LiveKit client adapter in `mobile/lib/features/sessions/application/media_session.dart` and `mobile/lib/features/sessions/data/livekit_media_session.dart`.
- [x] T024 [US1] Implement Riverpod start/join state, error boundary, and credential-safe connection lifecycle in `mobile/lib/features/sessions/application/session_room_controller.dart` and `mobile/test/features/sessions/application/session_room_controller_test.dart`.
- [x] T025 [US1] Implement Arabic-first session-room shell, participant loading state, audio state, and start/join actions in `mobile/lib/features/sessions/presentation/session_room_screen.dart` and `mobile/lib/features/sessions/presentation/session_ui_labels.dart`.
- [x] T026 [US1] Run focused backend and Flutter US1 suites, fixing only US1 failures in `backend/internal/sessions/`, `backend/tests/contract/live_sessions_start_join_contract_test.go`, `backend/tests/integration/live_sessions_start_join_test.go`, and `mobile/test/widget/sessions/session_room_start_join_test.dart`.

**Checkpoint**: Teacher/supervisor start and member join work independently with secure audio-only connections.

## Phase 3: User Story 2 — Run and Moderate a Safe Room (P1)

**Goal**: Teachers/supervisors safely moderate; participants see durable presence/hand state without queue behavior.

- [x] T027 [P] [US2] Write service tests for equal moderator rights, lock/pre-lock reconnect, remove, mute/unmute entitlement preservation, hand state, automatic end, and idempotency in `backend/internal/sessions/session_moderation_service_test.go`.
- [x] T028 [P] [US2] Write moderation/presence REST and WebSocket response-safety, RBAC-denial, and no-provider-leak contract tests in `backend/tests/contract/live_sessions_moderation_contract_test.go`.
- [x] T029 [P] [US2] Write duplicate webhook, remove/reconnect denial, lock race, duration/idle end, audit-redaction, and rate-limit integration tests in `backend/tests/integration/live_sessions_moderation_test.go`.
- [x] T030 [P] [US2] Write RTL moderator-controls, participant hand state, and denied-control widget tests in `mobile/test/widget/sessions/session_room_moderation_test.dart`.
- [x] T031 [US2] Extend `backend/internal/sessions/session_service.go` with lock/unlock, mute-all, mute/unmute existing publishers, remove, hand raise/lower, duration/idle end, and redacted audit events.
- [x] T032 [US2] Extend typed moderation operations only in `backend/internal/sessions/livekit/adapter.go` and expose no student publish-grant operation outside `ReciterAudioControl`.
- [x] T033 [US2] Implement generic ticket issue, heartbeat, three-connection/user limit, 30-message/minute limit, circle/session topic authorization, snapshots, and idempotent event broadcasting in `backend/internal/realtime/hub.go`, `backend/internal/realtime/ticket_service.go`, and `backend/internal/realtime/hub_test.go`.
- [x] T034 [US2] Extend `backend/internal/sessions/handler.go` and `backend/cmd/api/router.go` with moderation, participant snapshot, and realtime-ticket operations plus rate-limit/error mapping.
- [x] T035 [US2] Add realtime subscription, presence/hand models, and session moderation API calls in `mobile/lib/features/sessions/data/realtime_session_client.dart` and `mobile/lib/features/sessions/data/session_api_client.dart`.
- [x] T036 [US2] Extend Riverpod session state with deduplicated snapshots/events, hand commands, and role-gated moderation actions in `mobile/lib/features/sessions/application/session_room_controller.dart`.
- [x] T037 [US2] Extend the RTL room UI with participant list, hand state, and teacher/supervisor-only controls in `mobile/lib/features/sessions/presentation/session_room_screen.dart`.
- [x] T038 [US2] Run focused US2 backend/mobile tests and verify no queue/chat, video, screen-share, recording, attendance classification, or FCM behavior was introduced in `backend/internal/sessions/`, `backend/internal/realtime/`, and `mobile/lib/features/sessions/`.

### Phase 3 evidence — 2026-08-19

- T028/T029: `go test -tags=contract ./tests/contract -count=1` and `go test -tags=integration ./tests/integration -count=1` passed.
- T030/T035–T037: focused Flutter session tests passed (22 tests); full `flutter test test` passed (69 tests); `flutter analyze` and scoped Dart format passed in the approved Flutter Docker environment.
- T031/T032/T034: `go test -short ./...` passed; moderation contract and integration suites passed.
- T038: scope scan found no Phase 3 implementation of queue, chat, video, screen share, recording, transcription, attendance classification, or FCM behavior.
- T027: `go test ./internal/sessions -run 'TestModeration_' -count=1` passed with equal moderator, reconnect/lock, mute entitlement, hand, removal, end, and idempotency coverage.
- T033: `go test ./internal/realtime -run 'TestHub_' -count=1` passed with snapshot, command, heartbeat, connection/message limits, topic authorization, and deduplicated broadcast coverage; production wiring is in `backend/cmd/api/main.go`.

**Checkpoint**: Room moderation, privacy-scoped realtime presence, and standalone hand raising work without F-003/F-004/F-006.

## Phase 4: User Story 3 — Recover from Connection Loss (P2)

**Goal**: Participants recover safely from transient loss and receive clear terminal outcomes.

- [X] T039 [P] [US3] Write backend tests for server-selected join versus eligible pre-lock reconnect, fresh-ticket snapshot rehydration, expiry refresh, terminal authorization, and the ADR-017 reconciler seam in `backend/internal/sessions/session_recovery_service_test.go` and `backend/internal/realtime/hub_reconnect_test.go`.
- [X] T040 [P] [US3] Write reconnect ticket/topic, terminal authorization, duplicate-presence, `503 ERR_MEDIA_UNAVAILABLE`, and no-presence-mutation contract tests in `backend/tests/contract/live_sessions_reconnect_contract_test.go`.
- [X] T041 [P] [US3] Write provider create/close crash-window, stable-room orphan cleanup, advisory-lock race, bounded retry, missing-room idempotency, and reconnect integration tests in `backend/tests/integration/live_sessions_recovery_test.go`.
- [X] T042 [P] [US3] Write Flutter recoverable/terminal reconnect, credential refresh, removal, end, lock, and Arabic error widget tests in `mobile/test/widget/sessions/session_room_reconnect_test.dart`.
- [ ] T043 [US3] Implement ADR-017 reconciliation: HMAC-derived stable room references, shared session advisory locking, startup/30-second bounded sweeps (25 candidates per state), one 3-second provider attempt per candidate, scheduled/active/ended candidate queries, orphan cleanup, and no new recovery persistence in `backend/internal/sessions/reconciler.go` and `backend/internal/sessions/reconciler_test.go`.
- [X] T044 [US3] Extend server-selected reconnect authorization, fresh-ticket subscription restoration, and authoritative snapshot rehydration in `backend/internal/realtime/hub.go` and `backend/internal/realtime/hub_reconnect_test.go`.
- [X] T045 [US3] Implement media reconnect/credential-refresh policy with explicit recoverable versus terminal states in `mobile/lib/features/sessions/application/session_room_controller.dart` and `mobile/lib/features/sessions/application/media_session.dart`.
- [X] T046 [US3] Add Arabic-first recover/retry/leave terminal UI states in `mobile/lib/features/sessions/presentation/session_room_screen.dart` and `mobile/lib/features/sessions/presentation/session_ui_labels.dart`.
- [ ] T047 [US3] Run focused US3 backend/mobile suites, including `503 ERR_MEDIA_UNAVAILABLE` with no credential/presence mutation, one LiveKit adapter failure path, one expired-credential refresh path, and bounded Arabic Retry/Leave terminal flows, in `backend/tests/integration/live_sessions_recovery_test.go` and `mobile/test/widget/sessions/session_room_reconnect_test.dart`.

**Checkpoint**: Recovery converges to authoritative state without duplicate presence or an infinite reconnect loop.

### Phase 4 clarification gate — 2026-08-19

ADR-017 and MVP decisions OQ-047–OQ-052 freeze provider outage as recoverable
`503`, stable non-guessable room references, shared lifecycle locking, bounded
30-second reconciliation, end-before-close cleanup, and server-selected
reconnect. T039 must be rewritten to exercise the real durable reconnect path;
the prior test-only fake that defined `ReconnectPresence` but called
`JoinSession` is not completion evidence.

T043 review evidence (2026-08-19): the reconciler seam and focused tests are
present, but the task remains open until production wiring, post-lock rereads,
provider-failure/no-presence mutation, and stable room-key configuration are
closed.

## Phase 5: Cross-Cutting Verification and Review

- [X] T048 [P] [US1] Add dependency-boundary tests proving LiveKit imports/types and `livekit_*` fields are confined to `backend/internal/sessions/livekit/` and `mobile/lib/features/sessions/data/livekit_media_session.dart` in `backend/tests/contract/livekit_boundary_contract_test.go` and `mobile/test/features/sessions/livekit_boundary_test.dart`.
- [X] T049 [P] [US2] Add rate-limit, request-timeout, observability, and audit-redaction coverage in `backend/tests/integration/live_sessions_observability_test.go` and `backend/tests/integration/live_sessions_rate_limit_test.go`.
- [ ] T050 [US1] Apply the clean-code and test-guard checklists manually to all changed production and test paths; record any fixes in `specs/005-live-sessions-livekit/tasks.md`.
- [X] T051 [US1] Run contract lint and backend verification: `make api-lint` and `go test -short ./...` from `backend/`; record current outputs in `specs/005-live-sessions-livekit/validation-report.md`.
- [ ] T052 [US1] Run Flutter verification using the approved Docker workflow: `flutter test test`, `flutter test integration_test/`, `flutter analyze`, and `dart format --set-exit-if-changed .` from `mobile/`; record current outputs in `specs/005-live-sessions-livekit/validation-report.md`.
- [ ] T053 [US1] Run repository lint and secret scans with `make lint` and `make secrets`; record current outputs in `specs/005-live-sessions-livekit/validation-report.md`.
- [ ] T054 [US1] Submit the coherent F-005 diff, acceptance mapping, validation report, and ADR-015/ADR-016 constraints for Tech Lead review; obtain Karim’s mandatory manual review for auth, RBAC, media credentials, and webhook handling.
- [ ] T055 [US1] Add contract coverage for circle session discovery, complete `400/401/403/404/409/422/429/500/503` error mappings, `ERR_MEDIA_UNAVAILABLE`, `Cache-Control: no-store` on media and realtime tickets, and standard error-envelope responses in `backend/tests/contract/live_sessions_contract_completeness_test.go`.
- [ ] T056 [US1] Add signed LiveKit webhook contract/integration coverage for required signature headers, JSON body validation, invalid-signature rejection, duplicate delivery, rate limits, and credential-safe audit output in `backend/tests/contract/livekit_webhook_contract_test.go` and `backend/tests/integration/livekit_webhook_integration_test.go`.
- [ ] T057 [US1] Add session-discovery API/mobile coverage proving the session-card list reuses the canonical circle sessions operation without introducing F-006 scheduling or attendance behavior in `backend/tests/contract/live_sessions_discovery_contract_test.go` and `mobile/test/widget/sessions/session_discovery_test.dart`.
- [ ] T059 [US1] Reuse the canonical `GET /circles/{circleId}/sessions` operation for session-card discovery, wiring the existing backend list flow and Flutter session-card data source without adding scheduling or attendance behavior in `backend/internal/sessions/handler.go`, `backend/cmd/api/router.go`, and `mobile/lib/features/sessions/data/session_api_client.dart`.
- [ ] T058 [US1] Add migration ownership assertions proving F-005 creates the complete `sessions` base table and that F-003/F-006 migrations extend it without redefining lifecycle or attendance ownership in `backend/tests/integration/live_sessions_migration_ownership_test.go`.

## Dependencies and Execution Order

- **Critical gate**: T001 → T002 → T003 → T004. No migration or code task starts before this gate passes.
- **Foundation**: T005 → T006; T007/T008/T009 → T010 → T011; T012 completes route setup. All Foundation tasks complete before US1.
- **MVP chain**: T013–T016 → T017 → T018 → T019 → T020 → T021 → T022 → T025 → T026.
- **Moderation chain**: T027–T030 → T031 → T032/T033 → T034 → T035 → T036 → T037 → T038.
- **Recovery chain**: T039–T042 → T043/T044 → T045 → T046 → T047.
- **Finish**: T048/T049/T055/T056/T057/T058/T059 → T050 → T051/T052/T053 → T054.

## Parallel Opportunities

- After T004, T008 and T009 may run in parallel with T005 because they own separate files and have no schema dependency.
- Within each story, only the explicitly marked test tasks may run in parallel; implementation tasks deliberately remain sequential where they share services, adapters, controllers, or routes.
- T048 and T049 can run in parallel after all story work is complete.
