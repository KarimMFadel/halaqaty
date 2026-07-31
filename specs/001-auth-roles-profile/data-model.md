# Data Model: Authentication, Roles, and User Profile

## User and Profile

`users` holds the immutable local identity: `id` (UUID), `firebase_uid` (unique), `email` (unique), and timestamps. `profiles` is a 1:1 extension keyed by `user_id`, holding `full_name`, `country`, `phone`, `bio`, `avatar_url`, `completed_at`, and timestamps. There is no global role column.

First profile completion requires trimmed `full_name` and a valid ISO country code. Firebase UID is derived only from the verified bearer and is never client supplied.

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
