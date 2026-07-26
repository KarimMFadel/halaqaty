<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
`specs/001-auth-roles-profile/plan.md`
<!-- SPECKIT END -->

## Backend implementation guardrails

- Reuse centralized HTTP/API constants from `backend/internal/platform/httpconst/` (headers, auth scheme, content types, shared error messages). Do not inline these literals in handlers or middleware.
- Keep runtime SQL out of repository method bodies. Define SQL statements in package-level `*_queries.go` files (or generated query layer), then reference them from repository methods.
- Centralize HTTP route patterns in `backend/cmd/api/routes.go` and reference them from router wiring; avoid inline `"METHOD /path"` literals in handlers.
