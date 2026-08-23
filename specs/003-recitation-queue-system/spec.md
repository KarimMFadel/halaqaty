# Feature Specification: Recitation Queue System

**Feature Branch**: `003-recitation-queue-system`
**Created**: 2026-08-21
**Status**: Approved (2026-08-23)
**Input**: F-003 Recitation Queue System

## Scope

F-003 provides durable, sequential recitation rounds inside an active F-005 live
session. It reuses F-001 authentication/current-device sessions, F-002 active
circle memberships and roles, and F-005 session lifecycle, participant presence,
realtime transport, and media boundary. PostgreSQL is authoritative for rounds,
positions, turn state, grading, and history.

F-003 does not own session or room lifecycle, media-credential issuance, general
moderation, scheduling, chat, payments, recording, video, AI assessment, timers,
dashboards, Firebase/Auth redesign, or any F-005 session/media rebuild. F-003
persists and preserves all prior-round history server-side but exposes no mobile
history UI and no history REST operations beyond the existing current-queue
surface in the MVP; historical-round projections and UI are deferred to F-007.

## Clarifications

### Session 2026-08-23

- Q: How does pre-set ordering interact with late session joins? → A: By default, activate the round with currently present eligible students in their pre-set relative order and append later joiners; managers may instead configure the session to include all active student members from the start.
- Q: What happens to unfinished entries when a round resets or the session ends? → A: By default, mark unfinished entries `skipped`; managers may configure preservation of their last historical states, but F-003 always revokes reciter audio, finalizes without blocking F-005 session end, and never leaves an actionable turn after finalization.
- Q: How does opt-out approval work without adding a queue-entry state? → A: By default, persist a pending request outside the queue-entry state and require manager approval; managers may configure automatic approval for that session.
- Q: Who can see grades and teacher notes? → A: By default, current teachers/supervisors and the graded student; managers may configure managers-only or all-session-participants visibility.
- Q: May a completed grade or note be corrected? → A: By default, authorized managers may correct it with an audit trail and update the same progress record; managers may configure corrections only before round finalization or make completed grades immutable.
- Q: Does advancing a turn itself start recitation and grant audio? → A: No — advance is selection only; becoming `reciting`, including the audio grant via `ReciterAudioControl`, is the separate start transition (I1). (Karim, 2026-08-23)
- Q: Does F-003 expose recitation history to users in the MVP? → A: No — F-003 persists and preserves all prior-round history server-side (FR-006 unchanged) but ships no mobile history UI and no history REST operations beyond the existing current-queue surface; historical-round projections/UI are deferred to F-007 (I3). (Karim, 2026-08-23)
- Q: Does F-003 deliver notifications via FCM? → A: No — F-003 guarantees at-least-once REST/WebSocket/in-app delivery and emits stable, durable event IDs; Firebase device-token and FCM push infrastructure/delivery is owned by F-008, which projects from F-003 events. No FCM work in F-003 (I4). (Karim, 2026-08-23)
- Q: What timeout, retry, and convergence parameters govern queue-event delivery, audio control, and session-end finalization? → A: Documented defaults (the Architect may refine them in the plan phase): outbox/queue-event delivery retries 5 times with exponential backoff, then is parked for operator replay (no silent drop); each `ReciterAudioControl` grant/revoke call has a 5-second timeout that neither blocks nor rolls back the queue-state commit (queue state is authoritative; audio converges); session-end queue finalization retries idempotently until finalized, targeting convergence within 10 seconds of observing session end and never blocking or altering the F-005 session-end result (FR-014 unchanged) (A1). (Karim, 2026-08-23)
- Q: How are SC-007 and SC-008 made measurable? → A: SC-007: convergence deadline of 10 seconds after observing session end, with parked-retry exhaustion as the observable terminal outcome when convergence cannot complete. SC-008: start point = queue-mutation commit in PostgreSQL, end point = event dispatched to connected authorized clients, metric = p95 ≤ 500 ms, sample ≥ 100 committed actions per scenario, standard local-network test environment; disconnected clients are excluded from the latency sample and must recover via the FR-009 re-fetch (A1). (Karim, 2026-08-23)
- Q: What do "move" and full-list "reorder" mean, and which entries may move? → A: "Move" repositions one waiting entry to an arbitrary slot in the durable order; full-list reorder is allowed only before round activation; managers may move waiting entries while another entry is reciting; the `reciting` entry itself may not be moved (A2a). (team default, accepted by Karim, 2026-08-23)
- Q: How does advance behave at its edges? → A: Advance while another entry is already selected replaces the selection without creating a duplicate; advance with zero waiting entries is a clean no-waiting-entry rejection with no state mutation; advance while an entry is reciting is rejected until the turn ends (A2b). (team default, accepted by Karim, 2026-08-23)
- Q: How is a grading-optional round completed, and can a grade be added later? → A: When a round does not require grading, completion without a grade is allowed; a grade or note may be added or changed afterwards only through the existing FR-013 audited correction flow (no new workflow) (A2c). (team default, accepted by Karim, 2026-08-23)
- Q: When do grade/note visibility-policy changes take effect? → A: Immediately and prospectively for new snapshots and events; delivered history is never rewritten; clients re-fetch the current queue state on policy-change events using the existing FR-009 recovery pattern (A2d). (team default, accepted by Karim, 2026-08-23)
- Q: What triggers round activation, and which prepared round activates when several are prepared? → A: Activation is automatic in round-number order: the first prepared round activates when the F-005 session goes live, and each subsequent prepared round activates when the previous round finalizes. No explicit manager activate-round action, endpoint, or UI exists in F-003; managers control rounds via prepare, reset, and the existing operations, and preparation may occur while the session is scheduled or live (B1). (Karim, 2026-08-23)
- Q: Under `present_at_activation`, where are currently present eligible students without a manager pre-set position placed? → A: Students with a manager pre-set position keep their relative pre-set order first; present active student members without a pre-set position are appended after the pre-set students, in join order. The concurrent-join timestamp tie-break is an engineering detail already deferred to plan (B2). (Karim, 2026-08-23)

## User Scenarios & Testing

### User Story 1 - Prepare and run a recitation round (Priority: P1)

As a teacher or supervisor, I can prepare an ordered round and run one student
recitation at a time during an active session, so the circle has a fair,
visible, durable sequence.

**Why this priority**: Turn-based recitation is F-003's core value and requires
the established session, membership, and audio-publishing foundations.

**Independent Test**: Prepare a round for eligible students, start an active
session, advance successive turns, and verify the authoritative queue and
audio-publish entitlement always identify at most one active reciter.

**Acceptance Scenarios**:

1. **Given** an authorized teacher or supervisor and an eligible session, **When**
   they prepare a round using join order or an authorized manual order, **Then**
   the system persists the pre-set ordering and applies the session's queue
   population policy when the round automatically becomes active (FR-001)
   without creating more than one position for any student.
2. **Given** a prepared or active round, **When** an authorized manager reorders
   or moves a waiting student, **Then** the durable positions and the
   authoritative queue view reflect the resulting order without creating a
   second position for that student.
3. **Given** an active round with no current reciter, **When** an authorized
   manager advances a student's turn, **Then** that waiting entry becomes the
   sole selected next entry; advance does not itself make it `reciting` and
   does not grant audio.
4. **Given** a selected waiting entry, **When** an authorized manager starts
   that entry, **Then** it becomes the sole `reciting` entry and F-003 asks the
   sessions-owned `ReciterAudioControl` to grant audio publishing only to that
   student.
5. **Given** a student is reciting, **When** their turn ends, is skipped, or is
   finalized through an approved opt-out, **Then** F-003 asks
   `ReciterAudioControl` to revoke that student's audio-publishing entitlement
   before another entry is started as the active reciter.

---

### User Story 2 - Join late and opt out humanely (Priority: P1)

As a student, I can see my durable position, join an active round fairly when I
arrive late, and request an opt-out under the session policy without being
penalized.

**Why this priority**: Live circles need predictable recovery from late arrival
and temporary inability to recite without losing history or fairness.

**Independent Test**: Under each population and opt-out policy, join a live
session after a round starts, request opt-out, and verify the configured outcome,
no duplicate position, no penalty/progress record, and reconnect recovery.

**Acceptance Scenarios**:

1. **Given** an active round using the default population policy and an eligible
   student who joins the live session late, **When** their join is admitted by
   F-005, **Then** F-003 appends one durable waiting position at the end of that
   active round.
2. **Given** a student's current-round position, **When** the student requests
   opt-out and a teacher or supervisor approves it, **Then** the entry becomes
   `opted_out`, the approval is durably logged, and no penalty or progress/practice
   record is created.
3. **Given** a session using automatic opt-out approval, **When** the student
   requests opt-out, **Then** the entry becomes `opted_out` idempotently without
   a pending approval and no penalty or progress/practice record is created.
4. **Given** a reconnecting authorized session participant, **When** queue events
   were missed, duplicated, or received out of order, **Then** the client
   deduplicates delivery and re-fetches `GET /sessions/{id}/queue` before
   treating its local view as authoritative.

---

### User Story 3 - Grade completed recitations and preserve history (Priority: P1)

As a teacher or supervisor, I can record an allowed grade and optional note for a
completed turn, so teacher-verified practice history is accurate.

**Why this priority**: Completed recitations are the only F-003 source for
durable progress/practice records and must remain trustworthy under retries.

**Independent Test**: Complete a turn with every permitted grade, submit a note
at the boundary, retry the submission, and verify exactly one durable history and
progress/practice record result; skip and approved opt-out must result in none.

**Acceptance Scenarios**:

1. **Given** an entry in a grading-required round, **When** an authorized
   manager completes it, **Then** they must record `excellent`, `good`,
   `acceptable`, `needs_review`, or `repeat` with an optional note of at most
   500 characters in the same atomic completion action; the entry cannot become
   `completed` without that grade.
2. **Given** a completed entry, **When** its completion is committed, **Then**
   exactly one durable progress/practice record is created from that queue entry;
   retry or concurrent duplicate processing does not create another record.
3. **Given** a skipped or opted-out entry, **When** the round is finalized or
   reset, **Then** no durable progress/practice record is created for that entry.
4. **Given** an active round, **When** an authorized manager resets it, **Then**
   the prior round is finalized and retained in history, a new round is created,
   and the prior round is no longer active.
5. **Given** a completed entry, **When** a manager corrects its grade or note
   under the session's correction policy, **Then** the change is audited and the
   same practice record is updated without creating a duplicate.

## Safety, Reliability, and Edge Cases

- Every queue operation MUST verify the authenticated current-device session,
  active circle membership, and actor role. Teachers and supervisors are queue
  managers; session creation is audit attribution and grants no separate queue
  privilege. Students may only perform the configured opt-out-request behavior.
- The system MUST validate the round type, Quran Surah and Ayah range, grade,
  note length, approved state transition, and one-position-per-student
  invariant. Permitted transitions are `waiting → reciting → completed`,
  `waiting|reciting → skipped`, and `waiting|reciting → opted_out` after the
  configured approval outcome; `completed`, `skipped`, and `opted_out` are
  terminal for that round.
- PostgreSQL MUST be the source of truth. F-003's delivery surfaces are REST,
  WebSocket, and in-app, all at-least-once, and every queue event MUST carry a
  stable, durable event ID; duplicate or concurrent requests and deliveries
  MUST converge to one durable outcome. FCM push delivery and Firebase
  device-token infrastructure are owned by F-008, which projects from F-003
  events; F-003 performs no FCM work.
- Queue events and persistence MUST contain no media credential, endpoint,
  room reference, provider-specific identifier, or URL carrying such material.
  Credentials remain caller-private, memory-only F-005 start/join material.
- F-003 MUST call only the sessions-owned provider-neutral
  `ReciterAudioControl`; it MUST NOT import LiveKit or `SessionMediaGateway`.
  Audio publishing is granted only to the active reciter and revoked when their
  turn ends; video remains disabled.
- A reset MUST preserve all prior-round history. It MUST NOT overwrite or reuse
  earlier positions, states, grades, notes, opt-outs, or completed-turn history.
  A later correction allowed by policy updates the current grade/note projection
  only through its explicit audited workflow.
- F-003 MUST NOT reject, delay, or roll back an F-005 session-end transition.
  Session end revokes any reciter audio and makes the active round non-actionable;
  queue finalization is idempotent and retried if it cannot finish immediately.
- Queue-event delivery uses 5 retry attempts with exponential backoff and is
  then parked for operator replay — never silently dropped. Each
  `ReciterAudioControl` grant/revoke call has a 5-second timeout that neither
  blocks nor rolls back the queue-state commit; queue state is authoritative
  and audio converges. Session-end queue finalization retries idempotently
  until finalized, targeting convergence within 10 seconds of observing session
  end. These values are documented defaults; the Architect may refine them in
  the plan phase.
- Session queue policy is configurable only by a current active teacher or
  supervisor. Changes apply prospectively and MUST NOT rewrite durable history,
  weaken authentication/authorization, permit duplicate positions or reciters,
  bypass validation/idempotency, expose media secrets, enable video, or create
  progress from a non-completed entry.
- Invalid, unauthorized, stale, duplicate, or conflicting operations MUST not
  create a second entry, a second active round, more than one reciter, or a
  progress/practice record for a non-completed entry.

## Requirements

### Functional Requirements

- **FR-001**: The system MUST allow a teacher or supervisor to create sequential
  rounds associated with an eligible scheduled or live session, including
  preparation while the session is scheduled or live. Turn execution occurs only
  in an active live session. Round activation is automatic and follows
  round-number order: the first prepared round activates when the F-005 session
  goes live, and each subsequent prepared round activates when the previous
  round finalizes. F-003 provides no explicit manager activate-round action;
  managers control rounds via prepare, reset, and the existing operations.
  Each round MUST have a number, one of `new_memorization`, `revision`,
  `old_revision`, or `test`, a Surah, an Ayah range, and a grading requirement.
- **FR-002**: A round MUST create one durable queue position per eligible
  student selected by the session's population policy. The default
  `present_at_activation` policy includes currently present active student
  members: students with a manager pre-set position keep their relative pre-set
  order first, and present active student members without a pre-set position
  are appended after the pre-set students in join order; `all_active_students`
  includes every active student member from the start. Teachers and supervisors
  are excluded. The round MUST support join order and authorized
  teacher/supervisor manual ordering and allow a teacher to pre-set the queue
  while the session is scheduled or live.
- **FR-003**: Under `present_at_activation`, a student admitted after round
  activation MUST be appended once to the end. Under `all_active_students`, an
  existing pre-set position MUST be retained and a join MUST NOT duplicate it.
- **FR-004**: Authorized managers MUST be able to reorder, move, skip, advance,
  and start turns. "Move" repositions one waiting entry to an arbitrary slot in
  the durable order and is permitted while another entry is `reciting`; the
  `reciting` entry itself may not be moved. A full-list reorder is allowed only
  before round activation. Permitted transitions are
  `waiting → reciting → completed`, `waiting|reciting → skipped`, and
  `waiting|reciting → opted_out` after either required manager approval or
  configured automatic approval. `completed`, `skipped`, and `opted_out` are
  terminal for that round. Advance selects the next waiting entry and does not
  itself start that entry; the separate start transition makes the selected
  entry `reciting` and requests the audio grant through `ReciterAudioControl`.
  Advance while another entry is already selected replaces that selection
  without creating a duplicate; advance with zero waiting entries is rejected
  cleanly as a no-waiting-entry case with no state mutation; advance while an
  entry is `reciting` is rejected until the turn ends. At most one entry MAY be
  `reciting` at any time.
- **FR-005**: A student opt-out MUST require teacher/supervisor approval, be
  durably logged, and create no penalty under the default `approval_required`
  policy. A session MAY use `auto_approve`; the request then transitions the
  entry directly to `opted_out` without creating a pending queue-entry state.
- **FR-006**: Queue entry states MUST be limited to `waiting`, `reciting`,
  `completed`, `skipped`, and `opted_out`. A reset MUST finalize the previous
  round, create a new round, and preserve history; that finalization activates
  the next prepared round under the same automatic rule (FR-001). The default
  `mark_unfinished_skipped` policy transitions unfinished entries to `skipped`;
  `preserve_last_state` retains their states as immutable historical facts.
  Either policy revokes reciter audio and leaves no actionable entry in the
  finalized round.
- **FR-007**: Grades MUST be limited to `excellent`, `good`, `acceptable`,
  `needs_review`, and `repeat`. Grade notes are optional and MUST be at most
  500 characters. When a round requires grading, completion and grading MUST be
  one atomic action; an entry MUST NOT become `completed` without its grade.
  When a round does not require grading, completion without a grade MUST be
  allowed; a grade or note MAY be added or changed afterwards only through the
  FR-013 audited correction flow, with no separate post-completion workflow.
  Grade and note visibility MUST follow the session policy: default
  `managers_and_student`, `managers_only`, or `all_participants`. A grade records
  pedagogical assessment and MUST NOT prevent an authorized manager from
  advancing, resetting, or ending the session.
- **FR-008**: Only a completed queue entry MAY create a durable
  progress/practice record. Skipped and opted-out entries MUST NOT create one.
  A completed `test` round MUST also retain its practice record; the later F-007
  progress model MUST represent that type but exclude it from Quran-map
  memorization and revision status calculation.
- **FR-009**: The system MUST provide the existing authorized queue state as the
  recovery source after reconnect; clients MUST deduplicate at-least-once
  realtime delivery and re-fetch `GET /sessions/{id}/queue`.
- **FR-010**: F-003 MUST integrate with F-005 only through
  `ReciterAudioControl` for turn-based audio publishing. F-005 retains session,
  room, credential, and general-moderation ownership.
- **FR-011**: F-003 MUST NOT add scheduling, chat, payments, recording, video,
  AI assessment, timers, dashboards, a mobile history UI or history REST
  operations beyond the existing current-queue surface (historical-round
  projections/UI are F-007 work), Firebase/Auth redesign, or an alternative
  session/media implementation.
- **FR-012**: Each session MUST have queue-policy defaults for population,
  unfinished-entry finalization, opt-out approval, grade/note visibility, and
  grade/note correction. A current active teacher or supervisor MAY change them
  while the session is scheduled or active; every change is audited. The
  session creator has these
  powers only while they remain a current authorized teacher or supervisor.
  Workflow-policy changes apply only to subsequent actions, while grade/note
  visibility-policy changes apply immediately and prospectively to new
  snapshots and events; already-delivered history is never rewritten, and
  clients re-fetch the current queue state on policy-change events using the
  FR-009 recovery pattern.
- **FR-013**: Grade/note correction MUST follow one of `audited_any_time`
  (default), `before_round_finalization`, or `immutable`. An allowed correction
  MUST update the existing queue entry and its one progress record atomically,
  emit a redacted audit event, and MUST NOT create another progress record.
- **FR-014**: F-005 session end MUST commit and return independently of F-003.
  F-003 MUST revoke any active-reciter entitlement and finalize the active round
  idempotently after observing the end; failure is retried and MUST NOT change
  the already-ended session result. Prepared rounds that never activated MUST
  become permanently inert (retained, never activatable) at session end.
- **FR-015**: F-003 planning MUST reconcile `docs/contracts/openapi.yaml` and
  `docs/contracts/ws_events.md` with the complete queue-control and policy
  behavior before implementation. Clarification MUST NOT invent endpoints,
  status codes, or payload schemas.
- **FR-016**: Mobile queue experiences MUST be Arabic-first and RTL-aware, show
  reconnecting/recovery state, and keep manager reset, skip, policy change, and
  F-005 end controls available when a queue operation or delivery path fails.

### Key Entities

- **RecitationRound**: A session-scoped, sequential unit of recitation with
  round number, type, Quran range, grading requirement, active/finalized status,
  and preserved history.
- **QueueEntry**: One student's durable position in one round, limited state,
  optional grade/note, and durable turn timestamps/history.
- **CompletedTurnPracticeRecord**: The one durable progress/practice record that
  may be produced from one completed queue entry; it is not created for skipped
  or opted-out entries.
- **ReciterAudioControl**: The sessions-owned, provider-neutral F-005 boundary
  F-003 uses to grant or revoke the active reciter's audio publishing.
- **SessionQueuePolicy**: The session-scoped, auditable policy snapshot governing
  queue population, unfinished-entry finalization, opt-out approval, grade/note
  visibility, and correction. It cannot override platform safety invariants.
- **OptOutRequest**: A durable, idempotent student request used only when session
  policy requires approval; it is not a queue-entry state.

## Success Criteria

- **SC-001**: In acceptance and concurrency tests, every active round has at
  most one `reciting` entry and no student has more than one position in that
  round.
- **SC-002**: In duplicate/retry and reconnect tests, the authoritative queue
  converges to its PostgreSQL state with no duplicate positions, completed-turn
  practice records, or duplicate active-reciter audio-publish entitlements.
- **SC-003**: 100% of tested invalid Quran ranges, round types, grades, note
  lengths, unauthenticated callers, non-members, and unauthorized roles are
  rejected without state mutation.
- **SC-004**: 100% of tested skipped and opted-out entries produce no
  progress/practice record; every tested completed entry produces exactly one.
- **SC-005**: Contract and security tests prove queue data, events, logs, cache
  inputs, and URLs contain no media credential or room reference, and no F-003
  code imports provider-specific media types.
- **SC-006**: Every supported session-policy value is covered by acceptance
  tests, and changing a policy never rewrites an earlier queue action or record.
- **SC-007**: In session-end failure tests, F-005 returns the committed ended
  session even when queue finalization initially fails; idempotent retries
  converge to a finalized, non-actionable round with no active-reciter
  entitlement within 10 seconds of observing session end, and parked-retry
  exhaustion is the observable terminal outcome when convergence cannot
  complete (no silent drop).
- **SC-008**: Under the MVP limit of 50 session participants, p95 latency from
  queue-mutation commit in PostgreSQL to dispatch of the corresponding event to
  connected authorized clients is ≤ 500 ms, measured over a sample of at least
  100 committed queue actions per scenario in a standard local-network test
  environment. Disconnected clients are excluded from the latency sample and
  must recover via the FR-009 re-fetch; FCM push is not an F-003 delivery
  channel and is not measured (F-008 owns it).

## Assumptions

- F-001 and F-002 remain the authoritative authentication, current-device
  session, membership, and per-circle role sources; manager means an active
  teacher or supervisor in the session's circle.
- F-005 remains the sole owner of live-session lifecycle, durable presence,
  generic realtime transport, media credentials, and general room moderation.
- The canonical five-grade enum and the existing F-005 audio-only media policy
  remain frozen.
- F-007 may consume F-003's completed-turn records later. It must represent the
  `test` type while excluding it from Quran-map memorization/revision status
  calculation. F-003 does not add progress dashboards, analytics, or a student
  self-logging flow.
