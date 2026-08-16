# User Journey: Halaqaty

> Teacher-first, student-aware. Every screen a student sees is downstream of a teacher decision.

**Related Documents:** [PRD.md](./PRD.md) · [FEATURES.md](./FEATURES.md) · [../../../DEVELOPMENT.md](../../../DEVELOPMENT.md)

**Scope:** MVP only. Post-MVP features (recording, AI Tajweed, video) are noted but not detailed.  
**Format:** `[Screen / Event] → Actor: action → System: response → Outcome`

---

## Journey Map Overview

```
T-01  App Install & Onboarding
T-02  Teacher Registration
T-03  Email Verification
T-04  Teacher Profile Setup
T-05  Create a Circle
T-06  Invite Students
T-07  Student Joins Circle (Student side)
T-08  Schedule a Session
T-09  Pre-Session: Build Queue
T-10  Start a Live Session
T-11  Student Joins Live Session
T-12  Queue: Mark Student Reciting
T-13  Queue: Rate / Complete Recitation
T-14  Queue: Advance Queue
T-15  Chat During Session
T-16  Voice Note in Chat
T-17  End Session
T-18  Post-Session: Progress Review
T-19  Circle Management (ongoing)

Student Journey (abbreviated)
S-01  App Install & Join Circle
S-02  View My Progress
S-03  Join a Live Session (Student)

Cross-Cutting: Error States
Cross-Cutting: Offline Behavior
```

---

## T-01 — App Install & Onboarding

**Actor:** New user (teacher or student)  
**Entry:** App opens for the first time

1. **Splash screen** — Halaqaty logo, 2s, then navigate to Onboarding.
2. **Onboarding carousel** — 3 screens (circles, queue, progress). Skip button visible on all.
3. **"Get Started"** button → navigate to T-02 (Register) or login screen.
4. **Login screen** shows: Email/password, "Continue with Google", "Continue with Apple" (iOS only).

**Decision from MVP Register:** No phone-only accounts (OQ-001). Apple Sign-In required on iOS (App Store rule).

---

## T-02 — Teacher Registration

**Actor:** Teacher  
**Entry:** "Get Started" → "Create Account"

1. User selects **"I'm a Teacher"** (vs "I'm a Student").
2. Fills: full name, email, password (8+ chars, 1 uppercase, 1 digit).
3. Taps **"Create Account"**.
4. **System:** Creates Firebase account → calls `POST /api/v1/auth/register` with Firebase UID and profile data → creates `users` row with `role=teacher`.
5. Navigate to T-03 (Email Verification).

**Error cases:**
- Email already registered → "This email is already in use. Sign in instead?" with link.
- Weak password → inline field error, not a toast.
- Network error → retry banner (not modal), state preserved.

---

## T-03 — Email Verification

**Actor:** Teacher  
**Entry:** After T-02 registration

1. Screen: "Check your inbox" with masked email address.
2. "Resend email" button (rate-limited to 1/minute on backend).
3. **System:** Firebase sends verification email.
4. User taps link in email → Firebase marks email verified.
5. App detects `emailVerified: true` on next token refresh → navigate to T-04 (Profile Setup).
6. If user closes app before verifying: next app open shows the same verification screen until verified.

---

## T-04 — Teacher Profile Setup

**Actor:** Teacher  
**Entry:** First-time after email verification

1. **Optional fields:** Display name (pre-filled from registration), profile photo, city, teaching style (dropdown: Traditional / Modern / Mixed).
2. "Continue" button → navigate to Dashboard (empty state).
3. Profile setup is skippable — user lands on Dashboard with a banner "Complete your profile".

---

## T-05 — Create a Circle

**Actor:** Teacher  
**Entry:** Dashboard → "Create Circle" button (or empty state CTA)

1. **Circle creation form:**
   - Circle name (required, 2–100 chars)
   - Description (optional, max 500 chars)
   - Curriculum (dropdown: Quran memorization / Revision / Tajweed rules / Custom)
   - Max students (default 50, maximum 200)
   - Privacy: Public (discoverable/joinable) / Private (invite-only); both retain invite links
2. Tap **"Create Circle"**.
3. **System:** `POST /api/v1/circles` → creates `circles` row + `circle_members` row (teacher role) → returns circle with invite code.
4. Navigate to Circle Dashboard (empty member list, invite code displayed prominently).

**Error cases:**
- Name already used by this teacher → inline error.
- Free tier cap (3 circles) reached → "Upgrade to Pro to create more circles" modal.

---

## T-06 — Invite Students

**Actor:** Teacher  
**Entry:** Circle Dashboard → "Invite" button

1. Teacher sees: **Invite link** (halaqaty.app/join/[code]) and **QR code**. Public circles are also discoverable and joinable without an invite.
2. "Copy link" and "Share" (native share sheet) buttons.
3. Optionally: "Send invite by email" (enters email → system sends invite email).
4. Teacher can revoke and regenerate the invite code at any time.

---

## T-07 — Student Joins Circle (Student side)

**Actor:** Student  
**Entry:** Taps invite link or scans QR code

1. If not logged in: App opens → "Join [Circle Name]" screen → Login/Register.
2. If logged in: "Join [Circle Name]?" confirmation screen with circle details.
3. Tap **"Join"**.
4. **System:** `POST /api/v1/circles/:id/join` → creates `circle_members` row (student role) → sends notification to teacher.
5. Student lands on Circle view (member list, upcoming sessions).
6. Teacher receives push notification: "[Student Name] has joined [Circle Name]".

**Error cases:**
- Invalid or expired invite code → "This invite link is not valid or has expired."
- Circle is full → "This circle has reached its maximum capacity. Please contact your teacher."

---

## T-08 — Schedule a Session

**Actor:** Teacher  
**Entry:** Circle Dashboard → "Schedule Session"

1. **Session type:** One-off / Recurring (weekly, biweekly, custom days)
2. **Fields:**
   - Session title (optional, defaults to "Circle Session")
   - Date and time (date picker + time picker)
   - Timezone (pre-filled from teacher profile, editable)
   - Duration estimate (30 min / 1h / 2h / custom — informational only, no auto-end)
   - Recurring: days of week + end date or "no end date"
3. Tap **"Schedule"**.
4. **System:** `POST /api/v1/circles/:id/sessions` → creates session record(s) → sends push notifications to all circle members.
5. Session appears on circle calendar and on each student's home screen.

**Decision from MVP Register:** UTC stored in DB, IANA timezone per user for display (OQ-019). One-off sessions supported (OQ-018).

---

## T-09 — Pre-Session: Build Queue

**Actor:** Teacher  
**Entry:** Upcoming session card → "Prepare Queue" (available up to 30 min before session start)

1. Teacher sees student list with drag handles.
2. Drag to reorder → saves order automatically.
3. Long-press a student → options: "Remove from queue", "Move to top".
4. Tap **"Save Queue"**.
5. **System:** `PUT /api/v1/sessions/:id/queue` → saves ordered list → queue locked until teacher opens the session.

**Decision from MVP Register:** Pre-set queue allowed before session starts (OQ-009). One position per student per round (OQ-011).

---

## T-10 — Start a Live Session

**Actor:** Teacher or Supervisor
**Entry:** Ad-hoc session card → "Start Session"

1. Tap **"Start Session"**.
2. **System:**
   - Creates the session media room through `SessionMediaGateway` via `POST /api/v1/sessions/:id/start` (LiveKit-backed in MVP)
   - Backend returns the teacher's required participant-specific `MediaConnection`; the LiveKit MVP adapter maps it to `CanPublishAudio: true`, `CanPublishVideo: false`, `RoomAdmin: true`
   - Session status → `active`
   - Broadcasts metadata-only `session.started { session_id, circle_id }`; it never contains endpoint, credential, or room reference
3. Teacher or supervisor enters the **Session Room** screen: participant list, audio state, hand state, and moderation controls. Queue and chat panels are composed later by F-003 and F-004.
4. Teacher microphone is **muted by default** at session start.

**Decision from MVP Register:** Audio-only, video disabled (OQ-015). Max session 4 hours (OQ-016). Participant media credentials come from the backend only; LiveKit is the MVP adapter (constitution security invariant).

---

## T-11 — Student Joins Live Session

**Actor:** Student or other active circle member
**Entry:** Session card → "Join"

1. Tap **"Join Session"**.
2. **System:**
   - `POST /api/v1/sessions/:id/join` → returns the student's required `MediaConnection`; the LiveKit MVP adapter maps it to `CanPublishAudio: false` until called and `CanSubscribe: true`
   - Student added to participant list
3. Participant enters Session Room: sees participant presence, audio state, and hand controls. Queue and chat are provided by F-003/F-004 later.
4. Student microphone is muted and **disabled** until the teacher calls them.

---

## T-12 — Queue: Mark Student Reciting

**Actor:** Teacher or Supervisor
**Entry:** Session Room → Queue panel → Tap student name → "Start Recitation"

1. **System:**
   - `POST /api/v1/sessions/:id/queue/:studentId/start`
   - Calls the sessions-owned reciter-audio control to set effective audio publishing; the LiveKit MVP adapter maps it to `CanPublishAudio: true`
   - Broadcasts queue state update to all session participants
   - Creates `recitation_sessions` row with `started_at`
2. Student's microphone becomes active automatically.
3. All other students' microphones remain disabled.
4. Timer visible to teacher only (informational — no auto-stop).

---

## T-13 — Queue: Rate / Complete Recitation

**Actor:** Teacher  
**Entry:** Session Room → Queue panel → active student → "Complete"

1. Teacher sees recitation rating panel:
   - **Outcome:** Passed / Needs Review / Repeat
   - **Surah + Ayah range** (from Mushaf picker or manual entry)
   - **Notes** (optional, free text, max 500 chars)
2. Tap **"Save & Advance"**.
3. **System:**
   - `POST /api/v1/sessions/:id/queue/:studentId/complete` with rating payload
   - Creates `progress_records` row
   - Revokes student's `CanPublishAudio` permission
   - Advances queue pointer to next student
   - Notifies next student: "You're next — get ready"

**Decision from MVP Register:** Multiple passes allowed, full history retained (OQ-021). No self-logging (OQ-020).

---

## T-14 — Queue: Advance Queue (Skip / Opt-Out)

**Actor:** Teacher or Supervisor  
**Entry:** Queue panel → swipe student row → "Skip" or student requests to opt out

1. Tap **"Skip"** (teacher decision) or receive student opt-out request via chat.
2. If opt-out: teacher approves or declines.
3. **System:**
   - `POST /api/v1/sessions/:id/queue/:studentId/skip`
   - Logs skip reason (timeout / opt-out / absent)
   - Advances queue to next student
4. Skipped student moves to end of current round (not removed from queue).

**Decision from MVP Register:** Opt-out allowed with teacher approval, logged not penalized (OQ-007). No per-student timer (OQ-008).

---

## T-15 — Chat During Session

**Actor:** Any participant  
**Entry:** Session Room → chat icon (bottom right)

1. Chat panel slides up (bottom sheet).
2. Text messages visible to all circle members (teacher, supervisor, students).
3. Compose area: text input + voice note button + send.
4. Messages sent via `POST /api/v1/circles/:id/messages`.
5. Real-time delivery via WebSocket subscription on the circle channel.

---

## T-16 — Voice Note in Chat

**Actor:** Any participant  
**Entry:** Chat panel → hold microphone icon

1. Hold: recording starts (system mic permission requested if not yet granted).
2. Release: recording stops.
3. Duration indicator visible during recording. Max 5 minutes (300s).
4. Preview playback before sending (user can discard or send).
5. Tap **"Send"** → `POST /api/v1/circles/:id/messages` with audio file upload.
6. **System:** Stores in MinIO, returns playback URL. Message appears in chat with play button.

**Decision from MVP Register:** Max 5 min / 20 MB per voice note (OQ-013).

---

## T-17 — End Session

**Actor:** Teacher  
**Entry:** Session Room → "End Session" button

1. Confirmation dialog: "End session for all participants? This cannot be undone."
2. Tap **"End Session"**.
3. **System:**
   - `POST /api/v1/sessions/:id/end`
   - Closes the session media room through the configured adapter (LiveKit in MVP; all participants disconnected)
   - Session status → `ended`
   - All joined participants receive the metadata-only `session.ended` event with `end_reason`; general push delivery belongs to F-008.
4. Teacher navigates to Session Summary screen (T-18).
5. Students return to Circle view.

**Idle timeout:** If teacher disconnects without ending, room closes after 30 minutes of no active participants.

---

## T-18 — Post-Session: Progress Review

**Actor:** Teacher  
**Entry:** Session Summary screen (auto-navigate after T-17)

1. Screen shows:
   - Students recited: count and list with outcomes (Passed / Needs Review / Repeat)
   - Students skipped: list with reasons
   - Duration: session start to end time
   - Juz/Surah coverage across all students
2. "View full progress" → navigates to Circle Progress view (per-student Juz tracker).
3. "Export" → PDF summary (P2 feature, not MVP).

---

## T-19 — Circle Management (Ongoing)

**Actor:** Teacher  
**Entry:** Circle Dashboard → Settings gear

1. **Member management:**
   - Promote student to Supervisor (one tap)
   - Remove member (confirmation + reason optional)
   - View join history
2. **Circle settings:**
   - Edit name, description, curriculum
   - Regenerate invite code
   - Archive circle (all sessions preserved as read-only)
3. **Notifications:**
   - Toggle: "Notify me when a student joins"
   - Toggle: "Notify students 15 min before session"

---

## Student Journey (Abbreviated)

### S-01 — App Install & Join Circle

1. Download app → "I'm a Student" → register (email or Google/Apple).
2. Tap invite link or enter circle code manually.
3. Land on Circle view: upcoming sessions, member list.

### S-02 — View My Progress

1. Home screen → "My Progress" tab.
2. Shows Juz tracker (which Juz have been recited, how many times, latest outcome).
3. Tap a session → see all recitation records for that session.
4. **Cannot** edit own progress (no self-logging — OQ-020).

### S-03 — Join a Live Session (Student)

1. Push notification or session card → "Join".
2. Completes T-11: authorized `POST /sessions/:id/join` returns only their participant-specific `MediaConnection`.
3. Enters Session Room (muted, subscribed to audio).
4. Sees their queue position highlighted.
5. When called (T-12): microphone activates automatically.
6. When done (T-13): microphone deactivates. They stay in the room and hear others.

---

## Cross-Cutting: Error States

| Scenario | User-facing response |
|---|---|
| Firebase token expired (silent refresh fails) | Soft logout → login screen with "Your session expired, please sign in again." |
| Session media service unreachable (LiveKit in MVP) | "Could not connect to the session. Check your network." + Retry button. No spinner loop. |
| WebSocket disconnected mid-session | Reconnection attempt (3x backoff). Banner: "Reconnecting…" → "Reconnected." → if final failure: "Connection lost. Tap to rejoin." |
| API 500 / unexpected server error | Generic error toast: "Something went wrong. Please try again." Do not expose error details. |
| API 4xx validation error | Inline field error where possible; toast for non-form contexts. |
| Invite code expired | "This invite link has expired. Ask your teacher to share a new one." |
| Circle capacity full | "This circle is full. Contact your teacher." |
| File upload fails (voice note, photo) | "Upload failed. Check your connection and try again." Discard button available. |
| Push notification permission denied | App functions fully. Gentle prompt after first session ends: "Enable notifications to know when sessions start." |

---

## Cross-Cutting: Offline Behavior

| Action | Offline behavior |
|---|---|
| View circle member list | Shown from local cache (Hive / local storage). Stale indicator visible if >24h old. |
| Send chat message | Queued locally. Sent when connection restores. Banner: "Waiting for connection…" |
| Join live session | Blocked. Error: "You need an internet connection to join a live session." |
| View progress history | Shown from local cache. Read-only. |
| Schedule a session | Blocked. Error shown inline. |
| App launch offline | Shows cached home screen with an offline banner. All read actions available; write actions blocked with inline errors. |

---

*This document describes the MVP journey. Features marked (P2) are planned but not in scope for the initial release. See [FEATURES.md](./FEATURES.md) for the full feature status board.*
