# Development Guide

> How we build Halaqaty — Spec-Kit, Copilot, and the quality gates that protect production.

---

## Why Spec-Kit?

Traditional "vibe coding" — describing a feature in chat and accepting whatever Copilot generates — produces fast but fragile results. Field names are invented, edge cases are missed, and implementations drift from the product spec within days.

Halaqaty uses **[Spec-Kit](https://github.com/github/spec-kit)** (`v0.8.1`), an open-source spec-driven development toolkit by GitHub. The specification is the source of truth. Code is its generated output.

Before any production line is written:

1. A **specification** defines user stories, acceptance criteria, and requirements — based on our product docs.
2. A **plan** translates the spec into technical architecture, data models, and API contracts.
3. A **task list** breaks the plan into parallelizable implementation steps.
4. Copilot **implements** against the frozen spec and plan — not against a vague prompt.

Every PR is fully traceable: user story → contract → implementation → tests.

---

## Prerequisites

| Tool | Version | Install |
|---|---|---|
| `uv` | 0.11+ | See below |
| `specify-cli` | 0.8.1 | See below |
| VS Code | Latest | with **GitHub Copilot** extension |
| Git | 2.x+ | — |

### Install uv (Windows)

```powershell
powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"
```

Then refresh your PATH in the current shell:

```powershell
$env:PATH = [System.Environment]::GetEnvironmentVariable('PATH', 'User') + ';' + [System.Environment]::GetEnvironmentVariable('PATH', 'Machine')
```

### Install specify-cli

```powershell
uv tool install specify-cli --from git+https://github.com/github/spec-kit.git@v0.8.1
```

### Verify

```powershell
specify version
specify check
```

---

## Slash Command Reference

All Spec-Kit commands are available in **VS Code Copilot Chat** and **GitHub Copilot Agent** using the `/` prefix:

| Command | Phase | Purpose |
|---|---|---|
| `/speckit.constitution` | Setup | Review or amend the governing constitution in `.specify/memory/constitution.md` |
| `/speckit.specify` | Spec | Create a feature specification → `specs/NNN-feature-name/spec.md` + feature branch |
| `/speckit.clarify` | Spec *(optional)* | Ask structured questions to de-risk ambiguities before planning |
| `/speckit.plan` | Plan | Generate `plan.md`, `data-model.md`, `contracts/`, `quickstart.md` from the spec |
| `/speckit.checklist` | Plan *(optional)* | Validate that spec and plan are complete and consistent |
| `/speckit.tasks` | Tasks | Generate parallelizable `tasks.md` from the plan |
| `/speckit.analyze` | Tasks *(optional)* | Cross-artifact consistency check before implementation |
| `/speckit.implement` | Implement | Copilot executes all tasks — writes code, tests, and migrations |
| `/speckit.git.feature` | Branch | Create and name the feature branch per spec-kit convention |
| `/speckit.git.commit` | Commit | Structured commit with spec traceability |
| `/speckit.git.validate` | Validate | Validate git state before opening a PR |
| `/speckit.taskstoissues` | Sync | Push `tasks.md` entries to GitHub Issues |

---

## Feature Implementation Workflow

Every feature in Halaqaty follows this exact pipeline. **No shortcuts.**

### ✅ Pre-flight checklist

Before running any Spec-Kit command, verify:

```
[ ] Feature is listed in docs/FEATURES.md with status ≥ 🟡 Approved
[ ] All open questions for this feature are Decided in docs/MVP_DECISION_REGISTER.md
[ ] User journey for this feature is documented in docs/JOURNEY.md
[ ] You are on main branch, up to date (git pull)
```

---

### Step 1 — Create the spec

Open Copilot Chat in VS Code and run:

```
/speckit.specify [describe the feature in plain language, including doc references]
```

**Example:**
```
/speckit.specify
User authentication: email/password, Google Sign-In, and Apple Sign-In (required on iOS).
Flows: register, login, email verification, password reset, silent token refresh, logout.
See docs/FEATURES.md F-001 and docs/JOURNEY.md T-01 to T-04 for acceptance criteria.
```

This automatically:
- Creates branch `001-auth` (or next available number)
- Creates `specs/001-auth/spec.md` with structured user stories and acceptance criteria

**Review before continuing.** Check:
- User stories match `docs/FEATURES.md` acceptance criteria
- Edge cases from `docs/JOURNEY.md` are covered
- No `[NEEDS CLARIFICATION]` markers remain

---

### Step 2 — *(Optional)* Clarify ambiguities

```
/speckit.clarify
```

Run this if the spec has open areas. Generates structured questions; answer them to close gaps before planning.

---

### Step 3 — Create the plan

```
/speckit.plan [describe tech choices and reference architecture docs]
```

**Example:**
```
/speckit.plan
Go backend, Echo v4. Firebase Auth JWT middleware. PostgreSQL users table via golang-migrate.
Flutter + Riverpod 2.x. See docs/ARCHITECTURE.md for full schema and endpoint definitions.
Constitution: .specify/memory/constitution.md
```

This creates:
- `specs/001-auth/plan.md` — implementation plan
- `specs/001-auth/data-model.md` — entity definitions and DB migrations
- `specs/001-auth/contracts/` — REST endpoints and WebSocket events
- `specs/001-auth/quickstart.md` — key validation scenarios

**Review the plan.** Check:
- DB migration matches `docs/ARCHITECTURE.md` schema exactly
- No new tables or columns invented without an ADR
- API endpoints match planned contract in `docs/contracts/openapi.yaml`

---

### Step 4 — *(Optional)* Validate completeness

```
/speckit.checklist
```

Generates a quality checklist. Fix any gaps before continuing.

---

### Step 5 — Generate task list

```
/speckit.tasks
```

Creates `specs/001-auth/tasks.md` with tasks annotated `[P]` for parallelizable work.

Review it — confirm parallel tasks are genuinely independent.

---

### Step 6 — *(Recommended)* Cross-artifact analysis

```
/speckit.analyze
```

Checks consistency across spec, plan, data model, and contracts. Fix any inconsistencies before implementation.

---

### Step 7 — Implement

```
/speckit.implement
```

Copilot generates all code, tests, and migration files per the task list. Monitor actively — intervene if the implementation deviates from the spec or constitution.

---

### Step 8 — Verify quality gates

All of these must be **green** before opening a PR:

| Gate | Command | Requirement |
|---|---|---|
| Go unit tests | `go test ./...` | 100% pass |
| Flutter tests | `flutter test` | 100% pass |
| Go integration tests | `go test -tags=integration ./...` | 100% pass |
| DB migration (fresh schema) | `make migrate-fresh` | No errors |
| Go linter | `golangci-lint run` | Zero violations |
| Dart analyzer | `flutter analyze` | Zero issues |
| Dart formatter | `dart format --set-exit-if-changed .` | No diff |
| Go formatter | `gofmt -l .` | Empty output |
| Secret scan | `gitleaks detect` | No findings |

---

### Step 9 — Open PR

```
/speckit.git.commit
```

**PR title format:** `HLQ-NNN: feature name` (enforced by GitHub Actions)

**PR description must include:**
```
Implements: specs/NNN-feature-name/

## Summary
[brief description of what was implemented]

## Spec reference
- User stories: specs/NNN-feature-name/spec.md
- Plan: specs/NNN-feature-name/plan.md
- Tasks: specs/NNN-feature-name/tasks.md
```

PRs are **opened by Copilot**, reviewed and **merged by Karim only**. No merge without all green gates.

---

## Feature Status Lifecycle

```
🔵 Proposed → 🟡 Approved → 🔧 In Progress → ✅ Shipped → 🔒 Frozen (post-MVP, behind flag)
```

| Status | Meaning | Gate |
|---|---|---|
| 🔵 Proposed | Idea or backlog item | — |
| 🟡 Approved | Spec-ready: all open questions resolved, journey documented | PM approval |
| 🔧 In Progress | `/speckit.specify` run, branch is open | Spec file exists |
| ✅ Shipped | PR merged, deployed | All quality gates green |
| 🔒 Frozen | Post-MVP, behind feature flag | Feature flag `false` in all envs |

---

## Branch Naming

Spec-Kit creates branches automatically via `/speckit.git.feature`. Format:

```
NNN-feature-name
```

Examples: `001-auth`, `002-circles`, `003-queue`, `004-chat`

---

## Project Structure (when code exists)

```
halaqaty/
├── .specify/                        ← Spec-Kit config and templates
│   └── memory/
│       └── constitution.md          ← THE governing document — read this first
├── .github/
│   ├── prompts/                     ← Spec-Kit slash command definitions
│   ├── agents/                      ← Spec-Kit + custom Copilot agents
│   └── workflows/                   ← GitHub Actions CI/CD
├── docs/                            ← Human-readable strategy & product docs
│   ├── FEATURES.md                  ← Feature status board (index)
│   ├── ARCHITECTURE.md              ← DB schema, API endpoints, security
│   ├── JOURNEY.md                   ← Full user journey (teacher-first)
│   ├── MVP_DECISION_REGISTER.md     ← All frozen MVP decisions
│   ├── adr/                         ← Architecture Decision Records
│   │   ├── README.md                ← ADR index
│   │   ├── ADR-001-modular-monolith.md
│   │   ├── ADR-002-go-framework.md
│   │   ├── ADR-003-flutter-state-management.md
│   │   ├── ADR-004-auth-boundary.md
│   │   ├── ADR-005-feature-flags.md
│   │   └── ADR-006-db-migrations.md
│   ├── contracts/                   ← OpenAPI spec + WebSocket event catalog
│   └── arabic/                      ← Arabic business doc mirrors
├── specs/                           ← Spec-Kit generated (per feature) — DO NOT EDIT MANUALLY
│   ├── 001-auth/
│   │   ├── spec.md
│   │   ├── plan.md
│   │   ├── data-model.md
│   │   ├── contracts/
│   │   ├── tasks.md
│   │   └── quickstart.md
│   └── ...
├── cmd/
│   └── api/                         ← Go entry point (main.go)
├── internal/                        ← Go application code (domain packages)
│   ├── auth/
│   ├── circles/
│   ├── chat/
│   ├── sessions/
│   ├── queue/
│   ├── progress/
│   ├── schedule/
│   ├── notifications/
│   └── shared/
├── mobile/                          ← Flutter application
│   ├── lib/
│   │   ├── features/                ← Feature-first Flutter structure
│   │   ├── core/
│   │   └── main.dart
│   └── pubspec.yaml
├── migrations/                      ← golang-migrate SQL files
│   ├── 000001_create_users.up.sql
│   ├── 000001_create_users.down.sql
│   └── ...
├── docker-compose.yml               ← MVP deployment
├── Makefile                         ← build, test, lint, migrate targets
└── DEVELOPMENT.md                   ← This file
```

---

## Key Documents

| Document | Purpose |
|---|---|
| [`.specify/memory/constitution.md`](.specify/memory/constitution.md) | **Read first.** Governing principles for all code. |
| [`docs/FEATURES.md`](docs/FEATURES.md) | Feature status board — what's Approved vs Proposed |
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | DB schema, API endpoints, security model |
| [`docs/JOURNEY.md`](docs/JOURNEY.md) | Screen-by-screen user journey with error/offline states |
| [`docs/MVP_DECISION_REGISTER.md`](docs/MVP_DECISION_REGISTER.md) | All frozen MVP rules — binding on implementation |
| [`docs/adr/`](docs/adr/) | Architecture Decision Records — why we chose each technology |
| [`docs/contracts/`](docs/contracts/) | OpenAPI spec + WebSocket event catalog |

---

*Built with [Spec-Kit](https://github.com/github/spec-kit) · Powered by [GitHub Copilot](https://github.com/features/copilot)*
