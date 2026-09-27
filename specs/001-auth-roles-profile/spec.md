# Feature Specification: Authentication, Roles, and User Profile

**Feature Branch**: `[001-auth-roles-profile]`  
**Created**: 2026-07-25  
**Status**: Approved  
**Account-deletion amendment**: Approved for student-account deletion; teacher-account deletion depends on F-008 notification delivery
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

### Session 2026-09-26 — account deletion

- Q: Which identity remains on retained teaching, chat, and recitation history? → A: Display name only; erase full name, email, phone, avatar reference, and other non-retained profile fields.
- Q: How recent must reauthentication be? → A: The verified Firebase `auth_time`, not the ID token issue time, must be within five minutes of confirmation.
- Q: What is the deletion order? → A: Reauthenticate immediately before confirmation, close backend access and revoke all sessions, then remove the Firebase identity.
- Q: Can teacher-owned circles be archived and members notified now? → A: Wait for F-008 delivery; teacher-account deletion stays unavailable until then.
- Implementation boundary for the student-only slice: accounts with an active teacher or supervisor membership or an owned active circle are excluded until the F-008-dependent manager path is specified and delivered.
- Q: Does this batch include other unchecked F-001 account controls? → A: No; implement deletion and directly related privacy controls only.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Secure Account Access (Priority: P1)

As a new or returning user, I can register or sign in with email and password so I can securely access the platform and start a session.

**Why this priority**: Account access is the entry point to all other product value and must be reliable and secure before any downstream feature can be used.

**Independent Test**: Register a new user, sign in with valid credentials through Firebase, verify protected routes require both a Firebase ID token and the matching backend session ID, and verify logout/session-expiration behavior.

**Acceptance Scenarios**:

1. **Given** a new email, **When** the user registers with a valid password through the Flutter Firebase SDK and provisions their profile with the API, **Then** the account and local user record are created and a current-device backend session is returned.
   - **Given** a re-registration with the **same** Firebase UID, **When** the Flutter client re-sends provisioning, **Then** the API returns HTTP 409 with a valid `BackendSessionResponse` and the mobile client stores it as a fresh session.
   - **Given** a registration with a **different** Firebase UID but an email already bound to a local user, **When** the Flutter client sends provisioning, **Then** the API returns HTTP 409 with `ERR_CONFLICT` and no session body.
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

### User Story 4 - Delete My Account (Priority: P2; amendment approved for student deletion)

As a user, I can irreversibly close my account after confirming my identity so that my account and eligible personal data are removed while agreed teaching history remains intact.

**Why this priority**: The F-001 feature board promises account deletion, but the approved original specification and running application do not implement it.

**Independent Test**: A user with no active teacher or supervisor membership reauthenticates, confirms deletion, loses access from every signed-in device, and cannot sign in again with the deleted identity. Historical records retain only the approved attribution. A circle manager cannot complete deletion until the F-008-dependent circle consequences are available.

**Acceptance Scenarios**:

1. **Given** an authenticated student who recently reauthenticated, **When** they explicitly confirm deletion, **Then** the account becomes inaccessible, all backend sessions are revoked, the Firebase identity is removed, and the app shows a truthful completion outcome.
2. **Given** a missing or stale reauthentication, **When** deletion is requested, **Then** no deletion occurs and the user is guided to reauthenticate.
3. **Given** an active teacher or supervisor membership or an owned active circle, **When** deletion is requested before F-008 member notification exists, **Then** deletion is blocked with a clear reason and no circle is partly archived.
4. **Given** the user has an active participant record in an active live session, **When** deletion is requested, **Then** it is blocked before mutation and the user can retry after leaving or the session ends.
5. **Given** a partial failure between backend closure and Firebase identity removal, **When** the operation is retried, **Then** it resumes safely without restoring access or deleting extra history.

---

### Edge Cases

- Same-Firebase-UID re-registration returns HTTP 409 with a valid `BackendSessionResponse` (idempotent session replay — treated as success by mobile). Different-Firebase-UID registration with an already-bound email returns HTTP 409 with `ERR_CONFLICT` and no session body; Firebase Auth prevents this at the identity layer but the API enforces a safety net.
- Missing or malformed Firebase ID token is rejected.
- A missing, revoked, unknown, inactive, or user-mismatched backend session ID is rejected on a protected request.
- Backend session inactivity beyond 30 days forces re-authentication.
- Missing full_name or country during first-time profile completion blocks completion until both are provided.
- Backend authentication endpoints never accept passwords or return Firebase ID or refresh tokens.
- A deleted account must not be able to create a new backend session using an old token, even if Firebase identity removal is delayed or fails.
- Retained teaching, chat, and recitation history must remain accessible only to users who were already authorized for that history.
- User-authored historical content remains intact; deletion erases account/profile identity fields other than the display name, not the contents of past messages or recitation records.
- Account deletion is blocked without mutation while the user has an active participant record in an active live session; an already-issued media credential must not remain usable after deletion.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The Flutter client MUST allow account registration with email/password through Firebase Auth. Firebase Auth MUST reject any registration attempt that uses the same email under a different Firebase identity before the request reaches the API. The API MUST provision the corresponding local user from a verified Firebase ID token and MUST apply the following idempotency rules: (a) if the Firebase UID is already provisioned, the API MUST return HTTP 409 with a valid `BackendSessionResponse` body (a fresh session is issued and stored by the mobile client — treated as success); (b) if a different Firebase UID attempts to register with an email already bound to an existing local user, the API MUST return HTTP 409 with `{ "error": { "code": "ERR_CONFLICT", "message": "..." } }` and MUST NOT create or return a session.
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
- **FR-014**: The system MUST require explicit irreversible-deletion confirmation and a verified Firebase `auth_time` within five minutes before accepting an account-deletion request. The ID token's issue time MUST NOT substitute for `auth_time`.
- **FR-015**: The system MUST make a deleted account non-authenticatable and revoke every backend session for that account before attempting Firebase identity removal.
- **FR-016**: The system MUST erase the user's email, full name, phone, avatar reference, and other non-retained profile data while preserving existing circle, session, chat, and recitation history. History retains the user's display name only as human-readable identity attribution.
- **FR-017**: The system MUST preserve historical records without hard-deleting referenced circle or user records, and MUST prevent a deleted identity from viewing or changing them.
- **FR-018**: This student-only deletion batch MUST reject an account with any active teacher or supervisor membership or owned active circle before mutation. The future teacher-account path MUST require a designated supervisor for every owned active circle, archive those circles without automatic teacher transfer, and notify their members through F-008. Until that notification delivery exists, manager-account deletion MUST be unavailable.
- **FR-019**: A deletion operation MUST be safely retryable across backend and Firebase steps, with a truthful pending or failure outcome and no restoration of backend access after a committed closure.
- **FR-020**: The system MUST not represent a partly completed deletion as a successful 204 response or success screen.
- **FR-021**: Before closing a student account, the system MUST reject deletion while the account has an active participant record in an active live session. Session admission and account closure MUST serialize on the user row so no media credential is issued after closure commits. The user may retry after leaving or after the live session ends.

### Key Entities *(include if feature involves data)*

- **User**: Authenticated account identity with Firebase UID, email, status, and audit timestamps.
- **Profile**: User-managed personal details including full_name, display_name, bio, country, avatar_url, and updated_at. `full_name` and `country` are mandatory on first completion.
- **CircleMember**: Per-circle authorization record mapping user_id + circle_id to role (student/teacher/supervisor) and membership status.
- **UserSession**: Backend session activity record used for inactivity timeout enforcement, including last_activity_at and revoked_at.
- **Deleted account record**: A non-authenticatable reference retained solely to preserve authorized historical attribution and referential integrity, with personal fields limited by the approved retention decision.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: At least 95% of successful login attempts complete in under 2 seconds end-to-end.
- **SC-002**: 100% of requests to protected endpoints with missing, invalid, or unauthorized credentials are rejected.
- **SC-003**: At least 90% of users complete registration and first profile update without support assistance.
- **SC-004**: 0 confirmed cases of plaintext password exposure in stored records or API responses.
- **SC-005**: In acceptance tests, 100% of deleted-account backend sessions and subsequent sign-in attempts are rejected, including when Firebase cleanup is temporarily unavailable.
- **SC-006**: In acceptance tests, 100% of manager deletions and deletions during active live sessions are blocked before mutation; student deletion preserves all historical circle, session, chat, and recitation rows.
- **SC-007**: Retrying deletion after each simulated failure point never creates a new account session or removes additional teaching history.

## Assumptions

- Email/password registration and login use Firebase Auth token issuance and verification model.
- Authorization decisions are based on per-circle roles in `circle_members`, not global role-only authorization.
- Basic profile fields are limited to onboarding-relevant identity data and exclude advanced settings.
- API changes remain backward-compatible and contract-first through `docs/contracts/openapi.yaml`.
- Full admin dashboard remains out of scope for this feature.
- F-008 owns member notification delivery for manager-account deletion. This student-only amendment does not implement teacher/supervisor deletion; that path requires a later approved amendment after F-008 delivery exists.
- This amendment covers deletion and directly related privacy controls. Avatar upload, email verification, Google/Apple sign-in, and password reset remain separate unchecked F-001 work and are not part of this batch.
