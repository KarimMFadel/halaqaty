# Feature Specification: F-006 Schedule, Calendar & Attendance

**Feature Branch**: `006-schedule-calendar-attendance`

**Created**: 2026-09-26

**Status**: Approved — Karim, 2026-09-27 (including ADR-026's 31-day planned-duration bound)

**Input**: User description: "Create the already registered F-006 Schedule, Calendar & Attendance specification."

## Clarifications

### Session 2026-09-27

- Q: Which branch owns continued F-006 work? → A: Karim closed the shared `019-gap-completion` batch and requested a dedicated F-006 branch from `main`; preserve the existing feature directory and requirement IDs.
- Q: Which recurrence scope is required? → A: Include weekly, biweekly, custom days, optional recurrence end dates, and optional session titles from JOURNEY T-08, subject to architecture approval before implementation.
- Q: Which time restrictions apply? → A: Allow overnight planning with no 90-day scheduling cap. For DST gaps, shift forward by the clock change; for repeated local times, use the first occurrence. Creation is allowed for present/future planned times only, not past times. The earlier same-circle overlap prohibition was superseded by the later manager-warning decision below.
- Q: How do edits and cancellation affect occurrences? → A: Teachers may edit/cancel one unstarted occurrence or change/stop future occurrences; individual edits remain until a later series change supersedes them under the 2026-09-27 clarification below. Preserve all started/completed history. Active sessions use F-005 End. Cancellation is separate from the existing session lifecycle, subject to the required ADR and contracts process.
- Q: How are overlaps handled? → A: The earlier same-circle block was superseded on 2026-09-27: teachers and supervisors may save a same- or cross-circle overlap after a warning. Students see warnings for their own calendar overlaps. Reveal only details the viewer may read and never inspect other members' private commitments.
- Q: How is attendance classified? → A: For planned and ad-hoc sessions, first authorized presence through 10 minutes after actual start is Present; later first presence is Late. No minimum duration. Finalize Absent at session end; Excused is manual only. Reconnects/devices count once. Never-started/cancelled sessions have no classification.
- Q: What are roster, access, and correction rules? → A: Freeze active students at actual start and include newly enrolled students who subsequently participate. Current teachers read/correct all attendance; eligible students read only their own; supervisors may read attendance but cannot correct it. The earlier supervisor-scheduling restriction was superseded by the later scheduling decision below. Corrections retain actor, time, reason, previous/new classification and survive recalculation. Archived records are read-only.
- Q: Can F-006 release before F-008? → A: A clearly labelled partial pilot is allowed, but push acceptance and full feature completion remain pending F-008 approval and verified delivery at 1hr/30min/15min/5min. No placeholder substitutes for push delivery.

- Q: What range does the calendar initially show? → A: Current month, with navigation to any later month; the initial view does not cap scheduling.
- Q: Does custom recurrence support selected weekdays or arbitrary intervals/dates? → A: Both: selected weekdays on weekly/biweekly patterns and arbitrary repeat intervals or explicitly selected dates.
- Q: Which arbitrary repeat intervals are supported? → A: Any positive whole-number count of days or weeks, anchored to the first planned local date.
- Q: How are explicitly selected dates and an optional end date handled? → A: Use ordinary calendar behavior without a separate process: duplicate selected local dates produce one occurrence, past planned dates are rejected under the existing present/future rule, and an end date includes occurrences starting on that local date.
- Q: How should an overnight occurrence crossing DST retain its length? → A: Preserve its planned elapsed duration after resolving the start under the accepted DST gap/repeated-time rule; display the resulting local end time truthfully.
- Q: Does a later series change replace an individual edit to an unstarted occurrence? → A: Yes. The new series values replace that earlier individual edit for affected unstarted occurrences; started/completed history remains intact.
- Q: Where do past completed and cancelled occurrences appear? → A: Retain all dates in the calendar, including completed sessions and cancelled occurrences, subject to the viewer's current access rights.
- Q: Who controls scheduling and may choose to save after an overlap warning? → A: Current circle teachers and supervisors; warnings organize the calendar but do not prohibit scheduling. This extends F-006 scheduling rights beyond the earlier teacher-only draft and requires canonical product/architecture reconciliation.
- Q: May a student read personal attendance history after circle membership is revoked? → A: No. Keep the current circle-role authorization restriction permanently. Current members retain authorized read-only history for archived circles; historical records remain stored after revocation without granting access.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Manage a Circle Schedule (Priority: P1)

As a teacher or supervisor, I can add, view, change, or stop circle schedules using selected weekdays on weekly/biweekly patterns, arbitrary repeat intervals, or explicitly selected dates, with an optional end date, so that members know when sessions take place.

**Why this priority**: A reliable published schedule is the foundation for calendars, planned sessions, attendance, and future reminders.

**Independent Test**: A teacher creates two weekly entries for one circle, a supervisor updates one, and the teacher stops the other; active members see the resulting schedule in their local time.

**Acceptance Scenarios**:

1. **Given** an active teacher or supervisor manages a circle, **When** they create a weekly entry with a day, start time, end time, and time zone, **Then** it is shown as an upcoming recurring session time for that circle.
2. **Given** a circle has multiple active schedule entries, **When** an authorized teacher or supervisor changes or stops one entry, **Then** the other active entries remain unchanged and members see the revised future schedule; a changed series replaces earlier individual edits to its affected unstarted occurrences.
3. **Given** a student is an active circle member, **When** they view the circle schedule, **Then** each occurrence is displayed in the student's local time with a clear association to the circle.
4. **Given** a teacher selects weekdays on a weekly or biweekly pattern, **When** they save the schedule, **Then** occurrences follow those weekdays and cadence, respecting any end date.
5. **Given** a teacher chooses a positive whole-number repeat interval in days or weeks or explicitly selects dates, **When** they save the schedule, **Then** interval occurrences begin on the first planned local date and recur after the chosen count of days or weeks, while selected-date occurrences follow their chosen dates and all use the same timezone, conflict, and history-preservation rules.

---

### User Story 2 - Plan and Find Sessions (Priority: P1)

As a teacher or supervisor, I can create a one-off planned session, and as a member I can see eligible upcoming, completed, and cancelled circle sessions in one calendar, so that special and regular meetings and their history are discoverable.

**Why this priority**: One-off sessions are an approved requirement, and a unified view prevents members of several circles from missing a session.

**Independent Test**: A teacher creates an unlinked one-off session and a recurring schedule exists in another circle; an enrolled student sees both on one calendar with their local times and circle identity.

**Acceptance Scenarios**:

1. **Given** an active teacher or supervisor manages a circle, **When** they create a one-off planned session, **Then** it is independent of recurring entries and is visible to active circle members.
2. **Given** a member belongs to more than one circle, **When** they open their calendar, **Then** it combines their eligible sessions, including past completed and retained cancelled occurrences, and distinguishes the owning circles without relying only on color.
3. **Given** a planned session reaches its start, **When** an authorized teacher starts it, **Then** its lifecycle accurately reflects that it is live; after it ends, it is shown as completed.
4. **Given** a planned session is cancelled, **When** a member views the calendar, **Then** the cancellation is clear and the session cannot be started or recorded as attended.

---

### User Story 3 - Review and Correct Attendance (Priority: P1)

As a teacher, I can review attendance derived from session participation and correct an individual student's classification so that attendance records reflect what happened in class.

**Why this priority**: Attendance must be a durable teaching record, while live participation alone is not an attendance decision.

**Independent Test**: After a session with participation facts for several members, the teacher reviews every member's classified record, changes one classification, and sees the corrected result persist without changing the captured participation facts.

**Acceptance Scenarios**:

1. **Given** a completed circle session, **When** an authorized teacher reviews attendance, **Then** every student in the eligible session roster has a classification, including Absent for non-participants, and the system preserves the underlying participation record.
2. **Given** an authorized teacher reviews a student's attendance record, **When** they set it to present, late, absent, or excused, **Then** the new classification and its manual nature are clearly recorded.
3. **Given** a student or an unrelated user attempts to change another student's attendance, **When** they submit the request, **Then** the record remains unchanged and they receive an understandable permission outcome.

---

### User Story 4 - Avoid Scheduling Surprises (Priority: P2)

As a teacher or supervisor, I receive a clear overlap warning while retaining the decision to schedule, so that I can organize circle commitments.

**Why this priority**: Conflict awareness protects the calendar's usefulness but does not replace the primary scheduling and attendance journey.

**Independent Test**: A teacher or supervisor proposes a time overlapping another circle commitment they may view, receives a warning, and can choose to save the schedule.

**Acceptance Scenarios**:

1. **Given** an authorized teacher or supervisor plans a time overlapping another relevant planned session, **When** they attempt to save it, **Then** a privacy-safe warning identifies the conflict and they may choose to save anyway.
2. **Given** an authorized teacher or supervisor plans a time that does not overlap a relevant commitment, **When** they save it, **Then** the schedule is accepted without a false conflict warning.

### Edge Cases

- A recurring local time crosses a daylight-saving transition; valid local times retain their intended clock time, nonexistent starts shift forward by the clock change, and repeated starts use the first occurrence. Preserve the planned elapsed duration across an overnight DST transition and display the resolved local end time clearly; later occurrences retain the original recurring clock time.
- An overnight occurrence crosses midnight; it is allowed and its end date is explicit rather than rejecting it because the end clock time is earlier than the start clock time. Every occurrence must still identify an end instant after its start.
- Explicitly selected duplicate local dates create one occurrence; a selected past planned date is rejected; an optional local end date includes an occurrence beginning on that date, including one ending the next local day.
- A member loses circle membership after a session was planned; their calendar and attendance access to that circle ends while historical records remain stored. Current members of an archived circle retain authorized read-only history.
- A teacher changes or stops a recurring entry after occurrences have been planned; completed sessions and their attendance records are retained.
- Two update attempts modify the same schedule entry; the user is informed of the current outcome and no change is silently lost.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST allow an active circle teacher or supervisor to create, view, update, and stop multiple schedule entries per circle supporting selected weekdays on weekly/biweekly patterns, any positive whole-number interval of days or weeks anchored to the first planned local date, and explicitly selected dates. Duplicate selected local dates MUST produce one occurrence, and past planned dates MUST be rejected. An optional local recurrence end date MUST include an occurrence whose start is on that date, even if it ends overnight the next local day; no end date is also allowed. Entries MUST retain local start/end times and an IANA planning timezone. Overnight occurrences MUST be supported. All supported scheduling modes MUST follow the same timezone, conflict, occurrence-editing, cancellation, and history-preservation requirements.
- **FR-002**: The system MUST calculate occurrences from local clock times and the IANA planning timezone, storing occurrence instants in UTC and preserving valid local clock times across DST. Nonexistent starts MUST shift forward by the clock change; repeated starts MUST use the first occurrence. For an overnight occurrence crossing DST, its end instant MUST preserve the planned elapsed duration from its resolved start. Show resolved start/end local times clearly without changing the recurring clock time for later dates. New planned times MUST be present or future, never past; there MUST be no future-date scheduling cap. The calendar MUST initially show the current month in the viewer's stored timezone and allow navigation to any later month without restricting creation.
- **FR-003**: The system MUST allow an active circle teacher or supervisor to create a one-off planned session independent of recurring entries. Planned sessions MUST support an optional title, defaulting to "Circle Session" as in JOURNEY T-08. Planned duration is 1–44,640 minutes (31 days), with no restriction on permitted start dates or times. Planned duration is informational and MUST NOT introduce automatic ending or alter F-005's four-hour duration limit and 30-minute idle timeout.
- **FR-004**: The system MUST show each eligible member a circle's scheduled, active, completed, and retained cancelled occurrences in one calendar across their circles, including past dates. The initial view MUST be the current month, with navigation to earlier and later months, subject to the viewer's current access rights.
- **FR-005**: The system MUST identify each calendar item by its circle through text and an accessible non-color cue.
- **FR-006**: The system MUST display scheduled times in the viewing member's local time while preserving the time zone used to plan the occurrence.
- **FR-007**: The system MUST use F-005's scheduled → active → ended lifecycle, displaying Live/Completed appropriately. Active circle teachers and supervisors may cancel only unstarted planned occurrences; cancellation MUST be represented separately from that lifecycle and prevent start and attendance classification. Active sessions use F-005 End; completed history cannot be cancelled. Cancellation persistence and recurrence linkage require the architecture/ADR/contracts process before implementation, without inventing fields here.
- **FR-008**: The system MUST warn active circle teachers and supervisors of detected same-circle and cross-circle overlaps before saving while allowing them to proceed after the warning. Cross-circle comparisons MUST use only circles in which the acting manager has current eligible membership. Students MUST see warnings for overlaps in their own calendar; warnings MUST NOT restrict student participation. Conflict feedback MUST disclose only details the viewer is authorized to read and MUST NOT inspect other members' private commitments. Non-overlapping times MUST NOT trigger warnings; touching endpoints are not overlaps. Open-ended recurrences MUST NOT impose a future scheduling cap; future overlaps MUST be surfaced when their occurrences become available in the calendar.
- **FR-009**: The system MUST derive attendance for completed planned and ad-hoc sessions from durable participation facts without altering them. The eligible roster comprises active students at actual start plus newly enrolled students who subsequently participate. Later membership changes MUST NOT rewrite that historical roster.
- **FR-010**: The system MUST classify first authorized presence at or before actual start plus 10 minutes as Present, and later first presence as Late, with no minimum duration. At session end, eligible students without participation MUST be Absent unless manually overridden. Excused is manual only. Reconnects and multiple devices MUST count once per student/session. Never-started and cancelled sessions MUST have no attendance classification.
- **FR-011**: Current authorized teachers MUST be able to override an individual classification to Present, Late, Absent, or Excused. Every correction MUST retain actor, timestamp, reason, and previous/new classification, survive recalculation, and preserve raw presence facts.
- **FR-012**: Current circle teachers and supervisors may read the circle's attendance, manage schedules, and cancel planned occurrences; only teachers may correct attendance. Eligible students may read only their own attendance. Non-members, revoked members, unauthorized students, and cross-circle callers MUST NOT access protected schedules or attendance, including retained history after revocation. Archived circles retain authorized read-only history for current members and reject scheduling, starting, and attendance corrections.
- **FR-013**: Active circle teachers and supervisors MUST be able to edit/cancel one unstarted occurrence or change/stop future occurrences of an entry. A later series change MUST replace earlier individual edits to affected unstarted occurrences; one-off sessions and started/completed sessions and attendance remain unchanged. Each recurring occurrence MUST retain its identity across retries and generation so duplicate requests or concurrent changes cannot create duplicate sessions or attendance. Conflicting edits MUST be surfaced rather than silently losing a change.
- **FR-014**: The system MUST reuse the approved mobile shell and design foundations for Arabic-first RTL and usable LTR screens, accessible circle colors plus text/non-color identification, and explicit loading, empty, error, retry, success, and offline states.
- **FR-015**: The system MUST provide scheduled reminder eligibility for configurable 1hr/30min/15min/5min intervals, excluding cancelled or superseded occurrences. F-008 owns delivery, preferences, history, and background/closed-app behavior. A clearly labelled partial F-006 pilot is permitted; push acceptance and full feature completion MUST remain pending until F-008 is approved and delivery is verified. In-app placeholders cannot satisfy push acceptance.

### Key Entities *(include if feature involves data)*

- **Recurring schedule entry**: A circle's time commitment defined by selected weekdays on a weekly/biweekly pattern, a positive whole-number interval of days or weeks anchored to the first planned local date, or explicitly selected dates, including local times, IANA planning timezone, optional end date, active state, and identifiable occurrences. Individual edits to unstarted occurrences can be superseded by a later series change; started/completed history is retained.
- **Planned session**: A dated circle meeting, either produced from a recurring schedule or created as a one-off, with a lifecycle outcome and circle ownership.
- **Calendar item**: A member-visible representation of an eligible scheduled, active, completed, or cancelled occurrence on any date permitted by the viewer's current access rights.
- **Attendance record**: A student's policy classification for one completed session, derived from participation facts and optionally corrected by an authorized teacher.
- **Participation fact**: A durable record of a member's session presence, distinct from the attendance classification.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An authorized teacher can create a valid recurring schedule entry or one-off planned session in no more than two minutes without assistance.
- **SC-002**: In acceptance testing, 100% of tested occurrences around a DST transition follow the resolved FR-002 policy and show the correct converted time for the viewer; ordinary occurrences retain their intended planning clock time.
- **SC-003**: In acceptance testing, 100% of active members with sessions in two or more circles can locate all of their eligible upcoming sessions in the unified calendar.
- **SC-004**: In acceptance testing, 100% of completed sessions with known participation facts produce reviewable attendance records, and an authorized manual override does not change those participation facts.
- **SC-005**: In acceptance testing, unauthorized attempts to change schedules or attendance leave the relevant record unchanged.
- **SC-006**: Acceptance tests cover selected weekdays on weekly/biweekly patterns, positive whole-number day/week intervals, duplicate and past selected dates, inclusive local end dates, optional titles, overnight DST duration, one-occurrence edits/cancellation, series changes replacing prior unstarted individual edits, and preserved started/completed history; retries create each occurrence and attendance record exactly once.
- **SC-007**: Attendance tests cover exactly 10 minutes and just after that threshold, no minimum duration, reconnects/devices, absent roster members, newly enrolled participants, ad-hoc sessions, manual correction persistence/audit, and read/write denial by role and archive state.
- **SC-008**: Same- and cross-circle overlaps warn an authorized teacher or supervisor, who may proceed without unauthorized disclosure; students see their own overlap warnings without loss of participation rights. Non-overlapping and touching times produce no false warning.
- **SC-009**: Full completion requires verified F-008 push reminders at every approved interval in foreground/background/closed-app cases, with cancellation and rescheduling preventing stale reminders. A partial pilot explicitly reports this acceptance criterion as pending.

## Assumptions

- Existing circle membership and session lifecycle behavior remain the source of eligibility and session-state truth.
- One-off sessions are explicitly approved; recurring and one-off planning share the same member-facing calendar.
- Stored timestamps use UTC and the planning and display behavior uses named IANA time zones, as decided in OQ-019.
- Live-session participation facts are supplied by F-005; this feature owns attendance policy, classification, and manual overrides as decided in OQ-040.
- Scheduled-session creation belongs to F-006; existing F-005 ad-hoc session creation remains intact as decided in OQ-041.
- F-008 owns notification delivery, preferences, history, and background or closed-app behavior; this feature does not introduce a separate notification pipeline.
- The shared gap batch is closed; this dedicated branch carries the F-006 Spec-Kit lifecycle. Checklist is complete and the current request authorizes planning artifacts only. Tasks → analyze → implement remain separate phases; no production code or executable migration is authorized by planning.
- ADR-025 and the decision register record the approved recurrence, teacher/supervisor scheduling, warning-only overlap, and separate cancellation policy. FEATURES and JOURNEY now reflect that product boundary. ARCHITECTURE and OpenAPI explicitly identify their older weekly schema/contract as unimplemented drafts. `/speckit.plan` must design and obtain required approval for any additional persistence and final API/event contracts before implementation; F-005's three-state lifecycle remains unchanged.
