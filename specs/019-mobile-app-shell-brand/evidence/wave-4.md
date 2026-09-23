# Wave 4 Evidence — Authentication and Profile (US5)

Date: 2026-09-23 · Branch: `019-mobile-app-shell-brand` · Scope: T037–T042

## T041 — Auth/profile verification

Fresh results on the final code, after the T042 review fixes (below):

| Gate | Command | Result |
|---|---|---|
| Focused auth/profile | `flutter test test/widget/auth test/widget/profile test/widget_test.dart` | **75/75 passed** |
| Full unit/widget suite | `flutter test test` | **454/454 passed** ([log](log-test-wave4.txt)) |
| Wave 4 visual harness | `flutter test tool/wave4_visual_capture_test.dart` | **2/2 passed**; 48 PNGs captured |
| Static analysis | `flutter analyze` | **No issues found** ([log](log-analyze-wave4.txt)) |
| Format | `dart format --set-exit-if-changed .` | **0 changed** after the recorded format pass ([log](log-format-wave4.txt)) |
| Diff hygiene | `git diff --check` | clean |

TDD red evidence: the new T037 tests failed 8 ways against the old screens
([log](log-test-wave4-red.txt), 47 passed / 8 failed on
`test/widget/auth` + `test/widget_test.dart`), the new T039 tests failed 9
ways against the old profile screen
([log](log-test-wave4-profile-red.txt), 11 passed / 9 failed on
`test/widget/profile`). All were green after T038/T040.

**Integration tests were not run**: `emulator-5554` is shared with the
concurrent Wave 3 agent this session, so no `flutter test integration_test/`
or `flutter drive` runs were attempted. The existing `auth_journey_test.dart`
and `profile_flow_test.dart` expectations were checked by inspection — neither
references the changed presentation (only `LogoutButton`, whose key and
`onLoggedOut` callback are unchanged). This gate stays **unverified**, not
passing; the final-gates phase (T049) owns the device run.

### Screenshot recipe deviation (emulator shared)

Waves 0–2 captured on `emulator-5554` via `flutter drive`. With the emulator
shared, Wave 4 uses `mobile/tool/wave4_visual_capture_test.dart` — a
widget-test harness that rasterizes each state off a `RepaintBoundary`
(inside `tester.runAsync`) with the bundled Poppins/Cairo fonts and the
framework MaterialIcons font loaded via `FontLoader`, at 390dp logical width
(780px captures, pixelRatio 2). The harness lives outside `test/` so it is
not part of the `flutter test test` gate; run it explicitly:
`flutter test tool/wave4_visual_capture_test.dart` from `mobile/`.

## T041 — RTL/LTR × light/dark screenshot matrix

48 PNGs under [screenshots/wave4/](screenshots/wave4/) — twelve states ×
Arabic RTL / English LTR × light / dark:

| State | Files | Verified content |
|---|---|---|
| Welcome | `wave4_welcome_{ar,en}_{light,dark}.png` | Brand mark, tagline, full-width primary Sign in / secondary Register |
| Login ready | `wave4_login_ready_{ar,en}_{light,dark}.png` | Logo, supporting copy, email/password, dominant filled submit |
| Login validation | `wave4_login_validation_{ar,en}_{light,dark}.png` | Inline localized errors; no raw output |
| Login loading | `wave4_login_loading_{ar,en}_{light,dark}.png` | Submit disabled with visible progress; entered email preserved |
| Register ready | `wave4_register_ready_{ar,en}_{light,dark}.png` | All four preserved fields + language dropdown |
| Register validation | `wave4_register_validation_{ar,en}_{light,dark}.png` | Inline localized required-field errors |
| Profile loading | `wave4_profile_loading_{ar,en}_{light,dark}.png` | Branded `HalaqatyLoading` (logo + labelled progress), not a bare spinner |
| Profile ready | `wave4_profile_ready_{ar,en}_{light,dark}.png` | Grouped Profile details / Preferences / Account & support sections; RTL chevrons mirrored |
| Profile saved | `wave4_profile_saved_{ar,en}_{light,dark}.png` | Retained in-context `profileSaveSuccess` banner (gold container pair), logout separated below in the error role |
| Profile save error | `wave4_profile_save_error_{ar,en}_{light,dark}.png` | In-context error (icon + copy, not color alone); edits preserved |
| Profile load error | `wave4_profile_load_error_{ar,en}_{light,dark}.png` | Offline-style explanation with in-context Retry |
| Profile notice | `wave4_profile_notice_{ar,en}_{light,dark}.png` | Shared under-implementation snackbar with exact FR-033 copy after tapping Appearance |

All 48 files were checked for clipping, overflow, incorrect directionality,
raw identifiers, placeholder/developer copy, and theme regressions. The
matrix contains no debug banner or visual-test terminology.

The 320dp/600dp-width and 200%-text captures remain deferred to the Wave 5
audit (T043 owns the extended matrix), as in Wave 2; a 320dp × 200% text
widget regression ("large text on a compact width keeps the form usable")
already passes in `profile_form_test.dart`.

## T042 — UX Designer review

Review lens: `.github/agents/ux-designer.agent.md` against US5, the tap
budgets, and the required state matrix. Reviewer of record: implementation
agent self-review (role agents not dispatchable in this session), as in
Waves 0–1.

- **Tap budgets**: sign-in and registration complete from Welcome in 2
  actions (open form, submit — typing excluded); profile edit/save is
  Profile → Save (2); logout is Profile → Logout (2). Locked by the
  `wave 4 auth tap budgets (US5)` group in `widget_test.dart`. **Pass.**
- **Hierarchy**: one filled primary action per form (asserted); logout is a
  subordinate outlined action in a separate Account & support section, so
  routine edits and the sensitive account action are not mixed (US5).
  **Pass.**
- **States**: branded profile loading; load failure explains and retries in
  context; inline validation in the active language; save failure keeps
  edits and shows an in-context error; save success is retained in context
  (`profileSaveSuccess` survives past the snackbar window, FR-008) and
  announced via a live region (FR-014). **Pass.**
- **Notice-only actions**: Appearance, Notifications, Help & support, and
  Privacy & security each invoke only `showHalaqatyUnderImplementationNotice`
  — verified zero navigation, zero controller calls, zero state mutation, and
  exact EN/AR copy (FR-032/FR-033). Avatar upload and account deletion stay
  omitted (asserted absent). **Pass.**
- **Direction**: registration language choice and profile preferred language
  drive RTL/LTR (FR-029); Arabic copy reviewed in the ar captures; password
  visibility toggles carry localized tooltips/semantics. **Pass.**

## T042 — UI Designer review

Review lens: `.github/agents/ui-designer.agent.md` against the 48 PNGs and
changed files. Verdict: **approve with fixes** — both fixes landed and are
covered by the fresh gates and the recaptured matrix above.

- **UI-W4-1 (theme defect, affects all waves)**: `halaqatyLightTheme()`/
  `halaqatyDarkTheme()` built their text theme from
  `Typography.material2021(platform:)`, whose platform bases hard-code
  `fontFamily: 'Roboto'`. Because the copied styles had a non-null family,
  `ThemeData(fontFamily: 'Poppins')` never won the merge — Latin text
  actually rendered in Roboto, not the approved bundled Poppins (the Wave
  0–2 device captures hid this because the emulator ships Roboto). Fixed by
  appending `.apply(fontFamily: HalaqatyFonts.primary,
  fontFamilyFallback: HalaqatyFonts.fallback)` to the built text theme in
  `halaqaty_theme.dart`; weights/heights (the frozen 400/600 mapping) are
  unchanged and the wave-0 font-mapping tests stay green. The Wave 4 captures
  now render genuine Poppins Latin and Cairo Arabic glyphs.
- **UI-W4-2**: the retained save-success banner first shipped as
  `primaryContainer`/`onPrimaryContainer`, which measures only ~3.2:1 in
  both themes — below the 4.5:1 text floor (FR-012). Switched to the
  Wave 2-amended `secondaryContainer`/`onSecondaryContainer` pair
  (8.16:1 light / 6.58:1 dark). Recaptured.
- **UI-W4-3**: the in-flight submit spinner was `onPrimary`-white on the
  disabled grey button surface (~1.4:1). Switched to `onSurfaceVariant`,
  readable on the disabled treatment in both themes. Recaptured.
- **Tokens**: changed screens contain no `Color(...)`/`Colors.*` literals;
  spacing stays on the 8px scale; 12px banner radius per the card token.
  All interactive targets ≥48dp (theme-enforced for filled/outlined
  buttons; asserted in widget tests). **Pass.**

### Outcome

No open UX or UI findings. The device integration gate and the extended
320dp/600dp/200% captures remain owned by T049 and T043 respectively and
are recorded as deferred, not passing.
