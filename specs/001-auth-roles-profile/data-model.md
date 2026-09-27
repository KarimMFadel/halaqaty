# Data Model: Authentication, Roles, and User Profile

## User and Profile

`users` holds the immutable local identity: `id` (UUID), `firebase_uid` (unique), `email` (unique), and timestamps. `profiles` is a 1:1 extension keyed by `user_id`, holding `full_name`, `country`, `phone`, `bio`, `avatar_url`, `completed_at`, and timestamps. There is no global role column.

First profile completion requires trimmed `full_name` and a valid ISO country code. Firebase UID is derived only from the verified bearer and is never client supplied.

Profile payload validation keeps `display_name` within 2..100 characters for both registration and profile updates.

## UserSession

`user_sessions` holds an opaque, server-generated UUID `id` exposed as `X-Halaqaty-Session-ID`; `user_id` is a required foreign key. It also stores nullable `device_name`, `last_activity_at`, non-null `expires_at`, nullable `revoked_at`, and creation metadata/timestamps. A session is valid only when it belongs to the bearer-derived user, is not revoked, and has not exceeded its inactivity/expiry policy.

Migration plan: retain existing deployed session values; introduce/standardize the UUID session identifier and required expiry through a new sequential migration, with a compatibility backfill before enforcing `NOT NULL`/foreign-key constraints.

## Circle and CircleMember

`circles` remains the canonical parent table. `circle_members` is the authorization table with `circle_id` and `user_id` foreign keys, unique `(circle_id, user_id)`, `role` constrained to `student | supervisor | teacher`, and join/update timestamps.

Creation is one transaction: insert the circle, add selected registered teachers, add an optional registered backup supervisor, and add the creator as teacher when no teacher is selected or supervisor otherwise. Reject duplicate/overlapping selections and unknown users. Invite acceptance inserts only `student`.

Role changes lock the target circle membership set in one transaction. The actor and target must be distinct active members of that circle; actor role must be teacher or supervisor; the resulting set must contain at least one teacher.

## Relationships and state

- User 1:1 Profile; User 1:N UserSession; User N:M Circle through CircleMember.
- Session: active → revoked (logout) or expired (inactivity/expiry).
- Membership: active role may transition only through a permitted manager mutation; removal is rejected if it leaves no teacher.

## Account-deletion amendment

`users.deleted_at TIMESTAMPTZ NULL` is the irreversible local closure marker. `users.firebase_uid` and `users.email` become nullable; their existing unique constraints still prevent duplicate active values. A row with `deleted_at IS NOT NULL` is never a login, registration, session, membership-management, or profile-edit principal. The UUID `users.id` remains for immutable history foreign keys. No global role or deletion-state table is added.

Before closure, the transaction rejects accounts with an active teacher/supervisor membership, an owned active circle, or an active participant record in an active live session. Session admission locks the same user row and verifies it is not tombstoned, preventing a media credential from being issued after closure commits. The transaction sets `deleted_at`, clears `users.email`, deletes all `user_sessions` rows (invalidating every session ID and erasing session/device metadata), erases device tokens where present, and updates `profiles` so only `display_name` (plus technical keys/timestamps and the non-identifying language default) remains. `full_name`, `country`, `phone`, `bio`, `avatar_url`, and `completed_at` become NULL. The backend retains `firebase_uid` only while Firebase removal is pending; `deleted_at IS NOT NULL AND firebase_uid IS NOT NULL` identifies a retryable cleanup row. After successful Admin deletion (or user-not-found), set `firebase_uid = NULL`. Never roll back a committed `deleted_at` to regain access.

Historical circle, session, message, queue, and recitation/progress rows keep their existing `users.id` foreign keys and authorized audience. Their display-name projection reads the retained `profiles.display_name`; it must never fall back to email, full name, Firebase UID, or private profile fields. A user may be an ordinary historical member without an active circle owned by them; teacher/ownership cases remain blocked until F-008 notification delivery.

The new paired migration must have a down path that refuses to restore `NOT NULL` identity constraints if tombstones exist. It cannot reconstruct erased personal data. Any avatar object purge and external identity cleanup must be verified at the storage boundary rather than inferred from SQL state.
