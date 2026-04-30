# MVP Decision Register

> All frozen decisions for the Halaqaty MVP. Binding on all implementation. To change a decision, create an ADR in [`../../../engineering/architecture/adr/`](../../../engineering/architecture/adr/) and update this file with an entry in the Amendment Log.

**Last updated:** 2026-04-26

---

## 1. Authentication & Accounts

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-001 | Phone-only accounts (no email)? | **No.** Require email or social provider. Phone is supplementary verification only. | Simplifies auth; avoids OTP infrastructure in MVP; all pilot users have email or Google/Apple. |
| OQ-002 | Teacher identity verification? | **Optional.** Not required in MVP. | Trust built organically in pilot; formal verification adds friction without benefit at this scale. |
| OQ-003 | Session token expiry? | **Firebase default: 1hr auto-refresh.** Backend enforces 30-day inactivity logout. | Firebase handles silent refresh; 30-day rule protects abandoned devices. |
| PRD-4 | Co-teacher model (distinct role vs supervisor)? | **Deferred post-pilot.** MVP: teacher + supervisor only. No co-teacher role. | Adds role complexity without proven need; supervisor covers 95% of pilot use cases. |

---

## 2. Circle Management

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-005 | Cross-circle role combinations? | **Yes.** Roles are fully independent per circle. A user can be teacher in one circle and student in another. | This is real-world reality for a sheikh who teaches and also studies. |
| OQ-006 | Circle ownership on account deletion? | **Circle archived.** Members notified. Teacher must designate a supervisor before deletion. No automatic transfer. | Prevents silent data loss; requires conscious handoff from the teacher. |
| PRD-3 | Institution onboarding model? | **Self-serve in MVP.** Admin receives invite code, manages own school. Assisted onboarding deferred. | Reduces operational overhead; pilot scale doesn't need white-glove onboarding. |

---

## 3. Queue System

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-007 | Student "opt-out" of a round? | **Allowed** with teacher/supervisor approval. Opt-out logged, not penalized. | Temporary absences are real. System must be humane. |
| OQ-008 | Per-student recitation timer? | **No timer in MVP.** Teacher manages timing verbally. | Timers create test-anxiety during Quran recitation. Teacher judgment is preferred. |
| OQ-009 | Pre-set queue before session starts? | **Yes.** Teacher can pre-order the queue before starting the session. | Reduces dead time at session start; teachers typically know class order in advance. |
| OQ-010 | Late-joining student position? | **Added to end of current active round.** | Predictable, fair; no disruption to students already queued. |
| OQ-011 | Double-queue per student per round? | **No.** One position per student per round. Use sequential rounds for multiple recitations. | Simplifies queue state machine; multiple rounds is the correct UX pattern. |

---

## 4. Chat & Messaging

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-012 | Announcement-only channels? | **No.** Use pinned messages for circle-wide announcements. | Reduces schema complexity; pinning serves the same purpose at MVP scale. |
| OQ-013 | Voice message maximum length? | **5 minutes (300 seconds).** Max file size: 20 MB. | Covers all practical recitation feedback; prevents storage abuse. |
| OQ-014 | Emoji reactions? | **Deferred to P2.** Not in MVP scope. | Nice-to-have; adds implementation complexity without core value at launch. |

---

## 5. Live Sessions

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-015 | Student video permission? | **Audio-only in MVP.** `CanPublishVideo: false` always in LiveKit tokens. | Quran recitation is audio-first; video adds bandwidth, complexity, and privacy risk. |
| OQ-016 | Max session duration? | **4 hours.** Idle room timeout: 30 min after last participant leaves. | Covers standard Friday halaqa length; idle timeout reclaims LiveKit resources. |
| OQ-017 | Recording visibility? | **Deferred.** Recording disabled until privacy framework is formally approved and merged. | See Privacy section in `../../engineering/architecture/ARCHITECTURE.md`. This is non-negotiable. |
| PRD-5 | Video feature flag rollout model? | **Per-tier once enabled.** Free: no video. Pro/Institution: video only if both the global master flag AND the tier flag are true. | Aligns with monetization; prevents accidental activation via a single-flag change. |
| PRD-6 | Recording consent and retention model? | **Explicit participant consent screen before every session.** Consent stored per-session in DB. Teacher must acknowledge liability. Default retention: 7 days. | Protects minors who may be in circles; GDPR-aligned. |

---

## 6. Scheduling

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-018 | Non-recurring (one-off) sessions? | **Yes.** Teacher can create sessions not linked to a recurring schedule. | Covers Ramadan intensives, special sessions, and makeup classes. |
| OQ-019 | Timezone storage? | **UTC in DB.** IANA timezone string stored per user profile. All display in user's local timezone. | Standard best practice; avoids DST ambiguity. |

---

## 7. Progress Tracking

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-020 | Student self-logging outside sessions? | **No in MVP.** Progress records generated from session recitations only. | Simplifies data model; teacher-verified progress has higher trust. |
| OQ-021 | Multiple passes on same section? | **Yes.** Each queue entry creates a new progress record. Full history retained. | Quran memorization is iterative; history is pedagogically valuable. |
| PRD-1 | Free tier caps? | **3 circles, 30 students per circle.** Pro tier removes all caps. | Small enough to protect server costs; large enough to prove value in pilot. |
| PRD-2 | Premium packaging after recording returns? | **Recording before analytics.** Recording is higher-value, privacy-framework-dependent. | Recording directly serves teacher workflow; analytics can follow. |

---

## 8. Mushaf & Content

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-022 | Mushaf text source? | **Tanzil.net (CC BY 3.0).** Open-source, no licensing cost. | Widely used, accurate, maintained, and trusted in the Islamic tech community. |
| OQ-023 | Audio playback in Mushaf? | **No in MVP.** Mushaf is reading-only. Audio deferred to P2. | Adds streaming infrastructure; not needed for core teacher/queue workflow. |

---

## 9. Reports & Analytics

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-024 | Auto-generated monthly reports via email? | **No in MVP.** Manual PDF export only (P2). | Email scheduling adds infrastructure; manual export covers pilot need. |

---

## 10. AI Features

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-025 | AI model for Tajweed analysis? | **Fully deferred.** No architecture decision until recording is unblocked and privacy framework approved. | Cannot build Tajweed AI without audio access; blocked by OQ-017/PRD-6. |
| OQ-026 | Audio privacy/consent for AI processing? | **Fully deferred.** Same blocker as OQ-025. | Must resolve PRD-6 (recording consent model) first. |

---

## Amendment Log

| Date | Decision ID | Old Value | New Value | Rationale | ADR |
|---|---|---|---|---|---|
| *(none yet)* | — | — | — | — | — |

---

*Any change requires: (1) a new or updated ADR in `../../engineering/architecture/adr/`, (2) an entry in the Amendment Log above, (3) approval from Karim.*
