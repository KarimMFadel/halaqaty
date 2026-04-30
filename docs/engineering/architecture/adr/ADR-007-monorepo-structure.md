# ADR-007: Monorepo Structure and CI Strategy

**Status:** Accepted  
**Date:** 2026-04-26  
**Deciders:** Karim (product owner)

---

## Context

Halaqaty has two primary codebases — a Go backend and a Flutter mobile application — that must evolve together during the MVP phase. API contracts, database migrations, and mobile client implementations are tightly coupled in these early iterations: a single feature commonly spans a new endpoint, a migration, and a Flutter screen in the same unit of work.

We need to decide whether to maintain a single repository (monorepo) or separate repositories per layer. The decision affects how PRs are scoped, how CI pipelines are structured, where ownership boundaries are drawn, and how the project is released as it grows.

The team is currently one developer (Karim) assisted by Copilot agents. Operational simplicity, cross-layer traceability, and low cognitive overhead are the primary forces. The architecture must also support the clean future split described in ADR-001 if the platform scales past the single-developer stage.

---

## Decision

We will use a **single Git repository (monorepo)** for the entire Halaqaty project, with strict ownership boundaries enforced by directory layout, CI checks, and code review conventions.

### Repository Layout

```
halaqaty/
├── cmd/
│   └── api/                  ← Go binary entry point (main.go, DI wiring)
├── internal/                 ← Go domain packages (backend owners)
│   ├── auth/
│   ├── circles/
│   ├── sessions/
│   ├── queue/
│   ├── chat/
│   ├── progress/
│   ├── schedule/
│   ├── notifications/
│   └── shared/
├── migrations/               ← Plain SQL migration files (backend owners)
│   ├── 000001_create_users.up.sql
│   ├── 000001_create_users.down.sql
│   └── ...
├── mobile/                   ← Flutter application root (mobile owners)
│   ├── lib/
│   ├── test/
│   └── pubspec.yaml
├── docs/                     ← All project documentation (shared)
│   ├── contracts/            ← OpenAPI spec, WebSocket event catalog
│   ├── engineering/
│   └── management/
├── specs/                    ← Spec-Kit feature specs (shared)
│   └── NNN-feature-name/
│       ├── spec.md
│       ├── plan.md
│       └── tasks.md
├── .github/
│   └── workflows/            ← CI pipeline definitions
├── docker-compose.yml        ← Local dev and deployment (shared infra)
├── Makefile                  ← Developer task runner (shared infra)
└── .specify/                 ← Spec-Kit memory and templates
```

### Ownership Boundaries

| Directory | Owner | Change Policy |
|---|---|---|
| `internal/`, `cmd/`, `migrations/` | Backend | Requires Go test pass + golangci-lint green |
| `mobile/` | Mobile | Requires flutter test pass + flutter analyze green |
| `docs/contracts/` | Backend + Mobile (joint) | Requires OpenAPI lint pass; changes must be backward-compatible or version-bumped |
| `docker-compose.yml`, `Makefile` | Shared Infra | Either owner may propose; Karim approves |
| `specs/`, `docs/` | Shared | Any agent may update; Karim approves on merge |
| `.github/workflows/` | Shared Infra | Karim approves all changes |

### CI Strategy — GitHub Actions

Three workflow files are maintained in `.github/workflows/`:

**`ci-backend.yml`** — triggers on push/PR for any change under `internal/`, `cmd/`, `migrations/`, `docs/contracts/`:
```
jobs:
  lint:    golangci-lint run ./...
  test:    go test ./... (unit) + go test -tags=integration ./... (integration)
  migrate: docker run postgres:16-alpine → make migrate-fresh (tests all migrations on fresh schema)
  openapi: spectral lint docs/contracts/openapi.yaml
```

**`ci-mobile.yml`** — triggers on push/PR for any change under `mobile/`:
```
jobs:
  analyze: flutter analyze
  format:  dart format --set-exit-if-changed .
  test:    flutter test
```

**`ci-security.yml`** — triggers on every push to any branch:
```
jobs:
  secrets: gitleaks detect --source . --verbose
```

All three workflows must be green before a PR can be merged. The `ci-security.yml` job runs first as a fast-fail gate. PRs that only touch `mobile/` skip the backend lint/test jobs and vice versa, keeping CI runtimes short.

### Release and Versioning

Backend and mobile are released independently but coordinated via feature flag gates (ADR-005).

| Layer | Versioning Scheme | Release Mechanism |
|---|---|---|
| **Go backend** | Git tag semver: `v1.0.0`, `v1.1.0`, etc. | GitHub Release on tag push; Docker image tagged `halaqaty-api:v1.x.x` |
| **Flutter mobile** | `pubspec.yaml` version: `1.0.0+build` | App Store / Play Store submission; build number incremented per CI build |

**Coordination rule:** A backend API change that is required by a mobile release must be deployed and verified in production **before** the mobile release is submitted to the stores. Feature flags gate new UI flows until the backend endpoint is live and stable. This prevents the mobile client from calling endpoints that do not yet exist in the deployed backend.

Semantic versioning rules:
- **PATCH** (`v1.0.x`): bug fixes, non-breaking internal changes, migrations that add nullable columns.
- **MINOR** (`v1.x.0`): new endpoints, new optional fields. Backward-compatible.
- **MAJOR** (`v2.x.x`): breaking API changes. Requires ADR and mobile coordinated release.

---

## Consequences

**Positive:**
- A single PR can span the full feature slice: migration + backend endpoint + Flutter screen + spec update. No cross-repo coordination, no version references between repos, no "which commit goes with which."
- API contract drift between backend and mobile is detected at PR review time, not at integration test time weeks later. The OpenAPI lint job in `ci-backend.yml` catches undocumented endpoints immediately.
- Early-stage development benefits from shared context: Copilot agents reading the repo see both the Go handler and the Flutter consumer in one clone, producing more coherent implementations.
- The clean package structure (ADR-001) and ownership boundary table make future repo-split straightforward: `internal/` + `migrations/` become a new `halaqaty-backend` repo, `mobile/` becomes `halaqaty-mobile`. The split is a `git filter-repo` operation, not an architectural refactor.
- A single `docker-compose.yml` and `Makefile` serve both backend and mobile developers. New contributors clone one repo and run `make dev` to get a fully running local environment.

**Negative:**
- CI pipelines must be carefully path-filtered to avoid running backend tests on mobile-only changes and vice versa. GitHub Actions path filters handle this, but it requires maintenance discipline.
- Git history mixes backend and mobile commits. At post-MVP team scale, this can make `git log` noisier. Mitigated by the PR title convention (`HLQ-NNN: description`) and squash-merge policy.
- Large binary assets (e.g., test fixtures, screenshots) in `mobile/` inflate clone size for backend-only contributors. Mitigated by `.gitattributes` LFS rules if this becomes a problem.
- At multi-team scale, a monorepo requires more CI sophistication (affected package detection, build caching). Not a concern at MVP; the single-developer team benefits from simplicity today.

---

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| **Separate repos (`halaqaty-backend` + `halaqaty-mobile`)** | Every cross-layer feature requires two PRs, two code reviews, and a manually coordinated merge order. API contract drift is discovered late. Versioning references between repos add overhead with zero benefit at one-developer scale. |
| **Monorepo with Nx or Turborepo build orchestration** | Adds a Node.js dependency and configuration layer for a Go + Dart project. The path-filtered GitHub Actions approach achieves equivalent CI scoping without the overhead. Revisit if the repo grows to 5+ packages. |
| **Git submodules** | Shared infrastructure (`docker-compose.yml`, `Makefile`, `docs/contracts/`) cannot be a submodule without creating a third "infra" repo. Submodule pointer management is a known developer experience pain point. |
| **Backend monorepo + mobile as separate repo** | Offers no meaningful isolation benefit at current team size. Mobile still needs the OpenAPI spec, which lives in the backend repo. Cross-repo contract versioning is unnecessary complexity for MVP. |

---

## References

- `ADR-001-modular-monolith.md` — internal package structure that the `internal/` layout implements
- `ADR-005-feature-flags.md` — feature flag gates that coordinate backend and mobile releases
- `ADR-006-db-migrations.md` — `migrations/` directory conventions and `make migrate-*` targets
- `.specify/memory/constitution.md` — "Build as modular monolith" and quality gates policy
- `docs/contracts/openapi.yaml` — OpenAPI spec linted in `ci-backend.yml`
- `docs/engineering/deployment/DEPLOYMENT.md §10` — CD pipeline (`deploy.yml`) that deploys to server on push to main; distinct from the CI validation workflows defined here
- `Makefile` — `migrate-*`, `lint`, `test`, `dev` targets referenced by CI
