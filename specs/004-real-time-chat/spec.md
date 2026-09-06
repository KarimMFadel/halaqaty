# Feature Specification: Real-time Chat

**Feature Branch**: `004-real-time-chat`
**Created**: 2026-09-03
**Status**: Approved
**Input**: Circle-scoped group chat and restricted direct messaging with text, voice notes, images, PDFs, delivery states, moderation, search, replies, realtime delivery, and offline retry.

## Clarifications

### Session 2026-09-03

- Q: Which circle messages can a new or rejoining member access? → A: Only messages accepted on or after the start of their current active membership period.
- Q: What happens to an existing DM history when the pair later regains a qualifying active-circle relationship? → A: The existing complete DM history becomes accessible again.
- Q: How is DM media authorized when the pair shares multiple qualifying circles? → A: Any one qualifying circle authorizes the upload, and access continues while any qualifying circle remains; the media is owned by the pair conversation rather than the selected circle.
- Q: Does F-004 create background-notification triggers for F-008? → A: No. Trigger creation and all background-notification behavior are deferred to F-008.
- Q: What happens to an attachment object after its message is soft-deleted? → A: Access is revoked immediately, but F-004 adds no per-message physical purge; prior object versions may remain indefinitely until a future approved circle/account retention feature owns parent cleanup.
- Image attachments use the approved F-004 product limit of 5 MB; PDF attachments use 10 MB. The current 20 MB upload-contract values require correction during contract planning.
- A direct-message pair must share at least one active circle in which their current roles form a teacher-student or supervisor-student pair. Student-student, teacher-teacher, supervisor-supervisor, and teacher-supervisor direct messages remain prohibited.
- Voice-note encoding and container are not product requirements. Planning may select supported mobile formats without weakening the fixed 300-second and 20 MB limits or cross-platform playback requirement.
- A locally queued message is pending; a transmitted message is sent; durable server acceptance is delivered; a recorded recipient read fact is read. Group-chat senders may see per-member read facts, while the headline read state becomes active after at least one eligible non-sender reads the message.
- Members retained in an archived circle may read, search, and play chat history accepted during their retained membership period. Sending, uploading, typing, marking read, pinning, unpinning, and deleting are prohibited after archival.
- Upload operations retain their additive v1 paths and must include server-validated target-circle context. For direct-message media, any qualifying active shared circle may prove authorization, while the object belongs to the pair conversation and remains accessible while any qualifying circle remains.
- ADR-010 was amended at the start of F-004 without removing existing invite-code/link generation or sharing: active teachers and supervisors may invite a teacher or student, while active students may invite a student only. The role is bound to the invitation and cannot be changed during acceptance.

### Session 2026-09-06

- Q: Does the MVP recording prohibition block F-004 voice notes, and which Flutter dependencies are approved? → A: No; it blocks live-session capture/storage only. Use `record`, `just_audio`, `image_picker`, and `file_picker`, with waveform drawn from recorder amplitude samples.
- Q: How do REST projections expose read details and pinned messages? → A: Authorized message projections include sender-visible read receipts, and a dedicated group pinned-list endpoint returns the active pinned bar.
- Q: How are authorization changes enforced for realtime group and multi-circle DM delivery? → A: Reauthorize each client/session immediately before write; deliver DMs directly to the eligible pair's authenticated connections without choosing a shared-circle topic.
- Q: How are the five-pin limit and MinIO marker-before-commit crash serialized/recovered? → A: Serialize pin/unpin/delete on the circle row; if a marker exists while the message remains active, reconciliation removes only the latest internal delete marker and restores versionless access.
- Q: What is the reproducible MVP load and operational failure policy? → A: Use the fixed 50-user/10-circle dataset and bounded timeout/retry/degradation rules defined in requirements and planning; all F-004 P1/P2 stories are MVP scope.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Participate in a Circle Group Chat (Priority: P1)

As an active circle member, I can read the circle's message history and exchange text messages with all other active members so our learning communication stays inside Halaqaty.

**Why this priority**: A dependable group conversation is the core replacement for external messaging groups and provides value without any live session.

**Independent Test**: Open an active circle with no live session, load its paginated history, send a text message, and verify that it appears once for the sender and other authorized members.

**Acceptance Scenarios**:

1. **Given** an authenticated active member of an active circle, **When** the member opens chat, **Then** the member sees authorized, non-deleted history accepted on or after the start of their current active membership period in deterministic newest-page order.
2. **Given** valid text of no more than 4000 characters, **When** an active member sends it, **Then** it is stored once and becomes available to every active member of that circle.
3. **Given** an online authorized member, **When** another member's message is durably accepted, **Then** the online member receives one effective `chat.message` update even if transport redelivers the event.
4. **Given** no active live session, **When** a member uses circle chat, **Then** all group-chat behavior remains available.

---

### User Story 2 - Send Reliably Across Connection Changes (Priority: P1)

As a member with an intermittent connection, I can queue a message and have it sent safely after reconnecting so a network interruption neither loses nor duplicates my message.

**Why this priority**: Mobile connectivity is unreliable, and duplicate or lost messages would make the chat unsuitable for circle coordination.

**Independent Test**: Send while offline, reconnect, repeat the same request and realtime delivery, and verify one durable message and one visible client item.

**Acceptance Scenarios**:

1. **Given** the device is offline, **When** a member sends a valid message, **Then** the client queues it locally with a pending state and a stable client-generated idempotency key.
2. **Given** a queued message, **When** connectivity returns, **Then** the client retries it with the same key until the server confirms durable acceptance.
3. **Given** repeated requests with the same authorized sender and idempotency key, **When** the server processes them, **Then** all successful responses identify the same durable message.
4. **Given** a reconnect or an unknown, duplicate, or missed realtime event, **When** the client reconciles, **Then** it re-fetches authoritative history and deduplicates by `message_id`.

---

### User Story 3 - Share Practice Media (Priority: P1)

As a circle member, I can share a voice note, image, or PDF so recitation practice and learning material remain attached to the relevant conversation.

**Why this priority**: Voice-note practice is a primary teaching workflow, while images and PDFs support Mushaf pages and exercises.

**Independent Test**: Record, preview, send, and play a compliant voice note; separately upload and send a compliant image and PDF; verify rejection at every limit and authorization boundary.

**Acceptance Scenarios**:

1. **Given** microphone permission, **When** a member records a voice note, **Then** recording duration and waveform are visible and the member can preview, discard, or send it.
2. **Given** a voice note no longer than 300 seconds and no larger than 20 MB, **When** an authorized member sends it, **Then** recipients can play it through a renewable access-controlled link that expires after 7 days.
3. **Given** a JPEG or PNG image no larger than 5 MB, **When** an authorized member uploads and sends it, **Then** it appears as an image message in the intended conversation.
4. **Given** a PDF no larger than 10 MB, **When** an authorized member uploads and sends it, **Then** recipients can view or download it using an access-controlled link.
5. **Given** an unsupported MIME type, falsified extension, over-limit size, over-limit voice duration, or unauthorized target circle, **When** an upload is attempted, **Then** it is rejected before the object can be attached to a message.

---

### User Story 4 - Use an Authorized Direct Conversation (Priority: P2)

As a teacher, supervisor, or student, I can exchange one-on-one messages only with an eligible counterpart from a shared active circle so private learning communication respects circle-scoped roles.

**Why this priority**: Private feedback is valuable but must not create unrestricted global messaging or bypass circle membership.

**Independent Test**: Exercise both directions of each allowed role pair, then test disallowed pairs, cross-circle users, removed members, archived-only relationships, and role changes.

**Acceptance Scenarios**:

1. **Given** two users share an active circle as teacher and student, **When** either starts or continues their direct conversation, **Then** the operation succeeds.
2. **Given** two users share an active circle as supervisor and student, **When** either starts or continues their direct conversation, **Then** the operation succeeds.
3. **Given** student-student, teacher-teacher, supervisor-supervisor, or teacher-supervisor roles, **When** either user attempts direct messaging, **Then** the operation is rejected.
4. **Given** an otherwise valid role pair with no qualifying shared active circle, **When** either user attempts to list, read, send, upload for, or mark read in the direct conversation, **Then** access is rejected.
5. **Given** a role or membership change removes the last qualifying active-circle relationship, **When** either user next accesses the direct conversation, **Then** authorization is re-evaluated and access is denied.
6. **Given** the same pair later regains a qualifying active-circle relationship, **When** either user opens their direct conversation, **Then** the existing complete conversation history is accessible again.

---

### User Story 5 - Understand Delivery, Reading, and Typing (Priority: P2)

As a message sender, I can distinguish pending, sent, delivered, and read states and see when an eligible member is typing so I understand the current conversation state without treating transient signals as durable truth.

**Why this priority**: Clear status reduces duplicate sending and confusion, while keeping durable message history authoritative.

**Independent Test**: Drive a message from offline queue through durable acceptance and recipient read, and verify that typing expires without creating history.

**Acceptance Scenarios**:

1. **Given** a locally queued message, **When** it has not been transmitted, **Then** it displays pending rather than sent, delivered, or read.
2. **Given** a send attempt has left the local queue but durable acceptance is not confirmed, **When** status is displayed, **Then** it shows sent with one check.
3. **Given** the server confirms durable persistence and authorized availability, **When** status is displayed, **Then** it shows delivered with two checks.
4. **Given** a DM recipient reads a message, **When** the read fact is recorded, **Then** the sender receives a targeted `chat.message_read` update and sees the read state.
5. **Given** a group member other than the sender reads a message, **When** the read fact is recorded, **Then** the sender receives that member's read update, the headline state becomes read, and duplicate reads do not create duplicate facts.
6. **Given** an authorized member starts or stops typing, **When** the transient command is accepted, **Then** other authorized participants receive `chat.typing`; the signal expires automatically and is never stored as message history.

---

### User Story 6 - Reply, Search, and Find Important Messages (Priority: P2)

As a circle member, I can reply with context, search retained circle history, and view pinned messages so important teaching material remains easy to find.

**Why this priority**: Structured retrieval is a major advantage over unorganized external group chat.

**Independent Test**: Reply to an authorized message, search for it, pin up to the limit, and verify behavior after deletion and archival.

**Acceptance Scenarios**:

1. **Given** an accessible message in the same conversation, **When** a member replies, **Then** the new message includes a safe quoted preview and a reference to the original.
2. **Given** an active or archived circle, **When** a retained member searches its chat, **Then** results contain only matching, non-deleted messages accepted during their current retained membership period in that circle.
3. **Given** fewer than five pinned messages, **When** a teacher or supervisor pins an eligible group message, **Then** it appears in the pinned bar above that circle's chat.
4. **Given** five pinned messages, **When** a teacher or supervisor attempts to pin a sixth, **Then** the operation is rejected; after one is unpinned, another can be pinned.
5. **Given** a student or non-member, **When** they attempt to pin or unpin, **Then** the operation is rejected.

---

### User Story 7 - Moderate Messages Safely (Priority: P2)

As a sender or teacher, I can remove inappropriate or mistaken content within defined authority while preserving auditability and preventing deleted content from leaking through other views.

**Why this priority**: Circle chat may involve minors and Quran learning; moderation must be predictable, limited, and reviewable.

**Independent Test**: Delete as sender before and after the deadline, delete as teacher, and inspect history, search, pins, replies, media access, realtime updates, and audit records.

**Acceptance Scenarios**:

1. **Given** fewer than 10 minutes have elapsed since sending, **When** the sender deletes their own message, **Then** it is soft-deleted successfully.
2. **Given** more than 10 minutes have elapsed, **When** a non-teacher sender deletes their own message, **Then** the request returns the contract-defined conflict and content remains unchanged.
3. **Given** a teacher in the message's circle, **When** the teacher deletes any group message, **Then** it is soft-deleted and the action is audit-logged with actor, target, circle, and time.
4. **Given** a soft-deleted message, **When** any authorized user loads history, search, pins, or media, **Then** the message and its content are excluded, attachment access is immediately revoked, and any reply preview no longer reveals the deleted content.

---

### User Story 8 - Preserve Access Boundaries Through Lifecycle Changes (Priority: P1)

As a circle member or owner, I need chat access to follow session, membership, and circle lifecycle rules so retained history remains safe without coupling chat to live sessions.

**Why this priority**: Authorization changes must take effect immediately, and archived history must remain readable without permitting new activity.

**Independent Test**: Exercise chat with no session, revoke a device session, remove a member, archive a circle, and verify the permitted and denied actions.

**Acceptance Scenarios**:

1. **Given** missing, invalid, revoked, inactive, or user-mismatched identity/session credentials, **When** any protected chat operation is attempted, **Then** it is rejected using project-standard denial semantics.
2. **Given** a removed member, **When** they attempt group-chat or qualifying-DM access, **Then** access is denied immediately and future realtime delivery stops.
3. **Given** a retained member of an archived circle, **When** they read, search, or play group-chat history accepted during their retained membership period, **Then** that retained content remains available.
4. **Given** an archived circle, **When** any user attempts to send, upload, type, mark read, pin, unpin, or delete in its group chat, **Then** the operation is rejected as new activity.
5. **Given** a member opens chat inside or outside the live-session room shell, **When** they perform the same authorized operation, **Then** behavior and stored history are identical and do not depend on session lifecycle or media state.

### Edge Cases

- Empty or whitespace-only text, text exceeding 4000 characters, invalid message type, or a message with incompatible text/media fields is rejected.
- An idempotency key reused by a different sender or with materially different content is rejected rather than returning or creating the wrong message.
- A reply target that is missing, deleted, belongs to another circle, or belongs to another DM conversation is rejected; deletion after replying removes the quoted content from subsequent projections.
- A reply target outside the caller's current membership period or temporarily inaccessible because DM eligibility was lost is rejected without exposing whether the target exists.
- Upload authorization is rechecked when the message is sent; an object cannot be attached by another user or reused across an unauthorized circle or DM context.
- Losing the DM upload's originally validated circle does not revoke its media while another qualifying shared active circle still authorizes the pair.
- A media link that has expired is renewed only after current access is authorized; removed members cannot renew it.
- Soft-deleting an attachment message immediately prevents new or existing application access to its object, but does not require synchronous physical deletion or a new F-004 purge job.
- Concurrent attempts to pin a sixth message cannot exceed the five-message limit.
- Concurrent self-delete requests remain idempotent; a teacher deletion racing with a sender deletion produces one soft delete and at most one teacher audit fact when the teacher action takes effect.
- Sender timestamps cannot extend the 10-minute deletion window; the authoritative acceptance time controls it.
- Duplicate read submissions produce one `MessageRead` fact per message and user.
- A sender's own read does not satisfy the recipient-read state.
- Typing events from unauthorized, removed, disconnected, or archived-circle users are rejected and stale indicators expire.
- Rate limits are enforced without converting accepted retries into duplicate messages. Group messages use the 30-per-minute user-and-circle budget; direct messages use the same 30-per-minute budget per sender and peer conversation.
- History and search use `(sent_at DESC, id DESC)` ordering. A cursor anchors both values: newer concurrent inserts do not shift older pages, deleted rows are skipped, and authorization loss between requests rejects the next page without partial data.
- A membership period starts at the accepted `circle_members.joined_at`, survives role changes and circle archival for retained history, and ends when the membership row is removed; reacceptance creates a new period.
- Offline items survive app restart. Cancellation discards the local envelope and any unsubmitted local file. `400`, `401`, `403`, `404`, `409`, `413`, `415`, and `422` are terminal and remain visible for edit/discard; network, timeout, `429`, and `5xx` retry at most three times per send cycle using 1/2/4-second backoff, after which explicit user retry is required.
- Successfully staged but unattached uploads count against the upload rate and are inaccessible; a cleanup worker removes their objects and metadata after 24 hours. Validation/auth failures that create no staged object do not consume the upload budget.
- A staged `upload_id` may be retried only by the same uploader, for the same conversation and same idempotent message payload, until it is attached once; a terminal send failure leaves it staged until explicit retry/discard or 24-hour cleanup.
- Actual MIME means server-side magic-byte detection plus successful media/PDF parsing where required; filename extensions and client `Content-Type` are never authoritative.
- Search normalizes Unicode to NFC, lowercases Latin, removes Arabic diacritics and tatweel, maps Arabic alef variants to `ا` and `ى` to `ي`, AND-matches normalized tokens with final-token prefix matching, then orders by rank, `sent_at DESC`, and `id DESC`.
- Read facts are accepted only for messages visible to the active reader at submission time. Pre-join and post-removal group messages cannot be marked read; sender self-reads never advance status; restored DM eligibility restores the retained pair history and its existing read facts.
- Microphone denial keeps the composer usable for text/media and exposes a settings action; recording interruption preserves a previewable partial file when valid or offers discard; playback/renewal failure exposes retry without changing message state; renewal denial removes the inaccessible media control without revealing storage details.
- Every per-client realtime write rechecks the authenticated backend session and current conversation authorization. A stale audience snapshot, revoked session, removed membership, archived mutation, or lost DM eligibility suppresses that write and increments only redacted denial/gap telemetry.
- All pin, unpin, and deletion operations that can change pinned state lock the owning circle row in one database transaction before counting/updating pins, so concurrent requests cannot expose more than five active pins.
- For attachment deletion, MinIO places a versionless delete marker before the database soft-delete transaction. If the process crashes while the database message remains active, reconciliation lists internal versions, removes only the latest matching delete marker, and restores versionless access; if the database row is deleted, reconciliation ensures a marker exists. Version IDs never cross the server boundary.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide exactly one group chat per circle and MUST allow every active member of an active circle to read non-deleted messages accepted on or after the start of their current active membership period and send text, voice, image, and PDF messages.
- **FR-002**: The system MUST make F-004 usable without an active F-005 live session and MUST keep chat storage, authorization, interfaces, events, and lifecycle independent from session and media state.
- **FR-003**: Every protected chat operation MUST validate current Firebase identity, the matching current-device backend session, and current target-circle authorization at the time of the operation.
- **FR-004**: The system MUST apply project-standard `401`, `403`, and `404` denial semantics without exposing private circle, conversation, user, or message existence to unauthorized callers.
- **FR-005**: Text content MUST be plain text, MUST be no longer than 4000 characters, and MUST be rendered without interpreting raw HTML or scripts.
- **FR-006**: Message sending MUST be limited to 30 messages per minute per user per circle for group chat and 30 messages per minute per sender-and-peer conversation for direct chat.
- **FR-007**: Every accepted send MUST produce one durable message identified by `message_id`; retries using the same sender and idempotency key MUST return that same message and MUST NOT create another durable message.
- **FR-008**: The client MUST persist a local pending envelope across app restart, retain the stable idempotency key across retries, support edit/discard after terminal failure, use at most three automatic retries per cycle with 1/2/4-second backoff for network/timeout/`429`/`5xx`, treat other contract failures as terminal, and reconcile against authoritative history after reconnect.
- **FR-009**: Realtime `chat.message` delivery MUST use the shared F-005 authenticated circle-topic transport obtained through `POST /api/v1/realtime/tickets`, MUST be at-least-once, and MUST NOT require or grant access to a live-session topic or media room.
- **FR-010**: Realtime authorization MUST be revalidated on connection, subscription, and immediately before each client write using the authenticated backend session and current PostgreSQL membership/role state; membership removal, session revocation, archival, and DM eligibility changes MUST suppress future unauthorized delivery even when an earlier audience snapshot included the client.
- **FR-011**: Clients MUST deduplicate `chat.message` by `message_id`, use stable `(sent_at,id)` keyset cursors, and re-fetch authoritative history after reconnect, a delivery gap, an unknown event, or REST/realtime arrival in either order.
- **FR-012**: The client MUST expose local-only pending and sent states; canonical REST/WebSocket `Message` projections MUST expose only server-authoritative delivered or read state and MUST retain per-user read facts uniquely by message and reader.
- **FR-013**: Recording a recipient read MUST be idempotent and MUST emit a sender-targeted `chat.message_read` update identifying the reader and read time.
- **FR-014**: The group-chat headline read state MUST activate after at least one currently eligible non-sender has a read fact; sender-visible message projections MUST include `read_receipts` containing only currently authorized readers and their authoritative read times, while other callers receive no per-member details.
- **FR-015**: The system MUST accept authorized start/stop typing commands, broadcast `chat.typing` only to the authorized conversation audience, automatically expire stale state, and MUST NOT persist typing as message history.
- **FR-016**: The system MUST support one direct conversation per unordered user pair only when the users share at least one active circle whose current roles form teacher-student or supervisor-student; direct realtime events MUST target the eligible users' authenticated connections directly and MUST NOT select or disclose one qualifying circle when several exist.
- **FR-017**: Direct-message authorization MUST be checked for listing, reading, sending, uploading, marking read, realtime delivery, and media-link renewal; losing the last qualifying active-circle relationship MUST revoke access, and later regaining a qualifying relationship MUST restore access to the existing complete pair history.
- **FR-018**: Student-student, teacher-teacher, supervisor-supervisor, teacher-supervisor, cross-circle-only, removed-member, and archived-only direct-message relationships MUST be rejected.
- **FR-019**: The system MUST allow user-initiated in-app chat voice-note recording with duration, amplitude-sample waveform visualization, preview playback, discard, and send actions using the ADR-021-approved minimal Flutter dependencies. This is not live-session recording and does not use or enable `FEATURE_RECORDING_ENABLED`.
- **FR-020**: Voice notes MUST be no longer than 300 seconds and no larger than 20 MB. The product specification MUST remain encoding/container-neutral while requiring supported mobile recording and playback.
- **FR-021**: Image attachments MUST be JPEG or PNG and no larger than 5 MB; file attachments MUST be PDF and no larger than 10 MB.
- **FR-022**: Uploads MUST validate magic-byte MIME, successful format parsing where applicable, size, and voice duration server-side; MUST return `415` for unsupported detected media; MUST enforce 10 successfully staged uploads per rolling hour per user across devices; and MUST remove inaccessible unattached staged objects/metadata after 24 hours.
- **FR-023**: Each chat upload MUST be bound to its authenticated uploader and server-validated target-circle authorization context. For DM media, any qualifying shared active circle MAY satisfy that check, the object MUST be bound to the intended pair conversation rather than owned by that circle, and access MUST continue only while at least one qualifying circle remains.
- **FR-024**: Voice, image, and PDF objects MUST be stored in access-controlled media storage; playback/download links MUST be presigned for 7 days and renewable only after current authorization succeeds.
- **FR-025**: The system MUST support reply-to within the same authorized conversation and MUST show a safe quoted preview that cannot reveal deleted or unauthorized content.
- **FR-026**: Active and retained archived-circle members MUST be able to search non-deleted group text accepted during their current retained membership period using the defined Arabic/Latin normalization, AND token matching, final-token prefix matching, and deterministic rank/`sent_at`/`id` ordering; results MUST exclude earlier periods, other circles, DMs, and deleted content.
- **FR-027**: Teachers and supervisors MUST be able to pin or unpin group messages in an active circle; students and non-members MUST NOT do so; pin/unpin/delete MUST serialize by locking the owning circle row before counting or changing pin state, and each circle MUST have no more than five pinned messages.
- **FR-028**: Pinned messages MUST be retrievable through a dedicated authorized group pinned-list operation, appear in pinned-at/id order in a pinned bar above group chat, and disappear from all projections when unpinned or deleted.
- **FR-029**: A sender MUST be able to soft-delete their own message for 10 minutes from authoritative acceptance; a later self-delete MUST return the contract-defined conflict.
- **FR-030**: A current teacher MUST be able to soft-delete any group-chat message in their circle; an effective teacher deletion MUST create a durable audit record containing actor, target message, circle, and time.
- **FR-031**: Soft-deleted content MUST be excluded from history, search, pinned lists, quoted previews, and realtime projections. Attachment deletion MUST place a versionless MinIO delete marker before database commit; marker failure rejects the mutation, marker-before-commit crashes MUST restore active-message access by removing only the latest internal marker, and physical prior versions MAY remain indefinitely until a future approved circle/account retention feature owns parent cleanup.
- **FR-032**: Retained members of an archived circle MUST be able to read, search, and play group-chat content accepted during their retained membership period, while all chat mutations—including sending, uploading, typing, marking read, pinning, unpinning, and deleting—MUST be rejected.
- **FR-033**: Removed members MUST lose group-chat history, search, realtime, DM, and media access immediately; retained data MUST not grant continued user access.
- **FR-034**: F-004 MUST NOT create background-notification triggers or implement FCM token management, push transport, notification preferences, or background-delivery behavior; F-008 owns those capabilities.
- **FR-035**: The additive v1 REST contract MUST cover group history/send/delete, direct history/send/own-delete, pin/unpin, pinned retrieval, sender-only read-detail projection, group/direct mark-read with archived-circle denial, circle-history search, media-link renewal, and voice/image/PDF uploads with validated group-or-DM target context and `415` failures.
- **FR-036**: The additive realtime contract MUST cover `chat.message`, `chat.message_deleted`, `chat.message_read`, `chat.typing`, a client typing command, audience rules, authorization revalidation, delivery guarantees, deduplication, and reconnect recovery.
- **FR-037**: Persistent F-004 schema changes MUST follow ADR-021's approved `messages`, `message_reads`, `chat_uploads`, `chat_event_outbox`, and `message_moderation_audits` ownership, include forward and rollback migrations, preserve unrelated feature-owned data, and keep authoritative message/read state durable.
- **FR-038**: The Flutter chat experience MUST be Arabic-first and RTL-aware while remaining correct in LTR; every critical history, send, offline retry, upload, playback, search, pin, delete, DM, and archived flow MUST expose deterministic empty/loading/retrying/terminal/permission states; controls MUST have screen-reader labels and 44×44 logical-pixel targets; status/error meaning MUST use text/icon semantics rather than color alone.
- **FR-039**: F-004 MUST include focused backend, mobile, contract, and integration coverage for primary flows, authorization, limits, retries, realtime redelivery, archival, deletion, and media safety.

### Safety and Reliability Requirements

- **SR-001**: Durable message/read state is authoritative; realtime events, local queues, and cached views are projections that reconcile to it. PostgreSQL failure rejects durable operations; MinIO failure rejects upload/revocation operations; WebSocket failure never rolls back a committed message and is repaired through bounded outbox retry plus REST reconciliation.
- **SR-002**: Authorization MUST be evaluated from current per-circle memberships and roles rather than global account roles, invitation claims after acceptance, cached client state, or live-session participation.
- **SR-003**: Invitation changes introduced by the 2026-09-03 ADR-010 amendment MUST NOT themselves grant chat access; access begins only after valid acceptance creates the active bound membership.
- **SR-004**: Message, read, pin, delete, pagination, search, and idempotency operations MUST remain correct under concurrent requests and at-least-once delivery, using unique constraints, keyset cursors, row locking, and idempotent transactions rather than process-local state.
- **SR-005**: User-provided filenames, text, quoted previews, and sender display data MUST be safely rendered and MUST NOT enable script, markup, or path injection. The same least-privilege DM, history, read-detail, media, deletion-audit, and retention rules apply when participants are minors; no age-derived access expansion or public discovery is introduced.
- **SR-006**: Presigned media URLs, object/version keys, user filenames, message bodies, quoted text, realtime tickets, and backend session identifiers MUST NOT be exposed to unauthorized users or written to logs/events. Redacted metrics/logs MUST cover authorization denial, realtime suppression/gaps, upload rejection, idempotency conflict, outbox retry/parking, and PostgreSQL/MinIO/WebSocket failures using only safe IDs/outcomes.
- **SR-007**: F-004 MUST remain correct and complete without F-008, background-notification triggers, or push delivery.
- **SR-008**: Teacher message deletion audit records MUST be append-only and must not contain message body or media credentials.

### Scope Boundaries

**In scope**:

- One group chat per circle and restricted teacher/supervisor-student direct conversations.
- Text, voice, JPEG/PNG, and PDF messages; replies, search, pins, soft deletion, read facts, typing, delivery states, offline queueing, and realtime updates.
- Additive v1 service contracts, durable schema and reversible migration requirements, Arabic-first mobile chat UI, and focused verification.
- Integration into the live-session room shell as a presentation entry point only; the same circle chat works outside it.

**Out of scope**:

- Announcement-only channels, emoji reactions, message editing, forwarding, student-student DMs, unrestricted global DMs, multi-circle broadcast, chat analytics, and moderation dashboards.
- End-to-end encryption for DMs, live-session lifecycle/media, recitation queue behavior, scheduling, progress/history views, per-message physical media purge jobs, background-notification triggers, and push-notification infrastructure.
- Firebase Auth redesign, global roles, new roles, session-lifecycle changes, or any chat permission derived from live-session participation.
- A product-mandated voice codec/container; this is selected during planning within the fixed duration, size, compatibility, and validation requirements.
- Implementing the ADR-010 invitation amendment inside F-004; F-004 consumes the active memberships produced by F-002.
- An inbox/eligible-conversation discovery endpoint, extracted waveform package, background audio service, live-session capture/storage, and physical purge scheduling; only the existing pair route, amplitude-drawn waveform, foreground playback, and stated cleanup behavior are included.

### Key Entities

- **Message**: A durable circle or direct message with sender, type (`text`, `voice`, `image`, or `file`), content or authorized media reference, optional same-conversation reply reference, acceptance time, pin state for group messages, and optional soft-deletion time.
- **MessageRead**: An idempotent per-user fact that a recipient read a message, unique for each message and user, with authoritative read time.
- **PinnedMessage**: The active group-chat projection of a message's pin state; at most five are visible for a circle and no separate announcement channel exists.
- **ChatMediaObject**: An access-controlled voice, image, or PDF object bound to its uploader and authorization context, with safe filename/size metadata and renewable 7-day access links; message soft deletion revokes application access immediately without requiring physical deletion before normal parent-data cleanup.
- **OfflineSend**: Client-held pending message data with a stable idempotency key and local state until durable acceptance or a terminal validation/authorization failure.
- **RealtimeTicket**: The reused short-lived F-005 connection authorization for currently eligible circle topics; it grants neither chat-domain permissions by itself nor live-session/media access.
- **RoleBoundInvitation**: An F-002-owned invitation whose selected circle role is fixed at issuance; F-004 relies only on the active membership created after acceptance.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every F-004 acceptance criterion in `FEATURES.md`, except background FCM delivery now assigned wholly to F-008, maps to at least one numbered requirement and independently testable acceptance scenario.
- **SC-002**: 100% of tested valid online, offline-retry, reconnect, and realtime-redelivery flows create exactly one durable message per sender idempotency key and display no duplicate client messages.
- **SC-003**: 100% of tested non-member, removed-member, revoked-session, unauthorized cross-circle, disallowed DM-pair, unauthorized pin, archived-circle mutation, pre-join/post-removal read, and inaccessible-reply attempts are rejected; tested rejoin periods, DM eligibility restoration, and any-qualifying-circle DM media expose exactly the retained authorized history.
- **SC-004**: 100% of tested oversized text, over-duration/oversized voice, oversized/invalid image, oversized/invalid PDF, cross-context media, late self-delete, and sixth-pin attempts are rejected without unauthorized durable state.
- **SC-005**: Under the reproducible MVP fixture—10 active circles, 5 active users per circle (50 concurrent users), 10,000 messages per circle with 10% deleted and two membership periods—history page size 100 and 1,000 first-page search samples each achieve p95 ≤2 seconds after 100 warm-up operations on the single Docker Compose deployment.
- **SC-006**: Under the same 50-user/10-circle fixture, 1,000 accepted-message observations achieve p95 ≤2 seconds from database commit to authorized online client receipt; a second run with realtime delivery intentionally suppressed proves REST reconciliation returns authoritative state. Live sessions are not required for either run.
- **SC-007**: 100% of soft-deleted messages are absent from subsequent history, search, pinned lists, quoted previews, realtime projections, and renewed media access tests.
- **SC-008**: Chat's complete tested group workflow succeeds with no active live session and with F-003, F-006, and F-008 implementation unavailable.
- **SC-009**: All tested Arabic and LTR chat layouts expose the specified empty/loading/retrying/terminal/permission states without clipped controls, reversed message meaning, color-only status dependence, missing screen-reader labels, sub-44×44 targets, or unsafe rendering.
- **SC-010**: The specification has no unresolved clarification markers and introduces no global role, duplicate realtime transport, or dependency on session lifecycle.

## Assumptions

- F-001, F-002, F-003, and F-005 are complete dependencies as confirmed by Karim on 2026-09-03; current repository evidence must still be checked before implementation or release claims.
- F-001 continues to own Firebase identity and current-device backend sessions; F-002 continues to own circle membership, roles, invitations, removal, and archive state.
- The 2026-09-03 ADR-010 amendment is approved product direction but requires its own F-002 implementation/contract work before role-bound invitation behavior can be relied upon in production.
- F-005 continues to own the generic realtime ticket and topic transport; F-004 owns only chat-domain commands, events, storage, and client behavior.
- F-008 owns any future background-notification trigger and push-delivery integration; F-004 exposes durable messages through its own approved interfaces and does not depend on F-008.
- Leaving and later rejoining a circle starts a new membership period; the rejoining member does not regain group messages from an earlier membership period.
- A DM is one logical conversation per user pair. Any one currently qualifying shared active circle authorizes the pair; losing all qualifying circles revokes access.
- A temporary loss of DM eligibility does not split or erase the pair's conversation; complete existing history becomes accessible again only if the pair later regains eligibility.
- For direct messages, the same 30-message-per-minute protection applies per sender and peer because no single circle owns a pair that may share multiple circles.
- Normal REST operations use a 10-second connect timeout and 15-second response timeout; uploads use 60 seconds. `429` honors `Retry-After` capped at 30 seconds. Dependency calls and outbox retries are bounded and observable.
- The constitutional 10-simultaneous-live-session ceiling is a coexistence constraint, not a chat authorization or load input. Chat performance is proven independently with zero live sessions; a separate integration check may keep ten session records/topics active without changing chat results.
- F-004 in its entirety—all P1 and P2 user stories in this specification—is MVP scope. Story priority controls implementation order and independently testable checkpoints, not release exclusion.
- Voice codec/container selection is deliberately deferred to planning and contract work and is not a user-visible product commitment.
