# System Design Documentation

Detailed system design, data flow diagrams, component specifications, and technical deep-dives.

> For the authoritative architecture overview, database schema, and security model, see [`ARCHITECTURE.md`](../architecture/ARCHITECTURE.md). This document covers runtime lifecycle flows that are too dynamic to express in static architecture diagrams.

---

## Queue Lifecycle

The **Recitation Queue** is the core differentiator of Halaqaty. This section describes the complete lifecycle of a queue from creation to teardown.

### States

```
Session created
      │
      ▼
  [no_queue] ──── teacher calls POST /sessions/:id/queue ────►
      │
      ▼
  [pending] ─── teacher calls PATCH /sessions/:id (start) ──►
      │
      ▼
  [active] ────── teacher calls PATCH /queue/:entry (advance) ──►
      │                  │                    │
      │           [reciting]            [completed/skipped]
      │                                       │
      │◄── teacher calls POST /queue/reset ───┘
      │
  [active, round N+1]
      │
      ▼
  [closed] ─── session ends or teacher calls PATCH /sessions/:id (end)
```

### Queue Entry States

Each entry in the queue progresses through:

| State | Meaning | Trigger |
|-------|---------|---------|
| `waiting` | Student is in the queue, not yet their turn | Default on join |
| `reciting` | Currently reciting — LiveKit `CanPublish: true` granted | Teacher advances queue |
| `completed` | Finished — grade recorded | Teacher marks complete |
| `skipped` | Absent or skipped by teacher | Teacher marks skip |

**Only one entry can be `reciting` at a time.** The Go backend enforces this as a database constraint.

### Turn Transition (detailed)

When the teacher calls `PATCH /api/v1/sessions/:id/queue/:entry_id` to advance:

```
Go Backend
  │
  ├─ 1. Validate: caller is teacher of this circle
  ├─ 2. DB transaction:
  │     a. Set current `reciting` entry → `completed` or `skipped`
  │     b. Set next `waiting` entry → `reciting`
  │     c. Revoke LiveKit CanPublish from previous student token
  │     d. Grant LiveKit CanPublish to next student (new token issued)
  ├─ 3. Broadcast `queue.entry_updated` WS event to all session clients
  ├─ 4. Broadcast `session.turn_started` WS event (to next student only)
  └─ 5. Trigger FCM push ("Your turn to recite!") to next student
```

See [`docs/contracts/ws_events.md`](../../contracts/ws_events.md) for the full event schema.

### Queue Reset (new round)

`POST /api/v1/sessions/:id/queue/reset` with body:
```json
{
  "surah_number": 2,
  "start_ayah": 1,
  "end_ayah": 10,
  "label": "مراجعة"
}
```

This:
1. Sets all current round entries to read-only (archived)
2. Creates a new round record linked to the session
3. Clones the member list back into `waiting` state
4. Broadcasts `queue.state` (full snapshot) to all clients

**Multiple passes are fully supported.** Each round is an independent unit with its own Surah range and set of progress records.

---

## Session Lifecycle

A live session ties together LiveKit audio, the recitation queue, and WebSocket real-time delivery.

### States

```
[scheduled] → [active] → [idle_timeout] → [ended]
```

| State | Description |
|-------|-------------|
| `scheduled` | Session is planned; no LiveKit room exists yet |
| `active` | Teacher started — LiveKit room exists, WebSocket broadcasting |
| `idle_timeout` | Last participant left; room auto-closes after 30 minutes |
| `ended` | Teacher explicitly ended; progress records finalized |

### Session Start

`POST /api/v1/sessions` (teacher only):

```
Go Backend
  │
  ├─ 1. Create `sessions` DB record with status = active
  ├─ 2. Call LiveKit API to create room (name = session UUID)
  ├─ 3. Disable video publishing at room level
  ├─ 4. Return session ID to client
  └─ 5. Broadcast `session.started` WS event to circle members
```

### Joining Audio

`GET /api/v1/sessions/:id/token`:

```
Go Backend
  │
  ├─ 1. Validate caller is a member of the circle
  ├─ 2. Check session status = active
  ├─ 3. Generate LiveKit token:
  │     - CanPublish: false (default — overridden per-turn for active reciter)
  │     - CanSubscribe: true
  │     - CanPublishVideo: false (always)
  │     - Expiry: session max duration (4 hours)
  └─ 4. Return token to client (Flutter calls livekit_client to connect)
```

### Session End

`PATCH /api/v1/sessions/:id` with `{"status": "ended"}`:

```
Go Backend
  │
  ├─ 1. Set session status = ended in DB
  ├─ 2. Close LiveKit room (all participants disconnected)
  ├─ 3. Finalize any in-progress queue entry as skipped
  ├─ 4. Persist all progress records from queue history
  └─ 5. Broadcast `session.ended` WS event; clients disconnect gracefully
```

### Idle Timeout

LiveKit sends a webhook when the last participant leaves a room. The Go backend:
1. Records `last_empty_at` timestamp on the session
2. Starts a 30-minute countdown (via a scheduled check)
3. If no participant rejoins within 30 minutes, transitions session to `idle_timeout` → `ended`

---

## WebSocket Connection Lifecycle

```
Client                       Go Backend
  │                              │
  ├─ POST /sessions/:id/ws-token ►│
  │◄─ { "ws_token": "...", "expires_in": 60 } ─│
  │                              │
  ├─ wss://api.../ws?token=<ws_token> ─────────►│
  │◄─ HTTP 101 Switching Protocols ─────────────│
  │                              │
  │◄─ { "type": "connection.ready" } ───────────│
  │                              │
  │  [30s interval]              │
  ├─ { "type": "ping" } ───────────────────────►│
  │◄─ { "type": "pong", "server_time": "..." } ─│
  │                              │
  │  [on relevant event]         │
  │◄─ { "type": "queue.entry_updated", ... } ───│
  │                              │
  ├─ { "type": "disconnect" } ─────────────────►│ (graceful close)
```

**Max connections per user:** 3 active WebSocket connections (enforced server-side). Excess connections are rejected with close code `4001`.

**Reconnection:** The Flutter client uses exponential backoff (1s, 2s, 4s, max 30s) for automatic reconnection. On reconnect, the client calls `queue.state` and `session.state` to resync.

---

*For WS event schemas, see [`docs/contracts/ws_events.md`](../../contracts/ws_events.md). For database tables, see [`ARCHITECTURE.md §4`](../architecture/ARCHITECTURE.md).*

