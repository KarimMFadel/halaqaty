# F-019 Compatibility Inventory (T004)

**Date**: 2026-09-22 | **Scope**: Waves 0–4 preserved surfaces and approved-design action classification.

Preservation rule: routes, widget keys, semantics relied on by tests, controller
inputs/states/actions, and API error meanings below are invariant for F-019
(`contracts/compatibility.md`). Any change requires returning to product approval.

## 1. Preserved routes and redirects (`mobile/lib/app/router.dart`)

- Routes: `/splash`, `/welcome`, `/login`, `/register`, and the
  `StatefulShellRoute.indexedStack` branches `/home`, `/circles`, `/chats`,
  `/profile` (initial location `/home`).
- Redirect matrix: `unknown` → `/splash`; `unauthenticated` → `/welcome`
  (except `/welcome`, `/login`, `/register`); `authenticated` → `/home` from
  any entry route; all other locations unchanged.
- Shell behavior: reselecting Home pops the home branch to its route root;
  reselecting the current tab restores `initialLocation`; other tab stacks are
  preserved via the indexed stack.
- Pushed (non-shell) navigation: `CircleDetailScreen` from Home/Circles cards,
  `GroupChatScreen` from Chats/circle detail — `MaterialPageRoute`, preserved.

## 2. Preserved widget keys

Shell/foundation (Wave 0): `appNavigationBar`, `authInitializing`, `openLogin`,
`openRegister`, `homeNoCircles`, `homeCircle-<id>`, `chatsEmpty`,
`chatCircle-<id>`, `circleLoadError`, `circleLoadRetry`,
`halaqatyErrorSnackBar`, `logoutButton`.

Auth/profile (Wave 4): `displayNameField`, `emailField`, `passwordField`,
`languageDropdown`, `submitButton`, `profileFullNameField`,
`profileDisplayNameField`, `profileBioField`, `profileCountryField`,
`profileLanguageDropdown`, `profileAvatarUrlField`, `profilePhoneField`,
`profileSaveButton`, `profileSaveSuccess`.

Circles (Wave 1): `circleDiscoverySearchField`, `circleDiscoveryLoading`,
`openInviteJoinButton`, `circleInviteField`, `circleInviteSubmitButton`,
`circleInviteLink`, `refreshCircleInvite`, `shareCircleInvite`,
`confirmCircleInviteRefresh`, `joinCircle-<id>`, `confirmCircleJoinButton`,
`circleJoinError`, `openCircle-<id>`, `openCircleChat`, `openCircleManagement`,
`openCircleRetirement`, `circleMembersList`, `circleMembersLoading`,
`circleManagementDenied`, `circleMutationError`, `confirmCircleRoleChange`,
`confirmCircleMemberRemoval`, `circleArchivedBanner`,
`circleArchivedReadOnlyBanner`, `archiveCircleButton`, `confirmCircleArchive`,
`createCircleNameField`, `createCircleCapacityField`,
`createCircleGenderField`, `createCirclePrivateField`,
`createCircleUserSearchField`, `createCircleSubmitButton`.

Chat/sessions (Waves 2–3): `pinned-<messageId>`, per-message `Key(message.id)`,
plus semantics-driven finders used by existing chat/session tests.

New in F-019 (Wave 0): `halaqatyUnderImplementationSnackBar` (FR-033) — the
only new key Wave 0 adds.

## 3. Preserved semantics

- `HalaqatyLogo`: `Semantics(label: 'Halaqaty', image: true)`.
- Home circle cards: `Semantics(button: true, label: circle.name)`.
- `NavigationBar` destinations expose labels and `isSelected` (asserted in
  `widget_test.dart` for the Circles destination).
- SnackBars announce via the platform; `showHalaqatyError` uses
  `errorContainer` colors with a Dismiss action.

## 4. Preserved controller states

- `AuthController`/`AuthState`: `status` (unknown/authenticated/unauthenticated),
  `sessionId`, `user`, `errorMessage`, `isLoading`; actions `register`,
  `signIn`, `logout` (exact signatures incl. `preferredLanguage`).
- `ProfileController`/`ProfileState`: `profile`, `isLoading`, `isSaving`,
  `errorMessage`, `fieldErrors`; actions `loadProfile`, `updateProfile`.
- `CircleDiscoveryController`/`CircleDiscoveryState`: `myCircles`,
  `publicCircles`, `nextCursor`, `isLoading`, `joiningCircleId`, `failure`
  (`CircleJoinFailure` enum incl. `network`, `sessionExpired`);
  actions `loadMyCircles`, `discover`, `joinPublic`, `joinInvite`.
- Chat/session controllers (F-003/F-004/F-005): unchanged; Wave 2/3 work
  recomposes presentation only.

## 5. Behavior assertions to keep green

From `mobile/test/widget_test.dart` (Wave 0 surface): branded splash with
`authInitializing`; welcome exposes `openLogin`/`openRegister`; login/register
forms open; four-tab shell with `homeNoCircles`; NavigationBar indicator uses
`secondaryContainer`; retryable Home offline state (`circleLoadError` → retry →
2 API calls); chats/circles/profile tab switches; Home reselect pops pushed
detail; logout returns to welcome; auth loss clears the protected stack; auth
transition disposes profile state for the next session; Arabic RTL welcome
labels; default light theme primary `0xFF1B7E3C`.

## 6. Approved-design action classification (per `screen-samples-a-v2.html`)

Classification: **implemented** (existing behavior/routes) · **notice**
(intentionally planned, no behavior → shared under-implementation notice only,
FR-032/FR-033) · **omitted** (unsupported data or unplanned prototype action).

### Wave 0 — shell/Home/welcome/splash
| Sample action/data | Classification | Notes |
|---|---|---|
| Splash brand + auth progress | implemented | `HalaqatySplash`, `authInitializing`; adds semantic label (Wave 0 fix) |
| Welcome sign-in / register | implemented | `openLogin`, `openRegister` |
| Four bottom-nav destinations | implemented | Home, Circles, Chats, Profile |
| Home greeting ("السلام عليكم") | implemented | generic greeting; no invented personalization data |
| Home hero "next session" + join button | omitted | next-session/schedule data unsupported by existing contracts (FR-003) |
| Home quick action "اكتشاف حلقة" (discover) | implemented | switches to existing Circles destination |
| Home quick action "رابط الدعوة" (invite link) | implemented | switches to Circles destination hosting `openInviteJoinButton` |
| Home "عرض الكل" (see all circles) | implemented | switches to Circles destination |
| Home top-bar avatar → profile | omitted | redundant duplicate of the Profile destination; no hidden fifth entry |
| Chats list from memberships | implemented | `chatCircle-<id>`; failure must not masquerade as empty (Wave 0 fix). Retained as a Wave 3 compatibility prerequisite; Wave 3 implementation starts with group/direct conversation and composer/media presentation. |

### Wave 1 — circles (audit reference for later phases)
| Sample action/data | Classification | Notes |
|---|---|---|
| Search, My/Discover segments, invite link | implemented | existing keys above |
| Circle cards with role tag, member counts, "open circle" | implemented | existing discovery/detail |
| Per-card schedule text ("الأحد والثلاثاء") | omitted | no schedule data in contract |
| Detail: progress section ("تقدّمك هذا الشهر"), "آخر نشاط" feed | omitted | progress/attendance data unsupported (FR-003) |
| Detail actions: chat, members | implemented | `openCircleChat`, members list |
| Detail action "المواعيد" (schedule) | notice | planned in design, no F-002/F-005 schedule behavior |
| Management/retirement flows | implemented | existing controllers and confirmations |

### Wave 2 — sessions/queue (audit reference)
| Sample action/data | Classification | Notes |
|---|---|---|
| Circle session list, ad-hoc create/start/join | implemented | existing F-005 APIs; Wave 2 exposes entry |
| "صوتك مغلق حتى يبدأ دورك" (muted until turn) | omitted | stale prototype copy; frozen open-audio decision wins (FR-031) |
| Student queue position/turn/opt-out, raise hand | implemented | existing F-003 behavior |
| Teacher round/grading/moderation, reorder, participants | implemented | existing manager controls |
| Reconnect with last confirmed state, Try again/Leave | implemented | existing retryable/terminal recovery |
| Sample elapsed timers, exact countdowns | omitted | no per-student timer in MVP (frozen) |

### Wave 3 — chat (audit reference)
| Sample action/data | Classification | Notes |
|---|---|---|
| Group conversation, composer, delivery/read states | implemented | existing F-004 |
| Attachment sheet: photo / PDF / voice note with limits | implemented | existing media types and limits only |
| Voice note record/preview/send/discard | implemented | existing F-004 voice behavior |
| Offline strip, failed send retry/edit, draft retention | implemented | existing pending-message behavior |
| Unified DM inbox in Chats | omitted | explicitly excluded (FR-022) |

### Wave 4 — auth/profile (audit reference)
| Sample action/data | Classification | Notes |
|---|---|---|
| Sign-in/registration forms, language choice | implemented | existing fields/keys |
| Profile grouped fields, preferred language, save, logout | implemented | existing profile form |
| "تغيير الصورة" (avatar upload) | omitted | avatar-upload excluded (spec exclusion) |
| Appearance setting | notice | planned in design, no appearance behavior in F-019 |
| Notifications setting/toggle | notice | planned in design, no notification settings behavior |
| Help & support | notice | planned in design, no support behavior |
| Privacy & security | notice | planned in design, no privacy-center behavior |

The four **notice** actions are the only intentionally under-implementation
actions in the approved design; all invoke the shared
`halaqatyUnderImplementationSnackBar` notice with the exact FR-033 copy and no
navigation, controller/API call, or state mutation. They surface in Wave 4
(profile/preferences), not Wave 0 — Wave 0 ships zero notice actions.
