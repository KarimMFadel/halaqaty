# Feature Specification: Authentication, Roles, and User Profile

**Feature Branch**: `[001-auth-roles-profile]`  
**Created**: 2026-07-25  
**Status**: Approved  
**Input**: User description: "Authentication, Roles, and User Profile"

## Clarifications

### Session 2026-07-25

- Q: Which fields are required for first-time profile completion? → A: full_name + country.
- Q: Which roles can be selected during self-registration? → A: None. Self-registration does not create circle roles; circle roles are assigned through circle creation, invites, and authorized circle-role management.
- Q: What is the token policy? → A: Firebase ID tokens (1-hour lifecycle with SDK auto-refresh) and backend-enforced 30-day inactivity logout.

### Session 2026-07-31

- Q: Which component owns registration and sign-in? → A: The Flutter Firebase SDK owns password validation, identity creation, sign-in, and Firebase ID-token refresh. The Go API verifies Firebase ID tokens and creates or revokes durable per-device backend sessions; it never accepts passwords or returns Firebase tokens.
- Q: How are initial circle roles assigned? → A: A creator may assign existing registered users as one or more teachers and an optional backup supervisor. Invite acceptance creates a student membership. Without a selected teacher, the creator becomes teacher; otherwise the creator becomes supervisor.
- Q: Who may later change teacher or supervisor assignments? → A: A teacher or supervisor may change another member between student, supervisor, and teacher. Managers cannot change their own role or leave the circle with no teacher; students cannot manage roles.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Secure Account Access (Priority: P1)

As a new or returning user, I can register or sign in with email and password so I can securely access the platform and start a session.

**Why this priority**: Account access is the entry point to all other product value and must be reliable and secure before any downstream feature can be used.

**Independent Test**: Register a new user, sign in with valid credentials through Firebase, verify protected routes require both a Firebase ID token and the matching backend session ID, and verify logout/session-expiration behavior.

**Acceptance Scenarios**:

1. **Given** a new email, **When** the user registers with a valid password through the Flutter Firebase SDK and provisions their profile with the API, **Then** the account and local user record are created and a current-device backend session is returned.
2. **Given** valid credentials, **When** the user signs in through the Flutter Firebase SDK and creates a backend session, **Then** the mobile session becomes active.
3. **Given** an authenticated user, **When** the user logs out from the current device/session, **Then** only that session is invalidated and protected backend access is rejected until sign-in.

---

### User Story 2 - Complete Basic Profile (Priority: P2)

As an authenticated user, I can create, view, and update my basic profile from the mobile app so my identity information is available across platform experiences.

**Why this priority**: Profile completion is required for onboarding quality and personalized user presence, but depends on core authentication being in place.

**Independent Test**: Login on mobile, open profile details, update editable fields, and verify saved profile is returned consistently.

**Acceptance Scenarios**:

1. **Given** an authenticated user, **When** the user views profile details, **Then** the latest saved profile data is returned.
2. **Given** an authenticated user, **When** the user updates allowed profile fields, **Then** the changes persist and are returned by subsequent profile reads.
3. **Given** a first-time profile completion, **When** full_name or country is missing, **Then** the update is rejected with validation errors using the standard error envelope.

---

### User Story 3 - Enforce Circle Role-Based Access (Priority: P3)

As a system owner, I need protected endpoints to enforce per-circle authorization so only authorized members can perform restricted actions.

**Why this priority**: Role enforcement protects sensitive operations and governance, but builds on authentication and token validation foundations.

**Independent Test**: Call a role-management endpoint with teacher, supervisor, student, and non-member sessions and confirm only authorized managers may update another member without self-changing or removing the final teacher.

**Acceptance Scenarios**:

1. **Given** a role-management endpoint, **When** a student or non-member uses it, **Then** the request is rejected with an authorization error.
2. **Given** a protected endpoint, **When** a request has a missing, invalid, revoked, or mismatched Firebase token or backend session ID, **Then** the request is rejected.
3. **Given** a circle-role-management endpoint, **When** a teacher or supervisor updates another member without removing the final teacher, **Then** access is granted.

---

### Edge Cases

- Duplicate email registration attempt is rejected with conflict error and no account overwrite.
- Missing or malformed Firebase ID token is rejected.
- A missing, revoked, unknown, inactive, or user-mismatched backend session ID is rejected on a protected request.
- Backend session inactivity beyond 30 days forces re-authentication.
- Missing full_name or country during first-time profile completion blocks completion until both are provided.
- Backend authentication endpoints never accept passwords or return Firebase ID or refresh tokens.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The Flutter client MUST allow account registration with unique email/password through Firebase Auth, which rejects duplicate email attempts; the API MUST provision the corresponding local user from a verified Firebase ID token.
- **FR-002**: System MUST securely store passwords in non-plaintext form (via Firebase credential handling) and MUST never return passwords in API responses.
- **FR-003**: The Flutter Firebase SDK MUST authenticate credentials, create identities, and refresh Firebase ID tokens. The API MUST verify Firebase ID tokens and return only an opaque current-device backend session identifier, never Firebase tokens.
- **FR-004**: System MUST enforce backend session inactivity logout at 30 days and require re-authentication after inactivity expiration.
- **FR-005**: System MUST invalidate only the current device/session on logout and reject subsequent protected access for that revoked session until re-authentication.
- **FR-006**: System MUST not assign any circle role during self-registration. Circle creation MUST let the creator assign existing registered users as one or more teachers and one optional backup supervisor; if no teacher is selected, the creator MUST become teacher, otherwise the creator MUST become supervisor. Invite acceptance MUST create a student membership. A teacher or supervisor may change another member between student, supervisor, and teacher, but MUST NOT change their own role or leave the circle without a teacher.
- **FR-007**: After backend-session creation, system MUST enforce both Firebase ID-token and `X-Halaqaty-Session-ID` validation on every protected route, rejecting missing, malformed, expired, revoked, unknown, inactive, or user-mismatched credentials. Registration and backend-session creation require only the Firebase ID token.
- **FR-008**: System MUST enforce authorization using PostgreSQL `circle_members` roles per circle for protected endpoints.
- **FR-009**: System MUST allow authenticated users to create, read, and update their own basic profile.
- **FR-010**: System MUST provide mobile flows for register, login, logout, profile view, and profile edit.
- **FR-011**: System MUST return standardized error responses as `{ "error": { "code", "message", "fields?" } }` with documented codes for auth/profile/authorization failures.
- **FR-012**: System MUST require `full_name` and `country` for first-time profile completion.
- **FR-013**: System MUST enforce rate limits for REST requests per IP and per user, and WebSocket limits of max 3 active connections per user and max 30 messages/min/user/circle.

### Key Entities *(include if feature involves data)*

- **User**: Authenticated account identity with Firebase UID, email, status, and audit timestamps.
- **Profile**: User-managed personal details including full_name, display_name, bio, country, avatar_url, and updated_at. `full_name` and `country` are mandatory on first completion.
- **CircleMember**: Per-circle authorization record mapping user_id + circle_id to role (student/teacher/supervisor) and membership status.
- **UserSession**: Backend session activity record used for inactivity timeout enforcement, including last_activity_at and revoked_at.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: At least 95% of successful login attempts complete in under 2 seconds end-to-end.
- **SC-002**: 100% of requests to protected endpoints with missing, invalid, or unauthorized credentials are rejected.
- **SC-003**: At least 90% of users complete registration and first profile update without support assistance.
- **SC-004**: 0 confirmed cases of plaintext password exposure in stored records or API responses.

## Assumptions

- Email/password registration and login use Firebase Auth token issuance and verification model.
- Authorization decisions are based on per-circle roles in `circle_members`, not global role-only authorization.
- Basic profile fields are limited to onboarding-relevant identity data and exclude advanced settings.
- API changes remain backward-compatible and contract-first through `docs/contracts/openapi.yaml`.
- Full admin dashboard remains out of scope for this feature.
