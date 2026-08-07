# System Design Documentation

> **Status banner.** This document covers runtime lifecycle flows — too dynamic to express in static architecture diagrams. It is split into two parts:
>
> - **Contract-anchored sections** below this banner reflect the endpoint shapes and event names that are codified in [`docs/contracts/openapi.yaml`](../../contracts/openapi.yaml) and [`docs/contracts/ws_events.md`](../../contracts/ws_events.md). Where this document and those contracts disagree, **the contracts win**.
> - **[Proposed (not yet in contract or ARCHITECTURE)](#proposed-not-yet-in-contract-or-architecture)** collects forward-looking design proposals (`idle_timeout` session state, concurrent-WS connection limit, client reconnection parameters, initial handshake event). They are **not** part of the current REST/WS contract and must be codified via an ADR (and reflected in `openapi.yaml` / `ws_events.md`) before implementation.

For the authoritative architecture overview, database schema, and security model, see [`ARCHITECTURE.md`](../architecture/ARCHITECTURE.md).

---

## Queue Lifecycle

The **Recitation Queue** is the core differentiator of Halaqaty.

### States (contract-anchored)

```
Circle has scheduled sessions
      │
      ▼  teacher calls POST /circles/{circleId}/sessions
   [scheduled] ── teacher calls POST /sessions/{sessionId}/start ──►
      │
      ▼
   [active] ── teacher/supervisor calls POST /sessions/{sessionId}/queue/rounds ──►
      │                                                              │
      │   (queue entries visible to all members via GET /sessions/{sessionId}/queue)
      │                                                              │
      │   teacher/supervisor calls POST /sessions/{sessionId}/queue/entries/{entryId}/grade
      │                                                              │
      │◄── next round: POST /sessions/{sessionId}/queue/rounds ──────┘
      │
      ▼  teacher calls POST /sessions/{sessionId}/end
   [ended]
```

Session `status` values are codified in `openapi.yaml` (path `/circles/{circleId}/sessions`, query `?status=` enum): `scheduled` · `active` · `ended`.

### Queue Entry States

| State | Meaning | Trigger |
|-------|---------|---------|
| `waiting` | Student is in the queue, not yet their turn | Default when a round starts (`POST /sessions/{sessionId}/queue/rounds`) |
| `reciting` | Currently reciting | Per-turn advancement (see [Proposed](#turn-advancement-flow) — exact endpoint shape not yet in contract) |
| `completed` | Finished — grade recorded | Teacher/supervisor submits grade: `POST /sessions/{sessionId}/queue/entries/{entryId}/grade` |
| `opted_out` | Student opted out (requires teacher/supervisor approval) | Student calls `POST /sessions/{sessionId}/queue/opt-out` |

> **Note:** only `completed` and `opted_out` transitions are codified by REST endpoints today. The `waiting` → `reciting` → `completed` turn-advance step is described in the [Proposed](#turn-advancement-flow) section; its exact endpoint is not in `openapi.yaml` yet.

### Queue Reset (new round)

`POST /api/v1/sessions/{sessionId}/queue/rounds` (teacher/supervisor only) with body referencing `StartRoundRequest` (see `openapi.yaml` for the schema). This:

1. Archives the current round's entries (read-only).
2. Creates a new round record linked to the session.
3. Clones the member list back into `waiting` state.
4. Returns the full `QueueState` snapshot.

**Multiple passes are fully supported.** Each round is an independent unit with its own Surah range and set of progress records.

See [`docs/contracts/ws_events.md`](../../contracts/ws_events.md) for the broadcast events (`queue.state`, `queue.round_started`, `queue.entry_updated`, `queue.grade_submitted`, etc.).

---

## Session Lifecycle

A live session ties together LiveKit audio, the recitation queue, and WebSocket real-time delivery.

### States (contract-anchored)

```
[scheduled] → [active] → [ended]
```

| State | Description |
|-------|-------------|
| `scheduled` | Session is planned; no LiveKit room exists yet (`POST /circles/{circleId}/sessions` returns this status) |
| `active` | Teacher started — LiveKit room exists, WebSocket broadcasting (`POST /sessions/{sessionId}/start`) |
| `ended` | Teacher explicitly ended; progress records finalized (`POST /sessions/{sessionId}/end`) |

Additional lifecycle states (`idle_timeout`) and a `last_empty_at` timestamp are described in the [Proposed](#proposed-not-yet-in-contract-or-architecture) section.

### Session Start

`POST /api/v1/sessions/{sessionId}/start` (teacher only):

```
Go Backend
  │
  ├─ 1. Set session status = active in DB
  ├─ 2. Call LiveKit API to create room (name = session UUID)
  ├─ 3. Disable video publishing at room level
  ├─ 4. Return SessionStartResponse (LiveKit token to caller)
  └─ 5. Broadcast `session.started` WS event to circle members
```

### Joining Audio

`POST /api/v1/sessions/{sessionId}/join` (any circle member):

```
Go Backend
  │
  ├─ 1. Validate caller is a member of the circle
  ├─ 2. Check session status = active (else 409 "Session is not active")
  ├─ 3. Generate LiveKit token:
  │     - CanPublish: false (default — overridden per-turn for active reciter)
  │     - CanSubscribe: true
  │     - CanPublishVideo: false (always)
  │     - Expiry: session max duration (4 hours)
  └─ 4. Return token to client (Flutter calls livekit_client to connect)
```

### Session End

`POST /api/v1/sessions/{sessionId}/end` (teacher only):

```
Go Backend
  │
  ├─ 1. Set session status = ended in DB
  ├─ 2. Close LiveKit room (all participants disconnected)
  ├─ 3. Finalize any in-progress queue entry
  ├─ 4. Persist all progress records from queue history
  └─ 5. Broadcast `session.ended` WS event; clients disconnect gracefully
```

---

## WebSocket Connection Lifecycle (contract-anchored)

Codified in [`docs/contracts/ws_events.md`](../../contracts/ws_events.md) §Connection lifecycle:

```
Client                       Go Backend
  │                              │
  ├─ POST /sessions/{sessionId}/ws-token ►│  (requires Firebase bearer + session ID)
  │◄─ { "token": "...", "expires_at": "..." } ─│  (token valid for 60 seconds)
  │                              │
  ├─ wss://api.halaqaty.app/ws?token=<ws_token> ─────────►│
  │◄─ HTTP 101 Switching Protocols ──────────────────────│
  │                              │
  │  [every 30 seconds]          │
  ├─ { "type": "ping" } ─────────────────────────────────►│
  │◄─ { "type": "pong", "server_time": "..." } ───────────│
  │                              │
  │  [on relevant event]         │
  │◄─ { "type": "queue.entry_updated", ... } ─────────────│
  │                              │
  ├─ { "type": "disconnect" }  (or 3 missed pongs) ──────►│  (client reconnects automatically)
```

Codified rules per `ws_events.md`:

- WS token lifetime: **60 seconds** (`POST /sessions/{sessionId}/ws-token`).
- Heartbeat: client sends `ping` every **30 seconds**; server replies `pong` with `server_time`.
- **Dead-connection detection:** 3 missed `pong` responses ⇒ client reconnects.
- **Reconnection source of truth:** PostgreSQL is always the source of truth. On reconnection, clients re-fetch state via REST (`GET /api/v1/sessions/{id}/queue`, etc.) rather than relying on buffered WebSocket events.

---

## Proposed (not yet in contract or ARCHITECTURE)

The items below are **design proposals** documented for review. They are NOT part of the current REST or WebSocket contract and are NOT codified in `ARCHITECTURE.md`. Each must be ratified via an ADR (and reflected in `openapi.yaml` / `ws_events.md`) before implementation.

### Turn advancement flow

The turn-advance step (`waiting` → `reciting` → `completed`/`opted_out`) shown in the queue lifecycle above needs an explicit REST or WS command to advance the active reciter. Open questions:

- Should advancement be a `PATCH /sessions/{sessionId}/queue/entries/{entryId}` (status change) or a WS command (`cmd.advance_queue`)?
- Should `POST /sessions/{sessionId}/queue/entries/{entryId}/grade` implicitly advance the queue pointer, or is advancement a separate operation?

Once decided, the chosen shape must be added to `openapi.yaml` (REST) or `ws_events.md` (WS command) and this section collapsed into the contract-anchored text above.

### Concurrent WebSocket connection limit

**Proposal:** each user may hold a maximum of **3 simultaneous WebSocket connections**. Excess connections are rejected with close code `4001`.

**Status:** not in `ws_events.md`; close code `4001` is currently undocumented. Needs to be added to the WS catalog (and to `ARCHITECTURE.md`) before implementation.

### Client reconnection parameters

**Proposal:** the Flutter client uses exponential backoff **1 s → 2 s → 4 s → … → max 30 s** for automatic reconnection.

**Status:** `ws_events.md` only codifies the dead-connection rule (3 missed pongs) and the REST re-fetch on reconnect; the specific backoff schedule is a client-policy proposal. Belongs in a Flutter client runbook/ADR rather than the WS contract.

### `idle_timeout` session state and `last_empty_at`

**Proposal:**

```
[active] → [idle_timeout] → [ended]
```

| Proposed state | Description |
|-------|-------------|
| `idle_timeout` | Last participant left; room auto-closes after 30 minutes |

- LiveKit sends a webhook when the last participant leaves a room.
- The Go backend records a `last_empty_at` timestamp on the session.
- A 30-minute countdown (via a scheduled check) transitions `idle_timeout` → `ended` if no participant rejoins.

**Status:** not in `openapi.yaml` (`?status=` enum is only `scheduled|active|ended`), not in `ARCHITECTURE.md`, and no `last_empty_at` column exists in the documented schema. Requires an ADR + `openapi.yaml` status-enum change + schema ADR before implementation.

### Initial handshake event (`connection.ready`)

**Proposal:** on WS upgrade, the server emits `{"type": "connection.ready"}` to the client before any other events.

**Status:** `connection.ready` is not listed in the `ws_events.md` event catalog. Either add it to the catalog or drop it (reliance on the first `pong` as the implicit ready signal is a viable alternative).

---

*For WS event schemas, see [`docs/contracts/ws_events.md`](../../contracts/ws_events.md). For database tables, see [`ARCHITECTURE.md`](../architecture/ARCHITECTURE.md). For contractually defined endpoint shapes, see [`docs/contracts/openapi.yaml`](../../contracts/openapi.yaml).*