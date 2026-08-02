# Spec-Kit Exact Prompts (No Placeholders)

Use these prompts exactly in a new Copilot session.

---

## 0) Session bootstrap (paste first)

```text
I want to implement this project using Spec-Kit in strict low-token mode.

Global rules for all your responses:
1) Be concise and action-only.
2) No long reasoning/thinking logs.
3) No repeated context.
4) Use this output format only:
   - Result
   - Files
   - Next Command
5) If blocked, explain blocker in max 2 lines.
6) Keep output compact (max 8 bullets unless I ask for more).
7) Before every feature and every major task, provide exact git branch commands first.
```

---

## Feature 1: Authentication, Roles, and User Profile

### 1.1 Create feature branch

```bash
git checkout -b 001-auth-roles-profile
```

### 1.2 Run specification

```text
For this feature, first confirm the git command I already ran, then run /speckit.specify.

Feature name: Authentication, Roles, and User Profile

Business goals:
- Users can register and sign in securely.
- Circle permissions are enforced by the per-circle student, teacher, and supervisor roles.
- Users can complete and update a basic profile used across backend and mobile.

Primary users:
- Student
- Teacher

In scope:
- Firebase Auth registration and sign-in in Flutter (email/password, Google, and Apple where applicable).
- Backend provisioning and durable per-device sessions after a verified Firebase ID token.
- Current-device logout and 30-day backend session inactivity expiry.
- Profile create/read/update endpoints.
- Flutter screens for register, login, profile edit.
- Route protection by authentication and role.

Out of scope:
- Social login providers.
- Full admin dashboard.
- Advanced profile settings unrelated to onboarding.

User stories by priority:
1) P1: As a user, I can register and sign in through Firebase Auth, establish a backend device session, and access protected resources.
2) P2: As a user, I can view and edit my profile from mobile app.
3) P3: As a circle teacher, I can promote or revoke a supervisor while protected endpoints enforce per-circle roles.

Acceptance scenarios:
- Given a new email, when the Flutter Firebase SDK registers the user with a valid password, then Firebase creates the identity; after a verified ID token reaches the backend, a local account and device session are created.
- Given valid credentials, when the Flutter Firebase SDK signs the user in, then a verified ID token creates a backend device session and mobile session becomes active.
- Given an authenticated request with a revoked, unknown, or inactive backend session ID, then the backend rejects it even if the Firebase ID token is valid.
- Given an authenticated user, when user updates profile fields, then changes persist and are returned by profile endpoint.
- Given a teacher-only circle endpoint, when a student, supervisor, or non-member calls it, then the request is rejected with the documented authorization error.

Edge cases:
- Duplicate email registration attempt.
- Expired Firebase ID token; Firebase SDK refreshes it before retry.
- Missing, unknown, revoked, or 30-day-inactive backend device session.
- Current-device logout versus deferred logout-all-devices.
- Missing required profile fields on first-time profile completion.

Functional requirements:
- FR-001 Flutter Firebase Auth must register unique email/password identities and handle password validation; the Go backend must never receive or store passwords.
- FR-002 The Go backend must verify Firebase ID tokens, provision local users, and create an opaque per-device backend session without returning Firebase tokens.
- FR-003 The backend must reject missing, revoked, unknown, and 30-day-inactive device sessions; Firebase owns ID-token refresh, refresh-token rotation, and reuse detection.
- FR-004 System must enforce per-circle authorization from `circle_members`; self-registration must not create a circle role.
- FR-005 System must provide profile create/read/update for authenticated users.
- FR-006 Mobile app must support register, login, logout, and profile edit flows.

Key entities:
- User: id, firebase_uid, email, status, created_at, updated_at.
- Profile: user_id, full_name, display_name, bio, country, avatar_url, updated_at.
- UserSession: id, user_id, device_name, last_activity_at, expires_at, revoked_at, created_at. Never stores Firebase refresh tokens.
- CircleMember: circle_id, user_id, role (student|teacher|supervisor), status, joined_at. Circle creation assigns the creator teacher; invite joining assigns student; only the teacher may promote/revoke supervisor.

Success criteria:
- SC-001 95% of successful logins complete in under 2 seconds.
- SC-002 100% of protected endpoints reject missing or invalid tokens.
- SC-003 90% of users complete registration and first profile update without support.
- SC-004 Passwords are never stored or returned in plaintext.

Assumptions and dependencies:
- Backend stack is Go with PostgreSQL.
- Mobile stack is Flutter.
- Firebase ID tokens identify users; the backend session ID binds and revokes a current device. No backend-issued access or refresh tokens.
- Existing project structure under backend/ and mobile/ is used.

Response contract: concise only, no extended reasoning, max 8 bullets.
```

### 1.3 Clarify

```text
Run /speckit.clarify now.

Rules:
- Ask one question at a time.
- Recommend best option first.
- Prefer multiple-choice.
- Maximum 5 questions.
- Apply accepted answers directly to spec.md.
- Final output: questions answered count, sections updated, and next command.

Response contract: concise only, no extended reasoning, max 8 bullets.
```

### 1.4 Plan

```text
Run /speckit.plan now.

Implementation context:
- Backend: Go
- Mobile: Flutter
- Database: PostgreSQL
- Realtime/media: LiveKit
- Auth/notifications: Firebase

Architecture constraints:
- Keep API contract-first in docs/contracts.
- Before coding, require the feature specification, canonical OpenAPI contract, architecture, migration plan, and implementation plan to agree; resolve mismatches through clarification, ADRs, and replanning.
- Preserve backward compatibility for API changes.
- Security baseline: authentication, authorization, validation, rate limits, audit logging.
- Reliability baseline: retries, timeouts, idempotency, observability.

Testing expectations:
- Go: unit, integration, contract tests.
- Flutter: widget and integration tests.

Required outputs:
- research.md
- data-model.md
- contracts/*
- quickstart.md

Response contract: concise only, no extended reasoning, max 8 bullets.
```

### 1.5 Tasks

```text
Run /speckit.tasks now.

Task rules:
- Organize by user story priority P1 -> P2 -> P3.
- Strict format: - [ ] T### [P?] [US#] Description with exact file path.
- Clear dependencies and execution order.
- Mark [P] only when truly parallel.
- `[P]` means tasks have no incomplete dependency and no shared-file conflict; otherwise they must be sequential.
- Every acceptance criterion and buildable success criterion must map to at least one implementation task and at least one test task.
- Require contract tests, migration up/down rollback tests, rate-limit tests, RBAC denial tests, backend-session revocation tests, and response-safety tests when this feature changes the relevant behavior.
- Every P0/P1 analysis finding must become a current-phase task or an explicitly deferred task with an owner, rationale, and target phase.
- Explicit MVP scope as US1.

Output:
- Total tasks
- Tasks per phase and story
- Critical dependency chain
- Next command

Response contract: concise only, no extended reasoning, max 8 bullets.
```

### 1.6 Analyze

```text
Run /speckit.analyze now in read-only mode.

Focus:
- Spec/plan/tasks consistency
- Requirement-to-task coverage gaps
- Constitution violations
- Ambiguities, conflicts, duplication
- Security, reliability, performance coverage
- Confirm the specification, canonical OpenAPI contract, architecture, migration plan, and implementation plan agree before implementation.

Output:
- Compact findings table by severity
- Coverage summary
- Top 5 remediation actions

Response contract: concise only, no extended reasoning, max 8 bullets.
```

### 1.7 Implement

Before /speckit.implement, provide task branch commands, then run implementation. (reusable for other features)

```text
1. Read tasks.md.
2. Identify the selected phase.
3. Show the exact branch command.
4. Wait for the branch to be ready.
5. Run /speckit.implement for this phase.
```

Then the final prompt:

```text
Before /speckit.implement, provide task branch commands, then run implementation.

Feature: Authentication, Roles, and User Profile
Phase number: 4
Phase name: Secure Account Access
Branch: hql-001-ph04-secure-account-access

Authoritative workflow:
- Read `AGENTS.md`, `.specify/memory/constitution.md`, `DEVELOPMENT.md`, `docs/engineering/collaboration/AGENT_WORKFLOW_HARNESS.md`, and the current feature's `spec.md`, `plan.md`, contracts, and `tasks.md` before implementation.
- Follow the authority and conflict-resolution order in `AGENT_WORKFLOW_HARNESS.md`; stop and report conflicting approved artifacts instead of guessing.
- Spec-Kit owns scope, requirements, plan, contracts, and tasks. Superpowers supplies implementation discipline only.
- Do not run Superpowers `brainstorming` or `writing-plans` to recreate approved Spec-Kit artifacts.

Task branch command pattern:
git checkout -b hql-001-ph04-secure-account-access
- Check the current branch first. Reuse `hql-001-ph04-secure-account-access` if it already exists; create it only when absent.

Implementation rules:
- Execute phase-by-phase from tasks.md.
- Respect dependencies and [P].
- Mark completed tasks as [X].
- Stop on blocker and return actionable error only.
- Keep contracts and docs synchronized with behavior changes.
- Follow Section 6 of `AGENT_WORKFLOW_HARNESS.md`: pass paths and task IDs instead of full documents, use one implementer and one Tech Lead reviewer per coherent batch by default, run focused tests during development, and run full applicable gates once before completion.

Agent routing rules:
- Use `senior-golang-developer` for Go/backend tasks (`backend/**`, Go files, migrations, backend API wiring).
- Use `senior-flutter-mobile-engineer` for Flutter/mobile tasks (`mobile/**`, Dart files, widget/integration mobile flows).
- Do not run all agents for every task; use one primary agent per task.
- If a task touches both backend and mobile, split it into two sequential subtasks and assign each to the correct agent.
- Cross-stack subtasks must preserve the same canonical API contract, error behavior, and test expectations.
- Use the role-definition and capability fallbacks in `AGENT_WORKFLOW_HARNESS.md` when the active harness does not expose a requested agent or skill.

Superpowers execution rules:
- Use `superpowers:test-driven-development` before implementation code for each behavior change or bug fix.
- Use `superpowers:systematic-debugging` only when a bug, failed test, or unexpected behavior occurs, before proposing a fix.
- Use `superpowers:subagent-driven-development` only when the approved tasks justify its overhead; its implementer/reviewer loop is sequential.
- Use `superpowers:dispatching-parallel-agents` only outside subagent-driven development and only for independent `[P]` tasks with disjoint file ownership and no ordering dependency.
- Use `superpowers:requesting-code-review` to route one coherent review package to `tech-lead`.
- Use `superpowers:verification-before-completion` with fresh command output before marking a task or phase complete.
- Reuse the Spec-Kit phase branch or existing managed worktree; do not create a duplicate or nested worktree automatically.

Skill rules:
- After each implementation/refactor/fix task, run `$clean-code-guard` on changed production code before marking the task complete.
- For every test-writing or test-editing task, run `$test-guard` before marking the task complete.
- Run `$docs-guard` whenever OpenAPI, WebSocket contracts, specifications, migrations, ADRs, or architecture documents change.
- If a task includes production code, tests, and documentation, run `$clean-code-guard`, `$test-guard`, then `$docs-guard` as applicable.
- For implementation/status turns, use `/steno-mode` (default brief) to keep responses compact; use `/steno-mode machine` only when precision stays intact and the output is strictly internal.
- Do not use `steno-mode` for polished docs, tutorials, stakeholder copy, or clarifications that need full readability.
- If a named guard is unavailable, follow the harness capability fallback: read the matching repository `SKILL.md`, apply its checklist manually, and report the fallback accurately.
- Authentication, RBAC, Firebase integration, token/session handling, and account deletion or revocation require Tech Lead security review before their tasks are marked complete; Karim's mandatory manual review remains a hard gate before merge.

Output per phase:
- Progress
- Changed files
- Next task
- No long logs unless failure

Response contract: concise only, no extended reasoning, max 8 bullets.
```

---

## Feature 2: Session Discovery, Booking, and Queue

### 2.1 Create feature branch

```bash
git checkout -b 002-session-discovery-booking-queue
```

### 2.2 Run specification

```text
For this feature, first confirm the git command I already ran, then run /speckit.specify.

Feature name: Session Discovery, Booking, and Queue

Business goals:
- Students can discover available halaqah sessions quickly.
- Students can reserve a seat or join queue when session is full.
- Teachers can manage session capacity and queue progression.

Primary users:
- Student
- Teacher

In scope:
- Session listing and filtering (topic, teacher, time, level).
- Session details with seat availability.
- Booking flow with confirmation.
- Waitlist/queue join and leave flow.
- Queue promotion when seats open.
- Teacher controls for capacity and attendance state.
- Mobile screens for discover, detail, booking, queue status.

Out of scope:
- Payments and paid subscriptions.
- Multi-tenant organization support.
- Recommendation engine personalization.

User stories by priority:
1) P1: As a student, I can find a session and book an available seat.
2) P2: As a student, I can join a waitlist when a session is full and get promoted fairly.
3) P3: As a teacher, I can manage session capacity and view booking/queue states.

Acceptance scenarios:
- Given sessions exist, when student filters by topic and time, then matching sessions are returned with availability.
- Given a session has available seats, when student books, then booking is confirmed and capacity decrements.
- Given a session is full, when student joins queue, then student receives queue position.
- Given a seat becomes available, when promotion runs, then next queued student is promoted and notified.

Edge cases:
- Two students booking last seat at the same time.
- Student cancels booking after queue exists.
- Student attempts duplicate queue entry.
- Teacher reduces capacity below active bookings.

Functional requirements:
- FR-001 System must provide session search and filtering with availability metadata.
- FR-002 System must create booking atomically and prevent overbooking.
- FR-003 System must support queue join/leave and queue position retrieval.
- FR-004 System must promote queued students fairly when seats become available.
- FR-005 System must expose teacher controls for capacity and booking state management.
- FR-006 Mobile app must show live booking and queue status for the student.

Key entities:
- Session: id, teacher_id, topic, level, start_time, end_time, capacity, status.
- Booking: id, session_id, student_id, state, booked_at, cancelled_at.
- QueueEntry: id, session_id, student_id, position, state, created_at, promoted_at.
- CapacityEvent: id, session_id, old_capacity, new_capacity, reason, changed_by, changed_at.

Success criteria:
- SC-001 99% of bookings avoid overbooking under concurrent requests.
- SC-002 Session search results return in under 2 seconds for standard filters.
- SC-003 Queue promotion order is deterministic and auditable.
- SC-004 90% of users can complete discover-to-book flow without support.

Assumptions and dependencies:
- PostgreSQL transactions are used for booking and queue consistency.
- Notification events can be sent through existing notification channel.
- Existing user roles from Feature 1 are available.

Response contract: concise only, no extended reasoning, max 8 bullets.
```

### 2.3 Clarify

```text
Run /speckit.clarify now.

Rules:
- Ask one question at a time.
- Recommend best option first.
- Prefer multiple-choice.
- Maximum 5 questions.
- Apply accepted answers directly to spec.md.
- Final output: questions answered count, sections updated, and next command.

Response contract: concise only, no extended reasoning, max 8 bullets.
```

### 2.4 Plan

```text
Run /speckit.plan now.

Implementation context:
- Backend: Go
- Mobile: Flutter
- Database: PostgreSQL
- Realtime/media: LiveKit
- Auth/notifications: Firebase

Architecture constraints:
- Keep API contract-first in docs/contracts.
- Preserve backward compatibility for API changes.
- Security baseline: authentication, authorization, validation, rate limits, audit logging.
- Reliability baseline: retries, timeouts, idempotency, observability.
- Concurrency correctness for booking and queue operations is mandatory.

Testing expectations:
- Go: unit, integration, contract tests with concurrency cases.
- Flutter: widget and integration tests for booking and queue UI.

Required outputs:
- research.md
- data-model.md
- contracts/*
- quickstart.md

Response contract: concise only, no extended reasoning, max 8 bullets.
```

### 2.5 Tasks

```text
Run /speckit.tasks now.

Task rules:
- Organize by user story priority P1 -> P2 -> P3.
- Strict format: - [ ] T### [P?] [US#] Description with exact file path.
- Clear dependencies and execution order.
- Mark [P] only when truly parallel.
- Include concurrency and transaction test tasks.
- Explicit MVP scope as US1.

Output:
- Total tasks
- Tasks per phase and story
- Critical dependency chain
- Next command

Response contract: concise only, no extended reasoning, max 8 bullets.
```

### 2.6 Analyze

```text
Run /speckit.analyze now in read-only mode.

Focus:
- Spec/plan/tasks consistency
- Requirement-to-task coverage gaps
- Constitution violations
- Ambiguities, conflicts, duplication
- Security, reliability, performance, concurrency coverage

Output:
- Compact findings table by severity
- Coverage summary
- Top 5 remediation actions

Response contract: concise only, no extended reasoning, max 8 bullets.
```

### 2.7 Implement

Before /speckit.implement, provide task branch commands, then run implementation. (reusable for other features)

```text
1. Read tasks.md.
2. Identify the selected phase.
3. Show the exact branch command.
4. Wait for the branch to be ready.
5. Run /speckit.implement for this phase.
```

Then the final prompt:

```text
Before /speckit.implement, provide task branch commands, then run implementation.

Feature: Session Discovery, Booking, and Queue
Phase number: 1
Phase name: Session Discovery, Booking, and Queue
Branch: hql-002-ph01-foundational

Task branch command pattern:
git checkout -b hql-002-<phase-number>-<phase-name>

Implementation rules:
- Execute phase-by-phase from tasks.md.
- Respect dependencies and [P].
- Mark completed tasks as [X].
- Stop on blocker and return actionable error only.
- Keep contracts and docs synchronized with behavior changes.

Agent routing rules:
- Use `senior-golang-developer` for Go/backend tasks (`backend/**`, Go files, migrations, backend API wiring).
- Use `senior-flutter-mobile-engineer` for Flutter/mobile tasks (`mobile/**`, Dart files, widget/integration mobile flows).
- Do not run all agents for every task; use one primary agent per task.
- If a task touches both backend and mobile, split it into two sequential subtasks and assign each to the correct agent.

Skill rules:
- After each implementation/refactor/fix task, run `$clean-code-guard` on changed production code before marking the task complete.
- For every test-writing or test-editing task, run `$test-guard` before marking the task complete.
- Run `$docs-guard` whenever OpenAPI, WebSocket contracts, specifications, migrations, ADRs, or architecture documents change.
- If a task includes production code, tests, and documentation, run `$clean-code-guard`, `$test-guard`, then `$docs-guard` as applicable.
- For implementation/status turns, use `/steno-mode` (default brief) to keep responses compact; use `/steno-mode machine` only when precision stays intact and the output is strictly internal.
- Do not use `steno-mode` for polished docs, tutorials, stakeholder copy, or clarifications that need full readability.
- If you want slash-command support, register steno mode as a Copilot skill under `.github/skills/steno-mode/SKILL.md`; otherwise apply its shorthand rules manually.
- Authentication, RBAC, Firebase integration, token/session handling, and account deletion or revocation require a mandatory security review before their tasks are marked complete.

Output per phase:
- Progress
- Changed files
- Next task
- No long logs unless failure

Response contract: concise only, no extended reasoning, max 8 bullets.
```
