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

## Scope

This workflow validates static analysis plus Flutter unit and widget tests. Linux desktop integration tests remain a separate CI workflow because the selected image is not the project's Linux desktop integration-test environment.

## Verification

Verify the target by running `make flutter-docker-check` from the repository root and confirming:

- Flutter reports no analyzer issues.
- All files under `mobile/test/` pass.
- The command exits with status 0.
- `git diff --check` reports no whitespace errors.
