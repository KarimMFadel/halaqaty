# API Documentation

REST API reference, endpoint specifications, request/response schemas, and authentication details.

> **OpenAPI contract:** [`docs/contracts/openapi.yaml`](../../contracts/openapi.yaml) — the authoritative machine-readable spec. All endpoints below are normative only when they match that file.

---

## Authentication Flow

Halaqaty uses **Firebase Auth for identity** and the **Go backend for authorization**. The two steps are distinct and both required.

### Overview

```
Flutter App                 Firebase Auth            Go Backend
    │                            │                        │
    │── signInWithGoogle() ──────►                        │
    │◄── Firebase ID Token ───────                        │
    │                                                     │
    │── POST /api/v1/auth/sessions ───────────────────────►
    │   Authorization: Bearer <Firebase ID Token>           │
    │◄── 200 { "session_id": "<opaque id>", "user": {...} }│
    │                                                     │
    │── GET /api/v1/circles (Firebase token + session id) ►
    │◄── 200 { circles: [...] } ──────────────────────────│
```

### Step 1 — Firebase sign-in (client-side)

The Flutter client authenticates with Firebase using one of:

| Method | Package | Notes |
|--------|---------|-------|
| Email + password | `firebase_auth` | Requires email verification before login |
| Google Sign-In | `google_sign_in` + `firebase_auth` | Works on Android and iOS |
| Apple Sign-In | `sign_in_with_apple` + `firebase_auth` | **Mandatory on iOS** when Google Sign-In is offered |

```dart
// Example: Google Sign-In
final googleUser = await GoogleSignIn().signIn();
final googleAuth = await googleUser!.authentication;
final credential = GoogleAuthProvider.credential(
  accessToken: googleAuth.accessToken,
  idToken: googleAuth.idToken,
);
final userCredential = await FirebaseAuth.instance.signInWithCredential(credential);
final firebaseToken = await userCredential.user!.getIdToken();
```

### Step 2 — Backend device session

The backend verifies the Firebase ID token and creates an opaque, durable session for
the current device. It does not receive a password, issue a Halaqaty JWT, or return a
Firebase refresh token:

```
POST /api/v1/auth/sessions
Authorization: Bearer <Firebase ID Token>
Content-Type: application/json

{ "device_name": "Karim's iPhone" }
```

**Success response (200):**
```json
{
  "session_id": "opaque-backend-session-id",
  "expires_at": "2026-05-01T10:00:00Z",
  "user": {
    "id": "uuid",
    "display_name": "Ahmad Al-Sayed",
    "email": "ahmad@example.com",
    "timezone": "Asia/Riyadh"
  }
}
```

**Error responses:**

| Status | Code | Meaning |
|--------|------|---------|
| `401` | `ERR_INVALID_TOKEN` | Firebase token expired or invalid |
| `403` | `ERR_ACCOUNT_DISABLED` | Account suspended |
| `422` | `ERR_EMAIL_UNVERIFIED` | Email account not verified yet |

### Step 3 — Authenticated requests

Include both credentials in every protected request:

```
Authorization: Bearer <Firebase ID Token>
X-Halaqaty-Session-ID: <opaque-backend-session-id>
```

The Go backend verifies the Firebase ID token and checks the backend session's user
binding, expiry, inactivity, and revocation on every request. Failure returns
`401 ERR_UNAUTHORIZED`.

### Token refresh

- Firebase tokens expire after **1 hour**. The Flutter client calls `user.getIdToken(refresh: true)` transparently.
- Firebase owns refresh-token rotation and reuse detection; Halaqaty has no refresh endpoint.
- **30-day inactivity logout**: the backend tracks `last_activity_at` per device session.
  An inactive or revoked session is rejected even if the Firebase ID token is still valid.

### Logout

```
POST /api/v1/auth/logout
Authorization: Bearer <Firebase ID Token>
X-Halaqaty-Session-ID: <opaque-backend-session-id>
```

This revokes only the identified backend device session. The Flutter client also calls
`FirebaseAuth.instance.signOut()` locally. A logout-all-devices endpoint is deferred;
when introduced it will revoke all sessions for the authenticated user.

---

## Standard Error Envelope

All error responses follow this structure:

```json
{
  "error": {
    "code": "ERR_CIRCLE_NOT_FOUND",
    "message": "Circle does not exist or you are not a member"
  }
}
```

Standard HTTP semantics: `400` bad input · `401` unauthenticated · `403` forbidden · `404` not found · `409` conflict · `422` unprocessable · `500` internal.

---

## Endpoint Index

> Full schemas, query parameters, and validation rules are in [`docs/contracts/openapi.yaml`](../../contracts/openapi.yaml) — the authoritative source. This index mirrors that file; if it ever disagrees with the contract, the contract wins.
>
> All paths are relative to the API base URL (`/api/v1`). "Bearer" = `Authorization: Bearer <Firebase ID Token>`; "Session ID" = `X-Halaqaty-Session-ID` header. See the [Authentication Flow](#authentication-flow) above.

### Auth

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/auth/register` | Bearer | Provision a just-created Firebase identity and create a current-device backend session |
| `POST` | `/auth/sessions` | Bearer | Create a current-device backend session after Firebase sign-in |
| `POST` | `/auth/logout` | Bearer + Session ID | Invalidate the current backend device session |
| `POST` | `/auth/fcm-token` | Bearer + Session ID | Register/upsert a device FCM token for push notifications |
| `GET` | `/auth/me` | Bearer + Session ID | Get current user profile |
| `PUT` | `/auth/me` | Bearer + Session ID | Update profile (name, avatar, language) |
| `DELETE` | `/auth/me` | Bearer + Session ID | Delete account and all associated data (irreversible, GDPR-compliant) |

### Circles

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/circles` | Bearer + Session ID | List circles the user is a member of (optional `?role=` filter) |
| `POST` | `/circles` | Bearer + Session ID | Create a new circle (assigns teachers/supervisor; creator defaults to teacher if none selected) |
| `GET` | `/circles/{circleId}` | Bearer + Session ID | Get circle details |
| `PUT` | `/circles/{circleId}` | Bearer (teacher) | Update circle settings |
| `DELETE` | `/circles/{circleId}` | Bearer (teacher) | Archive a circle |
| `GET` | `/circles/{circleId}/members` | Bearer + Session ID | List circle members |
| `DELETE` | `/circles/{circleId}/members/{userId}` | Bearer (teacher) | Remove a member from the circle |
| `PUT` | `/circles/{circleId}/members/{userId}/role` | Bearer (teacher/supervisor) | Update another active member's role (student/supervisor/teacher) |
| `POST` | `/circles/join` | Bearer + Session ID | Join a circle using an invite code |

### Sessions

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/circles/{circleId}/sessions` | Bearer + Session ID | List sessions for a circle (paginated, optional `?status=` filter) |
| `POST` | `/circles/{circleId}/sessions` | Bearer (teacher) | Create a new session |
| `POST` | `/sessions/{sessionId}/start` | Bearer (teacher) | Start a session — ensures its media room and returns the teacher's `MediaConnection` |
| `POST` | `/sessions/{sessionId}/join` | Bearer (member) | Join an active session — returns the caller's `MediaConnection` |
| `POST` | `/sessions/{sessionId}/end` | Bearer (teacher) | End a session |
| `POST` | `/sessions/{sessionId}/ws-token` | Bearer + Session ID | Issue a short-lived (60 s) WebSocket connection token |

### Queue

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/sessions/{sessionId}/queue` | Bearer + Session ID | Get current queue state (full snapshot) |
| `POST` | `/sessions/{sessionId}/queue/rounds` | Bearer (teacher/supervisor) | Start a new recitation round (populates queue) |
| `POST` | `/sessions/{sessionId}/queue/entries/{entryId}/grade` | Bearer (teacher/supervisor) | Submit grade for a completed recitation turn |
| `POST` | `/sessions/{sessionId}/queue/opt-out` | Bearer (student) | Opt out of the current round (requires teacher/supervisor approval) |

### Chat

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/circles/{circleId}/messages` | Bearer + Session ID | List recent messages (paginated, `?limit=` and `?before=`) |
| `POST` | `/circles/{circleId}/messages` | Bearer + Session ID | Send a text, image, or file message |
| `DELETE` | `/circles/{circleId}/messages/{messageId}` | Bearer + Session ID | Delete a message (own within 10 min; teacher can delete any) |

### Schedules

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/circles/{circleId}/schedules` | Bearer + Session ID | List schedules for a circle |
| `POST` | `/circles/{circleId}/schedules` | Bearer (teacher) | Create a recurring schedule entry |

### Progress (F-007)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/students/me/circles/history` | Bearer + Session ID | Student's circle history with attendance/practice counts (paginated) |
| `GET` | `/students/me/progress` | Bearer + Session ID | Student's global Quran map — all 114 Surahs with memorization status (optional `?circle_id=`) |
| `GET` | `/students/me/sessions/history` | Bearer + Session ID | Student's session history (attended vs practiced breakdown, paginated) |
| `GET` | `/students/me/progress/stats` | Bearer + Session ID | Student's progress analytics (Ayahs per time bucket, attendance %, practice %) |
| `GET` | `/circles/{circleId}/progress` | Bearer (teacher/supervisor) | All students' progress summary in a circle |
| `GET` | `/circles/{circleId}/progress/{userId}` | Bearer (teacher) | One student's full cross-circle progress profile |
| `GET` | `/circles/{circleId}/surah-insights` | Bearer (teacher) | Surahs ranked by weak grade frequency in last N days |

### Uploads (MinIO)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/uploads/avatar` | Bearer + Session ID | Upload user avatar image (≤5 MB; jpeg/png/webp) |
| `POST` | `/uploads/voice` | Bearer + Session ID | Upload a voice note attachment (≤20 MB, ≤5 min; ogg/mpeg/mp4/webm) |
| `POST` | `/uploads/image` | Bearer + Session ID | Upload an image attachment for chat (≤20 MB; jpeg/png/webp/gif) |
| `POST` | `/uploads/file` | Bearer + Session ID | Upload a PDF file attachment for chat (≤20 MB) |

### Config

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/config/features` | Bearer + Session ID | Get active feature flags for the authenticated user (per-tier filtering server-side) |

> **Note on `/healthz`:** a health-check endpoint is not part of the versioned `/api/v1` contract in `openapi.yaml` and is therefore not listed here. Operational/liveness probes live outside the product API surface.

---

*See [`docs/contracts/openapi.yaml`](../../contracts/openapi.yaml) for full request/response schemas, query parameters, and validation rules.*

