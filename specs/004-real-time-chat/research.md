# Research: Real-time Chat

## Decision 1: Keep chat inside the modular monolith

- **Decision**: Add `backend/internal/chat` and `mobile/lib/features/chat`; compose them through existing route, auth/session, circle-membership, logging, metrics, and realtime seams.
- **Rationale**: F-004 is one bounded domain at the MVP scale. A separate service, broker, or cache would weaken PostgreSQL authority and add operations without improving the 50-user target.
- **Alternatives considered**: Chat microservice, Redis pub/sub, LiveKit data messages.

## Decision 2: Use PostgreSQL chat-owned persistence

- **Decision**: One paired `000018_real_time_chat` migration creates `messages`, `message_reads`, `chat_uploads`, `chat_event_outbox`, and `message_moderation_audits`, plus search/index support. ADR-021 records the schema, MinIO client, audit, and delivery decisions.
- **Rationale**: Messages and reads are authoritative; upload binding prevents object-key reuse; an outbox closes the commit/broadcast crash window; durable deletion audit is an explicit requirement.
- **Alternatives considered**: Reuse the F-003 queue outbox; structured logs as the only deletion audit; a generic event bus.

## Decision 3: Derive access from current membership periods

- **Decision**: Group list/search/reply/media queries require current `circle_members` and filter `messages.sent_at >= circle_members.joined_at`. Removal deletes the membership row; rejoining creates a new lower bound. Archived circles allow only retained-member reads.
- **Rationale**: This directly implements clarification Q1 without a new membership-history table.
- **Alternatives considered**: Full historical access; persisting membership epochs in F-004; invitation-time claims.

## Decision 4: Model a DM as an unordered user pair

- **Decision**: Keep directional `sender_id` plus `dm_recipient_id`; query the unordered pair in both directions and enforce exactly one message context. Current teacher-student or supervisor-student roles in any shared active circle authorize both directions. Eligibility restoration reveals the existing pair history.
- **Rationale**: This implements Q2/Q3 and avoids a conversation table that adds no MVP state.
- **Alternatives considered**: `direct_conversations` table; one DM per shared circle; permanent access after first contact.

## Decision 5: Make sends idempotent and realtime delivery retryable

- **Decision**: Require `Idempotency-Key` on group/DM sends, unique by sender. Insert each message and redacted `chat_event_outbox` row atomically. Reuse the proven F-003 dispatcher policy: five attempts, exponential delay with bounded jitter, parked-row metrics, and startup replay. Before every socket write, resolve the client user/session against current PostgreSQL authorization. Group events remain circle-topic projections; DM events target the eligible pair's authenticated connections directly, independent of which qualifying circle topics they joined.
- **Rationale**: A REST retry returns the same message while WebSocket delivery remains at-least-once and recoverable.
- **Alternatives considered**: Best-effort broadcast after commit; exactly-once WebSocket claims; reusing queue-owned tables.

## Decision 6: Store object keys and revoke through MinIO versioning

- **Decision**: Persist private object keys, never presigned URLs. Enable versioning on the chat bucket; attachment deletion is fail-closed and succeeds only after MinIO places a delete marker on the versionless key, while older bytes remain until a future parent-retention feature. A failed marker rejects the soft-delete transaction. If a crash leaves the database message active after marker creation, reconciliation lists versions internally and removes only the latest matching delete marker to restore versionless access; if the database row is deleted, it ensures a marker exists. Version IDs never cross the server boundary. Add the official `minio-go/v7` client behind a narrow media store.
- **Rationale**: MinIO documents that a delete marker hides versionless reads without removing older versions, reconciling immediate access revocation with Q5's retained bytes. The official Go SDK supports object operations and presigned GETs.
- **Alternatives considered**: Proxy media through Go; synchronous permanent purge; leave issued links valid until expiry.
- **Sources**: [MinIO object versioning](https://docs.min.io/aistor/administration/objects-and-versioning/versioning/), [MinIO Go SDK](https://github.com/minio/minio-go).

The Compose service is built from MinIO's official `RELEASE.2025-10-15T17-29-55Z` source tag in `docker/minio.Dockerfile`. That tag contains the final community security fix; relying on the older prebuilt September image or an untrusted third-party image was rejected.

## Decision 7: Use PostgreSQL full-text search with Arabic-safe normalization

- **Decision**: Store a generated/search-maintained `tsvector` from normalized plain-text content, use the `simple` configuration and a GIN index, and parse user text with `websearch_to_tsquery`. Normalization removes Arabic combining marks and tatweel and folds common Alef/Ya variants before indexing and querying; results order by `(sent_at, id)` descending.
- **Rationale**: PostgreSQL documents stored `tsvector` columns and GIN as the preferred text-search index. The `simple` dictionary avoids English stemming assumptions; Halaqaty-owned normalization makes Arabic matching deterministic.
- **Alternatives considered**: `ILIKE` scans; external search service; locale-dependent database defaults.
- **Sources**: [PostgreSQL text-search tables](https://www.postgresql.org/docs/current/textsearch-tables.html), [preferred text-search indexes](https://www.postgresql.org/docs/current/textsearch-indexes.html).

## Decision 8: Evolve v1 contracts additively

- **Decision**: Retain existing chat/upload paths and fields. Add a pinned-list route, sender-only `read_receipts`, idempotency header, authorization-context upload fields, cursors, reply/pin/media metadata, and error responses. Canonical server `delivery_status` is only `delivered` or `read`; pending/sent remain local Flutter states. Archived mark-read is denied and unsupported detected media returns `415`. Legacy uploads without chat context remain valid for existing callers but cannot attach to an F-004 message.
- **Rationale**: Additive request/response fields preserve old clients while new server-side attachment validation closes the security gap.
- **Alternatives considered**: Replace `/uploads/*`; introduce `/api/v2`; accept unbound legacy keys in chat.

## Decision 9: Persist the mobile pending queue with an existing dependency

- **Decision**: Store each pending envelope and its stable idempotency key through the already-installed `flutter_secure_storage`; store local attachment paths, never attachment bytes. Reload on app start, retry only on reconnect or explicit user retry, and keep terminal failures visible for edit/discard.
- **Rationale**: This covers process restart without adding SQLite/Hive. PostgreSQL remains authoritative after server acceptance.
- **Alternatives considered**: Memory-only Riverpod state; new SQLite dependency; infinite automatic retry.

## Decision 10: Bound transient behavior and failures

- **Decision**: Use 10-second connect, 15-second normal receive, and 60-second upload timeouts. Automatic send retries use 1s/2s/4s for network, timeout, `429`, and `5xx`; `Retry-After` is honored up to 30 seconds, and contract validation/auth/conflict failures are terminal. Typing indicators expire client-side after five seconds. History/search use `(sent_at,id)` keyset order with existing message-ID anchors. The reproducible performance fixture uses 50 concurrent users across 10 circles, 10,000 messages per circle, 100 warm-ups, and 1,000 measured samples.
- **Rationale**: This matches existing mobile timeout patterns and F-005 bounded reconnect behavior while avoiding indefinite spinners.
- **Alternatives considered**: Unbounded retries; offset pagination; persisted typing state.

## Decision 11: Keep F-008 completely separate

- **Decision**: F-004 emits no background-notification trigger and imports no Firebase Messaging API. It exposes durable messages and foreground WebSocket events only.
- **Rationale**: Clarification Q4 assigns trigger creation, preferences, FCM transport, and background delivery to F-008 as one cohesive feature.
- **Alternatives considered**: F-004 notification outbox; presence-aware partial push integration.

## Decision 12: Observe user-visible reliability without leaking content

- **Decision**: Add counters/histograms for send latency/outcome, authorization denial, upload rejection, outbox backlog/parking, reconnect recovery, and search latency. Logs include request/event/message IDs and actor IDs, but never message bodies, filenames, object keys, signed URLs, tokens, or session IDs.
- **Rationale**: Operators can distinguish database, storage, and realtime failures while protecting Quran-learning and minor-related content.
- **Alternatives considered**: Payload logging; distributed tracing dependency; no chat-specific metrics.

## Decision 13: Use the minimum Flutter media plugin set

- **Decision**: Add `record ^7.1.1`, `just_audio ^0.10.6`, `image_picker ^1.2.3`, and `file_picker ^12.2.0`. Draw the recording waveform directly from `record` amplitude samples; keep playback foreground-only and use native pickers. Do not add `just_waveform`, background-audio, codec-conversion, or media-service packages.
- **Rationale**: Each dependency owns one required platform boundary, while amplitude rendering and UI state remain ordinary Flutter code. This satisfies the approved voice/image/PDF UX without a speculative media stack.
- **Alternatives considered**: One broad media toolkit; custom platform channels; extracted waveform/background playback packages.
- **Sources**: [record](https://pub.dev/packages/record), [just_audio](https://pub.dev/packages/just_audio), [image_picker](https://pub.dev/packages/image_picker), [file_picker](https://pub.dev/packages/file_picker).
