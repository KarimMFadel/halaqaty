# GitHub Actions test workflows

The executable source of truth is
[`tests-unit-integration.yml`](../../../.github/workflows/tests-unit-integration.yml).
It runs on pull requests, pushes to `main`, and manual dispatch. The workflow
detects the Go and Flutter projects in `backend/` and `mobile/`.

## Flutter integration platform decision

Functional integration tests run on Linux under Xvfb. Screenshot suites run on
an Android emulator because the native `integration_test.takeScreenshot()`
API used by these suites has no Linux implementation. Running them on Linux
builds the application successfully but fails with `MissingPluginException`
for `captureScreenshot`. Repeating that run does not resolve the mismatch.

| Job | Target and scope | Output artifact |
| --- | --- | --- |
| `flutter-unit-tests` | `flutter test test` on Ubuntu | Console output |
| `flutter-integration-tests` | Each functional integration file on Linux, excluding the four screenshot suites below | `flutter-integration-linux-logs` |
| `flutter-visual-tests` | One Android matrix entry per screenshot suite | `flutter-visual-<suite>` |

The Android matrix contains these files under `mobile/integration_test/`:

- `ux_visual_journey_test.dart`
- `wave0_shell_visual_test.dart`
- `wave1_circles_visual_test.dart`
- `wave2_sessions_visual_test.dart`

The emulator job uses Ubuntu with KVM, Java 17, Flutter stable, and an API 35
Google APIs x86_64 emulator with the Pixel 2 profile. Each suite has a
45-minute job timeout. Matrix fail-fast is disabled so one failure does not
cancel the remaining suites. Linux and all Android entries must pass;
the split does not remove screenshot tests from CI.

Screenshot tests run through `flutter drive` and the shared
[`ci_screenshot_driver.dart`](../../../mobile/test_driver/ci_screenshot_driver.dart),
which writes PNG files to `mobile/build/visual-artifacts/screenshots/`.
The artifact also contains `test.log`, the integration response JSON when
the driver receives a result, and ADB status, Android logcat, and memory
snapshots taken before emulator cleanup. Both jobs preserve command failures
and attempt artifact upload even after test failure. The Linux job uses
`tee` with `pipefail`; the Android job saves Flutter's exit code before
collecting emulator diagnostics and then returns that same code. An early
build or emulator failure may leave no screenshots or response JSON.

## Test fixtures and limits

The visual suites override authentication and data providers with deterministic
fixtures. CI generates a placeholder `android/app/google-services.json` solely
to satisfy the Android build plugin. It contains no working Firebase credentials
and must not be reused for deployment or real-backend tests. Local developers
should retain their own configured Firebase file.

The Linux job does not provision the real application backend or authenticated
fixtures. Tests requiring those fixtures may report skips. A green workflow
with skips is not evidence that every real-backend journey passed. Follow the
[local integration runbook](../development/LOCAL_ENVIRONMENT_RUNBOOKS.md) for
those gates and the complete requirements in [DEVELOPMENT.md](../../../DEVELOPMENT.md).

Captured screenshots are review evidence, not automatic pixel-baseline
comparisons or UX/UI approval. Review Arabic RTL and English LTR, light and
dark outputs alongside the test assertions. Android results do not verify iOS.

## Running and diagnosing CI

1. Commit and push the workflow and driver changes before starting a run.
   Re-running an older run uses its original revision, not local edits.
2. In GitHub **Actions → Tests (Unit & Integration)**, select the run for the
   updated commit. Inspect both the Linux job and all four visual matrix entries.
3. Download artifacts from the run summary. Open the failing suite's `test.log`
   and any screenshots before deciding whether the failure is setup or UI behavior.

`MissingPluginException` mentioning `captureScreenshot` on Linux means a visual
suite was routed to the wrong job. The `flutter_webrtc: libpulse NOT found`
warning concerns system-audio loopback capture; it is not the cause of this
screenshot exception. Investigate other errors using their actual stack traces.

To reproduce one visual suite locally, run from `mobile/` with a configured
Android build and a connected emulator:

```sh
flutter drive --driver=test_driver/ci_screenshot_driver.dart --target=integration_test/wave0_shell_visual_test.dart -d emulator-5554
```

When adding a native screenshot suite, update both the Linux exclusion list
and the Android matrix in the same change. Confirm every integration file
still has a CI execution path. Workflow syntax checks and static review do not
prove emulator execution; record the first successful updated GitHub run before
claiming the platform fix is verified end to end.
