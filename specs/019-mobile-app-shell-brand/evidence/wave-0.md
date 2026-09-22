# Wave 0 Evidence — Shell and Brand Foundation (US1)

Date: 2026-09-22 · Branch: `019-mobile-app-shell-brand` · Scope: T001–T012

## T011 — Focused Wave 0 test run

Fresh command output (run after all Wave 0 edits, post-format):

| Gate | Command | Result |
|---|---|---|
| Unit/widget | `flutter test test` (in `mobile/`) | **363/363 passed** (`00:28 +363: All tests passed!`) |
| Static analysis | `flutter analyze` | **No issues found!** (7.1s) |
| Format | `dart format --set-exit-if-changed .` | **0 changed** (141 files scanned) |
| Diff hygiene | `git diff --check` | clean |

Baseline comparison: suite grew 341 → 363 tests (+22 US1 tests), zero regressions
(full log: [log-test-wave0.txt](log-test-wave0.txt)).

Wave 0 test coverage added:

- `mobile/test/widget_test.dart` — group `app locale and direction (US1)` (8 tests:
  platform-locale normalization ar/en/other→ar, registration `selectLanguage`,
  profile `applyProfileLanguage` on load and after save, `restorePlatformFallback`
  on logout, root `Directionality` RTL/LTR); group `shell quality gates (US1)`
  (48dp nav targets, branded splash semantics label, icon-shape selected nav
  state, dark theme mapping); group `home and chats states (US1)` (branded
  loading, degraded stale-content banners, chats error-vs-empty split).
- `mobile/test/widget/core/halaqaty_components_test.dart` — 5 tests for
  `showHalaqatyUnderImplementationNotice` (EN/AR copy, floating behavior,
  single-snackbar replacement, Dismiss action) — helper lands in Wave 4 (T042);
  tests live in Wave 0 per T009.

## T011 — RTL/LTR × light/dark screenshot matrix

Captured on `emulator-5554` (Android, 1080×2400) via
`mobile/integration_test/wave0_shell_visual_test.dart` +
`mobile/test_driver/wave0_screenshot_driver.dart` (`flutter drive --no-dds`),
real `MyApp` shell with faked auth/profile/circle providers. Harness notes:
each step detaches the previous tree before repumping (the global
`_homeNavigatorKey` in `router.dart` must not be reparented into a second live
GoRouter mid-frame), and sentinels must be **stable across 3 consecutive
frames** before capture (overlapping `loadMyCircles` calls briefly clear
`failure` at start, so a one-shot sentinel race can capture a transient frame).

24 PNGs in [screenshots/wave0/](screenshots/wave0/) — 6 states × Arabic RTL /
English LTR × light / dark:

| State | Files | Verified content |
|---|---|---|
| Splash (auth initializing) | `wave0_splash_{ar,en}_{light,dark}.png` | Brand logo + localized spinner semantics, branded background both themes |
| Welcome | `wave0_welcome_{ar,en}_{light,dark}.png` | Logo, tagline, login/register CTAs; RTL verified (ar) |
| Home loaded | `wave0_home_loaded_{ar,en}_{light,dark}.png` | Circle tile, welcome header, 4-tab shell; RTL nav order (الرئيسية rightmost, selected pill) |
| Home empty | `wave0_home_empty_{ar,en}_{light,dark}.png` | Branded empty state with guidance copy |
| Home recoverable error | `wave0_home_error_{ar,en}_{light,dark}.png` | Offline error card with Retry over last safe content (empty state when nothing cached) — banner no longer clears stale/empty context |
| Chats empty | `wave0_chats_empty_{ar,en}_{light,dark}.png` | Dedicated chats empty state (no longer masquerading as a load error), Chats tab selected |

## T012 — UX Designer / UI Designer review record

Review lens applied per role definitions (`.github/agents/ux-designer.agent.md`,
`.github/agents/ui-designer.agent.md`) against the US1 acceptance criteria in
`spec.md` and the Wave 0 scope in `plan.md`. Reviewer of record: implementation
agent self-review (role agents not dispatchable in this session); findings and
resolutions recorded below.

### Flow & navigation (UX)

- Splash → welcome → shell transitions are auth-status driven; the router is
  memoized per auth status, so locale/theme changes no longer rebuild the
  router or reset navigation state. **Pass.**
- Deferred Home quick actions (discover / invite / see-all) remain reachable
  via the Circles tab inside the tap budget — recorded in
  [compatibility-inventory.md](compatibility-inventory.md); no Wave 0 action.
  **Accepted deferral (Wave 2 owns the Home surface).**
- All four "notice" actions (under-implementation snackbar) are classified for
  Wave 4; Wave 0 introduces zero new dead ends. **Pass.**

### States & messaging (UX)

- Home: recoverable failure now renders the error card **above** stale content
  instead of replacing it; empty state only after a successful empty load.
  Chats: error and empty are distinct states (previously conflated). **Pass**
  (screenshots `home_error_*`, `chats_empty_*`).
- Loading states use the branded `HalaqatyLoading` with localized semantics
  labels on splash, Home, and Chats. **Pass.**

### Direction & localization (UX)

- Root `Directionality` is driven by `AppLocaleController`
  (normalize → ar/en, else ar; platform fallback restored on logout; profile
  language applied on load/save). Arabic screenshots show correct RTL
  layout, navigation order, and chevron direction. **Pass.**

### Tokens, themes, targets (UI)

- Text theme: `Typography.material2021` base, titles/labels w600, body w400,
  uniform 1.5 line height for Arabic legibility; light/dark mapping asserted
  in tests and visible in the screenshot matrix. **Pass.**
- Navigation destinations and welcome CTAs meet the 48dp minimum touch target;
  selected nav state uses the icon-shape pill in both themes. **Pass**
  (widget tests + screenshots).
- Snackbar helper uses floating behavior with an explicit Dismiss action.
  **Pass.**

### Semantics / accessibility

- Branded loading indicators carry localized `semanticsLabel`; splash spinner
  is announced while auth initializes. **Pass** for Wave 0 scope; full
  semantics sweep is scheduled with the per-surface waves (Wave 1–4).

### Outcome

No blocking findings. One accepted deferral (Home quick actions → Wave 2) and
one scoping note (full semantics sweep in later waves) — both already tracked
in the compatibility inventory and wave plans. Wave 0 is approved to close.
