# API Contracts

This directory contains the machine-readable and human-readable API contracts for Halaqaty.

## Contents

| File | Description |
|------|-------------|
| `openapi.yaml` | OpenAPI 3.0 specification for the REST API (`/api/v1/*`) |
| `ws_events.md` | WebSocket event catalogue (client ↔ server real-time events) |

## Principles

- **REST API** handles all CRUD operations and non-real-time commands (create circle, join queue, update profile, etc.).
- **WebSocket** handles low-latency real-time events (queue position updates, turn notifications, session state changes, chat delivery).
- Flutter Firebase Auth owns registration, sign-in, passwords, and ID-token refresh.
  The backend never accepts passwords or returns Firebase tokens. `POST /auth/register`
  and `POST /auth/sessions` require only a Firebase ID token; all other REST endpoints
  also require the opaque `X-Halaqaty-Session-ID` for the current backend device session.
- WebSocket connections are authenticated via a short-lived token exchanged over REST before the WS handshake.

## Base URL

| Environment | Base URL |
|-------------|----------|
| Production | `https://api.halaqaty.app/api/v1` |
| Staging | `https://staging.api.halaqaty.app/api/v1` |
| Local dev | `http://localhost:8080/api/v1` |

## Versioning Policy

- Current API version: **v1**
- Breaking changes require a new version prefix (`/api/v2/`).
- Non-breaking additions (new optional fields, new endpoints) do not require a version bump.
- Deprecated fields are marked in `openapi.yaml` with `deprecated: true` and removed after one full minor release cycle.

## How to Use

The canonical lint target is `make api-lint` (defined at repo root), which runs Spectral against this spec using `.spectral.yaml`:

```bash
make api-lint                    # lint docs/contracts/openapi.yaml with Spectral
```

To run Spectral directly (same command the Makefile invokes):

```bash
spectral lint docs/contracts/openapi.yaml
```

> **Note:** Go handlers are hand-written and centralized in `backend/cmd/api/routes.go` (see `AGENTS.md`). There is no OpenAPI → Go codegen pipeline.

## Error Codes

All error responses follow this envelope:

```json
{ "error": { "code": "ERR_...", "message": "human readable", "fields": { "field": "reason" } } }
```

| Code | HTTP Status | Meaning |
|------|-------------|---------|
| `ERR_UNAUTHORIZED` | 401 | Missing, invalid, or expired Firebase ID token |
| `ERR_SESSION_MISSING` | 401 | `X-Halaqaty-Session-ID` header absent |
| `ERR_SESSION_NOT_FOUND` | 401 | Session ID does not exist |
| `ERR_SESSION_EXPIRED` | 401 | Session exceeded inactivity (30d) or absolute (90d) TTL |
| `ERR_SESSION_REVOKED` | 401 | Session was explicitly revoked via logout |
| `ERR_SESSION_USER_MISMATCH` | 401 | Session belongs to a different Firebase identity |
| `ERR_FORBIDDEN` | 403 | Authenticated but lacks the required circle role |
| `ERR_NOT_FOUND` | 404 | Referenced resource does not exist |
| `ERR_CONFLICT` | 409 | Resource already exists (e.g. duplicate email from a different Firebase UID) |
| `ERR_VALIDATION_FAILED` | 400 | Request body or parameters failed validation; `fields` map contains per-field messages |
| `ERR_RATE_LIMIT_EXCEEDED` | 429 | Per-IP or per-user request budget exhausted |
| `ERR_REQUEST_TIMEOUT` | 503 | Handler exceeded the configured per-request timeout |
| `ERR_INTERNAL_SERVER_ERROR` | 500 | Unexpected server error |

Registration with an already-provisioned Firebase identity returns **409** with a valid `BackendSessionResponse` body (idempotent replay treated as success by the mobile client). Registration with a different Firebase UID but a conflicting email returns **409** with `ERR_CONFLICT` and no session body.

## Related Documents

- [ARCHITECTURE.md](../engineering/architecture/ARCHITECTURE.md) — Database schema, security model, LiveKit integration
- [DEVELOPMENT.md](../../DEVELOPMENT.md) — Development workflow, branching, and agent collaboration
- [TESTING_STRATEGY.md](../engineering/development/TESTING_STRATEGY.md) — How contracts are tested
