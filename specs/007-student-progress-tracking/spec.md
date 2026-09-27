# Feature Specification: F-007 Enhanced Student Progress Tracking

**Feature Branch**: `019-gap-completion`

**Created**: 2026-09-26

**Status**: Draft

**Input**: Approved F-007 entry in `FEATURES.md`, frozen progress decisions, and the existing F-003 recitation history.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Review My Learning History (Priority: P1)

As a student, I can review my attendance and completed recitations across circles so that I understand when I attended and when I practiced.

**Why this priority**: Attendance and practice are different commitments; students need a truthful history before summaries can be trusted.

**Independent Test**: A student with attended-only, completed-recitation, skipped, and missed sessions sees the correct labels and details without seeing another student's records.

**Acceptance Scenarios**:

1. **Given** a completed session where I attended but had no completed turn, **When** I open my history, **Then** it is labeled attended only, not practiced.
2. **Given** a session with at least one completed turn, **When** I open its history, **Then** I see the recited Surah, Ayah range, type, grade when available, and each pass as a separate record.
3. **Given** a skipped or opted-out turn without a completed turn, **When** I view that session, **Then** it does not count as practice.
4. **Given** that I belong to multiple circles, **When** I filter by a circle or view all circles, **Then** the history and counts reflect that selection.

---

### User Story 2 - See Quran Progress (Priority: P1)

As a student, I can see a Quran-wide view of my completed recitations and review needs so that I know which Surahs and Ayah ranges to practice next.

**Why this priority**: The Quran Map turns session records into an actionable learning view.

**Independent Test**: A student with overlapping Ayah ranges, repeated passes, and records in two circles sees a complete 114-Surah map and an accurate Surah detail.

**Acceptance Scenarios**:

1. **Given** completed recitations, **When** I open the Quran Map, **Then** all 114 Surahs are present, with coverage and status derived from the approved rules; untouched Surahs are shown as not started.
2. **Given** a long Surah with only some Ayahs recited, **When** I open its detail, **Then** covered and uncovered ranges are distinguishable without relying only on color.
3. **Given** a memorized Surah with no revision for 30 days, **When** I view it, **Then** it retains its memorized status and shows a stale-review explanation.
4. **Given** recitations in several circles, **When** I switch between all-circle and one-circle views, **Then** the global status follows the most recent update while the selected circle shows only its own records.

---

### User Story 3 - Review My Progress Summary (Priority: P2)

As a student, I can see attendance, practice, and recitation trends over time so that I can assess consistency without conflating attendance with practice.

**Why this priority**: A summary makes the detailed record useful for planning without inventing a new data source.

**Independent Test**: A student sees weekly and monthly recited-Ayah counts plus attendance and practice percentages for all circles and a selected circle, with the underlying session counts visible.

**Acceptance Scenarios**:

1. **Given** sessions in a selected period, **When** I open statistics, **Then** I see separate attendance and practice percentages with their numerator and denominator.
2. **Given** no eligible sessions in a period, **When** I open statistics, **Then** the rate is shown as unavailable rather than a misleading zero or full score.

---

### User Story 4 - Find Students Needing Attention (Priority: P2)

As a teacher, I can review progress for students I teach and see those who repeatedly attend without reciting so that I can offer support.

**Why this priority**: Teachers need actionable insight from the same trusted history students see.

**Independent Test**: A teacher with two circles sees per-student attendance, practice, and recency; a seven-session attended-without-practice run is flagged, and an unrelated teacher cannot access the student.

**Acceptance Scenarios**:

1. **Given** a student in my circle, **When** I open the circle summary, **Then** I see separate attendance and practice rates, last practice date, and an explained attention flag when the approved threshold is met.
2. **Given** I teach a student in at least one circle, **When** I open that student's detail, **Then** I may see the approved cross-circle history and Quran Map.
3. **Given** I do not teach the student in any circle, **When** I request that student's progress, **Then** access is denied without exposing their records.
4. **Given** recent grades for my circle, **When** I view Surah insights, **Then** Surahs with weak-grade frequency are ranked over the last 30 days with supporting student counts.

### Edge Cases

- Repeated and overlapping Ayah ranges must not inflate unique coverage beyond the Surah's Ayah count; all passes remain in history.
- A completed ungraded turn contributes practice and coverage but does not invent a grade.
- A `test` recitation remains in history but does not determine Quran Map status.
- A grade correction updates the derived current status without creating a second pass.
- A student's removed circle membership must follow the approved retained-history and teacher-access policy; historical data cannot be exposed merely through an old link.
- Arabic RTL and English LTR layouts remain usable, and status is never conveyed by color alone.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST preserve a separate progress record for each completed session recitation and MUST NOT create one for skipped or opted-out turns.
- **FR-002**: The system MUST distinguish attended sessions from practiced sessions; a practiced session has at least one completed turn.
- **FR-003**: The system MUST provide each student with a paginated cross-circle session and recitation history, with an optional circle filter and full pass details.
- **FR-004**: The system MUST provide each student with a 114-Surah Quran Map, global and circle-filtered views, approved status categories, and unique Ayah-range coverage.
- **FR-005**: The system MUST derive current cross-circle Surah status from the most recent applicable update, exclude `test` recitations from map-status derivation, and preserve all historical passes.
- **FR-006**: The system MUST show a stale-review badge after 30 days without revision on a memorized Surah without removing its memorized status.
- **FR-007**: The system MUST show separate attendance and practice rates and weekly/monthly recited-Ayah counts, with the underlying counts and selected circle or period visible. [NEEDS CLARIFICATION: Does a `late` F-006 attendance classification count as attended in these percentages, and which completed sessions belong in the denominator?]
- **FR-008**: The system MUST let a teacher review per-student progress in circles they teach and approved cross-circle detail for a student they currently teach in at least one circle.
- **FR-009**: The system MUST flag a student after at least seven consecutive sessions classified attended with no completed recitation, with an understandable explanation.
- **FR-010**: The system MUST show a teacher the Surahs with the highest weak-grade frequency in the last 30 days for an authorized circle, with supporting student counts.
- **FR-011**: The system MUST deny a student access to another student's progress and deny a teacher access to students outside their authorized circles.
- **FR-012**: The system MUST present progress in Arabic-first, RTL-aware screens with usable LTR layout, text and icon status cues, and accessible labels.

### Key Entities *(include if feature involves data)*

- **Completed recitation**: A durable session-based pass, with student, circle, session, Surah, Ayah range, recitation type, optional grade and note, and correction history.
- **Attendance classification**: F-006's policy outcome for one student and session, distinct from presence facts and completed recitations.
- **Surah progress**: A derived view of unique Ayah coverage, current status, and stale-review indicator for one Surah and selected circle scope.
- **Progress summary**: Period and scope-specific attendance, practice, and recitation counts used to explain displayed rates.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In acceptance cases, 100% of skipped and opted-out-only sessions are excluded from practiced counts, while every completed turn appears once in history.
- **SC-002**: In acceptance cases, unique Ayah coverage never exceeds 100% and repeated passes remain individually reviewable.
- **SC-003**: A student can find a recent recitation and its grade, when present, in no more than three actions from My Progress.
- **SC-004**: In authorization tests, 100% of unrelated-student and unrelated-teacher access attempts expose no progress records.
- **SC-005**: In acceptance cases, the seven-session attention flag and 30-day stale badge appear only at their approved thresholds.

## Assumptions

- F-003 already writes one progress record per completed queue entry, including Surah ID, Ayah range, nullable five-value grade, and correction updates. This feature consumes and verifies that foundation rather than recreating it.
- F-006 owns attendance classification and overrides; its decision is needed before final attendance percentages and attention-flag semantics can be approved.
- F-007 delivers student progress content and teacher progress summaries. F-010 owns any remaining dashboard shell and navigation; F-008 owns grade notifications.
- Student self-logging outside sessions, PDF reports, and parent access are outside this feature.
