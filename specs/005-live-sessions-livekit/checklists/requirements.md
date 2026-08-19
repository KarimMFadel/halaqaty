# Requirements Checklist: Live Sessions (LiveKit)

**Purpose**: Validate that the F-005 specification is complete, clear, consistent, and ready for technical planning.  
**Created**: 2026-08-16  
**Feature**: [../spec.md](../spec.md)

## Scope and traceability

- [x] CHK001 Feature identity, branch, status, and source input are recorded.
- [x] CHK002 Scope is limited to audio live sessions, presence, hand state, realtime transport, and the narrow LiveKit boundary.
- [x] CHK003 F-001 identity/current-device sessions and F-002 membership/roles are explicitly reused.
- [x] CHK004 F-003 queue/reciter behavior, F-004 chat, F-006 scheduling/attendance policy, and F-008 push delivery are explicitly excluded or assigned to their owners.
- [x] CHK005 All seven clarification decisions are recorded in the spec and the MVP Decision Register.

## User stories and acceptance scenarios

- [x] CHK006 Every story has priority, rationale, an independent test, and Given/When/Then acceptance scenarios.
- [x] CHK007 Start/join covers one room, a caller-specific connection, student listen-only access, and retry-safe start behavior.
- [x] CHK008 Moderation covers equal teacher/supervisor rights, lock/unlock, mute/unmute without granting student publishing, removal, and end.
- [x] CHK009 Presence and hand state cover all active participants, idempotent delivery, and privacy-limited session subscriptions.
- [x] CHK010 Recovery distinguishes recoverable network loss from removal, session end, revoked identity, and provider failure.
- [x] CHK011 Four-hour and idle-timeout automatic ends have explicit durable reasons and no human attribution.

## Functional requirements and data

- [x] CHK012 Functional requirements are uniquely numbered, contiguous, and testable MUST statements.
- [x] CHK013 Audio-only media, backend-only credential issuance, one-hour maximum credential life, no-store responses, and sensitive-data handling are explicit.
- [x] CHK014 Quran audio fidelity requirements specify Opus 48 kbps or higher and disabled processing where supported.
- [x] CHK015 Capacity, duration, idle-timeout, lock/reconnect, idempotency, and provider-reconciliation behavior are explicit.
- [x] CHK016 `actual_start` / `actual_end`, `end_reason`, `session_participant_presence`, and F-006-owned `session_attendance` responsibilities are explicit.
- [x] CHK017 Generic realtime tickets authorize circle topics; session topics require an authorized join and hub revalidation.
- [x] CHK018 The provider boundary confines LiveKit SDK types to the approved backend and Flutter adapters and prohibits speculative multi-provider machinery.

## Security, reliability, and testability

- [x] CHK019 Per-circle authorization, backend-session validation, credential isolation, and cross-circle denial are explicit.
- [x] CHK020 Student publishing remains exclusively turn-based under F-003; F-005 moderation cannot bypass it.
- [x] CHK021 Edge cases cover capacity races, duplicate delivery, partial provider failures, stale membership, removal, locks, credential expiry, and terminal reconnect states.
- [x] CHK022 Success criteria are measurable and include authorization, idempotency, recovery, and provider-boundary checks.
- [x] CHK023 Required backend, mobile, migration, contract, integration, observability, and dependency testing work is identified without prescribing an implementation.

## Canonical-document alignment

- [x] CHK024 `FEATURES.md`, `JOURNEY.md`, and the MVP Decision Register preserve the approved F-005 boundaries and duration rules.
- [x] CHK025 ADR-015 constrains the LiveKit media seam, and ADR-016 records the realtime, presence, moderation, and automatic-end decisions.
- [x] CHK026 `openapi.yaml` and `ws_events.md` use generic realtime tickets, `actual_start` / `actual_end`, `end_reason`, and joined-participant session topic access.
- [x] CHK027 Architecture documents the dedicated presence model and F-006 ownership of attendance policy.

## Checklist result

**Result**: PASS — the F-005 specification is clarified, internally consistent, and ready for `/speckit.plan`.

**Gate**: Preserve the canonical contract and ADR-016 decisions in the technical plan, migrations, feature-local contracts, and tests.
