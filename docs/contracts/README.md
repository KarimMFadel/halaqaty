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

## Related Documents

- [ARCHITECTURE.md](../engineering/architecture/ARCHITECTURE.md) — Database schema, security model, LiveKit integration
- [DEVELOPMENT.md](../../DEVELOPMENT.md) — Development workflow, branching, and agent collaboration
- [TESTING_STRATEGY.md](../engineering/development/TESTING_STRATEGY.md) — How contracts are tested
