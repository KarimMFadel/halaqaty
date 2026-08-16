# F-005 Realtime Contract

## Topic authorization

- A 60-second ticket from `POST /api/v1/realtime/tickets` authorizes current eligible circle topics only.
- `session.started` is broadcast to eligible circle members.
- A successful authorized start/join subscribes the caller to that session topic; only joined participants receive its presence, hand, lock, mute/remove, and end events.
- The hub revalidates membership, current backend session, session state, removal, and subscription eligibility. Media credentials and room references are never events.

## Events

| Type | Audience | Required payload |
|---|---|---|
| `session.started` | circle topic | `session_id`, `circle_id` |
| `session.snapshot` | joined participant | `session`, `participants` |
| `session.participant_joined` / `session.participant_left` | session topic | `session_id`, `user_id`, `display_name`, `role` |
| `session.hand_raised` / `session.hand_lowered` | session topic | `session_id`, `participant_id`, `participant_name`, `hand_raised_at` (raise only) |
| `session.lock_changed` | session topic | `session_id`, `locked`, `changed_by` |
| `session.participant_muted` / `session.participant_removed` | session topic | `session_id`, `user_id`, `changed_by` |
| `session.ended` | session topic | `session_id`, `ended_by` nullable, `end_reason`, `duration_seconds` |

## Commands and delivery

- `cmd.raise_hand` and `cmd.lower_hand` carry `session_id`; any active participant may issue them.
- Events are at-least-once. Clients deduplicate by event ID when supplied, otherwise by `session_id`, type, affected user, and monotonic state version.
- The client sends `ping` every 30 seconds; three missed pongs make the connection dead.
- On reconnect, the client obtains a new ticket if required and retrieves the REST participant snapshot. Media reconnect uses the current connection only while it remains usable; otherwise it repeats authenticated start/join.
