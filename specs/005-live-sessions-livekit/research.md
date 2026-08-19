# Research: Live Sessions (LiveKit)

## Decision 1: One narrow LiveKit adapter

- **Decision**: Construct and inject one `SessionMediaGateway` implementation backed by self-hosted LiveKit; only the approved backend/mobile adapter paths import provider SDK types.
- **Rationale**: ADR-015 preserves a future replacement seam without speculative provider selection machinery.
- **Rejected**: Direct LiveKit use throughout the app; registry/resolver/feature flag; custom WebRTC/SFU.

## Decision 2: Authoritative lifecycle and reconciliation

- **Decision**: PostgreSQL owns lifecycle, lock, removal, capacity, presence, and publish entitlement. Room ensure happens before CAS activation; end is persisted before close; a reconciler repairs provider/database crash windows.
- **Rationale**: A database transaction cannot include LiveKit API calls or webhook delivery.
- **Rejected**: In-memory room state; marking active before room ensure; treating webhooks as exactly once.

## Decision 3: Credential and audio policy

- **Decision**: Go issues participant-specific opaque connections for no more than one hour. Students subscribe only; every token has `CanPublishVideo=false`. Audio uses Opus ≥48 kbps and disables processing where supported.
- **Rationale**: Constitutional security and Quran audio-fidelity invariants.
- **Rejected**: Client-generated tokens, shared room links, video toggle, recording, or silent audio-processing fallback.

## Decision 4: Generic realtime transport with scoped topics

- **Decision**: `POST /realtime/tickets` authorizes current circle topics; a successful session join adds a session topic. Hub authorization is revalidated at connection/subscription and state snapshots repair at-least-once delivery.
- **Rationale**: F-004 can reuse transport while session presence remains private to room participants.
- **Rejected**: Session-only tickets or exposing session presence to every circle member.

## Decision 5: Presence is not attendance

- **Decision**: F-005 owns `session_participant_presence` raw facts and `actual_start`/`actual_end`; F-006 owns separate attendance policy and overrides.
- **Rationale**: Presence must be reliable now without prematurely defining future educational policy.

## Decision 6: Moderation and reconnect policy

- **Decision**: Teachers and supervisors share F-005 moderation. Unmute only restores an existing publisher; lock blocks new joins but not an eligible pre-lock reconnect; removal blocks all retries.
- **Rationale**: Supports supervisors while preserving F-003’s student-publish boundary and mobile recovery.

## Decision 7: Automatic lifecycle outcomes

- **Decision**: Four-hour and idle timeout endings persist `duration_limit` or `idle_timeout` with no human attribution.
- **Rationale**: Audit records must distinguish automatic lifecycle action from a moderator decision.

## Decision 8: Recovery and reconciliation clarification (2026-08-19)

- **Decision**: Use ADR-017's stable HMAC-derived room reference, shared
  session advisory lock, bounded startup/30-second reconciler, recoverable
  `503 ERR_MEDIA_UNAVAILABLE`, and end-before-close cleanup semantics.
- **Rationale**: Spec-Kit review found and resolved conflicts between the F-005
  specification, product journey, canonical contracts, ADR-015, and the current
  implementation. The decision preserves PostgreSQL authority without adding
  a recovery table or provider-selection machinery.
- **Rejected**: Literal session-ID room names, random room names per retry,
  infinite REST retries, provider-outage auto-ending, and close-before-end.
