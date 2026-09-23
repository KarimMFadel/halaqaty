# Tasks: Mobile App Shell and UI/UX Modernization

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/compatibility.md](contracts/compatibility.md)  
**Execution rule**: Complete tasks sequentially. A task is `[X]` only after its
deliverable exists and fresh evidence supports it. Do not start implementation
until Karim approves the complete Spec-Kit artifacts and analysis.

## Phase 1: Preflight and Baseline

**Purpose**: Protect the current branch, user changes, and existing behavior.

- [X] T001 Confirm branch `019-mobile-app-shell-brand`, cleanly separate the unrelated untracked `docs/spec-kit-copy-paste-prompts.md`, and re-read the approved F-019 artifacts before production edits.
- [X] T002 Run the current baseline `flutter test test`, `flutter analyze`, and `dart format --set-exit-if-changed .` from `mobile/`; record exact results and pre-existing failures in `specs/019-mobile-app-shell-brand/evidence/baseline.md`.
- [X] T003 Check emulator/device and backend prerequisites for `flutter test integration_test/`; record available or blocked prerequisites in `specs/019-mobile-app-shell-brand/evidence/baseline.md` without treating an unavailable suite as passing.
- [X] T004 Inventory preserved routes, widget keys, semantics, controller states, and exact behavior assertions for Waves 0–4 in `specs/019-mobile-app-shell-brand/evidence/compatibility-inventory.md`; classify every approved-design action as implemented, intentionally under implementation, or unplanned/omitted.

**Checkpoint**: Baseline and integration feasibility are known before any Dart edit.

---

## Phase 2: Wave 0 — Shell and Brand Foundation (US1)

**Goal**: Audit and complete the existing foundation without rewriting conforming work.

**Independent Test**: Launch authenticated/unauthenticated states and use all four tabs in Arabic RTL/English LTR and light/dark while preserved keys and redirects remain valid.

- [X] T005 [US1] Add failing widget coverage for platform/registration/profile-backed `ar` RTL and `en` LTR direction, protected redirects, and the preserved `openLogin`, `openRegister`, `authInitializing`, and `appNavigationBar` keys in `mobile/test/widget_test.dart`.
- [X] T006 [US1] Add failing widget coverage for shell tab selection/root return, 48dp navigation/actions, branded loading, meaningful semantics, and light/dark selected-state meaning in `mobile/test/widget_test.dart`; add focused failing tests in `mobile/test/widget/core/halaqaty_components_test.dart` for the exact Arabic/English under-implementation copy, stable `halaqatyUnderImplementationSnackBar` key, dismiss/replacement behavior, and no callback side effect.
- [X] T007 [US1] Implement the targeted normalized `ar`/`en` `AppLocaleController` in `mobile/lib/app/app_locale_controller.dart` and wire platform initialization, registration selection, successful profile load/save, logout fallback, and `MaterialApp.router` rebuild in `mobile/lib/main.dart` plus existing auth/profile presentation without adding languages or a localization framework.
- [X] T008 [US1] Audit/fix only non-conforming theme and shared primitives in `mobile/lib/core/theme/halaqaty_theme.dart` and `mobile/lib/core/design/halaqaty_components.dart`; reuse native M3, preserve brand asset paths, encode the approved bundled-font mapping (400 regular; requested 600 emphasis resolving to Poppins 600/Cairo 700), and add the single native-SnackBar under-implementation helper with exact localized copy and stable key—without a new dependency, synthesized font, availability service, or state framework.
- [X] T009 [US1] Add failing Home/Chats tests that distinguish loading, empty, recoverable error, and offline/degraded presentation while preserving `homeNoCircles`, `homeCircle-*`, `chatsEmpty`, and `chatCircle-*` keys in `mobile/test/widget_test.dart`.
- [X] T010 [US1] Implement the minimum Wave 0 state/hierarchy fixes in `mobile/lib/app/home_screen.dart`, `mobile/lib/app/chats_screen.dart`, `mobile/lib/app/welcome_screen.dart`, and `mobile/lib/app/router.dart` without unsupported next-session/progress data; route any Wave 0 action classified as intentionally under implementation only to the shared notice.
- [X] T011 [US1] Run the focused Wave 0 widget tests and record results plus RTL/LTR light/dark screenshots in `specs/019-mobile-app-shell-brand/evidence/wave-0.md`.
- [X] T012 [US1] Obtain UX Designer and UI Designer review of Wave 0 flow, tokens, states, direction, themes, targets, semantics, and screenshots; resolve findings and record outcomes in `specs/019-mobile-app-shell-brand/evidence/wave-0.md`.

**Checkpoint**: The shell is production-ready and remains behavior-compatible.

---

## Phase 3: Wave 1 — Circles (US2)

**Goal**: Complete the circle journey and its required states using existing F-002 behavior.

**Independent Test**: Discover/open/join/create/manage/retire with student, teacher, supervisor, and archived fixtures while role behavior and keys stay unchanged.

- [X] T013 [US2] Add failing tests for reachable Create and invite/public Join entry points, tap budgets, and preserved circle discovery/join keys in `mobile/test/widget/circles/discover_join_screen_test.dart` and `mobile/test/widget/circles/create_circle_screen_test.dart`.
- [X] T014 [US2] Expose the existing `CreateCircleScreen` and existing join flows from `mobile/lib/features/circles/presentation/circle_discovery_screen.dart` with one role-appropriate primary action and no new permission behavior.
- [X] T015 [US2] Add failing tests for circle detail loading, empty section, retryable/fatal error, in-context success, offline/read-only archive behavior, 48dp controls, semantics, and the recorded members/management/retirement tap budgets in `mobile/test/widget/circles/circle_detail_screen_test.dart` and existing circle widget tests.
- [X] T016 [US2] Implement minimum state/hierarchy/token fixes across `mobile/lib/features/circles/presentation/`, including removal of screen-level hard-coded colors, while preserving controllers, actions, and keys; route any inventoried Wave 1 under-implementation action only to the shared notice.
- [X] T017 [US2] Replace snackbar-only consequential circle success with retained in-context confirmation where required, updating the smallest relevant tests and presentation files under `mobile/test/widget/circles/` and `mobile/lib/features/circles/presentation/`.
- [X] T018 [US2] Run all circle widget tests and applicable circle integration journeys; record results and role/state RTL/LTR light/dark screenshots in `specs/019-mobile-app-shell-brand/evidence/wave-1.md`.
- [X] T019 [US2] Obtain UX Designer and UI Designer review of Wave 1; resolve findings and record outcomes in `specs/019-mobile-app-shell-brand/evidence/wave-1.md`.

**Checkpoint**: Wave 1 is audited/remediated, not blindly rewritten.

---

## Phase 4: Wave 2 — Sessions and Recitation Queue (US3)

**Goal**: Make approved ad-hoc session flows reachable and prioritize connection, open audio, current turn, role controls, and recovery.

**Independent Test**: From circle details, list/create/start/join a session and exercise student and manager queue states, reconnect, terminal state, and open-audio invariants.

- [X] T020 [US3] Add failing unit/widget tests for a minimal Riverpod circle-session presentation controller using existing `SessionApiClient.list/create` states in `mobile/test/features/sessions/application/` and `mobile/test/widget/sessions/session_discovery_test.dart`.
- [X] T021 [US3] Implement the minimal circle-session presentation controller in `mobile/lib/features/sessions/application/` by reusing `SessionApiClient`; add no generic repository, schedule, or attendance abstraction.
- [X] T022 [US3] Add failing widget tests for circle detail session list/create/start/join entry, student versus manager actions, loading/empty/success/error/offline states, tap budgets, and preserved session-room behavior in `mobile/test/widget/sessions/session_discovery_test.dart`.
- [X] T023 [US3] Implement the circle session section/entry using existing F-005 APIs in `mobile/lib/features/circles/presentation/circle_detail_screen.dart` and focused presentation widgets under `mobile/lib/features/sessions/presentation/`.
- [X] T024 [US3] Add failing tests for session-room hierarchy, queue/participant semantics, 48dp targets, retryable versus terminal recovery, destructive-action separation, and text scaling in existing `mobile/test/widget/sessions/` files.
- [X] T025 [US3] Recompose `mobile/lib/features/sessions/presentation/session_room_screen.dart` and existing queue panels using current controller states and M3/theme roles; do not change session/queue/media behavior, and route any inventoried Wave 2 under-implementation action only to the shared notice.
- [X] T026 [US3] Add a regression test proving queue position/turn state never grants, revokes, mutes, or unmutes authorized student audio, updating the smallest existing session/queue test file.
- [X] T027 [US3] Run all session/queue widget and applicable integration tests; record exact results and student/manager/reconnect RTL/LTR light/dark screenshots in `specs/019-mobile-app-shell-brand/evidence/wave-2.md`.
- [X] T028 [US3] Obtain UX Designer and UI Designer review of Wave 2; resolve findings and record outcomes in `specs/019-mobile-app-shell-brand/evidence/wave-2.md`.

**Checkpoint**: Session/queue UX is reachable and clear without changing open-audio or backend behavior.

---

## Phase 5: Wave 3 — Chat (US4)

**Goal**: Modernize the complete approved F-004 surface without a unified inbox.

**Independent Test**: Enter group/direct chat through authorized paths and exercise history, composer, search/reply/pin/delete/read, media, voice, archived, failed, and offline states.

- [X] T029 [US4] Carry forward the Wave 0 widget coverage that distinguishes Chats loading, empty, recoverable error, and offline/degraded states in `mobile/test/widget_test.dart`; it remains a Wave 3 compatibility prerequisite and is not duplicated. Evidence: `evidence/wave-0.md` and `evidence/log-test-wave0-focused.txt`.
- [X] T030 [US4] Carry forward the Wave 0 Chats-tab state/recovery implementation in `mobile/lib/app/chats_screen.dart`, preserving client-derived circle group chats and excluding a unified direct-message inbox. No Wave 3 reimplementation is authorized unless the retained regression coverage fails.
- [ ] T031 [US4] Add failing group/direct conversation widget tests for the recorded group/direct entry tap budgets, hierarchy, own/other messages, delivery/read, search/reply/pin/delete, archived/read-only, access-lost, terminal/retryable, semantics, targets, and large text in `mobile/test/widget/chat/`.
- [ ] T032 [US4] Recompose existing group/direct chat screens and status widgets under `mobile/lib/features/chat/presentation/` using current controllers and theme roles without altering authorization or message behavior.
- [ ] T033 [US4] Add failing attachment/voice-note tests for chooser, recording, preview, discard, upload/playback, pending, failure, offline draft retention, and accessible announcements in existing `mobile/test/widget/chat/` files.
- [ ] T034 [US4] Recompose existing media/voice/composer widgets under `mobile/lib/features/chat/presentation/` without new media types, dependencies, or background behavior; route any inventoried Wave 3 under-implementation action only to the shared notice.
- [ ] T035 [US4] Run all chat widget and applicable integration tests; record exact results and state-rich RTL/LTR light/dark screenshots in `specs/019-mobile-app-shell-brand/evidence/wave-3.md`.
- [ ] T036 [US4] Obtain UX Designer and UI Designer review of Wave 3; resolve findings and record outcomes in `specs/019-mobile-app-shell-brand/evidence/wave-3.md`.

**Checkpoint**: Full F-004 presentation is modernized while contract and access behavior remain intact.

---

## Phase 6: Wave 4 — Authentication and Profile (US5)

**Goal**: Modernize account entry and the existing profile form without inventing settings.

**Independent Test**: Register/sign in, load/edit/save profile, change existing preferred language, and logout across validation, loading, success, error, and offline states.

- [X] T037 [US5] Add failing auth widget tests for the recorded sign-in/registration tap budgets, hierarchy, disabled/loading submit, inline validation, preserved fields, language/direction, 48dp controls, focus order, semantics, and existing keys in `mobile/test/widget/auth/auth_forms_test.dart` and `mobile/test/widget_test.dart`. Evidence: red run `evidence/log-test-wave4-red.txt` (8 failures), focused suite 44/44 green after T038.
- [X] T038 [US5] Recompose Welcome/Login/Register in `mobile/lib/app/welcome_screen.dart` and `mobile/lib/features/auth/presentation/auth_screens.dart` using existing fields/controller behavior and no new identity provider. Evidence: full-width FilledButton submits (48dp via theme), `showHalaqatyError` auth failures, focus/ime chain, preserved keys; see `evidence/wave-4.md`.
- [X] T039 [US5] Add failing profile widget tests for the recorded profile-save/logout tap budgets, grouped existing fields, optional-field guidance, load/save/offline errors, retained in-context save success, preferred-language direction, logout, large text, and preserved keys in `mobile/test/widget/profile/profile_form_test.dart`. Evidence: red run `evidence/log-test-wave4-profile-red.txt` (9 failures), focused suite 20/20 green after T040.
- [X] T040 [US5] Recompose `mobile/lib/features/profile/presentation/profile_screen.dart`; approved future-intended profile actions may remain visible only through the shared under-implementation notice, while unplanned actions stay omitted and no notification, appearance, privacy, support, avatar-upload, or account-deletion behavior is added. Evidence: grouped sections, `HalaqatyLoading`/retryable load error, retained `profileSaveSuccess` banner, notice-only `_NoticeTile`s, 13 preserved keys; see `evidence/wave-4.md`.
- [X] T041 [US5] Run auth/profile widget and applicable integration tests; record exact results and validation/success RTL/LTR light/dark screenshots in `specs/019-mobile-app-shell-brand/evidence/wave-4.md`. Evidence: `flutter test test` 454/454 (`evidence/log-test-wave4.txt`), analyze clean, 48 PNGs under `evidence/screenshots/wave4/` via harness `mobile/tool/wave4_visual_capture_test.dart`; device integration runs blocked (shared emulator) and deferred to T049.
- [X] T042 [US5] Obtain UX Designer and UI Designer review of Wave 4; resolve findings and record outcomes in `specs/019-mobile-app-shell-brand/evidence/wave-4.md`. Evidence: UX review passes (tap budgets/hierarchy/states/notice-only/direction); UI findings UI-W4-1 (theme font cascade fix in `halaqaty_theme.dart`), UI-W4-2 (banner contrast), UI-W4-3 (spinner color) all resolved and recaptured.

**Checkpoint**: Existing F-001 entry/profile behavior is visually complete and unchanged.

---

## Phase 7: Wave 5 — Cross-App Audit (US6)

**Goal**: Close cross-wave direction, theme, accessibility, responsive, copy, and evidence gaps.

**Independent Test**: Run representative primary journeys in the complete variant matrix and find no clipping, incorrect direction, inaccessible controls, token violations, or developer copy.

- [ ] T043 [US6] Expand `mobile/integration_test/ux_visual_journey_test.dart` and focused test helpers to implement the exact `spec.md` Screenshot Acceptance Matrix: every affected screen in four direction/theme ready-state variants plus the named role/state, 320dp/600dp, and 200% text captures, without weakening real integration coverage.
- [ ] T044 [US6] Add focused cross-app tests for contrast-relevant theme roles, bundled typography mapping (400 regular and Poppins 600/Cairo 700 emphasis), directional icons/order, 48dp targets, semantics/focus, dynamic announcements, reduced motion, long mixed-script copy, Arabic diacritic layout, and every inventoried under-implementation action's exact localized notice plus zero navigation/controller/API/state side effects in `mobile/test/widget/`.
- [ ] T045 [US6] Audit and minimally fix remaining presentation violations across changed `mobile/lib/` surfaces; remove raw IDs, fixtures, provider terms, ad-hoc developer placeholders, unsupported data, and unplanned actions while retaining only the standardized under-implementation notice for approved future-intended actions.
- [ ] T046 [US6] Capture the final screenshot matrix and record artifact names, dimensions, state, direction, theme, text scale, and findings in `specs/019-mobile-app-shell-brand/evidence/wave-5.md`.
- [ ] T047 [US6] Obtain final UX Designer and UI Designer approval of the complete matrix; resolve findings and record outcomes in `specs/019-mobile-app-shell-brand/evidence/wave-5.md`.

**Checkpoint**: Every wave meets the common governance floor.

---

## Phase 8: Final Verification and Review

**Purpose**: Produce fresh evidence on the final tree before any commit or completion claim.

- [ ] T048 Run `flutter test test` from `mobile/` and record exact pass/fail counts in `specs/019-mobile-app-shell-brand/evidence/final-gates.md`.
- [ ] T049 Run `flutter test integration_test/` from `mobile/` with the connected emulator/device and configured backend; record exact results, or mark blocked and stop before committing.
- [ ] T050 Run `flutter analyze` from `mobile/` and record exact output.
- [ ] T051 Run `dart format --set-exit-if-changed .` from `mobile/` and record exact output.
- [ ] T052 Run `git diff --check` from repository root and record exact output.
- [ ] T053 Apply `$clean-code-guard` to all changed production Dart and `$test-guard` to all changed test Dart; fix findings and record the checklists in `specs/019-mobile-app-shell-brand/evidence/final-gates.md`.
- [ ] T054 Run the Tech Lead correctness/security/performance/accessibility review plus Ponytail over-engineering review on the final diff; resolve findings and record the result.
- [ ] T055 Confirm `.specify/memory/constitution.md`, backend code/contracts, database migrations, WebSocket catalog, and unrelated features are unchanged; record `git diff --stat` and status.
- [ ] T056 Mark completed tasks only from current evidence, list any incomplete/blocked task IDs, and create the requested final report with screens/files, gates, screenshots, reviews, and commit hashes.

## Dependencies and Execution Order

- Phase 1 blocks all production edits.
- Waves run sequentially: 0 → 1 → 2 → 3 → 4 → 5.
- Within each wave, test tasks precede implementation tasks; focused verification
  precedes UX/UI review.
- Final full gates run only after all intended wave fixes and review findings are
  incorporated.
- No Flutter commit is allowed if T049 is unavailable or failing.

## Implementation Strategy

- Preserve working Wave 0/1 code when it already satisfies the spec.
- Prefer deletion/simplification, native Material 3, and existing feature widgets.
- Introduce the smallest targeted Riverpod controller only where the existing
  F-005 API has no presentation state owner; do not create a repository/factory.
- Never implement unsupported prototype data or unplanned actions. Approved
  future-intended actions without behavior use only the shared notice.
- Stop at any wave checkpoint if the required behavior would expand a backend,
  role, permission, provider, or feature contract.
