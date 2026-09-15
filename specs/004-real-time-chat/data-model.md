# Data Model: Real-time Chat

## `messages`

| Field | Type | Constraints / purpose |
|---|---|---|
| `id` | UUID | Primary key; server-generated stable message/event identity |
| `circle_id` | UUID | Nullable FK to `circles.id`; group context |
| `dm_recipient_id` | UUID | Nullable FK to `users.id`; directional DM recipient |
| `sender_id` | UUID | FK to `users.id`, not null |
| `idempotency_key` | VARCHAR(128) | Not null; unique with `sender_id` |
| `message_type` | VARCHAR(20) | `text`, `voice`, `image`, or `file` |
| `content` | TEXT | Text only, trimmed, 1–4000 characters; null for media |
| `upload_id` | UUID | Nullable unique FK to `chat_uploads.id`; required for media |
| `reply_to_id` | UUID | Nullable self-FK; same authorized conversation only |
| `is_pinned` | BOOLEAN | Not null, default false; group only |
| `pinned_by` | UUID | Nullable FK to `users.id`; teacher/supervisor when pinned |
| `pinned_at` | TIMESTAMPTZ | Nullable; present with `pinned_by` |
| `sent_at` | TIMESTAMPTZ | Not null, database default `NOW()`; authoritative acceptance time |
| `deleted_at` | TIMESTAMPTZ | Nullable soft-delete time |

Constraints enforce exactly one of `circle_id`/`dm_recipient_id`, exactly one compatible text/upload payload, no self-DM, group-only pin state, and unique `(sender_id,idempotency_key)`. Business/history FKs use default `NO ACTION`. List/search order is `sent_at DESC, id DESC`; cursor anchors use the referenced message's pair.

Group access joins current `circle_members` and applies `sent_at >= joined_at`. DM identity is the unordered `(sender_id,dm_recipient_id)` pair; authorization is recalculated from current active shared circles on every operation.

## `chat_uploads`

| Field | Type | Constraints / purpose |
|---|---|---|
| `id` | UUID | Primary key; public upload reference |
| `uploader_id` | UUID | FK to `users.id`, not null |
| `authorization_circle_id` | UUID | FK to `circles.id`, not null; group owner or DM authorization witness |
| `dm_peer_id` | UUID | Nullable FK to `users.id`; null means group upload |
| `object_key` | TEXT | Unique private MinIO key; never returned in message/event projections |
| `mime_type` | VARCHAR(100) | Server-detected allowed MIME |
| `original_file_name` | VARCHAR(255) | Sanitized display name; never used as object path |
| `size_bytes` | BIGINT | Positive; type-specific limit |
| `duration_seconds` | INTEGER | Voice only, 1–300 |
| `state` | VARCHAR(16) | `staged`, `attached`, or `revoked` |
| `created_at`, `updated_at` | TIMESTAMPTZ | Not null |

For group media, `dm_peer_id` is null and the object may attach only to that circle. For DM media, any qualifying shared active circle may be the authorization witness, but the binding is to the uploader/peer pair. Loss of that one circle does not remove access while another qualifying circle remains. Legacy unbound upload responses cannot be attached to F-004 messages.

## `message_reads`

| Field | Type | Constraints / purpose |
|---|---|---|
| `message_id` | UUID | FK to `messages.id`, part of primary key |
| `user_id` | UUID | FK to `users.id`, part of primary key; cannot equal sender |
| `read_at` | TIMESTAMPTZ | Not null, database default `NOW()` |

`(message_id,user_id)` is unique and inserts use conflict-safe idempotency. Archived-circle reads are rejected as mutations. A group headline becomes read after one currently eligible non-sender fact; only the sender's authorized message projection includes currently eligible `read_receipts` (`reader_id`, `read_at`).

## `chat_event_outbox`

| Field | Type | Constraints / purpose |
|---|---|---|
| `event_id` | UUID | Primary key; used for event deduplication |
| `message_id` | UUID | FK to `messages.id`, not null |
| `event_type` | VARCHAR(48) | `chat.message`, `chat.message_read`, or `chat.message_deleted` |
| `recipient_id` | UUID | Nullable FK to `users.id`; targeted read event when present |
| `available_at` | TIMESTAMPTZ | Next attempt time |
| `delivered_at` | TIMESTAMPTZ | Nullable successful projection time |
| `attempt_count` | INTEGER | Non-negative, default 0 |
| `parked_at` | TIMESTAMPTZ | Nullable after retry exhaustion |

The row contains identifiers only. The projector reloads current authorized state and validates each connected user/session immediately before write, so revoked sessions, removed members, newly ineligible DM peers, and deleted content are not leaked. Group events use the circle topic; DM events target eligible authenticated user connections directly without selecting a qualifying circle. Claims use `FOR UPDATE SKIP LOCKED`; five failed attempts park the row without changing durable chat truth.

## `message_moderation_audits`

| Field | Type | Constraints / purpose |
|---|---|---|
| `id` | UUID | Primary key |
| `message_id` | UUID | FK to `messages.id`, not null |
| `circle_id` | UUID | FK to `circles.id`, not null |
| `actor_id` | UUID | FK to `users.id`, not null; current teacher |
| `action` | VARCHAR(32) | Fixed value `teacher_delete` |
| `occurred_at` | TIMESTAMPTZ | Not null, database default `NOW()` |

The table is append-only at the application layer and stores no body, filename, object key, URL, or credential. Sender self-deletion does not create a moderation audit row.

## Search support

Text messages maintain a normalized `tsvector` generated from content using an immutable Halaqaty SQL normalization function and PostgreSQL's `simple` configuration. A partial GIN index covers non-deleted text messages. Queries apply circle, membership-period, and deletion filters before returning results; DMs are excluded.

## State transitions

- Message: `pending` (mobile only) → `sent` (request transmitted) → `delivered` (durably accepted); `read` is a projection of recipient facts. Durable message state is `active → deleted` only.
- Upload: `staged → attached → revoked`; attachment is single-use. Failed/abandoned staged uploads remain inaccessible and are eligible for normal cleanup.
- Outbox: `pending → delivered` or `pending → retrying → parked`; replay may move a parked row back through delivery without duplicating the message.
- DM authorization: `eligible ↔ ineligible`; history is retained and visible only while eligible.
- Pin mutations: lock the owning `circles` row, recount active pins, then pin/unpin/delete in the same transaction; this serializes the five-pin invariant.
- Media deletion recovery: marker first, then database soft delete. Active message + latest delete marker means reconciliation removes only that internal marker; deleted message + no marker means reconciliation creates one. Internal MinIO version IDs are never persisted or projected.

## Migration and rollback

Create paired `backend/migrations/000018_real_time_chat.up.sql` and `.down.sql`. The up migration creates only F-004 objects/extensions/indexes and does not rewrite prior messages because no implemented chat table exists. The down migration removes only F-004 indexes, tables, and the Halaqaty search-normalization function in dependency-safe order. Validate fresh, upgrade, rollback, and reapply paths.
