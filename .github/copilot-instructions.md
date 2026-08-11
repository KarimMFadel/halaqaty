<!-- SPECKIT START -->
For additional context about technologies to be used, project structure,
shell commands, and other important information, read the current plan:
`specs/001-auth-roles-profile/plan.md`
<!-- SPECKIT END -->

## Agentic workflow harness

Before feature planning or implementation, follow `AGENTS.md` and
`docs/engineering/collaboration/AGENT_WORKFLOW_HARNESS.md`. Spec-Kit owns
feature scope and artifacts; Superpowers supplements implementation with
TDD, debugging, focused review, and verification without generating a
second specification, plan, task list, or review hierarchy.

## Backend implementation guardrails

- Reuse centralized HTTP/API constants from `backend/internal/platform/httpconst/` (headers, auth scheme, content types, shared error messages). Do not inline these literals in handlers or middleware.
- Keep runtime SQL out of repository method bodies. Define SQL statements in package-level `*_queries.go` files (or generated query layer), then reference them from repository methods.
- Centralize HTTP route patterns in `backend/cmd/api/routes.go` and reference them from router wiring; avoid inline `"METHOD /path"` literals in handlers.

## Recurring regression guardrails

- Before editing, load the matching `.github/agents/` role instructions. Flutter work must apply the role's recurring Flutter regression checklist; Go REST/RBAC work must apply the recurring REST/RBAC checklist.
- Contract changes must pass `$docs-guard`, including canonical versus feature-local OpenAPI synchronization and duplicate-key checks.
- When asked to handle review findings, implement every actionable finding in one bounded fix wave, add the smallest regression coverage, and rerun affected checks. Do not merely repeat the findings.
- Mark Spec-Kit tasks `[X]` only after their deliverables exist and current verification evidence supports completion; report unavailable tools or skipped environment-dependent tests explicitly.
