# Research: Authentication, Roles, and User Profile

## Decision 1: Identity and backend-session boundary

- **Decision**: Flutter Firebase Auth creates identities, validates passwords, signs users in, and refreshes Firebase ID tokens. Go verifies those tokens and creates/revokes opaque, per-device PostgreSQL sessions. Registration and session creation require only the Firebase bearer; every other protected request requires the bearer and `X-Halaqaty-Session-ID` for the same local user.
- **Rationale**: Preserves Firebase as the identity authority while allowing server-side revocation and the 30-day inactivity rule.
- **Alternatives considered**: Backend passwords or Firebase-token issuance (breaks the boundary); bearer-only protected access (cannot revoke an individual device session).

## Decision 2: Per-circle role model

- **Decision**: `circle_members` is the only authorization source. Creation may immediately add registered users as multiple teachers and one backup supervisor; with no selected teacher, the creator is the teacher, otherwise the creator is supervisor. Invitees become students.
- **Rationale**: Implements ADR-010 without global account roles or delayed assignment of initial managers.
- **Alternatives considered**: Global roles (breaks the constitution); a single owner-teacher (cannot express the approved workflow).

## Decision 3: Role-change safety

- **Decision**: An active teacher or supervisor may change another active member to `student`, `supervisor`, or `teacher`. Reject self-changes, absent/cross-circle targets, and any mutation that would remove the final teacher; perform the check and mutation in one transaction.
- **Rationale**: Avoids self-escalation/lockout and teacherless circles under concurrent requests.
- **Alternatives considered**: Client-side enforcement (bypassable); separate count and update queries (race-prone).

## Decision 4: Additive migration and API compatibility

- **Decision**: Keep `000010_auth_roles_profile` immutable once applied. Create the next sequential migration to align session identifiers, expiry, role constraints/indexes, and circle foreign keys with the approved model; deploy schema before handlers. Contract changes are additive request fields and preserve existing response shapes.
- **Rationale**: Protects deployed databases and existing mobile clients.
- **Alternatives considered**: Editing an applied migration (unsafe); a breaking v1 request/response change (unnecessary).

## Decision 5: Security, reliability, and tests

- **Decision**: Use request timeouts, validation, per-IP/user rate limits, audit records for registration/session/profile/role mutations, request IDs, structured logs, and retry only safe idempotent reads. Test Go unit, integration, and contract paths plus Flutter widget and integration flows.
- **Rationale**: Meets the feature and constitution baselines.
