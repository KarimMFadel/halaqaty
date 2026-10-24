# Feature Specification: Notification System

**Feature Branch**: `008-notification-system`  
**Created**: 2026-09-30  
**Status**: Draft  
**Input**: F-008 baseline scope approved by Karim in this conversation, plus existing notification obligations audited from F-001 through F-007, `JOURNEY.md`, and the current event contracts. Source requirements remain owned by their originating product features.

## User Scenarios & Testing

### User Story 1 - Receive Approved Event Notifications (Priority: P1)

As a Halaqaty user, I receive relevant notices about sessions, messages, grades, and queue turns while using the app or when it is in the background or closed.

**Why this priority**: Timely delivery is the core value of F-008 and several other approved features depend on it.

**Independent Test**: Exercise every event-matrix row with the app foregrounded, backgrounded, and closed; confirm channel behavior, recipient eligibility, and navigation without duplicating source-feature data.

**Acceptance Scenarios**:

1. **Given** an eligible approved event, **When** the app is foregrounded, **Then** the user sees the in-app behavior listed for that event.
2. **Given** an eligible approved event with push enabled for the user's app state and event type, **When** the app is backgrounded or closed, **Then** the user receives the event's approved push behavior.
3. **Given** a grade is posted, **When** the student opens its notification, **Then** the related recitation log opens, as required by F-007.
4. **Given** an F-006 reminder becomes cancelled or superseded, **When** reminder eligibility is evaluated, **Then** the reminder is not sent.
5. **Given** a student joins a live session, **When** the teacher is eligible for the notice, **Then** the teacher receives the in-app notice and no push is sent for that matrix row.
6. **Given** an eligible student joins a circle, **When** the membership succeeds, **Then** the teacher receives the push notice specified by Journey T-07.
7. **Given** a circle is archived, **When** the archive transition succeeds, **Then** all circle members are notified through FCM as stated in the F-002 lifecycle.
8. **Given** a session starts or ends, **When** the app is connected, **Then** eligible members receive the existing metadata-only F-005 WebSocket event; when backgrounded or closed, F-008 creates the corresponding history item and sends its approved generic FCM projection subject to preference, quiet-hours, and source-eligibility checks.
9. **Given** an authorized inviter sends an approved role-bound invitation to a named recipient, **When** that invitation is committed, **Then** the named recipient gets one F-008 history item and the matrix-approved notification channels, without changing invitation or membership state.

### User Story 2 - Control Notification Types and Quiet Hours (Priority: P1)

As a user, I can manage the notification types that are configurable and choose quiet hours, while types marked always on remain enabled.

**Why this priority**: Users need control over interruptions, and required notices must not become silently unavailable through a preference toggle.

**Independent Test**: Attempt each allowed and disallowed preference change, then exercise delivery inside and outside a user's quiet hours.

**Acceptance Scenarios**:

1. **Given** a type is marked configurable in the matrix, **When** the user turns that type off, **Then** F-008 mutes push and foreground notification alerts for that type while retaining history and unread state; the source feature's content and delivery remain unaffected, and a suppressed push is not replayed when the type is re-enabled.
2. **Given** a type is marked always on, **When** the user views its preference, **Then** it cannot be turned off.
3. **Given** the user sets quiet hours, **When** an event occurs during or outside that interval, **Then** push and sound are suppressed during the interval using the profile IANA timezone as the account-wide clock, while history/unread and foreground in-app history remain; suppressed pushes are not sent later. The default interval is 10pm–7am.

### User Story 3 - Review Notification History and Unread Count (Priority: P1)

As a user, I can review recent notifications and see the exact unread count in the app; the native app-icon badge follows that count where OS and launcher support and user settings permit.

**Why this priority**: A short durable history helps users recover notices they could not act on immediately.

**Independent Test**: Create notices across the 30-day boundary and with read/unread state, then compare history and badge behavior.

**Acceptance Scenarios**:

1. **Given** a notice was created within the last 30 days, **When** the user opens notification history, **Then** it is available to that user.
2. **Given** a notice is older than 30 days, **When** the user opens notification history, **Then** it is no longer available there.
3. **Given** the user opens a notification, **When** that item is viewed, **Then** that item is marked read; opening the history list alone does not mark items read.
4. **Given** the user chooses “Mark all as read”, **When** the action completes, **Then** all current notices are marked read and the badge reflects the remaining unread count.

### User Story 4 - Stay Informed About Circle Changes (Priority: P1)

As a circle member or teacher, I receive the appropriate notice when someone joins a circle, a circle is archived, or a live session starts or ends.

**Why this priority**: These notices already appear in the F-002/F-005 product journeys and lifecycle descriptions, even though they are missing from the F-008 matrix.

**Independent Test**: Exercise successful circle-join/archive transitions and session start/end while eligible recipients are connected or disconnected; verify one history item per source event, approved channel behavior, global preferences, and no duplicate lifecycle delivery.

**Acceptance Scenarios**:

1. **Given** a student successfully joins a circle, **When** the join completes, **Then** the teacher receives the push notice described in Journey T-07.
2. **Given** a teacher archives a circle, **When** archival succeeds, **Then** all members are notified through FCM as required by the F-002 lifecycle.
3. **Given** a session starts or ends, **When** a user is an authorized subscriber or participant, **Then** the existing `session.started` or `session.ended` metadata event remains the foreground source; F-005 never sends media endpoints, credentials, or room references in notification metadata.

### User Story 5 - Notify Members for Manager Account Deletion (Priority: P1)

As a circle member, I receive the required notice when a manager account deletion proceeds under the approved supervisor-handoff and circle-archive policy.

**Why this priority**: OQ-006 and ADR-024 keep manager deletion blocked until the F-008 notification path exists and require a designated supervisor before circle archival; there is no automatic supervisor transfer.

**Independent Test**: Exercise the approved deletion preconditions and confirm members receive the required notices while the account/circle outcome follows OQ-006 and ADR-024.

**Acceptance Scenarios**:

1. **Given** a manager deletion request has no designated supervisor for an owned active circle, **When** deletion is attempted, **Then** it is blocked without mutating the account or circle.
2. **Given** a designated supervisor exists for every owned active circle, **When** the manager-deletion flow is enabled, **Then** each circle archive, account closure, and required per-member durable notice/history intent commits atomically, with no automatic transfer.
3. **Given** a required supervisor or durable notification intent cannot be committed, **When** manager deletion is attempted, **Then** the request is rejected without changing the account or any circle. A transient FCM failure after the durable handoff commits does not roll back closure or archival; delivery remains subject to the durable retry policy and is not represented as handset receipt.

### Edge Cases

- A recipient has no registered device, multiple devices, or an invalid/expired device token.
- The app is offline, reconnects, or moves between foreground and background while a notice is pending.
- A user changes a preference or quiet-hours interval while notices are pending.
- F-004 has already delivered a message over foreground chat realtime when F-008 updates notification history or the bell.
- An F-006 occurrence is cancelled or superseded after reminder preparation but before send eligibility is checked.
- A notification event is repeated, delayed, or arrives out of order.
- Manager deletion involves multiple circles or a member whose eligibility changes during the flow.
- Push permission is denied; the app remains usable and the existing journey's gentle re-prompt occurs after the first session ends.

## Requirements

### Functional Requirements

- **FR-001**: The system MUST support all event types and channel/toggle behavior in the Notification Matrix below and must reconcile the additional existing product obligations in the cross-feature inventory; it MUST NOT send notifications for unrelated or unapproved events.
- **FR-002**: The system MUST provide an in-app notification bell/history for the authenticated recipient and support foreground updates over the approved in-app realtime boundary.
- **FR-003**: The system MUST deliver approved push notifications through FCM when the app is backgrounded or closed, subject to each event's approved channels, user preferences, and quiet-hours policy. It MUST consume authorized source events and MUST NOT create a second source-of-truth transition. FCM acceptance MUST NOT be described as proof of handset receipt or user read.
- **FR-004**: The system MUST provide global per-user, per-notification-type preferences in Settings. Types marked configurable may be turned off and default to ON; types marked always on may not be turned off. Turning a configurable type off MUST mute its F-008 push and foreground alert presentation while retaining its notification history and unread state; it MUST NOT suppress the underlying source feature's behavior or durable content (including F-004 chat messages). Student-circle-join, circle-archive, session-started, and session-ended notices are configurable and default ON. F-002 determines circle eligibility; F-008 applies each recipient's global event-type preference across their circles.
- **FR-005**: The system MUST support user-defined quiet hours, with 10pm–7am as the default, evaluated using the user's stored profile IANA timezone as the single account-wide local clock across devices. Quiet hours suppress push/sound, including for always-on types, but MUST NOT suppress in-app/history creation or clear unread state. A push suppressed during quiet hours is dropped for that event occurrence; it is not sent later.
- **FR-006**: The system MUST retain and display notification history for the last 30 days.
- **FR-007**: The system MUST maintain an authoritative unread count for each user and display that count consistently in the in-app notification UI after synchronization. Where the platform and launcher support app-icon badges and badge display is enabled, the app MUST synchronize the icon badge to the unread count after successful read-state/count updates and when the app next reconciles state. Unsupported or disabled native badges MUST NOT affect in-app history or unread behavior. Opening a notice marks that notice read; opening the history list alone does not; an explicit “Mark all as read” action marks all current notices read.
- **FR-008**: The system MUST support the session reminder intervals listed in the matrix and MUST use F-006's authoritative reminder-eligibility boundary. Cancelled or superseded occurrences MUST NOT generate stale reminders.
- **FR-009**: F-008 MUST own notification trigger creation, preferences, and FCM delivery for background/closed-app events. F-004 MUST remain the owner of durable chat messages and foreground chat WebSocket delivery; F-008 MUST NOT duplicate chat storage or replace that foreground message delivery. F-004 creates no FCM trigger.
- **FR-010**: Grade notifications MUST deep-link to the related recitation log as specified in the F-007 product dependency entry.
- **FR-011**: Queue notifications MUST preserve the two distinct matrix events: “You're next!” and “Your turn!”. They MUST project from F-003's `queue.next_soon` (next waiting student after order/selection changes) and `queue.your_turn` (selected entry becomes `reciting`) events and retain their stable `event_id` for deduplication. F-003 owns queue state and at-least-once WebSocket delivery; it sends no FCM.
- **FR-012**: Circle invitation notifications are in F-008 scope and fire when an authorized inviter sends an approved role-bound invitation to a named recipient. F-008 MUST create that recipient's history and delivery projection from the committed invitation transition. F-008 MUST NOT change invitation or membership state, or infer the trigger from reusable code sharing/rotation or circle joining. Resolve any missing recipient-specific source seam as an F-008 integration task within the existing F-002 authorization and role rules; do not move notification behavior out of F-008.
- **FR-013**: The “Student joined session” notice MUST be in-app only and directed to the eligible teacher, as shown in the matrix; F-008 MUST NOT replace F-005's session/presence facts. The separate “student joined circle” push notice in Journey T-07 MUST also be covered and MUST NOT be conflated with joining a live session.
- **FR-014**: Manager-account deletion notices MUST follow OQ-006 and ADR-024: require a designated supervisor for each owned active circle, do not automatically transfer supervision, archive the circle, and create durable notice/history intents for all eligible members atomically with the archive/deletion transition. This required notice MUST bypass the member's ordinary circle-archive preference and MUST still follow quiet-hours policy. If any required intent cannot be committed, the mutation MUST fail without changing the account or circle. The existing manager-deletion block remains until this notification path is deployed and operational. FCM acceptance or handset receipt MUST NOT be treated as proof that a member saw a notice.
- **FR-015**: F-008 MUST reuse F-001 authentication and registered per-device FCM-token ownership. Its authenticated POST registration and DELETE single-token detachment operations remain scoped to the current user; raw tokens MUST NOT appear in responses or logs.
- **FR-016**: F-008 MUST cover existing notification obligations referenced in F-001 through F-007 product/spec artifacts, including circle join/archive, role-bound invitation sends to named recipients, live-session lifecycle, queue turns, chat messages, schedule reminders, grades, and manager deletion. The cross-feature inventory below records source ownership and known behavior without changing those features' authoritative state.
- **FR-017**: When push permission is denied, the app MUST remain fully functional. The current `JOURNEY.md` behavior is to show a gentle permission prompt after the first session ends; this is a notification onboarding state, not a new event type.
- **FR-018**: Event payloads, API/schema designs, exact platform delivery guarantees, and manager-deletion operational evidence MUST NOT be inferred where this specification leaves them undecided; resolve them through the plan/ADR/contracts workflow. Decisions already recorded in ADR-027 and the canonical contracts are binding for this feature.
- **FR-019**: Notification projections MUST NOT include media credentials, media endpoints, room references, provider identifiers, or URLs carrying media material, consistent with the F-003/F-005 session and queue boundaries.
- **FR-020**: Notification recipients MUST follow the current recipient and authorization rules of their source feature, including circle membership and F-004 direct-message eligibility; F-008 MUST NOT deliver to an ineligible recipient or expose another user's private commitment/content.
- **FR-021**: FCM registration MUST accept Android, iOS, and web token platforms. On logout or account switch, the client MUST request authenticated removal of only its current token before local token deletion/sign-out; a missing token row is an idempotent success. Pending push intents are awakened on registration and rechecked for expiry, current preference, quiet hours, and source eligibility before dispatch.

### Notification Matrix

| Event | Push (FCM) | In-App (WS) | Configurable OFF? |
|---|---|---|---|
| Session reminder (1hr/30min/15min/5min) | Yes | Yes | Yes |
| New group message | Yes | Yes | Yes |
| New private message | Yes | Yes | No (always on) |
| Grade posted | Yes | Yes | Yes |
| Circle invitation (role-bound, addressed recipient) | Yes | Yes | No |
| Queue: “You're next!” | Yes | Yes | No |
| Queue: “Your turn!” | Yes | Yes | No |
| Student joined session (to teacher) | No | Yes | Yes |
| Student joined circle (to eligible teacher) | Yes | Yes | Yes (default ON; global) |
| Circle archived (to eligible members) | Yes | Yes | Yes (default ON; global) |
| Session started (to authorized subscribers) | Yes | Yes | Yes (default ON; global) |
| Session ended (to joined participants) | Yes | Yes | Yes (default ON; global) |

### Additional Cross-Feature Notification Obligations

These requirements already appear outside F-008. F-008 consolidates notification delivery/history/preferences while each source feature retains authority over its underlying state.

| Existing event/behavior | Source and known behavior | Channel/state still to reconcile |
|---|---|---|
| Student joins a circle → teacher | Journey T-07 says a successful student join notifies the teacher by push, with text `[Student Name] has joined [Circle Name]`. | Decision: create one F-008 history item for the eligible teacher; it contributes to unread state independently of push outcome. The teacher's global student-circle-join preference controls delivery across circles. |
| Circle archived → all members | F-002 lifecycle says all members are notified via FCM when a teacher archives the circle. | Decision: create one F-008 history item per eligible member; it contributes to unread state independently of push outcome. Each recipient's global, configurable circle-archive preference controls delivery. |
| Session started → authorized circle subscribers | F-005 broadcasts metadata-only `session.started`; JOURNEY says members see “Session started” and choose Join. No media endpoint, credential, or room reference is included. | Decision: F-008 sends an FCM projection when the app is backgrounded or closed. F-005 remains the source event. |
| Session ended → joined participants | JOURNEY says participants receive metadata-only `session.ended`; it assigns general push delivery to F-008. | Decision: F-008 sends a background/closed-app push projection and creates a history item for joined participants; their global preference and quiet-hours policy apply. |
| Push permission denied | JOURNEY says the app remains fully usable and prompts gently after the first session ends. | Preserve the existing behavior; detailed platform permission handling belongs in planning. |
| Teacher controls for notices | JOURNEY's circle settings include “Notify me when a student joins” and “Notify students 15 min before session.” | Decision: avoid duplicate controls. “Notify me when a student joins” maps to the teacher's global F-008 preference, rather than a separate per-circle setting. The 15-minute toggle remains a circle/source-side reminder gate, followed by each recipient's global F-008 preference. F-006 eligibility remains authoritative. |

### Source Feature Ownership and Dependencies

| Existing feature | Existing source of truth | F-008 responsibility |
|---|---|---|
| F-001 Auth/Profile | Authenticated user and per-device token registration | Deliver only to the intended authenticated recipient's registered devices; do not redefine identity or token ownership. |
| F-002 Circle Management | Circle membership, invitation role, designated supervisor, archive policy, and authorized role-bound invitation sends | Notify the eligible teacher on student circle-join and all members on archive. F-008 owns the addressed-invitation notification projection and integrates it at invitation send under F-002's existing authorization and role rules. Do not infer it from code sharing or join, or change membership, invitation, role, or supervisor state. |
| F-003 Recitation Queue | Queue state, `queue.next_soon`, and `queue.your_turn`; event IDs, at-least-once WebSocket delivery, and recovery are defined in its contract | Project both event types to FCM/history; do not create or mutate queue state. |
| F-004 Real-Time Chat | Durable messages and foreground chat WebSocket delivery | Own background/closed-state chat notification triggers, preferences, FCM delivery, and notification history; do not duplicate message persistence or foreground chat delivery. |
| F-005 Live Sessions | Live lifecycle, participant-presence facts, and metadata-only `session.started`/`session.ended` events | Deliver the matrix's teacher in-app participant-join notice; project `session.started` and `session.ended` to FCM when backgrounded/closed and create history items for their eligible recipients; do not change session facts or expose media credentials. |
| F-006 Schedule & Calendar | Current occurrence/revision eligibility; cancelled occurrences and non-current governing revisions are ineligible | Send eligible reminders at 1 hour, 30, 15, or 5 minutes before start; recheck current eligibility and never send stale reminders. |
| F-007 Student Progress | Grade and recitation log | Notify about a posted grade and link to its recitation log. |
| F-008 Notification System | Notification history, preferences, quiet hours, unread count, and approved delivery policy | Own those notification concerns across the channels and events defined above. |

### Key Entities

- **Notification**: A user-facing notice for one of the approved matrix events or explicitly listed cross-feature obligations, associated with its intended recipient and source event. Its precise content, state, and delivery fields are not designed in this phase.
- **Notification preference**: A user's choice for each configurable notification type and their quiet-hours interval; configurable types default on, while always-on types cannot be disabled. Current type preference is checked before first push dispatch and controls foreground alert presentation; a muted alert keeps its history item and unread state.
- **Registered device token**: Existing F-001 per-device FCM registration associated with an authenticated user; F-008 consumes this boundary without redefining it.
- **Reminder eligibility**: F-006's authoritative determination that a current planned occurrence may produce one of the listed reminders.
- **Unread count**: The server-authoritative count represented exactly in the in-app notification UI and synchronized to native app-icon badges where supported and enabled. Opening one notice marks it read; opening the history list does not; an explicit “Mark all as read” action marks all current notices read.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Every matrix row and reconciled cross-feature notification obligation has acceptance evidence for each applicable channel and foreground/background/closed app state.
- **SC-002**: Users can access notices created within the last 30 days, and notices older than that window are absent from history.
- **SC-003**: After each successful read-state transition is synchronized, the in-app unread count matches the authoritative unread set; on supported, enabled devices, the native app-icon badge converges to that count after reconciliation.
- **SC-004**: No reminder is sent for a cancelled or superseded F-006 occurrence.
- **SC-005**: The manager-deletion journey cannot mutate an account or circle without the OQ-006 designated-supervisor and notification-path prerequisites, and its required member notices follow the approved policy.
- **SC-006**: Notification delivery does not duplicate F-004 durable messages or replace its foreground chat realtime behavior.

## Assumptions

- The F-008 matrix in `FEATURES.md` is the approved baseline. This audit also found explicit notification obligations in F-002, F-005, F-006, and `JOURNEY.md`, now consolidated or reconciled here.
- Existing product features remain authoritative for identity, circle/invitation eligibility, queue turns, chat messages, live-session presence, schedule reminder eligibility, and grade records.
- “Last 30 days” is the notification-history window. The approved schema, payload minimization, APIs, and delivery semantics are detailed in the plan, ADR-027, and canonical contracts.
- Karim approved continuing through the remaining Spec-Kit phases in this conversation. Implementation remains bounded by this specification, the plan/contracts/ADR decisions, required verification, and the mandatory Karim review of manager-account deletion.

## Clarifications

- **Session-start push — 2026-09-30:** F-008 sends an FCM projection of the existing F-005 `session.started` event when the app is backgrounded or closed. F-005 remains the event source; no new trigger or event is introduced.
- **Circle notices in history — 2026-09-30:** Successful student-circle-join and circle-archive notices each create one recipient-scoped F-008 history item and contribute to unread state independent of FCM delivery. Push and history are channels for one logical notice, not separate notice records.
- **Circle-archive preference — 2026-09-30:** Circle-archive notices are configurable and default ON; recipients may turn them off.
- **Circle notification preference scope — 2026-10-01:** Student-circle-join and circle-archive preferences are global per-user event-type settings across circles. Circle membership and event eligibility remain authoritative in F-002. “Notify me when a student joins” uses the same global F-008 setting; it is not a separate per-circle preference.
- **Session-event notifications — 2026-10-01:** Both existing F-005 `session.started` and `session.ended` events create one F-008 history/unread item per eligible recipient. Their global event-type preferences default ON and control delivery; F-008 sends FCM when the app is backgrounded or closed. F-005 remains the event source and eligibility authority.
- **Read state — 2026-09-30:** Opening a notice marks that item read; opening the history list alone does not. An explicit “Mark all as read” action marks all current notices read.
- **Teams-style quiet time and defaults — 2026-10-01:** Use the existing 10pm–7am quiet-time interval in the user's stored profile IANA timezone as the account-wide clock across devices. Quiet time suppresses push/sound, including for always-on types, but does not suppress in-app/history creation or clear unread state. Configurable notification types default on; always-on types cannot be disabled. A suppressed push is dropped for that occurrence, not sent later.
- **Notification content — 2026-10-01:** Use generic lock-screen text by default. Do not place private message bodies, grade details, or media connection information in push payloads; show authorized details in-app after opening the notification.
- **Teacher notification controls — 2026-09-30:** Avoid duplicate controls. “Notify me when a student joins” uses the teacher-recipient's F-008 preference. “Notify students 15 min before session” remains a circle/source-side gate; each recipient's F-008 preference and quiet-time policy are subsequent delivery gates, and F-006 eligibility remains authoritative.
- **Quiet-hours timezone — 2026-10-17:** Use the user's stored profile IANA timezone as the single account-wide local clock across devices. Travel quiet hours follow the profile timezone until the user updates it.
- **Queued push preference — 2026-10-17:** Evaluate the current global type preference immediately before first push dispatch. If the user turned that type off while the intent was pending, suppress the push and preserve history/unread; if it is on at dispatch, it remains eligible subject to the other gates.
- **Preference OFF behavior — 2026-10-17:** A configurable type turned OFF mutes F-008 push and foreground alert presentation while preserving history/unread. It never suppresses the source feature's content or delivery, including F-004 chat messages.
- **Preference-muted unread state — 2026-10-17:** Karim chose A: preference-muted notices remain in history and count as unread; the user preference controls alert presentation, not whether the event exists in history or contributes to the unread count. Re-enabling a type does not replay a push already suppressed at its first dispatch check.
- **Native app-icon badge scope — 2026-10-17:** Karim accepted the Flutter/Architect/Tech Lead recommendation: the in-app unread count is exact and server-authoritative; native icon badge synchronization is required where OS/launcher support and user settings allow it, and best-effort where they do not.
- **No registered FCM token — 2026-10-17:** Karim chose to hold an eligible push intent until a device token registers. Ordinary intents expire no later than 24 hours after the source event; mandatory manager-deletion intents expire no later than 30 days. Earlier source-validity cutoffs apply. Recheck preference, quiet hours, and current source eligibility at token registration and dispatch; history/unread remains independent.
- **Circle invitation notification — 2026-10-17:** Karim accepted notifying the named recipient when an authorized inviter sends a role-bound invitation. F-008 owns recipient history and delivery. Use the committed invitation transition under F-002's role/authorization rules; do not infer the recipient or trigger from code sharing or circle joining.
- **Pending reminder expiry — 2026-10-17:** Per Architect's recommendation, an F-006 reminder expires when the next closer configured offset becomes due; the final offset expires at occurrence start. Recheck current occurrence/version eligibility before dispatch.
- **Logout token detachment and web platform — 2026-10-19:** Karim approved an authenticated server-side unregister operation for immediate per-device detachment before local logout/account switch. Keep `web` as an allowed token platform in F-008; web-client Firebase setup is implementation-dependent, but the backend and persistence contract include it.

- **Q1 — Notification delivery reliability**: Adopt ADR-027's durable, idempotent intent keyed by source event and recipient, maximum five attempts with exponential backoff and jitter, and an operational parking signal; FCM acceptance is not handset receipt.
- **Manager-deletion notice preference override — 2026-10-17:** Karim chose A: the required manager-deletion notice is mandatory and bypasses the member's ordinary circle-archive preference. It remains subject to quiet hours; the durable history/notice intent is still created.
