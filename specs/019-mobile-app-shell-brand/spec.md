# Feature Specification: Mobile App Shell and UI/UX Modernization

**Feature Branch**: `019-mobile-app-shell-brand`  
**Created**: 2026-09-22  
**Status**: Draft  
**Input**: Production Flutter implementation of the approved Halaqaty Calm Contemporary direction across Waves 0–5, with existing product behavior and backend contracts unchanged.

## Scope

F-019 is the single approved umbrella feature for the complete Waves 0–5
mobile modernization. Each wave remains a coherent implementation, review, and
verification batch inside this feature.

The governing sources are:

1. `docs/engineering/design/UI_UX_GOVERNANCE.md`
2. `docs/engineering/design/DESIGN.md`
3. `docs/engineering/design/screen-samples-a-v2.html`
4. Existing Flutter themes, shared components, routes, controllers, API clients,
   widget keys, and localization patterns
5. Approved F-001–F-005 specifications and contracts

The screen-sample HTML is the approved visual reference. It is not executable
product code and cannot create new product data, permissions, authentication,
chat/media behavior, session behavior, account behavior, or backend capability.

Wave 0 and Wave 1 are already substantially implemented. F-019 preserves work
that meets this specification and changes only missing or non-conforming parts.

## Clarifications

### Session 2026-09-22

- Q: Does F-019 cover only the shell or the complete modernization roadmap? →
  A: F-019 is the approved umbrella for Waves 0–5. (Karim, 2026-09-22)
- Q: May the visual reference introduce new backend or role behavior? → A: No.
  Existing F-001–F-005 behavior and contracts remain authoritative.
- Q: How should completed Wave 0 and Wave 1 work be handled? → A: Preserve
  conforming work and implement only verified gaps.
- Q: Is a unified direct-message inbox included? → A: No. The Chats tab may
  list existing circle group chats client-side; direct conversations remain
  reachable only through their existing authorized member flow.
- Q: What wins when the HTML sample and token catalogue differ? → A: The HTML
  governs hierarchy and visual direction only; `DESIGN.md`, the existing theme,
  WCAG AA, and the 48dp minimum govern production values.
- Q: Does "warm ivory" add a new background color? → A: No. The existing
  approved light background/surface roles are the production interpretation
  unless a separately approved token amendment changes them.
- Q: Which typography mapping applies? → A: Preserve bundled Poppins as the
  primary family and Cairo as Arabic glyph fallback. Follow the `DESIGN.md` M3
  role hierarchy using only bundled files: display/headline and body request
  400; title/label/button request 600, which maps exactly to Poppins SemiBold
  and to Cairo Bold (the nearest bundled Arabic emphasis weight). This is the
  approved bundled-font rendering of the design's semantic 500 emphasis; do
  not synthesize or add a 500 font asset. Arabic/Quranic lines use height at
  least 1.5. Sizes and letter spacing remain existing M3 roles; add no font or
  localization framework.
- Q: Does F-019 add desktop navigation or long hero animation? → A: No. The
  four-tab shell remains for supported mobile/tablet widths; F-016 owns desktop.
  Use existing Material transitions and purposeful short motion, with no new
  long/hero animation system.
- Q: What happens when the approved design contains an action whose product or
  backend behavior is not implemented in F-019? → A: Keep the intentionally
  planned action visible and tappable, but route it only to one shared localized
  under-implementation notice. It performs no navigation, API/controller call,
  or state mutation. Unplanned prototype actions and unsupported data remain
  omitted. (Karim, 2026-09-22)
- Q: Which session audio rule is authoritative where the older F-005 spec and
  prototype disagree? → A: The frozen 2026-08-28 decision and current F-003
  specification supersede stale listen-only wording. Authorized students may
  publish audio freely; the queue is voluntary and does not mute until a turn.
- Q: Which currently unreachable screens receive production entry points? →
  A: Circles exposes the existing create/join flows. Circle details exposes the
  existing F-005 ad-hoc session list/create/start/join flows using existing APIs;
  no backend or scheduling capability is added.
- Q: Does Profile become new settings features? → A: No. Keep the existing
  profile form and group it visually; only existing profile fields, preferred
  language, and logout are supported. Notification, appearance, privacy,
  support, and avatar-upload capabilities are excluded.
- Q: How is Arabic/English direction selected? → A: Use the existing `ar`/`en`
  preferred-language value for authenticated UI direction, registration choice
  for new users, and the platform locale as the unauthenticated fallback. This
  does not add languages or broader F-012 localization scope.
- Q: Are parent and institution screens included because `DESIGN.md` names those
  audiences? → A: No. F-019 covers only roles and behavior already provided by
  F-001–F-005; parent/institution experiences remain future scope.

## User Scenarios & Testing

### User Story 1 - Use a Consistent App Shell (Priority: P1)

As an authenticated user, I can move among Home, Circles, Chats, and Profile
through a recognizable Arabic-first shell so that the app has a clear starting
point and no developer-facing navigation bridge.

**Affected roles**: Student, teacher, supervisor.

**Why this priority**: Every other mobile journey depends on the shell, theme,
brand, and navigation foundation.

**Independent Test**: Launch the app in authenticated and unauthenticated states,
verify the branded entry flow and four-tab shell, switch all tabs, return Home to
its root, and repeat in Arabic RTL and English LTR with light and dark themes.

**Acceptance Scenarios**:

1. **Given** authentication is initializing, **When** the app launches, **Then**
   the user sees a branded, semantically labelled loading surface.
2. **Given** an unauthenticated user, **When** initialization completes, **Then**
   the welcome screen offers sign-in and registration without exposing protected
   tabs.
3. **Given** an authenticated user, **When** initialization completes, **Then**
   Home opens inside the four-tab shell and each destination is reachable in one
   tap.
4. **Given** the user has navigated deeper from Home, **When** Home is selected
   again, **Then** Home returns to its root without breaking the other tab stacks.

---

### User Story 2 - Find and Manage Circles Calmly (Priority: P1)

As a circle member or manager, I can discover, open, join, create, and manage
circles through a consistent, low-click hierarchy whose actions still obey my
existing circle role.

**Affected roles**: Student, teacher, supervisor.

**Why this priority**: Circles are the entry point to sessions, chat, membership,
and the product's primary learning context.

**Independent Test**: Exercise discovery, empty/error recovery, circle details,
join/create, member and management actions, and retirement using existing role
fixtures, verifying the same permissions and widget keys before and after the
visual modernization.

**Acceptance Scenarios**:

1. **Given** an authenticated user, **When** they select Circles, **Then** their
   circles and discoverable circles use human-readable content and one clear
   primary action.
2. **Given** an empty, loading, failed, or offline circle load, **When** the state
   is shown, **Then** it explains what happened and offers the appropriate next
   action without losing context.
3. **Given** a circle role, **When** the detail screen opens, **Then** only actions
   already authorized by F-002 are available and no new role behavior appears.
4. **Given** a consequential management action, **When** it succeeds or fails,
   **Then** durable in-context feedback is available and recovery is clear.

---

### User Story 3 - Join and Run a Recitation Session (Priority: P1)

As a student, teacher, or supervisor, I can enter a live audio session, understand
connection and queue state at a glance, and perform my existing session actions
without navigating through a dense control wall.

**Affected roles**: Student, teacher, supervisor.

**Why this priority**: The session and recitation queue are the highest-focus,
most time-sensitive Halaqaty experience.

**Independent Test**: From a circle, open a session, start or join as permitted,
exercise student queue and moderator controls, recover a transient disconnect,
and verify terminal errors, all without changing F-003/F-005 behavior.

**Acceptance Scenarios**:

1. **Given** a valid session, **When** an authorized participant enters, **Then**
   the primary start/join action and audio-only status are immediately clear.
2. **Given** a student in a queue, **When** queue state changes, **Then** position,
   current reciter, turn state, and opt-out feedback are understandable and
   announced accessibly.
3. **Given** a teacher or supervisor, **When** queue and moderation controls are
   available, **Then** the active round and dominant next action are visually
   prioritized while destructive actions remain clearly separated.
4. **Given** a transient or terminal connection failure, **When** it occurs,
   **Then** retry/leave behavior matches F-005 and the screen never loops without
   explanation.

---

### User Story 4 - Communicate in a Circle (Priority: P1)

As an authorized circle member, I can enter group chat, understand message and
delivery state, attach existing supported media, record a voice note, and recover
from offline sending without losing my draft.

**Affected roles**: Student, teacher, supervisor.

**Why this priority**: Chat is a core MVP flow and must remain reliable while its
presentation is modernized.

**Independent Test**: Open a circle conversation from Chats and circle details,
exercise text, attachment, voice-note, failure/retry, archived, moderation, and
direct-message entry flows using existing F-004 fixtures.

**Acceptance Scenarios**:

1. **Given** a user belongs to circles, **When** they select Chats, **Then** their
   existing circle group chats are reachable in two taps or fewer from the shell.
2. **Given** a conversation, **When** messages load or the history is empty, **Then**
   hierarchy, authorship, delivery meaning, and the next action are clear in RTL
   and LTR.
3. **Given** a supported attachment or voice note, **When** the user records,
   previews, discards, or sends it, **Then** the existing F-004 limits and behavior
   remain unchanged.
4. **Given** offline or failed delivery, **When** the user retries or edits, **Then**
   draft content and idempotent delivery behavior are preserved.

---

### User Story 5 - Enter and Maintain an Account (Priority: P2)

As a user, I can register or sign in through a calm entry path and maintain my
existing profile and preferences without mixing routine edits with sensitive
account actions.

**Affected roles**: All users.

**Why this priority**: Account access is essential, while its backend behavior is
already governed by F-001 and must not be redesigned by visual work.

**Independent Test**: Exercise welcome, sign-in, registration, profile load/edit,
validation, save success, logout, and offline/error states while preserving all
existing auth/profile keys and controller behavior.

**Acceptance Scenarios**:

1. **Given** an unauthenticated user, **When** they choose sign-in or registration,
   **Then** the form shows only required existing fields and clear inline errors.
2. **Given** an authenticated user, **When** Profile opens, **Then** current profile
   data, editable fields, preferences, and account actions have a clear hierarchy.
3. **Given** a profile save, **When** it succeeds or fails, **Then** success remains
   visible in context or an actionable error is shown without losing edits.
4. **Given** larger text or screen-reader navigation, **When** forms and controls
   are used, **Then** labels, errors, focus order, and actions remain available.

---

### User Story 6 - Use the App Across Access Needs (Priority: P1)

As a user with a different language, theme, screen size, text scale, or assistive
technology, I receive the same understandable hierarchy and complete actions
without clipped content, color-only meaning, or undersized controls.

**Affected roles**: All users.

**Why this priority**: Wave 5 is the acceptance gate for every preceding wave,
not optional polish.

**Independent Test**: Run the cross-app audit matrix for representative screens
from every wave in Arabic RTL and English LTR, light and dark themes, increased
text scale, compact phone width, and screen-reader semantics.

**Acceptance Scenarios**:

1. **Given** Arabic RTL or English LTR, **When** a representative journey runs,
   **Then** directional icons, ordering, navigation, and gestures follow the active
   direction without separate behavior.
2. **Given** light or dark theme, **When** any required state appears, **Then** text
   and essential controls meet WCAG AA and meaning never depends on color alone.
3. **Given** increased text scale or a 320dp-wide screen, **When** primary screens
   render, **Then** Quranic/Arabic text does not clip and primary actions remain
   reachable.
4. **Given** a screen reader, **When** focus moves through an affected screen,
   **Then** controls have meaningful labels/states and dynamic changes are
   announced.
5. **Given** an intentionally planned action has no implemented behavior,
   **When** the user activates it, **Then** the localized shared
   under-implementation notice appears and no navigation, controller/API call,
   or product-state mutation occurs.

## Screen Inventory and Wave Traceability

| Wave | Screens/surfaces | Primary roles | Explicit exclusions |
|---|---|---|---|
| 0 — Foundation | Splash, welcome, authenticated shell, Home, bottom navigation, shared loading/empty/error components | All users | No analytics dashboard; no new session-discovery endpoint |
| 1 — Circles | Circle discovery/list, circle detail, create/join, members, management, retirement | Student, teacher, supervisor | No new membership, invitation, role, capacity, or retirement behavior |
| 2 — Sessions/queue | Circle session list/entry, ad-hoc create/start/join, session room, student queue, manager queue, grading, participants/moderation, reconnect/terminal states | Student, teacher, supervisor | No schedule/attendance UI, video, recording, new queue policy, or media permission change; no turn-based muting |
| 3 — Chat | Chats tab, group conversation, composer, attachment chooser, voice note, direct conversation entry, archived/offline/failure states | Authorized members | No unified DM inbox; no new media type, search, moderation, or notification behavior |
| 4 — Profile/auth | Welcome, sign-in, registration, existing profile view/edit, preferred language, logout | All users | No new identity provider, profile field, notification/appearance/privacy/support setting, avatar upload, account deletion flow, or authorization behavior |
| 5 — Cross-app audit | Representative screens and states from Waves 0–4 | All users | No new product capability; audit/fix only |

### Per-Screen UX Inventory

| Wave | Screen/surface | Purpose and data | Primary action | Entry → exit | Role variants | Screen-specific exclusions |
|---|---|---|---|---|---|---|
| 0 | Splash/auth initialization | Brand plus current auth initialization state | Wait safely | App launch → Welcome or Home | All | No marketing carousel or user data |
| 0 | Welcome | Brand and existing sign-in/register choices | Sign in | Unauthenticated redirect → Login/Register | Unauthenticated | No social provider or onboarding feature |
| 0 | App shell | Four destinations and selected state | Select destination | Authenticated redirect → tab root | Authenticated | No fifth tab, desktop rail, or hidden feature index |
| 0 | Home | Existing memberships and useful entry context available from current flows | Open circle | Shell → circle detail | Authenticated | No invented schedule, progress, attendance, or analytics data |
| 1 | Circles/discovery | My/public circles, query, membership state | Open/join circle | Shell → detail/join/create | Student/teacher/supervisor | No new discovery/filter contract |
| 1 | Join by invite/public join | Existing invite/public circle facts and confirmation | Confirm join | Circles/invite → circle detail | Eligible authenticated user | No role choice outside invitation contract |
| 1 | Create circle | Existing F-002 fields and validation | Create | Circles → new circle detail | Authenticated creator/teacher | No new field, bulk import, or institution option |
| 1 | Circle detail | Existing circle facts, role, members/chat/session/management entries | Open next relevant existing flow | Home/Circles → child screen/back | Member, public viewer, archived member | No schedule/progress/attendance card without existing data |
| 1 | Circle members | Existing member identity/role and authorized actions | Open member/action | Circle detail → DM/role action/back | Student vs manager vs archived | No new role or directory behavior |
| 1 | Circle management | Existing invite/role/member operations | Complete selected management action | Detail/members → confirmation/back | Teacher/supervisor differences | No permission expansion |
| 1 | Circle retirement | Existing archive consequence and safeguards | Confirm archive | Detail → archived detail/back | Teacher only | No hard delete |
| 2 | Circle sessions section | Existing ad-hoc scheduled/active sessions and create eligibility | Join/start or create | Circle detail → room/detail | Student vs teacher/supervisor | No recurring schedule or attendance UI |
| 2 | Student session/queue room | Connection, participants, open-audio status, queue position/turn/opt-out | Join or current safe queue action | Session section → room/leave | Student | No turn-based muting or moderator controls |
| 2 | Manager session/queue room | Connection, participants, round, queue, grading, moderation | Current round/participant action | Session section → room/leave | Teacher/supervisor | No new moderation or queue policy |
| 2 | Session dialogs/recovery | Existing create/round/grade/move/confirm data or retry/terminal reason | Confirm, retry, or leave | Room/section → updated room/back | Role-dependent | No invented defaults, diagnostics, or endless retry |
| 3 | Chats list | Client-derived circle group chats | Open conversation | Shell → group chat | Authorized/archived member | No unified DM inbox or new endpoint; loading/empty/retryable/offline recovery is a retained Wave 0 compatibility prerequisite, not a second Wave 3 implementation slice |
| 3 | Group conversation | Existing history, delivery/read, search/reply/pin/delete, presence, composer | Send/continue conversation | Chats/circle/room → back | Member, moderator, archived read-only | No new message/moderation/notification behavior |
| 3 | Direct conversation | Existing authorized pair history and composer | Send/continue conversation | Eligible member → back | Authorized pair only | No conversation discovery or unauthorized history |
| 3 | Media/voice flow | Existing attachment choice, recording, preview, upload/playback, pending state | Send or discard | Composer → conversation | Authorized sender; read-only viewer | No new media type, background audio, or live recording |
| 4 | Login | Existing email/password fields and validation | Sign in | Welcome → Home/error | Unauthenticated | No new provider or password backend behavior |
| 4 | Registration | Existing display name/email/password/language fields and validation | Register | Welcome → Home/error | Unauthenticated | No new profile field/provider |
| 4 | Profile | Existing profile fields, preferred language, save, and logout | Save profile | Shell → retained profile/logout | Authenticated | No notification/appearance/privacy/support setting, upload, or deletion |
| 5 | Cross-app audit | Evidence for all screens above | Resolve verified gap | Test/evidence harness → review result | Every applicable role | No product screen or capability |

## Primary Tasks and Tap Targets

Tap counts begin at the authenticated shell unless stated otherwise. A form
submission counts as one action; typing does not.

| Core task | Expected taps | Constraint |
|---|---:|---|
| Open any shell tab | 1 | Destination remains visibly selected |
| Open a circle from Home | 1 | Existing circle card behavior preserved |
| Find/open a circle from Circles | 2–3 | Search/filter interaction may add one tap |
| Create a circle | 3 | Circles, Create, submit; existing form/behavior only |
| Join a public circle | 3 | Circles, Join, confirm |
| Join by invite | 4 | Explicit exception for invite review/confirmation safety |
| Open a circle group chat from Chats | 2 | Chats tab, then circle |
| Open a circle group chat from circle details | 2 | Circle, then chat |
| Reach an existing session and start/join | ≤3 | May use circle details because no new session endpoint is allowed |
| See or act on current queue state after joining | ≤1 | Primary turn/action stays in the session room |
| Open Profile | 1 | Profile tab |
| Edit and save Profile | 2 | Profile, then save the existing grouped form |
| Open an eligible direct chat | 4 | Explicit exception because member entry preserves authorization and unified inbox is excluded |
| Start sign-in/registration from welcome | 1 | Unauthenticated entry point |

### Complete Core Task Budgets

| Core task from its stated entry | Actions | Budget/rationale |
|---|---:|---|
| Complete sign-in from Welcome after text entry | Open Login, Submit | 2 |
| Complete registration from Welcome after text entry | Open Register, Submit | 2 |
| Open any shell destination from Home | Select tab | 1 |
| Open a Home circle | Select circle | 1 |
| Open a circle from shell | Circles, select circle | 2 |
| Join a public circle | Circles, Join, Confirm | 3 |
| Join by invite | Circles, Invite, Continue, Confirm | 4; explicit review/confirmation safety exception |
| Create a circle after field entry | Circles, Create, Submit | 3 |
| Open members | Circles, circle, Members | 3 |
| Complete invite/role/member management | Circles, circle, management/member action, confirm | 4–5; authorization and confirmation exception |
| Archive a circle | Circles, circle, retirement screen, Archive, Confirm | 5; destructive confirmation exception |
| Open group chat | Chats, circle | 2 |
| Open eligible direct chat | Circles, circle, Members, person | 4; authorization-context exception |
| Join/start an existing session | Circles, circle, session action | 3 |
| Create then start an ad-hoc session | Circles, circle, Create session, Start | 4; F-005 requires separate create and explicit start transitions |
| Request opt-out from inside room | Request opt-out | 1 inside room; ≤4 from shell including session entry |
| Complete and grade current turn inside room | Complete, select grade, Confirm | 3 |
| Edit/save profile after field entry | Profile, Save | 2 |
| Logout | Profile, Logout | 2; no new confirmation unless existing security behavior requires it |

## Required State Matrix

| Wave | Loading | Empty | Success | Error | Offline/degraded |
|---|---|---|---|---|---|
| 0 | Branded auth and shell initialization | Home/chats explain absent memberships | Selected destination and authenticated landing are stable | Protected-route/auth failure returns to safe entry | Previously rendered shell context remains understandable; unavailable actions are disabled/explained |
| 1 | Circle skeleton/branded progress for waits over 300ms | No circles/discovery results/members explain next action | Join/create/update/retire confirmation remains in context | Validation, permission, load, and action errors recover safely | Cached/current context remains visible where existing controllers provide it; network actions explain retry |
| 2 | Session/participant/queue loading distinguishes initial load from reconnect | No active queue/participants explains next role-appropriate action | Connected/round/turn/grade states are explicit | Retryable versus terminal failures have distinct actions | Reconnecting preserves authoritative last state without enabling unsafe actions |
| 3 | History/media progress has labelled state | No messages or chats teaches the next action | Delivery/read/upload/playback state is explicit | Failed draft/media/history state offers retry/edit/fallback | Local pending messages and drafts remain visible; unavailable media explains limits |
| 4 | Auth/profile submission and load disable duplicate actions | Empty optional profile content has guidance | Sign-in/profile save remains visibly confirmed | Inline validation and recoverable auth/profile errors preserve input | Network-required actions explain retry and preserve safe local input |
| 5 | Audit covers every loading treatment | Audit covers every empty treatment | Audit covers selected/success treatment | Audit covers recoverable and terminal errors | Audit covers representative degraded behavior from every product wave |

### Per-Screen State Matrix

`N/A` is permitted only with the reason shown here or a more specific test note.

| Screen | Loading | Empty | Success/ready | Error | Offline/degraded |
|---|---|---|---|---|---|
| Splash | Branded auth initialization | N/A: transition-only | Redirect completes | Safe Welcome with human auth-init failure | Restored session or safe Welcome; no loop |
| Welcome | N/A: static | N/A: static choices | Chosen form opens | N/A: no remote action | Forms remain reachable; submit owns network failure |
| App shell | Auth/profile locale initialization | N/A: fixed destinations | Selected destination persists | Safe auth redirect | Existing tab context remains; network screens own state |
| Home | Membership/session-context progress after 300ms | Instructional no-circles state | Existing circle cards/available context | Retry without clearing safe context | Stale/current context labelled; network actions explained |
| Circles/discovery | Skeleton/progress | No memberships/results with next action | Lists and query result | Retry/permission/fatal distinction | Retain safe results; join/create disabled/explained |
| Join | Disabled submit/progress | Empty/invalid code guidance | Joined circle visible | Inline validation/conflict/permission | Preserve code/context and retry |
| Create circle | Disabled submit/progress | Required-field guidance | Created circle visible | Inline/general recoverable error | Preserve non-sensitive entries and retry |
| Circle detail | Skeleton/progress | Section-specific no sessions/members content | Loaded facts and completed action context | Retry/permission/terminal distinction | Retain readable facts; mutation disabled/explained |
| Members | Progress | No visible members only if contract permits | Role-labelled list/action result | Retry/permission | Retain safe list; mutation disabled |
| Management | Disabled action/progress | No eligible targets/invites guidance | Retained role/invite/member result | Inline conflict/permission/retry | Preserve safe selections; mutation disabled |
| Retirement | Disabled confirmation/progress | N/A: action screen | Archived context remains visible | Safeguard/conflict/retry | Confirmation disabled; consequences remain readable |
| Circle sessions | List/create progress | No active/ad-hoc sessions with manager create or member explanation | Scheduled/active cards | Retry/permission/provider-unavailable meaning | Retain last list; create/start/join paused |
| Student room | Joining/reconnecting | No round/participants explanation | Connected/current queue state/ended | Retryable versus terminal | Last confirmed snapshot; actions paused; Retry/Leave |
| Manager room | Joining/reconnecting | No round/participants with role action | Connected/round/control result | Retryable/terminal/action failure | Last confirmed snapshot; unsafe controls paused |
| Session dialogs | Disabled confirm/progress | Required-field/no-entry guidance | Updated room/queue state | Inline validation/conflict | Preserve safe input; confirm disabled |
| Chats list | Membership/chat progress | No circle chats with next action | Circle chats | Retry without masquerading as empty | Retain safe list; opening unavailable content explained |
| Group chat | History/media progress | Instructional no-messages state | Delivered/read/sent/pinned context | Retryable/terminal/access-lost/read-only | Durable draft/pending queue; retry/edit/discard |
| Direct chat | History/media progress | Instructional no-messages state | Delivered/read/sent context | Retryable/terminal/access-lost | Durable draft/pending queue; retry/edit/discard |
| Media/voice | Recording/upload/playback progress | No selection/empty recording guidance | Preview/sent/playable media | Permission/size/duration/upload/playback retry | Preserve draft/preview; network send paused |
| Login | Disabled submit/progress | Required-field guidance | Authenticated redirect | Inline safe auth error | Preserve email, clear/retain secret per existing safety, retry |
| Registration | Disabled submit/progress | Required-field guidance | Authenticated redirect | Inline validation/conflict | Preserve non-secret fields, retry |
| Profile | Load/save progress | Optional-field guidance | Saved values and confirmation retained | Inline/general retry without lost edits | Preserve edits; save/logout network behavior explained |

## Screenshot Acceptance Matrix

Every screen in the per-screen inventory receives a ready-state capture in all
four combinations: Arabic RTL/light, Arabic RTL/dark, English LTR/light, and
English LTR/dark at 390dp width and normal text scale. In addition:

| Wave | Required additional captures | Width/text scale |
|---|---|---|
| 0 | Splash loading; Home loading/empty/error/loaded; each selected shell destination | Home at 320dp and 600dp; Home at 200% text |
| 1 | Discovery empty/error/loaded; student and manager detail; members; management success/error; archived read-only; create/join validation | Discovery and detail at 320dp/600dp; detail at 200% text |
| 2 | Session list empty/error/scheduled/active; student connected/current-turn/opt-out/reconnect/terminal; manager queue/grading/moderation; destructive confirmation | Student and manager room at 320dp/600dp and 200% text |
| 3 | Chats empty/error/loaded; group/direct empty/history; own/other delivered/read; attachment sheet; voice preview; failed send; offline draft; archived/access-lost | Conversation at 320dp/600dp and 200% text |
| 4 | Login/register validation/loading; Profile loading/saved/error/offline; language direction switch; logout state | Each form/Profile at 320dp/600dp and 200% text |
| 5 | Visible focus/semantics evidence and any corrected violation from Waves 0–4 | 320dp, 600dp, normal and 200% text as applicable |

## Edge Cases

- Text scale increases while Arabic diacritics, long circle names, user names,
  and validation messages are visible on a 320dp-wide screen.
- Device direction changes between Arabic RTL and English LTR while nested in a
  tab or dialog; directional icons and focus order update correctly.
- Theme switches while a loading, error, selected, destructive, or disabled state
  is visible; contrast and non-color meaning remain intact.
- A user loses circle/session/chat access while a screen is open; existing
  authorization behavior wins and the UI returns to a safe recoverable/terminal
  state without exposing content.
- A reconnect delivers duplicate realtime events; the modernized UI preserves
  existing idempotent state and does not duplicate people, queue entries, or
  messages.
- A user has no circles, a very large number of visible items, long mixed-script
  labels, or a missing optional image/avatar.
- Reduced motion is requested; nonessential transitions are suppressed without
  hiding state changes.
- The reference HTML shows data or actions unavailable from existing contracts;
  unsupported data and unplanned actions are omitted. An intentionally planned
  action without implemented behavior shows the shared under-implementation
  notice without inventing product state.
- The reference HTML says a student's sound is muted until their turn; production
  ignores that stale prototype copy and preserves open student audio publishing.

## Requirements

### Functional Requirements

- **FR-001**: F-019 MUST cover Waves 0–5 under one Spec-Kit feature while
  preserving wave-level implementation and review batches.
- **FR-002**: The product MUST preserve the approved four-tab shell: Home,
  Circles, Chats, and Profile.
- **FR-003**: Home MUST prioritize existing next-useful/session context only when
  available through existing product flows and MUST NOT become a speculative
  analytics dashboard.
- **FR-004**: Every affected screen MUST have one visually dominant primary action
  and a clear back/exit path.
- **FR-005**: Core tasks MUST meet the tap targets in this specification; any
  exception above three taps MUST be recorded with a role, confirmation, or
  security rationale.
- **FR-006**: Every affected screen MUST define and implement applicable loading,
  empty, success, error, and offline/degraded states.
- **FR-007**: Waits over 300ms MUST use branded progress or skeleton treatment;
  no screen may rely on an unexplained spinner-only wait.
- **FR-008**: Consequential success MUST remain visible in context and MUST NOT
  depend solely on a transient snackbar.
- **FR-009**: Recoverable errors MUST provide a safe retry/fallback; terminal
  errors MUST state that retry is unavailable and offer a safe exit.
- **FR-010**: Arabic MUST be RTL-first and English MUST remain correct in LTR,
  including directional icons, ordering, progress, dialogs, and gestures.
- **FR-011**: Affected screens MUST support light and dark themes using the
  approved visual roles, with no color-only status meaning.
- **FR-012**: Normal text MUST meet at least 4.5:1 contrast; large text and
  essential UI graphics MUST meet at least 3:1.
- **FR-013**: Every interactive target MUST be at least 48x48dp and separated
  sufficiently to avoid accidental activation.
- **FR-014**: Interactive controls MUST expose meaningful screen-reader labels,
  roles, values, and states; material dynamic changes MUST be announced.
- **FR-015**: Supported text scaling MUST keep primary content/actions reachable
  and MUST NOT clip Arabic or Quranic diacritics.
- **FR-016**: Motion MUST be purposeful, normally 200–400ms, avoid flashing, and
  honor reduced-motion preferences.
- **FR-017**: Screens MUST consume approved Material 3 theme roles and shared
  spacing/type tokens; screen-level hard-coded colors are prohibited.
- **FR-018**: A new shared component MAY be introduced only when at least two
  screens reuse it; otherwise an existing Material/shared component MUST be used.
- **FR-019**: Existing routes, Riverpod controllers, API clients, widget keys,
  semantics relied on by tests, and localization patterns MUST be preserved
  unless an approved requirement explicitly changes them.
- **FR-020**: Existing F-001–F-005 product behavior, role checks, limits, safety
  rules, error meanings, and contracts MUST remain unchanged.
- **FR-031**: The frozen open-audio decision MUST supersede stale F-005 and
  prototype wording: authorized students remain free to publish audio and F-003
  queue position MUST NOT grant, revoke, mute, or unmute that permission.
- **FR-021**: F-019 MUST NOT introduce backend endpoints, database fields/tables,
  WebSocket events, role/permission behavior, provider capabilities, or contract
  changes.
- **FR-022**: F-019 MUST NOT introduce a unified direct-message inbox, provider
  controls, a new UI framework, a runtime theme/plugin system, or speculative
  abstraction.
- **FR-023**: The screen-sample HTML MUST be used only for hierarchy and visual
  language; unsupported sample data and unplanned actions MUST be omitted.
- **FR-024**: Completed Wave 0 and Wave 1 work MUST be audited and retained when
  conforming; it MUST NOT be rewritten solely for stylistic uniformity.
- **FR-025**: Every changed state and primary action MUST have widget-test
  coverage, including semantics and preserved keys where applicable.
- **FR-026**: Representative end-to-end journeys MUST verify shell navigation,
  circles, session/queue, chat, authentication/profile, and failure recovery.
- **FR-027**: Screenshot evidence MUST satisfy the complete Screenshot Acceptance
  Matrix, including every affected screen in Arabic RTL/English LTR and
  light/dark plus the named role/state, 320dp/600dp, and 200% text captures.
- **FR-028**: User-facing copy MUST be human-readable Arabic/English and MUST NOT
  expose raw IDs, fixture names, provider names, or developer diagnostics.
- **FR-029**: The existing preferred-language value MUST drive authenticated
  Arabic RTL versus English LTR presentation; unauthenticated entry MUST use the
  registration choice or platform locale without adding languages beyond `ar`
  and `en` or expanding F-012.
- **FR-030**: Circles and circle details MUST expose existing create/join and
  ad-hoc session list/create/start/join presentation entry points so the approved
  flows are reachable without new backend behavior.
- **FR-032**: Every action intentionally retained from the approved design but
  lacking implemented product behavior MUST invoke one shared presentation-only
  notice and MUST NOT navigate, call a controller/API, mutate state, or imply
  successful completion.
- **FR-033**: The shared notice MUST be dismissible, replace any prior instance,
  expose stable key `halaqatyUnderImplementationSnackBar`, and use the active
  direction's exact copy: English “This feature is under implementation and is
  not available yet.” and Arabic “هذه الميزة قيد التنفيذ وغير متاحة حالياً.”

### Compatibility Surfaces

F-019 creates no new persistent entity. Its compatibility surfaces are the
existing route locations, controller states, widget keys, semantic labels,
localization behavior, and public feature contracts used by F-001–F-005 tests.

## Success Criteria

### Measurable Outcomes

- **SC-001**: 100% of the listed core tasks are reachable within their stated
  tap targets in tested role-appropriate journeys.
- **SC-002**: 100% of affected screens have verified applicable loading, empty,
  success, error, and offline/degraded states, or a recorded reason a state cannot
  occur under existing behavior.
- **SC-003**: 100% of tested interactive controls meet the 48dp minimum and have
  meaningful accessible names/states.
- **SC-004**: Representative text and essential controls meet WCAG AA contrast in
  both light and dark themes.
- **SC-005**: The RTL/LTR screenshot matrix contains no clipped primary action,
  incorrect directional icon, reversed meaning, raw identifier, or developer
  placeholder on the reviewed screens.
- **SC-006**: Increased-text and compact-width audits preserve readable Arabic
  diacritics and reachable primary actions on all reviewed screens.
- **SC-007**: Existing F-001–F-005 mobile behavior and automated tests remain
  passing without backend or contract changes.
- **SC-008**: All required Flutter unit/widget tests, integration tests, analysis,
  formatting, and diff checks produce fresh successful evidence before the
  feature is considered complete.
- **SC-009**: 100% of inventoried intentionally planned but unimplemented actions
  show the shared warning in the active language after one tap, with verified
  zero navigation, controller/API calls, and product-state mutations.

## Assumptions

- Existing F-001–F-005 specifications and contracts are complete authority for
  user behavior; F-019 changes presentation and information hierarchy only.
- Existing routes, controllers, API clients, and data exposed to current screens
  are sufficient. Unsupported data and unplanned actions are omitted; approved
  future-intended actions use only the shared under-implementation notice.
- Supported users for current MVP screens are students, teachers, and supervisors;
  parent and institution experiences remain future features.
- A state may be marked not applicable only when the controller/domain cannot
  produce it; the screen inventory or test must record that reason.
- The bundled Cairo and Poppins families and replaceable brand assets remain the
  approved typography and brand foundation.
- Physical-device/emulator screenshot and integration evidence depends on the
  configured environment and cannot be reported as passing when unavailable.

## Dependencies

- F-001 Authentication, Roles, and User Profile
- F-002 Circle Management
- F-003 Recitation Queue System
- F-004 Real-time Chat
- F-005 Live Sessions
- `docs/engineering/design/UI_UX_GOVERNANCE.md`
- `docs/engineering/design/DESIGN.md`
- `docs/engineering/design/screen-samples-a-v2.html`

## Explicit Exclusions

- Backend, schema, OpenAPI, WebSocket, authorization, and provider changes
- New business behavior or new product data
- Unified direct-message inbox or conversation-discovery API
- Video, recording, screen sharing, new media types, or provider selection
- New UI framework, state-management framework, dependency, runtime theming, or
  plugin system
- Changes to future parent/institution product scope
