# DRY, KISS, YAGNI

Three short principles. Often confused. Often applied wrong by AI agents (and humans).

## Contents

- DRY: do not duplicate knowledge
- KISS: keep complexity low and local
- YAGNI: avoid speculative configurability
- Ranked list: where AI agents over-engineer
- Self-check for DRY, KISS, YAGNI

---

## DRY — Don't Repeat Yourself

**Definition (Hunt & Thomas, *The Pragmatic Programmer*, verbatim).** *"Every piece of knowledge must have a single, unambiguous, authoritative representation within a system."*

Source: pragprog official DRY excerpt PDF; Wikipedia summary; O'Reilly *97 Things Every Programmer Should Know*, Ch. 30.

### The misreading

*"Don't have any duplicate code."* No. Hunt and Thomas frame DRY as duplication "of knowledge, of intent... expressing the same thing in two different places, possibly in two totally different ways." Two functions that **look alike but encode different rules** are not a DRY violation. **One rule** expressed in code + database schema + documentation **is**.

### Smells worth flagging

These are textual signals that *probably* indicate knowledge duplication, but verify the underlying meaning before refactoring:

- Identical token sequence of ≥5 non-trivial lines appearing in ≥2 functions.
- The same regex, SQL fragment, or URL literal repeated in ≥3 sites.
- The same magic number or string repeated ≥3 times outside a constants module.
- The same validation branch duplicated across siblings of one module.

### The Rule of 3 — wait for the third occurrence

Don't extract an abstraction the first time you see duplication. Don't extract on the second. Wait for the third — by then you have enough signal about the *real* shape of the shared knowledge to abstract correctly.

### The Sandi Metz corollary — wrong abstraction is worse than duplication

From Sandi Metz, "The Wrong Abstraction" (Jan 2016): *"duplication is far cheaper than the wrong abstraction."*

If an abstraction has accumulated per-caller branches and special cases, it is the wrong abstraction. The remedy:

1. Re-inline the abstraction back into each caller.
2. Delete the per-caller dead branches.
3. Live with honest duplication for a while.
4. Re-abstract only when the *real* shared knowledge becomes obvious.

**Rule.** Do not introduce an abstraction to eliminate three lines of duplication unless you can name the underlying *knowledge* the lines represent. If you can't name it, leave the duplication.

---

## KISS — Keep It Simple, Stupid

**Origin.** Coined by Clarence "Kelly" Johnson at Lockheed's Skunk Works. Original phrasing: *"Keep it simple stupid"* — the "stupid" refers to the mismatch between break-conditions and repair sophistication, not to the engineer.

### Operationalizing KISS for code review

- **Cyclomatic complexity ≤10 per function.** McCabe's 1976 metric — still useful as a structural floor. 11–20 is moderate risk; >20 is high risk.
- **Nesting depth ≤5.**
- **Go**: `golangci-lint` with `cyclop` or `gocyclo` enforces this — the CI gate is the floor, not the ceiling.
- **Dart**: `dart analyze` catches some complexity issues; use judgment for nesting depth.

### Self-check

When you see a function exceed cyclo 10 or nest depth 5, refactor *before* finishing — not "later." Extract a helper, replace nested `if/else` with early returns or a lookup table.

---

## YAGNI — You Aren't Gonna Need It

**Canonical reference.** Martin Fowler, *bliki: Yagni* (May 2015). *"Capabilities we presume our software needs in the future should not be built now because 'you aren't gonna need it.'"*

### Fowler's four cost categories

When you build a presumptive feature, you pay:

1. **Cost of build.** Analysis, coding, testing of a feature that ends up unused.
2. **Cost of delay.** Opportunity cost — revenue-generating work you didn't do instead.
3. **Cost of carry.** Added complexity makes every future modification and debug slower.
4. **Cost of repair.** When the presumed feature turns out to be wrong, you pay to rip it out plus the technical debt accumulated against it.

### Halaqaty-specific YAGNI alignment

Constitution §VII is YAGNI in explicit form:
- Scale target: **50 concurrent users, ≤10 simultaneous live sessions** in the first 6 months.
- Infrastructure: **single Docker Compose server**. No Kubernetes. No Redis. No multi-region.
- The ONLY approved feature flags are: `FEATURE_VIDEO_ENABLED`, `FEATURE_RECORDING_ENABLED`, `FEATURE_AI_TAJWEED_ENABLED`, `FEATURE_WEB_ENABLED`. Adding others requires an ADR.

### AI-specific YAGNI traps

1. **Config flags / env vars nobody asked for.** `enable_x_v2`, `legacy_mode`, toggles for a single code path.
2. **Plugin / strategy systems for 2 known cases.** Registry + base class + 2 subclasses where a direct conditional is shorter and clearer.
3. **Generic helpers with one caller.** `normalizeAnything(value, strict=false, mode="default")` invoked from exactly one site.
4. **Optional parameters never passed.** Delete them until a real caller exists.
5. **Speculative async / batching / caching.** Async wrappers and queues where current load is single-digit RPS.
6. **Premature interfaces/protocols with one implementation.** Inline until you have a second implementation.

### Self-check

For every parameter, class, file, or abstraction you introduce, answer: *who calls this today?* If the answer is "nobody yet," delete it.

---

## Ranked list — where AI agents over-engineer

By frequency observed:

1. **Premature interfaces/protocols** with one implementation.
2. **Factory classes for trivial constructors** — `CircleFactory.create(...)` wrapping `Circle{...}`.
3. **Try/catch wrappers that change nothing** — adds lines, hides tracebacks.
4. **Speculative config surface** — settings objects with 15 fields where 3 are read.
5. **Plugin / registry scaffolding for two cases.**
6. **Re-implementing standard libraries** — custom retry loops, custom UUID types when `github.com/google/uuid` exists.
7. **Excessive layering** (Handler → Service → Manager → Repository) for trivial CRUD — four files to read one row.
8. **Wrapping libraries "to make them swappable"** — thin pass-through adapters around `pgx` or Firebase that you will never swap.

---

## Self-check for DRY, KISS, YAGNI

Before you ship code:

1. (DRY) Did you eliminate duplication of *knowledge*, or just duplication of *text*? Can you name the underlying rule?
2. (DRY/Metz) If you introduced an abstraction, are there at least two callers today whose code is structurally identical? Or is the abstraction speculative?
3. (KISS) Any function over cyclomatic 10 or nest depth 5?
4. (YAGNI) Any optional parameter, config flag, env var, interface, factory, or base class without a caller using it today?
5. (YAGNI) Did you wrap a library to "make it swappable"? Delete the wrapper.
6. (Halaqaty) Did you introduce a new feature flag not in the approved list? If yes, this requires an ADR.
