---
name: test-guard
description: "Review generated or changed test code against universal testing rules before it ships. Adapted for Halaqaty's Go backend (go test, testify, gomock) and Flutter mobile (flutter_test, mocktail). Best used reactively after an agent writes, edits, generates, or refactors tests, before presenting, committing, or merging them. Use for Go (*_test.go), Dart (*_test.dart), files under tests/ or integration_test/, and review requests like 'write tests for X', 'add tests', 'test this', 'review these tests', or PR diffs containing tests. This skill is the quality gate that prevents AI-generated test bloat."
---

# Test Guard

You are reviewing generated or changed test code before it ships. Enforce the rules below after the first test-writing pass and before the tests are presented, committed, or merged. Be a sharp reviewer, not a pedantic one: flag what wastes maintenance effort or hides real bugs; ignore cosmetic preferences.

These rules exist because coding agents over-generate tests. The common failure modes: mock-heavy tests that assert implementation details, near-duplicate test bodies differing by one value, and tests that re-verify the framework instead of the project's logic. Each looks productive in a diff and costs maintenance forever.

## When this skill activates

- A coding agent has just written new test functions or test files in Go or Dart
- You are editing existing tests
- You are reviewing a diff that contains test changes
- The user asks you to write, add, or review tests

**Go activation patterns**: files matching `*_test.go`, packages ending in `_test`, or files tagged `//go:build integration`
**Dart/Flutter activation patterns**: files matching `*_test.dart`, directories named `test/`, `integration_test/`

## Adapt to the project first

Before reviewing:

1. Read `.specify/memory/constitution.md` — especially §VI (Test-First Development) which mandates:
   - Unit tests written alongside implementation (red-green-refactor)
   - Integration tests required for every API endpoint and every WebSocket event handler
   - Go coverage target: **≥80% for `backend/internal/` packages**
   - Every DB migration tested against a fresh schema
2. Check `backend/` for existing test helpers, fixtures, and test database setup patterns before inventing new ones.
3. Identify the language, then read the matching reference for concrete patterns:
   - **Go** → [references/go.md](references/go.md) — table-driven tests, testify, gomock, dockertest/testcontainers, httptest
   - **Dart / Flutter** → [references/flutter.md](references/flutter.md) — flutter_test, mocktail, widget testing, Bloc/Riverpod testing
4. Map the project's system boundaries: PostgreSQL (`pgx`), Firebase Auth, LiveKit (server SDK), MinIO, FCM. Existing test helpers reveal where the project already draws these lines.

## What to do

1. Read the test code: the diff, the new file, or the section being modified.
2. Check each test against the nine rules below.
3. Report violations concisely: rule number, location, why it violates, suggested fix.
4. If explicitly invoked before test writing, apply the rules while writing — don't write violations and then flag them.

When writing new tests, ask for each: "What specific bug does this catch that no other test in this suite catches?" If you can't answer clearly, don't write it.

## The Nine Rules

### Rule 1: Test behavior, not implementation

Test what code does from the caller's perspective. Assert return values and observable side effects. Never assert that an internal helper was called with specific arguments — that test breaks on every refactor while catching nothing.

**Go violation pattern**: asserting that a mock repository method was called with exact arguments when the test subject is a service, not the repository boundary.
**Go fix**: assert the value returned by the service, or the side effect visible to the caller.

**Dart violation pattern**: asserting that a private method or internal Bloc event was fired as an implementation detail.
**Dart fix**: assert the emitted state or the value returned by the use-case.

### Rule 2: Every mock must be justified

Mock only at system boundaries: **PostgreSQL** (`pgx` pool/connection), **Firebase Auth** SDK calls, **LiveKit** server SDK, **MinIO** S3 client, **FCM** push sender, **external HTTP** calls, **filesystem I/O**, **clock and randomness**.

Never mock internal packages, domain service structs, or Dart model classes to isolate a "unit" — the seams you create hide the integration bugs worth catching.

When you mock a boundary, assert what the caller **does with the response**, not that the mock received specific arguments.

**Go**: use `gomock`-generated mocks for interface boundaries. Never mock a concrete `struct` directly; define a minimal interface instead.
**Dart**: use `mocktail` (preferred over `mockito` for null-safe Dart). Mock only external SDK classes, never domain model classes or `freezed` data classes.

### Rule 3: One scenario per test, data-driven for variants

If two or more tests share identical setup and differ only in input/output values, merge them into a data-driven test:
- **Go**: `t.Run` with a table slice — see [references/go.md](references/go.md) for the canonical pattern
- **Dart**: parameterized tests are less native; use `for` loops over test-case tables inside a single `group()`, or the `test_api` package

**When separate tests are correct**: different setup, different assertions, different mock configurations, or genuinely different scenarios that happen to exercise the same function.

### Rule 4: Every test must justify its existence

Ask: "What bug does this catch that no other test catches?" Delete tests that only verify:
- A Go struct sets its fields from a constructor (the type system guarantees this)
- A `context.Context` is passed through (it will panic if nil in production)
- HTTP status codes returned by the framework (not your logic)
- That a Dart `freezed` data class copies fields correctly

**Exception — Halaqaty domain invariants are always justified**:
- Ayah numbers within Surah bounds
- Queue position ordering and single-position-per-student-per-round rules
- LiveKit token `CanPublish` scope enforcement
- `circle_members` role checks (teacher/supervisor/student)

### Rule 5: Name tests for the scenario

**Go pattern**: `TestSubject_Scenario_ExpectedOutcome` or descriptive `t.Run("scenario / expected outcome", ...)` sub-test names.
**Dart pattern**: `test('subject: scenario returns expected outcome', ...)` inside a descriptive `group('SubjectName', ...)`.

| Bad | Good |
|-----|------|
| `TestCreateCircle` | `TestCreateCircle_DuplicateName_ReturnsConflictError` |
| `TestJoinCircle` | `TestJoinCircle_StudentAlreadyMember_Returns409` |
| `testBuildCircleCard` | `'CircleCard: inactive circle shows archived badge'` |

### Rule 6: Production regression tests are sacred

Tests that reproduce a real production bug are always justified. Reference the incident (date, issue ID, or short description) in the name or a comment, and never delete them. They are exempt from Rule 4.

### Rule 7: No tests for framework guarantees

Don't test that:
- `pgx` commits a transaction you started
- Firebase SDK parses a JWT
- Flutter's `Navigator` pushes a route
- `go-chi` or `net/http` returns 404 for unregistered routes
- `freezed` generates correct `copyWith`

Test *your* logic sitting on top of the framework.

### Rule 8: State and value objects are real, never mocked

Never mock a domain struct, DTO, entity, or Dart data class. Construct a real instance.
- **Go**: create real `Circle`, `User`, `QueueEntry` structs. If setup is painful, add a `testdata` builder package under `backend/internal/testutil/`.
- **Dart**: create real `freezed` data class instances. Never mock a `CircleModel` or `UserProfile`.

Mocking state hides field-name typos and validation errors — exactly the bugs worth catching.

### Rule 9: Infrastructure under test gets real infrastructure

When PostgreSQL queries, schema behavior, or migration correctness **is the subject** of the test, run against a real test database with migrations applied.
- **Go**: use `dockertest` or `testcontainers-go` to spin up a real PostgreSQL instance. Apply `golang-migrate` migrations before the test suite runs. Tag these tests with `//go:build integration` so they don't run in the unit-test pass.
- **Dart**: for integration tests against the real backend, use `integration_test/` with a staging environment. Unit tests mock at the repository boundary.

**Constitutional alignment**: Constitution §VI mandates "every database migration tested against a fresh schema" — Rule 9 is how that gets done.

## Reporting format

```
**Rule N violation** in `path/to/file.go::TestFunctionName`
- What: <one sentence describing the violation>
- Fix: <one sentence describing what to do instead>
```

Group violations by file. If a file has no violations, don't mention it.

## Severity guide

- **Must fix:** Rules 1, 2, 8 — hide real bugs or make tests brittle
- **Should fix:** Rules 3, 4, 5, 7 — cause bloat and maintenance drag
- **Sacred:** Rule 6 — never delete, always allow
- **Worth noting:** Rule 9 — test architecture; flag it, but don't block small changes on it

## References

- [references/go.md](references/go.md) — Go/testify/gomock patterns: table-driven tests, mock boundaries, real pgx setup, httptest, integration build tags
- [references/flutter.md](references/flutter.md) — Dart/Flutter patterns: flutter_test, mocktail, WidgetTester, Bloc/Riverpod testing, integration tests

## What this skill does NOT do

- Run tests. Use `make test` (unit) or `make test-integration` (integration with `DATABASE_URL`). For Flutter: `make test` from root or `flutter test test` from `mobile/`.
- Enforce code style — that is `make lint` and `dart format`.
- Decide *what* to test — only *how* to test it.
- Flag pre-existing violations in files you are not touching, unless asked to audit.
