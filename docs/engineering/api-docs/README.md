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
    │── POST /api/v1/auth/login ──────────────────────────►
    │   Body: { "firebase_token": "<Firebase ID Token>" } │
    │◄── 200 { "access_token": "<JWT>", "user": {...} } ──│
    │                                                     │
    │── GET /api/v1/circles (Bearer <JWT>) ───────────────►
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

### Step 2 — Backend login (server-side session)

Exchange the Firebase ID Token for a Halaqaty JWT:

```
POST /api/v1/auth/login
Content-Type: application/json

{
  "firebase_token": "<Firebase ID Token>"
}
```

**Success response (200):**
```json
{
  "access_token": "<Halaqaty JWT>",
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

Include the Halaqaty JWT in every subsequent request:

```
Authorization: Bearer <access_token>
```

The Go backend middleware validates the JWT signature and expiry on every request. If validation fails, the response is `401 ERR_UNAUTHENTICATED`.

### Token refresh

- Firebase tokens expire after **1 hour**. The Flutter client calls `user.getIdToken(refresh: true)` transparently.
- Halaqaty JWTs also expire after 1 hour. The Flutter client detects a `401` and calls `/api/v1/auth/refresh` with the current token.
- **30-day inactivity logout**: The backend tracks `last_active_at` per user. If the user is inactive for 30 days, the JWT is invalidated regardless of expiry.

```
POST /api/v1/auth/refresh
Authorization: Bearer <current_token>
```

### Logout

```
POST /api/v1/auth/logout
Authorization: Bearer <access_token>
```

This revokes the backend session. The Flutter client also calls `FirebaseAuth.instance.signOut()` locally.

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

> Full schemas are in [`docs/contracts/openapi.yaml`](../../contracts/openapi.yaml). This is a quick-reference index.

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `POST` | `/api/v1/auth/login` | None | Exchange Firebase token for Halaqaty JWT |
| `POST` | `/api/v1/auth/refresh` | Bearer | Refresh JWT |
| `POST` | `/api/v1/auth/logout` | Bearer | Revoke session |
| `GET` | `/api/v1/users/me` | Bearer | Get current user profile |
| `PATCH` | `/api/v1/users/me` | Bearer | Update profile (display name, timezone, FCM token) |
| `POST` | `/api/v1/circles` | Bearer | Create a circle (teacher role) |
| `GET` | `/api/v1/circles` | Bearer | List circles the user belongs to |
| `GET` | `/api/v1/circles/:id` | Bearer | Get circle details |
| `POST` | `/api/v1/circles/:id/invite` | Bearer (teacher) | Generate invite code |
| `POST` | `/api/v1/circles/:id/join` | Bearer | Join circle with invite code |
| `DELETE` | `/api/v1/circles/:id/members/:uid` | Bearer (teacher) | Remove member |
| `POST` | `/api/v1/sessions` | Bearer (teacher) | Start a live session |
| `PATCH` | `/api/v1/sessions/:id` | Bearer (teacher) | End/update session |
| `GET` | `/api/v1/sessions/:id/token` | Bearer | Get LiveKit room token |
| `POST` | `/api/v1/sessions/:id/ws-token` | Bearer | Get short-lived WebSocket token |
| `POST` | `/api/v1/sessions/:id/queue` | Bearer (teacher) | Add/reorder queue entries |
| `PATCH` | `/api/v1/sessions/:id/queue/:entry_id` | Bearer (teacher) | Advance queue / set status |
| `POST` | `/api/v1/sessions/:id/queue/reset` | Bearer (teacher) | Reset queue for new round |
| `GET` | `/api/v1/messages` | Bearer | List messages in a circle (paginated) |
| `POST` | `/api/v1/messages` | Bearer | Send a message |
| `GET` | `/api/v1/progress/:circle_id` | Bearer | Get student progress records |
| `GET` | `/healthz` | None | Health check |

---

*See [`docs/contracts/openapi.yaml`](../../contracts/openapi.yaml) for full request/response schemas, query parameters, and validation rules.*

