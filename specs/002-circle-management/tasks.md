# Tasks: Circle Management

**Input**: Design documents from `/specs/002-circle-management/`  
**Prerequisites**: `spec.md`, `checklists/requirements.md`, `plan.md`, `research.md`, `data-model.md`, `contracts/circle-management.openapi.yaml`, `quickstart.md`  
**Branch**: `002-circle-management`

**Tests**: Go unit, contract, integration, migration, security/response-safety, rate-limit tests, and Flutter widget/integration tests are required.  
**MVP scope**: **US1–US5**. F-002 is approved P0; all five stories are required for the MVP feature. No P3 stories are defined.  
**Execution rule**: `[P]` means the task has no incomplete dependency and no shared-file conflict with another task in its phase.

## Phase 0: Canonical Alignment Gate

**Purpose**: Resolve the checklist findings before implementation.

- [X] T001 [US0] Amend F-002 retirement wording and prohibit hard deletion in `docs/management/product/FEATURES.md`.
- [X] T002 [US0] Amend T-05 public/private circle behavior and invite-link wording in `docs/management/product/JOURNEY.md`.
- [X] T003 [US0] Record the circle-retirement decision and required amendment in `docs/engineering/architecture/adr/ADR-011-circle-retirement.md` and `docs/management/product/MVP_DECISION_REGISTER.md`.
- [X] T004 [US0] Align circle gender values `male|female|mixed|unspecified` with default `unspecified` across `docs/management/product/FEATURES.md`, `docs/engineering/architecture/ARCHITECTURE.md`, `docs/contracts/openapi.yaml`, and `specs/002-circle-management/contracts/circle-management.openapi.yaml`, without coupling them to teacher gender.
- [X] T005 [US0] Merge public discovery/direct join, redacted public summaries, invite refresh, 8-character invite-code validation, and archive-only deletion semantics into `docs/contracts/openapi.yaml`.
- [X] T006 [US0] Create and accept the audit persistence decision in `docs/engineering/architecture/adr/ADR-012-audit-logging-persistence.md`, then run the documentation checklist and OpenAPI lint for all Phase 0 files.

**Checkpoint**: T001–T006 must pass before implementation tasks begin.

---

## Phase 1: Shared Foundation and Database

**Purpose**: Add the smallest shared circle foundation required by all MVP stories.

- [X] T007 [P] [US0] Add the next paired PostgreSQL migration for F-002 circle fields, defaults, constraints, and indexes in `backend/migrations/000015_circle_management.up.sql` and `backend/migrations/000015_circle_management.down.sql`.
- [X] T008 [P] [US0] Add migration integration coverage for fresh apply, upgrade from `000014`, rollback, rerun safety, preserved rows, and no hard-delete objects in `backend/tests/integration/circle_management_migration_test.go`.
- [X] T009 [P] [US0] Define circle domain models and API DTOs in `backend/internal/rbac/models.go` (existing circle ownership package).
- [X] T010 [P] [US0] Define circle error codes and validation mappings using centralized constants in `backend/internal/platform/httpconst/error_codes.go` and `error_messages.go`.
- [X] T011 [US0] Define package-level SQL statements for circles, memberships, discovery, joins, invite refresh, roles, members, and archive in the existing circle ownership package `backend/internal/rbac/queries.go`.
- [X] T012 [US0] Implement circle repository methods using the SQL statements in the existing circle ownership package `backend/internal/rbac/repository.go`.
- [X] T013 [US0] Implement transactional circle policies, row locking, membership limits, capacity checks, final-teacher protection, and archive guards in the existing circle ownership package `backend/internal/rbac/service.go`.
- [X] T014 [P] [US0] Add circle audit-event names and payload builders in `backend/internal/platform/logging/audit_logger.go`.
- [X] T015 [P] [US0] Add shared circle response-safety tests proving private/member data and invite codes are omitted from public discovery in `backend/tests/contract/circle_response_safety_contract_test.go`.

---

## Phase 2: User Story 1 — Create a Circle (Priority: P1) 🎯 MVP

**Goal**: Create a circle with settings, initial memberships, and an 8-character invite link.

**Independent test**: An authenticated user creates valid and invalid circles; verify atomic persistence, role fallback/assignment, validation, code format, and response safety.

### Tests for User Story 1

- [X] T016 [P] [US1] Add contract tests for valid circle creation, response shape, 100/500/1000-character limits, capacity 2..200/default 50, privacy, language, all four circle-gender values, omitted-gender default `unspecified`, invalid-value rejection, and invite link in `backend/tests/contract/circle_create_contract_test.go` and `backend/tests/contract/circle_create_extended_contract_test.go`.
- [X] T017 [P] [US1] Add contract tests for teacher assignment, optional backup supervisor, creator-teacher fallback, duplicate/unknown/overlapping assignees, and standard validation errors in `backend/tests/contract/circle_create_contract_test.go`.
- [X] T018 [P] [US1] Add unit tests for invite-code generation, exact 8-character format, successful uniqueness retry, and collision exhaustion in `backend/internal/rbac/service_test.go` and `backend/internal/rbac/invite_test.go`.
- [X] T019 [P] [US1] Add integration tests for atomic circle creation and initial membership persistence in `backend/tests/integration/circle_creation_test.go`.
- [X] T020 [P] [US1] Add security tests for unauthenticated creation, invalid Firebase/session credentials, and rate-limited repeated creation in `backend/tests/contract/circle_create_security_contract_test.go`.
- [X] T021 [P] [US1] Add Flutter widget tests for create-circle validation, Arabic RTL rendering, localized gender/settings fields, user selection, stale-success clearing, and server field-error mapping in `mobile/test/widget/circles/create_circle_screen_test.dart`.

### Implementation for User Story 1

- [X] T022 [US1] Implement create-circle repository transaction and initial membership assignment in `backend/internal/rbac/repository.go`.
- [X] T023 [US1] Implement create-circle service validation, OQ-036 role fallback, capacity defaults, invite generation, and audit event in `backend/internal/rbac/service.go`.
- [X] T024 [US1] Implement create-circle and authenticated registered-user-search handlers with standard error responses in `backend/internal/rbac/handler.go`.
- [X] T025 [US1] Add `POST /api/v1/circles` and `GET /api/v1/users/search` route constants and wire auth/session/rate-limit middleware, validation, handlers, and services in `backend/cmd/api/routes.go` and `backend/cmd/api/router.go`.
- [X] T026 [US1] Implement circle API client and request/response models in `mobile/lib/features/circles/data/circle_api_client.dart`.
- [X] T027 [US1] Implement Riverpod create-circle controller/provider in `mobile/lib/features/circles/application/create_circle_controller.dart`.
- [X] T028 [US1] Implement Arabic-first RTL create-circle screen and accessible validation states in `mobile/lib/features/circles/presentation/create_circle_screen.dart`.

**Checkpoint**: US1 creates a usable circle and is independently testable.

---

## Phase 3: User Story 2 — Discover and Join a Circle (Priority: P1) 🎯 MVP

**Goal**: Support public discovery/join and invite-link joining with capacity and membership safeguards.

**Independent test**: Discover/join a public circle and join a public/private circle by invite; verify all invalid and limit cases.

### Tests for User Story 2

- [X] T029 [P] [US2] Add contract tests for public discovery pagination/filtering and redacted public summaries in `backend/tests/contract/circle_discovery_contract_test.go`.
- [X] T030 [P] [US2] Add contract tests for public direct join and invite-code join in `backend/tests/contract/circle_join_contract_test.go`.
- [X] T031 [P] [US2] Add integration tests for duplicate membership, full circle, archived circle, sixth-circle limit, public/private behavior, and atomic join under concurrent requests in `backend/tests/integration/circle_join_test.go`.
- [X] T032 [P] [US2] Add security/rate-limit tests for unauthenticated discovery/join, invalid credentials, repeated join attempts, and response-safety of public results in `backend/tests/contract/circle_join_security_contract_test.go`.
- [X] T033 [P] [US2] Add Flutter widget tests for discovery cards, public/private visibility, join confirmation, invite-link errors, full/archive states, and RTL layouts in `mobile/test/widget/circles/discover_join_screen_test.dart`.
- [X] T034 [US2] Add Flutter integration test for public discovery, public join, private invite join, duplicate join, and sixth-circle rejection in `mobile/integration_test/circle_join_flow_test.dart`.

### Implementation for User Story 2

- [X] T035 [US2] Implement public discovery and authorized public-summary queries in `backend/internal/rbac/queries.go` and `backend/internal/rbac/repository.go`.
- [X] T036 [US2] Implement public direct join and invite-code join transactions, capacity checks, five-circle limit, archived checks, and audit events in `backend/internal/rbac/service.go`.
- [X] T037 [US2] Add discovery/join handlers and standard `400/401/404/409` responses in `backend/internal/rbac/handler.go`.
- [X] T038 [US2] Add discovery/direct-join/invite-join route constants and router wiring in `backend/cmd/api/routes.go` and `backend/cmd/api/router.go`.
- [X] T039 [US2] Implement Riverpod circle-list/discovery/join providers in `mobile/lib/features/circles/application/circle_discovery_controller.dart`.
- [X] T040 [US2] Implement public discovery, join confirmation, invite-link entry, and error/read-only states in `mobile/lib/features/circles/presentation/circle_discovery_screen.dart` and `mobile/lib/features/circles/presentation/circle_join_screen.dart`.

**Checkpoint**: US2 is independently testable with public and invite-based joining.

---

## Phase 4: User Story 3 — View Circle and Members (Priority: P1) 🎯 MVP

**Goal**: Let active members read circle details and member roles while protecting private data.

**Independent test**: Read active, private, public, and archived circles as member/non-member/unauthenticated callers.

### Tests for User Story 3

- [X] T041 [P] [US3] Add contract tests for circle details, member list, archived read-only responses, and standard `401/403/404` responses in `backend/tests/contract/circle_read_members_contract_test.go`.
- [ ] T042 [P] [US3] Add integration tests for member visibility, non-member denial, public-summary redaction, archived history retention, and no mutation after archive in `backend/tests/integration/circle_read_archive_test.go`.
- [X] T043 [P] [US3] Add response-safety tests for private member data, invite code exposure, and archived-history visibility in `backend/tests/contract/circle_read_response_safety_test.go`.
- [ ] T044 [P] [US3] Add Flutter widget tests for member list roles, loading/error states, archived read-only controls, and RTL accessibility in `mobile/test/widget/circles/circle_members_screen_test.dart`.

### Implementation for User Story 3

- [ ] T045 [US3] Implement authorized circle-detail and member-list repository queries in `backend/internal/rbac/queries.go` and `backend/internal/rbac/repository.go`.
- [X] T046 [US3] Implement read authorization and archived read-only service behavior in `backend/internal/rbac/service.go`.
- [X] T047 [US3] Implement circle-detail/member handlers and route wiring in `backend/internal/rbac/handler.go` and `backend/cmd/api/router.go`.
- [ ] T048 [US3] Implement Riverpod circle-detail/member providers in `mobile/lib/features/circles/application/circle_detail_controller.dart`.
- [ ] T049 [US3] Implement circle detail/member screens with role labels, safe public fields, archived history, and RTL support in `mobile/lib/features/circles/presentation/circle_detail_screen.dart` and `mobile/lib/features/circles/presentation/circle_members_screen.dart`.

**Checkpoint**: US3 provides the authenticated member read experience and closes the read/data-exposure boundary.

---

## Phase 5: User Story 4 — Manage Circle Roles and Invite Access (Priority: P2) 🎯 MVP

**Goal**: Provide safe manager role changes, member removal, invite sharing, and invite rotation.

**Independent test**: Exercise teacher/supervisor/student/non-member/self/final-teacher actions and concurrent invite refresh.

### Tests for User Story 4

- [ ] T050 [P] [US4] Add contract tests for teacher/supervisor role changes, student/non-member denial, self-change denial, cross-circle denial, and final-teacher protection in `backend/tests/contract/circle_role_management_contract_test.go`.
- [ ] T051 [P] [US4] Add contract tests for teacher-only member removal, supervisor/student/non-member denial, self/final-teacher protection, retained history, invite refresh, old-code invalidation, new-code response, and archived-circle denial in `backend/tests/contract/circle_invite_role_contract_test.go`.
- [ ] T052 [P] [US4] Add integration race tests for concurrent role changes, concurrent invite refresh, final-teacher protection, and duplicate audit events in `backend/tests/integration/circle_role_invite_race_test.go`.
- [ ] T053 [P] [US4] Add RBAC, response-safety, and rate-limit tests for role/invite mutation endpoints in `backend/tests/contract/circle_role_invite_security_contract_test.go`.
- [ ] T054 [P] [US4] Add Flutter widget tests for manager-only controls, role confirmation, invite display/share/refresh, and denial states in `mobile/test/widget/circles/circle_management_controls_test.dart`.
- [ ] T055 [US4] Add Flutter integration test for role management, invite sharing/refresh, and old-link rejection in `mobile/integration_test/circle_role_invite_flow_test.dart`.

### Implementation for User Story 4

- [ ] T056 [US4] Implement transactional role-change/member-removal service and audit events in `backend/internal/rbac/service.go`.
- [ ] T057 [US4] Implement transactional invite refresh and invite-link response mapping in `backend/internal/rbac/service.go` and `backend/internal/rbac/repository.go`.
- [ ] T058 [US4] Implement role/member/invite handlers with RBAC and standard error responses in `backend/internal/rbac/handler.go`.
- [ ] T059 [US4] Add role/member/invite route constants and middleware wiring in `backend/cmd/api/routes.go` and `backend/cmd/api/router.go`.
- [ ] T060 [US4] Implement Riverpod role/member/invite controllers in `mobile/lib/features/circles/application/circle_management_controller.dart`.
- [ ] T061 [US4] Implement role-management controls, invite sharing/refresh, and accessible confirmation/error UI in `mobile/lib/features/circles/presentation/circle_management_screen.dart`.

**Checkpoint**: US4 preserves the per-circle role invariants and invite integrity under concurrent mutation.

---

## Phase 6: User Story 5 — Retire a Circle (Priority: P2) 🎯 MVP

**Goal**: Retire circles through archive/soft state only; preserve all history and prohibit hard deletion.

**Independent test**: Teacher archives a circle, reads retained history, verifies new activity is blocked, and verifies no hard-delete route/query exists.

### Tests for User Story 5

- [ ] T062 [P] [US5] Add contract tests for teacher-only archive, non-teacher denial, idempotent archive, archived reads, and archive-only `DELETE /circles/{circleId}` in `backend/tests/contract/circle_retirement_contract_test.go`.
- [ ] T063 [P] [US5] Add integration tests for history retention, blocked joins/settings/member changes after archive, and archive audit events in `backend/tests/integration/circle_retirement_test.go`.
- [ ] T064 [P] [US5] Add hard-delete absence/response-safety tests proving no circle hard-delete route, SQL statement, repository method, or cascade is introduced in `backend/tests/contract/circle_hard_delete_safety_test.go`.
- [ ] T065 [P] [US5] Add Flutter widget/integration tests for archive confirmation, archived read-only state, and hidden mutation controls in `mobile/test/widget/circles/circle_retirement_test.dart` and `mobile/integration_test/circle_retirement_flow_test.dart`.

### Implementation for User Story 5

- [ ] T066 [US5] Implement archive/retirement transaction, idempotency, audit event, and mutation guards in `backend/internal/rbac/service.go` and `backend/internal/rbac/repository.go`.
- [ ] T067 [US5] Implement archive handler and archive-only route semantics in `backend/internal/rbac/handler.go` and `backend/cmd/api/router.go`.
- [ ] T068 [US5] Implement Riverpod retirement state and archive confirmation/read-only controls in `mobile/lib/features/circles/application/circle_retirement_controller.dart` and `mobile/lib/features/circles/presentation/circle_retirement_screen.dart`.

**Checkpoint**: US5 retires circles safely with no hard-delete path and preserved reporting history.

---

## Phase 7: Cross-Cutting Hardening and Verification

**Purpose**: Verify the full MVP feature and enforce project quality gates.

- [ ] T069 [P] [US0] Add focused REST rate-limit and timeout integration coverage for circle reads/discovery and mutation endpoints in `backend/tests/integration/circle_rate_limit_policy_test.go`.
- [ ] T070 [P] [US0] Add audit-log coverage for create, join, role change, member removal, invite refresh, and archive in `backend/tests/integration/circle_audit_test.go`.
- [ ] T071 [P] [US0] Add contract coverage for backward-compatible existing circle operations and all standard error envelopes in `backend/tests/contract/circle_backward_compatibility_test.go`.
- [ ] T072 [P] [US0] Add OpenAPI reference/operation/security validation for `docs/contracts/openapi.yaml` and `specs/002-circle-management/contracts/circle-management.openapi.yaml` in `backend/tests/contract/circle_openapi_contract_test.go`.
- [ ] T073 [P] [US0] Add observability assertions for request IDs, structured circle mutation logs, latency, and rejection metrics in `backend/tests/integration/circle_observability_test.go`.
- [ ] T074 [US0] Run `$clean-code-guard` on `backend/internal/rbac/` and `mobile/lib/features/circles/`; record findings in `specs/002-circle-management/validation-report.md`.
- [ ] T075 [US0] Run `$test-guard` on changed Go/Dart tests; record findings in `specs/002-circle-management/validation-report.md`.
- [ ] T076 [US0] Run `$docs-guard` on changed product, ADR, architecture, OpenAPI, and feature-contract files; record findings in `specs/002-circle-management/validation-report.md`.
- [ ] T077 [US0] Run focused Go/Flutter suites and full applicable gates (`go test -short ./...`, `flutter test test`, formatters, analyzers, lint, Spectral, and gitleaks); record current output in `specs/002-circle-management/validation-report.md`.
- [ ] T078 [US0] Send the completed MVP batch to Tech Lead review and preserve Karim's manual RBAC/data-retention review evidence in `specs/002-circle-management/validation-report.md`.

---

## Dependencies and Execution Order

### Phase dependencies

- Phase 0 (T001–T006) blocks all implementation.
- Phase 1 (T007–T015) depends on Phase 0 and blocks all user stories.
- US1 (T016–T028) is the first P1 slice and provides circle creation/client foundations.
- US2 (T029–T040) depends on shared foundation and US1 API/client models.
- US3 (T041–T049) depends on shared foundation and authenticated circle data from US1/US2.
- US4 (T050–T061) depends on US1 role assignments, US2 memberships, and US3 member reads.
- US5 (T062–T068) depends on US3 reads and shared archive guards; it must complete before final MVP verification.
- Phase 7 (T069–T078) follows all MVP stories.

### Critical dependency chain

`T001–T006 → T007–T015 → T016–T028 → T029–T040 → T041–T049 → T050–T061 → T062–T068 → T069–T078`

### Parallel execution examples

- After Phase 0: T007, T008, T009, T010, T014, and T015 may run in parallel because they have disjoint files.
- Within US1 tests: T016–T021 may run in parallel; implementation starts after the relevant red tests exist.
- Within US2 tests: T029–T034 may run in parallel; backend implementation remains sequential where it shares `service.go` and `handler.go`.
- Within US4 tests: T050–T055 may run in parallel; role and invite implementation remains sequential in shared circle files.
- Within Phase 7: T069–T073 may run in parallel; guard execution and final verification remain sequential.

### MVP delivery strategy

1. Complete Phase 0 alignment and Phase 1 foundation.
2. Deliver P1 stories US1, US2, and US3 in order with their independent tests and checkpoints.
3. Deliver P2 governance US4 and P2 retirement US5; both remain in MVP scope because F-002 is P0 approved.
4. Complete Phase 7 hardening, Tech Lead review, and Karim's manual RBAC/data-retention review before implementation completion.
