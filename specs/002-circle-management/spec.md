# Feature Specification: Circle Management

**Feature Branch**: `[002-circle-management]`  
**Created**: 2026-08-07  
**Status**: Draft  
**Input**: User description: "Circle Management (F-002)"

## Clarifications

### Session 2026-08-07

- Q: What is the circle-name limit? → A: 100 characters.
- Q: Who may create circles and how are initial roles assigned? → A: Follow OQ-036 and ADR-010. An authenticated user may create a circle and may assign existing registered users as teachers and one optional backup supervisor. If no teacher is selected, the creator becomes teacher; otherwise the creator becomes supervisor. Invite acceptance creates a student membership. Role management is per-circle and managers cannot change their own role or leave the circle without a teacher.
- Q: Should public circles also support invite links? → A: Yes. Public circles are discoverable and joinable through the public flow, and every circle also has an invite link. Private circles are invite-only.
- Q: Which circle-name and capacity baseline applies? → A: Use the F-002 and architecture baseline: name max 100 characters, default capacity 50, maximum capacity 200.
- Q: Should circles support permanent hard deletion? → A: No. Circles are retired by archiving them. Hard deletion is prohibited; circle data and history must remain available for reports and future needs.
- Q: Which gender values may a circle use? → A: `male`, `female`, `mixed`, or `unspecified`. This circle setting describes the student audience and is independent of the teacher's personal gender; omission defaults to `unspecified`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Create a Circle (Priority: P1)

As an authenticated user, I can create a Quran memorization circle with clear settings and initial role assignments so the circle is ready for members to join.

**Why this priority**: The circle is the core organizational unit and the first useful product action after authentication.

**Independent Test**: Create a circle with valid settings, verify the circle and creator/assigned memberships, and verify the returned invite link and code.

**Acceptance Scenarios**:

1. **Given** an authenticated user and valid circle data, **When** the user creates a circle, **Then** the system creates the circle with the requested settings, generates a unique 8-character invite code, and returns a shareable `halaqaty.app/join/{code}` link.
2. **Given** no teacher is selected during creation, **When** the circle is created, **Then** the creator receives the `teacher` role.
3. **Given** one or more existing registered teachers are selected during creation, **When** the circle is created, **Then** the selected users receive `teacher` membership and the creator receives the role defined by OQ-036; at most one optional backup supervisor may be assigned.
4. **Given** invalid, missing, or over-limit input, **When** the user submits creation, **Then** the request is rejected with standard field validation errors and no partial circle is created.

---

### User Story 2 - Discover and Join a Circle (Priority: P1)

As a student, I can discover a public circle or use an invite link to join an eligible circle so I can participate in a learning group.

**Why this priority**: Joining connects students to the circle and enables the rest of the Halaqaty experience.

**Independent Test**: Join one public circle through discovery and one private/public circle through an invite link; verify student membership and rejection of invalid cases.

**Acceptance Scenarios**:

1. **Given** an active public circle, **When** an authenticated user discovers and joins it, **Then** a `student` membership is created and the circle appears in the user's circles.
2. **Given** a valid invite link for an active public or private circle, **When** an authenticated user confirms joining, **Then** a `student` membership is created.
3. **Given** an invalid, regenerated, or unknown invite code, **When** a user attempts to join, **Then** the request is rejected with the contract-defined invalid-invite response.
4. **Given** an existing membership, a full circle, an archived circle, or a user already in 5 circles, **When** the user attempts to join, **Then** the request is rejected without creating a duplicate or partial membership.

---

### User Story 3 - View Circle and Members (Priority: P1)

As an active circle member, I can view my circle details and its member list with per-circle roles so I understand the group and its governance.

**Why this priority**: Shared visibility establishes trust and provides the authorization context used by later features.

**Independent Test**: Read a circle as an active member, then repeat as a non-member and unauthenticated caller to verify the access boundary.

**Acceptance Scenarios**:

1. **Given** an active member, **When** they request circle details, **Then** the response includes the circle settings and member list with each member's circle role.
2. **Given** a non-member, **When** they request a private circle's member data, **Then** access is denied using the project-standard authorization response.
3. **Given** an archived circle, **When** a member reads it, **Then** its retained history and member data remain available according to the architecture rules while new activity is prevented.

---

### User Story 4 - Manage Members, Roles, and Invite Access (Priority: P2)

As a circle teacher or supervisor authorized by OQ-036, I can manage another member's circle role; as a teacher, I can remove another member and manage invite access without changing my own role, removing the final teacher, or deleting historical records.

**Why this priority**: Safe per-circle governance prevents privilege escalation, self-lockout, and teacherless circles.

**Independent Test**: Exercise role changes and invite regeneration as teacher, supervisor, student, non-member, and self-targeted callers, including the final-teacher safeguard.

**Acceptance Scenarios**:

1. **Given** an authorized teacher or supervisor and another circle member, **When** the manager changes that member between `student`, `supervisor`, and `teacher`, **Then** the new role takes effect immediately for that circle.
2. **Given** a manager targets their own membership, **When** they attempt to change their own role, **Then** the request is rejected.
3. **Given** the last teacher, **When** a request would remove or demote that teacher without another valid teacher, **Then** the request is rejected.
4. **Given** an authorized teacher, **When** they regenerate the invite code, **Then** the old code is invalidated and the new unique 8-character code/link is returned.
5. **Given** a student or non-member, **When** they attempt role or invite-management actions, **Then** the request is rejected with `403 Forbidden` using the standard error envelope.
6. **Given** a teacher and another active member, **When** the teacher removes that member, **Then** active membership ends, historical circle records remain retained, and the operation is rejected if it would remove the final teacher; supervisors, students, and non-members cannot remove members.

---

### User Story 5 - Retire a Circle (Priority: P2)

As a circle teacher, I can retire a circle by archiving it so its history is preserved and no new activity is allowed.

**Why this priority**: Teachers need a safe end-of-life path without losing historical data accidentally.

**Independent Test**: Archive a circle and verify read-only behavior, preserved history, blocked new activity, and the absence of any hard-delete path.

**Acceptance Scenarios**:

1. **Given** a teacher and an active circle, **When** the teacher archives it, **Then** the circle remains readable with its history preserved and new activity is blocked.
2. **Given** an archived circle, **When** any caller attempts a hard-delete operation, **Then** no hard-delete operation exists or is accepted and all circle data remains retained.
3. **Given** a non-teacher caller, **When** they attempt to archive a circle, **Then** the request is rejected and the circle remains unchanged.

---

### Edge Cases

- Circle name is empty or exceeds 100 characters; description exceeds 500 characters; rules exceed 1000 characters.
- Gender is not one of `male`, `female`, `mixed`, or `unspecified`; omitted values default to `unspecified`.
- Capacity is below the architecture minimum, above 200, or would make the circle full at join time.
- Invite-code collision, malformed code, regenerated old code, and an invite link for a retired/archived circle.
- Public discovery must not expose private-circle membership or member data.
- Duplicate membership and joining a sixth circle are rejected atomically.
- Role changes across circles, self-role changes, student role management, and removing the final teacher are rejected.
- Archive/retirement is idempotent or returns contract-defined not-found/conflict errors without data loss; hard deletion is prohibited.
- Missing, invalid, revoked, inactive, or user-mismatched Firebase/backend-session credentials are rejected by Feature 001 protections.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST allow an authenticated user to create a circle with a required name (maximum 100 characters), optional description (maximum 500 characters), optional rules (maximum 1000 characters), capacity, privacy, language, and a circle gender value of `male`, `female`, `mixed`, or `unspecified`, defaulting to `unspecified` when omitted.
- **FR-002**: System MUST enforce the architecture capacity limits, including default capacity 50 and maximum capacity 200.
- **FR-003**: System MUST create the circle and its initial role memberships atomically, following OQ-036 and ADR-010.
- **FR-004**: System MUST generate a unique 8-character invite code and corresponding `halaqaty.app/join/{code}` link for every circle.
- **FR-005**: System MUST allow an authorized teacher to regenerate an invite code and MUST invalidate the previous code immediately.
- **FR-006**: System MUST support public circles that are discoverable/joinable and private circles that are invite-only; both public and private circles MUST support invite links.
- **FR-007**: System MUST allow an authenticated user to join through a valid public-join flow or invite link, creating exactly one `student` membership.
- **FR-008**: System MUST enforce a maximum of 5 simultaneous circle memberships per user and the configured circle capacity.
- **FR-009**: System MUST allow active members to read circle details and the member list with per-circle roles, while preventing unauthorized access to private/member data.
- **FR-010**: System MUST enforce per-circle authorization using `circle_members`; it MUST reject non-members, students performing manager actions, cross-circle role changes, and unauthorized invite management.
- **FR-011**: System MUST allow an authorized teacher or supervisor to change another member between `student`, `supervisor`, and `teacher`, while preventing self-role changes and removal/demotion of the final teacher.
- **FR-012**: System MUST allow a teacher to archive a circle while preserving its history and preventing new activity.
- **FR-013**: System MUST provide teacher-controlled circle retirement through archiving, preserve all circle data and history after retirement, block new activity, and prohibit permanent hard deletion of circles and their history.
- **FR-014**: System MUST return the project-standard error envelope and documented status codes for validation, authentication, authorization, invite, capacity, membership, and archive/retirement failures.
- **FR-015**: System MUST preserve Feature 001 Firebase ID-token and opaque backend-session validation on every protected circle endpoint.
- **FR-016**: System MUST provide contract-first REST APIs, PostgreSQL migrations with rollback, OpenAPI documentation, Flutter screens/state, and focused unit, contract, and integration tests for the approved behavior.
- **FR-017**: System MUST allow only a teacher to remove another active circle member, MUST reject self-removal and removal of the final teacher, and MUST preserve the removed member's historical circle records for reporting and audit purposes.

### Key Entities

- **Circle**: A Quran memorization group with name, description, rules, capacity, privacy, language, student-audience gender (`male`, `female`, `mixed`, or `unspecified`), invite code, archive state, and ownership/governance metadata. Circle gender does not restrict teacher gender.
- **CircleMember**: A per-circle membership linking a user to a circle with exactly one role: `teacher`, `supervisor`, or `student`.
- **InviteCode**: The unique active join credential associated with a circle; regenerating it invalidates the previous credential.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of applicable F-002 acceptance criteria and approved clarifications map to a requirement and acceptance scenario in this spec; the hard-deletion criterion is superseded by the clarification recorded above.
- **SC-002**: 100% of protected circle operations reject missing/invalid credentials and unauthorized role combinations in automated tests.
- **SC-003**: 100% of generated invite codes are unique within the active system and previous codes stop authorizing joins immediately after regeneration.
- **SC-004**: 100% of tested duplicate-membership, sixth-circle, full-circle, archived-circle, final-teacher, and hard-delete safeguards preserve database consistency with no partial writes or historical data loss.
- **SC-005**: A teacher can complete circle creation and share a valid invite link in under 2 minutes in the primary mobile flow.

## Assumptions

- Feature 001 authentication, profile, backend sessions, and existing RBAC middleware are available and reused; this feature does not change their lifecycle.
- Circle roles are stored only per circle in `circle_members`; no Firebase custom claims or new global roles are introduced.
- Public discovery and joining expose only the minimum public circle information; member details remain protected by membership authorization.
- Circle retirement is implemented as archive/soft state only. Hard deletion is prohibited; foreign-key behavior, retention, auditability, and reporting access must preserve circle history.
- REST is the source of truth for circle CRUD, discovery, membership, role management, invite management, and archive/retirement; hard deletion is not supported, and realtime queue/session work is deferred to later features.
- API changes remain contract-first and backward-compatible through `docs/contracts/openapi.yaml`.
