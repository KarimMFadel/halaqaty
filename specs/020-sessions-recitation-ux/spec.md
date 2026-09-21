# Feature Specification: Sessions and Recitation UX Modernization

**Feature Branch**: `020-sessions-recitation-ux`  
**Created**: 2026-09-22  
**Status**: Draft  
**Input**: Modernize the existing mobile sessions and recitation-queue journey under the approved Halaqaty UI/UX governance without changing backend behavior or contracts.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Reach and enter the relevant session (Priority: P1)

As a student, teacher, or supervisor, I can identify the relevant scheduled or
active session from the app's existing Home or circle journey and enter the
correct start or join flow without needing a session identifier or technical
knowledge.

**Why this priority**: A polished live room is not useful when people cannot
find it quickly. Session discovery and entry are the first steps in every live
recitation journey.

**Independent Test**: Provide scheduled, active, ended, empty, loading, failure,
and offline session states to each supported role and verify that the user sees
one clear primary action, safe human-facing status text, and the correct room
entry outcome.

**Acceptance Scenarios**:

1. **Given** an active session in one of the user's circles, **When** the user
   opens Home, **Then** the active session is prioritized over later scheduled
   sessions and exposes one role-appropriate primary action.
2. **Given** a scheduled session and an authorized teacher, **When** the teacher
   opens its entry surface, **Then** the primary action is to start the session
   and the action remains distinct from joining an already-active session.
3. **Given** an active session and an active circle member, **When** the member
   chooses Join, **Then** the live-room connection journey begins and the UI
   communicates progress without exposing identifiers, credentials, or provider
   terminology.
4. **Given** no eligible scheduled or active session, **When** the user views
   Home or the circle's session area, **Then** the empty state explains what is
   absent and offers only actions already permitted by the user's role.
5. **Given** a teacher or supervisor with the existing permission to create an
   ad-hoc session, **When** no reusable session exists, **Then** the circle
   journey exposes a clear creation action using the already-approved session
   behavior.

---

### User Story 2 - Understand my recitation turn (Priority: P1)

As a student in a live session, I can immediately understand whether I am
waiting, next, reciting, completed, skipped, or opted out, so I know what to do
without reading the full manager queue.

**Why this priority**: The student's primary recitation path is the core purpose
of this wave and must remain calm and legible while live state changes.

**Independent Test**: Render each allowed student queue state, including missed
events and reconnect recovery, and verify the current reciter, the student's
position/status, the next useful action, and privacy-safe peer context.

**Acceptance Scenarios**:

1. **Given** an active round and a waiting student, **When** the queue is
   displayed, **Then** the student sees their current position, the active
   reciter, and what happens next without manager-only controls.
2. **Given** the student becomes the selected or current reciter, **When** the
   authoritative queue update arrives, **Then** the change receives prominent
   text, icon, and semantic treatment rather than color-only emphasis.
3. **Given** the student cannot recite, **When** opt-out is available, **Then**
   the request action and pending, approved, declined, or automatic outcome are
   explained in human-facing language without inventing a new queue state.
4. **Given** duplicated, delayed, or missed live updates, **When** the client
   reconciles with the authoritative state, **Then** the UI shows a recoverable
   reconnecting state and converges without duplicate students or contradictory
   turn messaging.
5. **Given** the round or session ends, **When** the student remains on the room
   screen, **Then** the prior live controls become non-actionable and the final
   state provides a clear exit path.

---

### User Story 3 - Run the queue with focused controls (Priority: P1)

As a teacher or supervisor, I can prepare and operate the recitation queue with
a clear action hierarchy, so live teaching decisions are fast while destructive
or consequential actions remain deliberate.

**Why this priority**: Managers need the existing F-003 operations during a
time-sensitive live session, but the current surface must not present every
operation with equal visual weight.

**Independent Test**: Exercise preparation, selection, start, completion,
grading, correction, skip, move, reset, opt-out decisions, room moderation, and
session end for each permitted role; verify visibility, confirmation, recovery,
and authoritative state after each action.

**Acceptance Scenarios**:

1. **Given** no active round, **When** an authorized manager opens the queue,
   **Then** Prepare round is the dominant queue action and validation guidance
   is understandable in Arabic and English.
2. **Given** waiting entries, **When** the manager operates the queue, **Then**
   advancing/selecting, starting, completing/grading, and moving or skipping
   are visually grouped according to the current state and unavailable actions
   are disabled or hidden with an accessible explanation.
3. **Given** a consequential action such as reset, skip, participant removal,
   or session end, **When** the manager invokes it, **Then** confirmation text
   names the outcome and the UI does not depend on a transient snackbar alone.
4. **Given** an operation is rejected or the connection degrades, **When** the
   manager remains authorized, **Then** the room retains the last safe context,
   presents a recoverable action, and never shows raw exceptions.
5. **Given** a role without queue-management authority, **When** that person
   enters the same session, **Then** manager controls are absent and cannot be
   inferred from disabled visual placeholders.

---

### User Story 4 - Use the live room accessibly in both directions (Priority: P2)

As an Arabic- or English-speaking participant using touch, large text, or
assistive technology, I can understand and operate the session room without
clipped Quranic text, ambiguous direction, inaccessible controls, or layout
overflow.

**Why this priority**: Accessibility and RTL are part of correctness for a
Quran-learning product, not post-release polish.

**Independent Test**: Review the complete entry and live-room journey in Arabic
RTL and English LTR at supported phone widths and 200% text scale, using semantic
inspection and screenshots for every material state.

**Acceptance Scenarios**:

1. **Given** Arabic RTL, **When** session and queue screens render, **Then**
   direction-sensitive icons, ordering, dialogs, and progress treatment mirror
   correctly while Quran references and diacritics remain unclipped.
2. **Given** English LTR, **When** the same state renders, **Then** content and
   directional affordances mirror without creating a separate interaction model.
3. **Given** a screen reader, **When** focus reaches a live state or action,
   **Then** the control exposes a meaningful label, role, enabled/selected state,
   and important state changes are announced appropriately.
4. **Given** 200% text scale on a supported phone width, **When** long Arabic or
   English names and queue labels render, **Then** primary content remains
   readable and actions remain reachable without overflow.

### Edge Cases

- Multiple circles have active or scheduled sessions at the same time.
- A session becomes active, locked, full, ended, or unavailable while its entry
  card is visible.
- The current user's membership or role changes while the room is open.
- A participant is removed, authentication expires, or access is revoked during
  reconnect.
- The queue has no round, no waiting students, one student, long participant
  names, a selected entry but no reciter, or a reciter while other actions fail.
- The manager retries an action whose durable result already succeeded.
- The device goes offline before entry, during connection, or after the last
  authoritative queue state was received.
- Arabic and Latin names appear together, including Arabic diacritics and long
  fixture-like values that must never become developer-facing headings.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The mobile experience MUST surface the most relevant eligible
  session using this priority: active sessions first, then the nearest scheduled
  session; ties MUST use a stable user-understandable order.
- **FR-002**: The session entry surface MUST present exactly one dominant
  role- and state-appropriate action: Start for an authorized manager of a
  startable session, Join for an eligible member of an active session, or a
  clear non-actionable explanation for ended, locked, full, or unavailable
  states.
- **FR-003**: An eligible user MUST be able to reach an active session in no more
  than two taps from Home and no more than three taps from the app shell through
  the circle journey.
- **FR-004**: Authorized teachers and supervisors MUST be able to reach the
  existing create/start journey when no suitable reusable session exists,
  without introducing scheduling or recurrence behavior.
- **FR-005**: Session discovery and entry MUST define loading, empty, error,
  success, and offline/degraded states with safe localized copy and an
  appropriate retry, fallback, or exit path.
- **FR-006**: The connected room MUST establish a clear hierarchy in this order:
  connection/session status, current recitation focus, the current user's next
  action or state, queue context, participant context, and secondary room
  controls appropriate to the role.
- **FR-007**: A student MUST see their authoritative queue status and position,
  the current reciter when visible under existing rules, and only the opt-out or
  room actions permitted by existing behavior.
- **FR-008**: A student's selected, reciting, completed, skipped, opted-out,
  pending opt-out, approved, declined, and auto-approved outcomes MUST use
  localized text and a non-color cue; no new domain state may be introduced.
- **FR-009**: A teacher or supervisor MUST see queue actions grouped by the
  authoritative queue lifecycle and selected/current entry, with one dominant
  next action and subordinate management actions.
- **FR-010**: Existing preparation, move, advance/select, start, skip,
  complete/grade, correction, reset, opt-out decision, lock, mute, remove, and
  end operations MUST preserve their approved authorization and domain
  semantics; this feature changes presentation and navigation only.
- **FR-011**: Consequential actions MUST use contextual confirmation and visible
  completion or authoritative-state feedback; a snackbar alone MUST NOT be the
  only confirmation.
- **FR-012**: Recoverable connection or queue failures MUST retain safe prior
  context when available, distinguish retryable recovery from terminal access
  loss, and provide a retry or exit path without exposing raw errors.
- **FR-013**: Reconnecting and offline/degraded states MUST state what remains
  visible and which actions are temporarily unavailable. Terminal removal,
  ended-session, membership-loss, and authentication-loss states MUST NOT offer
  indefinite retry.
- **FR-014**: Every interactive control MUST expose a meaningful semantic label
  and state, provide at least a 48dp touch target, and announce important live
  status changes appropriately.
- **FR-015**: Arabic MUST be designed RTL-first and English MUST mirror it in
  LTR. Directional icons, content order, dialogs, progress, and gestures MUST be
  verified in both directions.
- **FR-016**: Session and queue content MUST remain usable at supported phone
  widths and 200% text scale. Arabic diacritics, Quran references, participant
  names, status copy, and primary actions MUST NOT clip or become unreachable.
- **FR-017**: Visual treatment MUST follow the approved Calm Contemporary
  governance and existing design tokens: warm ivory surfaces, restrained
  emerald primary actions, soft-gold low-area emphasis, non-color state cues,
  and no screen-level arbitrary colors, fonts, spacing, or radii.
- **FR-018**: Screens MUST use human-facing Arabic and English copy and MUST NOT
  display raw identifiers, fixture names, provider terminology, credentials,
  internal state names, or exception text.
- **FR-019**: RTL and LTR screenshot evidence MUST cover session entry, student
  waiting/reciting states, manager queue operation, recoverable failure, offline
  or reconnecting behavior, and terminal session state.
- **FR-020**: The feature MUST reuse existing F-003 and F-005 contracts,
  authorization, queue states, session states, and media boundaries. It MUST NOT
  add or change backend endpoints, database schema, roles, media capabilities,
  event semantics, or provider selection.

### Key Entities

- **Session**: Existing scheduled, active, or ended live-circle session surfaced
  for discovery and entry; no new fields or lifecycle states are introduced.
- **Recitation Round**: Existing authoritative round context that determines
  student and manager queue presentation.
- **Queue Entry**: Existing participant turn, position, lifecycle state, and
  permitted grade/notes projection; no new queue state is introduced.
- **Participant**: Existing authorized live-room participant with role, presence,
  hand state, and moderation visibility defined by F-005.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In usability verification, every eligible participant can enter an
  active session in no more than two taps from Home or three taps from the app
  shell through Circles.
- **SC-002**: In role-based tests, 100% of student, teacher, and supervisor
  session-entry states expose exactly one correct dominant action or one clear
  non-actionable explanation.
- **SC-003**: In state-matrix tests, 100% of affected screens cover each
  applicable loading, empty, error, success, and offline/degraded state with a
  recovery or exit path.
- **SC-004**: In queue-state tests, the student can identify their status,
  position, current reciter, and next available action within five seconds for
  every supported state.
- **SC-005**: In manager-flow tests, the next valid queue action is visually
  dominant and every unavailable or consequential action has the required
  explanation or confirmation.
- **SC-006**: All affected screens pass RTL and LTR review at supported phone
  widths and 200% text scale with no clipped Arabic diacritics, inaccessible
  primary action, or layout overflow.
- **SC-007**: 100% of interactive controls in the affected journey have
  meaningful semantics and meet the 48dp touch-target floor.
- **SC-008**: Screenshot review finds no raw identifier, fixture name, provider
  term, credential, internal state value, or raw exception on any affected
  screen.
- **SC-009**: Contract and regression verification confirms zero changes to
  existing backend endpoints, persistence, roles, media behavior, queue state
  semantics, or session lifecycle behavior.

## Assumptions

- F-003 Recitation Queue and F-005 Live Sessions remain approved, implemented
  dependencies and are the authority for domain behavior and permissions.
- Active sessions are more urgent than scheduled sessions; among scheduled
  sessions, the nearest eligible session is the most relevant default.
- "Upcoming session" means an existing eligible scheduled or active session; it
  does not authorize F-006 recurrence, calendar, reminders, or attendance policy.
- Existing session creation supports the ad-hoc/general case needed by this
  wave; no scheduling fields or new lifecycle behavior are required.
- Home and circle-detail entry points may reuse existing session information and
  operations but must not make the Home screen an analytics dashboard.
- Chat remains a separate app-shell destination and Wave 3 concern; the existing
  room-to-chat link may remain available but is not redesigned here.
- All visual and workflow decisions are governed by
  `docs/engineering/design/UI_UX_GOVERNANCE.md` and token values remain governed
  by `docs/engineering/design/DESIGN.md`.

## Out of Scope

- Backend endpoints, database migrations, API or WebSocket contract changes.
- New session, queue, grade, role, participant, media, or authorization states.
- Recurring scheduling, calendar management, reminders, attendance policy, or
  session-history/progress dashboards.
- Chat redesign, attachments, voice notes, or direct messaging.
- Video, recording, screen sharing, transcription, AI assessment, timers, or
  media-provider controls.
- New Flutter dependencies, a replacement navigation shell, a runtime theme
  system, or speculative shared components used by only one screen.
