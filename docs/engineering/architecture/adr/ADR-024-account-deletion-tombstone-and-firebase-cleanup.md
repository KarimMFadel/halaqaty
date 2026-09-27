# ADR-024: Account Closure Tombstone and Firebase Cleanup

**Status:** Accepted for F-001 design; Karim approved the implementation security review on 2026-09-27

**Date:** 2026-09-26
**Decider:** Karim (retention, reauthentication, and teacher-notification policy); Architect (transaction and retry design)

## Context

F-001 advertises account deletion, but `/auth/me` has no DELETE route. The canonical OpenAPI currently claims hard deletion of messages and progress, conflicting with the constitution's full-history rule, OQ-006, and ADR-019. History tables have foreign keys to `users.id`; hard-deleting a user would fail or erase educational provenance. Firebase and PostgreSQL cannot commit together. A valid Firebase JWT may remain cryptographically valid after a Firebase user is deleted unless revocation is checked.

Karim decided to retain history with **display name only** as human-readable identity, erase non-retained profile fields, require verified Firebase reauthentication within five minutes, close backend access and revoke all backend sessions before Firebase identity removal, and defer teacher deletion until F-008 can notify circle members. This batch covers student deletion and directly related privacy controls.

## Decision

Add nullable `users.deleted_at` and `users.deleted_firebase_uid_hash`, and make `users.firebase_uid` and `users.email` nullable in a new paired migration. The one-way SHA-256 digest prevents a delayed, already-verified Firebase token from recreating a tombstoned account after its raw UID is erased. Registration and closure serialize on a PostgreSQL transaction advisory lock keyed by Firebase UID so a registration started before cleanup cannot use an old snapshot to bypass the digest check. Keep `users.id` and historical foreign keys. In a transaction guarded by the user row lock, reject active teacher/supervisor memberships and owned active circles (student-only batch), set `deleted_at`, delete every backend session row (revoking each ID and erasing session/device metadata), clear email, erase all non-retained profile fields, and remove device tokens if present. Keep `profiles.display_name` for authorized historical attribution. Block deleted users at *every* backend authentication, registration, session-creation, authorization, and user-search boundary. A deleted member remains historical provenance, never an active authorization principal.

Use the verified Firebase Admin Go token's `AuthTime` field (`auth_time` claim), not token `IssuedAt` or client input, for the five-minute check. The mobile client reauthenticates with the Firebase SDK and sends explicit `confirm: true`. Rejected/stale requests commit no changes.

Before closure, reject accounts with an active teacher/supervisor membership, an owned active circle, or an active participant record in an active live session. Session admission must serialize on the same user-row lock (or atomically test `deleted_at IS NULL` in its admission transaction), preventing media credentials from being issued after closure commits. The active-session precondition keeps F-005 provider behavior unchanged while ensuring an issued room credential cannot remain usable after successful account closure.

Only after the closure transaction commits may the backend Admin SDK call `DeleteUser` for the stored Firebase UID. The UID stays on the tombstone while cleanup is pending and is cleared after success or `IsUserNotFound`. A periodic backend worker selects pending tombstones (`deleted_at IS NOT NULL AND firebase_uid IS NOT NULL`) in bounded user-ID pages, resuming after the previous page and wrapping to the start; failures in the oldest page cannot starve later accounts. The worker retries hourly with a bounded timeout per identity. Concurrent workers may retry the same Firebase UID; Admin deletion is repeat-safe because user-not-found counts as success and the conditional UID clear is idempotent, so no claim or lease field is needed. No client credential or authenticated retry is required. Session insertion must serialize with the same user-row lock or atomically test `deleted_at IS NULL` in its insert transaction, preventing a session from appearing after closure. A Firebase failure leaves the account closed and yields `202 pending_identity_removal`; `204` is returned only after confirmed Firebase removal. Once `firebase_uid` is cleared, the registration and session-creation entry points must use `VerifyIDTokenAndCheckRevoked`, which rejects old tokens even if their signatures have not expired. A Firebase outage on those entry points fails closed. No change is needed to normal protected-request JWT verification beyond the deleted-user guard.

Teacher and supervisor deletion remains unavailable in this student-only batch; teacher deletion additionally depends on F-008 member notification delivery. A later amendment may atomically archive all owned active circles and enqueue member notifications after verifying a designated supervisor for each; it must not silently archive or transfer ownership now.

## Schema and rollout

The migration adds `users.deleted_at` and a unique SHA-256 digest of a deleted Firebase UID, and relaxes only `users.firebase_uid`/`users.email` nullability; existing unique constraints continue to guard non-null active values. It must be applied before deletion routes are enabled. Do not edit previously applied migrations. The paired down migration may restore prior constraints only if no tombstones exist; otherwise it must fail rather than re-enable or corrupt erased accounts. No new table or external queue is needed at MVP scale. Update `ARCHITECTURE.md` and both OpenAPI contracts before implementation. A separate storage audit must confirm that an avatar URL is not mistaken for deletion of an underlying object.

This satisfies the constitution's Firebase-identity/PostgreSQL-authorization boundary (§IV), retained recitation history (frozen MVP business rules), test-first/migration verification (§VI), and PostgreSQL-only persistence with paired migrations (Technology Constraints). It does not amend the constitution.

## Consequences and verification

- History retains its author/participant UUID link internally and display-name attribution for authorized viewers. Email, full name, phone, avatar reference, and other profile details are erased; the pending Firebase UID is a temporary cleanup key, not historical attribution.
- The backend must test races between closure, registration, session creation, and worker cleanup; simulated Firebase failures and user-not-found replays; five-minute boundaries; and history/private-field projections. The Flutter app must distinguish pending from complete identity removal and clear local credentials in both cases.
- Logs must record a stable operation/request ID and stage without emitting Firebase UID, email, token, or erased field values. Pending cleanup must be observable and actionable if retries do not converge.
- Karim's manual deep review is required before committing/merging this security-sensitive path. This ADR does not claim legal compliance.

## Alternatives considered

| Option | Rejected because |
|---|---|
| Hard-delete `users` and cascade history | Contradicts retained educational history and ADR-019. |
| Firebase-first deletion | Can strand backend sessions/personal data when PostgreSQL fails. |
| Client-owned Firebase deletion | Cannot guarantee backend retry after sessions are revoked. |
| Separate deletion-job table/queue | Tombstone rows already provide a durable retry set. |
| Rely on JWT expiration after Firebase deletion | An old signed token could re-provision `/auth/register` during its remaining lifetime. |

## References

- [F-001 specification](../../../../specs/001-auth-roles-profile/spec.md), [F-001 plan](../../../../specs/001-auth-roles-profile/plan.md)
- [ADR-019: no cascading deletes on student history](ADR-019-no-cascade-student-history.md)
- [Firebase Admin user deletion](https://firebase.google.com/docs/auth/admin/manage-users)
- [Firebase Admin revocation checks](https://firebase.google.com/docs/auth/admin/manage-sessions)
- [Firebase Admin Go SDK](https://pkg.go.dev/firebase.google.com/go/v4/auth)
