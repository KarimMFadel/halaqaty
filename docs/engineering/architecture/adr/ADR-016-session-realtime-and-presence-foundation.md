# ADR-016: Shared Realtime Tickets and Session Presence Boundary

**Status:** Accepted  
**Date:** 2026-08-15  
**Deciders:** Karim (product owner)

## Context

The existing WebSocket design issues a token through a session-scoped endpoint,
although F-004 chat also needs authenticated circle-scoped realtime delivery when
no live session exists. The architecture also uses `session_attendance` for raw
join/leave facts while F-006 owns attendance policy, classification, and manual
overrides. Those responsibilities must not be coupled in F-005.

## Decision

- F-005 provides a generic authenticated realtime ticket endpoint:
  `POST /api/v1/realtime/tickets`. A ticket authorizes all of the caller's current
  eligible circle topics; session topics are added only after an authorized join.
  The WebSocket hub revalidates authorization on connection and subscription. It
  replaces the session-only ticket model.
- F-004 reuses this transport for circle chat topics without depending on an
  active session. F-005 does not implement any chat domain behavior.
- Teachers and supervisors share F-005 moderation rights: start/end, lock/unlock,
  mute-all, mute/unmute an existing audio publisher, and remove. Unmute never
  grants a student audio-publish permission; F-003 exclusively controls that
  temporary turn-based entitlement.
- Sessions retain `actual_start` and `actual_end`. F-005 persists raw live facts
  in `session_participant_presence`: join/leave/reconnect, current presence,
  removal, and standalone hand state.
- F-005 owns creation of the complete base `sessions` table because no earlier
  migration creates it. The table includes circle/creator ownership, lifecycle
  status, nullable `scheduled_at`, actual start/end timestamps, audio-only media
  policy, opaque room reference, lock state, end reason, participant count, and
  audit timestamps. F-003 and F-006 may extend it only through later paired
  migrations for queue and attendance concerns.
- F-006 owns a separate `session_attendance` model for attendance policy,
  classification, and manual overrides. It may consume presence facts but must
  not reinterpret or overwrite them.
- F-005 creates ad-hoc sessions only; F-006 owns scheduled-session creation.
  A room lock blocks new joins but permits an eligible pre-lock participant to
  reconnect while the room remains active. Every active participant may raise or
  lower a hand. Automatic duration-limit and idle-timeout ends record their
  reason and have no human attribution.

## Consequences

- A later F-004 implementation shares one authenticated realtime transport rather
  than creating a second WebSocket authorization mechanism.
- F-005's paired migration creates the complete `sessions` table and
  `session_participant_presence`, with rollback limited to F-005-owned objects.
  Later F-003/F-006 migrations must extend these tables without redefining the
  lifecycle or attendance ownership.
- Canonical OpenAPI and WebSocket contracts must replace the session-only ticket
  path and define topic authorization before F-005 implementation.
- F-006 plans its attendance policy as a consumer of durable presence facts; it
  does not make F-005 label a join as present, late, absent, or excused.

## Alternatives Considered

| Alternative | Reason Rejected |
|---|---|
| Retain a session-only ticket | Forces F-004 to add duplicate or session-dependent realtime authorization. |
| Store live presence in `session_attendance` | Couples F-005 transport facts to F-006 policy and manual override behavior. |
| Make F-003 queue actions invoke moderator mute/unmute | Couples voluntary ordering to exceptional moderation and conflicts with ADR-020. |

## References

- [MVP Decision Register](../../../management/product/MVP_DECISION_REGISTER.md)
- [F-004 and F-005 feature definitions](../../../management/product/FEATURES.md)
- [ADR-008 — WebRTC Solution](ADR-008-webrtc-solution.md)
- [ADR-015 — Session-Media Provider Boundary](ADR-015-session-media-provider-boundary.md)
