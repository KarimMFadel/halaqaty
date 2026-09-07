# Implementation Plan: Real-time Chat

**Branch**: `004-real-time-chat` | **Date**: 2026-09-06 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `specs/004-real-time-chat/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

Deliver one PostgreSQL-authoritative group chat per circle and role-restricted pair DMs with text/media, offline idempotency, search, replies, pins, read/typing state, soft deletion, and at-least-once WebSocket projection. Add one Go chat module and one Flutter chat feature; reuse F-001 authentication/session checks, F-002 memberships, F-005 generic realtime transport, MinIO storage, and existing observability patterns. F-004 has no LiveKit media or F-008 notification responsibility.

## Technical Context

**Language/Version**: Go 1.26 module; Dart 3.x / Flutter 3.44 CI image; SQL for PostgreSQL 16  
**Primary Dependencies**: pgx v5, Firebase Admin SDK (identity only), Gorilla WebSocket, Riverpod, Dio, flutter_secure_storage, approved minio-go/v7, `record ^7.1.1`, `just_audio ^0.10.6`, `image_picker ^1.2.3`, `file_picker ^12.2.0`; LiveKit dependencies remain F-005-only  
**Storage**: PostgreSQL authoritative chat state; private versioned MinIO chat bucket; encrypted device-local pending envelopes  
**Testing**: Go unit/contract/integration/race/coverage; migration tests; Flutter unit/widget/integration/analyze/format; Spectral and gitleaks  
**Target Platform**: Single Docker Compose Linux server; Android/iOS Flutter app
**Project Type**: Mobile application plus modular-monolith REST/WebSocket backend  
**Performance Goals**: p95 ≤2 seconds for newest 100-message page, first-page search, and commit-to-client delivery under 50 concurrent users across 10 circles with 10,000 messages/circle, 100 warm-ups, and 1,000 samples  
**Constraints**: Additive `/api/v1`; PostgreSQL source of truth; Arabic-first RTL; 30 sends/minute per group or pair; 10 staged uploads/hour/user; 4,000-char text; voice 300s/20MB, image 5MB, PDF 10MB; seven-day renewable media URLs; no Redis, LiveKit chat transport, FCM, live-session recording, video, or message E2EE; user-initiated chat voice notes are allowed  
**Scale/Scope**: 50 concurrent chat users across 10 circles; chat tests run independently of live sessions, with a separate coexistence check for up to 10 active session records/topics; one group chat per circle and one logical DM per eligible pair; all F-004 P1/P2 stories are MVP

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

- **Spec-first**: PASS — the approved spec exists and both clarification sessions are mapped below.
- **Approved scope**: PASS — F-004 spec and ADR-021 are approved; reactions, announcements, push, live-session recording, video, and analytics remain excluded, while chat voice notes are explicitly allowed.
- **Stack**: PASS — Go/Flutter/PostgreSQL/Firebase identity/MinIO and the existing WebSocket transport are retained; LiveKit is not used by chat.
- **Authorization**: PASS — every operation rechecks Firebase identity, current backend session, and current per-circle membership/role.
- **Persistence and migration**: PASS IN DESIGN — ADR-021 was accepted on 2026-09-06; the proposed tables, dependencies, reversible migration, and MinIO versioning boundary are approved.
- **Contract-first**: PASS FOR PLANNING — canonical and feature REST/WebSocket contracts are synchronized; both OpenAPI files passed Spectral on 2026-09-06. Implementation contract tests remain T002–T004 gates.
- **Reliability/security**: PASS IN DESIGN — idempotency, transactional outbox, bounded retry/timeouts, redacted audit/telemetry, MIME/size/duration checks, and rate limits are specified.
- **Testing**: PASS IN DESIGN — unit, contract, integration, migration, realtime redelivery, mobile, accessibility, and full repository gates are required.

## Clarification Traceability

| Clarification | Planning decision |
|---|---|
| Current membership-period group history | Filter list/search/reply/media by `messages.sent_at >= circle_members.joined_at`; rejoin creates a new lower bound. |
| DM history after eligibility restoration | One unordered pair history remains stored and becomes visible again only when a qualifying active-circle role pair returns. |
| DM media with multiple qualifying circles | `chat_uploads.authorization_circle_id` is an authorization witness; media belongs to the pair and remains accessible while any qualifying circle exists. |
| F-008 notification ownership | F-004 creates no notification trigger and uses no Firebase Messaging API. |
| Deleted attachment retention | Versioned chat bucket delete markers revoke versionless access immediately while prior bytes may remain indefinitely until a future approved parent-retention feature. |
| Image/PDF limits | Canonical and feature contracts use 5 MB JPEG/PNG and 10 MB PDF limits. |
| DM eligibility | Every DM operation rechecks a current shared active-circle teacher-student or supervisor-student pairing; every other pair is rejected. |
| Voice format | Product stays codec-neutral; planning permits OGG, MPEG, MP4, and WebM while enforcing 300 seconds and 20 MB. |
| Delivery states | Flutter owns pending/sent; durable REST acceptance produces delivered; unique eligible-recipient read facts produce read. |
| Archived circles | Retained members may list, search, and play their membership-period history; every mutation and typing is rejected. |
| Upload target context | New uploads carry exactly one group or DM target and are reauthorized on attachment; legacy unbound uploads cannot attach to F-004. |
| ADR-010 invite amendment | Recorded as an F-002 dependency only; F-004 neither removes invite-code/link sharing nor implements invitation roles. |
| Voice-note recording boundary | Constitution 1.2.0 and ADR-021 allow user-initiated chat voice notes but keep live-session capture/storage disabled. |
| Minimal Flutter media dependencies | Use only `record`, `just_audio`, `image_picker`, and `file_picker`; draw waveform from amplitude samples. |
| REST projection reconciliation | Add pinned retrieval and sender-only read receipts; server delivery states are `delivered/read`; archived mark-read is denied; uploads include `415`. |
| Realtime authorization and DM routing | Reauthorize every client/session immediately before write; target DMs directly to authenticated eligible-user connections. |
| Concurrency and recovery | Serialize pin mutations on the circle row; restore active media by removing only the latest internal delete marker after marker-before-commit crash. |
| Reference load | Fixed 50-user/10-circle/10,000-message fixture with 100 warm-ups and 1,000 measured samples. |

## Existing Baseline

- Reuse F-001 auth/session middleware, response envelopes, request IDs, rate limits, centralized HTTP constants, and Firebase identity verification.
- Reuse F-002 `circle_members`; F-004 does not implement the ADR-010 invitation amendment.
- Reuse F-005 `POST /api/v1/realtime/tickets`, `circle.{uuid}` topics, hub heartbeats, connection limit, and serialized writers; do not import LiveKit types.
- Follow the F-003 transactional-outbox algorithm without sharing its domain-owned table or package.
- The canonical contract contains partial legacy chat/upload shapes; Phase 0 replaces ambiguity only through additive fields/operations and retains old fields.

## Phase 0 — Governance, contracts, and research gate

1. Preserve the accepted ADR-021 chat schema/media/dependency/audit decision and Constitution 1.2.0 voice-note distinction.
2. Keep the Q4 ownership correction synchronized across the F-004 spec, `FEATURES.md`, Arabic business mirror, and decision register.
3. Synchronize canonical `docs/contracts/openapi.yaml` with `contracts/chat.openapi.yaml`; preserve compatible paths/fields, add pinned retrieval and sender-only read receipts, deny archived mark-read, restrict server delivery state to `delivered/read`, include `415`, and reject legacy unbound upload keys only when attaching them to F-004 chat.
4. Synchronize canonical `docs/contracts/ws_events.md` with `contracts/chat.ws_events.md`; remove the stale FCM claim and define group/DM audience, commands, deletion, typing expiry, event IDs, and reconciliation.
5. Update `ARCHITECTURE.md` with ADR-021's authoritative schema and endpoint inventory. Run docs-guard, duplicate-key/ref checks, and `make api-lint`.

## Phase 1 — PostgreSQL and Go chat domain

1. Add paired `000018_real_time_chat` migration with the five chat-owned tables, exact-one-context constraints, idempotency uniqueness, membership/search indexes, circle-row serialization support for the five-pin invariant, and reversible cleanup.
2. Add `backend/internal/chat` domain types, validation, package-level queries, repository transactions, service authorization, and stable error codes. Keep handlers free of SQL. Define route strings and router wiring in `backend/internal/api/routes.go` and `backend/internal/api/router.go`; export/alias only composition-facing constants in `backend/cmd/api/routes.go` when `cmd/api` consumes them.
3. Implement list/send/delete/pin/pinned-list/read/read-detail/search/DM/media-link operations contract-first. Query current membership and role on every protected operation; return standard non-enumerating 401/403/404/409/415/422/429 responses where applicable.
4. Add `chat_uploads` staging and a narrow MinIO store using route-specific 21 MB request-body allowance, 60-second timeout, magic-byte/parse validation, sanitized metadata, private keys, versionless presigning, delete markers, and 24-hour unattached cleanup. Preserve the global 1 MiB default for non-upload routes. Marker failure rejects deletion; reconciliation removes only the latest internal marker when the database message stayed active, or creates a marker when a deleted message lacks one.
5. Add redacted operational audit events and transactional `message_moderation_audits` for effective teacher deletions only.

## Phase 2 — Realtime and reliability

1. Add chat event constants and a chat command handler to the existing hub without changing F-005 session-topic semantics. Group events use circle topics; DM events use direct authenticated-user delivery and never choose a qualifying circle topic.
2. Insert outbox rows atomically with message/read/delete mutations. Rebuild payloads from PostgreSQL and revalidate each target user/session immediately before socket write; revoked sessions and removed/newly unauthorized group or DM recipients receive nothing.
3. Run a bounded dispatcher with startup replay, `SKIP LOCKED`, five attempts, 1/2/4/8-second jittered delays (hard cap 30 seconds), parked rows, backlog/age/parked metrics, and redacted structured logs.
4. Treat WebSocket delivery as optional projection: REST success is based only on committed PostgreSQL state, and reconnect always reconciles through paginated REST history.

## Phase 3 — Flutter chat feature

1. Add feature-local models/API/realtime clients and Riverpod controllers. Reuse the existing authenticated Dio and realtime ticket/session plumbing rather than adding another socket.
2. Persist pending envelopes one item per `flutter_secure_storage` key with stable idempotency keys and local attachment paths. Retry network/timeout/429/5xx at 1/2/4 seconds; keep terminal errors visible for edit/discard and reload pending items after restart.
3. Implement Arabic-first group and eligible-DM screens, reverse chronological pagination, replies, search, pinned-list bar, sender-only read details, five-second typing expiry, `record` amplitude-drawn waveform, `just_audio` foreground preview/playback, native image/PDF picking, media renewal, and non-color-only status/error semantics.
4. Keep chat usable inside and outside the session-room shell with identical state. Add no LiveKit or Firebase Messaging import to the chat feature.

## Phase 4 — Verification and review

1. Cover authorization matrices, membership periods, restored DM history, multi-circle DM media, limits, idempotency, concurrency, deletion/audit, delete-marker revocation, search normalization, outbox retry/redelivery, reconnect reconciliation, archived behavior, and F-008 absence.
2. Run migration fresh/upgrade/down/up tests; fixed-fixture performance tests; Go unit/contract/integration/race/coverage/lint/fmt; Flutter unit/widget/integration/analyze/format; Spectral; gitleaks; and canonical/feature-contract parity checks.
3. Apply docs-guard, then Tech Lead review. Karim performs mandatory manual deep review of RBAC, deletion, MinIO/upload, and Firebase authentication boundaries; review evidence belongs in the PR/review record, not the requirements-quality checklist.

## Project Structure

### Documentation (this feature)

```text
specs/004-real-time-chat/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)
```text
backend/
├── cmd/api/                         # centralized routes and composition
├── internal/chat/                   # domain, validation, SQL, service, handlers, outbox
├── internal/platform/{httpconst,logging,metrics}/
├── migrations/000018_real_time_chat.{up,down}.sql
└── tests/{contract,integration,performance}/

mobile/
├── lib/features/chat/{domain,data,application,presentation}/
├── test/features/chat/
├── test/widget/chat/
└── integration_test/chat_flow_test.dart

docs/
├── contracts/{openapi.yaml,ws_events.md}
└── engineering/architecture/{ARCHITECTURE.md,adr/ADR-021-chat-persistence-media-and-delivery.md}
```

**Structure Decision**: Extend the existing two-stack monorepo with one backend chat package and one Flutter chat feature. Cross-cutting changes are limited to route composition, shared realtime extension points, metrics/logging, canonical contracts, and the one approved migration.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Approved `chat_uploads`, `chat_event_outbox`, and `message_moderation_audits` support tables | Required for upload binding, commit-to-WebSocket recovery, and durable teacher deletion audit | Object-key inference, best-effort broadcast, and logs-only audit each fail explicit security/reliability requirements. |
| Approved `minio-go/v7` dependency | No S3-compatible object client exists in the backend | Hand-written Signature V4 and object/versioning calls add security-sensitive code and maintenance. |

## Generated Artifacts

- [research.md](./research.md)
- [data-model.md](./data-model.md)
- [contracts/chat.openapi.yaml](./contracts/chat.openapi.yaml)
- [contracts/chat.ws_events.md](./contracts/chat.ws_events.md)
- [quickstart.md](./quickstart.md)

## Post-design Constitution Check

**PASS IN DESIGN.** All clarification points are mapped, ADR-021 and Constitution 1.2.0 are accepted, and the design stays within the approved stack and MVP scale. Implementation remains gated on canonical contract validation and a clean `/speckit.analyze`; no unresolved clarification remains.
