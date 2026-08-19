# String Constants Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Centralize repeated backend protocol/domain strings and Flutter protocol/API strings without introducing a global constants dump or changing unique text.

**Architecture:** Go constants remain package-local unless a value is a cross-package HTTP contract, in which case the existing `platform/httpconst` package is reused. Flutter constants remain feature-local, with session protocol/API keys in the sessions data layer and user-facing copy in existing feature label files. SQL remains in package-level query constants.

**Tech Stack:** Go 1.26, Flutter/Dart, existing `httpconst` package, existing feature-first Flutter layout.

**Spec:** Approved user scope in conversation: extract repeated protocol/domain literals; preserve unique logs, fixtures, UUIDs, SQL query ownership, existing UI label ownership, and generated Spec-Kit artifacts.

## Global Constraints

- Do not create one repository-wide constants file.
- Do not change wire values, route values, error codes, SQL semantics, or user-visible wording.
- Do not edit `specs/005-live-sessions-livekit/tasks.md` manually.
- Preserve unrelated existing worktree changes.
- Run Go tests/lint and Flutter tests/analyze/format for changed stacks.

---

### Task 1: Inventory and backend protocol constants

**Files:**
- Create or modify package-local constants beside the owning Go package, primarily `backend/internal/realtime/`, `backend/internal/sessions/`, `backend/internal/auth/`, `backend/internal/rbac/`, and `backend/cmd/api/routes.go`.
- Reuse `backend/internal/platform/httpconst/` only for values shared across HTTP packages.
- Test: existing package tests; add no test-only constants unless a repeated production contract value is exercised.

**Interfaces:**
- Produces package-local names for repeated WebSocket event/action names, session status/role strings, and route/header/error values.
- Preserves all public string values exactly.

- [ ] **Step 1: Identify repeated production literals**

  Search only non-test Go production files for repeated literals in these categories: route patterns, JSON field names used in multiple functions, WebSocket event/action names, status/role values, HTTP headers, and approved error codes. Leave SQL query bodies, unique messages, log text, fixtures, and IDs unchanged.

- [ ] **Step 2: Add constants at the narrowest owner**

  Put WebSocket values in `backend/internal/realtime`, session lifecycle/role/event values in `backend/internal/sessions`, route patterns in `backend/cmd/api/routes.go`, and shared HTTP contract values in the existing `httpconst` package. Use unexported constants unless another package already needs the value.

- [ ] **Step 3: Replace production uses without changing values**

  Replace only exact repeated literals selected in Step 1. Keep SQL statements as package-level query constants and do not convert every JSON key in a one-off map into a named constant.

- [ ] **Step 4: Run backend verification**

  Run `gofmt -w` on changed Go files, `go test -short ./...` from `backend/`, and `golangci-lint run ./...` from `backend/`. Expected result: exit code 0 with no new diagnostics.

### Task 2: Inventory and Flutter protocol/API constants

**Files:**
- Create feature-local constants files only where an existing file would become cluttered, such as `mobile/lib/features/sessions/data/session_protocol_constants.dart`.
- Modify `mobile/lib/features/sessions/data/session_api_client.dart`, `mobile/lib/features/sessions/data/realtime_session_client.dart`, and other production session data files containing repeated protocol/API literals.
- Preserve existing `mobile/lib/features/sessions/presentation/session_ui_labels.dart` as the owner of user-facing session copy.
- Test: `mobile/test/features/sessions/data/realtime_session_client_test.dart` and relevant existing session tests.

**Interfaces:**
- Produces feature-local Dart constants for repeated REST paths, JSON keys, WebSocket message types/actions, and protocol headers.
- Keeps all serialized values and UI wording unchanged.

- [ ] **Step 1: Identify repeated production literals**

  Search `mobile/lib` production Dart files for repeated route fragments, JSON keys, WebSocket type/action values, and header names. Exclude one-off labels, test data, UUIDs, error text that is not repeated, and UI copy already owned by `session_ui_labels.dart`.

- [ ] **Step 2: Add the smallest feature-local constants owner**

  Keep session transport values together in a sessions data constants file; keep unrelated circle/auth/profile values in their feature data files. Do not create a global `constants.dart`.

- [ ] **Step 3: Replace exact repeated uses**

  Update API clients, realtime client, parsers, and serializers to use the new constants while preserving the exact wire strings.

- [ ] **Step 4: Run Flutter verification**

  Run `flutter test test`, `flutter analyze`, and `dart format --set-exit-if-changed lib test` in the repository-prescribed Flutter Docker environment. Expected result: all tests pass, analyzer reports no issues, and formatting reports no changed files.

### Task 3: Cross-stack duplicate and contract audit

**Files:**
- Modify only files identified by the searches in Tasks 1–2.
- Do not modify `specs/` generated artifacts.

**Interfaces:**
- Produces no new public API; it confirms that selected repeated values have one owning constant per stack and that wire contracts remain unchanged.

- [ ] **Step 1: Search for remaining selected literals**

  Re-run targeted searches for the selected route, header, event/action, status/role, and error values. Remaining occurrences must be either the constant declaration, documentation, SQL, tests, fixtures, or intentionally unique text.

- [ ] **Step 2: Verify the diff boundary**

  Run `git diff --check` and `git status --short`; confirm unrelated worktree changes are preserved and no generated cache or build artifacts are added.

- [ ] **Step 3: Run final applicable gates**

  Run the backend and Flutter gates from Tasks 1–2 plus contract lint if OpenAPI or shared HTTP values changed. Report integration gates separately when device/backend infrastructure is unavailable.
