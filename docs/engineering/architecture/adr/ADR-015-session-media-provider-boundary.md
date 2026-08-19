# ADR-015: Replaceable Session-Media Provider Boundary

**Status:** Accepted  
**Date:** 2026-08-15  
**Deciders:** Karim (product owner)

---

## Context

ADR-008 selects self-hosted LiveKit for MVP live audio. That choice remains valid,
but its implementation sketch couples session orchestration, queue coordination,
public response fields, and Flutter state directly to LiveKit concepts. Replacing
LiveKit later would therefore require changes outside the media integration itself.

Halaqaty needs a narrow session-media seam so a future approved provider or custom
media implementation can replace LiveKit without redesigning session lifecycle,
authorization, the recitation queue, REST/WebSocket behavior, or session UI. The
boundary is named for session media so a future approved video feature can extend
it, but F-005 remains strictly audio-only. This does not justify a project-wide
Clean/Onion Architecture conversion, database abstraction, dynamic plugin system,
or multiple providers in MVP.

## Decision

### Constitutional alignment

This boundary preserves the constitution's current choice of self-hosted LiveKit,
backend-only credential issuance, audio-only MVP permissions, and Quran-recitation
audio profile. It changes dependency direction and public naming, not the selected
MVP provider. Replacing LiveKit or enabling video later still requires the relevant
constitution amendment plus an approved feature specification and ADR.

### MVP boundary

LiveKit remains the sole MVP session-media implementation.

- `backend/internal/sessions` owns provider-neutral session policy and a small
  `SessionMediaGateway` contract expressed in Halaqaty operations: ensure/close a
  room, issue a participant-specific connection, apply audio-publish permission,
  mute active audio, and remove a participant.
- `backend/internal/sessions/livekit` implements that contract and is the only
  backend package allowed to import LiveKit SDK, JWT, room-service, track, or
  webhook types. Composition constructs and injects it directly; there is no
  provider registry, resolver, or selection flag while only LiveKit exists.
- Provider webhook signatures are verified inside the LiveKit adapter before
  payloads are translated into neutral, idempotently processed session events.
- F-003 never imports LiveKit or `SessionMediaGateway`. It calls a sessions-owned
  `ReciterAudioControl` application boundary; sessions maps the authoritative
  reciter entitlement to provider permission changes.
- `mobile/lib/features/sessions/application` owns a provider-neutral
  `MediaSession` contract and safe connection/lifecycle states.
  `mobile/lib/features/sessions/data/livekit_media_session.dart` is the only
  mobile file allowed to import `livekit_client`. Riverpod composition injects it
  directly during MVP.
- Public `Session` exposes a required server-controlled `media_mode`; F-005 always
  returns `audio_only`. The reserved `audio_video` value may be used only after a
  future video feature and its security/privacy decisions are approved.
- Successful start/join responses return a required opaque `MediaConnection`
  (`endpoint`, `credential`, `expires_at`) rather than provider-named fields. All
  three fields are required. Provider failure returns an error, never a successful
  response with a partial or nullable connection.
- The endpoint comes only from trusted adapter configuration and uses TLS. The
  credential is participant-specific, valid for at most one hour in MVP, kept in
  memory only, and never persisted, logged, cached, placed in URLs, or broadcast
  through WebSocket events. `expires_at` comes from the actual signed credential.
- Near or after credential expiry, clients repeat the authorized start/join
  operation to obtain a fresh connection. Ended sessions, removed participants,
  and revoked membership/authentication fail terminally and receive no credential.
- Starting is idempotent: replay for an active session returns that same `Session`
  plus a newly issued caller-specific connection. It never creates a second room.
- Session persistence uses an opaque `media_room_ref`; provider-specific room
  identifiers are not exposed by the public `Session` representation.
- Database/session state remains authoritative for lifecycle, membership,
  presence, removal, room lock, and publish entitlement. Provider state is
  reconciled to it after partial failure or duplicate webhook delivery.
- The concrete recovery and reconciliation policy is frozen in ADR-017.

The MVP gateway contains only the typed audio operations required by F-005. It has
no video, camera, screen-share, recording, generic capability-map, or arbitrary
publication API. A future video feature extends or composes this seam through its
own approved specification and ADR without changing the `MediaConnection` envelope.

### Future provider replacement

Dual-provider machinery is added only when a second provider is approved and is
actually being introduced:

1. Add and backfill an immutable `media_provider` value for sessions, then enforce
   a closed constraint and uniqueness on `(media_provider, media_room_ref)`.
   Provider selection is pinned when the durable session is created and is never
   recalculated from the current flag.
2. Compile and deploy both backend and mobile adapters.
3. Add a small closed resolver/switch in composition and a versioned `driver` in
   `MediaConnection`; unknown providers and drivers fail safely.
4. Gate selection on compatible mobile-version adoption.
5. Use a feature flag to select the provider only for newly created sessions.
   Every existing session remains pinned to its original provider.
6. Rollback changes only new-session selection. Both adapters remain deployed
   until pinned sessions, pending operations, and webhook retries drain.
7. Make the new provider the default, then remove the old adapter in a later
   cleanup release after database and metrics checks show it is unused.

No active room is migrated between providers. Provider selection is never accepted
from a client or untrusted webhook body.

## Consequences

- LiveKit-specific change is confined to two adapters and deployment configuration.
- A future provider requires new adapters and rollout wiring, not changes to core
  session, queue, API, event, or UI behavior.
- DB-to-provider operations still cross system boundaries; F-005 must design
  idempotent reconciliation for create, close, permission, presence, and webhook
  crash windows rather than pretending they are one transaction.
- A session remains non-joinable while its deterministic room is ensured. Only
  then does a compare-and-set persist `active` and `media_room_ref`. Activation
  failure compensates by closing the orphan room; a sessions-owned reconciler
  repairs orphan or missing rooms after process crashes. End persists `ended`
  first, then closes the room idempotently and retries cleanup.
- A custom WebRTC/SFU implementation remains substantial infrastructure work, but
  it can replace the adapters without an application redesign.
- LiveKit-specific monitoring and configuration remain valid for MVP, while
  application logs and metrics use provider-neutral operation names.

## Explicit Non-Goals

- Project-wide Clean/Onion Architecture or database repositories introduced only
  for replaceability
- Go dynamic plugins, runtime downloads, reflection-based registration, generic
  factories, or tenant-selected providers
- Multiple session-media providers in MVP
- Arbitrary provider metadata/capability maps
- Mid-session provider migration or automatic failover between providers
- Video, screen sharing, or recording in F-005
- Building a custom signaling, TURN, or SFU stack in F-005

## Alternatives Considered

| Alternative | Reason Rejected |
|---|---|
| Direct LiveKit calls throughout sessions and Flutter | Makes future replacement cross-cutting and leaks provider credentials/types into contracts and UI |
| Flat optional `media_room_name` / `media_tool_url` / `media_token` fields | Exposes provider semantics, permits invalid partial success states, and makes sensitive connection material easier to leak |
| Runtime provider framework in MVP | Speculative complexity with one implementation and no approved second provider |
| Project-wide Clean/Onion conversion | Unrelated to the targeted media volatility and contrary to MVP restraint |
| Build custom WebRTC now | Delays MVP and recreates signaling, TURN/SFU, mobile, reconnection, and observability infrastructure |

## References

- [ADR-001 — Modular Monolith](ADR-001-modular-monolith.md)
- [ADR-005 — Feature Flags](ADR-005-feature-flags.md)
- [ADR-008 — WebRTC Solution](ADR-008-webrtc-solution.md)
- [ADR-014 — MVP Deployment](ADR-014-mvp-deployment.md)
- [Architecture](../ARCHITECTURE.md)
- [Canonical OpenAPI contract](../../../contracts/openapi.yaml)
- [WebSocket event catalog](../../../contracts/ws_events.md)
