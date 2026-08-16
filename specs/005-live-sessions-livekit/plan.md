# Implementation Plan: Live Sessions (LiveKit)

**Branch**: `005-live-sessions-livekit` | **Date**: 2026-08-16 | **Spec**: [spec.md](./spec.md)

## Summary

Deliver F-005 as an audio-only, circle-scoped live-session foundation: ad-hoc lifecycle, participant presence and hand state, generic authenticated realtime transport, and one directly injected self-hosted LiveKit adapter. Reuse F-001 dual credentials and F-002 circle roles. F-003 owns turn-based student publishing; F-004 consumes the shared realtime transport; F-006 consumes presence facts for attendance policy.

## Technical Context

**Languages**: Go 1.22, Dart/Flutter 3.x, PostgreSQL 16  
**Dependencies**: Echo, pgx, Firebase Admin SDK, LiveKit server SDK, Riverpod, Dio, `livekit_client`  
**Storage**: PostgreSQL session/presence/audit state; LiveKit is media transport, never persistence  
**Testing**: Go unit/contract/integration; Flutter widget/integration; migration and LiveKit-adapter tests  
**Scale**: 50 concurrent users, at most 10 simultaneous rooms, 50 participants per room  
**Constraints**: API-contract-first; additive v1 changes; Arabic-first RTL; backend-only media credentials; no video, recording, screenshare, queue, chat, schedule, or attendance policy

## Constitution Check

- **Spec-first**: PASS — F-005 spec and requirements checklist are clarified and approved for planning.
- **Stack and media boundary**: PASS — Go, Flutter, PostgreSQL, Firebase, self-hosted LiveKit, ADR-015, and ADR-016 are preserved.
- **Security**: PASS — dual credentials, per-circle roles, backend-only short-lived media credentials, rate limits, audit logging, and no sensitive logging are designed.
- **Reliability**: PASS — transactions, compare-and-set lifecycle changes, retries, timeouts, reconciliation, idempotency, and observability are designed.
- **Contract-first**: CONDITIONAL — canonical OpenAPI and WebSocket contracts require the F-005 surface in Phase 0 before implementation.

## Existing Baseline

- Reuse F-001 auth/session middleware, rate limiting, request IDs, audit logging, response envelope, and centralized HTTP constants.
- Reuse F-002 `circle_members` as the sole per-circle authorization source, including teacher and supervisor roles.
- Add a `backend/internal/sessions` package; only `backend/internal/sessions/livekit/` imports LiveKit server/JWT/room-service/webhook types.
- Add `mobile/lib/features/sessions/`; only `data/livekit_media_session.dart` imports `livekit_client`.
- Keep WebSocket transport generic and topic-authorized. Do not add a queue/chat domain package to F-005.

## Phase 0 — Canonical alignment gate

1. Synchronize `docs/contracts/openapi.yaml` with [`contracts/live-sessions.openapi.yaml`](./contracts/live-sessions.openapi.yaml): ad-hoc create, start/join/end, moderation, presence snapshot, realtime tickets, and the signed LiveKit webhook callback.
2. Synchronize `docs/contracts/ws_events.md` with [`contracts/live-sessions.ws_events.md`](./contracts/live-sessions.ws_events.md): session topic authorization, snapshots, hand raise/lower, lock/remove/mute lifecycle events, end reasons, at-least-once delivery, and reconnect behavior.
3. Reconcile current canonical wording that says teacher-only start/end with OQ-037’s teacher/supervisor moderation decision, without changing F-001/F-002 role ownership.
4. Run the docs-guard checklist manually (the callable guard is unavailable) and `make api-lint`; resolve any contract mismatch before migrations or code.

## Phase 1 — Persistence and session domain

1. Create the complete F-005-owned `sessions` table in the next paired migration, including lifecycle, ad-hoc scheduling boundary, media policy, opaque room reference, lock, end reason, participant count, timestamps, and constraints; this is not an alteration of an existing sessions migration because none exists in the current repository.
2. Create `session_participant_presence` in the same paired migration and provide a rollback that removes only F-005 objects. F-003 and F-006 add later paired migrations for queue and attendance data without recreating the F-005 table.
3. Enforce session state (`scheduled → active → ended`), audio-only mode, unique opaque room reference, capacity 50, and one presence row per `(session_id, user_id)`.
4. Define SQL in package-level `*_queries.go`; use transactions and row locks/CAS for start, join, leave/reconnect, removal, lock, timeout end, and participant count.
5. Persist raw presence/hand facts only. Do not create or populate attendance classification; F-006 owns `session_attendance` policy and overrides.
6. Record structured, redacted audit events for lifecycle and moderation changes; credentials, Firebase tokens, backend session IDs, media room refs, and request bodies are excluded.

## Phase 2 — Media, realtime, and reliability

1. Define typed `SessionMediaGateway` and `ReciterAudioControl` operations for room ensure/close, connection issuance, audio publish entitlement, mute, and remove; no generic publication/capability APIs.
2. Implement the one injected LiveKit adapter with TLS endpoint configuration, Opus ≥48 kbps, disabled noise suppression/AGC/echo cancellation where supported, `CanPublishVideo=false`, recording disabled, and one-hour maximum credential lifetime.
3. Keep a session non-joinable until room ensure succeeds and `active` plus `media_room_ref` is persisted; close orphan rooms on failure. Persist `ended` before close and retry cleanup idempotently.
4. Verify signed LiveKit webhooks in the adapter, translate them to neutral events, and process duplicate presence events idempotently.
5. Build generic realtime tickets, heartbeat, topic subscriptions, authorization revalidation, per-user connection limit, message rate limit, snapshots, and a sessions-owned reconciler for provider/database crash windows.

## Phase 3 — Backend REST API

1. Add centralized route constants and Echo wiring for feature-contract operations; protected routes require Firebase bearer plus current backend session.
2. Authorize teacher/supervisor lifecycle/moderation actions through current `circle_members`; members join/read presence only in their own circle; session topic access starts only after successful join.
3. Validate identifiers, state, capacity, lock/removal, duration, media response completeness, and idempotency keys/request correlation. Return the standard error envelope and documented `400/401/403/404/409/422/429/5xx` semantics.
4. Apply existing per-IP/per-user REST limits, three active WebSocket connections per user, 30 messages/minute/user/circle, operation deadlines, safe retry rules, and request-scoped observability.
5. Preserve existing v1 behavior; add fields/endpoints additively, never expose provider room references or credentials in broadcasts/logs.

## Phase 4 — Flutter session-room shell

1. Add Riverpod models/controllers for session lifecycle, presence snapshot/events, hand state, moderation, realtime state, and `MediaSession` connection state.
2. Add the private LiveKit implementation behind `MediaSession`; retain credentials in memory only, reconnect only while usable, and refresh through authenticated start/join near expiry.
3. Build Arabic-first, RTL-aware session room UI: participant list, audio/reconnect state, hand controls, teacher/supervisor controls, empty states, and terminal error states. Do not add queue or chat panels.
4. Stop automatic reconnect for removal, ended/locked denial, revoked identity, or terminal provider failure; surface a clear recovery action where applicable.

## Phase 5 — Verification and review

1. Unit tests: lifecycle, authorization, lock/reconnect, capacity, duration/idle end, idempotency, credential policy, audio policy, and presence/hand transitions.
2. Contract tests: every F-005 REST operation, error envelope, `Cache-Control: no-store`, no credential/room-reference leakage, topic authorization, and backward compatibility.
3. Integration tests: fresh/upgrade/rollback migration, concurrent start/join/end, duplicate webhooks, provider failures/orphan cleanup, removal/reconnect, rate limits, audit redaction, and reconciliation.
4. Flutter tests: RTL room rendering, controls by role, hand state, presence dedupe, reconnect/expiry refresh, and terminal errors; use the approved Docker integration-test workflow before any Flutter commit.
5. Run clean-code/test/docs guard checklists manually if callable guards remain unavailable, then focused suites and all applicable repository gates. Send one coherent review package to Tech Lead; Karim performs the required manual security review.

## Design Outputs

- [research.md](./research.md) — confirmed technical decisions and alternatives.
- [data-model.md](./data-model.md) — schema, state, invariants, and migration strategy.
- [contracts/live-sessions.openapi.yaml](./contracts/live-sessions.openapi.yaml) — F-005 REST contract slice.
- [contracts/live-sessions.ws_events.md](./contracts/live-sessions.ws_events.md) — F-005 realtime contract slice.
- [quickstart.md](./quickstart.md) — implementation and verification sequence.

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Database/provider partial failure | CAS state transitions, compensating close, reconciler, idempotent webhook handling |
| Credential leakage | Backend-only issue, no-store responses, memory-only Flutter use, redaction contract tests |
| Student publishes outside a turn | Student `CanPublish=false`; only F-003 calls the narrow reciter-audio control |
| Lock/removal reconnect bypass | Durable presence/removal state and authorization revalidation before refresh/subscribe |
| Generic hub leaks session presence | Circle versus joined-session topic split and subscription authorization tests |
| Mobile reconnect loop | Explicit recoverable/terminal state model and bounded retry/refresh policy |

## Post-design Constitution Check

**CONDITIONAL PASS.** Planning respects the constitution and ADRs. Implementation is gated on Phase 0 canonical contract synchronization and successful documentation validation.
