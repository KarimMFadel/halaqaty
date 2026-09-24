# Local Environment Runbooks (on-demand recipes)

Step-by-step commands for environments where SDKs are not installed locally.
This file is read on demand — it is deliberately NOT part of the always-injected
agent instructions (token budget; see `AGENT_WORKFLOW_HARNESS.md` §6A).

Read the section you need before running the command for the first time in a
session.

## Flutter unit/widget tests, analyze, format (Docker fallback)

A local Flutter SDK exists (`C:\myPersonalData\programs\flutterSDK\flutter`,
3.47.4) with the Android SDK at `C:\Users\karim.fadel\AppData\Local\Android\Sdk`
— prefer it for daily builds, tests, and emulator runs. The Docker fallback
below remains for matching CI's pinned Flutter (3.44.0) exactly:

```powershell
docker run --rm -e FLUTTER_SUPPRESS_ANALYTICS=true -v "<repo-root>:/workspace" -v halaqaty-pub-cache:/root/.pub-cache -w /workspace/mobile ghcr.io/cirruslabs/flutter:stable <cmd>
```

- Image: `ghcr.io/cirruslabs/flutter:stable` (Flutter 3.44.0 / Dart 3.12.0;
  `gmeligio/flutter-web:3.44.9` also present).
- Run `flutter pub get` first. `build_runner` is currently a no-op (no
  `@riverpod` annotations).
- Wrap `<repo-root>` in quotes; keep the `halaqaty-pub-cache` volume so
  packages are not re-fetched per run.
- Token rule: pipe suite output to a file and grep failures — do not stream
  thousands of lines into the agent context.

## Flutter integration tests (current Android emulator first)

Use the existing Android emulator when it is already running and reachable.
This is the most useful local path for validating the real Android renderer,
Firebase/session startup, backend connectivity, touch targets, navigation, and
screenshots. It is not a substitute for the full integration gate: the run
still requires the configured backend and fixtures, and a screenshot review
does not prove every integration-test assertion.

### Select the execution path

Use the Android path only when all of these checks pass:

```powershell
$adb = "C:\Users\<user>\AppData\Local\Android\Sdk\platform-tools\adb.exe"
$appPackage = "com.halaqaty.mobile"

& $adb start-server
& $adb devices -l
& $adb shell getprop sys.boot_completed
& $adb shell pm path $appPackage
```

Expected results are one device in `device` state, `1` for
`sys.boot_completed`, and an installed package path. If ADB, the emulator,
the package, or the backend is unavailable, use the Docker/Xvfb recipe below
for functional integration checks. Screenshot suites require the connected
Android device/emulator and remain unrun until it is available.

For the Halaqaty Android emulator, the default development API URL is
`http://10.0.2.2:8080/api/v1`. Start exactly one temporary API dispatcher and
verify the host API before launching the app:

```powershell
Invoke-WebRequest http://localhost:8080/health -UseBasicParsing
```

If the app was built with another API URL, rebuild or launch it with the
matching `--dart-define=API_BASE_URL=...`. `localhost` inside the Android
emulator refers to the emulator itself, not the Windows host.

### Autonomous Android run, screenshot, and log workflow

Run from the repository root. Keep one output directory per run and never
write Firebase tokens, session IDs, or other fixture secrets into its logs.

```powershell
$adb = "C:\Users\<user>\AppData\Local\Android\Sdk\platform-tools\adb.exe"
$appPackage = "com.halaqaty.mobile"
$runDir = Join-Path (Get-Location) ("artifacts\android-integration-" + (Get-Date -Format "yyyyMMdd-HHmmss"))
New-Item -ItemType Directory -Force $runDir | Out-Null

# Start from a known app state without clearing users, Firebase data, or app data.
& $adb shell am force-stop $appPackage
& $adb shell monkey -p $appPackage 1 | Out-File (Join-Path $runDir "launch.txt")
Start-Sleep -Seconds 3

# Capture the initial screen and inspectable Flutter/UI hierarchy.
& $adb exec-out screencap -p > (Join-Path $runDir "01-start.png")
& $adb shell uiautomator dump /sdcard/window.xml | Out-File (Join-Path $runDir "uiautomator-dump.txt")
& $adb shell cat /sdcard/window.xml | Out-File (Join-Path $runDir "01-start.xml")

# Perform one interaction at a time, then capture the resulting state.
& $adb shell input tap <x> <y>
& $adb exec-out screencap -p > (Join-Path $runDir "02-after-tap.png")
& $adb shell input text "<url-encoded-text>"
& $adb shell input keyevent ENTER
& $adb exec-out screencap -p > (Join-Path $runDir "03-after-submit.png")

# Save app-only diagnostics plus the crash buffer.
$appPid = (& $adb shell pidof $appPackage).Trim()
if ($appPid) {
  & $adb logcat -d --pid=$appPid -v threadtime | Out-File (Join-Path $runDir "app.log")
}
& $adb logcat -d -b crash -v threadtime | Out-File (Join-Path $runDir "crash.log")
```

For a real integration test, run the test file on the connected Android
device and retain the same screenshot/log collection around the journey:

```powershell
Set-Location mobile
flutter test integration_test/<file>.dart -d emulator-5554
# UX screenshot matrix: Home, Circles, Profile, and offline error in RTL/LTR.
flutter test integration_test/ux_visual_journey_test.dart -d emulator-5554
```

The UX matrix uses deterministic fixtures and emits named screenshots such as
`ux_home_rtl`, `ux_circles_ltr`, `ux_profile_rtl`, and `ux_error_ltr` through
the integration-test screenshot channel. Keep the emulator/device run as the
visual evidence source; the test must still pass its widget assertions.

When the test fails, classify the evidence before changing code:

- `DioExceptionType.connectionError` with no HTTP status: backend address,
  host binding, emulator routing, firewall, or API process problem.
- HTTP `401`/`403`: Firebase/session fixture or authorization problem, not an
  emulator transport failure.
- `FATAL EXCEPTION`, `AndroidRuntime`, or a non-empty crash buffer: Android
  process crash; preserve the complete app and crash logs.
- A screenshot-only visual defect with no app error: UI/UX regression; attach
  the screenshot and the exact interaction sequence.
- Repeated system messages such as Wi-Fi, satellite, emulator GPU, or
  `system_server` diagnostics: record them separately unless they correlate
  with the app failure.

Do not use coordinate taps as a permanent test assertion when a semantic
Flutter finder or integration-test action can express the behavior. Use
coordinates only for exploratory black-box checks or when validating the
actual touch target. Review screenshots in both expected RTL/LTR states where
the tested flow supports them.

## Flutter integration tests (Linux scaffold + xvfb)

This path covers functional integration tests only. The workflow intentionally
excludes the UX journey and Wave 0–2 screenshot suites; run those locally on the
connected Android device/emulator using the existing device instructions above
and screenshot drivers. GitHub Actions does not start an emulator for them.
See [GitHub Actions test workflows](../deployment/GITHUB_ACTIONS.md) for the
suite list and exact local commands.

Use this path when no healthy Android emulator is available, when the test
needs a deterministic Linux runner, or when matching the CI Flutter image is
more important than Android rendering. It validates integration behavior but
does not validate Android-specific rendering, Android back navigation, or
emulator network routing.

1. Use the locally built image `halaqaty-flutter-ci:local`
   (cirruslabs/flutter:stable + `clang cmake ninja-build pkg-config
   libgtk-3-dev libsecret-1-dev xvfb`; rebuild via the Dockerfile in this
   repo's CI docs if absent).
2. Copy `mobile/` into a temporary directory outside the checkout. In that
   copy, run `flutter create --platforms=linux --project-name halaqaty_mobile .`.
   Restore any overwritten application/configuration files from the checkout
   (including `lib/main.dart`, `analysis_options.yaml`, and existing tests).
   Keep the generated Linux scaffold only in the temporary copy; never delete
   or replace the checkout's application files to prepare this gate.
3. Run each file sequentially, up to 3 retries each (batch launches flake on
   the debug connection):
   `xvfb-run -a flutter test integration_test/<file> -d linux`

Real-stack fixtures must be configured before the run; a skipped fixture is
not a passing gate. `chat_direct_flow_test.dart` needs four distinct isolated
accounts with `T064_{TEACHER,STUDENT,SUPERVISOR,OPERATOR}_{TOKEN,SESSION,USER_ID}`.
The operator performs role changes without attempting forbidden self-role
changes. `chat_media_flow_test.dart` needs
`T052_{MEMBER,OUTSIDER}_{TOKEN,SESSION,USER_ID}`. Configure `T064_API_BASE_URL`
and `T052_API_BASE_URL` to the API with chat media enabled and a versioned
MinIO bucket. Real queue tests also need their `T048_*` and `T062_*` fixtures
and the API's configured LiveKit service. Use disposable accounts so existing
memberships and rolling upload quotas cannot contaminate the results. Never
print fixture tokens or session identifiers in gate logs.

Run only one API dispatcher against the fixture database. Multiple local API
processes have separate in-memory WebSocket hubs and can consume each other's
outbox events, causing targeted realtime checks to time out despite successful
REST requests. Stop only a verified temporary API process before the gate;
never stop an unrelated service or weaken the realtime assertions.

## Spectral (OpenAPI lint) via Docker

```powershell
docker run --rm -v "<repo-root>:/workspace" -v halaqaty-npm-cache:/root/.npm -w /workspace node:22-alpine npx --yes @stoplight/spectral-cli lint docs/contracts/openapi.yaml --ruleset .spectral.yaml
```

## Focused / single-package test examples

Focused targets are for fast iteration only — they apply `-run` filters and
are NOT gates. Gates run unfiltered (`make test` / `test-contract` /
`test-integration` / `coverage`).

```powershell
# Backend: one package
cd backend; go test -short ./internal/auth
# Backend: single test by name
cd backend; go test -run TestName ./internal/auth
# Backend: feature-001 scoped dev subsets (not gates; needs DATABASE_URL for integration)
cd backend; make test-feature-001-unit
cd backend; make test-feature-001-contract
cd backend; make test-feature-001-integration

# Mobile: one widget test dir or single file
cd mobile; flutter test test/widget/auth
cd mobile; flutter test test/widget/auth/some_test.dart
```

Contract tests run with `-tags=contract`; integration tests use
`-tags=integration`. Feature-001 targets add a `-run` regex filter
(`Auth|Profile|CircleAssignRole|ResponseSafety` /
`AuthFlow|ProfileFlow|CircleRoleAccess|RateLimitPolicy|PasswordStorageSafety`)
— never mirror filtered subsets as gates or use them as coverage evidence.
