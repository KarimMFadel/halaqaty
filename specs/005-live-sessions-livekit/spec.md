# Feature Specification: Live Sessions (LiveKit)

**Feature Branch**: `005-live-sessions-livekit`  
**Created**: 2026-08-15  
**Status**: Planned — canonical contract synchronization remains an implementation gate  
**Input**: F-005 template supplied by Karim

## Scope

F-005 provides the independently usable, circle-scoped audio live-session foundation: session lifecycle, durable participant presence and hand state, authenticated realtime transport, and a provider-neutral media boundary backed by one self-hosted LiveKit adapter. It reuses F-001 identity/current-device sessions and F-002 circle membership and roles.

It does not implement recitation queues or turn orchestration (F-003), chat (F-004), scheduling/recurrence or attendance policy (F-006), general push delivery (F-008), video, screen sharing, recording, or transcription.

## User Scenarios & Testing

### User Story 1 - Start and join an audio session (Priority: P1)

An authorized teacher starts an eligible circle session. Members can join an active, unlocked session and use the Arabic-first session room without an external meeting tool.

**Why this priority**: It is the core value and must work without the queue or chat features.

**Independent Test**: Start one session, then join it as a member on another authenticated device; verify a single room, correct audio permissions, presence, and no video capability.

**Acceptance Scenarios**:

1. **Given** a teacher or supervisor and an eligible F-005 ad-hoc session, **When** they start it, **Then** exactly one audio-only media room is created, the session becomes `active`, and they receive their own complete short-lived media connection.
2. **Given** an active unlocked session and an active circle member, **When** they join, **Then** they receive only their own media connection and appear once in current participant presence.
3. **Given** a student joins, **When** their connection is issued, **Then** it permits subscribing but not audio or video publishing.
4. **Given** a start request is retried for an active session, **When** it is processed, **Then** no second room is created and the caller receives a newly issued connection.

---

### User Story 2 - Run and moderate a safe room (Priority: P1)

An authorized teacher can end a session, lock or unlock new joins, mute a participant, mute all participants, and remove a participant. Participants can raise and lower a hand without any queue behavior.

**Why this priority**: Quran circles require teacher-controlled, safe participation while preserving students’ ability to ask to recite.

**Independent Test**: In an active room, exercise each teacher control and hand command; verify authorization, durable state, idempotency, and reconnection enforcement.

**Acceptance Scenarios**:

1. **Given** an active session, **When** a moderator locks it, **Then** new joins are rejected, an eligible participant who was present before the lock may reconnect, and unlocking restores eligible joins.
2. **Given** an active participant, **When** a teacher removes them, **Then** they are disconnected and cannot rejoin by retrying or reconnecting.
3. **Given** an active session, **When** any active participant raises or lowers their hand, **Then** authorized connected participants receive the resulting authoritative hand state exactly once after deduplication.
4. **Given** an active session, **When** a moderator, the four-hour duration limit, or the idle timeout ends it, **Then** new joins stop, participants are disconnected, durable history remains, the room is closed, and the resulting end reason is recorded.

---

### User Story 3 - Recover from connection loss (Priority: P2)

A participant sees understandable connection and error states and can recover from a transient network loss without duplicate presence or endless loading.

**Why this priority**: Mobile connectivity is variable; graceful recovery protects a live learning experience.

**Independent Test**: Simulate transport and media reconnection, credential expiry, removal, lock, session end, and revoked backend session; verify each reaches the appropriate recovered or terminal state.

**Acceptance Scenarios**:

1. **Given** a recoverable network interruption and a usable media credential, **When** connectivity returns, **Then** the client reconnects and reconciles authoritative session, participant, and hand state.
2. **Given** a credential near expiry or expired, **When** recovery is needed, **Then** the client performs an authenticated start/join refresh rather than reusing the old credential.
3. **Given** removal, an ended session, revoked identity, or provider unavailability, **When** recovery is attempted, **Then** the client stops retrying when the failure is terminal and presents a clear Arabic-first error state.

## Edge Cases

- The 51st concurrent participant is rejected without a connection being issued, including concurrent join races.
- Scheduled, ended, full, missing, archived-circle, unauthorized, or stale-membership requests never issue a media connection; a locked room permits only an eligible pre-lock participant to reconnect.
- An ad-hoc session appears in circle session-card discovery while `scheduled` or `active`; only `active` sessions can be joined, and starting is an explicit teacher/supervisor action.
- Duplicate start, join, end, webhook, reconnect, and hand-state deliveries converge to one durable result.
- A provider timeout or partial create/close failure leaves the session non-joinable until reconciliation reaches a safe state; orphaned rooms are closed.
- A session ends at four hours, or after thirty minutes empty, and preserves history without retaining credentials or recording media.
- Video publishing, screen sharing, recording, cross-circle joins, credential reuse, and student audio publishing outside a future F-003 reciter turn are rejected.

## Requirements

### Functional Requirements

- **FR-001**: The system MUST reuse Firebase identity verification, the current backend device session, and active per-circle membership for every protected F-005 operation.
- **FR-002**: Teachers and supervisors MUST be able to create F-005 ad-hoc sessions only (`scheduled_at` is null). Creation persists the row in `scheduled` status for discovery; an explicit start request transitions it to `active`, and end transitions it to `ended`. F-005 MUST NOT auto-start or create recurring/scheduled sessions; F-006 owns those capabilities.
- **FR-003**: The system MUST create at most one opaque media room for a started session and MUST keep its provider room reference out of public session objects and realtime broadcasts.
- **FR-004**: A successful start or join MUST return a required participant-specific `MediaConnection` containing trusted TLS endpoint, opaque credential, and actual credential expiry, with `Cache-Control: no-store`.
- **FR-005**: Only the Go backend MAY issue media credentials; each credential MUST be identity-specific, least-privilege, memory-only, valid for at most one hour, and never logged, persisted, cached, broadcast, or placed in a URL.
- **FR-006**: The LiveKit-backed MVP MUST be audio-only: every connection forbids video publishing, and F-005 exposes no video, camera, screen-share, or recording control.
- **FR-007**: Teacher connections MUST have audio publishing and required moderation authority; student connections MUST subscribe but not publish audio or video. Only the future F-003 reciter control may grant a student temporary audio publishing.
- **FR-008**: Quran-recitation media configuration MUST use Opus at 48 kbps or higher and disable noise suppression, automatic gain control, and echo cancellation where the platform permits.
- **FR-009**: The system MUST enforce a maximum of 50 concurrently present participants, four-hour session duration, and a 30-minute idle timeout after the final participant leaves.
- **FR-010**: The system MUST persist authoritative join, leave, reconnect, current-presence, participant-count, removal, and standalone hand-raise facts for every active participant, sufficient for later queue ordering and attendance policy, without evaluating attendance or creating queue behavior.
- **FR-011**: The system MUST provide an authenticated realtime connection model with heartbeat, reconnect, topic authorization, authorization revalidation, at-least-once delivery, and idempotent client/server handling for `session.*` events and hand commands.
- **FR-012**: The realtime authorization model MUST use `POST /api/v1/realtime/tickets` to issue a generic authenticated ticket for all currently eligible circle topics. Session-topic access MUST be added only after an authorized session join and revalidated by the WebSocket hub. F-004 may reuse circle topics without requiring an active live session.
- **FR-013**: Authorized participants MUST receive an authoritative session snapshot after reconnect; broadcasts MUST never include media credentials or room references.
- **FR-014**: Teachers and supervisors MUST have the same F-005 moderation rights: start/end, mute all, mute or unmute an existing audio publisher, remove a participant, and lock or unlock a room. Unmute MUST restore only an existing publish entitlement and MUST NOT grant a student publishing permission.
- **FR-015**: An ended session MUST prevent new joins, disconnect participants, clear only approved transient presence/hand state, preserve durable history, and be persisted as ended before its room is closed. It MUST record `manual`, `duration_limit`, or `idle_timeout` as the end reason; automatic endings have no human `ended_by` attribution.
- **FR-016**: Start, join, end, moderation, lock, presence/webhook processing, and hand commands MUST be safe under retries and duplicate delivery.
- **FR-017**: A failed media-provider operation MUST return a non-success response and MUST NOT return a partial or null media connection.
- **FR-018**: The system MUST reconcile database-to-provider create, close, permission, presence, duplicate-webhook, and process-crash windows without making a room joinable prematurely.
- **FR-019**: The backend MUST use a typed, audio-only sessions-owned media gateway and reciter-audio-control boundary; only `backend/internal/sessions/livekit/` may import LiveKit SDK/JWT/room/webhook types.
- **FR-020**: The Flutter session state and UI MUST consume a provider-neutral `MediaSession`; only `mobile/lib/features/sessions/data/livekit_media_session.dart` may import `livekit_client`.
- **FR-021**: The system MUST directly construct and inject the single LiveKit adapter and MUST NOT introduce provider selection, registries, flags, identifiers, multiple adapters, or generic capability maps.
- **FR-022**: The Flutter session-room shell MUST be Arabic-first and RTL-aware and show participant presence, audio connection state, teacher controls, hand controls, empty states, and terminal/recoverable errors without queue or chat UI.
- **FR-023**: The system MUST emit structured, credential-safe audit events and useful room, participant, reconnect, and error metrics without recording audio or sensitive request bodies.
- **FR-024**: Public session data MUST include `media_mode`, with `audio_only` required for F-005 and `audio_video` reserved for a separately approved future feature.
- **FR-025**: Sessions MUST use `actual_start` and `actual_end`; a dedicated `session_participant_presence` model MUST persist live presence and standalone hand state. F-006 owns the separate `session_attendance` attendance classification and override model.
- **FR-026**: A room lock MUST prevent new joins while allowing a current eligible participant who joined before the lock to reconnect, provided they were not removed and the session remains active.

### Key Entities

- **Session**: Circle-scoped lifecycle record with scheduled, active, or ended state; audio-only media policy; timestamps; lock state; opaque room reference; and participant count.
- **SessionParticipantPresence**: Durable join, leave, reconnect, current-presence, removal, and standalone hand-state facts; it does not assign attendance status.
- **MediaRoom**: Provider-neutral representation of a session’s one opaque media room.
- **MediaConnection**: Private participant-specific endpoint, opaque credential, and expiry returned only on authorized start/join.
- **HandRaiseState**: Current persisted raise/lower state for any active session participant.
- **RealtimeConnectionTicket**: Short-lived authenticated authorization for the caller's allowed circle and session realtime topics.

## Success Criteria

### Measurable Outcomes

- **SC-001**: A teacher and up to 49 other eligible members can complete start, join, moderate, reconnect, and end flows without F-003 or F-004.
- **SC-002**: Tested video-publish, unauthorized moderation, cross-circle join, credential-reuse, locked-room, ended-room, full-room, and out-of-turn student-publish attempts are rejected.
- **SC-003**: Duplicate start, join, end, webhook, and hand-state operations produce one durable outcome and a consistent participant state.
- **SC-004**: Recoverable network interruptions converge to authoritative state without duplicate participants or indefinite loading.
- **SC-005**: Contract and dependency tests prove no LiveKit SDK types or `livekit_*` fields escape the approved adapters, and no multi-provider machinery is introduced.

## Assumptions

- F-001 and F-002 remain the authoritative identity, device-session, circle membership, and role sources.
- LiveKit is self-hosted and is the only F-005 media adapter; its endpoint configuration is trusted and TLS-enabled.
- F-005 may add shared realtime transport only when it remains generic and does not implement F-004 chat behavior.
- F-005 creates the complete base `sessions` table and `session_participant_presence`; F-003 and F-006 extend those tables only through later paired migrations for queue and attendance concerns.
- Current `session_attendance` fields are not treated as an attendance policy; F-006 owns that policy and manual overrides.

## Resolved Clarification Decisions

1. Teachers and supervisors have the same F-005 moderation rights.
2. Individual unmute restores only an existing audio-publish entitlement; F-003 alone may grant a student turn-based publishing and must revalidate it.
3. `POST /api/v1/realtime/tickets` issues a generic authenticated ticket for authorized circle and session topics, replacing the session-only ticket model.
4. Sessions use `actual_start` / `actual_end`; F-005 owns `session_participant_presence`, while F-006 owns `session_attendance` policy and overrides.
5. F-005 creates ad-hoc sessions only; F-006 owns scheduled-session creation.
6. Lock blocks new joins but permits eligible pre-lock participants to reconnect; every active participant may raise or lower a hand.
7. Automatic end records `duration_limit` or `idle_timeout` with no human attribution; circle topics use generic tickets and session topics require a successful join.
