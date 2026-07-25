# AI Failure Modes — the unique value of this skill

This file catalogs 14 systematic ways LLMs produce bad code, each backed by published research or widely-documented engineering observations. Read this first if you are an AI agent applying this skill — these are the patterns most likely to enter your own output.

For each failure mode you get:
- **Pattern:** one-line description.
- **Source:** the research or post documenting it as systematic, not incidental.
- **Bad / Good:** short before-and-after.
- **Rule:** the imperative for your own self-check.

## Contents

- 1. Catch-all error handling that swallows failures
- 2. Defensive guards for impossible cases
- 3. Premature abstraction
- 4. Comment pollution
- 5. Code duplication instead of reuse
- 6. Hallucinated APIs and packages
- 7. Generic, intent-less naming
- 8. Long functions doing many things
- 9. Parameter explosion
- 10. Inconsistency with surrounding code
- 11. Dead code, unused imports, half-implementations
- 12. Declares success with mock fallbacks in production code
- 13. Plausible-but-wrong code
- 14. YAGNI violations through speculative configurability
- Cross-cutting observation
- Where this skill differs from generic clean-code rules

---

## 1. Catch-all error handling that swallows failures

**Pattern.** Wrapping operations in broad catch-all handlers or returning null/empty success on any caught error, hiding real bugs.

**Source.** Karpathy directly observed that LLMs are unusually afraid of exceptions. Reinforced by field reports on LLM error suppression. Root cause is the reward signal during training — propagating exceptions penalizes the model, so the model learns to suppress them.

**Bad:**
```text
getEmail(userId):
  attempt:
    user = userStore.get(userId)
    return user.email
  catch anyError:
    return null
```
Looks safe. In practice, a database outage is now indistinguishable from "user has no email."

**Good:**
```text
getEmail(userId):
  user = userStore.get(userId)  // storage errors propagate
  return user.email             // null only means the user has no email
```

**Go-specific:** Never `if err != nil { return nil }`. Always `return nil, fmt.Errorf("getting email: %w", err)`.

**Rule.** Catch only the specific error type you can recover from. Never use broad catch-all handling without a documented recovery path. Returning null/empty success from a handler is forbidden unless the function's contract says so.

---

## 2. Defensive guards for impossible cases

**Pattern.** Adding null checks, runtime type checks, or truthiness checks for conditions the type system or call graph already prevents.

**Source.** arXiv 2409.19182, "AI-Generated Code Considered Harmful"; HN discussion of defensive code overuse.

**Bad:**
```text
total(orderItems):
  if orderItems is null: return 0
  if orderItems is not a collection: return 0
  return sum(order.amount for each non-null order in orderItems)
```

**Good:**
```text
total(orderItems):
  return sum(order.amount for each order in orderItems)
```

**Go-specific:** If a parameter is `*pgxpool.Pool`, do not add `if pool == nil` unless nil-pool is an explicitly documented contract.
**Dart-specific:** Dart sound null safety means `if (x != null)` guards for non-nullable `x` are dead code — delete them.

**Rule.** Do not add null checks, runtime type checks, or truthiness checks for values whose type annotation or caller contract already excludes that case. Trust the contract.

---

## 3. Premature abstraction

**Pattern.** Factories, strategy classes, base classes, plugin hooks, dependency-injection scaffolding introduced before a second concrete user exists.

**Source.** Martin Fowler, "Patterns for Reducing Friction in AI-Assisted Development" — names "overeagerness (adding unrequested features)" as a documented AI pattern.

**Bad:**
```text
PaymentProcessor interface
  charge(amount)

CardPaymentProcessor implements PaymentProcessor
  charge(amount): return paymentProvider.createCharge(amount).id

PaymentProcessorFactory
  create(): return new CardPaymentProcessor()
```
There is exactly one payment processor. The abstract interface, the factory, and the indirection are pure ceremony.

**Good:**
```text
charge(amount):
  return paymentProvider.createCharge(amount).id
```

**Go-specific:** "Accept interfaces, return concrete types" is idiomatic Go — but only when a second implementation genuinely exists. One implementation = inline it.

**Rule.** Do not introduce an interface, abstract class, factory, registry, strategy, or plugin pattern unless two or more concrete implementations exist today or the spec explicitly requires extensibility. One implementation = inline it.

---

## 4. Comment pollution

**Pattern.** Line-by-line comments restating the code in English; step-number scaffolding comments left in; documentation comments that paraphrase the signature.

**Source.** HN thread #43929768 — *"The most common thing that makes agentic code ugly is the overuse of comments."* arXiv 2402.13013.

**Bad:**
```text
// Increment counter by one
counter += 1

// Step 3: return the result
return result
```

**Good:**
```text
counter += 1

// Reset counter at midnight UTC to align with billing periods.
if counter > daily_limit:
    counter = 0
```

**Go exception:** Exported symbols MUST have a `// FunctionName ...` doc comment — this is required by `golint`. Keep these; delete internal implementation comments that restate what the code does.
**Dart exception:** `///` doc comments on public APIs are required — keep them, but ensure they add contract (not paraphrase).

**Rule.** Comments explain *why*, never *what*. Strip restating-code comments and any leftover "Step N" scaffolding before finalizing.

---

## 5. Code duplication instead of reuse

**Pattern.** Inline copies of logic that already exists in a helper, instead of importing it.

**Source.** GitClear, AI Copilot Code Quality 2025 — 211M-LoC longitudinal analysis. Copy-pasted 5+ line blocks increased **8x** between 2021 and 2024.

**Go-specific:** Especially risky when copying handler registration, middleware chains, or `pgx` query functions — each has subtle differences that copy-paste hides.

**Rule.** Before writing a function, search the codebase for a similar existing one. If a block of ≥5 lines matches existing code in the repo, extract or call the existing function.

---

## 6. Hallucinated APIs and packages

**Pattern.** Imports, method names, or signatures that don't exist in the version of the library actually installed.

**Source.** Spracklen et al., USENIX Security '25 — average hallucination rate 19.6% across 16 models.

**Common Halaqaty hallucination targets:**
- `pgx` transaction API changes between v4 and v5 (e.g., `BeginTx` signatures differ)
- `livekit-server-sdk-go` method signatures (check godoc for the pinned version)
- `firebase_auth` Flutter package API differences between major versions
- `riverpod` Flutter package: 1.x vs 2.x API differences

**Rule.** Every import and external API call must be verified against the actual installed version — read `go.mod`/`pubspec.lock`, check the package source or godoc. Do not call a method based on what "should exist."

---

## 7. Generic, intent-less naming

**Pattern.** `data`, `result`, `item`, `temp`, `value`, `obj`, `info`, `helper`, `manager`, `utils`, `process_*`, `handle_*`, `do_*`.

**Source.** arXiv 2512.01141, "Neural Variable Name Repair".

**Rule.** Identifiers must reveal intent. Ban unqualified generic names. Qualified versions are fine: `rawCSVBytes`, `parsedCircleID`, `dedupByEmail`.

---

## 8. Long functions doing many things

**Pattern.** A single function mixing I/O, business logic, formatting, and side effects.

**Source.** GitClear 2025 — function size 142→267 LoC, cyclomatic complexity 4.2→8.1 in AI-assisted commits.

**Go-specific:** HTTP handler functions that do more than bind + validate + delegate to service are doing too many things. The handler body should be ≤15 lines.

**Rule.** A function does one thing. Hard caps: ~20 lines, ≤4 parameters, cyclomatic complexity ≤10.

---

## 9. Parameter explosion

**Pattern.** Functions taking 6+ positional or keyword args that should have been a typed config object.

**Go-specific:** Use functional options (`func(o *Options)`) for truly optional configuration; use a request struct for required input groups. Never add a boolean flag argument — split into two functions.

**Rule.** When a function reaches 5 parameters, stop and introduce a typed request/config object.

---

## 10. Inconsistency with surrounding code

**Pattern.** Introduces a new HTTP client when the repo has one, a new error type when an existing taxonomy exists, a new logging style.

**Go-specific:** Before adding a new `pgx` query pattern, middleware, or error type, read `backend/internal/` for the existing conventions.
**Dart-specific:** Before choosing a state management approach, read the neighboring feature modules.

**Rule.** Before writing in a file, read the file and at least one neighbor. Reuse the project's existing utilities.

---

## 11. Dead code, unused imports, half-implementations

**Pattern.** Imports never referenced, helper functions never called, branches never reachable, "just in case" exports.

**Source.** arXiv 2411.01414, "A Deep Dive Into LLM Code Generation Mistakes".

**Rule.** Before finalizing, run `golangci-lint` / `flutter analyze` for unused imports and symbols; remove them.

---

## 12. "Declares success" — mock fallbacks in production code

**Pattern.** Returning hardcoded success values, fixture data, or empty defaults instead of doing the actual work.

**Source.** Fowler, "Patterns for Reducing Friction"; claude-code issue #6984.

**Halaqaty-specific:** A handler that returns 200 without querying `circle_members` for authorization is a **security violation** (Constitution §IV). Return `http.StatusNotImplemented` with the standard error envelope if the work is not yet done.

**Rule.** Never return hardcoded "success" values or fixture data from a function the spec says should perform real work. If you cannot implement, fail explicitly.

---

## 13. Plausible-but-wrong code

**Pattern.** Code that compiles and reads correctly but encodes a slightly wrong formula, range, or null semantic — often lifted from a similar-but-different function.

**Source.** arXiv 2411.01414 — 4 of the 7 mistake categories are non-syntactic semantic mistakes.

**Halaqaty-specific:** Ayah numbers are 1-based and bounded per Surah. Queue positions are 1-based. Never assume zero-based indexing without checking the data model.

**Rule.** For any boundary, range, off-by-one, or null-semantic question, write the case enumeration in a comment first and verify each case before the code. Never copy a similar function and adapt — re-derive from the spec.

---

## 14. YAGNI violations — speculative configurability

**Pattern.** Config flags, env vars, optional parameters, and feature toggles for use cases that don't exist.

**Source.** Fowler's overeagerness pattern.

**Halaqaty-specific:** The ONLY approved feature flags are those defined in the constitution (`FEATURE_VIDEO_ENABLED`, `FEATURE_RECORDING_ENABLED`, `FEATURE_AI_TAJWEED_ENABLED`, `FEATURE_WEB_ENABLED`). Do not introduce new ones without an ADR and Karim's approval.

**Rule.** No optional parameter, config flag, env var, or feature toggle without a present-day caller.

---

## Cross-cutting observation

Eight of the 14 failure modes (1, 2, 3, 9, 12, 14, plus pieces of 8 and 11) trace to one root cause: **the model is biased toward emitting more code, more parameters, more guards, more abstractions** — anything but the minimum required by the spec. The cure is restraint, not knowledge. Before writing each line, ask: *does the spec require this, today?* If no, do not write it.

## Where this skill differs from generic clean-code rules

Sections in [naming-and-functions.md](naming-and-functions.md), [solid.md](solid.md), and [dry-kiss-yagni.md](dry-kiss-yagni.md) cover the classic principles. They are necessary but not sufficient — an LLM that "knows" SOLID can still produce code that fails for the reasons in this file. The 14 patterns above are the high-leverage check. Walk them before delivery.
