# Wave 1 Evidence — Circles (US2)

Date: 2026-09-22 · Branch: `019-mobile-app-shell-brand` · Scope: T013–T019

## T018 — Circle verification

Fresh results after the final RTL fix and formatting:

| Gate | Command | Result |
|---|---|---|
| Circle widget tests | `flutter test test/widget/circles` | **51/51 passed** |
| Circle device journeys | `flutter test integration_test/circle_join_flow_test.dart integration_test/circle_retirement_flow_test.dart integration_test/circle_role_invite_flow_test.dart -d emulator-5554` | **5/5 passed** |
| Wave 1 visual harness | `flutter drive --no-dds --driver=test_driver/wave1_screenshot_driver.dart --target=integration_test/wave1_circles_visual_test.dart -d emulator-5554` | **3/3 passed**; 32 PNGs captured |
| Full unit/widget suite | `flutter test test` | **372/372 passed** |
| Static analysis | `flutter analyze` | **No issues found** |
| Format | `dart format --output=none --set-exit-if-changed .` | **0 changed** |
| Diff hygiene | `git diff --check` | clean |

The Windows Java selector failed with `Unable to establish loopback connection`
when the default long temporary path was used. The device commands above ran
with process-local `TEMP` and `TMP` set to `C:\jtmp`; no repository or machine
configuration was changed.

The unfiltered `flutter test integration_test/ -d emulator-5554` commit gate
was also attempted. It passed the auth journey, then skipped
`chat_direct_flow_test.dart` because its isolated `T064_*` Firebase/backend
fixture credentials are unavailable. The unrelated full-directory run was
stopped at that point and is recorded as **unverified**, not passing. The three
applicable Wave 1 circle journeys above ran without skips and passed.

## T018 — RTL/LTR × light/dark screenshot matrix

Captured on `emulator-5554` at 1080×2400 using fresh provider stubs for every
state. The harness detaches the preceding widget tree before each state and
waits for its sentinel to remain visible across three consecutive frames.

32 PNGs are stored under [screenshots/wave1/](screenshots/wave1/) — eight
states × Arabic RTL / English LTR × light / dark:

| State | Files | Verified content |
|---|---|---|
| Discovery loaded | `wave1_discovery_loaded_{ar,en}_{light,dark}.png` | Create is the primary action; invite is secondary; member and public cards remain reachable |
| Discovery empty | `wave1_discovery_empty_{ar,en}_{light,dark}.png` | Genuine localized empty state, distinct from failure |
| Discovery recoverable error | `wave1_discovery_error_{ar,en}_{light,dark}.png` | Retry action and safe connection guidance remain in context |
| Detail — student | `wave1_detail_student_{ar,en}_{light,dark}.png` | Members/chat available; management and archive hidden |
| Detail — manager | `wave1_detail_manager_{ar,en}_{light,dark}.png` | Members/chat/management/archive actions visible |
| Detail — archived | `wave1_detail_archived_{ar,en}_{light,dark}.png` | Read-only banner; members/chat retained; mutation actions hidden |
| Create form | `wave1_create_form_{ar,en}_{light,dark}.png` | Existing fields and disabled-until-valid primary action render without overflow |
| Join form | `wave1_join_form_{ar,en}_{light,dark}.png` | Invite entry and continuation action render without overflow |

All 32 files were checked for clipping, overflow, incorrect directionality,
raw identifiers, placeholder/developer copy, and theme regressions. The final
matrix contains no debug banner or visual-test terminology.

## T019 — UX Designer review

Review lens: `.github/agents/ux-designer.agent.md`, applied against US2 and the
recorded tap budgets. Reviewer of record: implementation-agent self-review;
separate role-agent dispatch was unavailable in this session.

- Create is reachable from Circles in two shell-level taps and remains the
  single primary discovery action. Invite entry stays secondary and within its
  four-tap budget. **Pass.**
- Public join opens the joined circle detail; invite join returns to discovery
  with the joined circle retained in the member list. Consequential success no
  longer depends on a transient snackbar. **Pass.**
- Detail loading is branded; recoverable failures retry; terminal 403/404
  states do not offer a misleading retry; archived circles preserve read-only
  members/chat access and hide mutation controls. **Pass.**
- Student and manager screenshots expose only role-appropriate actions, with no
  new permission behavior or dead ends. **Pass.**

## T019 — UI Designer review

Review lens: `.github/agents/ui-designer.agent.md`, applied to the final 32 PNGs
and changed presentation files.

- Changed circle surfaces use Material components and `ColorScheme` roles; the
  changed production files contain no `Color(...)` or `Colors.*` literals.
  Light/dark hierarchy remains legible. **Pass.**
- Existing native controls preserve the 48dp target gate and semantic labels;
  Arabic/English layouts remain unclipped at the captured device size. **Pass.**
- Review found three blocking evidence/RTL issues: a debug ribbon, developer
  fixture copy, and double-mirrored Arabic chevrons. The harness now suppresses
  the ribbon and uses localized product copy; directional icons now rely on
  Flutter's single automatic RTL mirror. A widget regression was updated and
  the entire matrix was recaptured. **Resolved.**

### Outcome

No open UX or UI findings. Wave 1 is approved to close; the separate global
integration commit gate remains unverified because the unrelated T064 fixture
credentials are unavailable.
