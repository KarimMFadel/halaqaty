# ADR-021: Chat Persistence, Media Revocation, and Delivery Boundaries

**Status:** Accepted  
**Date:** 2026-09-03  
**Accepted:** 2026-09-06  
**Deciders:** Karim; Architect

## Context

F-004 requires durable group and restricted direct messages, offline-safe idempotent sends, membership-period history, reliable WebSocket projection, access-controlled MinIO attachments, and durable teacher-deletion audit facts. The current architecture stores expiring media URLs, has no chat upload binding or chat outbox, and ADR-012 otherwise defers queryable audit storage. Clarification also assigns all background notification behavior to F-008.

## Decision

- PostgreSQL remains authoritative. F-004 adds `messages`, `message_reads`, `chat_uploads`, `chat_event_outbox`, and `message_moderation_audits` in one paired migration; business/history foreign keys use `NO ACTION`.
- A message has exactly one context: `circle_id` for group chat or `dm_recipient_id` for a direct message. Direct conversations are the unordered user pair; no conversation table is added.
- `circle_members.joined_at` defines the current membership-period lower bound. Removal deletes the membership row; reacceptance creates a new row and timestamp.
- MinIO object keys, not presigned URLs, are persisted. `chat_uploads` binds an object to uploader, authorizing circle, and optional DM peer before attachment.
- The private chat bucket has versioning enabled. Attachment soft deletion is fail-closed: it writes a delete marker to the versionless object key before committing the database deletion, so previously issued versionless URLs stop resolving while bytes may remain indefinitely as an older version until a future approved parent-retention feature. Marker failure rejects the deletion. If a crash leaves the database message active, reconciliation removes only the latest matching delete marker using internal version metadata and restores versionless access; if the database row is deleted, reconciliation ensures a marker exists. Version IDs are never exposed to or accepted from clients.
- Use the official `github.com/minio/minio-go/v7` client behind a narrow chat media store; this is the only new backend dependency approved for F-004. LiveKit is not used for stored chat media or chat realtime delivery.
- The constitutional recording prohibition and `FEATURE_RECORDING_ENABLED` apply only to live-session audio/video capture and storage. F-004 chat voice notes are user-initiated message attachments and remain permitted under the fixed 300-second/20 MB, authorization, and retention controls.
- Flutter uses only `record`, `just_audio`, `image_picker`, and `file_picker` for the approved capture, amplitude-driven waveform, preview/playback, image selection, and PDF selection flows. No waveform-extraction, background-audio, or media-service abstraction dependency is added.
- `chat_event_outbox` is inserted in the same transaction as each durable message/read/delete change. A bounded worker projects redacted events through the existing F-005 WebSocket hub with at-least-once delivery, five attempts, exponential backoff capped at 30 seconds, and parked-row metrics.
- F-004 creates no notification trigger and imports no Firebase Messaging API. Firebase remains identity-only here; F-008 later reads approved durable chat interfaces and owns notification preferences and delivery.
- Teacher deletion writes `message_moderation_audits` in the same database transaction. This is a narrow exception to ADR-012 because the F-004 specification explicitly requires a durable audit fact; structured redacted operational logs remain in parallel.

## Consequences

- Immediate logical media revocation is compatible with retained physical bytes and seven-day versionless presigned URLs.
- Four chat-owned support tables are justified by authorization binding, delivery recovery, and mandatory durable auditability; no generic event bus, cache, notification outbox, or conversation abstraction is introduced.
- The next migration is additive and rollback removes only F-004-owned objects. Applied migrations are never edited.
- The architecture gate is satisfied by Karim's 2026-09-06 remediation instruction. Implementation remains blocked until the canonical OpenAPI/WebSocket contracts pass documentation validation and `/speckit.analyze` reports no critical blocker.

## Alternatives Considered

| Alternative | Reason Rejected |
|---|---|
| Persist presigned URLs | Credentials expire, cannot be safely renewed, and are unsuitable as durable state. |
| Proxy every media byte through Go | Makes immediate revocation simple but adds avoidable bandwidth and latency on the single MVP server. |
| Physically purge synchronously on message deletion | Conflicts with the accepted minimal-change retention decision and couples message deletion success to MinIO availability. |
| Add a `direct_conversations` table | One unordered user pair is sufficient; the extra aggregate adds no MVP behavior. |
| Reuse `queue_event_outbox` | It is F-003-owned and keyed to session/round semantics. |
| Operational log only for teacher deletion | Does not satisfy the explicit durable audit-record requirement. |
| Build an F-004 notification trigger | Contradicts the accepted clarification assigning the complete pipeline to F-008. |

## Acceptance Record

Karim accepted this ADR on 2026-09-06 by directing completion of the five `/speckit.analyze` remediation actions, including ADR-021 acceptance, recording terminology, minimal Flutter dependencies, contract reconciliation, and replanning. This acceptance also authorizes the Constitution 1.2.0 clarification; it does not waive contract validation, Spec-Kit analysis, tests, or manual security review.

## References

- [F-004 specification](../../../../specs/004-real-time-chat/spec.md)
- [ADR-006 — Database migrations](ADR-006-db-migrations.md)
- [ADR-012 — Structured audit logs](ADR-012-audit-logging-persistence.md)
- [ADR-016 — Shared realtime transport](ADR-016-session-realtime-and-presence-foundation.md)
- [ADR-019 — No cascade on history](ADR-019-no-cascade-student-history.md)
- [Halaqaty Constitution](../../../../.specify/memory/constitution.md)
