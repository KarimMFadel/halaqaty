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

## Decision 6: Profile display_name contract alignment

- **Decision**: `UpdateProfileRequest.display_name` enforces `minLength: 2` and `maxLength: 100`, matching registration/profile expectations.
- **Rationale**: Resolves the generated-contract mismatch while preserving existing API behavior and avoiding broader schema changes.
- **Alternatives considered**: Keep update payload less strict (inconsistent validation semantics); widen limits (unjustified behavioral change).

## Account-deletion amendment: closure before Firebase cleanup

- **Decision**: Use the existing `users` row as a durable tombstone and cleanup work item. A transaction closes local access, deletes all backend session rows (invalidating their IDs), and erases non-retained personal fields before the backend calls Firebase Admin `DeleteUser`. On Firebase failure, a backend worker retries pending tombstones; `IsUserNotFound` is an idempotent success. Never require an authenticated client retry after closure.
- **Rationale**: PostgreSQL and Firebase cannot share one atomic transaction. A local closure first guarantees no backend access during an external outage. Preserving `firebase_uid` only while cleanup is pending gives the worker a target, and nulling it after success minimizes retained identifiers. The surviving local UUID is a provenance FK, not a public identity field.
- **Alternatives considered**: Client-side Firebase delete (leaves backend unable to retry reliably); Firebase-first delete (can strand personal profile/session data); hard-deleting `users` (breaks retained history FKs); a new outbox table (unneeded because pending rows are enumerable).
- **Verification boundary**: Firebase Admin `VerifyIDToken` checks the JWT but does not check revocation; use `VerifyIDTokenAndCheckRevoked` on registration/session creation so a pre-deletion token cannot reprovision after UID scrubbing. This makes those operations dependent on Firebase availability and fail closed. [Firebase Admin Go reference](https://pkg.go.dev/firebase.google.com/go/v4/auth), [Firebase session guidance](https://firebase.google.com/docs/auth/admin/manage-sessions).
- **Retention**: Only display name remains as human-readable attribution in the profile/history projection. Historical chat and educational records retain their content and FKs under existing authorization. The email, full name, phone, avatar reference, and other profile data are erased. This is a product data-retention decision, not a legal-compliance claim.
- **Teacher dependency**: Until F-008 can deliver member notifications, teacher deletion must be blocked before any mutation. The archive and notification step belongs in a later F-008-integrated amendment; it cannot be simulated by a local notice.
