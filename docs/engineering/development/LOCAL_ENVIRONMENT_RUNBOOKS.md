# Local Environment Runbooks (on-demand recipes)

Step-by-step commands for environments where SDKs are not installed locally.
This file is read on demand — it is deliberately NOT part of the always-injected
agent instructions (token budget; see `AGENT_WORKFLOW_HARNESS.md` §6A).

Read the section you need before running the command for the first time in a
session.

## Flutter unit/widget tests, analyze, format (Docker fallback)

This machine has no local Flutter/Dart/Node. Karim pre-authorized the Docker
fallback — use it without asking:

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

## Flutter integration tests (Linux scaffold + xvfb)

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
