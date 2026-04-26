# ADR-006: Database Migration Tool — golang-migrate

**Status:** Accepted  
**Date:** 2026-04-26  
**Deciders:** Karim (product owner)

---

## Context

The Halaqaty backend needs a database migration strategy. Every schema change must be:
- Version-controlled alongside application code.
- Reversible (`down` migrations for rollback).
- Applicable to a fresh database from scratch (CI environment, new developer setup).
- Applicable incrementally (CI applies only new migrations on an already-initialized DB).
- Compatible with the modular monolith structure (ADR-001), where multiple domain packages contribute to the same PostgreSQL schema.

---

## Decision

We will use **golang-migrate v4** (`github.com/golang-migrate/migrate/v4`) with plain SQL migration files.

**File naming convention:**
```
migrations/
├── 000001_create_users.up.sql
├── 000001_create_users.down.sql
├── 000002_create_circles.up.sql
├── 000002_create_circles.down.sql
├── 000003_create_circle_members.up.sql
├── 000003_create_circle_members.down.sql
└── ...
```

**Rules:**
1. Sequential 6-digit numbering (`000001`, `000002`, …). No timestamp prefixes.
2. Every migration **must** have a corresponding `.down.sql`.
3. Migrations are plain SQL only — no Go code in migration files.
4. All migrations run in a single `migrations/` directory at the repo root. Domain packages do not have their own migration directories.
5. Application startup does **not** auto-migrate. Migrations are run explicitly via `make migrate-up`.
6. Migration state is tracked in the `schema_migrations` table managed by golang-migrate (not custom).

**Makefile targets:**
```makefile
migrate-up:      # apply all pending migrations
migrate-down:    # roll back the last migration
migrate-create:  # create a new migration pair (up + down)
migrate-fresh:   # drop all tables, re-apply all migrations (CI / local reset)
migrate-status:  # show current migration version
```

---

## Consequences

**Positive:**
- Migration files are plain SQL — readable, diffable, reviewable in PR without Go knowledge.
- Reversibility is enforced structurally: a PR with an `up.sql` but no `down.sql` fails the CI quality gate.
- golang-migrate integrates with the Go test suite: integration tests run `migrate-fresh` to get a clean schema.
- Sequential numbering makes migration order unambiguous. Timestamp-based naming (e.g., `20240426_create_users`) can cause merge conflicts when two developers create migrations concurrently — this is rare in a one-developer project but still avoided.

**Negative:**
- No auto-migration on startup means a developer who pulls the branch must remember to run `make migrate-up`. Mitigated by the Makefile and the `migrate-status` command showing drift.
- Single migration directory means domain packages cannot "own" their migrations. A circle domain change and an auth domain change land in the same flat directory. Acceptable for MVP; revisit if domain count grows.

---

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| **GORM AutoMigrate** | Migrates schema automatically from struct tags. Non-reversible; produces no migration files; incompatible with our no-ORM policy (constitution mandates `pgx` or `sqlc`). |
| **Atlas** | Schema-as-code, powerful diff engine. More complex CLI; overkill for MVP. Recommended as a migration path if the schema grows substantially. |
| **Flyway / Liquibase** | Java-based tooling. Foreign to Go ecosystem; adds JVM dependency to the build environment. |
| **Hand-rolled migration runner** | Full control. Would require writing retry logic, locking, schema_versions table management — all solved by golang-migrate. No benefit over the library. |
| **Timestamp-based numbering** | Common in Ruby on Rails. Causes non-deterministic ordering in edge cases with parallel branches; sequential is unambiguous. |

---

## References

- `docs/ARCHITECTURE.md` — full DB schema (all tables that migrations will create)
- `.specify/memory/constitution.md` — "golang-migrate v4 for schema evolution" entry
- `Makefile` — `migrate-*` targets
