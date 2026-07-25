# Research: Authentication, Roles, and User Profile

## Decision 1: Token/session model
- **Decision**: Use Firebase-issued ID tokens (1-hour lifecycle with SDK refresh) and backend-managed inactivity sessions (30-day timeout) in PostgreSQL.
- **Rationale**: Aligns with constitution identity boundary while preserving backend-controlled session expiry and auditability.
- **Alternatives considered**: Backend-issued JWT + refresh token pair (constitution conflict), in-memory session store (non-durable).

## Decision 2: Auth boundary with Firebase
- **Decision**: Firebase remains identity verification boundary; backend performs authorization and session lifecycle.
- **Rationale**: Aligns with constitution auth boundary and keeps role checks centralized in backend + database.
- **Alternatives considered**: Pushing authorization into Firebase custom claims (revocation lag / role staleness risk).

## Decision 3: Role model
- **Decision**: Self-registration account type is restricted to `student|teacher`, and authorization is enforced by per-circle roles in PostgreSQL `circle_members` (`student|teacher|supervisor`).
- **Rationale**: Aligns with constitution rule that authorization is per-circle and avoids global-role privilege escalation.
- **Alternatives considered**: Global admin-only authorization model (constitution conflict), client-claim-based role checks (stale/unsafe).

## Decision 4: Profile completion rules
- **Decision**: First-time profile completion requires `full_name` and `country`; enforce server-side validation and clear field-level errors.
- **Rationale**: Directly satisfies FR-012 and improves onboarding consistency.
- **Alternatives considered**: Client-side-only validation (bypass risk), optional completion (violates requirement).

## Decision 5: Security baseline controls
- **Decision**: Enforce authentication middleware, per-circle authorization guards, request validation, rate limits (REST per IP+user and WS limits), and structured audit logs for auth/profile/role events.
- **Rationale**: Covers mandatory security baseline and supports incident forensics.
- **Alternatives considered**: Partial controls (insufficient), delayed audit logging (reduced accountability).

## Decision 6: Reliability baseline controls
- **Decision**: Apply explicit timeouts (HTTP/DB/Firebase), safe retry policy, idempotent register/profile update behavior, request IDs + structured logs + health checks.
- **Rationale**: Meets reliability baseline and supports SC-001 operationally.
- **Alternatives considered**: No timeout discipline (hang risk), blind retries on non-idempotent writes.

## Decision 7: Testing strategy
- **Decision**: Go unit/integration/contract tests; Flutter widget/integration tests mapped to P1/P2/P3 user stories.
- **Rationale**: Matches required testing expectations and keeps contract-first enforcement.
- **Alternatives considered**: Unit-only backend tests (insufficient for auth contract behavior), UI-only Flutter tests (insufficient business flow confidence).
