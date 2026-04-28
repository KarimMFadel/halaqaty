# Contributing to Halaqaty

Thank you for your interest in Halaqaty — the Quran memorization circle management platform.

> **Current status:** Halaqaty is in active pre-launch development by a solo founder with AI agent assistance. External contributions are welcome but reviewed carefully. Please read this document before opening a pull request.

---

## Table of Contents

1. [Who Can Contribute](#who-can-contribute)
2. [What We're Looking For](#what-were-looking-for)
3. [Development Setup](#development-setup)
4. [Branching & Commit Conventions](#branching--commit-conventions)
5. [Pull Request Process](#pull-request-process)
6. [Code Standards](#code-standards)
7. [Issue Guidelines](#issue-guidelines)
8. [Community](#community)

---

## Who Can Contribute

- **Native Arabic speakers** — localization, Arabic UX review, right-to-left layout feedback
- **Quran teachers / حفّاظ** — domain expertise on recitation workflows, grading conventions
- **Flutter / Go developers** — bug fixes and feature PRs (read the Acceptance Policy below)
- **Security researchers** — responsible disclosure (see [Security](#security))

---

## What We're Looking For

### Green-light contributions

- Bug fixes with a clear reproduction case
- Localization improvements (Arabic and other languages)
- Documentation improvements
- Performance improvements with benchmark evidence
- Accessibility improvements (Flutter a11y)

### Contributions that need prior discussion

Open an issue before coding:
- New features — check if it's already on the [roadmap](docs/management/planning/PLAN.md)
- Architecture changes — read [ARCHITECTURE.md](docs/engineering/architecture/ARCHITECTURE.md) first
- Changes to the auth flow, RBAC, or data deletion paths

### We won't accept

- Features that haven't been discussed and agreed in an issue first
- PRs that remove existing test coverage without justification
- PRs that introduce new runtime dependencies without a clear performance/security rationale

---

## Development Setup

### Prerequisites

| Tool | Version |
|------|---------|
| Go | ≥ 1.23 |
| Flutter | ≥ 3.24 |
| Docker + Docker Compose | ≥ 26 |
| PostgreSQL (via Docker) | 16 |

### First-time setup

```bash
# 1. Clone the repository
git clone https://github.com/KarimMFadel/halaqaty.git
cd halaqaty

# 2. Start infrastructure (PostgreSQL, LiveKit, MinIO)
docker compose up -d

# 3. Run database migrations
make migrate-up

# 4. Start the Go backend
go run ./cmd/server

# 5. Start the Flutter app (in a separate terminal)
flutter run
```

See [DEVELOPMENT.md](DEVELOPMENT.md) for the complete development workflow.

---

## Branching & Commit Conventions

### Branch naming

```
feature/F-NNN-short-description   # new feature (F-NNN = feature ID from FEATURES.md)
fix/short-description              # bug fix
chore/short-description            # deps, tooling, config
docs/short-description             # documentation only
```

### Commit messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat(queue): add student opt-out endpoint

Implements POST /api/v1/sessions/{id}/queue/opt-out.
Returns 200 on success, 409 if no active round.

Closes #42
```

**Types:** `feat` | `fix` | `docs` | `refactor` | `test` | `chore` | `perf`

---

## Pull Request Process

1. **Open an issue first** for anything beyond a trivial bug fix.
2. Fork the repository and create a branch from `main`.
3. Write tests for your change. Coverage must not drop below the baseline.
4. Ensure all CI checks pass: `go test ./...`, `flutter test`, OpenAPI lint.
5. Fill in the PR template completely — empty template fields will result in the PR being closed.
6. A review will be conducted by the Tech Lead AI agent + final merge approval by Karim (the sole maintainer). Response time target: 5 business days.

### PR Checklist

- [ ] Tests added / updated
- [ ] `openapi.yaml` updated if REST API changed
- [ ] `ws_events.md` updated if WebSocket events changed
- [ ] Documentation updated if behavior changed
- [ ] No hardcoded secrets or API keys
- [ ] No new runtime dependencies without prior discussion

---

## Code Standards

### Go

- Follow [Effective Go](https://go.dev/doc/effective_go) and the [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments)
- `golangci-lint` must pass with zero new warnings
- All exported functions must have a doc comment
- Error handling: wrap errors with `fmt.Errorf("operation: %w", err)`, never discard errors silently

### Flutter / Dart

- Follow the [Dart Style Guide](https://dart.dev/guides/language/effective-dart/style)
- Use Riverpod for all state management — no `setState` in feature code, only in truly isolated leaf widgets
- All `async` functions must handle errors at the call site or bubble them to the nearest error boundary provider

### Markdown / Documentation

- Use sentence case for headings
- All tables must have a header row
- Cross-references use relative paths (not absolute URLs)

---

## Issue Guidelines

### Bug reports

Include:
1. Flutter/Go version
2. Device/OS details
3. Steps to reproduce (exact, numbered)
4. Expected vs actual behavior
5. Relevant logs or screenshots

### Feature requests

1. Describe the user story: "As a [role], I want [capability] so that [benefit]"
2. Check if it conflicts with existing design decisions in [ARCHITECTURE.md](docs/engineering/architecture/ARCHITECTURE.md) or [FEATURES.md](docs/management/product/FEATURES.md)
3. Indicate if you're willing to implement it

---

## Security

For security vulnerabilities, **do not open a public issue**. Email: security@halaqaty.app

We follow responsible disclosure: we will acknowledge within 48 hours and aim to patch critical issues within 7 days.

See the [Security section in ARCHITECTURE.md](docs/engineering/architecture/ARCHITECTURE.md#6-security--authorization-model) for the current security model.

---

## Community

- Questions about Halaqaty or Quran memorization circles: open a Discussion (not an Issue)
- Language: English or Arabic — both are fully supported in discussions

*جزاكم الله خيرًا على مساهمتكم*
