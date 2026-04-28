# ADR-003: Flutter State Management — Riverpod 2.x

**Status:** Accepted  
**Date:** 2026-04-26  
**Deciders:** Karim (product owner)

---

## Context

The Halaqaty Flutter app needs a state management solution that handles:
- Authentication state shared across the entire widget tree
- Per-screen local UI state (loading, error, form fields)
- Real-time session state (queue position, live participant list) updated via WebSocket streams
- Navigation with deep-link support (circle invites, session join links)

The solution must be testable, predictable, and compatible with Copilot-generated code that follows well-documented patterns.

---

## Decision

We will use **Riverpod 2.x** with `riverpod_generator` for code generation and `go_router` for routing.

**Key configuration decisions:**

1. **`riverpod_generator`** — All providers defined using the `@riverpod` annotation. Build runner generates the `*Provider` and `*Ref` boilerplate. No hand-written provider classes.
2. **Provider types by use case:**
   - `@Riverpod(keepAlive: true)` — Auth state, user profile (app lifetime)
   - `@riverpod` (auto-dispose) — Screen-scoped state (queue, session, forms)
   - `StreamProvider` — WebSocket-backed real-time state (live queue updates)
   - `AsyncNotifier` — Server-dependent state with loading/error/data lifecycle
3. **`go_router`** with `riverpod` integration for auth-guarded routes — route redirects watch the auth provider directly.
4. **Testing** — `ProviderContainer` in unit tests; `ProviderScope` with overrides in widget tests. No mocking frameworks needed.

---

## Consequences

**Positive:**
- Code generation eliminates boilerplate that Copilot would otherwise invent inconsistently.
- `AsyncNotifier` gives us a structured way to handle loading/error states without a custom state wrapper.
- `StreamProvider` maps directly to our WebSocket streams — real-time queue updates are just `ref.watch(queueProvider)`.
- Compile-time provider reference safety: typos in provider names are caught at build time, not at runtime.
- Excellent Copilot training signal — Riverpod 2.x is well-represented in the Flutter community and GitHub corpus.

**Negative:**
- Code generation requires `build_runner` to run after any `@riverpod` annotation change. Adds a step to the development loop (mitigated by `dart run build_runner watch`).
- The generated files (`*.g.dart`) must be committed or gitignored consistently. **Decision: commit them** to avoid requiring a build runner run after every `git clone`.

---

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| **BLoC / flutter_bloc** | Explicit, well-tested, but verbose. Every feature requires Bloc, Event, and State classes. This pattern produces a high volume of boilerplate that Copilot generates inconsistently without explicit instruction per file. |
| **Provider (original)** | The direct predecessor to Riverpod; effectively deprecated for new projects. Lacks compile-time safety and `AsyncNotifier`. |
| **GetX** | All-in-one framework (state + navigation + DI + HTTP). Deep coupling makes unit testing difficult without a full GetX environment. Not recommended for apps where testability is a quality gate. |
| **`setState` / `InheritedWidget`** | Appropriate for very simple apps. Unworkable for real-time session state shared across unrelated subtrees. |
| **MobX for Flutter** | Mature, code-gen-based. Less community traction in Flutter than Riverpod; fewer Copilot training examples. |

---

## References

- `../ARCHITECTURE.md` — Flutter app layer architecture diagram
- `.specify/memory/constitution.md` — "Flutter + Riverpod 2.x" as mandatory tech stack entry
