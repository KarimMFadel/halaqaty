# Data Model: Live Sessions (LiveKit)

## F-005-owned `sessions` table

F-005 owns the complete base `sessions` table because no earlier feature migration
currently creates it. F-003 and F-006 extend the table only through later paired
migrations for queue and attendance concerns; they must not recreate or alter the
F-005 lifecycle invariants.

| Field | Type | Constraint | Purpose |
|---|---|---|---|
| `id` | UUID | primary key | Opaque session identity |
| `circle_id` | UUID | FK → `circles.id`, not null | Circle scope |
| `created_by` | UUID | FK → `users.id`, not null | Creator audit attribution |
| `status` | VARCHAR(20) | `scheduled`, `active`, `ended` | Lifecycle state |
| `scheduled_at` | TIMESTAMPTZ | nullable; F-005 rows must be null | F-006 scheduling extension point |
| `actual_start` | TIMESTAMPTZ | nullable | Authoritative activation time |
| `actual_end` | TIMESTAMPTZ | nullable | Authoritative end time |
| `end_reason` | VARCHAR(20) | null or `manual`, `duration_limit`, `idle_timeout` | Durable end attribution |
| `media_mode` | VARCHAR(20) | not null, `audio_only` for F-005 | Server-controlled media policy |
| `media_room_ref` | VARCHAR(200) | unique, nullable until active | Opaque provider reference; never public |
| `is_locked` | BOOLEAN | not null, default false | Blocks new joins |
| `participant_count` | INTEGER | not null, 0..50 | Current durable count |
| `created_at`, `updated_at` | TIMESTAMPTZ | not null | Audit and synchronization timestamps |

F-005 creates only ad-hoc rows (`scheduled_at IS NULL`) in `scheduled` status. The canonical circle-session discovery response may show `scheduled` and `active` rows; only `active` rows are joinable. Starting is always explicit. F-006 owns scheduled/recurring creation and attendance policy; it may add its own schedule/attendance tables without changing F-005's session state machine.

F-005 creates only ad-hoc rows (`scheduled_at IS NULL`). Existing fields remain compatible; no applied migration is edited.

## `session_participant_presence`

| Field | Type | Constraint | Purpose |
|---|---|---|---|
| `id` | UUID | primary key | Presence record identity |
| `session_id`, `user_id` | UUID | unique pair, foreign keys | One durable participant record per session |
| `first_joined_at`, `last_joined_at`, `last_left_at` | TIMESTAMPTZ | nullable | Join/leave timeline |
| `reconnect_count` | INTEGER | not null, default 0 | Rejoins after the first join |
| `is_currently_present` | BOOLEAN | not null, default false | Authoritative current presence |
| `removed_at` | TIMESTAMPTZ | nullable | Prevents future join/reconnect |
| `hand_raised_at` | TIMESTAMPTZ | nullable | Current hand state; null is lowered |

`session_attendance` is not created or written by F-005. F-006 consumes this table’s facts to define attendance later.

## Invariants and transactions

1. A session changes only `scheduled → active → ended`; active/ended transitions use compare-and-set row updates.
2. Activation persists the room reference only after ensure-room succeeds; on failure, the session stays non-joinable and the orphan is closed.
3. Join locks the session/presence decision, rejects capacity above 50, and increments count only for a transition to current presence.
4. Lock rejects a first join but permits an eligible existing pre-lock presence record to reconnect. Removed, ended, or unauthorized participants never reconnect.
5. End sets `actual_end` and `end_reason` before provider close. It clears only current-presence/hand state and retains history.
6. Presence/webhook handling is idempotent using the unique pair, state transitions, and provider event identity where available.
7. Provider room refs, credentials, Firebase tokens, and backend session IDs are not persisted in audit payloads or public projections.

## Migration approach

- Add the next sequential paired migration after the current head; never edit applied migrations.
- Create `session_participant_presence`, relevant foreign-key/index/unique constraints, and additive `sessions` fields.
- Verify fresh schema, upgrade, rollback, rerun safety, 51st-join race, duplicate webhook, and end/close failure cases.
