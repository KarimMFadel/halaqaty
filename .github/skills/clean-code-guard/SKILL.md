---
name: clean-code-guard
description: Review generated or changed production code before it ships, using Clean Code, SOLID, DRY, KISS, YAGNI, and LLM-specific failure-mode checks. Adapted for Halaqaty's Go backend and Flutter mobile stack. Best used reactively after an agent writes, edits, refactors, or fixes code, before presenting, committing, or merging the result. Use when the user asks "review this PR", "is this safe to merge?", "make this cleaner", "audit this code", "refactor this", "fix this bug", or after a coding agent produced implementation code. DO NOT USE for factual/conceptual questions, CI/tooling config, git workflow, running/debugging tests, pure architecture discussion, prose writing, data analysis, or test-code review (use test-guard).
---

# clean-code-guard

You are reviewing generated or changed code before it ships. Apply the rules below as a guard pass after the first implementation pass. If the user explicitly invokes this skill before writing code, use the same rules while writing and still run the self-check before delivery.

## Compatibility

This skill requires no MCP server, network access, API key, or shell command. It works in any runtime that supports `SKILL.md`. It does not replace project linters, formatters, type checkers, or test runners — use `golangci-lint`, `gofmt`, `flutter analyze`, and `dart format` for mechanical verification; use this skill for the judgment layer around code quality.

## How to use this skill

**Guard-pass mode** (recommended): after code has been generated, edited, refactored, or fixed, check the diff or target files against the *Always-applied imperatives* below. Fix violations before presenting, committing, or merging.

**Live mode** (explicit): when invoked before a risky code edit, apply the same imperatives while writing, then run the *Self-check before delivery* checklist.

**Review mode** (triggered by "review", "audit", "critique", or "rate"): walk every rule below against the target files and produce a structured findings report with file:line evidence. Do not edit code in review mode unless asked.

## Halaqaty project context

Before applying any rule, read:
- `.specify/memory/constitution.md` — the project's non-negotiable rules (stack, security, YAGNI, test-first)
- The file being edited and one neighbor file, to match existing style

Key constitutional constraints that interact with this skill:
- **YAGNI is mandatory** (§VII): no speculative abstractions, feature flags, or infrastructure beyond current MVP scope
- **Parameterized queries only** (§IV.7): never string-interpolate SQL — `pgx` named/positional parameters exclusively
- **No hardcoded success** (§IV.6): input is always validated server-side; never return a successful response without real validation
- **Security invariants** (§IV): LiveKit tokens server-only, Firebase JWT never grants authorization, roles per-circle from `circle_members`

When a generic clean-code rule conflicts with a constitutional rule, the constitution wins. Document the override in a comment.

## Always-applied imperatives

### Functions and names

1. **Names reveal intent.** Never use `data`, `data2`, `result`, `result_final`, `item`, `temp`, `value`, `obj`, `info`, `helper`, `manager`, `utils`, or `handle_*`/`process_*`/`do_*` without a qualifier. A name must answer *why it exists and what it does*.
   - **Go**: unexported names may be short in very tight scopes (`err`, `ctx`, `tx` are idiomatic) but anything at package scope must be descriptive.
   - **Dart**: avoid generic names like `data`, `response`, `model` as top-level identifiers.

2. **Functions stay small.** Target ≤20 lines, one level of abstraction, one thing. If you can extract a function with a name that doesn't restate the body, the parent was doing more than one thing.
   - **Go**: HTTP handler functions should delegate to service methods immediately; keep handler bodies ≤15 lines of wiring.
   - **Dart**: widget `build()` methods that exceed ~30 lines should extract sub-widgets or builders.

3. **Argument ceiling.** At five arguments, stop and introduce a config struct or options type — never a boolean flag argument; split into two functions instead.
   - **Go idiomatic override**: use the *functional options pattern* (`func(o *Options)`) when a function has optional configuration — this is idiomatic Go and preferred over a config struct when options are optional. Always validate the `Options` struct before use.
   - **Dart**: use named parameters with `required` for mandatory arguments; optional named parameters for defaults.

4. **No output arguments.** A function either returns a value (query) or has a side effect (command), never both.
   - **Go exception**: functions that return `(value, error)` are idiomatic and correct — this is not a CQS violation. The error return is part of the query contract.

### Comments and structure

5. **Comments explain *why*, never *what*.** Delete any comment that paraphrases the line below it. Delete step-number scaffolding comments. Delete commented-out code — version control exists.
   - **Go**: exported symbols require a doc comment (`// FunctionName does X`) — this is a `golint` requirement, not a violation of this rule. Keep these; delete internal implementation comments that restate what the code does.
   - **Dart**: `///` doc comments on public APIs are required by `dart analyze` — keep them descriptive, not paraphrasing.

6. **Match the file's existing style.** Read the file you're editing and at least one neighbor before writing. Mirror the casing, import order, error handling, logging, and HTTP/DB client choices. Do not introduce a second pattern.
   - **Go**: imports must follow the three-group convention (stdlib / external / internal). Use `goimports` ordering.
   - **Dart**: follow the project's existing state management pattern (check neighboring files before choosing a new approach).

### SOLID

7. **One actor per module.** A struct/class should be answerable to one stakeholder group. If two unrelated subsystems both reach into the same struct, split it.

8. **Extension via new code, not edits.** If adding a new variant requires another type-tag branch in an existing function, refactor to a registry, strategy, or interface dispatch first.
   - **Go**: use interface dispatch. Avoid `switch v := x.(type)` chains that grow with new types.

9. **No type refuses its interface's contract.** Never implement an interface method to return "not implemented" or "unsupported operation." If you need to do that, the interface boundary is wrong.
   - **Go**: never implement an `io.Reader` or custom interface with `panic("not implemented")` in production code. Use the `errors.ErrUnsupported` sentinel only when the contract explicitly allows it.

10. **Abstractions live with the client, not the implementation.** When you introduce an interface, put it in the package that consumes it, not next to the concrete type.
    - **Go**: this is idiomatic Go ("accept interfaces, return concrete types"). A `Repository` interface belongs in the service package, not the `postgres` package.

### DRY, KISS, YAGNI

11. **Delete duplicated *knowledge*, not duplicated *text*.** Two functions that look alike but encode different rules are not a DRY violation. One rule expressed in code + docs + schema is.

12. **The wrong abstraction is worse than duplication.** If an abstraction has accumulated branches for each caller's special case, re-inline it back into callers, then delete the dead branches before re-abstracting.

13. **Complexity ceiling: cyclomatic ≤10, nesting depth ≤5.** Refactor before exceeding.
    - **Go**: `golangci-lint` with `cyclop` or `gocyclo` enforces this automatically — align with the CI gate.

14. **No speculative anything.** No optional parameter, config flag, env var, feature toggle, interface, factory, or base class without a present-day caller.
    - **Constitutional alignment**: this maps directly to Constitution §VII MVP Scope Discipline. The feature flags defined in the constitution (`FEATURE_VIDEO_ENABLED`, `FEATURE_RECORDING_ENABLED`, etc.) are the *only* approved feature flags. Do not introduce new ones without an ADR.
    - If you find yourself adding `enable_*`, `use_*_v2`, or `*_mode`, delete it and ship the concrete behavior.

### AI-specific guardrails

15. **Never swallow errors with broad catch-all handling.** Catch only the specific error type you can recover from. If you cannot recover, propagate.
    - **Go**: errors are values. Never do `if err != nil { return nil }` — always return the error or wrap it: `return nil, fmt.Errorf("creating circle: %w", err)`. Use `errors.Is` / `errors.As` for specific error handling, never string comparison. Sentinel errors (`var ErrCircleNotFound = errors.New(...)`) belong in the domain layer.
    - **Dart**: never catch `Exception` broadly in business logic. Use typed catches or `on SpecificException`.

16. **No defensive guards for impossible cases.** Do not add null checks or runtime type checks for values whose declared type or caller contract already excludes that case.
    - **Go**: if a function parameter is typed as `*pgxpool.Pool`, do not add a `if pool == nil` guard inside unless the function explicitly documents nil-pool as valid input.
    - **Dart**: Dart's sound null safety already excludes null for non-nullable types. Do not add `if (x != null)` guards for `x` declared as non-nullable — this is dead code.

17. **Verify every import and external call.** Before calling a method on a library, confirm it exists in the installed version.
    - **Go**: check `go.mod` + `go.sum` for the module version. Read the actual package source or godoc before generating a method call. Common hallucination targets: `pgx` transaction API changes between v4 and v5, `livekit-server-sdk-go` method signatures.
    - **Dart**: check `pubspec.lock` for the pinned version. Common hallucination targets: `firebase_auth` API differences between major versions, `riverpod` API changes between 1.x and 2.x.

18. **No hardcoded "success" returns or mock fixtures in production code.** Never return `{"status": "ok"}` from a function whose spec says it does real work.
    - **Constitutional alignment**: Constitution §IV.6 mandates server-side validation. A handler that returns 200 without querying `circle_members` for authorization is a security violation, not just a code smell.
    - **Go**: if a handler is not yet implemented, return `http.StatusNotImplemented` with the standard error envelope `{"error": {"code": "ERR_NOT_IMPLEMENTED", "message": "..."}}`. Never return a success status without real work.

19. **Re-derive, do not copy from similar.** When tempted to copy a function and modify it, stop. Re-derive from the spec. Off-by-one and wrong-null-semantic bugs almost always enter through copy-from-similar.
    - **Go**: this is especially risky when copying handler registration, middleware chains, or `pgx` query functions. Each has subtle differences that copy-paste hides.

20. **Enumerate boundary cases before writing them.** For any range, off-by-one, null/empty/one/many, or unicode/byte boundary, write the case list in a comment first. Cover each case in code before moving on.
    - **Halaqaty-specific**: Quran Ayah numbers are 1-based and bounded per Surah. Queue positions are 1-based. Never assume zero-based indexing without checking the data model.

21. **Strip dead code before delivery.** Run `golangci-lint` / `flutter analyze` for unused imports, symbols, unreachable branches, and "just in case" exports. Remove them.
    - **Go**: `make lint` (from repo root or `backend/`) runs `golangci-lint run ./...`
    - **Dart**: `make lint` (from repo root) or `make analyze` (from `mobile/`) runs `flutter analyze`

22. **Read before write.** Before writing in an unfamiliar file, read the file being edited, one neighbor, and `.specify/memory/constitution.md`. Use the project's existing helpers, error types, and logging patterns.
    - **Go**: check `backend/internal/` for existing domain errors, middleware helpers, and `pgx` query patterns before inventing new ones.
    - **Dart**: check neighboring feature modules for the established state management and navigation patterns.

23. **Centralize repeated literals.** If a protocol or domain literal appears in more than one production file (e.g., `Content-Type`, `application/json`, auth scheme names, shared error messages), move it to a dedicated constants module and reuse it.
    - **Go (Halaqaty)**: use `backend/internal/platform/httpconst/` for HTTP/auth header names, content types, and reusable API error message constants.

24. **No inline runtime SQL in repository methods.** Repository functions should orchestrate query execution and row mapping only. SQL text belongs in dedicated `*_queries.go` files (or generated query layer) inside the same package.
    - **Go (Halaqaty)**: keep `const ...Query` blocks in a separate query-definition file; avoid embedding multiline SQL literals directly in method bodies.

### Refactoring discipline

25. **Preserve observable behavior when refactoring.** When asked to clean up, simplify, or refactor, do not change the contract. If you spot a bug while refactoring, flag it separately and ask before changing it. Refactoring and bug fixes are two operations — never bundle them.

## Self-check before delivery

1. Walk imperatives 1–25 against your diff. Fix every violation.
2. For new functions: lines ≤20? params ≤4 (or functional options)? complexity ≤10? names reveal intent?
3. For new comments: does this explain *why*? If it explains *what*, delete it. (Exception: exported Go/Dart doc comments.)
4. For new error handling in Go: does the error propagate with `fmt.Errorf("…: %w", err)`? Is the caught type specific?
5. For new abstractions (interface, factory, base class): is there a second concrete consumer *today*? If no, inline it.
6. Did you read the file edited and at least one neighbor? Did your style match?
7. Any hardcoded "ok" returns without real authorization/validation? If yes — this is a security issue, not just a code smell.
8. If this is a refactor: did you change observable behavior? If yes, split it out and ask the user.
9. **Go-specific**: does every new exported type/function have a doc comment? Does every error get wrapped with context?
10. **Dart-specific**: are all non-nullable types truly non-nullable at the type level (not defended at runtime)?
11. Constitutional compliance: does this change respect the stack, security invariants, and MVP scope?

If you cannot answer yes to every check, fix before shipping.

## What this skill does not do

- Run linters or static analysis — use `make lint` (root), `make lint` (backend), or `make analyze` (mobile).
- Enforce formatter preferences — use `gofmt` (`go build` runs it) and `dart format`.
- Replace tests — use `test-guard` for test quality; run tests with `make test` or `make test-integration`.
- Override the Halaqaty constitution — the constitution is the final authority.
