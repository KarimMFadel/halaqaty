# Halaqaty UI/UX Wave 0–1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Finish the calm-contemporary visual foundation and modernize Home/Circles with clear hierarchy, soft-gold active navigation, complete states, and RTL/LTR evidence.

**Architecture:** Reuse the existing Material 3 theme, go_router shell, Riverpod controllers, and shared Halaqaty components. Keep this client-only: no backend endpoints, tables, roles, providers, or new dependencies.

**Tech Stack:** Flutter/Dart, Material 3, Riverpod, go_router, flutter_svg, widget tests, emulator screenshot journey.

**Spec:** docs/engineering/design/UI_UX_GOVERNANCE.md

## Global Constraints

- Calm Contemporary: warm ivory surfaces, restrained emerald primary actions, soft-gold selected navigation state.
- Arabic is RTL-first; English is the mirrored LTR experience.
- Core actions target two to three taps from the app shell.
- Affected screens define loading, empty, error, success, and offline behavior where the underlying flow can enter that state.
- Use Theme.of(context).colorScheme and shared components; no screen-level hex colors or arbitrary fonts.
- Preserve existing widget-test keys and protected-route behavior.
- Do not add backend APIs, database schema, provider controls, or Flutter dependencies.
- Every Flutter change requires fresh flutter test test, flutter analyze, and dart format --set-exit-if-changed .; screenshots are evidence, not a passing test gate.

## Review Focus

1. Long Arabic or fixture-derived circle names remain readable without overflow or developer identifiers — Task 3 test.
2. Failed circle requests show localized recovery with retry — Task 3 test.
3. RTL directional affordances mirror correctly — Task 4 test.
4. Switching tabs after circle detail returns Home to its route root and keeps the gold indicator — Task 2 test.
5. Loading, empty, and populated states do not overlap or clip at phone widths — Tasks 3 and 5.

---

### Task 1: Baseline and worktree reconciliation

**Files:** Read the approved spec; mobile/lib/core/theme/halaqaty_theme.dart; mobile/lib/app/router.dart; mobile/lib/app/home_screen.dart; mobile/lib/features/circles/presentation/circle_discovery_screen.dart; mobile/lib/features/circles/presentation/circle_detail_screen.dart; mobile/test/widget_test.dart.

**Interfaces:** Consume existing dirty worktree changes and produce a recorded baseline; do not reset or stage unrelated files.

- [ ] Confirm branch, status, and diff scope with git branch --show-current, git status --short, and git diff --stat. Expected branch: 019-mobile-app-shell-brand.
- [ ] Run the focused baseline from mobile/: flutter test test/widget_test.dart test/widget/circles test/widget/profile. Record pass/fail output.
- [ ] Confirm source already uses scheme.secondaryContainer for the selected navigation indicator, the four-tab shell, and identifiable circle state keys. Do not add a second theme or navigation abstraction.
- [ ] Leave the baseline unchanged unless a documentation correction is required.

### Task 2: Complete the app-shell visual foundation

**Files:** Modify mobile/lib/core/theme/halaqaty_theme.dart and mobile/lib/app/router.dart. Test mobile/test/widget_test.dart.

**Interfaces:** Consume ColorScheme.secondaryContainer/onSecondaryContainer and existing NavigationBar. Produce soft-gold selected state with accessible selected semantics without changing paths.

- [ ] Add a failing authenticated-shell assertion: tap Circles, keep appNavigationBar, and assert selected destination semantics.
- [ ] Run from mobile/: flutter test test/widget_test.dart -r expanded. Confirm the new assertion fails if the state is absent.
- [ ] Use the existing NavigationBarThemeData: indicatorColor is scheme.secondaryContainer and selected icon/label content uses scheme.onSecondaryContainer. Do not add hex literals or a new widget.
- [ ] Re-run flutter test test/widget_test.dart -r expanded and confirm shell, tab switching, route reset, and selected-state assertions pass.
- [ ] Format the three changed Dart/test files with dart format.

### Task 3: Finish Home and Circles hierarchy and states

**Files:** Modify mobile/lib/app/home_screen.dart, mobile/lib/features/circles/presentation/circle_discovery_screen.dart, mobile/lib/features/circles/presentation/circle_load_error.dart, and mobile/lib/features/circles/presentation/circle_name_text.dart. Test mobile/test/widget_test.dart and mobile/test/widget/circles/discover_join_screen_test.dart.

**Interfaces:** Consume CircleDiscoveryController, CircleSummary, CircleNameText, CircleLoadError, and existing API overrides. Produce human-readable content with recoverable loading/empty/error/populated states and invite/search actions.

- [ ] Add failing tests for a localized name (for example حلقة الإتقان), a retry key circleLoadRetry, and a long name that does not produce a RenderFlex overflow.
- [ ] Run flutter test test/widget_test.dart test/widget/circles/discover_join_screen_test.dart -r expanded from mobile/ and confirm failure.
- [ ] Keep existing controllers/API calls. Use shared section/card patterns, one prominent action per state, localized copy, semantic labels, and bounded Text behavior. Do not expose raw test IDs as headings.
- [ ] Cover loading, empty, populated, and failure branches with the existing test API override. Empty explains discovery/invite; failure exposes retry; loading does not claim success.
- [ ] Run the focused tests and dart format on the changed production/test files.

### Task 4: Align circle detail actions and directionality

**Files:** Modify mobile/lib/features/circles/presentation/circle_detail_screen.dart. Test mobile/test/widget/circles/circle_detail_screen_test.dart.

**Interfaces:** Consume the existing detail/controller flow and Directionality. Produce clear action hierarchy, one dominant join/open action, and mirrored directional icons without changing role authorization.

- [ ] Add failing RTL and LTR renders. Assert localized title/action labels and correct back/chevron direction through stable keys or widget predicates.
- [ ] Run flutter test test/widget/circles/circle_detail_screen_test.dart -r expanded and confirm failure.
- [ ] Use existing theme roles/components; keep role-restricted actions intact; do not add backend data; keep interactive rows at least 48dp with semantics.
- [ ] Re-run the detail test in both directions and format changed files.

### Task 5: Visual journey evidence and mobile gates

**Files:** Modify mobile/integration_test/ux_visual_journey_test.dart. Modify docs/engineering/development/LOCAL_ENVIRONMENT_RUNBOOKS.md only if its command/evidence description is inaccurate.

**Interfaces:** Consume Home, Circles, Circle Details, Profile, theme, and test doubles. Produce RTL/LTR screenshot evidence with semantic assertions beside capture.

- [ ] Confirm the journey captures Home, Circles, Profile, and recoverable circle-load error in both directions.
- [ ] Run from mobile/: flutter test integration_test/ux_visual_journey_test.dart -d emulator-5554. If unavailable, report blocked/unverified.
- [ ] Run from mobile/: flutter test test; flutter analyze; dart format --set-exit-if-changed . and record each exit status.
- [ ] Review screenshots for soft-gold selected navigation, emerald primary action, warm ivory surface, mirrored RTL, no clipped names/diacritics, readable states, and no raw fixture identifiers.
- [ ] Stage only Task 2–5 files and commit with git commit -m "feat: modernize circles journey UI". Do not stage existing unrelated artifacts or backend changes.

## Plan self-review

- Spec coverage: visual tokens/gold navigation Task 2; states/copy Task 3; RTL/action hierarchy Task 4; screenshot and gates Task 5; scope limits are global constraints.
- Placeholder scan: no TBD, TODO, or unspecified implementation step.
- Type consistency: tasks reuse CircleDiscoveryController, CircleSummary, CircleNameText, CircleLoadError, and existing keys; no new public interface.
- Review focus: all five likely failures have an owning task and test/evidence step.

## Execution handoff

This plan is native-first because the current worktree already contains interdependent foundation and circles changes. After review, execute Tasks 1–5 sequentially with superpowers:executing-plans, keeping unrelated worktree changes unstaged.
