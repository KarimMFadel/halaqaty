# Naming and Functions — Clean Code Chapters 2 and 3

Source: Robert C. Martin, *Clean Code*. Sample chapters online at the Pearson PDF; chapter summaries at Vivek Khatri's Ch. 2 notes and Herberto Graça's Ch. 3 summary.

## Contents

- Meaningful names
  - N1. Intention-revealing
  - N2. No disinformation, no encodings
  - N3. Meaningful distinctions
  - N4. Searchable, pronounceable
  - N5. Class names are nouns, method names are verbs
  - N6. Banned generic names
- Functions
  - F1. Small. Then smaller.
  - F2. Do one thing
  - F3. One level of abstraction per function
  - F4. Step-down rule
  - F5. Few arguments
  - F6. No flag arguments
  - F7. No output arguments / Command-Query Separation
  - F8. No side effects in queries
  - F9. Prefer exceptions to return codes
  - F10. Duplication is the root evil
- Self-check for naming and functions

## Meaningful names

### N1. Intention-revealing

A name should tell you *why it exists, what it does, and how it's used*. If you need a comment to explain a name, the name is wrong.

**Bad:**
```go
d  // elapsed time in days
ts := []
fn(xs)
```

**Good:**
```go
elapsedDays
timestamps
filterOverdueCircles(circles)
```

### N2. No disinformation, no encodings

No Hungarian notation (`strName`, `iCount`). No interface-prefix `I` (`ICircleService`). No member prefix `m_`. No "List" suffix unless the type is actually a list.

**Go**: Unexported names may be short in tight scopes (`err`, `ctx`, `tx`, `r`, `w` in handlers). Package-level exported names must be descriptive.
**Dart**: Class names are PascalCase; variables and functions are camelCase. No prefixes.

### N3. Meaningful distinctions

Do not differentiate names by adding noise words. `CircleInfo`, `CircleData`, `Circle` — what's the difference? If the distinction is real, name the distinction.

### N4. Searchable, pronounceable

Single-letter names are acceptable inside short loop scope. Anywhere else they hurt grep. `maxQueueSize` is searchable; `7` is not.

### N5. Class names are nouns, method names are verbs

`Circle`, `QueueEntry`, `RecitationSession` — structs/classes are things. `CreateCircle`, `joinQueue`, `revokePublishPermission` — functions are actions.

### N6. Banned generic names

Without a qualifier, these names always violate intention-revealing:

- `data`, `data2`, `dataFinal`
- `result`, `resultFinal`
- `item`, `value`, `temp`, `obj`, `info`
- `helper`, `manager`, `utils`, `common`
- `handle_*`, `process_*`, `do_*` (when `*` is also generic)

Qualified versions are fine: `rawAyahText`, `parsedCircleID`, `dedupByEmail`.

---

## Functions

### F1. Small. Then smaller.

Target ≤20 lines. **Go HTTP handlers** should be ≤15 lines — bind request, validate, call service, write response. No business logic in handlers. **Dart `build()` methods** that exceed ~30 lines should extract sub-widgets.

### F2. Do one thing

A function does one thing when you cannot extract another function from it with a name that is not a restatement of its body.

### F3. One level of abstraction per function

Do not put an HTTP call, a SQL query, a business rule, and JSON serialization in the same function — those are four levels.

**Good Go handler:**
```go
func (h *CircleHandler) Create(w http.ResponseWriter, r *http.Request) {
    req, err := decodeCreateCircleRequest(r)
    if err != nil {
        writeError(w, http.StatusBadRequest, "ERR_INVALID_INPUT", err.Error())
        return
    }
    circle, err := h.svc.CreateCircle(r.Context(), req)
    if err != nil {
        writeServiceError(w, err)
        return
    }
    writeJSON(w, http.StatusCreated, circle)
}
```

### F4. Step-down rule

Callers above callees. High-level functions first, then the helpers they call.

### F5. Few arguments

Zero is best. One is fine. Two is OK. Three "should be avoided." Four or more "requires very special justification."

**Go idiomatic override**: use functional options (`func(o *Options)`) for optional configuration — this is idiomatic Go and preferred over a boolean or a growing parameter list.

### F6. No flag arguments

A boolean parameter that switches behavior is always wrong. Split into two functions.

**Bad Go:**
```go
func GenerateLiveKitToken(circleID uuid.UUID, userID string, canPublish bool) string
```

**Good Go:**
```go
func GenerateReciterToken(circleID uuid.UUID, userID string) string
func GenerateListenerToken(circleID uuid.UUID, userID string) string
```

### F7. No output arguments / Command-Query Separation

A function either returns a value (query) or has a side effect (command). Never both.

**Go exception**: `(value, error)` return tuples are idiomatic and correct — this is NOT a CQS violation. The error return is part of the query contract.

### F8. No side effects in queries

A getter-style or predicate-style function must not mutate state.

### F9. Prefer exceptions to return codes

**Go**: Return errors as values — `(result, error)` — not status codes embedded in a single return. The error propagates; status codes get ignored.
**Dart**: Throw typed exceptions or return `Either<Failure, Success>` from use-cases (whichever pattern the project uses consistently).

### F10. Duplication is the root evil

If two functions share a non-trivial block, extract it. But — see [dry-kiss-yagni.md](dry-kiss-yagni.md) for when this is wrong (Sandi Metz's "wrong abstraction" caveat).

---

## Self-check for naming and functions

Before you ship code:

1. Do all names answer "what does this represent" without a comment?
2. Are functions ≤20 lines? (Handlers ≤15? Dart `build()` methods reasonably sized?)
3. Are functions doing one thing?
4. Are mixed abstraction levels eliminated?
5. Are there any functions with >4 parameters? Extract a config/request object (or use functional options in Go).
6. Are there boolean flag arguments? Split.
7. Do any functions both return a value *and* mutate state in a way callers depend on? Split. (Exception: Go `(value, error)` is fine.)
