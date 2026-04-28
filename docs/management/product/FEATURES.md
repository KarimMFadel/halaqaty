# Halaqaty — Feature Specification & Status Board

> **Version:** 1.0 | **Status:** Planning Phase | **Last Updated:** 2026

**Related Documents:** [FEATURES_AR.md](../arabic/FEATURES_AR.md) · [PLAN.md](../planning/PLAN.md) · [ARCHITECTURE.md](../../engineering/architecture/ARCHITECTURE.md) · [DEVELOPMENT.md](../../../DEVELOPMENT.md) · [AGENT_COLLABORATION_GUIDE.md](../../engineering/collaboration/AGENT_COLLABORATION_GUIDE.md)

This is a **living document**. It tracks every feature from proposal through delivery, hosts design discussions, and captures open questions for the team.

**Workflow Note**: Features marked `🟡 Approved` are ready for development using Spec-Kit. To start building:
1. Run `/speckit.specify` in VS Code Copilot Chat for the feature
2. Follow all 7 Spec-Kit phases: specify → clarify → checklist → plan → tasks → analyze → implement
3. The 5 specialized agents (Golang Developer, Flutter Engineer, Architect, Tech Lead, Team Leader) will collaborate autonomously
4. See [DEVELOPMENT.md](../../../DEVELOPMENT.md) and [AGENT_COLLABORATION_GUIDE.md](../../engineering/collaboration/AGENT_COLLABORATION_GUIDE.md) for detailed workflow

---

## Status Legend

| Status | Symbol | Meaning |
|--------|--------|---------|
| Proposed | 🔵 | Feature identified; not yet reviewed by team |
| Approved | 🟡 | Team has agreed to build this |
| In Progress | 🟠 | Actively being developed |
| Done | 🟢 | Shipped and verified |
| Rejected | 🔴 | Decided not to build (with reason) |

---

## Feature Status Table

| ID | Feature | Priority | Status | Phase | Owner |
|----|---------|----------|--------|-------|-------|
| [F-001](#f-001-user-management--authentication) | User Management & Authentication | P0 | 🟡 Approved | 1 | Backend |
| [F-002](#f-002-circle-management) | Circle Management | P0 | 🟡 Approved | 1 | Full Stack |
| [F-003](#f-003-recitation-queue-system) | 🔥 Recitation Queue System | P0 | 🟡 Approved | 2 | Full Stack |
| [F-004](#f-004-real-time-chat) | Real-time Chat | P0 | 🟡 Approved | 1 | Backend |
| [F-005](#f-005-live-sessions-livekit) | Live Sessions (Audio-only, LiveKit) | P0 | 🟡 Approved | 2 | Full Stack |
| [F-006](#f-006-schedule--calendar) | Schedule & Calendar | P0 | 🟡 Approved | 2 | Full Stack |
| [F-007](#f-007-memorization-progress-tracking) | Memorization Progress Tracking | P1 | 🔵 Proposed | 3 | Full Stack |
| [F-008](#f-008-notification-system) | Notification System | P1 | 🔵 Proposed | 2 | Backend |
| [F-009](#f-009-built-in-digital-mushaf) | Built-in Digital Mushaf | P2 | 🔵 Proposed | 4 | Mobile |
| [F-010](#f-010-student--teacher-dashboards) | Student & Teacher Dashboards | P2 | 🔵 Proposed | 3 | Full Stack |
| [F-011](#f-011-reports--statistics) | Reports & Statistics (PDF) | P2 | 🔵 Proposed | 3 | Backend |
| [F-012](#f-012-multi-language-support) | Multi-language Support | P2 | 🔵 Proposed | 3 | Mobile |
| [F-013](#f-013-ai-tajweed-assessment) | AI Tajweed Assessment | P3 | 🔵 Proposed | 5 | AI/Backend |
| [F-014](#f-014-ai-memorization-planner) | AI Memorization Planner | P3 | 🔵 Proposed | 5 | AI/Backend |
| [F-015](#f-015-certificate-system) | Certificate System | P3 | 🔵 Proposed | 4 | Full Stack |
| [F-016](#f-016-desktop-app) | Desktop App (Flutter) | P3 | 🔵 Proposed | 5 | Mobile |
| [F-017](#f-017-institutional-platform) | 🏢 Institutional Platform | P3 | 🔵 Proposed | 5 | Full Stack |

---

## Detailed Feature Discussions

---

### F-001: User Management & Authentication

**Priority:** P0 | **Status:** 🔵 Proposed | **Phase:** 1

#### Description
Secure, multi-method user registration and authentication system with role-based access control. This is the foundation everything else depends on.

#### Acceptance Criteria
- [ ] Email/password registration with email verification link
- [ ] Google Sign-In (OAuth 2.0)
- [ ] Apple Sign-In (required for iOS App Store policy compliance)
- [ ] Phone OTP verification (WhatsApp-style; critical for Arabic-speaking markets with lower email usage)
- [ ] JWT session management backed by Firebase Auth
- [ ] User profile: display name, avatar (stored in MinIO), bio (optional), preferred language
- [ ] Password reset via email
- [ ] Account deletion with data erasure (GDPR/privacy compliance)
- [ ] Device token registration for FCM push notifications

#### Open Questions
- **OQ-001:** Should we support phone-only accounts (no email)? Common in some markets.
- **OQ-002:** Should teachers require identity verification (Quran credentials) to build trust? Or keep it open?
- **OQ-003:** Session expiry policy? Firebase default is long-lived; should we enforce shorter TTLs for security?

#### Design Decisions
- **DD-001:** Firebase Auth chosen over custom auth to avoid managing credential storage and OAuth provider integrations from scratch. Firebase handles token refresh and device sessions.
- **DD-002:** Role is stored in our PostgreSQL `users` table, not in Firebase. Firebase Auth is only for identity; our backend controls authorization.
- **DD-003:** Teacher identity verification is optional (configurable) and not required in MVP.
- **DD-004:** Phone-only signup remains under long-term investigation; keep signup open with current methods for now.

#### Dependencies
- None (this is foundational)

---

### F-002: Circle Management

**Priority:** P0 | **Status:** 🔵 Proposed | **Phase:** 1

#### Description
Circles are the core organizational unit. A circle is a Quran memorization group with a teacher, students, optional supervisors, and associated sessions, chat, and progress records.

#### Acceptance Criteria
- [ ] Create circle: name (required, max 100 chars), description (optional, max 500 chars), circle rules (optional, max 1000 chars), max capacity (default 50, max 200)
- [ ] Auto-generate unique 8-character invite code on creation
- [ ] Shareable deep link: `halaqaty.app/join/{code}`
- [ ] Invite code can be regenerated (old code invalidated) by teacher
- [ ] Student can join up to **5 circles** simultaneously (configurable limit)
- [ ] Teacher can assign Supervisor role to any circle member at any time (before session, during session, after session)
- [ ] Supervisor role can be revoked by teacher at any time
- [ ] Circle privacy: **Public** (discoverable in explore/search) vs **Private** (invite-only)
- [ ] Circle settings: language, gender specification (male/female/mixed/unspecified)
- [ ] Teacher can archive a circle (preserves all history, prevents new activity)
- [ ] Teacher can delete a circle (with confirmation; permanently deletes all data)
- [ ] Circle member list shows all members with roles, visible to all members

#### Open Questions
- **OQ-004:** Should there be a "co-teacher" role, distinct from supervisor? Some circles have two qualified teachers.
- **OQ-005:** Can a student be a supervisor in Circle A while being a student in Circle B? (Likely yes — roles are per-circle)
- **OQ-006:** What happens to a circle if the teacher deletes their account? Transfer to another teacher? Archive?

#### Design Decisions
- **DD-005:** Roles are per-circle (stored in `circle_members` table), not per-user globally. A user can be teacher in one circle and student in another.
- **DD-006:** Max 5 circles per student is a soft policy initially. Revisit based on user behavior.
- **DD-007 (Interim):** MVP uses teacher + supervisor permissions. Distinct co-teacher role details are deferred until after pilot outcomes.

#### Edge Cases
- Teacher invites someone who is already a member → Show "already a member" message, do not create duplicate
- Student tries to join a 6th circle → Error message explaining the limit
- Invite code collision (extremely unlikely with 8 chars) → Regenerate automatically

#### Dependencies
- F-001 (User Auth)

---

### F-003: Recitation Queue System

**Priority:** P0 | **Status:** 🔵 Proposed | **Phase:** 2

#### Description
The most unique and differentiating feature of Halaqaty. An intelligent, real-time ordered queue for student recitation during live sessions. This replaces the chaotic verbal ordering common in circles today.

#### Why This is the Killer Feature
Current pain point: In a typical online Quran circle, the teacher verbally says "now it's Ali's turn, then Fatima, then Omar..." — this is unstructured, hard to track, and leaves no record. Halaqaty makes the queue visible, interactive, and fully logged.

#### Queue States

```
                    ┌──────────────────────────────┐
                    │    QUEUE STATE MACHINE        │
                    │                               │
   Teacher creates  │   ┌──────────┐                │
   session ────────►├──►│  EMPTY   │                │
                    │   └─────┬────┘                │
                    │         │ Students join        │
                    │         ▼                      │
                    │   ┌──────────┐                │
                    │   │ WAITING  │◄───────────────┐│
                    │   │    ⏳    │   Skip/reorder  ││
                    │   └─────┬────┘                ││
                    │         │ Teacher starts turn  ││
                    │         ▼                      ││
                    │   ┌──────────┐                ││
                    │   │RECITING  │                ││
                    │   │    🎙️   │                ││
                    │   └─────┬────┘                ││
                    │         │ Teacher marks done   ││
                    │         ▼                      ││
                    │   ┌──────────┐ ┌──────────┐   ││
                    │   │COMPLETED │ │ SKIPPED  │───┘│
                    │   │    ✅   │ │    ⏭️   │    │
                    │   └──────────┘ └──────────┘    │
                    │                               │
                    │   QUEUE RESET ─────────────► EMPTY
                    └──────────────────────────────┘
```

#### Round System — Detailed Specification

Each session consists of one or more **rounds**. A round defines:
- `round_number` — 1, 2, 3, ...
- `round_type` — `new_memorization` | `revision` | `old_revision` | `test`
- `surah_name` — e.g., "Al-Baqarah"
- `from_ayah` — integer, e.g., 1
- `to_ayah` — integer, e.g., 20

When teacher resets the queue:
1. All entries in current round are marked as final
2. A new round record is created with new Surah/Ayah range
3. All students' statuses reset to ⏳ Waiting
4. Queue ordering can be re-configured for the new round

#### Real-Time Sync Requirements
- Queue state must be visible to all session participants simultaneously
- Latency target: < 500ms from teacher action to all clients reflecting update
- Technology: WebSocket broadcast to all session participants
- Offline handling: client should show "Reconnecting..." and re-sync queue state on reconnect
- Idempotency: duplicate WebSocket events must not cause double-state changes

#### Acceptance Criteria
- [ ] Queue visible to all session participants in real-time via WebSocket
- [ ] Minimum 3 ordering modes: join order, teacher manual, supervisor manual
- [ ] Per-round metadata: Surah name, from Ayah, to Ayah, round type
- [ ] Queue can be reset unlimited times per session
- [ ] When student's turn starts: in-app and push notification sent immediately
- [ ] Teacher can mute all → unmute only current reciter when their turn starts
- [ ] Grading mode configurable per circle (required or optional per completed turn)
- [ ] Student audio publish permission is teacher-controlled per turn (grant on turn start, revoke after turn)
- [ ] Teacher notes field per grading entry (free text, max 500 chars)
- [ ] Full session log persisted after session ends
- [ ] Queue handles students who join the session late (added to end of current round queue)
- [ ] Queue handles network disconnections gracefully (state preserved server-side)
- [ ] Student can request a temporary skip/opt-out for current turn (e.g., mic issue, permission break), approved by teacher/supervisor

#### Open Questions
- **OQ-007:** Should students be able to "opt out" of a specific round? (e.g., "I didn't prepare for revision today") → Teacher could mark as excused?
- **OQ-008:** Should there be a timer per student? (e.g., teacher sets 5-minute limit per recitation) → Useful but adds complexity
- **OQ-009:** Can the queue be pre-set before the session starts? Or only after session begins?
- **OQ-011:** Should there be a "double queue" — student appears twice (once for new memorization, once for revision) in the same round?

#### Design Decisions
- **DD-005:** Queue state is stored server-side in PostgreSQL (not just in-memory). This ensures history is preserved and reconnecting clients can recover state.
- **DD-006:** WebSocket events are the delivery mechanism, but PostgreSQL is the source of truth.
- **DD-007:** Grading mode is configured per circle (required vs optional per completed turn).
- **DD-008:** Temporary student opt-out is allowed for operational issues and is logged in queue history.
- **DD-009:** Late-joining students are appended to the end of the current active round.

#### Dependencies
- F-001 (User Auth)
- F-002 (Circle Management)
- F-005 (Live Sessions — queue exists within a session)
- F-008 (Notification System — turn notifications)

---

### F-004: Real-time Chat

**Priority:** P0 | **Status:** 🔵 Proposed | **Phase:** 1

#### Description
Full-featured messaging within circles, replacing WhatsApp/Telegram group chats and enabling structured communication between teachers and students.

#### Acceptance Criteria
- [ ] Group chat: one per circle, all members can participate
- [ ] Direct messages: teacher ↔ student (one-on-one); supervisor ↔ student
- [ ] **Voice messages** — record in-app, send, visualize waveform, playback; stored in MinIO
- [ ] Image attachments: JPEG, PNG; max 5MB per image
- [ ] File attachments: PDF only (for Mushaf pages, exercises); max 10MB
- [ ] Message delivery status: sent ✓ / delivered ✓✓ / read ✓✓ (blue)
- [ ] Typing indicators ("Ali is typing...")
- [ ] Full-text message search within a circle's chat history
- [ ] Pin messages: teacher/supervisor only; max 5 pinned per circle; pinned bar shown above chat
- [ ] Reply to specific message (shows quoted preview)
- [ ] Delete own messages within 10 minutes; teachers can delete any message
- [ ] WebSocket delivery when app is open; FCM push when app is in background
- [ ] Offline mode: messages queued locally and sent when connection restored

#### Open Questions
- **OQ-012:** Should there be "announcement-only" channels where only teachers can send? (Similar to Telegram channels)
- **OQ-013:** Should voice message length be limited? (e.g., max 5 minutes)
- **OQ-014:** Should we support emoji reactions?

#### Design Decisions
- **DD-009:** Voice messages are stored in MinIO with a pre-signed URL returned to clients. URLs expire after 7 days (renewable).
- **DD-010:** No end-to-end encryption in V1 (complex to implement with group messages). Encryption in transit (TLS) is sufficient for V1. E2E encryption is a P3 item.

#### Dependencies
- F-001 (User Auth)
- F-002 (Circle Management)

---

### F-005: Live Sessions (LiveKit)

**Priority:** P0 | **Status:** 🔵 Proposed | **Phase:** 2

#### Description
Unlimited-time audio-only sessions powered by LiveKit (open-source, self-hosted WebRTC SFU). This is the primary replacement for Zoom/Google Meet in MVP. Video is deferred to post-MVP behind a feature flag.

#### Why LiveKit?
- **Open-source:** No per-minute costs; self-hosted = full control
- **WebRTC-based:** Industry-standard, browser and mobile compatible
- **Official Flutter SDK:** `livekit_client` package with active maintenance
- **No time limits:** Unlike Zoom free tier (40 min), LiveKit sessions are unlimited
- **Audio quality control:** Can disable noise suppression, adjust bitrates — critical for Quran recitation

#### Audio Configuration (Critical)

Quran recitation requires preservation of specific phonetic qualities (tajweed) that voice-optimized audio processing can distort:

```yaml
# LiveKit room audio configuration
audio:
  codec: opus
  bitrate: 48000        # 48kbps minimum (vs Zoom's ~32kbps)
  noise_suppression: false   # OFF — preserves makhraj subtleties
  auto_gain_control: false   # OFF — consistent recitation volume
  echo_cancellation: true    # ON — still needed to prevent feedback
```

#### LiveKit Integration Flow

```
Step 1: Session Creation
  Teacher taps "Start Session" in Flutter app
       ↓
  Flutter → POST /api/v1/sessions/{id}/start → Go Backend
       ↓
  Go Backend calls LiveKit Server API → Creates room "{session_id}"
       ↓
  Go Backend returns: { livekit_token: "eyJ...", livekit_url: "wss://..." }

Step 2: Token Generation (Go Backend)
  Using livekit-server-sdk-go:
  at := auth.NewAccessToken(lkApiKey, lkApiSecret)
  grant := &auth.VideoGrant{RoomJoin: true, Room: roomName, CanPublishVideo: false} // MVP audio-only
  at.AddGrant(grant).SetIdentity(userID).SetValidFor(time.Hour)
  token, _ := at.ToJWT()

Step 3: Flutter Connects
  Flutter livekit_client package:
  room = await LiveKitClient.connect(livekitUrl, token, roomOptions)

Step 4: Media Routing
  LiveKit SFU routes audio streams between all participants
  Teacher controls (mute, remove) via Flutter → REST → Go → LiveKit API
```

#### Acceptance Criteria
- [ ] Flutter package: `livekit_client` (official) integrated
- [ ] Token generation exclusively on Go backend (never client-side)
- [ ] Audio-only in MVP (no video toggle in app)
- [ ] No time limits
- [ ] Teacher controls: mute all, mute individual, remove participant, lock room (no new joiners)
- [ ] Hand raise: students tap 🤚 → appears in teacher's UI; integrated with recitation queue
- [ ] Screen sharing is deferred to post-MVP (same feature-flag family as video)
- [ ] Session recording is disabled in MVP and deferred until a privacy consent/retention framework is approved
- [ ] Audio: Opus 48kbps+, noise suppression OFF, auto-gain OFF
- [ ] Maximum 50 participants (scalable with server resources)
- [ ] Graceful reconnection on network drop (LiveKit SDK handles this)

#### Open Questions
- **OQ-015:** Resolved — MVP is audio-only. No student video request path until post-MVP feature flag rollout.
- **OQ-016:** What is the maximum session duration we should target for testing? (For LiveKit server sizing)
- **OQ-017:** Deferred — recording stays disabled until privacy framework is approved; sharing model decided before activation.

#### Dependencies
- F-001 (User Auth)
- F-002 (Circle Management)
- F-003 (Recitation Queue — queue is embedded in sessions)

---

### F-006: Schedule & Calendar

**Priority:** P0 | **Status:** 🔵 Proposed | **Phase:** 2

#### Description
Recurring weekly schedule management with smart reminders and integrated attendance tracking.

#### Acceptance Criteria
- [ ] Weekly recurring schedule per circle: day(s) of week, start time, end time, timezone
- [ ] A circle can have multiple schedule entries (e.g., Sun + Wed)
- [ ] Push notifications: configurable reminder intervals (1hr, 30min, 15min, 5min before session)
- [ ] Auto-attendance: when student joins LiveKit session → marked Present
- [ ] Manual override: teacher can mark Present / Absent / Excused for any student
- [ ] Unified calendar: students in multiple circles see all sessions in one color-coded calendar view
- [ ] Conflict detection: alert if two circles have overlapping scheduled times
- [ ] Session lifecycle: Scheduled → Live (auto when teacher starts) → Completed (auto after end) → Cancelled (manual)

#### Open Questions
- **OQ-018:** Should we support non-recurring sessions (one-off makeup sessions)?
- **OQ-019:** Timezone handling: store in UTC, display in user's local timezone?

#### Dependencies
- F-001, F-002, F-005, F-008

---

### F-007: Memorization Progress Tracking

**Priority:** P1 | **Status:** 🔵 Proposed | **Phase:** 3

#### Description
Advanced per-student Quran memorization analytics, automatically populated from recitation queue history with teacher grading. MVP baseline remains session-level progress visibility (history + grades).

#### Acceptance Criteria
- [ ] Auto-create memorization record from each completed recitation queue entry
- [ ] Fields per record: student, circle, session, round type (new/revision), surah, from_ayah, to_ayah, grade, teacher notes, date
- [ ] Separate views: New Memorization tab vs Revision tab
- [ ] Visual Quran map: 114-surah grid, color-coded (memorized/partial/not started)
- [ ] Progress charts: Ayahs memorized per week/month (line graph)
- [ ] Attendance correlation: days attended vs progress made
- [ ] Teacher dashboard: side-by-side comparison of all students' progress

#### Open Questions
- **OQ-020:** Should students be able to self-log memorization done outside of sessions (home practice)?
- **OQ-021:** How do we handle a student memorizing the same section multiple times (multiple passes)?

---

### F-008: Notification System

**Priority:** P1 | **Status:** 🔵 Proposed | **Phase:** 2

#### Description
Multi-channel notification system ensuring no important event is missed.

#### Notification Matrix

| Event | Push (FCM) | In-App (WS) | Configurable OFF? |
|-------|-----------|------------|------------------|
| Session reminder (1hr/30min/15min/5min) | ✅ | ✅ | ✅ |
| New group message | ✅ | ✅ | ✅ |
| New private message | ✅ | ✅ | ❌ (always on) |
| Grade posted | ✅ | ✅ | ✅ |
| Circle invitation | ✅ | ✅ | ❌ |
| Queue: "You're next!" | ✅ | ✅ | ❌ |
| Queue: "Your turn!" | ✅ | ✅ | ❌ |
| Student joined session (→ teacher) | ❌ | ✅ | ✅ |

#### Acceptance Criteria
- [ ] FCM integration for background/closed state
- [ ] WebSocket-based in-app notification bell for foreground state
- [ ] Per-notification-type preferences in Settings
- [ ] Quiet hours: no notifications between user-defined hours (default: 10pm–7am)
- [ ] Notification history page in app (last 30 days)
- [ ] Unread notification count badge on app icon

---

### F-009: Built-in Digital Mushaf

**Priority:** P2 | **Status:** 🔵 Proposed | **Phase:** 4

#### Description
Integrated Quran text (Uthmani script) with Ayah-level interaction tied to memorization records.

#### Open Questions
- **OQ-022:** Use open-source Quran text (Tanzil.net) or license a digital Mushaf?
- **OQ-023:** Should the Mushaf support audio recitation playback (e.g., Sheikh Sudais)? This is a large feature that could be scoped as P3.

---

### F-010: Student & Teacher Dashboards

**Priority:** P2 | **Status:** 🔵 Proposed | **Phase:** 3

#### Description
Role-based dashboards for student self-tracking and teacher oversight across circles.

#### Acceptance Criteria
- [ ] Student dashboard shows own attendance, grades, notes, and memorization progress
- [ ] Teacher dashboard shows all students taught by that teacher with summary metrics
- [ ] Circle dashboard shows per-circle progress, attendance, and queue history
- [ ] No parent-linked account management in MVP scope

---

### F-011: Reports & Statistics

**Priority:** P2 | **Status:** 🔵 Proposed | **Phase:** 3

#### Description
Comprehensive reporting for teachers and students, with PDF export capability.

#### Report Types
- **Student Progress Report:** Attendance %, grades distribution, memorization progress, trend charts
- **Circle Overview Report:** All students side-by-side, session history, top performers
- **Custom Date Range Reports**

#### Open Questions
- **OQ-024:** Should reports be auto-generated monthly and emailed to teachers?

---

### F-012: Multi-language Support

**Priority:** P2 | **Status:** 🔵 Proposed | **Phase:** 3

#### Languages (Priority Order)
1. Arabic (default, RTL) — core market
2. English (LTR)
3. Urdu (RTL) — large Muslim community in Pakistan, India, UK
4. Malay (LTR) — Southeast Asia
5. Turkish (LTR)
6. French (LTR) — North Africa diaspora

#### Acceptance Criteria
- [ ] Full RTL layout support for Arabic and Urdu
- [ ] Locale-aware date/time formatting
- [ ] Locale-aware number formatting (Eastern Arabic numerals option)
- [ ] Language preference saved per user account

---

### F-013: AI Tajweed Assessment

**Priority:** P3 | **Status:** 🔵 Proposed | **Phase:** 5

> **Dependency note:** This feature is blocked until post-MVP recording is available with an approved privacy consent/retention framework.

#### Description
AI-powered analysis of student recitation audio to detect tajweed errors and assist teachers in grading.

#### How It Works
1. Session recording (or dedicated recitation recording) is processed
2. Audio segmented per student turn (using recitation queue timestamps)
3. AI model analyzes:
   - Makhraj (articulation points) correctness
   - Madd (elongation) lengths
   - Tanwin and noon sakinah rules
   - Waqf (stopping) correctness
4. Error report generated with timestamp references
5. Grade suggestion provided to teacher (teacher can override)

#### Open Questions
- **OQ-025:** Train our own model or license an existing AI Quran recitation system?
- **OQ-026:** Privacy implications: storing audio for AI processing. Need explicit consent.

---

### F-014: AI Memorization Planner

**Priority:** P3 | **Status:** 🔵 Proposed | **Phase:** 5

#### Description
Personalized memorization schedule based on student's historical pace.

#### Algorithm (Conceptual)
- Analyze: Ayahs memorized per session, session frequency, grade distribution
- Apply spaced repetition principles (adapted for Quran, not generic flashcards)
- Generate: weekly plan (e.g., "Memorize 5 Ayahs from Al-Imran on Mon/Wed/Fri; Revise Al-Fatiha on Tue/Thu")
- Adaptive: if student underperforms one week, reduce next week's target automatically

---

### F-015: Certificate System

**Priority:** P3 | **Status:** 🔵 Proposed | **Phase:** 4

#### Description
Digitally-signed completion certificates for Juz, Khatm al-Quran, or custom milestones.

#### Certificate Types
- Juz completion (30 types)
- Complete Quran (Khatm)
- Custom milestone (teacher-defined)

#### Technical Approach
- PDF certificate generated server-side with teacher's signature and circle name
- QR code embedded → verifiable link showing student name, teacher, date, milestone
- Shareable: student can download and share on social media

---

### F-016: Desktop App

**Priority:** P3 | **Status:** 🔵 Proposed | **Phase:** 5

#### Description
Flutter Desktop builds providing a native experience for teachers who prefer desktop.

**Platforms:** Windows, macOS, Linux

**Primary use case:** Teachers using desktop for session management, progress review, and report generation.

---

### F-017: Institutional Platform

**Priority:** P3 | **Status:** 🔵 Proposed | **Phase:** 5

#### Description
The most significant long-term business feature. Enables Quran memorization institutions to manage all their circles and teachers centrally.

#### Target Institutions
- Quran memorization schools (مدارس تحفيظ القرآن)
- Mosque-affiliated circles
- Islamic universities and colleges
- National Quran competition bodies
- Online Quran academies

#### Features
- Institution registration with approval process
- Centralized teacher onboarding (invite teachers, assign them circles)
- Centralized student enrollment (bulk import via CSV)
- Institution-wide analytics dashboard
- Custom branding (logo, color scheme, custom subdomain: `al-noor.halaqaty.app`)
- Academic year management (terms, exams, final assessments)
- Institution-level certificate templates
- Billing: institution pays per teacher (B2B model)

#### Business Impact
This single feature could generate more revenue than all individual subscriptions combined, while serving the organizations that most need a solution.

---

## Open Questions Log

All open questions from feature discussions, consolidated:

| ID | Question | Feature | Status | Decision |
|----|---------|---------|--------|---------|
| OQ-001 | Phone-only accounts (no email)? | F-001 | Decided | No phone-only accounts in MVP. Require email or social provider. Phone is supplementary verification only. |
| OQ-002 | Teacher identity verification? | F-001 | Decided | Optional (not required in MVP) |
| OQ-003 | Session token expiry policy? | F-001 | Decided | Firebase default 1hr auto-refresh. Backend enforces 30-day inactivity logout. |
| OQ-004 | Co-teacher role distinct from supervisor? | F-002 | Decided | Deferred post-pilot. MVP uses teacher + supervisor only. No co-teacher role in MVP. |
| OQ-005 | Cross-circle role combinations? | F-002 | Decided | Yes — roles are fully independent per circle. A user can be teacher in one circle and student in another. |
| OQ-006 | Circle ownership transfer on account deletion? | F-002 | Decided | Circle is archived on teacher account deletion; members are notified. Teacher must assign a supervisor before deletion. No automatic transfer. |
| OQ-007 | Student "opt-out" of a round? | F-003 | Decided | Allowed for temporary issues with teacher/supervisor approval |
| OQ-008 | Per-student recitation timer? | F-003 | Decided | No timer in MVP. Teacher manages timing verbally. |
| OQ-009 | Pre-set queue before session starts? | F-003 | Decided | Yes — teacher can pre-order the queue before starting the session. |
| OQ-010 | Late-joining student added to current round? | F-003 | Decided | Added to end of current active round |
| OQ-011 | Double-queue per student per round? | F-003 | Decided | No. One position per student per round. Use sequential rounds for multiple recitations. |
| OQ-012 | Announcement-only channels? | F-004 | Decided | No announcement channels in MVP. Use pinned messages for circle-wide announcements. |
| OQ-013 | Voice message maximum length? | F-004 | Decided | Max 5 minutes (300 seconds). Max file size: 20 MB. |
| OQ-014 | Emoji reactions? | F-004 | Decided | Deferred to post-MVP (P2). Not in MVP scope. |
| OQ-015 | Student video permission model? | F-005 | Decided | MVP is audio-only; no video permission in MVP |
| OQ-016 | Max session duration for server sizing? | F-005 | Decided | Max 4 hours per session. Idle room timeout: 30 minutes after last participant leaves. |
| OQ-017 | Recording visibility (teacher-only vs all)? | F-005 | Deferred | Recording disabled until privacy framework approval; finalize model before activation |
| OQ-018 | Non-recurring (one-off) sessions? | F-006 | Decided | Yes — teacher can create one-off sessions not linked to a recurring schedule. |
| OQ-019 | Timezone storage strategy? | F-006 | Decided | UTC stored in DB. IANA timezone string stored per user profile. All display in user's local timezone. |
| OQ-020 | Student self-logging (outside sessions)? | F-007 | Decided | No in MVP. Progress records are generated from session-based recitations only. |
| OQ-021 | Multiple passes on same section? | F-007 | Decided | Yes — each queue entry creates a new progress record. Full history of all passes is retained. |
| OQ-022 | Open-source vs licensed Mushaf text? | F-009 | Decided | Tanzil.net (CC BY 3.0, open-source). No licensing cost. |
| OQ-023 | Audio recitation playback in Mushaf? | F-009 | Decided | No in MVP. Mushaf viewer is reading-only. Audio playback deferred to P2. |
| OQ-024 | Auto-generated monthly reports via email? | F-011 | Decided | No in MVP. Manual PDF export only. |
| OQ-025 | In-house AI model vs licensed for Tajweed? | F-013 | Decided | Fully deferred. No AI architecture decision until recording feature is unblocked and privacy framework is approved. |
| OQ-026 | Audio privacy/consent for AI processing? | F-013 | Decided | Fully deferred. No AI architecture decision until recording feature is unblocked and privacy framework is approved. |

---

## Community Suggestions

*This section will be populated once the platform launches and user feedback comes in.*

Ideas from the broader community will be logged here for team review:

| # | Suggestion | Source | Date | Status |
|---|-----------|--------|------|--------|
| CS-001 | *(placeholder)* | — | — | — |

---

*This document is maintained alongside [arabic/FEATURES_AR.md](arabic/FEATURES_AR.md). Any business-facing changes here must be mirrored there.*

*See [SYNC_GUIDE.md](SYNC_GUIDE.md) for the documentation sync policy.*
