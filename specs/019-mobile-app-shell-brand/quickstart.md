# Quickstart: F-019 Verification

## Preconditions

- Branch is `019-mobile-app-shell-brand`.
- F-019 spec, plan, tasks, and analysis are approved before production edits.
- Flutter SDK and Android/iOS test target are available.
- Integration backend is configured per the local runbook.

## Per-wave loop

1. Select the next incomplete wave batch from `tasks.md`.
2. Add/update the smallest widget/integration test for the state, key, semantics,
   direction, tap path, under-implementation side-effect boundary, or other
   compatibility rule; confirm the intended failure.
3. Make the minimum presentation change using existing controllers, routes,
   native M3, theme roles, and feature widgets.
4. Run focused tests for the changed screen.
5. Capture the wave's required screenshots.
6. Obtain UX Designer and UI Designer review.
7. Run the affected suite and mark tasks complete only with fresh evidence.

For every action introduced from the approved design, classify it in the
compatibility inventory as implemented, intentionally under implementation, or
unplanned/omitted. The middle category must show the exact localized shared
notice after one tap and must make no navigation, controller/API call, or state
mutation.

## Final gates

From `mobile/`:

```powershell
flutter test test
flutter test integration_test/
flutter analyze
dart format --set-exit-if-changed .
```

From repository root:

```powershell
git diff --check
```

The integration suite requires a connected emulator/device and configured
backend. If unavailable, record the gate as blocked and stop before committing
Flutter changes.

## Review matrix

For every changed primary screen, record Arabic RTL and English LTR in light and
dark themes. Wave 5 also records compact phone/tablet widths, increased text
scale, semantics/focus findings, and representative required states.
