# MVP Decision Register

> All frozen decisions for the Halaqaty MVP. Binding on all implementation. To change a decision, create an ADR in [`../../engineering/architecture/adr/`](../../engineering/architecture/adr/) and update this file with an entry in the Amendment Log.

**Last updated:** 2026-08-19

---

## 1. Authentication & Accounts

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-001 | Phone-only accounts (no email)? | **No.** Require email or social provider. Phone is supplementary verification only. | Simplifies auth; avoids OTP infrastructure in MVP; all pilot users have email or Google/Apple. |
| OQ-002 | Teacher identity verification? | **Optional.** Not required in MVP. | Trust built organically in pilot; formal verification adds friction without benefit at this scale. |
| OQ-003 | Session token expiry? | **Firebase default: 1hr auto-refresh.** Backend enforces 30-day inactivity logout. | Firebase handles silent refresh; 30-day rule protects abandoned devices. |
| OQ-035 | Authentication, device sessions, and logout ownership? | **Flutter Firebase Auth owns password validation, identity creation, sign-in, and Firebase token refresh.** The Go API verifies Firebase ID tokens and owns durable per-device sessions. The backend never accepts passwords or returns Firebase tokens. Current-device logout revokes one backend session; logout-all-devices is a later explicit endpoint that revokes all sessions. | Preserves the Firebase identity boundary while allowing immediate server-side revocation and 30-day inactivity enforcement. |
| OQ-036 | Initial circle roles and supervisor management? | Roles are per-circle only. At creation, the creator may assign existing registered users as one or more teachers and one optional backup supervisor; if no teacher is selected, the creator becomes teacher, otherwise the creator is supervisor. Invite acceptance creates a student membership. Any teacher or supervisor may change another member's teacher/supervisor/student role, but cannot change their own role or leave the circle with no teacher. | Supports shared teaching while preventing global-role escalation, self-lockout, and teacherless circles. |
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
| OQ-037 | F-005 room moderation roles? | **Teachers and supervisors have identical F-005 moderation rights:** start/end, lock/unlock, mute-all, mute/unmute an existing audio publisher, and remove. | Supervisors support teachers in running a circle. This does not grant students publishing permission. |
| OQ-038 | Individual unmute and student publishing? | **Individual unmute is supported only for a participant who already has audio-publish entitlement.** It never grants a student publish permission; F-003 remains the sole owner of turn-based student publishing and must revalidate this rule. | Preserves teacher/supervisor moderation without weakening turn-based student audio safety. |
| OQ-039 | Shared realtime authorization model? | **Use one generic authenticated realtime ticket endpoint, `POST /api/v1/realtime/tickets`, for authorized circle and session topic subscriptions.** It replaces the session-only ticket model; the WebSocket hub revalidates authorization. | F-005 supplies common transport while F-004 can reuse it for circle chat without an active live session. |
| OQ-040 | Live presence versus attendance persistence? | **F-005 uses `actual_start` / `actual_end` and a dedicated `session_participant_presence` model for durable live presence and hand state.** F-006 owns the separate `session_attendance` policy, classification, and overrides. | Keeps authoritative realtime facts distinct from future attendance policy and avoids coupling F-005 to F-006. |
| OQ-041 | F-005 session creation scope? | **F-005 creates ad-hoc sessions only; F-006 owns scheduled-session creation.** | Prevents scheduling scope from entering the live-session foundation. |
| OQ-042 | Lock behavior during reconnect? | **Lock blocks new joins but permits an eligible participant who joined before the lock to reconnect, unless removed or the session ended.** | Preserves graceful mobile recovery without admitting new attendees. |
| OQ-043 | Hand-raise eligibility? | **Every active session participant may raise or lower a hand.** | Supports practical room coordination without creating queue behavior. |
| OQ-044 | Automatic session-end attribution? | **Duration-limit and idle-timeout endings record their machine-readable reason and have no human `ended_by` attribution.** | Keeps automatic lifecycle actions truthful and auditable. |
| OQ-045 | Realtime session-topic access? | **Circle members receive circle topics; session presence and hand topics require a successful authorized join.** | Limits participant-presence disclosure to people in the room. |
| OQ-046 | Generic ticket scope? | **One ticket authorizes all currently eligible circle topics; session-topic access is added only after an authorized join and is revalidated by the hub.** | Keeps shared transport simple while enforcing session privacy. |
| OQ-047 | Provider outage during recovery? | **Recoverable.** Start/join returns `503` with `ERR_MEDIA_UNAVAILABLE`, no credential, and no presence/count mutation. Flutter offers Arabic Retry/Leave; it never loops REST retries automatically. | Aligns the canonical REST contract and product journey; provider outage alone must not fabricate a terminal session end. |
| OQ-048 | Media-room identity for crash recovery? | **Stable, opaque, non-guessable derivation from the session ID using a backend-only HMAC key.** The literal session ID is never used as a room name or public field. | ADR-015 and architecture require deterministic recovery while media-room names remain unguessable. |
| OQ-049 | Reconciliation concurrency boundary? | **One session-scoped PostgreSQL advisory lock covers start, join/reconnect through credential issuance, end, and reconciliation.** Background reconciliation uses a try-lock and skips busy sessions. | Prevents a room close/ensure race from leaving an active session pointing at a closed room or ghost presence consuming capacity. |
| OQ-050 | Reconciliation persistence and cadence? | **No recovery table or retry columns in MVP.** Sweep at startup and every 30 seconds; process at most 25 candidates per lifecycle state, with one 3-second provider attempt per candidate; failures retry on the next sweep. | Existing lifecycle, timestamps, and room reference are sufficient at MVP scale; bounded repeated idempotent work is simpler than an outbox. |
| OQ-051 | End ordering and provider-close failure? | **Persist `ended` first and return the authoritative ended session.** Provider close is idempotent background cleanup; a close failure is redacted telemetry, not a failed end response. | Keeps lifecycle truth durable and makes retries safe under provider outages; allowed end reasons remain unchanged. |
| OQ-052 | Client reconnect policy? | **WebSocket retries use 1s, 2s, and 4s backoff, then stop automatic retries and show “Tap to rejoin.”** Near-expiry means two minutes before expiry; authenticated start/join issues a fresh connection. | Provides bounded recovery without infinite loading while preserving the existing credential boundary. |

### Recovery clarification record — 2026-08-19

These decisions were frozen after a Spec-Kit consistency review of the F-005
specification, plan, research, data model, feature-local and canonical REST/
WebSocket contracts, ADR-015/016, the architecture reference, and the product
journey. The review found four conflicts: provider outage was described as both
terminal and retryable; the implementation generated random room references
despite deterministic reconciliation; end cleanup could turn a committed end
into an error; and the reconnect test did not reach the durable reconnect path.
The decisions above resolve those conflicts without adding a recovery table,
provider registry, job framework, or new lifecycle end reason.

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
| 2026-06-30 | GRADE-ENUM | 4-grade: `excellent/good/needs_improvement/repeat` (ARCHITECTURE.md) / 6-grade: `excellent/very_good/good/acceptable/needs_review/repeat` (FEATURES.md) | **5-grade canonical:** `excellent/good/acceptable/needs_review/repeat` | Resolved mismatch between ARCHITECTURE.md (4-grade) and FEATURES.md F-003 (6-grade). Merged `very_good` into `good`; renamed `needs_improvement` → `needs_review` for clarity. Approved by Karim 2026-06-30. | ADR-013 |
| 2026-06-30 | OQ-027 | Open | Fixed globally — same Surah status threshold rules for all circles | Simpler to reason about; teacher customisation deferred | — |
| 2026-06-30 | OQ-028 | Open | "Practiced" = only `completed` turns count; `skipped`/`opted_out` do NOT | Semantic correctness; a skipped turn is not a recitation event | — |
| 2026-06-30 | OQ-029 | Open | Teacher CAN see student's cross-circle progress (not restricted to own circle) | Teachers need full student context for informed guidance | — |
| 2026-06-30 | OQ-030 | Open | 🚩 flag triggers at ≥7 consecutive sessions attended with zero completed turns | 7 sessions = ~2 weeks; enough signal without being too sensitive | — |
| 2026-06-30 | OQ-031 | Open | ⚠️ stale badge on `memorized` Surahs after 30 days without revision | Aligns with traditional weekly revision expectation | — |
| 2026-06-30 | OQ-032 | Open | Add `surah_id INT FK` to `memorization_progress`; keep `surah_name` deprecated until v1.1 | Normalises schema; enables Quran Map JOIN without name-matching hacks | — |
| 2026-06-30 | OQ-033 | Open | Soft degradation — `memorized` stays but shows ⚠️ badge after 30 days | Preserves student motivation; does not penalise infrequent revision artificially | — |
| 2026-06-30 | OQ-034 | Open | Medina Mushaf standard — 240 Rub' divisions for `quran_divisions` seed | Most widely used globally; matches printed Mushaf most students use | — |
| 2026-06-30 | CROSS-CIRCLE | Open | Most recent update wins for global Quran Map cross-circle conflict resolution | Simplest rule; full history always preserved for audit | — |
| 2026-07-31 | OQ-035, OQ-036 | Underspecified | Firebase/client identity boundary; backend per-device session lifecycle; teacher-owned circle role lifecycle | Removes contradictory backend password/token APIs and prevents self-assigned privileges. | ADR-009 |
| 2026-07-31 | OQ-036 | Single creator-teacher; teacher-only supervisor management | Multiple teachers, optional backup supervisor, delegated manager role changes, self-change and final-teacher safeguards | Supports the approved circle-management workflow while preserving per-circle authorization safety. | ADR-010 |
| 2026-08-07 | OQ-006 / F-002 | Circle deletion may permanently remove data | Circle retirement is archive-only; history is retained and hard deletion is prohibited | Prevents accidental loss of circle history and aligns REST DELETE with soft-state retirement. | ADR-011 |
| 2026-08-15 | OQ-037, OQ-038 | Underspecified F-005 moderator and individual-unmute behavior | Teachers and supervisors share moderation; unmute only restores an existing publisher and never grants student publishing | Protects the F-003 turn-based publishing boundary while enabling supervisor support. | ADR-016 |
| 2026-08-15 | OQ-039 | Session-scoped WebSocket ticket | Generic authenticated realtime tickets authorize circle and session topics | Lets F-004 reuse the common transport without a live-session dependency. | ADR-016 |
| 2026-08-15 | OQ-040 | `session_attendance` conflated live presence with attendance policy | Dedicated F-005 participant-presence model; F-006 owns attendance policy | Separates realtime facts from future attendance classification and overrides. | ADR-016 |
| 2026-08-16 | OQ-041–OQ-046 | Open F-005 scope, lock, hand, automatic-end, and realtime-topic questions | Ad-hoc-only F-005, pre-lock reconnect, all-participant hand raise, truthful automatic end, and joined-participant session topics | Resolves F-005 behavior while preserving F-004/F-006 boundaries and session privacy. | ADR-016 |

*Any change requires: (1) a new or updated ADR in `../../engineering/architecture/adr/`, (2) an entry in the Amendment Log above, (3) approval from Karim.*
