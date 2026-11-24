# MVP Decision Register

> All frozen decisions for the Halaqaty MVP. Binding on all implementation. To change a decision, create an ADR in [`../../engineering/architecture/adr/`](../../engineering/architecture/adr/) and update this file with an entry in the Amendment Log.

**Last updated:** 2026-11-22

---

## 1. Authentication & Accounts

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-001 | Phone-only accounts (no email)? | **No.** Require email or social provider. Phone is supplementary verification only. | Simplifies auth; avoids OTP infrastructure in MVP; all pilot users have email or Google/Apple. |
| OQ-002 | Teacher identity verification? | **Optional.** Not required in MVP. | Trust built organically in pilot; formal verification adds friction without benefit at this scale. |
| OQ-003 | Session token expiry? | **Firebase default: 1hr auto-refresh.** Backend enforces 30-day inactivity logout. | Firebase handles silent refresh; 30-day rule protects abandoned devices. |
| OQ-035 | Authentication, device sessions, and logout ownership? | **Flutter Firebase Auth owns password validation, identity creation, sign-in, and Firebase token refresh.** The Go API verifies Firebase ID tokens and owns durable per-device sessions. The backend never accepts passwords or returns Firebase tokens. Current-device logout revokes one backend session; logout-all-devices is a later explicit endpoint that revokes all sessions. | Preserves the Firebase identity boundary while allowing immediate server-side revocation and 30-day inactivity enforcement. |
| OQ-036 | Initial circle roles, invitations, and supervisor management? | Roles are per-circle only. The creator initially accepts teacher responsibility by creating the circle. Once another named person accepts teacher responsibility, an authorized other manager may change the creator to supervisor (organizer). Every active circle must retain at least one accepted teacher. Assigning another person as a teacher requires a named, role-bound invitation and that person's acceptance. A recipient who is already a member becomes a teacher only after accepting; a pending invitation creates no new membership or teacher permissions and an existing member retains their prior role. There is no distinct backup-teacher role: additional accepted teachers have the ordinary teacher role. An optional backup supervisor may still be assigned at creation. For an active circle, a teacher or supervisor may invite a person as teacher or student, while a student may invite only a student. Named invitations do not expire; an active teacher or supervisor may revoke a pending invitation, and a new invitation for the same circle and recipient replaces the previous pending invitation. Acceptance is recipient-only and single-use, with archive, authority, capacity, and membership eligibility rechecked atomically. Managers cannot change their own role or leave the circle without a teacher. | Ensures every teacher accepts the responsibility, supports optional shared teaching and later replacement/addition, and preserves per-circle authorization, self-lockout protection, and the final-teacher safeguard. |
| OQ-056 | Which identity remains on retained history after account deletion? | **Display name only.** Erase email, full name, phone, avatar reference, and other non-retained profile data while preserving teaching, chat, and recitation history. | Preserves readable educational provenance without retaining unrelated identity fields. Approved by Karim 2026-09-26. |
| OQ-057 | How recent must reauthentication be for account deletion? | A verified Firebase `auth_time` no more than five minutes before confirmation; token issue time is not a substitute. | Requires a fresh identity check for irreversible closure. Approved by Karim 2026-09-26. |
| OQ-058 | Which accounts may use the current deletion batch? | Student-only. Block active teacher/supervisor memberships and owned active circles until the F-008 durable notification path is deployed and operational; do not require handset receipt. | Avoids orphaning managed circles. Approved by Karim 2026-09-26 and refined by ADR-027. |
| OQ-059 | Which unchecked F-001 controls are included in this batch? | Account deletion and directly related privacy controls only. Avatar upload, email verification, Google/Apple sign-in, and password reset remain out of scope. | Keeps the approved batch limited to the requested gaps. Approved by Karim 2026-09-26. |
| PRD-4 | Co-teacher model (distinct role vs supervisor)? | **Deferred post-pilot.** MVP: teacher + supervisor only. No co-teacher role. | Adds role complexity without proven need; supervisor covers 95% of pilot use cases. |

---

## 2. Circle Management

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-005 | Cross-circle role combinations? | **Yes.** Roles are fully independent per circle. A user can be teacher in one circle and student in another. | This is real-world reality for a sheikh who teaches and also studies. |
| OQ-006 | Circle ownership on account deletion? | When enabled, require a designated supervisor, archive each owned active circle, and atomically create durable member history/notification intents with account closure; no automatic transfer. Required notices bypass ordinary circle-archive preferences but follow quiet hours. A post-commit FCM outage is retried and does not undo closure. **Manager deletion stays blocked until F-008 is deployed and operational.** | Prevents silent data loss and requires a conscious handoff; the current F-001 batch remains student-only. See ADR-024/027. |
| PRD-3 | Institution onboarding model? | **Self-serve in MVP.** Admin receives invite code, manages own school. Assisted onboarding deferred. | Reduces operational overhead; pilot scale doesn't need white-glove onboarding. |

---

## 3. Queue System

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-007 | Student "opt-out" of a round? | **Allowed and session-configurable.** Default: persist a pending request and require teacher/supervisor approval. A manager may configure automatic approval for that session. Every request/outcome is logged and never penalized. | Preserves the humane default while allowing managers to run the live circle without queue-policy friction. |
| OQ-008 | Per-student recitation timer? | **No timer in MVP.** Teacher manages timing verbally. | Timers create test-anxiety during Quran recitation. Teacher judgment is preferred. |
| OQ-009 | Pre-set queue before session starts? | **Yes.** Teacher can pre-order the queue before starting the session. | Reduces dead time at session start; teachers typically know class order in advance. |
| OQ-010 | Late-joining student position? | **Session-configurable.** Default `present_at_activation`: apply the pre-set relative order to eligible students present when the round activates and append later joiners. Optional `all_active_students`: create all active student positions from the start and retain each pre-set position on join. | Keeps the predictable append default while supporting managers who want a complete roster prepared in advance. |
| OQ-011 | Double-queue per student per round? | **No.** One position per student per round. Use sequential rounds for multiple recitations. | Simplifies queue state machine; multiple rounds is the correct UX pattern. |
| OQ-053 | Which F-003 rules are configurable per session? | **Managers may configure queue population, unfinished-entry finalization, opt-out approval, grade/note visibility, and grade/note correction.** Defaults are `present_at_activation`, `mark_unfinished_skipped`, `approval_required`, `managers_and_student`, and `audited_any_time`. Changes are prospective and audited. F-003 never blocks F-005 session end. | Gives teachers/supervisors operational flexibility without making security, data integrity, or media safety optional. |
| OQ-054 | Does the recitation queue control student microphone publishing? | **No.** F-003 records the voluntary order and manager-set displayed turn status only. Students may publish audio freely in an authorized live session; speaking out of order has no automatic queue or media consequence. | Quran circles rely on mutual respect and teacher guidance. Queue coordination must not turn ordinary participation into a technical restriction. |

---

## 4. Chat & Messaging

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-012 | Announcement-only channels? | **No.** Use pinned messages for circle-wide announcements. | Reduces schema complexity; pinning serves the same purpose at MVP scale. |
| OQ-013 | Voice message maximum length? | **5 minutes (300 seconds).** Max file size: 20 MB. | Covers all practical recitation feedback; prevents storage abuse. |
| OQ-014 | Emoji reactions? | **Deferred to P2.** Not in MVP scope. | Nice-to-have; adds implementation complexity without core value at launch. |
| OQ-055 | Which feature owns background chat notifications? | **F-008 owns trigger creation, preferences, FCM transport, and all background/closed-state delivery.** F-004 owns durable chat messages and foreground WebSocket delivery only. | Keeps chat independently usable and avoids building a partial notification pipeline twice. |

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
| OQ-038 | Individual unmute and student publishing? | **Students receive audio-publish entitlement when they join an authorized live session.** Individual unmute restores an existing publisher only; teachers and supervisors retain explicit mute, mute-all, remove, lock, and end controls. F-003 never changes publishing permission. | Keeps Zoom-like voluntary participation while preserving separate moderator safety controls. |
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
| OQ-018 | Non-recurring (one-off) sessions? | **Yes.** An active circle teacher or supervisor can create sessions not linked to a recurring schedule (F-006 manager scope clarified by OQ-060). | Covers Ramadan intensives, special sessions, and makeup classes. |
| OQ-019 | Timezone storage? | **UTC in DB.** IANA timezone string stored per user profile. All display in user's local timezone. | Standard best practice; avoids DST ambiguity. |
| OQ-060 | F-006 recurrence, manager rights, overlaps, cancellation, and planned duration? | **Both selected weekdays on weekly/biweekly patterns and arbitrary positive day/week intervals or explicit dates.** Active teachers and supervisors may schedule and cancel unstarted planned occurrences. Same- and cross-circle overlaps warn but do not block a manager's choice. Planned duration is 1–44,640 minutes (31 days), with no restriction on permitted start dates or times; F-005's live-session limit is unchanged. Cancellation stays separate from F-005's three-state lifecycle; completed history is retained. F-006 may run a labelled partial pilot, but full push acceptance waits for F-008. | Records Karim's F-006 clarification and 2026-09-27 ADR-026 approval. The duration bound keeps calendar queries finite without imposing a future scheduling horizon. See ADR-025, ADR-026 and the F-006 spec. |

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

## 11. Provider Replaceability

| ID | Question | Decision | Rationale |
|---|---|---|---|
| PROVIDER-BOUNDARY | How should storage and live audio remain replaceable? | **F-018: targeted provider boundaries, current providers only.** Secure chat attachments currently use MinIO; live audio currently uses LiveKit. Replacement is a development/deployment-time concern. Keep application contracts independent of provider SDKs, preserve deletion/recovery and configuration behavior, and retain direct injection of one adapter per capability. No new vendor, runtime registry, settings UI, database migration, or live failover is included. | Approved by Karim on 2026-09-16 after reviewing the plan with Architect and Tech Lead. Spec-Kit owns the feature artifacts; Superpowers supplies implementation discipline. Current constitutional provider choices and ADR-015's future replacement gate remain in force. |

---

## 12. Mobile UI/UX Numbering

The 2026-09-17 `APP-SHELL / UI-OVERHAUL` amendment approved Wave 0. The
subsequent F-019 decision consolidates Waves 0–5 into one feature while
retaining each wave as a coherent implementation and review batch.

| ID | Decision | Rationale |
|---|---|---|
| F-019-NUMBERING | **F-019 is the sole umbrella feature for Waves 0–5.** | Keeps the complete design modernization traceable through one Spec-Kit lifecycle while preserving wave-level implementation and review batches. Explicitly approved by Karim on 2026-09-22. |
| F-019-UNIMPLEMENTED-ACTIONS | **An intentionally planned design action with no implemented behavior remains visible and shows one shared localized under-implementation notice; it performs no navigation, controller/API call, or state mutation. Unsupported data and unplanned actions remain omitted.** | Preserves the approved design direction without faking capability or expanding F-019 backend scope. Explicitly approved by Karim on 2026-09-22. |

## 13. Notifications

| ID | Question | Decision | Rationale |
|---|---|---|---|
| OQ-061 | How do notification controls affect delivery? | Configurable event types are global per-user settings defaulting ON. OFF mutes F-008 push and foreground alert presentation but keeps history/unread; always-on types cannot be disabled. Current preference is checked before first push dispatch; a suppressed occurrence is not replayed after re-enable. | Gives users control over alerting without losing history or suppressing source content. |
| OQ-062 | What are quiet-hours and unread rules? | Default 22:00–07:00 in the stored profile IANA timezone across devices. Quiet hours suppress/drop push, including always-on types, while history/unread remains. Opening one notice marks it read; opening the list does not; explicit read-all is supported. In-app unread count is exact; native badge sync applies where platform support/settings permit it. | Makes cross-device behavior predictable and preserves a server-authoritative unread state. |
| OQ-063 | How long is notification history retained? | Display the last 30 days. Pending required manager-deletion dispatch intents remain durable until terminal handling; they are not evidence of handset receipt. | Keeps user-facing history bounded without losing required durable work. |
| OQ-064 | Do manager-deletion notices follow ordinary preferences? | No. Required member notices bypass the ordinary circle-archive preference, remain subject to quiet hours, and are inserted atomically with archive/closure. Database failure aborts the mutation; transient post-commit FCM failure does not. | Ensures required member notice intent cannot be silently opted out of or falsely treated as delivered. |
| OQ-065 | What if no device token exists at push dispatch? | Hold the intent until a token registers. Ordinary events expire within 24 hours of the source event; required manager-deletion pushes expire within 30 days, shortened by an earlier source cutoff. Recheck current preference, quiet hours, and eligibility before dispatch. F-006 reminders expire at the next closer configured offset, with the last expiring at occurrence start. | Allows a newly registered device to receive still-relevant events without indefinite or stale replay. Approved by Karim 2026-10-17. |
| OQ-066 | When does a circle-invitation notification fire? | When an authorized inviter sends a role-bound invitation to its named recipient. F-008 owns history and delivery; use the committed invitation transition under the existing F-002 authorization and role rules. Do not infer the trigger from reusable invite-code sharing or circle joining. | Gives the intended invitee notice while an action is pending and avoids treating a reusable link as an addressed invitation. Approved by Karim 2026-10-17. |

---

## Amendment Log

| Date | Decision ID | Old Value | New Value | Rationale | ADR |
|---|---|---|---|---|---|
| 2026-10-17 | OQ-006, OQ-061–OQ-066 / F-008 | F-008 delivery, preference/unread behavior, quiet-hours clock, native badge scope, manager-deletion notice policy, and invitation trigger unresolved | Approved F-008 lifecycle decisions: profile IANA timezone; OFF mutes both F-008 alert surfaces while history/unread remain; current preference checked at first dispatch; 30-day history; exact in-app count and best-effort native badge where supported; mandatory deletion notices bypass ordinary archive preference and are transactionally durable; tokenless intents wait under 24-hour/30-day bounds with earlier source cutoffs; circle-invitation notification fires when the role-bound invitation is sent to its named recipient | Karim's clarifications and accepted agent recommendations. F-008 owns the notification projection; source integration follows F-002 authorization and role rules. | ADR-027 |
| 2026-09-27 | OQ-060 / F-006 planned duration | No explicit planned-duration bound | 1–44,640 minutes (31 days), without a start-date or time-of-day scheduling cap | Karim approved the bounded planning interval with the F-006 persistence design; F-005 live limits remain unchanged. | ADR-026 |
| 2026-09-27 | OQ-060 / F-006 | Weekly, teacher-only planning and a misleading Completed → Cancelled sequence | Expanded recurrence; teacher/supervisor scheduling; advisory overlaps; cancellation separate from F-005 lifecycle; F-008 push dependency retained | Reconciles Karim's approved F-006 clarification with canonical product and architecture boundaries without inventing the future schema/API. | ADR-025 |
| 2026-09-26 | OQ-006, OQ-056–OQ-059 | Deletion retention, reauthentication, eligible account types, and batch scope unspecified | Display name only remains; verified `auth_time` must be within five minutes; current path blocks managers; other unchecked F-001 flows stay out of scope | Implements the approved student-only account deletion amendment without bypassing F-008 notification delivery. | ADR-024 |
| 2026-09-22 | F-019-UNIMPLEMENTED-ACTIONS | Prototype actions without existing behavior were omitted with other unsupported content | Intentionally planned actions remain visible and show the shared localized under-implementation notice with zero product side effects; unplanned actions remain omitted | Makes future scope visible without simulating completion or adding backend behavior. | — |
| 2026-09-22 | F-019-NUMBERING | Wave 0 approved separately; remaining redesign planned as separate feature lifecycles | F-019 is the sole umbrella for Waves 0–5 | Keeps the complete modernization in one traceable lifecycle while retaining coherent wave-level implementation and review batches. | — |
| 2026-09-16 | PROVIDER-BOUNDARY / F-018 | MinIO SDK-shaped storage seam; LiveKit SDK construction outside its adapter | Approved provider adapter boundary refactor preserving current providers, public behavior, and existing safety guarantees | Makes future approved replacements local to their integrations. Spec-Kit plus Superpowers workflow approved by Karim. | ADR-023 |
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
| 2026-08-23 | OQ-007, OQ-010, OQ-053 | Fixed opt-out approval and late-join append behavior; other queue policies unspecified | Per-session queue policy with safe defaults, prospective audited manager changes, immutable safety invariants, and non-blocking F-005 session end | Lets managers adapt each circle session without weakening authorization, queue consistency, progress truth, or media safety. Approved by Karim 2026-08-23. | ADR-018 |
| 2026-08-28 | OQ-038, OQ-054 | F-003 owned turn-based student publishing; students joined listen-only | Students publish audio freely in authorized live sessions; F-003 is a voluntary ordered queue and displayed-turn tracker, while F-005 retains explicit moderator controls | Matches the intended Zoom-like halaqa experience without removing teacher/supervisor safety controls. Approved by Karim 2026-08-28. | ADR-020 |
| 2026-09-03 | OQ-036 / F-002 | Invite acceptance always created a student membership and only teachers shared circle invites | Active teachers and supervisors may invite a teacher or student; active students may invite a student only; the selected role is bound to the invitation and cannot be altered during acceptance | Approved by Karim while starting F-004 so chat eligibility can rely on explicit, active, circle-scoped memberships without introducing global roles. | ADR-010 |
| 2026-09-03 | OQ-055 / F-004 | F-004 created a background-notification trigger for later F-008 delivery | F-008 owns the complete background notification pipeline; F-004 retains durable messages and foreground realtime delivery only | Approved by Karim during F-004 clarification to minimize changes and keep notification ownership cohesive. | ADR-021 |
| 2026-09-17 | APP-SHELL / UI-OVERHAUL | Mobile root was a developer bridge (implemented-features index) on the default Flutter theme, with no logo, fonts, or app shell | Wave 0 of the mobile UI overhaul approved: Material 3 light+dark theme from DESIGN.md tokens, bundled Cairo/Poppins fonts, replaceable Islamic logo placeholder, 4-tab shell (Home/Circles/Chats/Profile) on go_router; Chats tab lists my circles' group chats client-side only; unified DM inbox deferred (would need a new conversations API + ADR) | Foundation for the approved full per-feature UI redesign roadmap. Approved by Karim 2026-09-17. | - |

| 2026-10-25 | F-008 T023 / circle_invitation | Named-recipient invitation-send transition does not exist in F-002 (only invite-code/link flows); the approved `circle_invitation` matrix row was therefore unimplementable without new schema/endpoints/artifacts | **Option B — ship F-008 without the invitation row; T023's invitation portion stays open as a documented gap.** All `circle_invitation` notification plumbing (EventCircleInvitation, SourceInvitation, CreateInTx, dispatcher, migration CHECK allowlist, openapi enums) ships and stays inert; the projection lands as a single same-transaction `CreateInTx` call inside the committed invitation transition when a named-invitation feature cycle (F-002 reopen or new feature, requiring an ADR for invitation state and an actionable invitation tap target) is approved. No inference from invite-code sharing or joining (F-008 FR-012; ADR-010 role binding preserved). Mobile taps on invitation items route to the discover/join entry or none until that cycle decides the target. | Approved by Karim 2026-10-25. | — |
| 2026-10-25 | F-008 student_joined_session | Notification type constant and matrix/preference entries exist but no session-join projection exists (F-005 `JoinSession` has none wired) and recipient scope is unspecced | **Deferred, same pattern as circle_invitation:** documented known gap beside the invitation row; no standalone task opened. The projection lands as a one-call integration inside a future F-005 participation cycle, which decides recipients at that time. | Approved by Karim 2026-10-25. | — |
| 2026-10-25 | F-008 notification copy language | Server-stored title/body strings are English generic lock-screen-safe copy while the product is Arabic-first | **Keep server copy English/generic as fallback hints; localization is a T026–T029 mobile concern.** The Flutter client renders Arabic-first copy from ARB files keyed off notification `type` + `source_ref.type`, using server title/body only as fallback, keeping the REST contract locale-neutral for future clients and non-Arabic locales. | Approved by Karim 2026-10-25. | — |
| 2026-11-22 | OQ-036 / F-002 teacher consent | Circle creation could directly assign other users as active teachers, and later manager role changes could assign teacher without the target's acceptance | The creator defaults to teacher when no other person has accepted that responsibility; the creator may instead be a supervisor (organizer) when another named person has accepted it. Every active circle must retain at least one accepted teacher. Additional teachers are optional and become teachers only after accepting a named, role-bound invitation; an existing member's promotion to teacher also requires acceptance. There is no separate backup-teacher role. The optional backup supervisor remains separate. Active teachers/supervisors can invite teachers or students; students can invite students only. Managers may later add or replace teachers, subject to acceptance and the final-teacher safeguard. | Explicitly approved by Karim 2026-11-22. | ADR-010 |

| 2026-11-22 | OQ-036 / named invitation lifecycle | Expiry, revocation, and replacement were unspecified | No expiry. Active teachers and supervisors may revoke pending invitations; a new invitation for the same circle and recipient replaces the prior pending invitation. Immutable-role, recipient-only, single-use acceptance rechecks eligibility atomically. | Explicitly approved by Karim. | ADR-010 |

*Any change requires: (1) a new or updated ADR in `../../engineering/architecture/adr/`, (2) an entry in the Amendment Log above, (3) approval from Karim.*
