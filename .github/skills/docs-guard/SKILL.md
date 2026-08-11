---
name: docs-guard
description: "Review generated or changed documentation before it ships — OpenAPI contracts, WebSocket event catalogs, ADRs, Go package docstrings, Dart API docs, READMEs, and changelogs. Adapted for Halaqaty's contract-first API design (docs/contracts/) and architecture decision records. Best used reactively after code changes a documented behavior, after writing a new endpoint, or before merging a PR that touches API contracts. Use when the user says 'review the docs', 'update the API contract', 'add a docstring', 'write an ADR', 'is this documentation accurate', or 'document this endpoint'. Core job: verify every referenced symbol, endpoint, and schema against the source; catch docs-vs-code drift; keep contracts honest. DO NOT USE for production code review (use clean-code-guard), test review (use test-guard), or marketing copy."
---

# Docs Guard

You are reviewing generated or changed documentation before it ships. Apply the rules below as a guard pass after the first documentation pass. The core principle: documentation is a set of claims about a codebase, and every claim is checkable. Your job is to check them.

These rules exist because AI agents document from memory of how APIs *usually* look, not from the code in front of them. Readers cannot tell verified docs from hallucinated docs — but you can, because you have the source.

## How to use this skill

**Guard-pass mode** (recommended): after docs or docstrings have been generated or edited, verify every claim against the source and run the self-check before delivery.

**Live mode** (explicit): when invoked before writing docs, read the actual implementation first, then document what it does. Run the self-check before delivery.

**Review mode** (user asks to review, audit, or fact-check docs): walk the rules against the target docs and produce a findings report with file:line evidence. Do not rewrite in review mode unless asked.

## Halaqaty documentation surfaces

Halaqaty's critical documentation surfaces are:

| Surface | Path | Must stay in sync with |
|---------|------|------------------------|
| OpenAPI contract | `docs/contracts/openapi.yaml` | Go handler routes, request/response structs |
| WebSocket event catalog | `docs/contracts/ws_events.md` | Go WebSocket event types and payloads |
| Architecture Decision Records | `docs/engineering/architecture/adr/` | Constitution + actual implementation |
| Go package docstrings | `backend/internal/**/*.go` (exported symbols) | Actual function signatures and behavior |
| Dart API docs | `mobile/lib/**/*.dart` (public APIs) | Actual class/method signatures |
| README / DEVELOPMENT.md | root, `backend/`, `mobile/` | Actual build commands, env vars, setup steps |
| Spec artifacts | `specs/NNN-feature-name/` | Implementation in `backend/` and `mobile/` |

**The OpenAPI contract is the source of truth for REST API shape** (Constitution §III). No endpoint may be implemented that is not in that contract, and no endpoint in that contract may describe a different shape than what is implemented.

## Adapt to the project first

1. Read `.specify/memory/constitution.md` — especially:
   - §III: "REST endpoints are documented in `docs/contracts/openapi.yaml`. No endpoint may be implemented that is not in that contract."
   - §III: "WebSocket events follow the schema catalog in `docs/contracts/ws_events.md`."
   - The standard error envelope format: `{"error": {"code": "...", "message": "..."}}`
2. Check which docs surfaces the current change touches — a handler change likely owes both an OpenAPI update and a docstring update.
3. Grep docs for old symbol names before a rename lands.

## The Rules

### Accuracy — must fix

1. **Every referenced symbol must exist.** Every function, method, endpoint path, query parameter, request field, response field, config key, env var, and file path mentioned in the docs must be verified against the actual source — by reading it, not recalling it. An unverifiable reference does not ship.
   - **Go**: verify Go exported function/type names against actual package symbols. Use the file directly, not memory.
   - **OpenAPI**: verify that every `operationId`, path, and schema `$ref` resolves against the actual route registrations and Go response structs.
   - **WebSocket catalog**: verify that every event `type` field matches the Go constant or string used in the WebSocket handler.

2. **Every code sample must work.** Imports resolve, APIs exist with the documented signatures, and the sample runs outside the author's machine — no hardcoded local paths, no real credentials, no implicit prior state.
   - **Go samples**: verify package import paths match `go.mod` module name. Verify function signatures match the current version.
   - **Dart samples**: verify package names match `pubspec.yaml`. Verify API matches the installed package version.
   - **curl/HTTP samples**: verify that the path, method, headers, and body shape match `openapi.yaml`.

3. **Document the code's actual behavior, not its intended behavior.** Read the implementation before describing it. Where code and docs disagree, the code is the truth — and flag the disagreement to the user instead of silently picking a side.
   - **Critical for Halaqaty**: if a handler has a security check (e.g., `circle_members` role verification) that the docs do not mention, add it. Undocumented auth requirements are a security gap.

4. **No unverifiable claims.** Performance numbers, compatibility matrices, scale limits, and "production-ready" assertions require a source in the repository (benchmark, CI matrix, changelog) or they come out. "Fast" is marketing; "benchmarked in `bench/`" is documentation.

### Versioning and drift

5. **A code change owes a docs change.** When editing code whose behavior is documented — rename, signature change, new default, removed field, new error code — update every doc surface that mentions it in the same change.
   - **Go endpoint change**: update `docs/contracts/openapi.yaml` in the same PR. No exceptions.
   - **Feature contract sync**: when `specs/NNN-feature/contracts/` contains the same REST surface, regenerate or synchronize it through the Spec-Kit workflow; do not hand-edit generated artifacts. Compare methods, paths, schemas, and every documented success/error response against the canonical contract; reject duplicate YAML keys.
   - **WebSocket event change**: update `docs/contracts/ws_events.md` in the same PR.
   - **Grep rule**: before finishing, run `grep -r "old_symbol_name" docs/` to find every docs surface that mentions the old name.

6. **ADRs document real decisions.** An ADR in `docs/engineering/architecture/adr/` must:
   - Describe the decision that was actually made (not a placeholder)
   - State the alternatives that were considered
   - Reference the constitutional constraint it satisfies or amends
   - Be numbered sequentially and linked from the constitution if it amends it
   An ADR with "TBD" in the decision section does not ship.

### Substance — should fix

7. **No filler, no slop.** Delete:
   - Go docstrings that paraphrase the signature (`// GetCircle gets the circle by ID` above `func GetCircle(id uuid.UUID)`)
   - OpenAPI `description` fields that repeat the `summary`
   - Sections in READMEs that restate their heading
   - Marketing adjectives in technical prose ("powerful", "seamless", "blazingly fast")
   A docstring earns its place by adding contracts the signature cannot express: units, ranges, error conditions, side effects, auth requirements, ordering guarantees.

8. **Don't paraphrase upstream docs.** Link to the official docs for `pgx`, LiveKit, Firebase, and Flutter packages instead of restating them. Document only Halaqaty's relationship to the external thing: what subset is used, what is configured differently, what is explicitly disabled (e.g., noise suppression, recording).

9. **Show the failure path too.** API documentation that only shows the happy-path request/response documents half the API. The OpenAPI contract must include:
   - `400` for invalid input (with the error envelope schema)
   - `401` for unauthenticated
   - `403` for forbidden (wrong role in `circle_members`)
   - `404` for not found
   - `409` for conflict where applicable
   - `422` for unprocessable

### Structure — worth noting

10. **Navigation tells the truth.** Headings describe their sections, the table of contents matches actual headings, internal links resolve, and there are no TODO stubs in published docs. In OpenAPI: all `$ref` values resolve, all `operationId` values are unique, all required fields in schemas are actually required in the implementation.

## Self-check before delivery

1. List every symbol, endpoint, field, event type, and env var your docs mention. Did you verify each one against the source in this session — not from memory?
2. Would every code/curl sample run on a clean machine? Did you check each import, path, and signature?
3. Any number, compatibility claim, or performance assertion without a repo-verifiable source?
4. If this change touched a handler: did you update `docs/contracts/openapi.yaml`? Did you grep docs for the old endpoint path?
5. If this change touched a WebSocket event: did you update `docs/contracts/ws_events.md`?
6. Any docstring that just restates the signature? Any section that restates its heading?
7. Are all error response codes (400/401/403/404/409/422) documented in the OpenAPI spec for new endpoints?
8. If a feature-local OpenAPI contract covers the changed surface, does it exactly match the canonical paths, shapes, and responses, with no duplicate keys?
9. If this is an ADR: does it state the real decision? Are alternatives listed? Is it numbered sequentially?

If any answer is wrong, fix it before showing the user.

## Reporting format (review mode)

```
**Rule N violation** in `docs/path.md:<line or section>`
- Claim: <what the docs say>
- Reality: <what the code/schema actually has, with file:line>
- Fix: <one sentence>
```

Lead with Rule 1–4 findings (false claims), then drift, then substance. If a doc surface is clean, say so in one line.

## Severity guide

- **Must fix:** Rules 1–4 — false documentation is worse than no documentation; readers act on it
- **Should fix:** Rules 5–9 — drift debt and noise that buries the signal
- **Worth noting:** Rule 10 — navigation and polish

## What this skill does not do

- Review the code itself — use `clean-code-guard` for that.
- Generate documentation strategy or information architecture from scratch — it guards accuracy and substance, not scope decisions.
- Enforce a prose style guide — tone belongs to the project; truth belongs to this skill.
- Override the Halaqaty constitution — if the constitution says an endpoint must exist, this skill verifies the docs reflect it, but cannot waive the requirement.
