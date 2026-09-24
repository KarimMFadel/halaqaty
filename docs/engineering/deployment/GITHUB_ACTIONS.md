# GitHub Actions test workflows

The executable source of truth is
[`tests-unit-integration.yml`](../../../.github/workflows/tests-unit-integration.yml).
It runs on pull requests, pushes to `main`, and manual dispatch. The workflow
detects the Go and Flutter projects in `backend/` and `mobile/`.

## Flutter integration checks

Functional integration tests run on Linux under Xvfb. Screenshot suites use
`integration_test.takeScreenshot()` and run locally on Karim's connected
Android device/emulator. GitHub Actions intentionally does not start an
Android emulator or run those visual suites. The screenshot API is unsupported
on the Linux desktop target and would fail with `MissingPluginException` for
`captureScreenshot`.

| Job | Target and scope | Output artifact |
| --- | --- | --- |
| `flutter-unit-tests` | `flutter test test` on Ubuntu | Console output |
| `flutter-integration-tests` | Functional integration files on Linux; four native screenshot suites are excluded | `flutter-integration-linux-logs` |

Run these visual suites locally on the configured Android device/emulator:

- `ux_visual_journey_test.dart`
- `wave0_shell_visual_test.dart`
- `wave1_circles_visual_test.dart`
- `wave2_sessions_visual_test.dart`

The Wave 0–2 screenshot drivers write PNGs under
`specs/019-mobile-app-shell-brand/evidence/screenshots/` when run with the
commands below. The UX journey uses the device integration-test command in the
[local runbook](../development/LOCAL_ENVIRONMENT_RUNBOOKS.md). These local
runs remain the visual evidence gate; CI reports do not imply that they ran.

## Test fixtures and limits

The Linux job does not provision the real application backend or authenticated
fixtures. Tests requiring those fixtures may report skips. A green workflow
with skips is not evidence that every real-backend journey passed. Follow the
[local integration runbook](../development/LOCAL_ENVIRONMENT_RUNBOOKS.md) for
those gates and the complete requirements in [DEVELOPMENT.md](../../../DEVELOPMENT.md).

Captured screenshots are review evidence, not automatic pixel-baseline
comparisons or UX/UI approval. Review Arabic RTL and English LTR, light and
dark outputs alongside the test assertions.

## Running and diagnosing CI

1. Commit and push workflow changes before starting a run. Re-running an older
   run uses its original revision, not local edits.
2. In GitHub **Actions → Tests (Unit & Integration)**, select the run for the
   updated commit. Inspect the Linux integration logs artifact if that job fails.
3. Run the four visual suites locally on Android and review their screenshots;
   their success is not represented in the GitHub workflow status.

The `flutter_webrtc: libpulse NOT found` warning concerns system-audio loopback
capture; it does not fail the Linux integration tests. If a screenshot suite
appears in the Linux job, the exclusion list and test routing have drifted.

To capture Wave 0–2 screenshots locally, run from `mobile/` with the existing
drivers and a configured Android device/emulator:

```sh
flutter drive --driver=test_driver/wave0_screenshot_driver.dart --target=integration_test/wave0_shell_visual_test.dart -d emulator-5554
flutter drive --driver=test_driver/wave1_screenshot_driver.dart --target=integration_test/wave1_circles_visual_test.dart -d emulator-5554
flutter drive --driver=test_driver/wave2_screenshot_driver.dart --target=integration_test/wave2_sessions_visual_test.dart -d emulator-5554
```

When adding a native screenshot suite, add it to the Linux exclusion list and
document its local Android command. Confirm every functional integration file
still has a CI execution path.
