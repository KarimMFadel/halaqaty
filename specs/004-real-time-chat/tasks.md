# Tasks: Real-time Chat

**Input**: Design documents from `specs/004-real-time-chat/`  
**Prerequisites**: `plan.md`, `spec.md`, `research.md`, `data-model.md`, `contracts/`, `quickstart.md`  
**MVP scope**: All F-004 P1 and P2 stories in Phases 1–11. User Story 1 is the first independently demonstrable checkpoint, not a reduced product MVP.  
**Implementation gate**: ADR-021 is accepted. `/speckit.implement` MUST NOT create F-004 runtime code or migrations until the remediated `/speckit.analyze` has no critical blocker and T002–T004 pass.

## Phase 1: Governance and Contract-First Gate

**Purpose**: Freeze the approved architecture and prove the additive contracts before implementation.

- [x] T001 Record Karim's 2026-09-06 acceptance and Constitution 1.2.0 authorization in `docs/engineering/architecture/adr/ADR-021-chat-persistence-media-and-delivery.md` and `.specify/memory/constitution.md`
- [X] T002 [P] Add canonical-to-feature OpenAPI operation, schema, and response-code parity coverage including pinned retrieval, sender-only read receipts, active-circle read denial, `delivered/read`, and `415` in `backend/tests/contract/chat_openapi_contract_test.go`
- [x] T003 Run the OpenAPI/docs guard and resolve any contract mismatch in `docs/contracts/openapi.yaml`, `docs/contracts/ws_events.md`, `specs/004-real-time-chat/contracts/chat.openapi.yaml`, and `specs/004-real-time-chat/contracts/chat.ws_events.md`
- [X] T004 [P] Add dependency-boundary coverage proving F-004 consumes active F-001/F-002 memberships and F-005 realtime tickets without implementing ADR-010 invitations, LiveKit chat, or F-008 notifications in `backend/tests/contract/chat_dependency_boundaries_test.go`

**Gate**: T001–T004 are complete; the additive REST and WebSocket contracts are authoritative, dependency boundaries are proven, and docs are lint-clean.

---

## Phase 2: Foundational Chat Infrastructure

**Purpose**: Establish shared persistence, validation, media, observability, and realtime seams required by every story.

- [X] T005 Add fresh, upgrade, down, and reapply migration tests that preserve prior-feature data in `backend/tests/integration/real_time_chat_migration_test.go`
- [X] T006 Implement the five F-004 tables, constraints, Arabic normalization function, GIN/index support, and idempotency uniqueness in `backend/migrations/000018_real_time_chat.up.sql`
- [X] T007 Implement dependency-safe rollback of only F-004-owned objects in `backend/migrations/000018_real_time_chat.down.sql`
- [X] T008 [P] Add table-driven tests for exact-one-context, payload compatibility, text length, cursor, MIME, size, duration, and idempotency validation in `backend/internal/chat/validation_test.go`
- [X] T009 Define chat domain types, stable errors, state values, limits, and validation in `backend/internal/chat/models.go` and `backend/internal/chat/validation.go`
- [X] T010 Add repository integration tests for transactions, keyset ordering, current membership-period filtering, unordered DM pairs, and unique read/idempotency facts in `backend/internal/chat/repository_integration_test.go`
- [X] T011 Define all chat SQL as package-level constants, including authorization joins and `FOR UPDATE SKIP LOCKED` claims, in `backend/internal/chat/queries.go`
- [X] T012 Implement transaction-safe chat persistence and query methods in `backend/internal/chat/repository.go`
- [X] T013 [P] Add configuration/Compose tests for official-source MinIO `RELEASE.2025-10-15T17-29-55Z`, secret-free credentials, healthcheck, persistent volume, private chat bucket, required versioning, and bounded timeouts in `backend/internal/platform/config/chat_media_config_test.go` and `backend/tests/integration/chat_minio_compose_test.go`
- [X] T014 Build MinIO from its pinned official source tag in `docker/minio.Dockerfile`; add the service/volume/healthcheck to `docker-compose.yml`, secret-free variables to `.env.example`, chat media configuration to `backend/internal/platform/config/chat_media_config.go`, and approved `github.com/minio/minio-go/v7` dependency to `backend/go.mod` and `backend/go.sum`
- [X] T015 Add adapter tests for private object keys, sanitized metadata, versionless presigning, bucket-versioning enforcement, timeout propagation, 24-hour staged cleanup, and best-effort failed-finalization cleanup in `backend/internal/chat/media_store_test.go`
- [X] T016 Implement the narrow MinIO-backed media store, fail-fast versioned-bucket startup check, and 24-hour unattached cleanup in `backend/internal/chat/media_store.go` and `backend/internal/chat/media_cleanup.go`
- [X] T017 [P] Add tests that chat metrics and audit logs expose identifiers/outcomes but never bodies, filenames, keys, URLs, tokens, or session IDs in `backend/internal/platform/metrics/chat_metrics_test.go` and `backend/internal/platform/logging/chat_audit_test.go`
- [X] T018 Implement chat latency/outcome, denial, upload, outbox, reconnect, and search metrics plus redacted audit events in `backend/internal/platform/metrics/chat_metrics.go` and `backend/internal/platform/logging/chat_audit.go`
- [X] T019 [P] Add regression tests for per-client/session authorization immediately before write, circle-topic group delivery, direct authenticated-user DM delivery across different subscribed circles, revoked-session suppression, chat commands, and unchanged session-topic behavior in `backend/internal/realtime/hub_chat_test.go` and `backend/internal/realtime/types_test.go`
- [X] T020 Extend the generic hub with chat event constants, injectable per-write authorization, and direct eligible-user delivery seams without LiveKit or chat-domain imports in `backend/internal/realtime/types.go` and `backend/internal/realtime/hub.go`

**Gate**: Foundation tests pass; migration rollback is proven; no user story starts before this phase completes.

---

## Phase 3: User Story 1 — Participate in a Circle Group Chat (P1) — Core Checkpoint

**Goal**: Active members can load current-membership-period group history and exchange safe text messages without a live session.

**Independent test**: With no live session, load paginated history, send one valid text message, observe it once on another authorized client, and reject invalid or unauthorized requests without enumeration.

- [X] T021 [US1] Add service tests for active-circle membership-period history, deterministic `(sent_at,id)` pagination, plain-text validation, idempotent sends, and no-session independence in `backend/internal/chat/group_service_test.go`
- [X] T022 [US1] Implement group history and text-send authorization/business behavior in `backend/internal/chat/group_service.go`
- [X] T023 [P] [US1] Add REST contract tests for group list/send, required idempotency, `400/401/403/404/409/422/429` semantics, RBAC denial, rate limits, and response safety in `backend/tests/contract/chat_group_contract_test.go`
- [X] T024 [US1] Implement response-safe group list/send handlers using centralized HTTP constants in `backend/internal/chat/group_handler.go`
- [X] T025 [P] [US1] Add tests for atomic message/outbox insertion, five-attempt bounded delivery, audience rebuild plus per-client/session reauthorization, redacted projection, and parked-event replay in `backend/internal/chat/outbox_test.go`
- [X] T026 [US1] Implement chat outbox creation, claim, retry/backoff/jitter, parking, startup replay, and payload reload in `backend/internal/chat/outbox.go` and `backend/internal/chat/outbox_queries.go`
- [X] T027 [US1] Add effective-once `chat.message` projection tests for duplicate event IDs and currently authorized circle subscribers in `backend/internal/chat/realtime_projector_test.go`
- [X] T028 [US1] Implement group `chat.message` projection through the shared circle topic in `backend/internal/chat/realtime_projector.go`
- [X] T029 [US1] Define group route strings/wiring, dependencies, outbox lifecycle, and 30-per-minute user-and-circle limiter in `backend/internal/api/routes.go` and `backend/internal/api/router.go`; expose only composition-required aliases in `backend/cmd/api/routes.go` and wire them in `backend/cmd/api/main.go`
- [X] T030 [P] [US1] Add model/parser/API-client tests for safe text, cursor pagination, message-ID deduplication, and authenticated group list/send in `mobile/test/features/chat/data/chat_api_client_test.dart` and `mobile/test/features/chat/domain/chat_models_test.dart`
- [X] T031 [US1] Implement feature-local message models, protocol constants, and Dio group list/send calls in `mobile/lib/features/chat/domain/chat_models.dart`, `mobile/lib/features/chat/data/chat_protocol_constants.dart`, and `mobile/lib/features/chat/data/chat_api_client.dart`
- [X] T032 [US1] Add Riverpod controller tests for initial history, pagination, optimistic item replacement, duplicate realtime events, and safe unknown-event reconciliation in `mobile/test/features/chat/application/group_chat_controller_test.dart`
- [X] T033 [US1] Implement authoritative group history state, pagination, send confirmation, and message-ID deduplication in `mobile/lib/features/chat/application/group_chat_controller.dart` and `mobile/lib/features/chat/data/chat_realtime_client.dart`
- [X] T034 [US1] Add Arabic/English widget tests for escaped text, empty/overlong validation, deterministic message ordering, accessible error/status labels, and no-session operation in `mobile/test/widget/chat/group_chat_screen_test.dart`
- [X] T035 [US1] Implement the Arabic-first RTL-aware group thread, composer, message bubble, pagination, and circle navigation entry in `mobile/lib/features/chat/presentation/group_chat_screen.dart`, `mobile/lib/features/chat/presentation/chat_widgets.dart`, and `mobile/lib/features/circles/presentation/circle_detail_screen.dart`
- [X] T036 [US1] Add the no-live-session group history/send/realtime/deduplication acceptance flow in `mobile/integration_test/chat_group_flow_test.dart`

**Core checkpoint**: T001–T036 deliver the first independently usable slice. Continue through every remaining P1/P2 story for the approved F-004 MVP.

---

## Phase 4: User Story 2 — Send Reliably Across Connection Changes (P1)

**Goal**: Offline, retry, restart, reconnect, and redelivery paths converge on one durable message and one visible item.

**Independent test**: Queue while offline, restart/reconnect, repeat REST and WebSocket delivery, and verify the stable idempotency key yields one database row and one UI item.

- [ ] T037 [P] [US2] Add encrypted pending-envelope storage tests for one-item keys, stable idempotency keys, restart reload, local attachment paths, edit, and discard in `mobile/test/features/chat/data/pending_message_store_test.dart`
- [ ] T038 [US2] Implement pending-envelope persistence with the existing `flutter_secure_storage` dependency in `mobile/lib/features/chat/data/pending_message_store.dart`
- [ ] T039 [US2] Add controller/realtime tests for pending→sent→delivered transitions, 1/2/4-second retry eligibility, terminal failures, reconnect history refresh, gaps, duplicates, and unknown events in `mobile/test/features/chat/application/chat_delivery_controller_test.dart` and `mobile/test/features/chat/data/chat_realtime_client_test.dart`
- [ ] T040 [US2] Implement bounded retry, reconnect, authoritative reconciliation, and visible terminal-error edit/discard behavior in `mobile/lib/features/chat/application/chat_delivery_controller.dart` and `mobile/lib/features/chat/data/chat_realtime_client.dart`
- [ ] T041 [P] [US2] Add backend integration coverage for concurrent same-key sends, materially different payload conflicts, accepted-retry rate accounting, outbox crash recovery, redelivery, and PostgreSQL failure rejection in `backend/tests/integration/chat_reliability_test.go`
- [ ] T042 [US2] Add offline/restart/reconnect and duplicate REST/WebSocket acceptance coverage in `mobile/integration_test/chat_offline_recovery_test.dart`

---

## Phase 5: User Story 3 — Share Practice Media (P1)

**Goal**: Members can record and share authorized voice, JPEG/PNG, and PDF attachments through renewable private links.

**Independent test**: Record/preview/send/play a compliant voice note, send compliant image/PDF files, renew links, and reject every MIME, size, duration, ownership, context, and upload-rate violation.

- [ ] T043 [P] [US3] Add upload-service tests for magic-byte MIME plus parse validation, voice 300s/20MB, image 5MB, PDF 10MB, 10 successfully staged uploads/hour/user across devices, uploader/context binding, 24-hour abandoned cleanup, reauthorization, and legacy-unbound-key rejection in `backend/internal/chat/upload_service_test.go`
- [ ] T044 [US3] Implement staged upload, server validation, group/DM context binding, attach-once semantics, 24-hour inaccessible-stage cleanup, and seven-day renewal authorization in `backend/internal/chat/upload_service.go`
- [ ] T045 [P] [US3] Add upload/media-renewal contract tests for auth/session/RBAC denials, route-specific 21 MB cap vs global 1 MiB cap, `413/415/422/429` failures, 60-second timeout, non-enumerating errors, sanitized filenames, and response safety in `backend/tests/contract/chat_media_contract_test.go`
- [ ] T046 [US3] Implement additive voice/image/file upload and media-link renewal handlers with route-specific 21 MB body limits while preserving the global 1 MiB default in `backend/internal/chat/upload_handler.go`, `backend/internal/chat/media_handler.go`, and `backend/internal/api/router.go`
- [ ] T047 [US3] Add real-MinIO integration tests for private objects, supported media, expired/renewed versionless URLs, unauthorized renewal, and failed database-finalization cleanup in `backend/tests/integration/chat_media_test.go`
- [ ] T048 [P] [US3] Add mobile API/recorder tests for `record` permission/interruption/amplitude streams, `just_audio` preview/playback failures, native picker cancellation, codec-neutral supported formats, duration/size limits, upload progress, and renewal in `mobile/test/features/chat/data/chat_media_api_test.dart` and `mobile/test/features/chat/application/voice_note_controller_test.dart`
- [ ] T049 [US3] Add `record ^7.1.1`, `just_audio ^0.10.6`, `image_picker ^1.2.3`, and `file_picker ^12.2.0` to `mobile/pubspec.yaml`/`mobile/pubspec.lock`; implement media API calls and recording/playback state in `mobile/lib/features/chat/data/chat_media_api.dart` and `mobile/lib/features/chat/application/voice_note_controller.dart`
- [ ] T050 [US3] Add Arabic/English accessible widget tests for recording duration/waveform, preview/discard/send, image preview, PDF action, upload errors, and non-color-only progress in `mobile/test/widget/chat/chat_media_widgets_test.dart`
- [ ] T051 [US3] Implement voice recording with amplitude-drawn waveform, foreground preview/playback, native JPEG/PNG and PDF picking, playback/download, and renewable-link UI in `mobile/lib/features/chat/presentation/chat_media_widgets.dart`
- [ ] T052 [US3] Add voice/image/PDF happy paths and all server-enforced limit/authorization boundaries in `mobile/integration_test/chat_media_flow_test.dart`

---

## Phase 6: User Story 8 — Preserve Access Boundaries Through Lifecycle Changes (P1)

**Goal**: Session revocation, removal, rejoin, and archival immediately change access while permitted retained history remains independent of live sessions.

**Independent test**: Revoke a device session, remove/rejoin a member, archive a circle, and verify retained reads/search/media plus denial of every archived mutation and all removed-member realtime access.

- [ ] T053 [P] [US8] Add integration coverage for missing/invalid/revoked/mismatched sessions, removal, rejoin membership periods, archived retained history/search/play, explicit archived mark-read denial, all other archived mutations, per-write realtime cutoff, and non-enumerating denial responses in `backend/tests/integration/chat_lifecycle_security_test.go`
- [ ] T054 [US8] Apply current-session, membership-period, removal, and archived read-only checks consistently across chat services and realtime projection in `backend/internal/chat/authorizer.go`, `backend/internal/chat/group_service.go`, `backend/internal/chat/upload_service.go`, and `backend/internal/chat/realtime_projector.go`
- [ ] T055 [P] [US8] Add controller/widget tests for immediate access loss, new membership-period history, archived read/search/play, disabled mutations, and identical in/out-of-session behavior in `mobile/test/features/chat/application/chat_lifecycle_controller_test.dart` and `mobile/test/widget/chat/archived_chat_screen_test.dart`
- [ ] T056 [US8] Implement lifecycle refresh, access-loss state, archived read-only controls, and session-shell-independent navigation in `mobile/lib/features/chat/application/group_chat_controller.dart`, `mobile/lib/features/chat/presentation/group_chat_screen.dart`, and `mobile/lib/features/sessions/presentation/session_room_screen.dart`
- [ ] T057 [US8] Add end-to-end removal/rejoin/archive/revoked-session behavior with no F-003, F-006, or F-008 dependency in `mobile/integration_test/chat_lifecycle_flow_test.dart`

**P1 checkpoint**: T001–T057 complete all P1 stories.

---

## Phase 7: User Story 4 — Use an Authorized Direct Conversation (P2)

**Goal**: One unordered-pair DM is available only for current teacher–student or supervisor–student relationships in at least one shared active circle.

**Independent test**: Exercise both directions of allowed role pairs, all denied role/circle states, multi-circle media authorization, loss of the last qualifying relationship, and complete-history restoration.

- [ ] T058 [P] [US4] Add repository/service tests for unordered-pair history, allowed role pairs in both directions, every denied pair, any-qualifying-circle media, direct authenticated-user realtime delivery when pair members subscribe to different qualifying circles, last-circle revocation, and full-history restoration in `backend/internal/chat/direct_service_test.go`
- [ ] T059 [US4] Implement current-role/shared-active-circle DM list/send/delete/read/upload/renew authorization and pair-history queries in `backend/internal/chat/direct_service.go`, `backend/internal/chat/repository.go`, and `backend/internal/chat/queries.go`
- [ ] T060 [P] [US4] Add DM contract tests for list/send/own-delete/read/media operations, required idempotency, RBAC matrix denials, cross-circle non-enumeration, rate limits, and response safety in `backend/tests/contract/chat_direct_contract_test.go`
- [ ] T061 [US4] Implement DM handlers, per-write-reauthorized direct-user realtime projection, pair rate-limit keys, and route strings/wiring in `backend/internal/chat/direct_handler.go`, `backend/internal/chat/realtime_projector.go`, `backend/internal/api/routes.go`, and `backend/internal/api/router.go`
- [ ] T062 [P] [US4] Add mobile DM model/API/controller/widget tests for eligibility changes, pair-history restoration, media continuity across qualifying circles, and safe denial UI in `mobile/test/features/chat/application/direct_chat_controller_test.dart` and `mobile/test/widget/chat/direct_chat_screen_test.dart`
- [ ] T063 [US4] Implement direct-chat entry from an eligible shared-circle member context, the direct controller, and Arabic-first thread—without an uncontracted inbox/list endpoint—in `mobile/lib/features/chat/application/direct_chat_controller.dart`, `mobile/lib/features/chat/presentation/direct_chat_screen.dart`, and `mobile/lib/features/circles/presentation/circle_members_screen.dart`
- [ ] T064 [US4] Add both allowed role pairs, all prohibited pairs, cross-circle denial, last-circle loss, multi-circle media, and restored-history acceptance coverage in `mobile/integration_test/chat_direct_flow_test.dart`

---

## Phase 8: User Story 5 — Understand Delivery, Reading, and Typing (P2)

**Goal**: Users see truthful pending/sent/delivered/read state and ephemeral authorized typing indicators.

**Independent test**: Drive a message through all four states, record duplicate reads, and verify typing start/stop/expiry without durable history or unauthorized audience leakage.

- [ ] T065 [P] [US5] Add service/repository tests for idempotent visible-message non-sender reads, sender-only currently authorized `read_receipts`, group headline state, sender-targeted events, pre-join/post-removal/archived denial, restored-DM facts, and non-persisted five-second typing in `backend/internal/chat/presence_service_test.go`
- [ ] T066 [US5] Implement read transactions, headline/detail projections, and transient typing authorization in `backend/internal/chat/presence_service.go` and `backend/internal/chat/repository.go`
- [ ] T067 [P] [US5] Add WebSocket/REST contract tests for sender-only `read_receipts`, group/direct mark-read, archived `409`, server-only `delivered/read`, `chat.message_read`, `cmd.chat.typing`, `chat.typing`, command rate limits, malformed context, RBAC denial, expiry, and response safety in `backend/tests/contract/chat_presence_contract_test.go`
- [ ] T068 [US5] Implement read handlers, sender-targeted outbox delivery, and authorized typing command/broadcast handling in `backend/internal/chat/presence_handler.go`, `backend/internal/chat/realtime_projector.go`, and `backend/internal/realtime/hub.go`
- [ ] T069 [P] [US5] Add mobile tests for all four delivery states, group read details, sender-own-read exclusion, duplicate facts, typing stop loss, and five-second expiry in `mobile/test/features/chat/application/chat_presence_controller_test.dart` and `mobile/test/widget/chat/chat_status_test.dart`
- [ ] T070 [US5] Implement read submission/details and non-color-only status/typing projections in `mobile/lib/features/chat/application/chat_presence_controller.dart` and `mobile/lib/features/chat/presentation/chat_status_widgets.dart`
- [ ] T071 [US5] Add pending→sent→delivered→read and typing start/stop/expiry acceptance coverage for group and DM in `mobile/integration_test/chat_presence_flow_test.dart`

---

## Phase 9: User Story 6 — Reply, Search, and Find Important Messages (P2)

**Goal**: Authorized users can reply safely, search retained group history, and manage a concurrency-safe five-message pinned bar.

**Independent test**: Reply within one conversation, search Arabic/Latin text, pin five messages, reject concurrent sixth pins and unauthorized actors, then verify deletion/archival projections.

- [ ] T072 [P] [US6] Add repository/service tests for same-conversation replies, deleted/missing/cross-context rejection, safe previews, defined Arabic normalization/prefix ordering, membership-period search, pinned retrieval order, and concurrent pin/unpin/delete serialization on the circle row in `backend/internal/chat/discovery_service_test.go`
- [ ] T073 [US6] Implement reply projection, defined PostgreSQL normalized full-text search, dedicated pinned retrieval, and circle-row-locked pin/unpin/delete behavior in `backend/internal/chat/discovery_service.go`, `backend/internal/chat/repository.go`, and `backend/internal/chat/queries.go`
- [ ] T074 [P] [US6] Add search/pinned-list/pin contract tests for archived reads, DM exclusion, deleted content, five-item/order guarantees, sixth-pin conflict, student/non-member RBAC denial, and response safety in `backend/tests/contract/chat_discovery_contract_test.go`
- [ ] T075 [US6] Implement search and pin/unpin handlers plus centralized routes in `backend/internal/chat/discovery_handler.go`, `backend/internal/api/routes.go`, and `backend/internal/api/router.go`
- [ ] T076 [P] [US6] Add mobile controller/widget tests for reply previews, deleted-target redaction, Arabic/Latin search, pinned bar ordering, five-pin conflict, and archived search in `mobile/test/features/chat/application/chat_discovery_controller_test.dart` and `mobile/test/widget/chat/chat_discovery_widgets_test.dart`
- [ ] T077 [US6] Implement reply composer/preview, search results, and pinned bar flows in `mobile/lib/features/chat/application/chat_discovery_controller.dart` and `mobile/lib/features/chat/presentation/chat_discovery_widgets.dart`
- [ ] T078 [P] [US6] Add reproducible 10-circle/50-user/10,000-message-per-circle fixtures with 10% deletion, two membership periods, 100 warm-ups, 1,000 samples, and p95 ≤2-second history/search assertions in `backend/tests/performance/chat_fixture_test.go` and `backend/tests/performance/chat_history_search_performance_test.go`
- [ ] T079 [US6] Add reply/search/pin/unpin/deletion/archive acceptance coverage in `mobile/integration_test/chat_discovery_flow_test.dart`

---

## Phase 10: User Story 7 — Moderate Messages Safely (P2)

**Goal**: Senders and teachers can soft-delete within their authority while deleted content and media are revoked everywhere and teacher actions remain durably auditable.

**Independent test**: Exercise timely/late/concurrent self-delete and teacher delete, then prove absence from every projection, immediate versionless media revocation, safe crash reconciliation, and exactly-once effective teacher audit.

- [ ] T080 [P] [US7] Add service/repository tests for authoritative 10-minute self-delete, late conflict, current-teacher group deletion, DM own-delete, idempotent races, and effective teacher audit facts in `backend/internal/chat/moderation_service_test.go`
- [ ] T081 [US7] Implement soft-delete transactions, server-time enforcement, teacher authorization, and append-only moderation audit persistence in `backend/internal/chat/moderation_service.go`, `backend/internal/chat/repository.go`, and `backend/internal/chat/queries.go`
- [ ] T082 [P] [US7] Add real-MinIO integration tests for immediate versionless revocation, retained prior bytes, marker failure rejection, active-message restoration by removing only the latest internal marker after marker-before-commit crash, deleted-message marker repair, and unauthorized renewal denial in `backend/tests/integration/chat_media_revocation_test.go`
- [ ] T083 [US7] Implement fail-closed marker-before-database ordering and reconciliation that removes only the latest matching internal marker for active messages or creates a missing marker for deleted messages in `backend/internal/chat/media_store.go` and `backend/internal/chat/media_reconciler.go`
- [ ] T084 [P] [US7] Add group/DM delete contract tests for sender/teacher RBAC, non-teacher and non-member denial, deadline conflict, idempotency, audit response safety, and hidden content/key/URL fields in `backend/tests/contract/chat_moderation_contract_test.go`
- [ ] T085 [US7] Implement delete handlers, routes, `chat.message_deleted` outbox events, and deleted-content filtering across history/search/pins/replies/media in `backend/internal/chat/moderation_handler.go`, `backend/internal/chat/realtime_projector.go`, `backend/internal/api/routes.go`, and `backend/internal/api/router.go`
- [ ] T086 [P] [US7] Add mobile controller/widget tests for delete authority/deadline, conflict recovery, duplicate deletion events, media removal, pin removal, and reply-preview redaction in `mobile/test/features/chat/application/chat_moderation_controller_test.dart` and `mobile/test/widget/chat/chat_moderation_test.dart`
- [ ] T087 [US7] Implement delete actions and deletion-event projection cleanup in `mobile/lib/features/chat/application/chat_moderation_controller.dart` and `mobile/lib/features/chat/presentation/chat_widgets.dart`
- [ ] T088 [US7] Add sender/teacher deletion, race, audit, projection redaction, and media-revocation acceptance coverage in `mobile/integration_test/chat_moderation_flow_test.dart`

**P2 checkpoint**: T058–T088 complete all P2 stories. There are no P3 stories in F-004.

---

## Phase 11: Polish, Cross-Cutting Verification, and Review

**Purpose**: Prove complete security, reliability, performance, compatibility, and release readiness without expanding MVP scope.

- [ ] T089 [P] Add an architecture-boundary contract test proving no chat LiveKit/Firebase Messaging imports, no notification triggers, no global roles, and no second socket transport in `backend/tests/contract/chat_architecture_boundary_test.go` and `mobile/test/features/chat/chat_architecture_boundary_test.dart`
- [ ] T090 [P] Add integration coverage for redacted audit/metrics, upload/outbox/search failure attribution, backlog/parking, and reconnect recovery counters in `backend/tests/integration/chat_observability_test.go`
- [ ] T091 [P] Add the shared fixed fixture's 1,000-sample p95 ≤2-second commit-to-client delivery test plus intentional realtime-suppression REST recovery run in `backend/tests/performance/chat_delivery_performance_test.go`
- [ ] T092 Run focused Go race, security, contract, integration, migration, and fixed-fixture performance suites from `backend/`; resolve scoped defects in `backend/` and keep immutable commands in `specs/004-real-time-chat/quickstart.md`
- [ ] T093 Run `$docs-guard`, Spectral, canonical/feature parity, link checks, and secret scanning; resolve documentation-only defects in `docs/contracts/openapi.yaml`, `docs/contracts/ws_events.md`, `docs/engineering/architecture/ARCHITECTURE.md`, and `specs/004-real-time-chat/`
- [ ] T094 Run full Go unit, contract, integration, coverage, lint, format, and race gates from `backend/` and resolve scoped failures in `backend/`
- [ ] T095 Run Flutter unit/widget, per-file Linux integration, analyze, and format gates from `mobile/`; verify Arabic/RTL and LTR accessibility and resolve scoped failures in `mobile/`
- [ ] T096 Complete Tech Lead review plus Karim's mandatory manual deep review of Firebase authentication, RBAC, deletion, MinIO/upload, response safety, and rollback boundaries for `backend/internal/chat/`, `backend/internal/realtime/`, `backend/internal/api/`, `backend/migrations/000018_real_time_chat.up.sql`, and `backend/migrations/000018_real_time_chat.down.sql`; record approval in the GitHub PR review, not `specs/004-real-time-chat/checklists/requirements.md`

---

## Acceptance-Criteria Traceability

| Acceptance criterion | Implementation tasks | Test tasks |
|---|---|---|
| US1-AC1 membership-period history | T022, T033 | T021, T030, T032, T036 |
| US1-AC2 safe text stored once | T022, T024, T029, T031 | T021, T023, T030, T036 |
| US1-AC3 effective-once online event | T026, T028, T033 | T025, T027, T032, T036 |
| US1-AC4 no live-session dependency | T020, T022, T035 | T004, T021, T034, T036 |
| US2-AC1 offline pending with stable key | T038, T040 | T037, T039, T042 |
| US2-AC2 reconnect retry with same key | T038, T040 | T039, T041, T042 |
| US2-AC3 repeated requests return one message | T012, T022, T026 | T010, T021, T041, T042 |
| US2-AC4 reconnect/unknown/duplicate reconciliation | T040 | T039, T041, T042 |
| US3-AC1 record/waveform/preview/discard/send | T049, T051 | T048, T050, T052 |
| US3-AC2 300s/20MB voice and renewable playback | T044, T046, T049, T051 | T043, T045, T047, T048, T052 |
| US3-AC3 JPEG/PNG up to 5MB | T044, T046, T049, T051 | T043, T045, T047, T052 |
| US3-AC4 PDF up to 10MB | T044, T046, T049, T051 | T043, T045, T047, T052 |
| US3-AC5 invalid/oversized/unauthorized rejection | T044, T046 | T043, T045, T047, T052 |
| US8-AC1 identity/session denials | T054 | T053, T057 |
| US8-AC2 removal revokes group/DM/realtime | T054, T056 | T053, T055, T057 |
| US8-AC3 archived retained reads/search/play | T054, T056 | T053, T055, T057 |
| US8-AC4 archived mutations denied | T054, T056 | T053, T055, T057 |
| US8-AC5 session-shell independence | T056 | T004, T053, T055, T057 |
| US4-AC1 teacher–student both directions | T059, T061, T063 | T058, T060, T062, T064 |
| US4-AC2 supervisor–student both directions | T059, T061, T063 | T058, T060, T062, T064 |
| US4-AC3 disallowed role pairs | T059, T061 | T058, T060, T064 |
| US4-AC4 no qualifying shared circle | T059, T061 | T058, T060, T064 |
| US4-AC5 last qualifying relationship lost | T059, T061, T063 | T058, T060, T062, T064 |
| US4-AC6 complete pair history restored | T059, T063 | T058, T062, T064 |
| US5-AC1 pending state | T038, T040, T070 | T037, T039, T069, T071 |
| US5-AC2 sent state | T040, T070 | T039, T069, T071 |
| US5-AC3 delivered state | T040, T070 | T039, T069, T071 |
| US5-AC4 DM read update | T066, T068, T070 | T065, T067, T069, T071 |
| US5-AC5 group read headline/detail/idempotency | T066, T068, T070 | T065, T067, T069, T071 |
| US5-AC6 typing audience/expiry/non-persistence | T066, T068, T070 | T065, T067, T069, T071 |
| US6-AC1 safe same-conversation reply | T073, T075, T077 | T072, T074, T076, T079 |
| US6-AC2 retained-circle search boundaries | T073, T075, T077 | T072, T074, T076, T078, T079 |
| US6-AC3 authorized pin below limit | T073, T075, T077 | T072, T074, T076, T079 |
| US6-AC4 reject sixth pin then allow replacement | T073, T075, T077 | T072, T074, T076, T079 |
| US6-AC5 student/non-member pin denial | T073, T075 | T072, T074, T079 |
| US7-AC1 timely sender soft-delete | T081, T085, T087 | T080, T084, T086, T088 |
| US7-AC2 late self-delete conflict | T081, T085, T087 | T080, T084, T086, T088 |
| US7-AC3 teacher delete with durable audit | T081, T085 | T080, T084, T088 |
| US7-AC4 total projection/media revocation | T083, T085, T087 | T082, T084, T086, T088 |

## Success-Criteria Traceability

| Success criterion | Primary proof tasks |
|---|---|
| SC-001 complete requirement/acceptance mapping | T002–T004, T021–T096, this traceability table |
| SC-002 exactly one durable/visible message | T021, T025, T027, T032, T039, T041, T042 |
| SC-003 authorization denial matrix | T023, T045, T053, T058, T060, T067, T074, T084 |
| SC-004 validation and limit rejection | T008, T023, T043, T045, T072, T074, T080, T084 |
| SC-005 history/search performance | T078, T094 |
| SC-006 online delivery and recovery performance | T091, T094, T095 |
| SC-007 deletion exclusion and revocation | T080–T088 |
| SC-008 independent complete group workflow | T004, T036, T052, T057, T089 |
| SC-009 Arabic/LTR usability and safe rendering | T034, T050, T055, T062, T069, T076, T086, T095 |
| SC-010 no unresolved/global-role/duplicate-transport coupling | T004, T089, T093, T096 |

## Dependencies and Critical Chain

- Governance gate: T001 → T002 → T003.
- Shared foundation: T003 → T005 → T006/T007 → T010 → T011 → T012; T008 → T009; T013 → T014 → T015 → T016; T017 → T018; T019 → T020.
- Core-slice chain: governance → foundation → T021 → T022 → T024 → T029 → T031 → T033 → T035 → T036.
- Remaining P1 order: US1 → US2; US1 + media foundation → US3; US1 + authorization foundation → US8. US2, US3, and US8 may proceed in parallel after US1 if shared-file ownership is scheduled.
- P2 order: US8 → US4; US2 → US5; US3 + US4 + US5 → US6; US3 + US4 + US6 → US7.
- Release chain: all story checkpoints → T089–T091 → T092–T095 → T096.
- External dependency: F-001, F-002, F-003, and F-005 are declared complete, but T004 verifies the consumed behavior. ADR-010 invitation-role implementation remains F-002-owned and outside F-004; invite code/link generation and sharing remain intact.

## Parallel Execution Examples

- After T003: run T005, T008, T013, T017, and T019 concurrently because they touch distinct migration/domain/config/observability/realtime test files.
- After Phase 2: start T021/T023/T025 and T030 concurrently; implementations follow their respective failing tests.
- After US1: US2 mobile persistence, US3 backend media validation, and US8 lifecycle tests can proceed concurrently when shared chat files have one scheduled owner.
- In each P2 story: backend service tests, contract tests, and mobile tests marked `[P]` can start together; shared repository/router/controller implementation remains sequential.
- Before release: T089–T091 can run concurrently; T092–T096 remain ordered verification/review gates.

## Implementation Strategy

1. Complete Phase 1 and stop if ADR-021 is not accepted or contracts diverge.
2. Complete Phase 2 once; do not duplicate auth, membership, realtime, rate-limit, media, or observability infrastructure inside stories.
3. Deliver and review the core US1/T036 checkpoint without treating it as the full product MVP.
4. Complete every remaining P1 story, then every P2 story; together they are the approved F-004 MVP. No P3 work exists.
5. Use `/speckit.analyze` before implementation, then `/speckit.implement` with test-first execution and task checkboxes updated only after fresh evidence.
