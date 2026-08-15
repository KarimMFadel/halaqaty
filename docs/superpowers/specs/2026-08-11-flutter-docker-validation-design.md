# Flutter Docker Validation Design

## Goal

Provide one repeatable local command that validates Flutter changes before they are committed or pushed, even when Flutter is not installed on the host.

## Interface

From the repository root, contributors run:

```sh
make flutter-docker-check
```

The root Makefile delegates to the mobile Makefile. The mobile target runs, in order:

1. `flutter pub get`
2. `flutter analyze`
3. `flutter test test`

The command stops on the first failure and returns a non-zero exit status.

## Docker Runtime

Use only `gmeligio/flutter-web:3.44.9`. It matches the Flutter 3.44.9 version used by GitHub Actions, is smaller than the previously tried `ghcr.io/cirruslabs/flutter:stable` image, and has been verified against this repository's unit and widget tests.

Mount `mobile/` at `/app`, set `/app` as the working directory, and reuse the named volume `halaqaty-flutter-pub-cache` at `/home/flutter/.pub-cache`. Run the container with `--rm`, so the stopped container is removed while the image and dependency cache remain available for later checks.

The floating `ghcr.io/cirruslabs/flutter:stable` image is not part of this workflow. It currently contains Flutter 3.44.0 and is not needed. Removing that local image is a separate, explicitly authorized cleanup action.

## Documentation

- `DEVELOPMENT.md` explains prerequisites, the Make target, the equivalent manual Docker command, why this image is pinned, and why no container remains in `docker ps` after completion.
- `AGENTS.md` tells coding agents to use the Docker check before committing or pushing Flutter changes when a matching host Flutter SDK is unavailable.
- The root and mobile Makefile help output expose the new targets.

## Failure Prevention and Troubleshooting

The contributor documentation records the failure modes discovered while stabilizing the Flutter checks and the rule that prevents each one:

| Failure mode | Prevention rule |
|---|---|
| Flutter is not installed or is missing from the Windows `PATH`. | Use `make flutter-docker-check`; do not depend on a host Flutter installation. |
| A floating Docker tag uses a different SDK than CI. | Pin `gmeligio/flutter-web:3.44.9`, matching the workflow SDK version. |
| Docker Desktop shows an image but `docker ps` is empty. | Explain that images are templates, `docker ps` shows running containers, and this check uses `--rm` to remove its temporary container after completion. |
| The image name is mistyped or the wrong shell resets its `PATH`. | Keep the exact image name in the Makefile and invoke its command with `sh -c`. |
| `flutter analyze` fails on warnings or deprecated APIs. | Run analysis before tests and treat every analyzer issue as a failing gate. |
| Widget taps miss controls below the viewport because a focused field scrolls back into view. | In widget tests, unfocus the field, move the outer form scroll position, settle, and then tap the target. |
| A test asserts a transient snackbar or platform-channel side effect unrelated to its stated behavior. | Assert durable UI state relevant to the test name; test clipboard/platform behavior separately with an intentional platform mock when required. |
| Multiple integration scenarios leak controller or provider state. | Give each scenario a fresh widget/controller/provider scope and assert typed state plus stable widget keys. |
| Running every Linux integration file in one Flutter process causes the debug log reader to stop. | Run each `integration_test/*_test.dart` file in its own `xvfb-run` and Flutter process while aggregating failures. |
| Integration tests reference moved files or undefined role types. | Run `flutter analyze` before launching integration tests so import and type errors fail quickly. |

These rules guide diagnosis but do not weaken assertions or turn genuine product regressions into ignored test failures.

## Scope

This workflow validates static analysis plus Flutter unit and widget tests. Linux desktop integration tests remain a separate CI workflow because the selected image is not the project's Linux desktop integration-test environment.

## Verification

Verify the target by running `make flutter-docker-check` from the repository root and confirming:

- Flutter reports no analyzer issues.
- All files under `mobile/test/` pass.
- The command exits with status 0.
- `git diff --check` reports no whitespace errors.
