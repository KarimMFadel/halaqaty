# Quickstart: 002-circle-management

1. Confirm the branch is `002-circle-management` and read `AGENTS.md`, the constitution, the harness, `spec.md`, `checklists/requirements.md`, and the canonical circle docs.
2. Before implementation, amend the stale F-002 permanent-delete/private-only/gender wording, record the retirement decision, accept `ADR-012-audit-logging-persistence.md`, and synchronize `docs/contracts/openapi.yaml`.
3. Apply the next additive PostgreSQL migration after the current repository head. Verify fresh-schema migration, upgrade from the current schema, rollback, defaults, constraints, indexes, and no hard-delete SQL.
4. Implement backend routes in this order: list/create, read/update/archive, public discovery/direct join, invite join/refresh, members/remove, then role changes. Reuse Feature 001 auth/session middleware and centralized route/error constants.
5. Implement transactional repository methods with package-level SQL queries, row locking where membership/role invariants depend on concurrent state, audit events, request IDs, timeouts, and safe retry behavior.
6. Implement Flutter circle list, public discovery, create/edit, invite/share, join confirmation, member/role management, and archived read-only state using Riverpod and Arabic-first RTL layouts.
7. Run focused backend unit/contract/integration tests and Flutter widget tests for every user story, then run `make api-lint`, `go test -short ./...`, `flutter test test`, formatters, analyzers, lint, and secret scanning.
