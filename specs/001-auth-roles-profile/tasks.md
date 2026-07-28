# Tasks: Authentication, Roles, and User Profile

**Input**: Design documents from `/specs/001-auth-roles-profile/`  
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/auth-roles-profile.openapi.yaml, quickstart.md

**Tests**: Go unit/integration/contract tests and Flutter widget/integration tests are required.  
**Organization**: Tasks are grouped by user story priority (P1 → P2 → P3). MVP scope is **US1**.

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Initialize feature scaffolding and contract baseline.

- [X] T001 Create backend feature scaffolding in `backend/cmd/api/router.go` and `backend/internal/{auth,profile,rbac,middleware,platform}/`
- [X] T002 Create mobile feature scaffolding in `mobile/lib/features/{auth,profile,admin}/` and `mobile/test/widget/{auth,profile}/`
- [X] T003 [P] Add backend dependencies for Firebase auth verification and profile/rbac modules in `backend/go.mod`
- [X] T004 [P] Add Flutter dependencies for Firebase auth and feature flows in `mobile/pubspec.yaml`
- [X] T005 [P] Merge feature OpenAPI paths/schemas into `docs/contracts/openapi.yaml`
- [X] T006 [P] Add feature-focused test commands in `backend/Makefile` and `mobile/Makefile`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Build constitution-aligned auth/session/rbac/rate-limit foundations.

- [X] T007 Create schema migration for users/profiles/user_sessions/circle_members in `backend/migrations/000010_auth_roles_profile.up.sql`
- [X] T008 Create rollback migration for users/profiles/user_sessions/circle_members in `backend/migrations/000010_auth_roles_profile.down.sql`
- [X] T009 [P] Define auth/session/rate-limit/timeout configuration in `backend/internal/platform/config/auth_config.go`
- [X] T010 Implement Firebase token verification service in `backend/internal/auth/firebase_verifier.go`
- [X] T011 Implement backend session inactivity enforcement service in `backend/internal/auth/session_service.go`
- [X] T012 Implement backend logout session invalidation repository in `backend/internal/auth/session_repository.go`
- [X] T013 [P] Implement bearer authentication middleware in `backend/internal/middleware/auth_middleware.go`
  - [X] **Correction**: Resolve PostgreSQL UUID from Firebase UID and set it in `AuthPrincipal` to fix the `session.UserID` (UUID) vs `decoded.UID` (Firebase UID) mismatch.
- [X] T014 [P] Implement per-circle role authorization middleware using `circle_members` in `backend/internal/middleware/role_middleware.go`
  - [X] **Correction**: Pass local database User UUID (from `principal.UserID`) to `RoleForUserInCircle` instead of the Firebase UID string. Also added `RoleForUserInCircle` to `SessionRepository` and `getCircleMemberRoleQuery` to `session_queries.go`.
- [X] T015 [P] Implement audit logger for auth/profile/role-change events in `backend/internal/platform/logging/audit_logger.go`
- [X] T016 [P] Implement REST rate limits per IP and per user in `backend/internal/middleware/rate_limit_middleware.go`
  - [X] **Correction**: Implement periodic eviction/expiration of rate limiter counters to fix the unbounded map memory leak.
- [X] T017 [P] Implement WebSocket limits (max 3 connections/user, max 30 messages/min/user/circle) in `backend/internal/middleware/ws_rate_limit_middleware.go`
  - [X] **Correction**: Exposed `OpenConnection`/`CloseConnection` public methods for WebSocket handler lifecycle. `LimitUpgrade` now only checks capacity; handler is responsible for tracking the live connection.
- [X] T018 Wire authentication, authorization, validation, timeout, and rate-limit middleware in `backend/cmd/api/router.go`
- [ ] T018a Bootstrap database connection pool, load configuration, wire dependencies, and start HTTP server in `backend/cmd/api/main.go`
  - ⚠️ **Blocker**: Code implemented but cannot compile — system Go version is 1.20.2; project requires Go 1.22+. Install Go 1.22 from https://go.dev/dl/ to unblock.

**Checkpoint**: Foundational platform complete; user stories can begin.

---

## Phase 3: User Story 1 - Secure Account Access (Priority: P1) 🎯 MVP

**Goal**: Deliver secure registration/login/logout with Firebase identity and backend session enforcement.

**Independent Test**: Register/login with valid credentials, access protected route, logout, and verify protected route rejection until re-authentication.

### Tests for User Story 1

- [ ] T019 [P] [US1] Add contract tests for `POST /auth/register` and duplicate-email `409` response in `backend/tests/contract/auth_register_contract_test.go`
- [ ] T020 [P] [US1] Add contract tests for `POST /auth/login` and `401` invalid-credential response in `backend/tests/contract/auth_login_contract_test.go`
- [ ] T021 [P] [US1] Add contract tests for `POST /auth/logout` current-session invalidation and session-expired `401` response in `backend/tests/contract/auth_logout_contract_test.go`
- [ ] T022 [P] [US1] Add unit tests for Firebase token verification success/failure paths in `backend/internal/auth/firebase_verifier_test.go`
- [ ] T023 [P] [US1] Add unit tests for session inactivity expiration and logout invalidation in `backend/internal/auth/session_service_test.go`
- [ ] T024 [US1] Add integration test for register→login→protected-route→logout flow in `backend/tests/integration/auth_flow_test.go`
- [ ] T025 [P] [US1] Add response-safety tests asserting no plaintext password in responses in `backend/tests/contract/auth_response_safety_contract_test.go`
- [ ] T026 [P] [US1] Add Flutter widget tests for register/login validation in `mobile/test/widget/auth/auth_forms_test.dart`
- [ ] T027 [US1] Add Flutter integration test for register/login/logout journey in `mobile/integration_test/auth_journey_test.dart`

### Implementation for User Story 1

- [ ] T028 [P] [US1] Implement auth request/response models in `backend/internal/auth/models.go`
- [ ] T029 [US1] Implement auth application service for register/login/logout and session recording in `backend/internal/auth/service.go`
- [ ] T030 [US1] Implement auth HTTP handlers in `backend/internal/auth/handler.go`
- [ ] T031 [US1] Wire auth routes and validators in `backend/cmd/api/router.go`
- [ ] T032 [P] [US1] Implement Flutter auth API client in `mobile/lib/features/auth/data/auth_api_client.dart`
- [ ] T033 [US1] Implement Flutter auth controller and secure token persistence in `mobile/lib/features/auth/application/auth_controller.dart`
- [ ] T034 [US1] Implement Flutter register/login/logout screens in `mobile/lib/features/auth/presentation/auth_screens.dart`

**Checkpoint**: US1 is independently functional and is the MVP scope.

---

## Phase 4: User Story 2 - Complete Basic Profile (Priority: P2)

**Goal**: Allow authenticated users to create/read/update profile with first-completion validation.

**Independent Test**: Login, read profile via `/auth/me`, update profile, and verify required fields (`full_name`, `country`) are enforced for first completion.

### Tests for User Story 2

- [ ] T035 [P] [US2] Add contract tests for `GET /auth/me` and `PUT /auth/me` in `backend/tests/contract/profile_me_contract_test.go`
- [ ] T036 [P] [US2] Add contract tests for validation error envelope and error codes in `backend/tests/contract/profile_validation_contract_test.go`
- [ ] T037 [P] [US2] Add unit tests for first-completion required fields in `backend/internal/profile/service_test.go`
- [ ] T038 [US2] Add integration test for profile create/read/update flow in `backend/tests/integration/profile_flow_test.go`
- [ ] T039 [P] [US2] Add Flutter widget tests for required profile fields and server error mapping in `mobile/test/widget/profile/profile_form_test.dart`
- [ ] T040 [US2] Add Flutter integration test for login→profile view/edit flow in `mobile/integration_test/profile_flow_test.dart`

### Implementation for User Story 2

- [ ] T041 [P] [US2] Implement profile repository and persistence mapping in `backend/internal/profile/repository.go`
- [ ] T042 [US2] Implement profile service with completion validation and error-code mapping in `backend/internal/profile/service.go`
- [ ] T043 [US2] Implement profile handlers for `GET /auth/me` and `PUT /auth/me` in `backend/internal/profile/handler.go`
- [ ] T044 [US2] Wire authenticated profile routes in `backend/cmd/api/router.go`
- [ ] T045 [P] [US2] Implement Flutter profile API client in `mobile/lib/features/profile/data/profile_api_client.dart`
- [ ] T046 [US2] Implement Flutter profile controller in `mobile/lib/features/profile/application/profile_controller.dart`
- [ ] T047 [US2] Implement Flutter profile screen for view/edit in `mobile/lib/features/profile/presentation/profile_screen.dart`

**Checkpoint**: US2 is independently functional and testable on top of auth.

---

## Phase 5: User Story 3 - Enforce Circle Role-Based Access (Priority: P3)

**Goal**: Enforce per-circle RBAC using `circle_members` for restricted actions.

**Independent Test**: Call circle role-assignment endpoint with supervisor/teacher/student/non-member users and verify only authorized users succeed.

### Tests for User Story 3

- [ ] T048 [P] [US3] Add contract tests for `POST /circles/{circleId}/members/{userId}/role` in `backend/tests/contract/circle_assign_role_contract_test.go`
- [ ] T049 [P] [US3] Add unit tests for per-circle role guard policy in `backend/internal/rbac/service_test.go`
- [ ] T050 [US3] Add integration test for supervisor/teacher/student/non-member access outcomes in `backend/tests/integration/circle_role_access_test.go`
- [ ] T051 [P] [US3] Add Flutter integration test for role-restricted UX behavior in `mobile/integration_test/role_access_test.dart`

### Implementation for User Story 3

- [ ] T052 [P] [US3] Implement circle role-assignment service using `circle_members` in `backend/internal/rbac/service.go`
- [ ] T053 [US3] Implement circle role-assignment handler in `backend/internal/rbac/handler.go`
- [ ] T054 [US3] Enforce supervisor/teacher route policy in `backend/cmd/api/router.go`
- [ ] T055 [US3] Implement Flutter role-gated navigation guard in `mobile/lib/features/auth/presentation/role_guard.dart`
- [ ] T056 [US3] Implement Flutter circle-role API client in `mobile/lib/features/admin/data/circle_role_api_client.dart`

**Checkpoint**: US3 is independently functional with per-circle RBAC.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final hardening, observability, and measurable acceptance gates.

- [ ] T057 [P] Align final API docs, error envelopes, and explicit error codes in `docs/contracts/openapi.yaml` and `docs/contracts/README.md`
- [ ] T058 [P] Add auth latency/rejection/session-expiry metrics instrumentation in `backend/internal/platform/metrics/auth_metrics.go`
- [ ] T059 Add explicit login performance benchmark/load-test gate for SC-001 in `backend/tests/performance/auth_login_performance_test.go`
- [ ] T060 Add explicit rate-limit integration tests (IP/user/WS message caps) in `backend/tests/integration/rate_limit_policy_test.go`
- [ ] T061 Harden idempotency/retry/timeout handling in `backend/internal/platform/http/server.go`
- [ ] T062 Execute quickstart validation and record evidence in `specs/001-auth-roles-profile/validation-report.md`
- [ ] T063 Add persistence-safety test asserting no plaintext password storage paths in `backend/tests/integration/password_storage_safety_test.go`
- [ ] T064 Add coverage gate task for `backend/internal/` >=80% and record output in `specs/001-auth-roles-profile/validation-report.md`

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1 (Setup) → required before Phase 2
- Phase 2 (Foundational) → blocks all user stories
- Phase 3 (US1/P1) → MVP delivery target
- Phase 4 (US2/P2) → depends on Phase 2 and reuses US1 auth behavior
- Phase 5 (US3/P3) → depends on Phase 2 and reuses auth + per-circle role middleware
- Phase 6 (Polish) → after selected stories are complete

### User Story Dependencies

- **US1 (P1)**: starts after Phase 2; no dependency on US2/US3
- **US2 (P2)**: requires authenticated session behavior from US1
- **US3 (P3)**: requires authentication/token validation from US1 and per-circle middleware from Phase 2

### Critical Dependency Chain

`T007 → T010 → T013 → T018 → T029 → T030 → T042 → T043 → T054 → T059`

---

## Parallel Execution Examples

### US1 Parallel Block

Run in parallel: `T019`, `T020`, `T021`, `T022`, `T023`, `T025`, `T026`, `T028`, `T032`

### US2 Parallel Block

Run in parallel: `T035`, `T036`, `T037`, `T039`, `T041`, `T045`

### US3 Parallel Block

Run in parallel: `T048`, `T049`, `T051`, `T052`

---

## Implementation Strategy

### MVP First (US1 only)

1. Complete Phase 1 and Phase 2.
2. Complete US1 tests and implementation tasks (T019–T034).
3. Validate US1 independent test and ship MVP.

### Incremental Delivery

1. Deliver US1 (MVP), then US2, then US3 in priority order.
2. After each story, run its contract/integration/mobile tests before proceeding.
3. Finish with Phase 6 hardening plus performance and rate-limit validation gates.
