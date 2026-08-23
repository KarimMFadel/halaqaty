# F-003 Recitation Queue WebSocket Events

Canonical catalog: `docs/contracts/ws_events.md`. All events use the existing authenticated F-005 session topic and are server-to-client projections only. PostgreSQL is authoritative; clients re-fetch `GET /sessions/{id}/queue` after reconnect, an unknown event, or any version gap. Regenerated 2026-08-23 from the approved spec clarifications; pending canonical sync deltas are listed in `plan.md` §Canonical sync.

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

Delivery is at-least-once (initial attempt plus 5 exponential-backoff retries, then parked for operator replay — never silently dropped). Clients deduplicate by `event_id`, ignore a version not newer than their snapshot, and re-fetch on a version gap. Events never contain a media credential, media endpoint, room reference, provider identifier, or URL carrying media material. F-003 emits no FCM payloads; F-008 may later project these durable event IDs through Firebase.

## Broadcast events

### `queue.state`

Full visibility-filtered queue snapshot sent after authorized subscription and
material reconciliation. Its payload matches REST `QueueState`, except entry
IDs use `queue_entry_id` for event-catalog clarity. The `preorder` projection is
included for managers only; other participants receive an empty array.

### `queue.round_started`

Emitted when a prepared round activates. Activation is automatic in round-number
order: the first prepared round activates when the session is live and no round
is active, and each subsequent prepared round activates when the previous round
finalizes (including the round created by reset). No manager activate action
exists.

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
    "surah_name": "Al-Imran",
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

Broadcast after a manager changes the durable order, by either control:

- `preorder_students` — full-list replace of pre-set candidates (round `prepared` only);
- `entry_move` — one `waiting` entry repositioned in the active round (permitted while another entry recites; the `reciting` entry is never moved).

`ordered_ids` is the resulting complete order (candidate student IDs for
`preorder_students`; the full round entry order for `entry_move`).

```json
{
  "type": "queue.reordered",
  "event_id": "uuid",
  "occurred_at": "2026-08-23T10:17:00Z",
  "payload": {
    "session_id": "uuid",
    "round_id": "uuid",
    "order_kind": "entry_move",
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

Selection does not change the selected entry from `waiting` to `reciting`. A
replacing advance emits this event again with the new `selected_entry_id`.
Rejected advances (zero waiting entries; an entry already `reciting`) mutate
nothing and emit no event — the actor receives the REST conflict.

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

`reason` is `reset` or `session_ended`. Session-end convergence finalizes the
active round and every never-activated prepared round (permanently inert,
retained); each emits this event with reason `session_ended`. Finalization never
means the F-005 session end waited for queue cleanup.

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

Workflow-policy changes apply to subsequent actions only; grade/note
visibility-policy changes apply immediately and prospectively to new snapshots
and events. Delivered history is never rewritten; clients re-fetch the current
queue state on this event (FR-009 pattern) so stale or out-of-order grade
visibility projections self-heal.

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

The event omits `graded_by`; actor attribution remains in redacted audit telemetry. Notes are never logged by delivery infrastructure. For a grading-optional completion the first projection carries no grade; later grade/note additions or changes arrive through the same event after an allowed correction.

## Targeted events

### `queue.your_turn`

Sent to the selected student's authorized devices after the entry becomes `reciting`. It is the stable future FCM deduplication source (F-008 projects it; F-003 sends no push).

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
    "surah_name": "Al-Baqarah",
    "from_ayah": 1,
    "to_ayah": 10,
    "version": 5
  }
}
```

### `queue.next_soon`

Sent to the next waiting student after order/selection changes. It contains no timer or estimated wait (no per-student timer in MVP).

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

Sent only to current teachers/supervisors when approval is required. Auto-approved opt-outs do not emit this event.

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
