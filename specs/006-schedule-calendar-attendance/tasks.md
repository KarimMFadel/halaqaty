# Tasks: F-006 Schedule, Calendar & Attendance

**Branch:** `006-schedule-calendar-attendance`

**Input:** [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [REST contract](contracts/schedule-calendar.openapi.yaml), [profile timezone contract](contracts/profile-timezone.md), ADR-025 and accepted ADR-026.

**Scope:** MVP partial pilot includes US1–US4 and every F-006 security/accessibility requirement. Full SC-009 push completion remains dependent on F-008 approval and real delivery evidence.

**Labels:** `[US0]` means governance, shared foundation or final gate; `[US1]`–`[US4]` match the spec stories. No task is marked `[P]` while the migration and shared route dependencies remain incomplete; reassess disjoint work after those gates pass.
**Evidence rule:** Leave a box open until its named deliverable exists and current verification supports it. Tests are written to fail first where behavior is new. T001–T004 are complete after Karim's approval, architecture/contract synchronization, parity check, API lint and manual docs-guard pass; implementation tasks remain open.

## Phase 1 — Governance and contract setup (US0)

- [X] T001 [US0] Obtain Karim's explicit acceptance or revision of proposed persistence design and record the actual decision in `docs/engineering/architecture/adr/ADR-026-schedule-occurrence-and-attendance-persistence.md`; approved by Karim on 2026-09-27, including the 31-day bound.
- [X] T002 [US0] Reconcile the approved profile timezone, local end clock, recurrence, planned-session, cancellation, durable later-participant roster and index model in `docs/engineering/architecture/ARCHITECTURE.md`; F-005's three-state lifecycle and ADR-025 boundary remain intact.
- [X] T003 [US0] Reconcile every approved F-006 path, method, schema, success/error response, authentication and role rule from `specs/006-schedule-calendar-attendance/contracts/schedule-calendar.openapi.yaml` and additive `/auth/me` timezone shape from `specs/006-schedule-calendar-attendance/contracts/profile-timezone.md` into canonical `docs/contracts/openapi.yaml`, preserving existing F-005 v1 operations and start response.
- [X] T004 [US0] Compare canonical and feature-local REST shapes, resolve design mismatches, run `make api-lint`, and apply the `.github/skills/docs-guard/SKILL.md` checklist manually to `docs/contracts/openapi.yaml` and `specs/006-schedule-calendar-attendance/contracts/schedule-calendar.openapi.yaml`; completed with exact path/schema parity after canonical ref mapping and lint on 2026-09-27.

**Gate:** T001–T004 must be complete before an executable F-006 migration or endpoint is written. If approval changes the plan, revise `specs/006-schedule-calendar-attendance/plan.md` and rerun analysis first.

## Phase 2 — Shared persistence and security foundation (US0)

- [X] T005 [US0] Add approved `profiles.timezone` with `UTC` backfill plus additive tables, foreign keys, checks, unique occurrence/attendance identities and month/role indexes in `backend/migrations/000020_schedule_calendar_attendance.up.sql`; recheck the migration tip before naming the pair.
- [X] T006 [US0] Add the matching disposable-schema rollback in `backend/migrations/000020_schedule_calendar_attendance.down.sql`, while documenting that production rollback must preserve existing F-006 data.
- [X] T007 [US0] Verify fresh up/down, upgrade of an existing profile to `UTC`, constraints, duplicate keys and historical-data safety in `backend/internal/scheduling/migration_integration_test.go`.
- [X] T008 [US0] Write failing current-membership, archived-write denial, cross-circle privacy and retry-key tests in `backend/internal/scheduling/authorization_test.go` and `backend/internal/scheduling/idempotency_test.go`; write profile timezone read/update, invalid-zone, old-client compatibility and closed-account scrub tests in `backend/internal/profile/handler_test.go`, `backend/internal/profile/handler_integration_test.go`, `backend/internal/profile/repository_integration_test.go` and `backend/internal/auth/session_repository_integration_test.go`.
- [X] T009 [US0] Implement transaction-time circle-role authorization and scoped request replay in `backend/internal/scheduling/authorization.go` and `backend/internal/scheduling/idempotency.go`; add IANA timezone validation/read/update in `backend/internal/profile/service.go`, `backend/internal/profile/profile_queries.go`, `backend/internal/auth/models.go`, `backend/internal/auth/session_queries.go` and the existing profile/auth repositories/handlers as needed, including account-deletion scrub to `UTC`. Do not trust a Firebase token alone.
- [X] T010 [US0] Centralize approved F-006 route patterns in `backend/cmd/api/routes.go` and prepare the existing authenticated per-user rate-limit/error-envelope wiring points in `backend/internal/api/router.go`; register each route only with its completed story handler.
- [X] T011 [US0] Verify the existing authenticated route wiring and per-IP/per-user `429` behavior used by future F-006 handlers in `backend/internal/api/router_wiring_test.go` and `backend/cmd/api/router_test.go`; each story's contract tests cover its invalid input, safe response and cross-circle denial.

**Gate:** T005–T011 precede story handlers; failed migration/security tests block them.

## Phase 3 — US1 Manage a Circle Schedule (P1)

**Goal:** Teachers and supervisors manage multiple recurring entries with all accepted modes, timezone rules and future-only edits.

**Independent test:** Create two entries, edit/stop one, and confirm the other and immutable started history are unchanged.

- [X] T012 [US1] Write recurrence tests for weekly/biweekly weekday sets, positive day/week intervals, deduplicated selected dates, inclusive local end, distant future and no end date in `backend/internal/scheduling/recurrence_test.go`.
- [X] T013 [US1] Implement bounded on-demand occurrence generation with stable `(schedule_id, original_local_date)` identity and effective-dated revisions in `backend/internal/scheduling/recurrence.go`.
- [X] T014 [US1] Write timezone tests for gap-shift, first repeated occurrence, retained local start/end clocks, nominal-clock/duration validation including proposed 1–44,640 minute bounds, DST-resolved end, overnight and multi-day elapsed duration, and stored viewer-zone conversion in `backend/internal/scheduling/timezone_test.go`.
- [X] T015 [US1] Implement IANA-zone start resolution, retained local start/end input validation and UTC end computation without a future-date cap in `backend/internal/scheduling/timezone.go`.
- [X] T016 [US1] Write repository integration tests for multiple entries, mode-specific checks, series CAS conflicts, superseded unstarted exceptions and retained started/past revisions in `backend/internal/scheduling/schedule_repository_integration_test.go`.
- [X] T017 [US1] Implement parameterized schedule/revision/selected-date/exception persistence and indexed reads in `backend/internal/scheduling/schedule_queries.go` and `backend/internal/scheduling/schedule_repository.go`.
- [X] T018 [US1] Write service tests for teacher/supervisor rights, past-date rejection, create/view/change/stop, single-occurrence edit/cancel, retry identity and concurrent edit outcomes in `backend/internal/scheduling/schedule_service_test.go`.
- [X] T019 [US1] Implement schedule validation, versioned series changes, one-occurrence exceptions and cancellation under the recurring parent-schedule lock, updating virtual exception and any materialized detail atomically in `backend/internal/scheduling/schedule_service.go`; preserve one-offs and started/completed history.
- [X] T020 [US1] Write contract tests for circle schedule list/create, series patch and occurrence patch, including `400/401/403/404/409/422/429` and role denial, in `backend/internal/scheduling/schedule_handler_contract_test.go`.
- [X] T021 [US1] Implement those approved schedule handlers with safe warning/conflict responses in `backend/internal/scheduling/schedule_handler.go` and connect them in `backend/internal/api/router.go`.
- [X] T022 [US1] Write Flutter data/controller and widget tests for manager rights, every recurrence mode, optional title, local zone, past rejection, edit/stop, Arabic RTL/LTR and loading/error/retry/offline/success states in `mobile/test/features/scheduling/schedule_controller_test.dart` and `mobile/test/widget/scheduling/schedule_editor_test.dart`; test stored timezone read/edit and old profile payload fallback in `mobile/test/widget/profile/profile_form_test.dart` and `mobile/test/features/profile/application/profile_timezone_test.dart`.
- [X] T023 [US1] Implement the schedule API/controller/editor and connect the existing circle entry in `mobile/lib/features/scheduling/data/schedule_api_client.dart`, `mobile/lib/features/scheduling/application/schedule_controller.dart`, `mobile/lib/features/scheduling/presentation/schedule_editor_screen.dart` and `mobile/lib/features/circles/presentation/circle_detail_screen.dart`; expose stored timezone read/edit in `mobile/lib/features/profile/data/profile_api_client.dart`, `mobile/lib/features/profile/presentation/profile_screen.dart` and `mobile/lib/features/auth/data/auth_api_client.dart`.
- [X] T024 [US1] Run a teacher/supervisor end-to-end schedule create/edit/stop journey, including multiple entries and local-time display, in `mobile/integration_test/schedule_management_flow_test.dart`.

## Phase 4 — US2 Plan and Find Sessions (P1)

**Goal:** One-off and recurring occurrences share an authorized current-month calendar with retained history and truthful lifecycle.

**Independent test:** Create an unlinked one-off beside another circle's recurrence; one active student sees both, including later completed/cancelled history.

- [X] T025 [US2] Write REST contract tests for one-off create/edit/cancel, recurring occurrence start and personal month calendar, including F-005 `SessionStartResponse`, no-store/503 media behavior, authentication, role and error envelopes, in `backend/internal/scheduling/calendar_handler_contract_test.go`.
- [X] T026 [US2] Write integration tests for one-off future-only creation, default title, informational duration, retry safety and cancel/edit eligibility in `backend/internal/scheduling/planned_session_integration_test.go`.
- [X] T027 [US2] Implement one-off planned-session persistence and service methods using F-005 `sessions.scheduled_at` plus approved separate details in `backend/internal/scheduling/planned_session_queries.go` and `backend/internal/scheduling/planned_session_service.go`.
- [X] T028 [US2] Write F-005 regression tests for virtual-exception and materialized-detail cancellation denial, discovery exclusion, active End, ad-hoc compatibility, media-failure rollback, and start/cancel/series-edit races in `backend/internal/sessions/session_service_test.go` and `backend/internal/sessions/session_repository_connection_test.go`.
- [X] T029 [US2] Refactor F-005 start into a reusable single-transaction body; check planned exception/details cancellation before activation, preserve `SessionStartResponse` and 503/no-store semantics, and filter cancelled discovery in `backend/internal/sessions/session_repository.go`, `backend/internal/sessions/session_service.go` and `backend/internal/sessions/session_queries.go`.
- [X] T030 [US2] Write concurrent virtual cancellation between materialization and activation, series-edit, one-off start/cancel and duplicate-start tests proving one outcome/session per occurrence in `backend/internal/scheduling/occurrence_start_integration_test.go`.
- [X] T031 [US2] Implement idempotent occurrence materialization and F-005 guarded start in one transaction under the recurring parent-schedule lock, or one-off session lock, in `backend/internal/scheduling/occurrence_start_service.go`, `backend/internal/scheduling/occurrence_start_queries.go` and `backend/internal/sessions/session_repository.go`.
- [X] T032 [US2] Write month-query tests for stored viewer timezone, multiple circles, revoked/archived filtering, 31-day occurrences starting in earlier months, distant anchors, moved exceptions and past completed/cancelled items in `backend/internal/scheduling/calendar_service_test.go`.
- [X] T033 [US2] Implement bounded personal-month and circle calendar queries with per-rule duration lookback, arithmetic recurrence jump, stored-viewer-zone conversion and retained/moved history in `backend/internal/scheduling/calendar_service.go` and `backend/internal/scheduling/calendar_queries.go`.
- [X] T034 [US2] Implement one-off, occurrence-start and personal-calendar handlers against canonical OpenAPI in `backend/internal/scheduling/calendar_handler.go` and wire them in `backend/internal/api/router.go`.
- [X] T035 [US2] Write Flutter API and widget tests for current month in the stored profile timezone, earlier/later navigation, cross-circle identity by text and non-color cue, planning/viewer timezone display, cancelled/completed states, RTL/LTR and offline/error states in `mobile/test/features/scheduling/calendar_controller_test.dart` and `mobile/test/widget/scheduling/calendar_screen_test.dart`.
- [X] T036 [US2] Implement one-off form, month calendar, data/controller and navigation using the approved shell in `mobile/lib/features/scheduling/presentation/one_off_screen.dart`, `mobile/lib/features/scheduling/presentation/calendar_screen.dart`, `mobile/lib/features/scheduling/data/calendar_api_client.dart` and `mobile/lib/features/scheduling/application/calendar_controller.dart`.
- [X] T037 [US2] Verify real backend one-off/recurrence visibility, start/end/cancel display, multi-circle and revoked-member denial in `mobile/integration_test/schedule_calendar_flow_test.dart`.

## Phase 5 — US3 Review and Correct Attendance (P1)

**Goal:** Completed planned/ad-hoc sessions have durable roster-based attendance and teacher-only audited correction.

**Independent test:** End a session with known presence, review every eligible student, correct one record and verify raw presence is unchanged.

- [X] T038 [US3] Write classification tests for exactly/after ten minutes, reconnects/devices, no minimum duration, absent/excused rules, cancelled/never-started exclusion and later-enrolled participant eligibility in `backend/internal/attendance/classifier_test.go`.
- [X] T039 [US3] Implement pure first-authorized-presence classification from F-005 facts in `backend/internal/attendance/classifier.go`.
- [X] T040 [US3] Write PostgreSQL integration tests for actual-start roster snapshot, later-enrolled authorized join followed by membership removal before end, concurrent revocation before join commit, automatic/manual end and recovery retries, unique attendance rows and unchanged presence in `backend/internal/attendance/attendance_repository_integration_test.go`.
- [X] T041 [US3] Implement start snapshot, transaction-time `circle_members FOR KEY SHARE` student-role recheck plus join-time later-participant eligibility upsert, and finalization from durable roster, with narrow F-005 activation/join/end/recovery hooks in `backend/internal/attendance/attendance_queries.go`, `backend/internal/attendance/attendance_repository.go`, `backend/internal/sessions/session_repository.go`, `backend/internal/sessions/session_service.go` and `backend/internal/sessions/reconciler.go`.
- [X] T042 [US3] Write tests for teacher-only correction, append-only actor/time/reason/previous/new audit, duplicate retry and recalculation precedence in `backend/internal/attendance/correction_service_test.go`.
- [X] T043 [US3] Implement audited correction and effective-status precedence in `backend/internal/attendance/correction_service.go` and `backend/internal/attendance/correction_queries.go`.
- [X] T044 [US3] Write attendance GET/PATCH contract tests for teacher/supervisor/student scopes, revoked/non-member/cross-circle denial, archived read-only, invalid input, rate limits and safe error bodies in `backend/internal/attendance/attendance_handler_contract_test.go`.
- [X] T045 [US3] Implement role-filtered attendance reads and teacher correction handlers in `backend/internal/attendance/attendance_handler.go` and wire them in `backend/internal/api/router.go`.
- [X] T046 [US3] Write Flutter data/controller and widget tests for roster/status, manual correction reason/audit, teacher/supervisor/student permissions, Arabic RTL/LTR and all required screen states in `mobile/test/features/attendance/attendance_controller_test.dart` and `mobile/test/widget/attendance/attendance_screen_test.dart`.
- [X] T047 [US3] Implement attendance API/controller and review/correction UI in `mobile/lib/features/attendance/data/attendance_api_client.dart`, `mobile/lib/features/attendance/application/attendance_controller.dart` and `mobile/lib/features/attendance/presentation/attendance_screen.dart`.
- [X] T048 [US3] Verify real backend planned and ad-hoc attendance, correction persistence, role denial and archived history in `mobile/integration_test/attendance_flow_test.dart`; passed on emulator-5554 against the local API (PostgreSQL + LiveKit dev stack) with the disposable Firebase Identity Toolkit fixture on 2026-09-29.

## Phase 6 — US4 Avoid Scheduling Surprises (P2)

**Goal:** Managers see authorized advisory warnings before saving; students see only their own calendar warnings.

**Independent test:** An overlap warns a teacher/supervisor and still saves on confirmation; a touching interval produces no warning.

- [X] T049 [US4] Write overlap tests for half-open boundaries, same-/cross-circle commitments, first 31 local days, later viewed months, finite selected dates, authorized circle names/UTC overlap intervals and privacy-safe projection in `backend/internal/scheduling/overlap_service_test.go`.
- [X] T050 [US4] Implement bounded preview and authorized half-open UTC overlap evaluation without a future scheduling cap in `backend/internal/scheduling/overlap_service.go`.
- [X] T051 [US4] Write preview/write contract tests for actionable safe warning details, echo of reviewed warning IDs, 409 with refreshed warnings when an overlap appears or changes after preview despite `confirm_overlaps=true`, non-overlap, role denial and no private member data in `backend/internal/scheduling/overlap_handler_contract_test.go`.
- [X] T052 [US4] Implement preview handler and recomputed warning-ID confirmation in schedule/one-off writes in `backend/internal/scheduling/overlap_handler.go` and `backend/internal/scheduling/schedule_service.go`.
- [X] T053 [US4] Write student month-warning tests proving only that student's eligible calendar is compared and participation is never restricted in `backend/internal/scheduling/calendar_warning_test.go`.
- [X] T054 [US4] Add viewer-only calendar warnings to the month response in `backend/internal/scheduling/calendar_service.go`.
- [X] T055 [US4] Write Flutter tests for actionable overlap intervals/circle names, refreshed 409 warnings requiring renewed manager confirmation, student informational view, RTL/LTR and non-color cues in `mobile/test/widget/scheduling/overlap_warning_test.dart`.
- [X] T056 [US4] Implement manager warning-ID confirmation/reconfirmation and student informational warnings in `mobile/lib/features/scheduling/presentation/overlap_warning.dart` and `mobile/lib/features/scheduling/presentation/calendar_screen.dart`.
- [X] T057 [US4] Verify overlap/confirm/no-overlap and no cross-member disclosure against a real backend in `mobile/integration_test/schedule_overlap_flow_test.dart`.

## Phase 7 — Cross-cutting verification and release boundary (US0)

- [X] T058 [US0] Write eligibility tests for configurable 1hr/30min/15min/5min offsets, cancellation and superseded-version exclusion, without asserting F-008 delivery, in `backend/internal/scheduling/reminder_eligibility_test.go`.
- [X] T059 [US0] Expose only stable, current occurrence/time eligibility for later F-008 consumption in `backend/internal/scheduling/reminder_eligibility.go`; add no FCM sender or preference/history store.
- [X] T060 [US0] Run focused and unfiltered API security, response-safety, idempotency and existing F-005 compatibility tests; record command/result and any remaining risk in `specs/006-schedule-calendar-attendance/quickstart.md`.
- [X] T061 [US0] Recheck REST parity, OpenAPI lint, architecture/ADR alignment and whether any actual new WebSocket event needs cataloging; apply `$docs-guard` to `docs/contracts/openapi.yaml` and update `docs/contracts/ws_events.md` only if an approved event exists.
- [X] T062 [US0] Run unfiltered Go unit, contract, integration, combined ≥80% coverage, lint, gofmt, migration up/down, API lint and secret scan; record exact results/unavailable prerequisites in `specs/006-schedule-calendar-attendance/quickstart.md`.
- [X] T063 [US0] Run fresh Flutter `flutter test test`, device-backed `flutter test integration_test/`, `flutter analyze` and `dart format --set-exit-if-changed .`; record exact results and block Flutter commits if unavailable in `specs/006-schedule-calendar-attendance/quickstart.md`.
- [ ] T064 [US0] Run timed teacher/supervisor schedule and one-off creation (SC-001), multi-circle calendar, attendance and accessibility/RTL/LTR journeys with connected device/backend; record results and any visual review gap in `specs/006-schedule-calendar-attendance/quickstart.md`.
- [X] T065 [US0] Apply `$clean-code-guard`, `$test-guard`, Ponytail restraint, Tech Lead review and Karim's applicable manual security review; record findings/closure in `specs/006-schedule-calendar-attendance/quickstart.md`.
- [X] T066 [US0] Label a partial F-006 pilot accurately, leave full SC-009 open while F-008 is Proposed, and only after F-008 approval verify actual 1hr/30min/15min/5min foreground/background/closed-app delivery and stale-reminder suppression in `specs/006-schedule-calendar-attendance/quickstart.md`.

## Dependencies and critical path

`T001 ADR approval → T002 architecture → T003–T004 canonical contract → T005–T011 migration/security → T012–T024 US1 → T025–T037 US2 → T038–T048 US3 → T049–T057 US4 → T058–T065 validation → T066 full-completion dependency.`

US1, US2 and US3 are all P1, but US2 uses US1's recurrence identity and US3 uses US2's planned-start path; implement them in that order while preserving each story's independent acceptance test. US4 is P2 and is required for a complete F-006 pilot. After T011, disjoint recurrence tests and attendance classifier tests can be developed in separate files, but no `[P]` marker is pre-assigned while their prerequisites and shared integration boundaries remain open. F-008 implementation stays in its own approved feature lifecycle.

## Acceptance traceability

| Acceptance | Implementation tasks | Test/evidence tasks |
|---|---|---|
| Product: recurrence modes, multiple entries, local zone/end date; US1 scenarios 1–5; FR-001/002/006/013; SC-002/006 | T013, T015, T017, T019, T021, T023 | T012, T014, T016, T018, T020, T022, T024 |
| Product: unified calendar and accessible circle identity; US2 scenarios 1–2; FR-003–006/014; SC-001/003 | T027, T033–T034, T036 | T025–T026, T032, T035, T037, T064 |
| Product: planned lifecycle/cancellation and retained history; US2 scenarios 3–4; FR-007/013; SC-006 | T029, T031, T033–T034 | T028, T030, T032, T037 |
| Product: presence-derived attendance; US3 scenario 1; FR-009/010; SC-004/007 | T039, T041, T045, T047 | T038, T040, T044, T046, T048 |
| Product: manual override; US3 scenarios 2–3; FR-011/012; SC-004/005/007 | T043, T045, T047 | T042, T044, T046, T048, T060 |
| Product: advisory overlaps; US4 scenarios 1–2; FR-008; SC-008 | T050, T052, T054, T056 | T049, T051, T053, T055, T057 |
| Product: configurable push reminder intervals; FR-015; SC-009 | T059 plus F-008-owned delivery after its separate approval | T058, T066; full acceptance stays open until real delivery evidence exists |
| Cross-story: current-role denial, archive, retry/concurrency, response safety, rate limits; FR-012–014; SC-005/006/007 | T005, T009–T010, T017, T019, T029, T031, T041 | T007–T008, T011, T016, T018, T020, T028, T030, T040, T044, T060–T065 |

## MVP delivery order

1. Complete governance/contract and foundation gates. They are required, not optional polish.
2. Deliver US1 → US2 → US3 → US4 and validate each independent journey. The F-006 partial pilot includes all four stories, security, accessibility, and warning behavior.
3. Keep T066 open for full completion until F-008 is approved and verified. No placeholder or unrun test satisfies push acceptance.
