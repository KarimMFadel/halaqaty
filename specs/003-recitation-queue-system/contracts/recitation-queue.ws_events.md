# F-003 Recitation Queue WebSocket Events

Canonical catalog: `docs/contracts/ws_events.md`. All events use the existing authenticated F-005 session topic and are server-to-client projections only. PostgreSQL is authoritative; clients re-fetch `GET /sessions/{id}/queue` after reconnect, an unknown event, or any version gap.

## Common envelope

Every queue event includes a stable `event_id` and UTC `occurred_at`.
Round-scoped payloads include `session_id`, `round_id`, and the committed
monotonic round `version`; the session-scoped policy event carries its policy
version instead.

```json
{
  "type": "queue.entry_updated",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T10:15:30Z",
  "payload": {
    "session_id": "uuid",
    "round_id": "uuid",
    "version": 8
  }
}
```

Delivery is at-least-once. Clients deduplicate by `event_id`, ignore a version not newer than their snapshot, and re-fetch on a version gap. Events never contain a media credential, media endpoint, room reference, provider identifier, or URL carrying media material.

## Broadcast events

### `queue.state`

Full visibility-filtered queue snapshot sent after authorized subscription and
material reconciliation. Its payload matches REST `QueueState`, except entry
IDs use `queue_entry_id` for event-catalog clarity.

### `queue.round_started`

```json
{
  "type": "queue.round_started",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T10:15:30Z",
  "payload": {
    "session_id": "uuid",
    "round_id": "uuid",
    "round_number": 2,
    "round_type": "revision",
    "lifecycle": "active",
    "surah_id": 3,
    "from_ayah": 1,
    "to_ayah": 20,
    "grading_required": true,
    "version": 1
  }
}
```

### `queue.entry_updated`

```json
{
  "type": "queue.entry_updated",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T10:16:00Z",
  "payload": {
    "session_id": "uuid",
    "round_id": "uuid",
    "queue_entry_id": "uuid",
    "student_id": "uuid",
    "old_status": "waiting",
    "new_status": "reciting",
    "position": 1,
    "entry_version": 2,
    "version": 2
  }
}
```

`new_status` is one of `waiting`, `reciting`, `completed`, `skipped`, or `opted_out`. Grade/note are absent; authorized clients obtain their visibility-filtered projection through `queue.state` or REST.

### `queue.reordered`

```json
{
  "type": "queue.reordered",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T10:17:00Z",
  "payload": {
    "session_id": "uuid",
    "round_id": "uuid",
    "order_kind": "queue_entries",
    "ordered_ids": ["uuid1", "uuid2"],
    "version": 3
  }
}
```

### `queue.advanced`

```json
{
  "type": "queue.advanced",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T10:18:00Z",
  "payload": {
    "session_id": "uuid",
    "round_id": "uuid",
    "selected_entry_id": "uuid",
    "version": 4
  }
}
```

Selection does not change the selected entry from `waiting` to `reciting`.

### `queue.round_finalized`

```json
{
  "type": "queue.round_finalized",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T11:00:00Z",
  "payload": {
    "session_id": "uuid",
    "round_id": "uuid",
    "round_number": 1,
    "reason": "reset",
    "version": 12
  }
}
```

`reason` is `reset` or `session_ended`. Finalization never means the F-005 session end waited for queue cleanup.

### `queue.policy_changed`

```json
{
  "type": "queue.policy_changed",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T10:10:00Z",
  "payload": {
    "session_id": "uuid",
    "policy": {
      "population": "present_at_activation",
      "unfinished_finalization": "mark_unfinished_skipped",
      "opt_out": "approval_required",
      "grade_visibility": "managers_and_student",
      "grade_correction": "audited_any_time",
      "version": 2
    }
  }
}
```

### `queue.grade_submitted`

Visibility-filtered targeted/broadcast projection after completion or an allowed correction. The server sends it only to recipients allowed by the current grade visibility policy.

```json
{
  "type": "queue.grade_submitted",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T10:30:00Z",
  "payload": {
    "session_id": "uuid",
    "round_id": "uuid",
    "queue_entry_id": "uuid",
    "student_id": "uuid",
    "grade": "excellent",
    "notes": "Strong tajweed",
    "entry_version": 3,
    "version": 7
  }
}
```

The event omits `graded_by`; actor attribution remains in redacted audit telemetry. Notes are never logged by delivery infrastructure.

## Targeted events

### `queue.your_turn`

Sent to the selected student's authorized devices after the entry becomes `reciting`. It is the stable future FCM deduplication source.

```json
{
  "type": "queue.your_turn",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T10:20:00Z",
  "payload": {
    "session_id": "uuid",
    "round_id": "uuid",
    "queue_entry_id": "uuid",
    "round_type": "revision",
    "surah_id": 2,
    "from_ayah": 1,
    "to_ayah": 10,
    "version": 5
  }
}
```

### `queue.next_soon`

Sent to the next waiting student after order/selection changes. It contains no timer or estimated wait.

```json
{
  "type": "queue.next_soon",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T10:20:00Z",
  "payload": {
    "session_id": "uuid",
    "round_id": "uuid",
    "queue_entry_id": "uuid",
    "position": 2,
    "version": 5
  }
}
```

### `queue.opt_out_requested`

Sent only to current teachers/supervisors when approval is required.

```json
{
  "type": "queue.opt_out_requested",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T10:22:00Z",
  "payload": {
    "session_id": "uuid",
    "round_id": "uuid",
    "request_id": "uuid",
    "queue_entry_id": "uuid",
    "student_id": "uuid",
    "version": 5
  }
}
```

## Errors and recovery

REST commands use the canonical error envelope and HTTP statuses. Existing WebSocket `error` remains transport/command-only; F-003 clients do not mutate queue state through WebSocket. Event or notification failure never rolls back a committed queue action. On reconnect, version gap, unknown event, or ambiguous duplicate, the client re-fetches `GET /sessions/{id}/queue`.
