# Data Model: Recitation Queue System

All objects are PostgreSQL-authoritative. The planned paired migration is the next sequence after `000016_live_sessions`; implementation must re-check numbering immediately before creation.

## Existing tables extended

### `sessions`

Add non-null columns with ADR-018 defaults and CHECK constraints:

| Field | Type | Allowed/default |
|---|---|---|
| `queue_population_policy` | varchar(32) | `present_at_activation` (default), `all_active_students` |
| `queue_finalization_policy` | varchar(32) | `mark_unfinished_skipped` (default), `preserve_last_state` |
| `queue_opt_out_policy` | varchar(24) | `approval_required` (default), `auto_approve` |
| `queue_grade_visibility` | varchar(32) | `managers_and_student` (default), `managers_only`, `all_participants` |
| `queue_grade_correction` | varchar(32) | `audited_any_time` (default), `before_round_finalization`, `immutable` |
| `queue_policy_version` | bigint | `1`, positive and incremented atomically per effective change |

Policy changes are permitted only while the session is `scheduled` or `active`, are prospective, and do not authorize the session creator independently of current circle role.

## New tables

### `quran_surahs`

F-003 creates and seeds the immutable 114-row Quran range reference before any
round table FK is added: `id` (1-114 primary key), Arabic name,
transliterated name, and positive `ayah_count`. Application roles receive read
access only; migrations own all changes. Backend validation checks the requested
range against `ayah_count` inside the round transaction.

### `recitation_queue`

| Field | Type | Constraints/meaning |
|---|---|---|
| `id` | uuid | primary key |
| `session_id` | uuid | FK `sessions(id)` ON DELETE RESTRICT, required |
| `round_number` | integer | positive; UNIQUE with `session_id` |
| `round_type` | varchar(30) | `new_memorization`, `revision`, `old_revision`, `test` |
| `surah_id` | integer | FK `quran_surahs(id)`, required |
| `from_ayah`, `to_ayah` | integer | positive and `from_ayah <= to_ayah`; service also validates against Surah count |
| `grading_required` | boolean | required |
| `lifecycle` | varchar(16) | `prepared`, `active`, `finalized` |
| `selected_entry_id` | uuid nullable | FK to an entry in this round, set by advance; never means reciting by itself |
| `version` | bigint | starts at 1; incremented on any queue-visible mutation |
| `created_by` | uuid | FK users, audit attribution |
| `created_at`, `activated_at`, `finalized_at` | timestamptz | UTC lifecycle timestamps |

Indexes/constraints: one `prepared` or `active` round per session using a partial unique index; selected entry must be validated inside the locked transaction as belonging to the round and remaining `waiting`.

### `recitation_queue_preorder`

| Field | Type | Constraints/meaning |
|---|---|---|
| `queue_id` | uuid | FK round ON DELETE CASCADE |
| `student_id` | uuid | FK users ON DELETE RESTRICT |
| `position` | integer | positive |
| `added_by` | uuid | FK users, manager attribution |
| `created_at` | timestamptz | UTC |

Primary/unique constraints: `(queue_id, student_id)` and `(queue_id, position)`. Only active student members may be included. This is preparation data, not a queue-entry state or practice record.

Activation preserves eligible pre-ordered students' relative order. For
`present_at_activation`, other present students follow by F-005
`first_joined_at` then user ID, and later joiners append. For
`all_active_students`, other active students follow by `circle_members.joined_at`
then user ID.

### `recitation_queue_entries`

| Field | Type | Constraints/meaning |
|---|---|---|
| `id` | uuid | primary key |
| `queue_id` | uuid | FK round ON DELETE RESTRICT |
| `student_id` | uuid | FK users ON DELETE RESTRICT |
| `position` | integer | positive; unique per round |
| `status` | varchar(20) | only `waiting`, `reciting`, `completed`, `skipped`, `opted_out` |
| `grade` | varchar(24) nullable | five canonical values only |
| `teacher_notes` | varchar(500) nullable | maximum 500 characters |
| `version` | bigint | starts at 1; optimistic mutation guard |
| `started_at`, `completed_at` | timestamptz nullable | UTC; completion timestamp only for completed entry |
| `resolved_by` | uuid nullable | FK users; manager responsible for terminal transition |
| `created_at`, `updated_at` | timestamptz | UTC |

Unique constraints: `(queue_id, student_id)` and `(queue_id, position)`. A partial unique index on `queue_id WHERE status='reciting'` enforces one reciter. A completed grading-required entry must have a grade; skipped/opted-out entries must have no grade/progress. Cross-table grading-required validation remains transactional service logic because PostgreSQL CHECK constraints cannot reference the round.

### `queue_opt_out_requests`

| Field | Type | Constraints/meaning |
|---|---|---|
| `id` | uuid | primary key |
| `queue_entry_id` | uuid | FK entry ON DELETE RESTRICT |
| `requested_by` | uuid | must equal entry student |
| `status` | varchar(16) | `pending`, `approved`, `declined` (request status, not entry status) |
| `decided_by` | uuid nullable | current teacher/supervisor for decided requests |
| `requested_at`, `decided_at` | timestamptz | UTC |
| `idempotency_key` | varchar(128) nullable | unique with requester when present |

A partial unique index permits at most one pending request per entry. Auto-approve changes the entry directly and may retain an already-decided request row for audit; it never introduces a queue-entry state.

### `queue_command_receipts`

| Field | Type | Constraints/meaning |
|---|---|---|
| `session_id`, `actor_id` | uuid | authorization/audit scope |
| `idempotency_key` | varchar(128) | client-supplied key |
| `command` | varchar(48) | closed backend command name |
| `resource_id` | uuid nullable | resulting round/entry/request |
| `result_version` | bigint nullable | committed round version |
| `created_at` | timestamptz | UTC |

Primary key: `(session_id, actor_id, idempotency_key)`. A reused key with another command is a conflict. Receipts contain no request body, notes, grade, media data, or response secret.

### `queue_event_outbox`

| Field | Type | Constraints/meaning |
|---|---|---|
| `event_id` | uuid | primary key and client deduplication key |
| `session_id`, `round_id` | uuid | required scope FKs |
| `event_type` | varchar(48) | closed `queue.*` event name |
| `resource_id` | uuid nullable | entry/request affected by the event |
| `round_version` | bigint | committed queue version |
| `event_metadata` | jsonb | server-built non-sensitive transition/order facts only |
| `available_at`, `delivered_at` | timestamptz | retry scheduling and completion |
| `attempt_count` | integer | non-negative, bounded operational retry count |

The outbox row is inserted with the business transaction. Metadata may retain
facts needed for an exact delta (for example old/new state or ordered IDs) but
stores no grade, note, student display name, media value, URL, or provider data.
Delivery reconstructs visibility-sensitive fields from PostgreSQL, so grade
visibility is enforced at send time. Retention cleanup is operational data
hygiene, not deletion of queue history.

### `memorization_progress`

F-003 creates the minimal canonical completed-turn source consumed later by F-007:

| Field | Type | Constraints/meaning |
|---|---|---|
| `id` | uuid | primary key |
| `student_id`, `circle_id`, `session_id` | uuid | required FKs |
| `queue_entry_id` | uuid | FK entry, UNIQUE |
| `surah_id` | integer | FK Quran Surah |
| `from_ayah`, `to_ayah` | integer | copied validated range |
| `type` | varchar(30) | four round types including `test` |
| `grade` | varchar(24) nullable | canonical values; nullable only for non-grading rounds |
| `notes` | varchar(500) nullable | current authorized note projection |
| `date` | date | session/turn UTC date |
| `created_at`, `updated_at` | timestamptz | UTC |

Only the completion transaction may insert. Correction updates this same row. F-007 may add views/indexes and compatibility fields later; it must exclude `type='test'` from Quran-map status derivation but retain it in practice history.

## Relationships

- Session 1 — many rounds; at most one current prepared/active round.
- Round 1 — many pre-order candidates and entries.
- Student 1 — at most one entry per round.
- Entry 1 — zero/many opt-out request attempts, at most one pending.
- Completed entry 1 — exactly one progress row; every non-completed entry — zero progress rows.

## State transitions

### Round

- `prepared -> active`: F-005 session becomes active; population materializes transactionally.
- `prepared|active -> finalized`: reset or ended-session convergence.
- Finalized is terminal. Reset creates a new row and next round number; it never reopens/reuses history.

### Entry

- `waiting -> reciting -> completed`
- `waiting|reciting -> skipped`
- `waiting|reciting -> opted_out` only through approved/auto-approved opt-out
- Completed, skipped, and opted-out are terminal.

Default finalization converts unfinished entries to skipped. `preserve_last_state` retains their last values but the finalized round makes them immutable/non-actionable and clears selection. Both paths revoke any reciter audio.

## Transaction boundaries

- Round create/activation/reset and population are single transactions.
- Reorder locks all affected pre-order candidates or waiting entries (selected by `order_kind`) and rewrites positions without exposing intermediate duplicates.
- Advance locks the round and waiting entries, sets the next selected entry, and increments round version.
- Start locks round/entry and relies on the partial unique reciter index as the final race guard.
- Complete locks round/entry, validates conditional grade, transitions entry, and inserts progress atomically.
- Correction locks entry/progress, validates policy/version, and updates both atomically.
- Policy patch locks session and increments its policy version only when values change.
- Events and structured audit logs occur after commit; failure triggers telemetry/retry and never rolls back queue truth.

## Visibility projection

Queue identity, position, and state are visible to authorized session participants. Grade and note fields are nullable or omitted unless allowed by `queue_grade_visibility`. Only current teachers/supervisors can mutate or view pending opt-out requests. Media room references, endpoints, credentials, provider identifiers, and URLs are absent from every F-003 entity and projection.
