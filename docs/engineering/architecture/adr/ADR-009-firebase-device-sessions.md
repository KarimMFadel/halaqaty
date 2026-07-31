# ADR-009: Firebase Identity and Backend Device Sessions

**Status:** Accepted  
**Date:** 2026-07-31  
**Deciders:** Karim (product owner)

---

## Context

Firebase Auth is the mandated identity provider, while the backend must enforce a
30-day inactivity policy and immediately revoke access from one lost or logged-out
device. Earlier documentation incorrectly described Go endpoints that accept an
email/password pair and return Firebase tokens. That contradicts the Firebase identity
boundary and would make the backend responsible for credentials it must not own.

The platform also needs a non-escalating way to establish initial circle roles and to
manage supervisors.

## Decision

1. Flutter Firebase Auth exclusively handles password validation, account creation,
   sign-in, sign-out, and Firebase ID-token refresh. The Go backend never receives a
   password or emits a Firebase ID or refresh token.
2. The Go backend verifies the Firebase ID token and maintains a durable, opaque
   per-device backend session in PostgreSQL. A session is created after Firebase
   registration or sign-in, supplied in `X-Halaqaty-Session-ID`, expires after 30 days
   of inactivity, and may be revoked immediately.
3. Current-device logout revokes only the supplied backend session and the app signs
   out from Firebase locally. A future explicit logout-all-devices endpoint revokes all
   backend sessions for the authenticated user. Firebase remains responsible for
   refresh-token rotation and reuse detection.
4. Creating a circle atomically creates a teacher membership for its creator. Joining
   with an invite creates a student membership. Only that circle's teacher can promote
   an existing student to supervisor or revoke a supervisor; the role endpoint cannot
   assign a teacher role.

## Consequences

- Revocation is checked server-side on every protected request without replacing
  Firebase as the identity provider.
- Compromise of a Firebase ID token alone is insufficient after the matching backend
  device session has been revoked.
- Contracts and tests must cover missing, expired, unknown, revoked, and inactive
  backend sessions, plus current-device and all-devices logout semantics.
- The `user_sessions` schema must record a server-generated session ID, user ID,
  device label/metadata as appropriate, creation time, last activity, expiry, and
  revocation time. It must not store Firebase refresh tokens.

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| Backend email/password login and Firebase-token response | The backend would handle credentials and cannot safely issue Firebase client refresh tokens. |
| Backend-issued access and refresh tokens | Duplicates Firebase identity/token infrastructure and conflicts with the established Firebase boundary. |
| Firebase token only, no backend session | Cannot provide reliable current-device server-side logout or a durable 30-day inactivity policy. |
| Self-selected teacher/supervisor role | Enables privilege escalation and confuses account preference with per-circle authorization. |

## References

- [ADR-004](ADR-004-auth-boundary.md)
- [Architecture](../ARCHITECTURE.md)
- [MVP Decision Register](../../../management/product/MVP_DECISION_REGISTER.md)
- [Canonical OpenAPI contract](../../../contracts/openapi.yaml)
