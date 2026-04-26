# ADR-004: Authentication and Authorization Boundary

**Status:** Accepted  
**Date:** 2026-04-26  
**Deciders:** Karim (product owner)

---

## Context

Halaqaty requires two distinct security concepts:

1. **Authentication** — proving who you are ("is this a valid user?")
2. **Authorization** — determining what you can do ("is this user a teacher in circle X?")

Firebase Authentication is already mandated by the tech stack for its cross-platform SDK and social login support. The question is: where should role-based authorization live?

---

## Decision

**Firebase Authentication handles identity only.** The backend enforces all authorization rules from PostgreSQL.

**Boundary rules:**
1. The Go backend trusts a Firebase ID token to identify *who* the user is (UID). Period.
2. The backend **never** reads Firebase custom claims for authorization decisions.
3. All role checks (`is_teacher`, `is_supervisor`, `is_student` for circle X) are read from the `circle_members` table in PostgreSQL at request time.
4. The Flutter app **never** makes authorization decisions — it renders UI based on role data returned by the API, not by inspecting the Firebase token locally.
5. LiveKit room tokens are issued exclusively by the Go backend, never by the Flutter app, and encode the minimum required permissions (`CanPublishAudio: true`, `CanPublishVideo: false`, `CanSubscribe: true`).

**JWT middleware behavior:**
- Every protected route calls Firebase Admin SDK `VerifyIDToken()` to validate the token.
- The extracted `uid` is stored in Echo's request context.
- Domain handlers call `circles.GetMemberRole(ctx, circleID, uid)` — not a custom claim.

---

## Consequences

**Positive:**
- Role changes take effect immediately — no Firebase token cache delay (Firebase custom claims can take up to 1 hour to propagate).
- PostgreSQL is the single source of truth for authorization. No split-brain between Firebase claims and DB state.
- LiveKit tokens issued by the backend cannot be forged or manipulated by the client.
- Security invariant is simple to audit: "if it's not in `circle_members`, access is denied."

**Negative:**
- Every protected API call that checks roles incurs a DB query (one `SELECT` on `circle_members`). Mitigated by an indexed query on `(circle_id, user_id)`.
- Role data is not embedded in the Firebase token — the Flutter app must fetch role context from the API on startup and after role-change events.

---

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| **Firebase custom claims for roles** | Claims cache up to 1 hour. A teacher could be removed from a circle, and their custom claim would still grant access for up to an hour. Unacceptable security gap. |
| **JWT with custom backend token (not Firebase)** | Would require our own token issuance infrastructure. Firebase already handles refresh, revocation, and cross-platform SDKs — reinventing this adds cost without benefit. |
| **RBAC middleware library (Casbin, etc.)** | Adds a dependency and a policy language for a permission model simple enough to express in two SQL queries. Deferred to post-MVP if permission complexity grows. |

---

## References

- `docs/ARCHITECTURE.md` — Security Considerations section
- `docs/MVP_DECISION_REGISTER.md` — OQ-003 (Firebase 1hr refresh), OQ-015 (video disabled)
- `.specify/memory/constitution.md` — Security invariants 1–3
