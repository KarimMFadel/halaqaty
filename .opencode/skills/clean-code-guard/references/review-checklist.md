# Review-Mode Checklist

When the user asks you to **review, audit, critique, or rate code** (rather than write it), follow this structured walk-through. Do not edit the code unless asked. Produce a findings report.

## Contents

- Output format
- Pre-flight: is this a refactor or a rewrite?
- Walk order
  - Section A: naming and functions
  - Section B: comments and formatting
  - Section C: SOLID
  - Section D: DRY, KISS, YAGNI
  - Section E: AI failure modes
  - Section F: Halaqaty constitutional compliance
- What to do with each finding
- When the review is contested
- What this review does not do

## Output format

```
# Code review: <file or scope>

## Summary
<2–3 sentence verdict: ship / needs work / rewrite>

## Critical findings
<must-fix before merge>
- **<short title>** — `<file>:<line>`
  Evidence: <quoted code or behavior>
  Principle violated: <e.g., LSP, catch-all error swallowing, security invariant>
  Suggested fix: <concrete>

## Important findings
<should fix but not blocking>
- ...

## Nits
<style, naming, minor structure>
- ...

## What's good
<at least 2–3 specific positives — naming, structure, test coverage>

## Self-check coverage
- [ ] Walked Section A (naming & functions)
- [ ] Walked Section B (comments & formatting)
- [ ] Walked Section C (SOLID)
- [ ] Walked Section D (DRY/KISS/YAGNI)
- [ ] Walked Section E (AI failure modes)
- [ ] Walked Section F (constitutional compliance)
```

Severity:
- **Critical** — security, correctness, data loss, swallowed exceptions, hardcoded "success" returns, constitutional violations.
- **Important** — design defects with maintenance cost: SOLID violations, premature abstractions, parameter explosion, generic naming.
- **Nit** — style, minor naming improvements, missing doc comments on public APIs.

## Pre-flight: is this a refactor or a rewrite?

Before walking the sections, classify the review:

- **Refactor review:** the user wants the code to be cleaner, not different. **Observable behavior must not change** — same inputs, same outputs, same exceptions, same side effects. If you'd suggest a change that alters behavior, mark it as a *separate finding* labelled "Behavior change — confirm with author" and do not bundle it with refactor recommendations.
- **Code-review for correctness:** the user wants you to find bugs. Behavior changes are in scope. Flag them at Critical severity if they affect the contract.

If you can't tell which one the user wants, ask before writing the review.

## Walk order

### Section A — naming and functions

Pull [naming-and-functions.md](naming-and-functions.md) if you need source citations.

1. Scan all identifiers. Flag generic ones: `data`, `result`, `item`, `temp`, `value`, `obj`, `info`, `helper`, `manager`, `utils`, `handle_*`, `process_*`, `do_*` without qualifier.
2. For each function: lines ≤20? params ≤4? one thing? one level of abstraction? Flag violations.
3. Flag boolean flag arguments.
4. Flag functions that both return value *and* mutate observable state ambiguously (CQS violation). (Note: Go `(value, error)` is NOT a violation.)
5. Flag getter-style or predicate-style functions that mutate.

### Section B — comments and formatting

Pull [comments-and-formatting.md](comments-and-formatting.md) if needed.

1. Flag every comment that paraphrases the code below it.
2. Flag commented-out code blocks.
3. Flag step-number, "First...", or "Then..." scaffolding comments.
4. Flag docstrings that restate the signature with no contract.
5. Flag style inconsistencies with the surrounding file (casing, import order, error handling style).
6. **Go-specific**: flag missing doc comments on exported symbols (Critical-level in a library package).

### Section C — SOLID

Pull [solid.md](solid.md) if needed.

1. (SRP) Any struct/class with methods serving two unrelated stakeholder groups?
2. (OCP) Conditional or switch dispatch on a type tag that grew with the codebase?
3. (LSP) Any type with an unimplemented/unsupported-operation failure, strengthened preconditions, or weakened postconditions?
4. (ISP) Any interface where the concrete client uses only a subset of methods?
5. (DIP) High-level module importing a concrete from a low-level module? **Go-specific**: abstractions should live in the consuming package, not the `postgres`/`firebase` packages.

### Section D — DRY, KISS, YAGNI

Pull [dry-kiss-yagni.md](dry-kiss-yagni.md) if needed.

1. (DRY) ≥5-line duplicated blocks. Confirm it's knowledge duplication before recommending extraction.
2. (DRY/Metz) Wrong abstractions: per-caller branches and special-case flags accumulating in a shared function.
3. (KISS) Any function with cyclomatic >10 or nesting >5?
4. (YAGNI) Optional parameters never called, config flags with one path, abstractions with one implementation.
5. **Halaqaty-specific (YAGNI)**: any new feature flag not in the approved list (`FEATURE_VIDEO_ENABLED`, `FEATURE_RECORDING_ENABLED`, `FEATURE_AI_TAJWEED_ENABLED`, `FEATURE_WEB_ENABLED`)? This is a constitutional violation — Critical.

### Section E — AI failure modes

Pull [ai-failure-modes.md](ai-failure-modes.md) for every check here.

1. Any catch-all error handler that swallows without recovery? **Critical.**
2. Any defensive guards for types/values the system already excludes (including Dart non-nullable type guards)?
3. Any premature abstraction — interface or factory with one implementation?
4. Any comment pollution — line-by-line restating, step-number scaffolding?
5. Any duplication of logic that already exists in `backend/internal/` or a neighboring mobile module?
6. Any imports or library methods you should verify exist in the installed version? (`go.mod`/`pubspec.lock`)
7. Any dead code, unused imports, unreachable branches, half-implementations?
8. **Any hardcoded "success" returns, mock fixtures, or fake values in production code?** **Critical + security issue.**
9. Any code copy-pasted from a similar function (off-by-one, wrong null semantic)?
10. Any Ayah/queue position indexing that assumes zero-based when the data model is 1-based?

### Section F — Halaqaty constitutional compliance

Cross-check against `.specify/memory/constitution.md`:

1. **Security invariants (§IV)**: LiveKit tokens generated server-only? Firebase JWT not used for authorization? Roles checked against `circle_members`? `CanPublish` scoped to active reciter only?
2. **Parameterized queries (§IV.7)**: No string-interpolated SQL anywhere.
3. **Audio configuration (§V)**: Noise suppression, AGC, echo cancellation disabled? `CanPublishVideo` always false?
4. **Recording flag (§IV.5)**: `FEATURE_RECORDING_ENABLED` still false? Nothing enables recording implicitly?
5. **API contract (§III)**: Any new endpoint not in `docs/contracts/openapi.yaml`? Any response shape that diverges from the contract?
6. **MVP scope (§VII)**: Any complexity, infrastructure, or feature beyond current sprint scope?

## What to do with each finding

For each item flagged:
1. Quote the offending code (file + line range).
2. Name the principle or AI failure mode in `references/`.
3. Propose a concrete fix — code if the change is small, prose if it's structural.
4. Assign severity (Critical / Important / Nit).

## When the review is contested

If the user pushes back on a finding, cite the source from the relevant `references/` file. If the user has a context-specific reason to override a clean-code rule (e.g., a constructor genuinely needs 8 params for a config DTO), document the exception in a code comment. **Constitutional violations cannot be overridden here — they require an ADR and Karim's approval.**

## What this review does not do

- Run linters or formatters. That's `golangci-lint` and `dart format`.
- Execute the code or run tests.
- Override the Halaqaty constitution.
