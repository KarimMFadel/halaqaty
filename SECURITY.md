# Security Policy

## Supported Versions

Halaqaty is in active pre-launch development. Security fixes are applied to the `main` branch only.

| Version | Supported |
| ------- | --------- |
| `main` (active development) | ✅ |

---

## Reporting a Vulnerability

**Please do not open a public GitHub issue for security vulnerabilities.**

Report privately by emailing: **security@halaqaty.app**

Include in your report:

- Description of the vulnerability
- Steps to reproduce
- Potential impact assessment
- Any suggested fix (optional)

We will acknowledge your report within **48 hours** and aim to resolve critical issues within **7 days**.

---

## Scope

### In scope

- Authentication or authorization bypass
- Unauthorized access to circle, session, or student data
- SQL injection or other server-side injection attacks
- WebSocket message spoofing or session hijacking
- LiveKit token forgery or publish-scope escalation (e.g., forcing `CanPublish: true` outside a student's turn)
- FCM token leakage enabling unauthorized push notifications

### Out of scope

- Denial-of-service attacks against development or staging environments
- Issues requiring physical access to a device
- Vulnerabilities in third-party dependencies — report these to their maintainers directly
- Social engineering of team members

---

## Security Invariants

The following rules are non-negotiable and never broken in any environment. Any PR that violates these is rejected:

1. **LiveKit tokens are generated exclusively by the Go backend.** The Flutter client never calls LiveKit APIs directly.
2. **Firebase Auth is for identity only.** All authorization checks query PostgreSQL `circle_members`. A valid Firebase JWT grants no action without a matching role record.
3. **Roles are per-circle.** Global roles do not exist.
4. **Student publish scope is turn-based.** `CanPublish: true` is granted only to the active reciter and revoked immediately after their turn ends.
5. **Recording is DISABLED in MVP.** `FEATURE_RECORDING_ENABLED` stays `false` until a privacy framework is approved.
6. **All input is validated server-side.** The Flutter client is never trusted.
7. **Parameterized queries only.** No string-interpolated SQL.
8. **Rate limiting is enforced.** REST: per IP and per user ID. WebSocket: max 3 active connections per user, max 30 messages/min per user per circle.

See [`.specify/memory/constitution.md`](.specify/memory/constitution.md) §IV for the full list.

---

## Disclosure Policy

We follow **responsible disclosure**:

1. Give us reasonable time to patch before public disclosure.
2. Do not access or modify user data beyond what is needed to demonstrate the issue.
3. Do not disrupt service availability.

Researchers who responsibly disclose valid vulnerabilities will be credited in the release notes (unless they prefer to remain anonymous).
