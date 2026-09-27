# Feature Specification: F-006 Schedule, Calendar & Attendance

**Feature Branch**: `019-gap-completion`

**Created**: 2026-09-26

**Status**: Draft

**Input**: User description: "Create the already registered F-006 Schedule, Calendar & Attendance specification."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Manage a Circle Schedule (Priority: P1)

As a teacher, I can add, view, change, or stop recurring schedule entries for a circle so that its members know when regular sessions take place.

**Why this priority**: A reliable published schedule is the foundation for calendars, planned sessions, attendance, and future reminders.

**Independent Test**: A teacher creates two weekly entries for one circle, views their upcoming occurrences, changes one entry, and stops the other; active members see the resulting schedule in their local time.

**Acceptance Scenarios**:

1. **Given** an active teacher manages a circle, **When** they create a weekly entry with a day, start time, end time, and time zone, **Then** it is shown as an upcoming recurring session time for that circle.
2. **Given** a circle has multiple active schedule entries, **When** a teacher changes or stops one entry, **Then** the other active entries remain unchanged and members see the revised future schedule.
3. **Given** a student is an active circle member, **When** they view the circle schedule, **Then** each occurrence is displayed in the student's local time with a clear association to the circle.

---

### User Story 2 - Plan and Find Sessions (Priority: P1)

As a teacher, I can create a one-off planned session, and as a member I can see all of my upcoming circle sessions in one calendar, so that special and regular meetings are discoverable.

**Why this priority**: One-off sessions are an approved requirement, and a unified view prevents members of several circles from missing a session.

**Independent Test**: A teacher creates an unlinked one-off session and a recurring schedule exists in another circle; an enrolled student sees both on one calendar with their local times and circle identity.

**Acceptance Scenarios**:

1. **Given** an active teacher manages a circle, **When** they create a one-off planned session, **Then** it is independent of recurring entries and is visible to active circle members.
2. **Given** a member belongs to more than one circle, **When** they open their calendar, **Then** it combines their eligible upcoming sessions and distinguishes the owning circles without relying only on color.
3. **Given** a planned session reaches its start, **When** an authorized teacher starts it, **Then** its lifecycle accurately reflects that it is live; after it ends, it is shown as completed.
4. **Given** a planned session is cancelled, **When** a member views the calendar, **Then** the cancellation is clear and the session cannot be started or recorded as attended.

---

### User Story 3 - Review and Correct Attendance (Priority: P1)

As a teacher, I can review attendance derived from session participation and correct an individual student's classification so that attendance records reflect what happened in class.

**Why this priority**: Attendance must be a durable teaching record, while live participation alone is not an attendance decision.

**Independent Test**: After a session with participation facts for several members, the teacher reviews every member's classified record, changes one classification, and sees the corrected result persist without changing the captured participation facts.

**Acceptance Scenarios**:

1. **Given** a completed circle session has participant-presence facts, **When** an authorized teacher reviews attendance, **Then** every relevant active student has a classification and the system preserves the underlying participation record.
2. **Given** an authorized teacher reviews a student's attendance record, **When** they set it to present, late, absent, or excused, **Then** the new classification and its manual nature are clearly recorded.
3. **Given** a student or an unrelated user attempts to change another student's attendance, **When** they submit the request, **Then** the record remains unchanged and they receive an understandable permission outcome.

---

### User Story 4 - Avoid Scheduling Surprises (Priority: P2)

As a teacher, I receive a clear conflict outcome when planning times overlap, so that I can avoid creating confusing circle commitments.

**Why this priority**: Conflict awareness protects the calendar's usefulness but does not replace the primary scheduling and attendance journey.

**Independent Test**: A teacher creates a proposed time that overlaps another circle session they manage and receives the agreed conflict outcome before the schedule is committed.

**Acceptance Scenarios**:

1. **Given** a teacher plans a time overlapping another relevant planned session, **When** they attempt to save it, **Then** the system applies the approved overlap policy and identifies the conflicting commitment.
2. **Given** a teacher plans a time that does not overlap a relevant commitment, **When** they save it, **Then** the schedule is accepted without a false conflict warning.

### Edge Cases

- A recurring local time crosses a daylight-saving transition; future occurrences keep the intended local clock time and are shown correctly in each member's local time.
- A schedule entry has an end time that is not later than its start time; it is rejected with an understandable correction.
- A member loses circle membership after a session was planned; future calendar access follows their current eligibility while retained attendance history follows the approved record policy.
- A teacher changes or stops a recurring entry after occurrences have been planned; completed sessions and their attendance records are retained.
- Two update attempts modify the same schedule entry; the user is informed of the current outcome and no change is silently lost.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST allow an authorized teacher to create, view, update, and stop multiple recurring weekly schedule entries for a circle, each with a weekday, local start and end time, and named time zone.
- **FR-002**: The system MUST calculate future recurring occurrences from an entry's local time and named time zone, retaining the intended local clock time across daylight-saving changes.
- **FR-003**: The system MUST allow an authorized teacher to create a one-off planned session that is not linked to a recurring schedule entry.
- **FR-004**: The system MUST show each active member a circle's eligible upcoming sessions and provide each member with one calendar that combines their eligible upcoming sessions across circles.
- **FR-005**: The system MUST identify each calendar item by its circle through text and an accessible non-color cue.
- **FR-006**: The system MUST display scheduled times in the viewing member's local time while preserving the time zone used to plan the occurrence.
- **FR-007**: The system MUST support planned-session lifecycle outcomes of scheduled, live, completed, and cancelled, and prevent cancelled sessions from being started or used for attendance.
- **FR-008**: The system MUST detect overlapping commitments relevant to the teacher before saving a new or changed scheduled time. [NEEDS CLARIFICATION: Does an overlap only warn and allow saving, or must it block the save? Which commitments are relevant: all circles the teacher manages, only the same circle, or all circles where the teacher is a member?]
- **FR-009**: The system MUST derive attendance candidates from durable participation facts for a completed session without altering those facts.
- **FR-010**: The system MUST classify each relevant student's attendance as present, late, absent, or excused. [NEEDS CLARIFICATION: What rule and threshold classify a participant as late, and when is a non-participant classified absent?]
- **FR-011**: The system MUST allow an authorized teacher to manually override an individual attendance classification while preserving that the result was manually changed.
- **FR-012**: The system MUST prevent students and unauthorized users from changing another student's attendance or a circle's schedule.
- **FR-013**: The system MUST retain completed sessions and their attendance records when a future recurring schedule entry is changed or stopped.
- **FR-014**: The system MUST provide Arabic-first, right-to-left-aware schedule, calendar, and attendance screens while preserving usable left-to-right layouts.
- **FR-015**: The system MUST make reminder eligibility available for scheduled sessions without duplicating notification delivery or user-preference behavior owned by F-008. [NEEDS CLARIFICATION: Before F-008 is approved and delivered, is an in-app schedule reminder sufficient for F-006's initial release, or must F-006 wait for background/closed-app delivery?]

### Key Entities *(include if feature involves data)*

- **Recurring schedule entry**: A circle's reusable weekly time commitment, including local day and time, named time zone, active state, and future occurrences.
- **Planned session**: A dated circle meeting, either produced from a recurring schedule or created as a one-off, with a lifecycle outcome and circle ownership.
- **Calendar item**: A member-visible representation of an eligible upcoming or cancelled planned session.
- **Attendance record**: A student's policy classification for one completed session, derived from participation facts and optionally corrected by an authorized teacher.
- **Participation fact**: A durable record of a member's session presence, distinct from the attendance classification.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An authorized teacher can create a valid recurring schedule entry or one-off planned session in no more than two minutes without assistance.
- **SC-002**: In acceptance testing, 100% of tested occurrences around a daylight-saving transition show the intended local time for the planning time zone and the correct converted time for the viewer.
- **SC-003**: In acceptance testing, 100% of active members with sessions in two or more circles can locate all of their eligible upcoming sessions in the unified calendar.
- **SC-004**: In acceptance testing, 100% of completed sessions with known participation facts produce reviewable attendance records, and an authorized manual override does not change those participation facts.
- **SC-005**: In acceptance testing, unauthorized attempts to change schedules or attendance leave the relevant record unchanged.

## Assumptions

- Existing circle membership and session lifecycle behavior remain the source of eligibility and session-state truth.
- One-off sessions are explicitly approved; recurring and one-off planning share the same member-facing calendar.
- Stored timestamps use UTC and the planning and display behavior uses named IANA time zones, as decided in OQ-019.
- Live-session participation facts are supplied by F-005; this feature owns attendance policy, classification, and manual overrides as decided in OQ-040.
- Scheduled-session creation belongs to F-006; existing F-005 ad-hoc session creation remains intact as decided in OQ-041.
- F-008 owns notification delivery, preferences, history, and background or closed-app behavior; this feature does not introduce a separate notification pipeline.
