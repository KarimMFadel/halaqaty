# Quickstart: 001-auth-roles-profile

## Goal
Implement and validate auth, role enforcement, and profile flows for Go backend + Flutter mobile with PostgreSQL and Firebase identity boundary.

## 1) Contract-first steps
1. Merge `specs/001-auth-roles-profile/contracts/auth-roles-profile.openapi.yaml` changes into `docs/contracts/openapi.yaml`.
2. Preserve backward compatibility (additive changes only).
3. Validate OpenAPI contract linting before coding handlers.

## 2) Backend implementation outline (Go)
1. Add DB migrations for users/profiles/user_sessions/circle_members and profile completion fields.
2. Implement auth endpoints:
   - `POST /auth/register`
   - `POST /auth/login`
   - `POST /auth/logout` (invalidate current session only)
   - `GET/PUT /auth/me`
3. Add circle-role-protected role assignment endpoint:
   - `POST /circles/{circleId}/members/{userId}/role`
4. Add middleware/policies for:
   - authentication + authorization (resolving database User UUID from Firebase UID)
   - validation
   - rate limits (with periodic memory eviction of counters)
   - audit logging
   - timeout handling
5. Implement application bootstrapping, database connection pooling, dependency wiring, and HTTP server startup in `backend/cmd/api/main.go`.

## 3) Mobile implementation outline (Flutter)
1. Build/register/login/logout flows.
2. Build profile read/edit flow with mandatory first completion (`full_name`, `country`).
3. Enforce Firebase token lifecycle handling and unauthorized state handling.
4. Ensure role-restricted UX paths map correctly to backend authorization responses.

## 4) Testing expectations
- **Go unit tests**: Firebase verifier, auth service, session inactivity rules, role guards, profile validation.
- **Go integration tests**: endpoint + DB flows including inactivity-session expiration handling.
- **Go contract tests**: endpoint request/response conformance to OpenAPI.
- **Go quality gate**: verify `backend/internal/` coverage meets or exceeds 80%.
- **Flutter widget tests**: auth/profile screen validation and state rendering.
- **Flutter integration tests**: register/login/logout; profile completion; role-protected behavior.

## 5) Security and reliability acceptance checklist
- Authentication and authorization enforced on protected routes.
- Input validation and clear error envelopes implemented.
- Rate limits configured for auth and protected routes.
- Audit logs emitted for auth/admin/profile-sensitive actions.
- Timeouts, safe retry behavior, idempotency, and request observability present.
