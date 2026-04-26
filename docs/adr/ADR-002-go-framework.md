# ADR-002: Go Web Framework — Echo v4

**Status:** Accepted  
**Date:** 2026-04-26  
**Deciders:** Karim (product owner)

---

## Context

The Halaqaty Go backend needs an HTTP router and middleware framework. The chosen framework will handle routing, request validation, error handling, middleware chains (auth, CORS, rate-limiting, logging), and WebSocket upgrade for the live session gateway.

The constitution requires: clean error handling, predictable middleware composition, good documentation, and compatibility with the modular monolith package structure.

---

## Decision

We will use **Echo v4** (`github.com/labstack/echo/v4`).

**Key configuration decisions:**
- All routes grouped by domain prefix: `/api/v1/auth`, `/api/v1/circles`, `/api/v1/sessions`, etc.
- Middleware chain per group: JWT validation middleware applied at group level, not per-route.
- Custom `HTTPError` handler registered globally to return consistent JSON error envelopes.
- WebSocket connections use Echo's built-in `c.Upgrade()` with a dedicated `/ws` route group.
- `EchoError` types define error codes using the `errors` package + sentinel constants — no raw HTTP status codes in domain code.

---

## Consequences

**Positive:**
- Echo's route group + middleware system maps cleanly to the domain package structure (ADR-001). Each domain registers its own route group.
- Typed error handling (`echo.NewHTTPError`) produces consistent API error envelopes without custom middleware.
- Echo's validator integration (`go-playground/validator/v10`) handles request body validation declaratively.
- Widely used in the Go community; Copilot has strong training signal on Echo patterns.
- Built-in WebSocket support removes the need for a separate `gorilla/websocket` dependency (though `gorilla/websocket` is used internally by Echo).

**Negative:**
- Echo is not in the Go standard library. Vendor lock-in is minimal (swap cost is ~2 days of handler refactoring) but non-zero.
- Less minimalistic than `chi` or `net/http` stdlib — acceptable trade-off for team velocity.

---

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| **Gin** | Error handling model is weaker than Echo (requires manual `c.AbortWithError`). Route groups exist but middleware composition is less clean. Echo's typed error envelope is a material advantage. |
| **chi** | Excellent minimalistic router, fully stdlib-compatible. Rejected because it provides no built-in validation, error handling, or WebSocket helpers — we would reinvent what Echo gives us. |
| **Standard library (`net/http`)** | Zero dependencies, maximum longevity. Rejected for MVP because middleware composition requires writing boilerplate that Copilot would generate inconsistently. Revisit post-MVP if needed. |
| **Fiber** | Gin-like API on top of `fasthttp`. Rejected because `fasthttp` is not fully net/http-compatible, which creates friction with standard tooling (testing, profiling, middleware ecosystem). |

---

## References

- `docs/ARCHITECTURE.md` — API endpoint definitions use Echo route group naming convention
- `.specify/memory/constitution.md` — "Go + Echo v4" as mandatory tech stack entry
