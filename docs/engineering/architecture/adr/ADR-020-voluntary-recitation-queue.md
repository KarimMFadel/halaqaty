# ADR-020: Voluntary Recitation Queue and Open Student Audio

**Status:** Accepted
**Date:** 2026-08-28
**Deciders:** Karim (product owner)

## Context

F-003 previously coupled the displayed recitation queue to student microphone
publishing: one queue entry in `reciting` granted that student audio and every
turn transition revoked it. This does not match the intended halaqa experience.
The live session should behave like a Zoom meeting: students may participate in
audio freely and respect the teacher-managed recitation order themselves.

Teachers and supervisors still need explicit F-005 moderation controls for
exceptional cases, without turning ordinary queue management into automatic
media enforcement.

## Decision

F-003 is a voluntary coordination and progress-tracking queue only.

- Teachers and supervisors manage ordering and set the displayed `reciting`,
  `completed`, `skipped`, and related queue states.
- A displayed `reciting` state records the intended current turn; it never
  grants, revokes, mutes, or otherwise changes a student's audio permission.
- Students may publish audio throughout an authorized live session. Speaking
  out of queue order does not automatically change queue state or invoke
  moderation.
- F-005 retains its explicit moderator controls: teachers and supervisors may
  mute one active publisher, mute all, unmute an already-authorized publisher,
  remove a participant, and lock or end the room.
- F-003 must not call `ReciterAudioControl` or depend on a one-active-reciter
  audio entitlement. Reset, skip, completion, and session-end queue work must
  not wait for or retry queue-driven audio revocation.

The queue's data integrity remains unchanged: one position per student per
round, durable ordering, explicit manager transitions, idempotency, and
PostgreSQL concurrency protection. "At most one `reciting` entry" remains a
displayed queue-state invariant, not a media-publishing restriction.

## Consequences

- F-005 admission must issue student connections with audio-publish permission,
  while video remains disabled.
- Existing queue audio grant/revoke code, private reset revoke intents, and
  their replay worker are removed or replaced when F-003 Spec-Kit artifacts are
  regenerated.
- Queue UI may show current, next, and ordered students without implying that
  speaking is technically blocked outside that order.
- Moderator actions remain independently authorized, explicit, and auditable.

## Alternatives Rejected

| Alternative | Reason Rejected |
|---|---|
| Queue-controlled one-reciter publishing | Turns a voluntary halaqa order into an unnecessary technical restriction and adds fragile queue-to-media recovery. |
| Remove F-005 moderation controls | Leaves teachers and supervisors without an explicit safety tool for exceptional cases. |
| Infer queue status from who is speaking | Cannot reliably represent a voluntary order and would make normal participant audio alter durable progress state. |

## References

- [MVP Decision Register](../../../management/product/MVP_DECISION_REGISTER.md)
- [F-003 Specification](../../../../specs/003-recitation-queue-system/spec.md)
- [ADR-015](ADR-015-session-media-provider-boundary.md)
- [ADR-016](ADR-016-session-realtime-and-presence-foundation.md)
- [ADR-018](ADR-018-configurable-session-queue-policy.md)
