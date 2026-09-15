# Feature Specification: Provider Adapter Boundaries

**Feature Branch**: `018-provider-adapter-boundaries`  
**Created**: 2026-09-16  
**Status**: Approved scope; implementation pending  
**Input**: Karim approved recording and executing the reviewed Provider Adapter Boundary Refactor (F-018; PROVIDER-BOUNDARY decisions).

## Clarifications

### Session 2026-09-16

Previously accepted answers are recorded here; no questions are repeated.

- Q: Who controls provider configuration? → A: Global deployment configuration only.
- Q: Is there a settings interface? → A: No mobile or admin settings UI.
- Q: Are alternative providers implemented now? → A: Establish interfaces and seams now; alternatives later.
- Q: When may replacement occur? → A: Before live deployment; migration and failover are outside this feature.
- Q: Which capabilities are covered? → A: Stored chat attachments using MinIO and live session audio using LiveKit only.
- Q: How should business documents describe them? → A: Capability-first wording, retaining the currently selected provider names.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Preserve private attachment lifecycle (Priority: P1)

As a participant, I can upload and access authorized chat attachments, and deletion revokes access immediately while retained bytes and crash recovery remain intact.

**Why this priority**: Storage replaceability must preserve privacy and established deletion guarantees.

**Independent Test**: Exercise upload, authorized download, deletion, cleanup, and recovery against the current storage service without changing live-session behavior.

**Acceptance Scenarios**:

1. **Given** an authorized valid attachment, **When** it is staged and finalized, **Then** its server-generated reference and existing access response remain unchanged; filenames never determine the stored reference.
2. **Given** a previously issued download link, **When** its message is deleted, **Then** that versionless link loses access immediately while historical bytes remain retained.
3. **Given** deletion reached storage before its database transaction committed, **When** recovery observes an active message, **Then** it removes the latest internal deletion marker without deleting retained bytes.
4. **Given** a deleted message or expired staged upload, **When** reconciliation or cleanup retries, **Then** access stays revoked and failures retain the existing retry behavior.

### User Story 2 - Preserve authorized live audio (Priority: P2)

As a teacher or student, I use the same authorized audio session and moderation flows while provider setup remains behind the media integration boundary.

**Why this priority**: Provider changes should not spread into session policy, queue behavior, or user flows.

**Independent Test**: Exercise the existing session lifecycle, authorization, moderation, and mobile recovery suites independently of attachment storage.

**Acceptance Scenarios**:

1. **Given** existing deployment settings, **When** the backend starts, **Then** the current media integration is directly selected and its configuration validation remains unchanged.
2. **Given** an authorized session participant, **When** they start or join, **Then** connection shape, credential protection, audio-only permissions, and existing session recovery behavior remain unchanged.
3. **Given** the mobile media boundary, **When** application state consumes it, **Then** it depends on neutral connection and lifecycle types; deployment wiring selects the existing adapter separately.

### Edge Cases

- Missing, partial, invalid, or entirely absent deployment configuration keeps current startup/disabled behavior; storage startup requires an existing versioned private bucket and never creates or enables one.
- Invalid attachment type, MIME, size, filename, and authorization retain existing rejection behavior and server validation.
- Repeated delete, absent marker, version-listing failure, failed exact marker removal, deletion-before-commit crash, cleanup failure, and reconciliation retries preserve existing semantics.
- Provider failures, credential expiry, removed membership, ended sessions, and duplicate webhooks retain existing terminal/retry behavior.
- Arabic/RTL and accessibility behavior remain unchanged because no new UI is introduced.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Stored attachment operations MUST use a chat-owned neutral boundary; provider libraries, provider types, and provider client construction MUST remain inside the storage adapter.
- **FR-002**: Chat MUST retain server-generated deterministic attachment references and validation policy; no client-selected key or filename may determine storage paths or metadata.
- **FR-003**: Storage MUST preserve staging, versionless download signing, startup validation, immediate logical revocation, idempotent marker application, retained versions, exact internal marker removal, and latest-marker crash recovery.
- **FR-004**: Durable attachment state MUST retain internal object references only; signed URLs, provider version IDs, and client keys MUST NOT become durable or public identity.
- **FR-005**: Live audio provider client construction MUST remain inside its existing adapter; session and queue policy MUST remain provider-neutral.
- **FR-006**: Mobile media interfaces and connection types MUST be free of concrete provider composition and API-client dependencies; separate wiring MUST directly supply the current adapter.
- **FR-007**: Existing global configuration names, defaults, validation, optional disabled behavior, secret redaction, and operational timeouts MUST remain unchanged.
- **FR-008**: Public REST and WebSocket contracts, persistence, authorization, deletion ordering, lifecycle recovery, audio fidelity, and user flows MUST remain unchanged.
- **FR-009**: Architecture guards MUST cover backend application composition as well as internal packages, and mobile boundaries; they MUST complement behavioral and real-storage verification.
- **FR-010**: Scope MUST remain MinIO storage and LiveKit audio only: no second provider, registry, selection flag, settings UI, schema migration, migration/failover machinery, or added dependency.

### Key Entities

- **Attachment reference**: Existing internal server-owned object key associated with a staged/attached/revoked upload; no new fields.
- **Media connection**: Existing endpoint, ephemeral participant credential, and expiry; held in memory and never persisted or logged.
- **Deletion marker**: Storage-private revocation mechanism; exact version identity stays inside the adapter.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: All established attachment upload/access/deletion/cleanup/recovery acceptance scenarios pass with the selected storage service.
- **SC-002**: All established live-audio lifecycle, authorization, moderation, and mobile recovery acceptance scenarios pass.
- **SC-003**: There are zero changes to public payloads, durable schema, deployment configuration behavior, or user-facing flows.
- **SC-004**: All applicable quality gates succeed on the final tree, with mandatory Karim security review recorded separately before merge.

## Assumptions

- F-004 has merged to main at `dda98ef`; this feature preserves that accepted baseline without altering its generated artifacts or rerunning its task workflow.
- The current selected infrastructure remains required by constitution §§III–V; replacing it later requires separate approval and ADR/constitution work where applicable.
- Target remains 50 concurrent users and at most 10 simultaneous live sessions on the current MVP deployment.
- Approval to implement does not waive tests, review, or merge authority.
