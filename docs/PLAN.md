# Halaqaty — Master Project Plan

> **Version:** 1.0 | **Status:** Planning Phase | **Last Updated:** 2026

**Related Documents:** [PRD.md](PRD.md) · [arabic/PLAN_AR.md](arabic/PLAN_AR.md) · [FEATURES.md](FEATURES.md) · [ARCHITECTURE.md](ARCHITECTURE.md) · [DEPLOYMENT.md](DEPLOYMENT.md)

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Target Users & Roles](#2-target-users--roles)
3. [Detailed Feature Specifications](#3-detailed-feature-specifications)
4. [Technical Architecture Overview](#4-technical-architecture-overview)
5. [Deployment Strategy](#5-deployment-strategy)
6. [Release Strategy](#6-release-strategy)
7. [Business Model](#7-business-model)
8. [Timeline — 12-Month Plan](#8-timeline--12-month-plan)

---

## 1. Executive Summary

### 1.1 Vision Statement

> *To empower every Quran memorization teacher and student worldwide with a single, purpose-built digital platform — replacing the fragmented toolset of today with one seamless, spiritually-mindful application.*

### 1.2 Problem Statement

Across the Muslim world, tens of thousands of Quran memorization circles operate today using a patchwork of general-purpose tools:

| Pain Point | Current Workaround | Cost/Limitation |
|------------|-------------------|-----------------|
| Group communication | WhatsApp / Telegram groups | No role hierarchy, no structured data |
| Live sessions | Zoom / Google Meet | **40-minute limit** on free Zoom; expensive paid plans |
| Progress tracking | Paper notebooks / Excel | Manual, error-prone, not shared in real-time |
| Recitation management | Verbal ordering during session | Chaotic; no records; hard to scale |
| Attendance tracking | Manual headcount | Forgotten entries; no historical data |
| Quran-specific features | None | None of these apps understand Surahs or Ayahs |

**The result:** Teachers spend significant time managing logistics rather than teaching. Students miss sessions without automated reminders. Progress data is scattered or lost entirely.

### 1.3 Solution Overview

Halaqaty is a **unified, Quran-native platform** that brings together:
- Real-time group chat and voice notes
- Unlimited-time audio sessions (powered by LiveKit WebRTC in MVP; video is post-MVP behind feature flag)
- An intelligent recitation queue system — the platform's killer feature
- Structured memorization progress tracking
- Smart scheduling with push notifications
- Role-based access for teachers, students, and supervisors

All in a single mobile-first application with full iOS, Android, and Web support from a single Flutter codebase.

### 1.4 Target Market

**Primary:** Individual Quran memorization teachers (solo-teacher first) running circles of 5–7 students initially, starting with Egypt and later expanding across Arabic-speaking countries and Muslim diaspora communities worldwide.

**Secondary:** Quran memorization schools, Islamic centers, and mosques that run multiple circles simultaneously.

**Tertiary (Future):** Institutional — formal Quran memorization organizations, universities with Quran faculties, national-level Quran competition bodies.

### 1.5 Future: Institutional Platform (منصة المؤسسات)

One of the most significant long-term growth vectors is the **Institutional Platform**. Quran memorization institutions — schools, centers, mosques, and national organizations — manage dozens or hundreds of circles simultaneously. Today they do this with chaos.

The institutional tier of Halaqaty will allow organizations to:
- Register as an institution with centralized admin control
- Onboard all teachers and students under one roof
- View institution-wide analytics: memorization rates, attendance, teacher performance
- Set institution-wide schedules and academic calendars
- Custom branding (institution logo, colors)
- Bulk enrollment of students from spreadsheets
- Generate reports for students, donors, or regulatory bodies

This opens a **B2B revenue model** that is more predictable and higher-value than individual subscriptions.

---

## 2. Target Users & Roles

### 2.1 Role Overview

| Role | Arabic Title | Description |
|------|-------------|-------------|
| Teacher / Reciter | مُقرئ / محفظ | Creates and manages circles; conducts sessions; evaluates students |
| Student | طالب | Joins circles; recites; tracks progress |
| Supervisor | مُشرف | Assigned by teacher; helps manage sessions and queue |
| Institution Admin | مدير مؤسسة | Manages entire institution (future) |

### 2.2 Teacher (مُقرئ/محفظ)

The primary power user of Halaqaty.

**Capabilities:**
- Create and configure circles (name, rules, capacity, privacy settings)
- Generate and share invite codes/links
- Conduct live audio sessions (video is post-MVP)
- Manage the recitation queue during sessions (order, skip, grade)
- Assign and revoke the Supervisor role to any circle member at any time
- Grade students' recitations (Excellent → Repeat scale)
- Send private messages and voice notes to students
- View session-level progress per student in MVP (comprehensive analytics later)
- Set and manage weekly schedules
- Pin important announcements in circle chat

### 2.3 Student (طالب)

**Key design decision: A student can join MULTIPLE circles with DIFFERENT teachers simultaneously.** This is a common real-world scenario — a student may recite new memorization with one teacher and do revision sessions with another.

**Capabilities:**
- Join multiple circles with different teachers
- Participate in live sessions; join recitation queue
- Send and receive messages (group and private)
- View own session-level progress and grades in MVP (advanced analytics later)
- Receive schedule reminders and turn notifications (when it's their time to recite)
- View Quran map of memorized portions

### 2.4 Supervisor (مُشرف)

A trusted student or assistant appointed by the teacher to help manage sessions.
MVP uses this assignable role model; distinct co-teacher role details remain open until after pilot.

**Key design decision: A supervisor can be assigned at any point** — before the session is created, before the session starts, or during a live session. The teacher retains full authority.

**Capabilities (granted by teacher):**
- Manage the recitation queue (reorder, skip, add late joiners)
- Mute/unmute participants in live sessions
- Track attendance
- Pin messages in circle chat
- Cannot grade students (grading is teacher-only)
- Cannot remove the teacher from the circle

### 2.5 Institution Admin (مدير مؤسسة) — *Future*

- Manages the entire institution's presence on Halaqaty
- Onboards teachers and students in bulk
- Views institution-wide dashboards and analytics
- Manages institution-level settings and branding

---

## 3. Detailed Feature Specifications

Each feature is documented with: Description, User Stories, Acceptance Criteria, Priority, and Status.

**Priority Levels:**
- **P0** — Core MVP: must launch with this
- **P1** — Important: ship in first major update
- **P2** — Enhancement: valuable but not urgent
- **P3** — Future: long-term roadmap

**Status:** 🔵 Proposed | 🟡 Approved | 🟠 In Progress | 🟢 Done | 🔴 Rejected

---

### F-001 · User Management & Auth | P0 | 🔵 Proposed

**Description:** Secure user registration and authentication with role assignment.

**User Stories:**
- As a new user, I can register with email/password, Google, Apple, or phone OTP so I can start using Halaqaty quickly
- As a registered user, I can log in securely from any device
- As a user, I can set and update my profile (name, avatar, bio)
- As a teacher, I can optionally complete verification to build trust with students

**Acceptance Criteria:**
- [ ] Email/password registration with email verification
- [ ] Google Sign-In (OAuth)
- [ ] Apple Sign-In (required for iOS App Store)
- [ ] Phone OTP verification (critical for Arabic-speaking markets)
- [ ] JWT-based session management via Firebase Auth
- [ ] Profile: display name, avatar (stored in MinIO), bio
- [ ] Password reset via email
- [ ] Account deletion (GDPR/privacy compliance)

---

### F-002 · Circle Management | P0 | 🔵 Proposed

**Description:** The core organizational unit. A circle represents a Quran memorization group led by a teacher.

**User Stories:**
- As a teacher, I can create a circle with a name, description, and rules so students know what to expect
- As a teacher, I can generate an invite code/link to share with students
- As a student, I can join multiple circles simultaneously with different teachers
- As a teacher, I can assign the Supervisor role to a trusted member at any time
- As a teacher, I can set circle privacy (public/discoverable vs private/invite-only)

**Acceptance Criteria:**
- [ ] Create circle: name (required), description, rules, max capacity (default 50)
- [ ] Auto-generate unique 8-character invite code (e.g., `HLQ-7X2K`)
- [ ] Generate shareable deep link (e.g., `halaqaty.app/join/HLQ-7X2K`)
- [ ] Student can join up to 5 circles simultaneously
- [ ] Teacher can assign Supervisor role before/during/after sessions
- [ ] Circle privacy settings: Public (discoverable in search) / Private (invite only)
- [ ] Gender-specific setting (male/female/mixed)
- [ ] Language setting for the circle
- [ ] Teacher can archive or delete a circle
- [ ] Circle member list with roles visible to all members

---

### F-003 · 🔥 Recitation Queue System | P0 | 🔵 Proposed

**Description:** The most unique and critical feature of Halaqaty. An intelligent, real-time ordered queue for students to recite during a live session — structured, transparent, and historically recorded.

**User Stories:**
- As a teacher, I can see all students in an ordered queue during a live session
- As a student, I can see my position in the queue and know when it's my turn
- As a teacher, I can start a new recitation round specifying Surah and Ayah range
- As a teacher, I can reset the queue to start a new round (e.g., switch from new memorization to revision)
- As a teacher/supervisor, I can reorder, skip, or move students in the queue
- As a student, I receive a notification when it's my turn to recite
- As a teacher, I can grade a student's recitation immediately after they finish

**Queue Ordering Modes:**
1. **Auto — Join Order:** First student to join the session gets first position in queue
2. **Manual — Teacher Arrangement:** Teacher manually sorts the queue before or during the session
3. **Supervisor Arrangement:** Teacher delegates queue ordering to the assigned Supervisor

**Visual Status for Each Student:**

| Status | Icon | Meaning |
|--------|------|---------|
| Waiting | ⏳ | Hasn't recited yet; position N in queue |
| Currently Reciting | 🎙️ | Highlighted for all participants; audio unmuted |
| Completed | ✅ | Recited; grade recorded based on circle grading policy |
| Skipped | ⏭️ | Absent or skipped by teacher; can re-add |

**Queue Round System:**

A single session can have **multiple rounds**, each with a reset:

```
Session: "Sunday Halaqa — Week 23"
│
├── Round 1: New Memorization (حفظ جديد)
│   └── Surah: Al-Baqarah | From: Ayah 1 | To: Ayah 20
│   └── [Ali ✅ Grade: Excellent] [Fatima ✅ Grade: Very Good] [Omar ⏭️ Absent]
│
│   ↓ QUEUE RESET ↓
│
├── Round 2: Revision (مراجعة)
│   └── Surah: Al-Fatiha | From: Ayah 1 | To: Ayah 7
│   └── [Ali ✅ Grade: Good] [Fatima 🎙️ Reciting...] [Omar ⏳ Waiting]
│
│   ↓ QUEUE RESET ↓
│
└── Round 3: Old Revision (مراجعة قديمة)
    └── Surah: An-Nas | From: Ayah 1 | To: Ayah 6
    └── [Ali ⏳ Waiting] [Fatima ⏳ Waiting] [Omar ⏳ Waiting]
```

**Real-Time Queue Operations:**
- **Advance:** Teacher marks current student as ✅ Done → next student is auto-notified
- **Skip:** Teacher moves a student to ⏭️ Skipped (student can be re-added)
- **Reorder:** Drag-and-drop queue reordering by teacher/supervisor
- **Add Late-Joiner:** Students who join the session late are added to the end of the current round queue
- **Turn-based Audio Publish:** Student audio publish permission is granted for the active turn and revoked after the turn (teacher-controlled)
- **Emergency Mute:** When student's turn starts, their microphone is unmuted; all others are muted

**Grading Scale:**

| Grade | Arabic | Meaning |
|-------|--------|---------|
| Excellent | ممتاز | Perfect recitation, excellent tajweed |
| Very Good | جيد جداً | Minor errors, good tajweed |
| Good | جيد | Some errors, acceptable tajweed |
| Acceptable | مقبول | Notable errors, basic tajweed |
| Needs Review | يحتاج مراجعة | Significant errors; review required |
| Repeat | إعادة | Must fully repeat before advancing |

**Session History Log:**
- Full log stored per session: student name, round, Surah, Ayah range, grade, teacher notes, time started, time completed
- Exportable as PDF/CSV (P2)

**Acceptance Criteria:**
- [ ] Queue is visible to all session participants in real-time
- [ ] Queue state syncs via WebSocket (< 500ms latency)
- [ ] At least 3 queue ordering modes available
- [ ] Round system: each round tracks Surah + Ayah range + round type (new/revision)
- [ ] Teacher can reset queue unlimited times per session
- [ ] When student's turn starts: notification sent, microphone auto-unmuted
- [ ] All other participants muted when a student begins reciting (unless teacher disables)
- [ ] Grading mode is configurable per circle (required or optional per completed turn)
- [ ] Student audio publish permission is teacher-controlled per turn (grant on turn start, revoke on turn end)
- [ ] Full session log accessible after session ends
- [ ] Queue updates must be idempotent and handle network reconnections gracefully

---

### F-004 · Real-time Chat System | P0 | 🔵 Proposed

**Description:** Full-featured messaging within each circle, with private teacher-student channels.

**User Stories:**
- As a member, I can send messages in the circle group chat
- As a teacher, I can send a private message or voice note to a student
- As a teacher, I can pin important messages for all members to see
- As a student, I can record and send a voice note with my recitation for practice

**Acceptance Criteria:**
- [ ] Group chat per circle (all members)
- [ ] Private direct messages (teacher ↔ student; supervisor ↔ student)
- [ ] Voice messages with waveform visualizer — critical for recitation practice outside sessions
- [ ] Image attachments (JPEG, PNG)
- [ ] File attachments (PDF — for Mushaf pages, exercises; max 10MB)
- [ ] Message read receipts (single tick = sent; double tick = read)
- [ ] Typing indicators
- [ ] Message search (full-text)
- [ ] Pin messages (teacher/supervisor only; max 5 pinned per circle)
- [ ] Reply to specific message (threaded)
- [ ] Delete message (own messages; teachers can delete any)
- [ ] WebSocket-based delivery; FCM fallback for background

---

### F-005 · Live Sessions (Audio via LiveKit) | P0 | 🔵 Proposed

**Description:** Unlimited-time audio-only sessions integrated with the recitation queue system, powered by LiveKit (open-source, self-hosted WebRTC SFU). Video is explicitly deferred to post-MVP and gated by feature flag.

**Flutter ↔ LiveKit Integration Flow:**

```
┌──────────────┐    1. Request token     ┌──────────────┐
│              │ ──────────────────────►  │              │
│  Flutter App │    (REST API call)       │  Go Backend  │
│  (livekit_   │                          │              │
│   client)    │  2. Generate token       │  (LiveKit    │
│              │ ◄──────────────────────   │   Server SDK │
│              │    (JWT token returned)   │   for Go)    │
└──────┬───────┘                          └──────┬───────┘
       │                                         │
       │  3. Connect with token                  │ Manages rooms
       │                                         │ via LiveKit API
       ▼                                         ▼
┌─────────────────────────────────────────────────────┐
│              LiveKit Server (SFU)                    │
│  - Handles WebRTC connections                       │
│  - Routes audio streams between participants         │
│  - Manages Opus codec in MVP (VP8/VP9/AV1 post-MVP)│
│  - Simulcast deferred with post-MVP video rollout  │
└─────────────────────────────────────────────────────┘
```

**Flutter Package:** `livekit_client` (official LiveKit Flutter SDK)

**Audio Optimization for Quran Recitation:**
Zoom and Google Meet apply heavy audio processing optimized for speech — this can distort the precise phonetic qualities of Quranic recitation. Halaqaty configures LiveKit for recitation-quality audio:

| Setting | Default (Zoom-style) | Halaqaty Configuration |
|---------|---------------------|------------------------|
| Codec | Opus ~32kbps | Opus 48–64kbps (higher quality) |
| Noise Suppression | ON | **OFF** (preserves tajweed subtleties) |
| Auto-Gain Control | ON | **OFF** (consistent recitation volume) |
| Echo Cancellation | ON | ON (still needed) |
| Bitrate | Adaptive (often low) | 48kbps minimum guaranteed |

**User Stories:**
- As a teacher, I can start a live session from the circle page with one tap
- As a student, I can join a session from a push notification or calendar reminder
- As a teacher, I can mute/unmute individual participants
- As a teacher, I can lock the room to prevent new joiners mid-session
- As a participant, I can raise my hand to signal the teacher
- As a teacher, I can trust that live-session audio is not recorded in MVP

**Acceptance Criteria:**
- [ ] Flutter package: `livekit_client` integrated
- [ ] Audio-only in MVP; no video toggle exposed to users
- [ ] No time limits on sessions
- [ ] Teacher controls: mute all, mute individual, remove participant, lock room
- [ ] Hand raise feature (synced with recitation queue)
- [ ] Screen sharing deferred to post-MVP (same feature-flag family as video)
- [ ] Session recording is disabled in MVP and deferred due to privacy risk; future rollout requires explicit consent framework
- [ ] Noise suppression OFF by default; auto-gain OFF by default
- [ ] Opus codec at 48kbps minimum
- [ ] LiveKit room name tied to session ID; room created by Go backend using LiveKit Go SDK
- [ ] Token generated by Go backend using LiveKit Go SDK (never by the client)
- [ ] Maximum 50 participants per room (can increase with server scaling)
- [ ] Graceful reconnection on network drop

---

### F-006 · Schedule & Calendar | P0 | 🔵 Proposed

**Description:** Recurring weekly schedule management with smart reminders and attendance tracking.

**User Stories:**
- As a teacher, I can set a recurring weekly schedule for my circle
- As a student in multiple circles, I can see a unified calendar of all my sessions
- As a student, I receive configurable push notification reminders before sessions
- As a teacher, I can record manual attendance (override for students who called ahead)

**Acceptance Criteria:**
- [ ] Weekly recurring schedule per circle (day of week, start time, end time, timezone)
- [ ] Push notification reminders: configurable intervals (1hr, 30min, 15min, 5min before)
- [ ] Auto-detect attendance: when student joins a live session, attendance is marked present
- [ ] Manual attendance override: teacher can mark a student as present, absent, or excused
- [ ] Unified calendar view for students in multiple circles (shows all circles color-coded)
- [ ] Conflict detection: warn teacher/student if two circles have overlapping schedules
- [ ] Session status: Scheduled → Live → Completed / Cancelled

---

### F-007 · Memorization Progress Tracking | P1 | 🔵 Proposed

**Description:** Advanced per-student Quran memorization analytics layer linked to recitation queue history. MVP baseline remains session-level progress visibility (history + grades).

**Acceptance Criteria:**
- [ ] Automatic log creation from recitation queue entries (Surah, Ayah range, grade, date, session)
- [ ] Separate tracking: New Memorization (حفظ جديد) vs Revision (مراجعة)
- [ ] Teacher notes per student per session
- [ ] Visual Quran map: color-coded visualization of memorized vs pending portions
- [ ] Daily / weekly / monthly progress charts
- [ ] Filterable by Surah, date range, grade, or type (new/revision)

---

### F-008 · Notification System | P1 | 🔵 Proposed

**Description:** Multi-channel notification system covering all important events.

**Notification Types & Channels:**

| Event | Push (FCM) | In-App (WebSocket) |
|-------|-----------|-------------------|
| Session starting soon (reminder) | ✅ | ✅ |
| New message in circle | ✅ | ✅ |
| Private message received | ✅ | ✅ |
| Grade posted by teacher | ✅ | ✅ |
| Circle invitation | ✅ | ✅ |
| Queue turn — "You're next!" | ✅ | ✅ |
| Queue turn — "You're reciting now!" | ✅ | ✅ |

**Acceptance Criteria:**
- [ ] FCM for background/closed app
- [ ] WebSocket-based in-app for foreground
- [ ] Per-notification-type preferences (user can disable specific types)
- [ ] Quiet hours setting (no notifications between configurable hours)
- [ ] Notification history accessible in app

---

### F-009 · Built-in Digital Mushaf | P2 | 🔵 Proposed

- Full Quran text (Uthmani script) displayed page-by-page
- Tap any Ayah to link to memorization records
- Highlight memorized portions visually
- Teacher can share specific Mushaf pages during sessions (screen share)

---

### F-010 · Student & Teacher Dashboards | P2 | 🔵 Proposed

- Student dashboard for own attendance, grades, notes, and memorization progress
- Teacher dashboard for all members learning with the teacher
- Circle-level dashboard for progress and attendance per circle
- No parent-linked accounts in MVP

---

### F-011 · Reports & Statistics | P2 | 🔵 Proposed

- Teacher: per-student report covering attendance %, grades distribution, memorization progress
- Student: self-view of own report
- PDF export for sharing with students or institutions
- Charts: line graphs, heatmaps, Quran completion wheels

---

### F-012 · Multi-language Support | P2 | 🔵 Proposed

Priority languages: Arabic (default), English, Urdu, Malay, Turkish, French
- Full RTL support for Arabic, Urdu
- Locale-aware date/time formatting

---

### F-013 · AI Tajweed Assessment | P3 | 🔵 Proposed

- Record student's recitation audio
- Run through AI model trained on Tajweed rules
- Return error report: makhraj issues, madd lengths, tanwin, etc.
- Grade suggestion to assist teacher

---

### F-014 · AI Memorization Planner | P3 | 🔵 Proposed

- Analyze student's historical pace (Ayahs memorized per week)
- Suggest a personalized memorization plan to complete a target (e.g., 1 Juz in 3 months)
- Adaptive: adjust plan based on actual performance

---

### F-015 · Certificate System | P3 | 🔵 Proposed

- Teacher awards certificate upon completion of Juz, full Quran (Khatm), or milestone
- Digitally signed certificate with QR code for verification
- Shareable as PDF or image

---

### F-016 · Desktop App | P3 | 🔵 Proposed

- Flutter Desktop builds for Windows, macOS, Linux
- Targeted at teachers who prefer desktop for session management

---

### F-017 · 🏢 Institutional Platform | P3 | 🔵 Proposed

The most significant long-term feature. See [Executive Summary §1.5](#15-future-institutional-platform-منصة-المؤسسات) for full description.

---

## 4. Technical Architecture Overview

See [ARCHITECTURE.md](ARCHITECTURE.md) for the complete technical specification.

### 4.1 Communication Protocols Summary

| Protocol | Used For |
|----------|---------|
| HTTPS / REST | All standard CRUD: auth, circle management, user profiles, progress records |
| WebSocket (Go) | Real-time chat, presence/online status, queue updates, in-app notifications |
| WebRTC (via LiveKit) | Audio streaming in live sessions (video is post-MVP, feature-flagged) |
| FCM | Push notifications when app is in background or closed |

### 4.2 High-Level System Diagram

```
┌─────────────────────────────────────────────────────────┐
│                     Client Apps                          │
│          Flutter (iOS / Android / Web)                   │
└──────────┬──────────────────┬───────────────────────────┘
           │                  │
    REST + WebSocket    WebRTC (LiveKit)
           │                  │
┌──────────▼──────────┐       │
│     Go Backend      │       │
│  ┌───────────────┐  │       │
│  │  REST API     │  │       │
│  │  (Gin/Echo)   │  │       │
│  └───────┬───────┘  │       │
│  ┌───────▼───────┐  │       │
│  │  WebSocket    │  │       │
│  │  Hub (Chat,   │  │       │
│  │  Queue, Notif)│  │       │
│  └───────┬───────┘  │       │
│          │           │       │
└──────────┼──────────┘       │
           │                  │
    ┌──────▼──────┐    ┌──────▼──────┐
    │ PostgreSQL  │    │LiveKit SFU  │
    │ (Primary DB)│    │(Self-hosted)│
    └─────────────┘    └─────────────┘
           │
    ┌──────▼──────────────┐
    │  Supporting Services │
    │  ┌──────────┐        │
    │  │  MinIO   │ Files  │
    │  └──────────┘        │
    │  ┌──────────┐        │
    │  │ Firebase │ Auth+  │
    │  │ Auth/FCM │ Push   │
    │  └──────────┘        │
    └──────────────────────┘
```

---

## 5. Deployment Strategy

See [DEPLOYMENT.md](DEPLOYMENT.md) for the full phase-by-phase plan with server configurations.

| Phase | Users | Infrastructure | Estimated Cost/Month |
|-------|-------|---------------|---------------------|
| **1 — MVP** | 10–50 | Single Hetzner CX22, Docker Compose | ~$8–12 |
| **2 — Growth** | 100–500 | 2 Hetzner servers (App + LiveKit) | ~$23–30 |
| **3 — Scale** | 500–5,000 | Kubernetes on Hetzner/DigitalOcean | ~$100–200 |
| **4 — Global** | 5,000+ | AWS/GCP multi-region | $500+ |

**Phase 1 Strategy:** Start on a single Hetzner CX22 ($4–6/month, 2 vCPU / 4GB RAM) running all services via Docker Compose. This is sufficient for internal testing with 10–50 users. The goal is to validate the product, not the infrastructure.

---

## 6. Release Strategy

| Phase | Timeline | Channel | Target |
|-------|----------|---------|--------|
| **Internal Alpha** | Month 1–3 | Android APK (sideloaded) | Core team + 5–10 pilot teachers |
| **Beta** | Month 4–6 | Google Play (Open Beta) + Apple TestFlight | 50–200 early adopters |
| **iOS Public** | Month 6–8 | Apple App Store | Full iOS launch |
| **Web** | Month 8–12 | Flutter Web (Progressive Web App) | Desktop users, institutions |
| **Institutional** | Month 12+ | B2B outreach | Quran schools and centers |

**Note on iOS:** Apple Sign-In is **mandatory** when Google Sign-In is offered on iOS. This will be implemented from the start.

---

## 7. Business Model

**Guiding Principle:** This is a religious and educational platform. Monetization must never compromise the spiritual mission or create barriers for underprivileged communities.

| Tier | Price | Features |
|------|-------|---------|
| **Free** | $0/month | Core operations; circle/student caps remain open until pilot outcomes; audio sessions (no recording, no video) |
| **Teacher Pro** | $5–8/month | Unlimited circles, unlimited students, video (post-MVP, feature-flagged), recording (post-MVP after privacy framework), AI features (future), advanced reports |
| **Institution** | Custom/year | All Pro features for all teachers + institution dashboard + custom branding |

**No ads.** Ever. This is non-negotiable for a Quran-focused platform.

---

## 8. Timeline — 12-Month Plan

### Month 1–2: Foundation
- [ ] Finalize all planning documents (this phase)
- [ ] Set up development environment: Go backend scaffold, Flutter project scaffold
- [ ] PostgreSQL schema design and migrations
- [ ] Firebase Auth integration (email, Google, Apple)
- [ ] Basic user registration/login (Flutter UI)
- [ ] Circle creation and invitation (backend API)
- [ ] Hetzner server provisioning + Docker Compose setup

### Month 3: Core Chat & Circles
- [ ] Circle member management (join, leave, roles)
- [ ] WebSocket server (Go) for real-time messaging
- [ ] Group chat — text messages, image attachments
- [ ] Voice note recording and playback
- [ ] Push notifications via FCM (basic)

### Month 4: Live Sessions (LiveKit)
- [ ] LiveKit server deployment on Hetzner
- [ ] Go backend: LiveKit room creation + JWT token generation
- [ ] Flutter: `livekit_client` integration
- [ ] Basic audio session (teacher-controlled mute, hand raise)
- [ ] Audio-only hardening (no video publish paths in MVP clients/tokens)

### Month 5: Recitation Queue System
- [ ] Queue backend: real-time queue state via WebSocket
- [ ] Queue ordering modes (join order, manual)
- [ ] Student status (waiting/reciting/completed/skipped)
- [ ] Queue reset and round management
- [ ] Grading system
- [ ] Turn notification ("You're next!", "Your turn!")

### Month 6: Scheduling & Attendance
- [ ] Weekly schedule per circle
- [ ] Push notification reminders
- [ ] Auto-attendance from LiveKit room join events
- [ ] Manual attendance override

### Month 7: Progress Tracking & Reports
- [ ] Memorization log (linked to queue history)
- [ ] Visual Quran map
- [ ] Progress charts (weekly/monthly)
- [ ] Basic PDF report export

### Month 8: Beta Launch Preparation
- [ ] Google Play Beta deployment
- [ ] Apple TestFlight deployment
- [ ] Onboarding flow polish
- [ ] Bug bash and performance optimization
- [ ] Admin dashboard (teacher web view)

### Month 9–10: Post-Beta Improvements
- [ ] User feedback integration
- [ ] Performance tuning (WebSocket scalability)
- [ ] Student + teacher dashboards (P2)
- [ ] Multi-language: English + Arabic complete

### Month 11: App Store Launch
- [ ] Apple App Store submission and review
- [ ] Marketing materials (Arabic + English)
- [ ] Pilot program with 3–5 Quran schools

### Month 12: Foundation for Growth
- [ ] Kubernetes migration planning
- [ ] Institution platform architecture design
- [ ] AI Tajweed assessment research spike
- [ ] Flutter Web deployment (PWA)
- [ ] Analytics dashboard

---

*This document is the source of truth for project planning. Business-facing updates should be reflected in [arabic/PLAN_AR.md](arabic/PLAN_AR.md).*

*See [SYNC_GUIDE.md](SYNC_GUIDE.md) for the documentation synchronization policy.*
