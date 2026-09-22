# Wave 2 Evidence — Sessions & Recitation Queue (US3)

Date: 2026-09-22 · Branch: `019-mobile-app-shell-brand` · Scope: T020–T028

## T027 — Session/queue verification

Fresh results on the final code — after the Tech Lead review fixes (F1–F5
plus two nits) **and** the T028 role-agent review fixes (UX/UI sections
below):

| Gate | Command | Result |
|---|---|---|
| Session/queue unit+widget | `flutter test test/widget/sessions test/features/sessions` | **141/141 passed** (includes the 8 new review-fix tests) |
| Full unit/widget suite | `flutter test test` | **416/416 passed** ([log](log-test-wave2.txt)) |
| Queue device journeys | `flutter test integration_test/queue_flow_test.dart integration_test/queue_grading_history_test.dart integration_test/queue_late_join_opt_out_test.dart -d emulator-5554` | **2 passed, 2 skipped** ([log](log-test-wave2-integration.txt)) |
| Wave 2 visual harness | `flutter drive --no-dds --driver=test_driver/wave2_screenshot_driver.dart --target=integration_test/wave2_sessions_visual_test.dart -d emulator-5554` | **All tests passed**; 48 PNGs captured ([log](log-drive-wave2.txt)) |
| Static analysis | `flutter analyze` | **No issues found** ([log](log-analyze-wave2.txt)) |
| Format | `dart format --output=none --set-exit-if-changed .` | **0 changed** ([log](log-format-wave2.txt)) |
| Diff hygiene | `git diff --check` | clean |

As in Wave 1, the Windows Java selector fails with the default long temporary
path; the device commands above ran with process-local `TEMP`/`TMP` set to
`C:\jtmp`. No repository or machine configuration was changed.

Integration skips are environmental, not failures:
`queue_grading_history_test.dart` (T062) and `queue_late_join_opt_out_test.dart`
(T048) self-skip because their `T062_*`/`T048_*` env vars (pre-provisioned
Firebase tokens and backend sessions) are unavailable in this session.
`queue_flow_test.dart` ran without skips, including "manager mutations keep
the separate student microphone untouched" (the T026/FR-031 audio-isolation
journey).

### Tech Lead review fixes applied in this wave

Verdict was approve-with-fixes; all fixes landed and are covered by the fresh
gates above:

- **F1** `queue_manager_panel.dart`: `_dominantAction` no longer compares
  localized label strings; it returns a private `_DominantAction` enum
  (`prepare/advance/start/complete`) compared at the `_QueueAction` call sites.
- **F2** `circle_management_screen.dart` / `circle_retirement_screen.dart`:
  LTR AppBar titles now reuse `CircleDetailLabels.manageAr/manageEn` and
  `archiveAr/archiveEn` instead of inline literals.
- **F3** `circle_sessions_controller.dart`: a failed create keeps the previous
  list `status` (e.g. ready) and sets only `failure`, so an empty section no
  longer flips to the misleading "could not load sessions" copy when the
  snackbar already reported the create failure. Locked by an updated test and
  a new empty-list case in `circle_sessions_controller_test.dart`.
- **F4** `session_room_start_join_test.dart`: new test renders the retryable
  error header (status copy, `sessionRoomRetry`/`sessionRoomLeave`, primary
  action) at 2.0 text scale on a 360×640 surface and asserts no overflow.
- **F5** `session_room_moderation_test.dart`: new negative test asserts a
  non-moderator never sees `sessionRoomEndSession`.
- **Nits**: `_QueueLabels.roundTypeName` falls back to the raw value for
  unknown round types instead of borrowing the "Test" label; the participant
  Remove `TextButton` in `session_room_screen.dart` now enforces a 48dp
  minimum size (FR-013).

Only the Remove-button nit alters a captured state (manager screens with
participant rows). The 48-PNG matrix was recaptured after these fixes and
again after the later T028 role-agent fix wave; the screenshots below reflect
the final code.

## T027 — RTL/LTR × light/dark screenshot matrix

Captured on `emulator-5554` at 1080×2400 using fresh provider stubs for every
state; the harness detaches the preceding tree before each state and waits for
its sentinel to remain visible. Harness: `mobile/integration_test/wave2_sessions_visual_test.dart`
+ `mobile/test_driver/wave2_screenshot_driver.dart`.

48 PNGs are stored under [screenshots/wave2/](screenshots/wave2/) — twelve
states × Arabic RTL / English LTR × light / dark:

| State | Files | Verified content |
|---|---|---|
| Sessions empty (member) | `wave2_circle_sessions_empty_member_{ar,en}_{light,dark}.png` | Localized empty-state card; no create action for non-managers |
| Sessions load error | `wave2_circle_sessions_error_{ar,en}_{light,dark}.png` | Safe error copy with retry; no raw error text |
| Sessions loaded (manager) | `wave2_circle_sessions_loaded_manager_{ar,en}_{light,dark}.png` | Session tiles with status/participant copy; create action visible |
| Student connected | `wave2_student_connected_{ar,en}_{light,dark}.png` | Audio-only header, connected status, own-row "You" badge in the queue list, no Start/Join action while connected |
| Student current turn | `wave2_student_current_turn_{ar,en}_{light,dark}.png` | Turn announcement distinguishes the student's own turn; own-row badge |
| Student opt-out pending | `wave2_student_optout_pending_{ar,en}_{light,dark}.png` | Pending opt-out state without a misleading retry |
| Student reconnecting | `wave2_student_reconnecting_{ar,en}_{light,dark}.png` | Reconnecting status announced; queue snapshot retained |
| Student terminal | `wave2_student_terminal_{ar,en}_{light,dark}.png` | Terminal copy with "retry unavailable"; primary action disabled; safe Leave exit |
| Manager queue ready | `wave2_manager_queue_ready_{ar,en}_{light,dark}.png` | Full queue action set; contextually-next action is the single filled/dominant button (Start/Join collapses once connected) |
| Manager grading | `wave2_manager_grading_{ar,en}_{light,dark}.png` | Grading panel for the selected entry with localized contract grades only; selected grade is filled tonal |
| Manager moderation | `wave2_manager_moderation_{ar,en}_{light,dark}.png` | Per-participant Mute/Remove (48dp targets); End session separated below a divider in the error color role; scroll anchored at the participants header |
| Reset-round dialog | `wave2_dialog_reset_confirm_{ar,en}_{light,dark}.png` | Visible field labels above inputs; consequence copy ("current round progress will be discarded"); confirm/cancel mirrored correctly in RTL |

All 48 files were checked for clipping, overflow, incorrect directionality,
raw identifiers, placeholder/developer copy, and theme regressions. The matrix
contains no debug banner or visual-test terminology. Every capture reflects
the amended contrast tokens (UI-M2/M3) and the role-agent fix wave below.

## T028 — UX Designer review

Reviewer of record: dedicated `ux-designer` role agent dispatched by the
orchestrator. Verdict: **approve with fixes**. All findings resolved or
documented below; fixes are covered by the fresh gates above.

- **UX-M1** Opt-out is now disabled while the queue is not `ready`
  (`loading`/`reconnecting`/`recoverableError`): `_OptOutFeedback` takes an
  `interactive` flag and `queue_student_panel.dart` passes
  `interactive: status == QueueStudentPanelStatus.ready`. Covered by "keeps
  the opt-out action disabled while the queue is not ready".
- **UX-M2** "Reorder queue"/"Queue policy" no longer show the misleading
  "choose the round details first" snackbar. Investigation evidence:
  `QueueController.reorder`/`updatePolicy` exist and work when driven
  programmatically (the queue integration journeys call them), but
  `SessionRoomScreen` has no UI to build their payloads (target order /
  policy fields), so the actions are never functional from this screen.
  Resolution per T025: both callbacks route to the shared
  `showHalaqatyUnderImplementationNotice`; `_showQueueActionPrompt` was
  deleted. Covered by "reorder and policy actions surface the shared
  under-implementation notice".
- **UX-M3** The dominant Start/Join `FilledButton` is now collapsed once the
  room is `connected` — exactly one truthful dominant action per state, with
  the status section carrying the connected copy. Tests that re-joined while
  connected were moved to the real recovery path (realtime close + Retry);
  new coverage: "connected state shows no enabled Join/Start primary action"
  and "a retry after a prior connection announces reconnecting".
- **UX-M4** Grade values are localized everywhere: new
  `SessionUiLabels.gradeLabel` (excellent/good/acceptable/needs_review/repeat
  → ممتاز/جيد/مقبول/يحتاج مراجعة/إعادة) backs the grade dialog dropdown, the
  manager entry-row grade + semantics, and the student/grading panels. No raw
  contract values render (FR-028). Covered by "renders localized grade copy,
  never the raw contract value" and the updated grade-dialog test.
- **UX-M5** Documented tap-budget exception — no code change: the
  circle → session → room → join path is 4 taps because join/start requires
  the explicit in-room transition per F-005; accepted with the same rationale
  as the approved "create then start = 4" budget.
- **UX-m1** The student's own queue row carries a "أنت"/"You" badge
  (`secondaryContainer` pill per the amended contrast tokens).
- **UX-m2** The grading save action distinguishes completion ("حفظ التقييم"/
  "Save grade") from correction ("حفظ التصحيح"/"Save correction").
- **UX-m3** With a prior connection, the room status section announces
  "جارٍ إعادة الاتصال..."/"Reconnecting..." instead of the first-load copy.
- **UX-m4** (with UI-m11) Arabic participant-count pluralization corrected:
  0 → "لا مشاركين", 1 → "مشارك واحد", 2 → dual "مشاركان", 3–10 →
  "N مشاركين", 11+ → "N مشاركاً"; English keeps singular/plural.
- **UX-m5** The reset-round dialog states the consequence up front:
  "سيُتجاهل تقدم الجولة الحالية."/"Current round progress will be discarded."
- **UX-m6** No code — the 320dp/600dp/200%-text and manager-empty-variant
  captures are deferred to the Wave 5 audit; T043 owns the full matrix.

## T028 — UI Designer review

Reviewer of record: dedicated `ui-designer` role agent dispatched by the
orchestrator. Verdict: **approve with fixes**. All findings resolved or
documented below; fixes are covered by the fresh gates above.

- **UI-M1** `circle_sessions_section.dart` trailing icon is always
  `Icons.chevron_right` (`matchTextDirection: true` already mirrors it; the
  manual branch double-mirrored — the same class of bug Wave 1 fixed).
- **UI-M2 + UI-M3** Theme contrast amendment (user-approved; recorded in
  `docs/engineering/design/DESIGN.md` — "Token amendment 2026-09-22").
  Measured WCAG 2.1 ratios, old → new:
  - light `onSecondaryContainer` on `#E8C8A0`: 2.96:1 → `#3F2E1B` = **8.16:1**
  - dark `secondaryContainer`/`onSecondaryContainer`: 2.96:1 →
    `#5D4729`/`#F0DDBE` = **6.58:1**
  - dark `onPrimary` on `#4CB368`: 3.34:1 → `#062B14` = **5.83:1**
  Light `primary` `#1B7E3C` and all other tokens unchanged; the relational
  NavigationBar indicator tests stay green.
- **UI-M4** The selected grade in `queue_grading_panel.dart` renders as a
  filled tonal button (unselected stay outlined), not semantics-only.
  Covered by "marks the selected grade with a filled tonal treatment".
- **UI-m5** Resolved by UX-M3: the queue panel's filled dominant action can
  no longer coexist with a filled Start/Join button, because the room's
  primary action collapses on connect — one filled dominant action per
  screen.
- **UI-m6** `logo_monochrome.svg` is now a single `fill="currentColor"` path
  (`fill-rule="evenodd"`, viewBox/paths preserved); `HalaqatyLogo` tints it
  via `ColorFilter.mode(scheme.onSurfaceVariant, BlendMode.srcIn)`.
- **UI-m7** The `manager_moderation` captures anchor the scroll at the
  participants header (settled offset) instead of centering the End-session
  button, so no row is half-clipped; the full matrix was recaptured.
- **UI-m8** Sessions error-card padding 20 → 24 (8px scale).
- **UI-m9** Raw `TextStyle(color:)` call sites (`circle_sessions_section.dart`,
  `session_room_screen.dart` action-error and error announcement) now derive
  from `textTheme` styles with `copyWith`.
- **UI-m10** (with UX-m1) Student queue entries render in position order with
  the own-row badge, not peers-first.
- **UI-m12** The per-participant Mute `TextButton` enforces
  `minimumSize: Size(48, 48)` like Remove (FR-013); asserted in the
  moderation destructive-actions test.

