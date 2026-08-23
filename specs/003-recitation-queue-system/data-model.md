# Data Model: Recitation Queue System

All objects are PostgreSQL-authoritative. The planned paired migration is `000017_recitation_queue_system` (next sequence after `000016_live_sessions`; implementation must re-check numbering immediately before creation). This model is regenerated 2026-08-23 against the reconciled `docs/engineering/architecture/ARCHITECTURE.md` (canonical) and the approved spec clarifications (A1/A2/B1/B2).

**FK policy (ADR-019)**: every F-003 business/history table uses plain FKs with default `NO ACTION` (restrict) behavior — no `ON DELETE CASCADE` anywhere in F-003 schema. Rounds, entries, and progress rows are never deleted; history is preserved.

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

Policy changes are permitted only while the session is `scheduled` or `active`, are prospective, and do not authorize the session creator independently of current circle role. F-005 lifecycle columns are untouched; session end never waits on queue cleanup.

## New tables

### `quran_surahs`

F-003 creates and seeds the immutable 114-row Quran range reference before any
round table FK is added: `id` (1-114 primary key), Arabic name,
transliterated name, and positive `ayah_count`. Application roles receive read
access only; migrations own all changes. Backend validation checks the requested
range against `ayah_count` inside the round transaction (invalid range → HTTP 422).

### `recitation_queue`

| Field | Type | Constraints/meaning |
|---|---|---|
| `id` | uuid | primary key |
| `session_id` | uuid | FK `sessions(id)` NOT NULL, NO ACTION |
| `round_number` | integer | positive; `UNIQUE (session_id, round_number)`; allocated as max+1 under the per-session round lock (no reuse, stable under retries and concurrent creation) |
| `round_type` | varchar(30) | `new_memorization`, `revision`, `old_revision`, `test` (CHECK) |
| `surah_id` | integer | FK `quran_surahs(id)`, required |
| `from_ayah`, `to_ayah` | integer | positive and `from_ayah <= to_ayah`; service validates against Surah count |
| `grading_required` | boolean | required |
| `lifecycle` | varchar(16) | `prepared`, `active`, `finalized` (CHECK) — separate from queue-entry state |
| `selected_entry_id` | uuid nullable | FK to a waiting entry in this round, set by advance; consumed by start; never means `reciting` by itself |
| `version` | bigint | starts at 1; incremented on any queue-visible mutation |
| `created_by` | uuid | FK users, audit attribution |
| `created_at`, `activated_at`, `finalized_at` | timestamptz | UTC lifecycle timestamps; `activated_at` NULL marks a round finalized without ever activating |

Indexes/constraints:

- **Partial unique `(session_id) WHERE lifecycle = 'active'`** — at most one active round. Several `prepared` rounds may stack per session (clarification B1). *(Pending one-line sync of ARCHITECTURE.md, which still reads `IN ('prepared','active')` — see plan §Canonical sync.)*
- `UNIQUE (session_id, round_number)` — sequential numbering without reuse (closes CHK036).
- Selected entry must be validated inside the locked transaction as belonging to the round and remaining `waiting`.

**Activation invariant (B1)**: while a session is live, if no round is active and prepared rounds exist, the lowest-numbered prepared round is active. Restored by F-005 session start, round creation on a live session, finalization-via-reset, and the restart/reconciliation pass. Session end suppresses the invariant permanently: the convergence worker finalizes the active round **and every never-activated prepared round** (retained, `activated_at` NULL, never activatable — FR-014 "permanently inert").

### `recitation_queue_preorder`

| Field | Type | Constraints/meaning |
|---|---|---|
| `queue_id` | uuid | FK round NOT NULL, NO ACTION |
| `student_id` | uuid | FK users NOT NULL, NO ACTION |
| `position` | integer | positive |
| `added_by` | uuid | FK users, manager attribution |
| `created_at` | timestamptz | UTC |

Primary/unique constraints: `(queue_id, student_id)` and `(queue_id, position)`. Only active student members may be included. This is preparation data, not a queue-entry state or practice record. Pre-activation editing is the manager surface (full-list reorder via `PUT /order` while the round is prepared); no student-facing pre-activation projection exists in MVP (CHK008 — students receive an empty `preorder` array in `QueueState`).

Activation materializes entries: pre-set students **who are present-eligible under the policy** keep their relative order first. For `present_at_activation`, other present students follow by F-005 `first_joined_at` then user ID, and later joiners append. For `all_active_students`, other active student members follow by `circle_members.joined_at` then user ID. The UUID tie-break makes concurrent timestamps deterministic (closes B2's deferred tie-break).

### `recitation_queue_entries`

| Field | Type | Constraints/meaning |
|---|---|---|
| `id` | uuid | primary key |
| `queue_id` | uuid | FK round NOT NULL, NO ACTION |
| `student_id` | uuid | FK users NOT NULL, NO ACTION |
| `position` | integer | positive; unique per round |
| `status` | varchar(20) | CHECK: only `waiting`, `reciting`, `completed`, `skipped`, `opted_out` |
| `grade` | varchar(30) nullable | CHECK: five canonical ADR-013 values only (`excellent`, `good`, `acceptable`, `needs_review`, `repeat`) |
| `teacher_notes` | varchar(500) nullable | maximum 500 characters |
| `version` | bigint | starts at 1; optimistic mutation guard |
| `started_at`, `completed_at` | timestamptz nullable | UTC; completion timestamp only for completed entry |
| `resolved_by` | uuid nullable | FK users; manager responsible for terminal transition |
| `created_at`, `updated_at` | timestamptz | UTC |

Unique constraints: `(queue_id, student_id)` and `(queue_id, position)`. A partial unique index on `queue_id WHERE status='reciting'` enforces one reciter. A completed grading-required entry must have a grade; skipped/opted-out entries must have no grade/progress. Cross-table grading-required validation remains transactional service logic because PostgreSQL CHECK constraints cannot reference the round.

**Move semantics (A2a)**: moving one `waiting` entry to an arbitrary slot rewrites the affected positions inside one locked transaction (the `(queue_id, position)` unique constraint holds throughout); the `reciting` entry and terminal entries are immovable. Full-list reorder applies only to preorder candidates while the round is `prepared`.

### `queue_opt_out_requests`

| Field | Type | Constraints/meaning |
|---|---|---|
| `id` | uuid | primary key |
| `queue_entry_id` | uuid | FK entry NOT NULL, NO ACTION |
| `requested_by` | uuid | must equal entry student |
| `status` | varchar(16) | `pending`, `approved`, `declined` (request status, not entry status) |
| `decided_by` | uuid nullable | current teacher/supervisor for decided requests |
| `requested_at`, `decided_at` | timestamptz | UTC |
| `idempotency_key` | varchar(128) nullable | unique with requester when present |

A partial unique index permits at most one pending request per entry. Auto-approve changes the entry directly and may retain an already-decided request row for audit; it never introduces a queue-entry state.

**Decline path (CHK005)**: decline terminally closes the request (`declined`, `decided_by`/`decided_at` set, durably logged) and leaves the entry `waiting` — no other outcome is permitted by the transition table. Any pending request becomes non-actionable when its round finalizes; deciding it afterwards is a clean conflict with no state mutation.

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
| `attempt_count` | integer | non-negative |
| `parked_at` | timestamptz nullable | set when the 5 exponential-backoff retries are exhausted |

The outbox row is inserted with the business transaction. Delivery attempts: initial attempt plus 5 retries with exponential backoff (+1s, +2s, +4s, +8s, +16s, with jitter); on exhaustion the row is parked (`parked_at` set, alert fires) for operator replay — never silently dropped. Metadata may retain facts needed for an exact delta (for example old/new state or ordered IDs) but stores no grade, note, student display name, media value, URL, or provider data. Delivery reconstructs visibility-sensitive fields from PostgreSQL, so grade visibility is enforced at send time. Retention cleanup is operational data hygiene, not deletion of queue history.

### `memorization_progress`

F-003 creates the canonical completed-turn source exactly as defined by the reconciled `ARCHITECTURE.md` (canonical — do not diverge):

| Field | Type | Constraints/meaning |
|---|---|---|
| `id` | uuid | primary key |
| `student_id` | uuid | FK users NOT NULL, **NO ACTION — no cascade** (ADR-019; history survives unless explicitly erased) |
| `circle_id` | uuid | FK circles NOT NULL, NO ACTION |
| `session_id` | uuid | FK sessions NOT NULL — always known; only the completion transaction inserts |
| `queue_entry_id` | uuid | FK entry **NOT NULL UNIQUE** — idempotent completion and re-grade upsert target |
| `surah_id` | integer | FK Quran Surah NOT NULL — normalized surah reference |
| `surah_name` | varchar(100) | **DEPRECATED** — retained until v1.1 for in-flight client compat (F-007-SPEC OQ-032); new writers populate it for compat but all reads use `surah_id` |
| `from_ayah`, `to_ayah` | integer | NOT NULL — copied validated range |
| `type` | varchar(30) | CHECK: four round types including `test`; completed `test` records remain practice history but F-007 excludes them from Quran-map derivation |
| `grade` | varchar(30) nullable | CHECK: five canonical values (ADR-013); NULL only when the round had `grading_required = false` |
| `notes` | varchar(500) | same bound as `recitation_queue_entries.teacher_notes` |
| `date` | date | NOT NULL — session date |
| `created_at`, `updated_at` | timestamptz | UTC; `updated_at` set on re-grade |

Only the completion transaction may insert. Correction updates this same row (repeated identical corrections converge to one record state — CHK035; clearing the optional note sets it NULL). F-007 may add views/indexes and compatibility fields later; it must exclude `type='test'` from Quran-map status derivation but retain it in practice history.

## Relationships

- Session 1 — many rounds; at most one `active` round; any number of stacked `prepared` rounds.
- Round 1 — many pre-order candidates and entries.
- Student 1 — at most one entry per round.
- Entry 1 — zero/many opt-out request attempts, at most one pending.
- Completed entry 1 — exactly one progress row; every non-completed entry — zero progress rows.

## State transitions

### Round lifecycle (separate from entry states)

- `prepared -> active`: automatic activation invariant (B1) — lowest-numbered prepared round when the session is live and no round is active.
- `prepared -> finalized`: session-end convergence only (never-activated round becomes permanently inert; `activated_at` stays NULL).
- `active -> finalized`: reset (finalization policy applied) or ended-session convergence. Reset also creates the next sequential round; the activation invariant then activates the next prepared round in round-number order (which may be a previously prepared round or the reset-created one).
- `finalized` is terminal. Reset creates a new row and the next round number; it never reopens or reuses history.

### Entry states (exactly five)

- `waiting -> reciting -> completed` (start applies only to the selected entry; grading-required completion carries the grade atomically; grading-optional completion carries none)
- `waiting | reciting -> skipped`
- `waiting | reciting -> opted_out` only through approved/auto-approved opt-out
- `completed`, `skipped`, `opted_out` are terminal for that round.

Default finalization converts unfinished entries to `skipped`. `preserve_last_state` retains their last values, but the finalized round makes them immutable/non-actionable and clears selection. Both paths revoke any reciter audio.

## Transaction boundaries

- Round create/activation/reset and population are single transactions under the per-session round lock (round numbers allocated max+1 — CHK036).
- Pre-activation full-list reorder locks all pre-order candidates; post-activation move locks the round and rewrites only the affected positions.
- Advance locks the round and waiting entries, replaces any existing selection, and increments round version.
- Start locks round/entry, requires the entry be the selected waiting entry, and relies on the partial unique reciter index as the final race guard.
- Complete locks round/entry, validates conditional grade, transitions the entry, and inserts progress atomically.
- Correction locks entry/progress, validates policy/version, and updates both atomically.
- Policy patch locks the session and increments its policy version only when values change.
- Events and structured audit records are written in-transaction (outbox/audit rows); realtime dispatch and audio-control calls occur after commit; their failure triggers bounded retry/parking and never rolls back queue truth.

## Visibility projection

Queue identity, position, and state are visible to authorized session participants. Grade and note fields are nullable or omitted unless allowed by `queue_grade_visibility`. The prepared-order (`preorder`) projection is manager-surface only; non-managers receive an empty array (CHK008). Only current teachers/supervisors can mutate or view pending opt-out requests. Media room references, endpoints, credentials, provider identifiers, and URLs are absent from every F-003 entity and projection (SC-005).
