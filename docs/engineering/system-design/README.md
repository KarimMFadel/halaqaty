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
      │   GET queue → POST advance → PUT entry status=reciting       │
      │   PUT entry status=completed|skipped (grade atomic when required)
      │                                                              │
      │◄── next round: POST /sessions/{sessionId}/queue/reset ───────┘
      │
      ▼  teacher calls POST /sessions/{sessionId}/end
   [ended]
```

Session `status` values are codified in `openapi.yaml` (path `/circles/{circleId}/sessions`, query `?status=` enum): `scheduled` · `active` · `ended`.

### Queue Entry States

| State | Meaning | Trigger |
|-------|---------|---------|
| `waiting` | Student is in the queue, not yet their turn | Default when a round starts (`POST /sessions/{sessionId}/queue/rounds`) |
| `reciting` | Currently reciting | Manager selects with `POST .../advance`, then starts with `PUT .../status` |
| `completed` | Finished; one practice record exists | Manager uses `PUT .../status` with atomic conditional grade/note |
| `skipped` | Turn ended without practice credit | Manager uses `PUT .../status`; default reset also skips unfinished entries |
| `opted_out` | Approved/auto-approved humane opt-out | Student calls `POST .../opt-out`; manager decision is separate when required |

`completed`, `skipped`, and `opted_out` are terminal. The existing
`POST .../grade` route corrects a completed grade/note under session policy; it
does not perform the completion transition or implicitly advance.

### Queue Reset (new round)

`POST /api/v1/sessions/{sessionId}/queue/reset` (teacher/supervisor only) with body referencing `ResetQueueRequest` (see `openapi.yaml` for the schema). This:

1. Archives the current round's entries (read-only).
2. Creates the next sequential round linked to the session.
3. Applies the configured population policy and durable pre-set relative order.
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
| `scheduled` | Session is planned; no media room exists yet (`POST /circles/{circleId}/sessions` returns this status) |
| `active` | Teacher started — media room exists through the configured adapter (LiveKit in MVP), WebSocket broadcasting (`POST /sessions/{sessionId}/start`) |
| `ended` | Teacher explicitly ended; progress records finalized (`POST /sessions/{sessionId}/end`) |

Additional lifecycle states (`idle_timeout`) and a `last_empty_at` timestamp are described in the [Proposed](#proposed-not-yet-in-contract-or-architecture) section.

### Session Start

`POST /api/v1/sessions/{sessionId}/start` (teacher only):

```
Go Backend
  │
  ├─ 1. Lock and validate the scheduled session; keep it non-joinable
  ├─ 2. Idempotently ensure a deterministic room through SessionMediaGateway (LiveKit adapter in MVP)
  ├─ 3. Atomically persist status = active + media_room_ref
  ├─ 4. Issue required participant MediaConnection (video publishing disabled)
  └─ 5. Broadcast metadata-only `session.started` after commit (no endpoint, credential, or room ref)
```

If the activation commit fails after room creation, close the orphan room. A
sessions-owned reconciler retries orphan cleanup and missing-room repair after
process crashes. Repeating start for an already-active session returns the same
`Session` with a newly issued caller-specific `MediaConnection`.

The broadcast only marks the session available in subscribed clients. A student
joins only after choosing Join and completing the authorized REST join flow below;
the event itself neither authorizes entry nor carries connection material.

### Joining Audio

`POST /api/v1/sessions/{sessionId}/join` (any circle member):

```
Go Backend
  │
  ├─ 1. Validate caller is a member of the circle
  ├─ 2. Check session status = active (else 409 "Session is not active")
  ├─ 3. Issue participant MediaConnection through the LiveKit MVP adapter:
  │     - CanPublish: true (authorized student audio participation)
  │     - CanSubscribe: true
  │     - CanPublishVideo: false (always)
  │     - Credential expiry: at most 1 hour, independent of the 4-hour session maximum
  └─ 4. Return endpoint, opaque credential, and expiry to the caller (Flutter LiveKit adapter connects)
```

Near or after credential expiry, the client repeats the authorized join operation
to receive a fresh `MediaConnection`. Ended, removed, or revoked participants do
not receive replacement credentials.

### Session End

`POST /api/v1/sessions/{sessionId}/end` (teacher only):

```
Go Backend
  │
  ├─ 1. Set session status = ended in DB, immediately blocking new joins
  ├─ 2. Close the media room idempotently through SessionMediaGateway
  ├─ 3. Retry/reconcile provider cleanup after timeout or process crash
  └─ 4. Broadcast `session.ended`; clients disconnect gracefully
```

Queue turn cleanup and progress persistence remain owned by F-003/F-007 and react
through their integration boundary; F-005 does not write their tables.

---

## WebSocket Connection Lifecycle (contract-anchored)

Codified in [`docs/contracts/ws_events.md`](../../contracts/ws_events.md) §Connection lifecycle:

```
Client                       Go Backend
  │                              │
  ├─ POST /realtime/tickets ─────────────►│  (requires Firebase bearer + session ID)
  │◄─ { "token": "...", "expires_at": "..." } ─│  (token valid for 60 seconds)
  │                              │
  ├─ wss://api.halaqaty.app/ws?token=<ticket> ───────────►│
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

- Realtime ticket lifetime: **60 seconds** (`POST /realtime/tickets`); it authorizes only current eligible circle and session topics, which the hub revalidates.
- Heartbeat: client sends `ping` every **30 seconds**; server replies `pong` with `server_time`.
- **Dead-connection detection:** 3 missed `pong` responses ⇒ client reconnects.
- **Reconnection source of truth:** PostgreSQL is always the source of truth. On reconnection, clients re-fetch state via REST (`GET /api/v1/sessions/{id}/queue`, etc.) rather than relying on buffered WebSocket events.

---

## Proposed (not yet in contract or ARCHITECTURE)

The items below are **design proposals** documented for review. They are NOT part of the current REST or WebSocket contract and are NOT codified in `ARCHITECTURE.md`. Each must be ratified via an ADR (and reflected in `openapi.yaml` / `ws_events.md`) before implementation.

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
