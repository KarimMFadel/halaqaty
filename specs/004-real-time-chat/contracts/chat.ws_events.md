# F-004 Chat WebSocket Contract

This is the feature overlay for the canonical `docs/contracts/ws_events.md` catalog. Chat reuses the authenticated WebSocket connection and authorized `circle.{circle_id}` topics from F-005; LiveKit carries audio only and is not a chat transport. Delivery is at least once, so clients deduplicate durable events by `event_id` and reconcile messages through REST.

## Server envelope

Every server event uses:

```json
{
  "type": "chat.message",
  "event_id": "uuid",
  "occurred_at": "2026-09-03T12:00:00Z",
  "payload": {}
}
```

## `chat.message` (server to client)

Emitted after durable acceptance. A group event is delivered only to currently authorized subscribers of `circle.{circle_id}`. A direct event is sent to the eligible pair's authenticated user connections without selecting or disclosing one qualifying circle. Immediately before every write, the server revalidates that connection's backend session and current group/DM authorization from PostgreSQL. The payload is the canonical REST `Message` projection and therefore uses only server-authoritative `delivered` or `read`; the client-local `sent` state ends when REST acceptance is confirmed.

## `chat.message_deleted` (server to client)

```json
{
  "type": "chat.message_deleted",
  "event_id": "uuid",
  "occurred_at": "2026-09-03T12:01:00Z",
  "payload": {
    "message_id": "uuid",
    "circle_id": "uuid-or-null",
    "dm_peer_id": "uuid-or-null",
    "deleted_at": "2026-09-03T12:01:00Z"
  }
}
```

Clients remove message content, media access, and reply-preview content from their projection. The event never exposes an object key or deletion reason.

## `chat.message_read` (server to sender)

```json
{
  "type": "chat.message_read",
  "event_id": "uuid",
  "occurred_at": "2026-09-03T12:02:00Z",
  "payload": {
    "message_id": "uuid",
    "reader_id": "uuid",
    "read_at": "2026-09-03T12:02:00Z"
  }
}
```

Targeted only to the message sender after the read fact is stored idempotently. It is not a broadcast receipt for every circle member.

## `cmd.chat.typing` (client to server)

```json
{
  "type": "cmd.chat.typing",
  "request_id": "uuid",
  "payload": {
    "circle_id": "uuid-or-null",
    "dm_peer_id": "uuid-or-null",
    "is_typing": true
  }
}
```

Exactly one context field is required. The server rechecks current authorization, rate limits the command, and does not persist it.

## `chat.typing` (server to client)

```json
{
  "type": "chat.typing",
  "event_id": "uuid",
  "occurred_at": "2026-09-03T12:02:05Z",
  "payload": {
    "user_id": "uuid",
    "circle_id": "uuid-or-null",
    "dm_peer_id": "uuid-or-null",
    "is_typing": true,
    "expires_at": "2026-09-03T12:02:10Z"
  }
}
```

Receivers clear the indicator at `expires_at` (five seconds) even if a stop event is lost.

## Reliability and authorization

- Durable message, deletion, and read events are written to `chat_event_outbox` in the same PostgreSQL transaction as their authoritative state and retried with bounded exponential backoff and jitter.
- The worker parks an event after five failed deliveries and exposes retry, parking, and lag metrics without message content or participant identifiers.
- Every delivery attempt rebuilds its audience and reauthorizes each client/session immediately before write; a stale snapshot never authorizes delivery. Group events use circle topics, while DM events target authenticated eligible-user connections directly.
- Reconnects re-authorize subscriptions and reconcile by REST cursor; revoked sessions, removed members, and newly ineligible DM peers receive no subsequent event.
- F-004 creates no Firebase/FCM trigger. F-008 owns every background or closed-app notification projection.
