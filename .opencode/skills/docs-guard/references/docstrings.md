# Docs Guard — Docstring, PHPDoc, and JSDoc Rules

In-code documentation has one extra constraint the other surfaces lack: it sits next to the truth. There is no excuse for a docstring that disagrees with the signature three lines below it.

## Contents

- When a docstring is justified
- The paraphrase test
- What a good docstring contains
- Tag accuracy (Go doc comments / Dart doc comments)
- Generated-docs hygiene

## When a docstring is justified

- Public API surface: always — it feeds IDEs, generated references, and agents.
- Internal helpers: only when the contract is not expressible in the signature (units, invariants, side effects, "why"). An internal one-liner with an intention-revealing name usually needs nothing — and clean-code-guard's comment rules apply.

## The paraphrase test

Delete any docstring whose entire information content is recoverable from the signature:

**Go — fails the test:**
```go
// GetCircleByID gets the circle by ID.
func GetCircleByID(ctx context.Context, id uuid.UUID) (*Circle, error) { /* … */ }
```

**Go — passes the test:**
```go
// GetCircleByID returns the circle for the given ID.
// Returns ErrCircleNotFound if no circle exists for that ID.
// Returns ErrCircleArchived if the circle has been archived.
func GetCircleByID(ctx context.Context, id uuid.UUID) (*Circle, error) { /* … */ }
```

**Dart — fails the test:**
```dart
/// Gets the user profile.
Future<UserProfile> getUserProfile(String userId) async { /* … */ }
```

**Dart — passes the test:**
```dart
/// Returns the user profile for [userId].
/// Throws [UserNotFoundException] if the user does not exist.
/// Throws [PermissionDeniedException] if the caller lacks read access.
Future<UserProfile> getUserProfile(String userId) async { /* … */ }
```

AI generators emit paraphrase docstrings by the thousand — they are comment pollution wearing a suit. Either say something the signature cannot, or say nothing.

## What a good docstring contains

The contract the types cannot express:

- Units and ranges (`timeout` in seconds? milliseconds? what happens at 0?)
- Error behavior: which errors on failure, and when (verify the error return/throw sites)
- Side effects: writes, cache invalidation, events fired, global state touched
- Null/empty semantics: what a nil pointer/empty slice means here
- Ordering, idempotency, concurrency guarantees when callers depend on them
- The "why" for surprising design

## Tag accuracy (Go doc comments / Dart doc comments)

**Go:**
- Doc comment begins with `// FunctionName` — this is the `godoc` convention.
- Exported types, functions, constants, and variables must have a doc comment.
- Document error return values with specific sentinel errors or conditions.
- Use `// Deprecated: use NewFunction instead.` for deprecated symbols.

**Dart:**
- Use `///` triple-slash doc comments for all public API.
- `@param` / `@returns` are not idiomatic Dart — use prose with `[paramName]` references.
- `@throws` equivalent: describe exceptions in prose: "Throws [CircleNotFoundException] if..."
- Use `@deprecated` annotation on the element; document the replacement in the doc comment.

## Generated-docs hygiene

When docstrings feed a generated reference (`godoc`, `dart doc`):

- A wrong docstring becomes a published wrong reference page — Rule 1 severity applies as if it were the README.
- Check that examples inside docstrings obey [code-samples.md](code-samples.md) — they are the least-reviewed samples in any codebase.
- Go `Example*` functions in `_test.go` files are runnable by `godoc` — verify they compile and produce the documented output.
