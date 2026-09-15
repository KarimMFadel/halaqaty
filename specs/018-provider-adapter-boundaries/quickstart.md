# Verification Quickstart

1. Confirm `018-provider-adapter-boundaries`, current worktree/diff, approved F-018 decisions, and unchecked tasks; preserve unrelated edits.
2. Run the boundary contract tests with contract tags before production changes; the extended composition scan must expose current SDK imports. Add meaningful neutral-fake policy tests and preserve existing adapter/lifecycle coverage.
3. Run affected chat and adapter unit packages after storage changes. Adapt real storage fixtures and run the existing chat-media/revocation/cleanup/reconciliation integration scenarios with the authorized local environment. Create only isolated disposable buckets; do not remove user data.
4. Run affected LiveKit/session tests and existing mobile session/model/controller suites after the media seam changes. Extend the existing mobile guard, retaining serialization and lifecycle coverage.
5. Run the full final gates listed in plan.md once on the completed tree. Read `docs/engineering/development/LOCAL_ENVIRONMENT_RUNBOOKS.md` before Docker integration and Spectral fallback; credentials stay local and redacted.
6. Record commands, final exit codes, counts, failed/blocked/skipped gates and review status. Apply project guards and Tech Lead review. Karim manual security review remains a distinct approval gate; do not merge automatically.

Focused test success is iteration evidence, not the complete contract/integration/coverage gate. No F-004 task artifacts are changed.
