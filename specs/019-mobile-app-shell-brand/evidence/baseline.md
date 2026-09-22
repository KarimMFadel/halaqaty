# F-019 Baseline Evidence (Phase 1: T001–T003)

**Date**: 2026-09-22 | **Branch**: `019-mobile-app-shell-brand` | **Flutter**: local SDK 3.47.4 (`C:\myPersonalData\programs\flutterSDK\flutter`)

## T001 — Branch and preflight

- Current branch confirmed: `019-mobile-app-shell-brand` (no phase-specific branch created).
- Unrelated untracked file `docs/spec-kit-copy-paste-prompts.md` is left untouched and untracked; it is excluded from every stage/commit of this feature.
- Approved F-019 artifacts re-read before production edits: `spec.md`, `plan.md`, `tasks.md`, `contracts/compatibility.md`, plus constitution, `DEVELOPMENT.md`, and `AGENT_WORKFLOW_HARNESS.md`.
- Note: Flutter commands run from Git Bash require the repo-local `run-flutter.bat` wrapper because this shell lacks `%PROGRAMFILES(X86)%` (raw `flutter.bat` exits with "%PROGRAMFILES(X86)% environment variable not found"). Analyzer/formatter ran natively; the test suite ran through the wrapper.

## T002 — Unit/widget baseline (from `mobile/`)

| Gate | Command | Result | Log |
|---|---|---|---|
| Dependencies | `flutter pub get` | OK | `log-pub-get.txt` |
| Unit/widget tests | `flutter test test` | **PASS — 341/341** (00:45, exit 0, "All tests passed!") | `log-test-baseline.txt` |
| Analyzer | `flutter analyze` | **PASS — No issues found! (23.8s)** | `log-analyze-baseline.txt` |
| Formatter | `dart format --set-exit-if-changed .` | **PASS — 137 files, 0 changed** | `log-format-baseline.txt` |

No pre-existing failures. Baseline is fully green.

## T003 — Integration prerequisites (`flutter test integration_test/`)

| Prerequisite | Status | Evidence |
|---|---|---|
| Flutter SDK 3.47.4 | ✅ available | baseline run above |
| Emulator/device | ✅ `emulator-5554` connected | `adb devices` (Android SDK at `C:\Users\karim.fadel\AppData\Local\Android\Sdk`) |
| Backend service | ✅ reachable | `GET http://localhost:8080/health` → 200 |
| PostgreSQL | ✅ port open | `localhost:5432` accepts connections (curl exit 52 = empty reply, port listening) |
| Docker CLI | ⚠️ not on this shell's PATH | `docker` not recognized in Git Bash or `cmd`; backend already running so not needed for the suite |
| Firebase/LiveKit credential files | ⚠️ not present at conventional paths (`backend/.env`, `firebase-service-account.json` absent) | live-backend journeys depending on real credentials would be blocked; the existing `integration_test/` suite boots with in-app fakes/stubs (e.g. `_IntegrationAuthNotifier`, `_VisualCircleApi`) and does not reference a live backend URL |

Conclusion: emulator and backend prerequisites for the stub-backed `integration_test/` suite are **available**. No integration result is claimed here; availability is recorded only. Real-credential integration runs remain contingent on the Firebase/LiveKit setup guide and are not treated as passing.
