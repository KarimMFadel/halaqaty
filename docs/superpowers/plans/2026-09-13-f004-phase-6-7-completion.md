# F-004 Phase 6-7 Completion Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete the remaining Phase 6 lifecycle-security and Phase 7 authorized-direct-conversation test deliverables without expanding the approved F-004 surface.

**Architecture:** Reuse the existing integration environments, service seams, REST handlers, and Flutter fakes. Add only the missing matrix cases and assertions; do not add endpoints, schema, dependencies, or alternate authorization paths. Normalize only files touched by this work and keep repository-wide line-ending policy separate from feature behavior.

**Tech Stack:** Go 1.26, pgx/PostgreSQL, httptest/WebSocket hub, Flutter/Riverpod, integration_test, existing Docker/Xvfb and Firebase environment.

**Spec:** `specs/004-real-time-chat/spec.md`, `specs/004-real-time-chat/plan.md`, `specs/004-real-time-chat/tasks.md`

## Global Constraints

- Current Firebase identity, matching backend session, current membership period, and current DM eligibility are checked on every protected operation.
- Archived circles retain eligible history/search/playback but reject send, upload, typing, mark-read, pin, unpin, and delete mutations.
- A direct conversation is authorized only for teacher-student or supervisor-student current roles in at least one shared active circle.
- Direct history is retained and restored when a qualifying relationship returns; media remains authorized while any qualifying circle remains.
- Do not add a new database table, column, endpoint, dependency, or provider abstraction.
- Mark a Spec-Kit task `[X]` only after its deliverable exists and its required verification has fresh evidence.

---

### Task 1: Normalize the touched-file formatting surface

**Files:**
- Modify: files already changed in the current worktree, preserving unrelated user edits
- Test: repository formatting checks

- [X] Step 1: Inspect `.gitattributes`, Git line-ending settings, and `git ls-files --eol` for touched files.
- [X] Step 2: Run `gofmt` on changed Go files and `dart format` on changed Dart files only.
- [X] Step 3: Run `git diff --check` and record any remaining warnings as Git normalization diagnostics rather than code whitespace failures.

### Task 2: Complete T053 backend lifecycle security coverage

**Files:**
- Modify: `backend/tests/integration/chat_lifecycle_security_test.go`
- Test: the same integration file with PostgreSQL and the existing lifecycle harness

- [ ] Step 1: Add failing cases for active-session expiry/inactive session, archived upload/delete/typing/pin/unpin denials, archived retained history/search/playback, and DM denial after removal.
- [ ] Step 2: Run the focused integration tests and confirm each missing assertion fails for the expected reason.
- [ ] Step 3: Use existing service/handler authorization seams; add no production behavior unless a focused test exposes a real regression.
- [X] Step 4: Run all `TestChatLifecycleSecurity_*` integration tests and mark T053 only if the complete suite passes.

### Task 3: Complete T057 mobile lifecycle acceptance coverage

**Files:**
- Modify: `mobile/integration_test/chat_lifecycle_flow_test.dart`
- Test support: existing fake API, realtime client, and controller seams in that file

- [ ] Step 1: Add assertions for retained archived history, archived search/playback visibility, all archived mutation controls, and behavior outside the live-session shell.
- [ ] Step 2: Run the focused Flutter integration test with the configured backend/device; preserve `markTestSkipped` only when required environment variables are absent.
- [X] Step 3: Mark T057 only after the integration test runs rather than skips.

### Task 4: Complete T058 backend direct-service matrix coverage

**Files:**
- Create or modify: `backend/internal/chat/direct_service_test.go` or the repository’s equivalent direct integration test file
- Test: direct-service/repository integration tests with PostgreSQL and media fixture support

- [ ] Step 1: Add failing cases covering both allowed role directions, every prohibited role pair, no qualifying circle, multi-circle media continuity, last-circle revocation, restored full history, and authenticated-user direct realtime delivery.
- [ ] Step 2: Run the focused tests and verify the failures are behavioral, not fixture/setup errors.
- [ ] Step 3: Reuse the production SQL authorization and projector seams; do not duplicate role rules in an unused helper.
- [X] Step 4: Run the complete direct-service integration package and mark T058 only after all cases pass.

### Task 5: Complete T060 direct REST contract coverage

**Files:**
- Modify: `backend/tests/contract/chat_direct_contract_test.go`
- Test: unfiltered direct contract suite under the `contract` build tag

- [ ] Step 1: Add failing contract cases for list/send/own-delete/read/media, idempotency replay/conflict, every RBAC denial, cross-circle non-enumeration, rate-limit responses, and private response-field safety.
- [ ] Step 2: Run the focused contract tests and verify expected failures.
- [ ] Step 3: Keep the contract tests at the handler boundary using existing service stubs or real repository fixtures as appropriate.
- [X] Step 4: Run `make test-contract` from `backend` and mark T060 only on an unfiltered pass.

### Task 6: Complete T062 mobile direct unit/widget coverage

**Files:**
- Modify: `mobile/test/features/chat/application/direct_chat_controller_test.dart`
- Modify: `mobile/test/widget/chat/direct_chat_screen_test.dart`
- Test support: existing direct-chat fakes and circle-member entry tests

- [ ] Step 1: Add failing tests for eligibility loss/restoration, pair-history restoration, multi-circle media continuity, safe denied operations, both allowed role pairs, and prohibited/self/archived entry.
- [ ] Step 2: Run the focused Flutter tests and confirm RED behavior before implementation changes.
- [ ] Step 3: Implement only the smallest missing controller/widget behavior, preserving Riverpod and Arabic/RTL conventions.
- [X] Step 4: Run the focused tests and then the full Flutter unit/widget suite.

### Task 7: Complete T064 direct mobile acceptance coverage

**Files:**
- Modify: `mobile/integration_test/chat_direct_flow_test.dart`
- Test support: existing CircleApiClient, ChatApiClient, and ChatMediaApiClient environment credentials

- [ ] Step 1: Add both allowed directions, all prohibited pairs, cross-circle denial, last-circle loss, multi-circle media, restored-history, and safe-denial assertions where absent.
- [X] Step 2: Run the integration test against the real backend/Firebase/PostgreSQL/MinIO stack.
- [X] Step 3: Mark T064 only if the real acceptance test passes; otherwise leave it unchecked with the exact environment blocker.

### Task 8: Final phase gate and checklist reconciliation

**Files:**
- Modify: `specs/004-real-time-chat/tasks.md`

- [ ] Step 1: Run backend unit, contract, and integration gates plus Flutter unit/widget, analyze, and formatting checks.
- [ ] Step 2: Run available mobile integration tests and distinguish passed, skipped, blocked, and failed outcomes.
- [ ] Step 3: Update only task markers supported by fresh evidence; leave environment-gated tasks unchecked when skipped.
- [ ] Step 4: Review the final diff for scope, security, data-loss, contract, and Ponytail regressions.
