# Spec-Kit Prompt Playbook

Use this file as a reference, not as one giant prompt. Paste only the small section you need.

The durable rules live in:

- `AGENTS.md`
- `.specify/memory/constitution.md`
- `DEVELOPMENT.md`
- `docs/engineering/collaboration/AGENT_WORKFLOW_HARNESS.md`
- current `specs/NNN-feature/{spec.md,plan.md,tasks.md}`

Core rule:

```text
Follow Halaqaty's harness:
Spec-Kit owns scope.
Superpowers owns execution discipline.
Role agents own domain work.
Ponytail owns restraint.

Do not duplicate specs, plans, or task lists outside Spec-Kit.
Do not use Ponytail to skip required security, validation, accessibility, contracts, tests, or acceptance criteria.
Keep responses concise.
```

---

## 0) Session Bootstrap

Paste once at the start of a new OpenCode or GitHub Copilot session.

```text
Work in strict Halaqaty mode.

Before feature work, read:
- AGENTS.md
- .specify/memory/constitution.md
- DEVELOPMENT.md
- docs/engineering/collaboration/AGENT_WORKFLOW_HARNESS.md

Use repo files as authority. Do not rely on pasted memory when a file exists.
Keep output compact:
- Result
- Files
- Next Command

No long reasoning logs. Ask only if a decision changes scope, architecture, contracts, security, or user-visible behavior.
```

---

## 1) New Feature: Specify

Use this when starting a feature through `/speckit.specify`.

```text
Run /speckit.specify.

Feature name: <feature-name>

Business goal:
- <one to three bullets>

Primary users:
- <users>

In scope:
- <must-have behavior>

Out of scope:
- <explicit exclusions>

User stories by priority:
1. P1: As a <user>, I can <goal>, so <value>.
2. P2: As a <user>, I can <goal>, so <value>.
3. P3: As a <user>, I can <goal>, so <value>.

Acceptance scenarios:
- Given <state>, when <action>, then <result>.

Edge cases:
- <edge cases>

Functional requirements:
- FR-001 <requirement>
- FR-002 <requirement>

Key entities:
- <Entity>: <important fields only>

Success criteria:
- SC-001 <measurable outcome>

Assumptions and dependencies:
- <assumptions>

Response contract: concise only, max 8 bullets.
```

---

## 2) Clarify

```text
Run /speckit.clarify.

Rules:
- Ask one consolidated batch of 5-7 questions when clarification is needed.
- Recommend best option first.
- Prefer multiple choice.
- Keep each question concise and decision-oriented.
- Apply accepted answers directly to spec.md.

Final output:
- Questions answered
- Sections updated
- Next command
```

---

## 3) Plan

```text
Run /speckit.plan.

Read the Halaqaty harness and current spec first.

Implementation context:
- Backend: Go
- Mobile: Flutter
- Database: PostgreSQL
- Realtime/media: LiveKit
- Auth/notifications: Firebase

Constraints:
- Keep API contract-first in docs/contracts.
- Preserve backward compatibility for API changes.
- Security baseline: authentication, authorization, validation, rate limits, audit logging.
- Reliability baseline: retries, timeouts, idempotency, observability.
- Resolve mismatches through clarification, ADRs, or replanning before implementation.

Required outputs:
- research.md
- data-model.md
- contracts/*
- quickstart.md

Response contract: concise only, max 8 bullets.
```

---

## 4) Tasks

```text
Run /speckit.tasks.

Task rules:
- Organize by user story priority P1 -> P2 -> P3.
- Format: - [ ] T### [P?] [US#] Description with exact file path.
- Mark [P] only when there is no incomplete dependency and no shared-file conflict.
- Every acceptance criterion must map to implementation and test tasks.
- Add security, contract, migration rollback, RBAC denial, rate-limit, and response-safety tests when relevant.
- Explicitly mark MVP scope.

Output:
- Total tasks
- Tasks per phase/story
- Critical dependency chain
- Next command
```

---

## 5) Analyze

```text
Run /speckit.analyze in read-only mode.

Focus:
- Spec/plan/tasks consistency
- Requirement-to-task coverage gaps
- Constitution violations
- Ambiguities, conflicts, duplication
- Security, reliability, performance, and concurrency coverage
- Agreement between spec, canonical contracts, architecture, migration plan, and implementation plan

Output:
- Compact findings table by severity
- Coverage summary
- Top 5 remediation actions
```

---

## 6) Implement A Phase

Use this for OpenCode implementation.

```text
Run /speckit.implement for phase 3:
- Feature: Recitation Queue System
- Feature path: specs/003-recitation-queue-system
- NNN-feature: 003-recitation-queue-system
- Phase number: 3
- Phase name: Prepare and run a recitation round

Before editing, read:
- AGENTS.md
- .specify/memory/constitution.md
- DEVELOPMENT.md
- docs/engineering/collaboration/AGENT_WORKFLOW_HARNESS.md
- specs/<NNN-feature>/spec.md
- specs/<NNN-feature>/plan.md
- specs/<NNN-feature>/tasks.md
- related contracts under specs/<NNN-feature>/contracts and docs/contracts

Branch rule:
- Check current branch first.
- Do not create a phase-specific branch.

Execution rules:
- Execute only dependency-ready tasks for this phase.
- Respect [P] only when file ownership is disjoint.
- Use one primary domain agent per task.
- Use senior-golang-developer for backend/**.
- Use senior-flutter-mobile-engineer for mobile/**.
- Split cross-stack work into sequential backend/mobile subtasks.
- Mark completed tasks as [X].
- Stop on blocker with a short actionable error.

Superpowers:
- Use TDD for behavior changes and bug fixes.
- Use systematic debugging only after a failure or unexpected behavior.
- Use subagent-driven development only when tasks justify the overhead.
- Use requesting-code-review once per coherent batch.
- Use verification-before-completion with fresh command output.

Ponytail:
- Reuse existing code, stdlib, native platform features, and installed dependencies first.
- Avoid speculative abstractions and unnecessary new dependencies.
- Do not skip validation, authorization, persistence safety, contracts, tests, accessibility, localization/RTL, or acceptance criteria.

Project guards:
- Run $clean-code-guard after production-code changes.
- Run $test-guard after test changes.
- Run $docs-guard after contract, migration, ADR, architecture, or API docs changes.
- If a guard is unavailable, read its SKILL.md and apply the checklist manually.

Output:
- Progress/yes please
- Changed files
- Tests
- Next task
```

---

## 7) Codex Review Prompt

Use this when asking Codex to review OpenCode or GitHub Copilot work.

```text
Review this diff using Halaqaty rules.

Read:
- AGENTS.md
- docs/engineering/collaboration/AGENT_WORKFLOW_HARNESS.md
- relevant specs/<NNN-feature> artifacts
- relevant contracts/docs for touched surfaces

Use normal review priority:
1. Bugs and behavioral regressions
2. Security/auth/RBAC/data-loss risks
3. Missing or weak tests
4. Contract/API/doc mismatches
5. Ponytail review: over-engineering, avoidable abstraction, unnecessary dependencies, missed stdlib/native reuse

Return findings first, ordered by severity, with file/line references.
If no findings, say so and list remaining test gaps or residual risk.
```

---

## 8) Feature Starters

Keep feature-specific data short. Do not paste implementation rules again; point to the templates above.

### Feature 001: Authentication, Roles, and User Profile

```text
Use template "1) New Feature: Specify".

Feature name: Authentication, Roles, and User Profile

Business goals:
- Users register and sign in securely.
- Circle roles enforce per-circle student, teacher, and supervisor permissions.
- Users complete and update a basic profile used by backend and mobile.

Primary users:
- Student
- Teacher

In scope:
- Firebase Auth registration/sign-in in Flutter.
- Backend provisioning after verified Firebase ID token.
- Opaque per-device backend sessions.
- Current-device logout and 30-day inactivity expiry.
- Profile create/read/update endpoints and mobile screens.
- Route protection by auth and role.

Out of scope:
- Social providers beyond configured Firebase providers.
- Full admin dashboard.
- Advanced profile settings unrelated to onboarding.

Key entities:
- User
- Profile
- UserSession
- CircleMember
```

### Feature 002: Circle Management

```text
Use template "1) New Feature: Specify".

Feature name: Circle Management

Feature ID: F-002
Spec-Kit feature directory: specs/002-circle-management
Branch: 002-circle-management

Context:
- Feature 001 (Authentication, Roles, and User Profile) is complete on branch 001-auth-roles-profile.
- Treat the existing Feature 001 implementation, migrations, auth middleware, profile flow, and RBAC patterns as the baseline.
- Do not redesign or duplicate Feature 001. Reuse its authenticated per-device session and per-circle role boundaries.
- This is the Circle Management feature. Do not include F-003 Recitation Queue, scheduling, live sessions, chat, payments, or notifications beyond what is required to complete this feature.

Canonical references to read before writing the spec:
- docs/management/product/FEATURES.md — F-002 Circle Management
- docs/management/product/JOURNEY.md — T-05 Create a Circle, T-06 Invite Students, T-07 Student Joins Circle
- docs/management/product/MVP_DECISION_REGISTER.md — OQ-036 and related circle-role decisions
- docs/engineering/architecture/ARCHITECTURE.md and relevant ADRs
- specs/001-auth-roles-profile/spec.md, plan.md, tasks.md, and contracts/

Business goal:
- Make the circle the first useful organizational unit after authentication.
- Let authenticated users create and manage circles and let students join them safely.

Primary users:
- Student
- Teacher
- Supervisor

In scope:
- An authenticated user creates a circle with name, description, rules, capacity, privacy, language, gender settings, and optional initial teachers/backup supervisor.
- System applies OQ-036 initial memberships and creates a unique 8-character invite code.
- Teacher views, shares, regenerates, and invalidates the invite code/link (`halaqaty.app/join/{code}`).
- Student joins a circle through the invite flow and may belong to at most 5 circles.
- Authorized members can view the circle and member list with per-circle roles.
- Teacher manages supervisor assignment/revocation and preserves the no-teacher/self-lockout safeguards from OQ-036.
- Teacher can archive a circle; permanent circle deletion is prohibited.
- Backend REST API, PostgreSQL migrations with rollback, OpenAPI contract, Flutter screens/state, and focused tests.

Out of scope:
- Recitation queue (F-003), scheduling, live sessions/LiveKit, chat, payments, recommendations, and notifications.
- New global roles or a co-teacher role.
- Changes to Firebase Auth or the Feature 001 session lifecycle.

Key entities:
- Circle
- CircleMember
- InviteCode

Required safety and edge cases:
- Duplicate or invalid invite code; regenerated code invalidates the old code.
- Full circle, archived circle, duplicate membership, and 5-circle membership limit.
- Unauthorized student/non-member access returns the project-standard denial response.
- Any authenticated user may create a circle; role-management permissions follow OQ-036, and no member can self-promote or leave the circle without a valid remaining teacher.
- Delete semantics, cascade/retention behavior, and audit requirements must follow the architecture and ADRs; do not invent them.

Acceptance scenarios:
- An authenticated user creates a circle, receives the OQ-036 initial membership, and receives an invite link.
- Student joins with a valid invite and appears in the member list as a student.
- Invalid, regenerated, expired, full, or archived invite flows fail safely with contract-defined errors.
- Teacher-only circle and role-management actions reject unauthorized callers.
- Teacher archives a circle according to the approved persistence rules; no hard-delete operation exists.

Success criteria:
- Every F-002 acceptance criterion in FEATURES.md maps to a spec requirement and acceptance scenario.
- The generated spec contains no unresolved `[NEEDS CLARIFICATION]` markers after clarification.
- No requirement or schema is invented that conflicts with the constitution, architecture, ADR-010, or Feature 001 contracts.

Response contract: concise only, max 8 bullets. List assumptions and questions separately; ask before resolving any material ambiguity.
```

### Feature 003: Recitation Queue System

```text
Use template "1) New Feature: Specify" and run `/speckit.specify`.

Feature name: Recitation Queue System

Feature ID: F-003
Spec-Kit feature directory: specs/003-recitation-queue-system
Branch: 003-recitation-queue-system

Project context:
- Project phases 1, 2, and 5 are already complete. This prompt starts Feature 003; reuse their delivered authentication, circle-role, and live-session foundations rather than duplicating or redesigning them.

Read first:
- docs/management/product/FEATURES.md — F-003 acceptance criteria
- docs/management/product/JOURNEY.md — T-08 Pre-set Recitation Queue and T-11 Queue Management During Live Session
- docs/management/product/MVP_DECISION_REGISTER.md — OQ-007 through OQ-011, OQ-020, OQ-021, and GRADE-ENUM
- docs/engineering/architecture/ARCHITECTURE.md — queue, progress, session, real-time, and security constraints
- docs/engineering/architecture/adr/ADR-010-circle-role-management.md
- docs/engineering/architecture/adr/ADR-015-session-media-provider-boundary.md
- docs/contracts/openapi.yaml and docs/contracts/ws_events.md
- specs/001-auth-roles-profile/, specs/002-circle-management/, and specs/005-live-sessions-livekit/ artifacts and contracts

Create `specs/003-recitation-queue-system/spec.md` only. Produce a complete, testable feature specification with user stories, functional requirements, safety/reliability requirements, scope boundaries, and measurable acceptance scenarios. Do not create a plan, checklist, task list, production code, migrations, or contract changes in this step.

Specify these requirements:
- Teachers or supervisors run sequential recitation rounds for an active live session. A round has number, type (`new_memorization`, `revision`, `old_revision`, or `test`), Surah, Ayah range, and grading requirement.
- A round creates one durable position per eligible student. Support join order and authorized teacher/supervisor manual ordering; teachers can pre-set the queue before the session. Late joiners append to the active round.
- Authorized managers reorder, move, skip, and advance turns; only one student recites at a time. Student opt-out requires teacher/supervisor approval and is logged without penalty.
- Use only `waiting`, `reciting`, `completed`, `skipped`, and `opted_out` queue states. A reset finalizes the prior round, creates a new one, and preserves history.
- Grades are only `excellent`, `good`, `acceptable`, `needs_review`, and `repeat`; notes are optional and at most 500 characters. Only completed entries create a durable progress/practice record; skipped and opted-out entries do not.
- PostgreSQL is the source of truth. REST/WebSocket/FCM/in-app delivery is at-least-once; clients deduplicate and re-fetch `GET /sessions/{id}/queue` after reconnect.
- Validate authenticated membership, role, Quran range, round type, grade, state transition, and idempotency for duplicate or concurrent requests.
- F-005 owns session lifecycle, room lifecycle, media-credential issuance, and general moderation. F-003 integrates only through the provider-neutral `ReciterAudioControl`: the backend grants audio publishing only to the active reciter and revokes it when the turn ends; video remains disabled.
- Never expose media credentials or room references in queue events, persistence, logs, caches, or URLs.

Preserve scope boundaries: no scheduling, chat, payments, recording, video, AI assessment, timers, dashboards, Firebase/Auth redesign, or rebuilding F-005 session/media behavior. Surface material ambiguity as `[NEEDS CLARIFICATION]`; do not invent endpoints, status codes, schemas, or state transitions.

Response contract: concise only, maximum 8 bullets. List assumptions and questions separately; ask before resolving any material ambiguity or contract conflict.
```

### Feature 005: Live Sessions (LiveKit)

```text
Use template "1) New Feature: Specify".

Feature name: Live Sessions (LiveKit)

Feature ID: F-005
Spec-Kit feature directory: specs/005-live-sessions-livekit
Branch: 005-live-sessions-livekit

Context:
- Features 001 (Authentication, Roles, and User Profile) and 002 (Circle Management) provide authenticated per-device sessions, active circle membership, and per-circle teacher, supervisor, and student roles. Reuse them; do not redesign or duplicate them.
- F-005 is the stable live-session, participant-presence, realtime-session, and LiveKit foundation that F-003 Recitation Queue consumes later. F-005 must be independently usable and testable without F-003.
- F-004 Real-time Chat is an independent circle-scoped feature. F-005 may establish shared generic realtime transport, authentication, heartbeat, and topic authorization, but must not implement chat storage, endpoints, events, attachments, or UI.
- F-006 owns recurrence, calendar, reminders, and attendance policy. F-008 owns general push-notification delivery. Do not pull either feature into F-005.
- Keep queue/chat UI as later composable additions to the session-room shell. Do not create placeholder queue or chat domain behavior.
- ADR-015 defines a targeted compile-time session-media seam: LiveKit is the sole MVP adapter, while session, queue, API/event, and Flutter UI/state code remain provider-neutral. F-005 remains audio-only, while a future approved video feature may extend the same seam. This is not permission to apply Clean/Onion Architecture across the project.

Canonical references to read before writing the spec:
- docs/management/product/FEATURES.md — F-005 Live Sessions acceptance criteria and related F-003/F-004 boundaries
- docs/management/product/JOURNEY.md — T-10 Start a Live Session, T-11 Student Joins Live Session, T-17 End Session, S-03 Join a Live Session, and live-session error states
- docs/management/product/MVP_DECISION_REGISTER.md — OQ-015, OQ-016, and OQ-017
- docs/management/planning/PROJECT_PLAN.md — Month 3 realtime foundation, Month 4 LiveKit, and Month 5 queue dependency ordering
- docs/engineering/architecture/ARCHITECTURE.md — communication protocols, session lifecycle, LiveKit security/integration, sessions and session_attendance schemas, endpoint planning, and deployment constraints
- docs/engineering/architecture/adr/ADR-001-modular-monolith.md
- docs/engineering/architecture/adr/ADR-002-go-framework.md
- docs/engineering/architecture/adr/ADR-003-flutter-state-management.md
- docs/engineering/architecture/adr/ADR-004-auth-boundary.md
- docs/engineering/architecture/adr/ADR-008-webrtc-solution.md
- docs/engineering/architecture/adr/ADR-009-firebase-device-sessions.md
- docs/engineering/architecture/adr/ADR-010-circle-role-management.md
- docs/engineering/architecture/adr/ADR-014-mvp-deployment.md
- docs/engineering/architecture/adr/ADR-015-session-media-provider-boundary.md
- docs/contracts/openapi.yaml — Sessions and realtime-token paths/schemas
- docs/contracts/ws_events.md — session events, hand-raise commands, delivery guarantees, and reconnect behavior
- specs/001-auth-roles-profile/ and specs/002-circle-management/ artifacts and contracts

Business goal:
- Replace external meeting tools with a secure, reliable, Quran-appropriate audio-only live circle experience.
- Give teachers controlled room lifecycle and participant moderation while giving members simple joining, hand raising, and graceful recovery from network loss.
- Establish the stable session and media boundary required by later queue, scheduling, attendance, and progress features.

Primary users:
- Student
- Teacher
- Supervisor

In scope:
- An authorized teacher creates or selects an ad-hoc/general session, starts it, and ends it through the canonical session lifecycle: `scheduled` -> `active` -> `ended`.
- Starting a session creates a unique self-hosted LiveKit-backed media room and returns an opaque participant-specific `MediaConnection` issued only by the Go backend.
- An active circle member joins an active, unlocked session and receives only their own required, short-lived `MediaConnection` after authentication, backend-session validation, circle-membership validation, capacity validation, and room-state validation.
- Persist authoritative participant join, leave, and reconnect facts needed for current presence, participant count, later queue join order, and future attendance policy. Do not implement F-006 attendance evaluation or manual overrides.
- Enforce audio-only MVP behavior. No client or backend path may grant video publishing or expose a video control.
- Configure Quran-recitation audio with Opus at 48 kbps or higher, noise suppression OFF, automatic gain control OFF, and echo cancellation OFF where the platform permits, following Constitution V.
- Teacher controls include mute all, mute an active audio publisher, remove a participant, and lock/unlock the room against new joins. Clarify the canonical individual-unmute behavior before specifying it because students must not gain publish permission outside an F-003 active recitation turn.
- Students can raise and lower a hand; authorized participants see the persisted/current hand state and teacher UI updates independently of any queue implementation.
- Provide a Flutter session-room shell with Arabic-first/RTL-aware participant list, audio connection state, teacher controls, hand-raise controls, clear empty/error states, and graceful LiveKit reconnection.
- Provide authenticated realtime session transport for `session.*` presence, lifecycle, and hand-state events, including heartbeat, reconnect, authorization revalidation, and idempotent duplicate handling. Keep the transport generic enough for later queue/chat consumers without implementing their domains.
- Apply the frozen maximum session duration of 4 hours and an idle room timeout of 30 minutes after the final participant leaves.
- Enforce a maximum of 50 active participants per room.
- Disable recording and screen sharing in MVP; recording remains blocked until a privacy consent and retention framework is approved.
- Backend REST/session services, PostgreSQL migrations with rollback, LiveKit adapter and webhooks, realtime session events, Flutter session state/UI, self-hosted LiveKit development/deployment configuration, observability, and focused backend/mobile/contract/integration tests.

Out of scope:
- Recitation rounds, queue entries, ordering, grading, queue notifications, progress records, or queue-driven active-reciter orchestration (F-003).
- Chat messages, direct messages, typing/read status, uploads, MinIO attachments, or chat UI (F-004).
- Recurring schedules, calendars, reminder configuration, attendance status policy, or manual attendance overrides (F-006).
- General FCM/push-notification infrastructure or unrelated notification workflows (F-008).
- Video, screen sharing, recording, transcription, AI analysis, payments, analytics dashboards, or digital Mushaf/audio playback.
- Firebase Auth redesign, new roles, or changes to the Feature 001 backend-session lifecycle.

Key entities:
- Session
- SessionParticipantPresence (durable join/leave/reconnect facts; reconcile its final schema with the canonical `session_attendance` model)
- CircleMember
- MediaRoom (provider-neutral session media room; LiveKit-backed in MVP)
- MediaConnection (opaque participant endpoint, credential, and expiry returned only to that authenticated participant)
- RealtimeConnectionTicket (short-lived authenticated connection authorization, if approved during clarification)
- HandRaiseState

Required safety, reliability, and edge cases:
- Every protected operation validates both the Firebase identity/current-device backend session and active membership in the session's circle.
- Only authorized teachers may create/start/end sessions, lock/unlock rooms, mute participants, or remove participants. Confirm whether supervisors receive any moderation rights; do not infer them from queue permissions.
- Media credentials are participant-specific, opaque, least-privilege, valid for at most one hour in MVP, and issued only by the Go backend through the LiveKit MVP adapter. Derive `expires_at` from the actual signed credential. Never broadcast, cache, log, persist client-side, place in URLs, or reuse one participant's credential for another participant.
- A student joins listen-only with `CanPublish=false`, `CanPublishVideo=false`, and `CanSubscribe=true`. F-005 must not add a general path that grants student publishing outside the later F-003 active-reciter permission interface.
- The LiveKit adapter's teacher credential and moderation mapping must preserve `CanPublishVideo=false`; no feature flag alone may enable video without an approved post-MVP feature specification and ADR.
- Start, join, end, lock/unlock, remove, presence/webhook processing, and hand-state commands must be idempotent under safe retry and duplicate delivery.
- Repeating start for an already-active session returns `200` with the same `Session` and a newly issued caller-specific `MediaConnection`; it never creates a second room. `409` is reserved for non-startable states such as `ended`.
- Concurrent session start/end, duplicate webhooks, participant reconnect, capacity races, room lock races, stale role/membership, revoked sessions, removed participants, LiveKit API timeouts, and partial database/LiveKit failures must converge safely.
- Joining a scheduled, ended, locked, full, missing, archived-circle, or unauthorized session must fail with the approved contract semantics and must not issue a media credential.
- Ending a session prevents new joins, revokes/disconnects participants, closes the LiveKit room, clears transient presence/hand state as approved, and preserves durable session history.
- Reconnection must distinguish recoverable transport loss from removal, room lock, ended session, revoked authentication, and LiveKit unavailability; never show an infinite spinner loop.
- Flutter may retry provider reconnect only while the current credential is usable. Near or after expiry it repeats the authenticated start/join operation for a fresh `MediaConnection`; terminal authorization/session failures do not reconnect.
- The application must not log LiveKit secrets, participant media credentials, Firebase bearer tokens, backend session IDs, or sensitive request bodies.
- The application must expose useful room/participant/reconnect/error metrics and structured audit events without recording audio.

Mandatory session-media implementation boundary:
- Backend session services own provider-neutral `SessionMediaGateway` types and operations. Only `backend/internal/sessions/livekit/` may import LiveKit SDK, JWT, room-service, track, or webhook types.
- F-003 calls a sessions-owned `ReciterAudioControl`; it never imports LiveKit, the provider gateway, or provider identifiers.
- Flutter session controllers/UI own and consume a provider-neutral `MediaSession`. Only `mobile/lib/features/sessions/data/livekit_media_session.dart` may import `livekit_client`.
- Canonical REST uses required `MediaConnection { endpoint, credential, expires_at }` on every successful start/join response. Public `Session` uses required `media_mode` (`audio_only` in F-005; `audio_video` reserved for a future approved feature). Persistence uses opaque `media_room_ref`; neither room references nor connection credentials appear in public session objects or WebSocket broadcasts.
- `MediaConnection.endpoint` comes from trusted adapter configuration and uses TLS; `credential` is identity-specific, opaque, short-lived, memory-only, and never cached, placed in URLs, logged, or persisted. Start/join responses use `Cache-Control: no-store`; provider failure returns non-2xx rather than a partial/null successful connection.
- Construct and inject the single LiveKit adapter directly for MVP. Do not add `media_provider`, a driver field, provider registry/resolver, selection flag, multiple adapters, or conformance framework while no second provider exists.
- Document the ADR-015 future rollout but do not implement it now: when provider two is approved, add immutable session pinning, a closed resolver/switch, compatible-app gating, new-session-only selection, rollback for new sessions, drain checks, and later adapter removal. Never migrate an active room.
- DB-to-provider actions and signed webhook delivery are not one transaction. Keep the session non-joinable while ensuring its deterministic room, then compare-and-set `active` plus `media_room_ref`; compensate failed activation by closing the orphan. Persist `ended` before closing the room. Plan a sessions-owned reconciler for create/close, permission, presence, duplicate-webhook, and process-crash windows.
- Keep `SessionMediaGateway` operations typed and audio-only for F-005. Do not add video/camera/screen-share methods, generic publication APIs, or capability maps. A future video feature extends or composes the seam only after its own approval.
- Do not generalize this boundary into project-wide Clean/Onion Architecture, database abstraction, dynamic Go plugins, runtime downloads, arbitrary capability maps, or custom WebRTC infrastructure.

Frozen decisions to preserve:
- The frozen maximum session duration is 4 hours with a 30-minute idle-room timeout after the final participant leaves; this is independent of the one-hour renewable media-credential lifetime.
- Hand raise is standalone F-005 raise/lower session state. Queue consumption/composition belongs to F-003, so F-005 remains independently completable.

Material conflicts that `/speckit.clarify` must resolve before planning:
- The realtime token is currently session-scoped while the same hub is planned for circle chat. Clarify a generic authenticated realtime ticket/topic-subscription model that does not make F-004 depend on a live session.
- Canonicalize `actual_start`/`actual_end` versus `started_at`/`ended_at`, room lock state, participant/hand persistence, and missing moderation contracts before generating migrations or code. The terminal session state is `ended`.

Acceptance scenarios:
- A teacher starts an eligible session; exactly one LiveKit-backed media room is created through `SessionMediaGateway`, the session becomes active with `media_mode=audio_only`, and the teacher receives a complete identity-specific `MediaConnection`.
- An authenticated active member joins an unlocked active session, receives only their own complete least-privilege `MediaConnection`, connects as listen-only when they are a student, and appears once in participant presence.
- A teacher mutes active audio, removes a participant, and locks/unlocks the room; unauthorized callers are rejected and removed/locked users cannot bypass enforcement through reconnect or retry.
- A student raises and lowers a hand; connected authorized participants receive idempotent session updates and a reconnecting client recovers the authoritative current state without requiring a queue.
- Network loss triggers the approved reconnect UX and preserves the session safely; an ended session, explicit removal, or revoked identity does not reconnect indefinitely.
- The 51st concurrent participant is rejected safely, a session ends automatically or manually within the approved 4-hour/idle limits, and the room is cleaned up without recording media.
- Attempts to publish video, grant student audio publishing outside the future F-003 permission boundary, forge/reuse a media credential, join across circles, or access an ended/locked room are rejected safely.
- Audio configuration and end-to-end tests verify the Quran-recitation settings, backend-only media credential issuance, no recording/video paths, LiveKit adapter failure handling, and clean rollback of new database objects.

Success criteria:
- Every genuine F-005 acceptance criterion in FEATURES.md maps to a spec requirement and independently testable acceptance scenario; cross-feature queue/chat composition is explicitly assigned to its owning feature rather than left as unfinished F-005 work.
- A teacher and up to 49 other participants can complete the supported audio-only start/join/moderate/reconnect/end flow without F-003 or F-004 being installed.
- 100% of tested video-publish, unauthorized moderation, cross-circle join, credential-reuse, locked-room, ended-room, full-room, and out-of-turn student-publish attempts are rejected without weakening room state.
- Duplicate start/join/end/webhook/hand-state requests produce one durable result and consistent participant state.
- Recoverable network interruptions converge to the authoritative session and participant state without duplicate participants or indefinite loading.
- The generated spec contains no unresolved `[NEEDS CLARIFICATION]` markers after clarification.
- No requirement, endpoint, event, schema, permission, duration rule, or package boundary conflicts with the constitution, architecture, ADRs, canonical contracts, or frozen MVP decisions.
- Contract and dependency tests prove no LiveKit SDK types or `livekit_*` fields escape the two adapters, and no speculative multi-provider machinery is introduced during MVP.

Response contract: concise only, max 8 bullets. List assumptions and questions separately; ask before resolving any material ambiguity or contract conflict.
```
