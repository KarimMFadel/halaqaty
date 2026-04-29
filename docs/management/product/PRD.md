# Halaqaty — Product Requirements Document (PRD)

> Version: 1.0  
> Status: Draft for alignment  
> Owner: Product Management (**GPT-5.3-Codex acting as Project Manager**)  
> Related: [PRD_AR.md](../arabic/PRD_AR.md) · [PLAN.md](../planning/PLAN.md) · [FEATURES.md](./FEATURES.md) · [ARCHITECTURE.md](../../engineering/architecture/ARCHITECTURE.md) · [DEPLOYMENT.md](../../engineering/deployment/DEPLOYMENT.md) · [DEVELOPMENT.md](../../../DEVELOPMENT.md) · [AGENT_COLLABORATION_GUIDE.md](../../engineering/collaboration/AGENT_COLLABORATION_GUIDE.md)

---

## 1) Product Vision

Halaqaty (حِلْقَتي) is a mobile-first platform built specifically for Quran memorization circles, replacing fragmented tools (WhatsApp/Telegram + Zoom/Meet + manual tracking) with one unified experience.

**Vision:** Become the default platform for Quran circles globally, from individual teachers to institutions.

---

## 2) Business Problem

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

---

## 3) Product Goals (12 months)

| Goal | Metric | Target |
|---|---|---|
| Prove product-market fit for individual teachers | Monthly Active Teachers | 300+ |
| Drive recurring student usage | Student WAU/MAU | > 60% |
| Validate core differentiator (queue) | Sessions using queue | > 80% |
| Build retention moat | 90-day teacher retention | > 55% |
| Prepare B2B expansion | Pilot institutions | 5+ |

---

## 4) Target Users and Jobs-to-be-Done

### Primary Segments

1. **Teacher / Reciter (مُقرئ/محفظ)**
   - JTBD: run circles efficiently, evaluate students, track outcomes
2. **Student (طالب)**
   - JTBD: attend sessions, recite in order, track progress
3. **Institution Admin (future)**
   - JTBD: operate many circles with centralized reporting

### Core User Insights

- Students may belong to **multiple circles** with different teachers.
- Teachers need audio quality suitable for Quran recitation, not generic voice chat.
- Solo teachers are the initial priority, while preserving a future path for institutions.
- Pilot circles will start small (5–7 students) with size-limit policy left open for now.

### Future: Institutional Platform (منصة المؤسسات)

> **See [ROLES.md](./ROLES.md)** for detailed role capabilities, authorization rules, and the full permission matrix.

One of the most significant long-term growth vectors is the **Institutional Platform**.Quran memorization institutions — schools, centers, mosques, and national organizations — manage dozens or hundreds of circles simultaneously.

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

## 5) Value Proposition

### Why users choose Halaqaty

- One app for circle operations end-to-end
- No arbitrary session limits
- Quran-native recitation workflow
- Actionable progress visibility
- Institution-ready growth path

### Differentiator

**Recitation Queue System** is the core wedge: structured turns, live status, repeatable rounds, grading history.

---

## 6) Scope

### In Scope (MVP / P0)

- Authentication and role-based access
- Circle creation and membership
- Recitation queue with rounds and statuses
- Real-time chat + voice notes
- Live sessions (audio-only via LiveKit for MVP)
- Scheduling + reminders + attendance basics
- Basic session-level progress visibility (session history + grades)

### Out of Scope (MVP)

- AI tajweed scoring
- AI memorization planning
- Full institution control center
- Live session video (post-MVP, feature-flagged rollout)
- Session recording (post-MVP only; high privacy risk and explicit consent framework required)
- Desktop applications
- Advanced progress analytics (Quran map, trend charts, comparative dashboards)

### Feature Flag & Privacy Policies

- **Recording Policy**: Recording remains disabled in MVP for privacy reasons. Any future recording rollout requires explicit participant consent UX, retention policy, and access-control rules.
- **Rollout Control**: `live_session_video` and `session_recording` feature flags must remain OFF in MVP and require PM + architect sign-off before activation.

---

## 7) Business Requirements

### BR-1 Growth & Adoption

- onboarding must allow first circle creation in <10 minutes
- invite flow must support link/code sharing in common messaging apps

### BR-2 Retention

- teachers must get weekly progress visibility for each student
- students must receive clear turn reminders and session reminders
- grading policy must be configurable per circle (required vs optional per completed turn)

### BR-3 Monetization Readiness

- architecture and packaging must support a free tier + paid tier transition
- premium flags must be definable per feature (recording, advanced analytics, future AI)

### BR-4 Trust & Brand Fit

- no ads model
- respectful UX for religious/educational context
- clear privacy boundaries for teacher/student data
- no server-side live-session recording in MVP

---

## 8) Success Metrics (North Star + Supporting)

### North Star

**Weekly completed recitation rounds**

### Supporting KPIs

- circles created per week
- average active students per circle
- session attendance rate
- queue completion rate per session
- messages and voice notes per active circle
- teacher churn rate
- free → paid conversion (post-monetization)

---

## 9) Pricing and Business Model (Future)

| Tier | Target | Value | Pricing direction |
|---|---|---|---|
| Free | Individual/small circles | core operations | $0 |
| Premium | Active teachers | video sessions (post-MVP, feature-flagged), recordings (deferred until privacy framework), advanced reports, future AI | subscription |
| Institutional | schools/centers | centralized management + branding + analytics | annual contract |

**Policy:** No ads in any tier.

**Paywall Activation Decision (TBD):**  
The free tier is the default through MVP and pilot. The paywall activates when the 300+ MAT target is reached **or** at the public App Store launch (M4), whichever comes later. This trigger must be explicitly confirmed by Karim before M3.

The feature-flag architecture (ADR-005) allows per-user and per-feature activation without deployment.

---

## 10) Go-To-Market (GTM)

### Phase A — Pilot

- recruit 10–20 Quran teachers in Egypt first
- run closed cohort with weekly feedback cycles

### Phase B — Early Public

- Android-first launch and referral-driven invites
- teacher ambassador program

### Phase C — Trust Expansion

- student + teacher dashboards and selective institution pilots
- partnerships with Quran centers and online academies

---

## 11) Risks and Mitigation

| Risk | Impact | Mitigation |
|---|---|---|
| Low teacher activation | High | improve first-session setup, templates, concierge onboarding |
| Audio quality complaints | High | audio-only defaults, device guidance, session diagnostics |
| Audio-only perceived as "less than Zoom/Meet" | Medium | message queue-centric value in onboarding and GTM, and position video as intentional post-MVP upgrade |
| Premature enablement of video/recording | High | keep both behind feature flags off by default; require privacy framework sign-off before rollout |
| Multi-circle scheduling conflicts | Medium | conflict warnings, unified student calendar |
| Slow institutional sales cycle | Medium | prove teacher traction first, then package B2B pilots |
| App Store / Play Store rejection | High | Pre-submission checklist for audio/video apps; minimum 2-week TestFlight beta before App Store submission; follow Apple HIG for religious/educational apps; avoid keywords that trigger automatic review holds |

---

## 12) Milestones

| Milestone | Outcome | Acceptance Gates |
|---|---|---|
| M1 | MVP scope sign-off + pilot readiness | F-001 through F-006 all `✅ Shipped`; internal alpha APK tested by ≥1 pilot teacher with ≥5 students; zero P0 bugs open; Hetzner server live with all services running |
| M2 | Pilot launch with queue-centric workflows | Google Play Open Beta deployed; 5–10 active pilot teachers onboarded; queue used in ≥3 real sessions; session audio quality rated ≥4/5 by pilot teachers; error rate <1% on auth + queue + session paths |
| M3 | Public beta with retention instrumentation | Apple TestFlight deployed; 50+ MAT; student WAU/MAU ≥40%; analytics instrumented (circles, session count, queue completion rate); P0 bug SLA <48 h |
| M4 | Monetization experiment + institution pilot | 150+ MAT; ≥1 paid tier activated (feature-flagged); 2–3 institution pilots running; teacher churn <15%/month; infrastructure scaled to Phase 2 (DEPLOYMENT.md) |

---

## 13) Product Decisions (Resolved)

All six open product decisions have been resolved. Full rationale is in [`docs/MVP_DECISION_REGISTER.md`](MVP_DECISION_REGISTER.md).

| # | Decision | Resolution |
|---|---|---|
| 1 | Free tier caps | **3 circles, 30 students/circle.** Pro removes all caps. |
| 2 | Premium packaging sequence | **Recording before analytics** (post-MVP, once privacy framework approved). |
| 3 | Institution onboarding model | **Self-serve in MVP.** Admin receives invite code, manages own school. |
| 4 | Co-teacher model | **Deferred post-pilot.** MVP: teacher + supervisor only. |
| 5 | Video feature-flag rollout | **Per-tier once enabled.** Free: no video. Pro/Institution: video requires both global master flag AND tier flag = true. |
| 6 | Recording consent and retention | **Explicit consent screen before every session.** Consent stored per-session. Default retention: 7 days. |

---

## 14) PM Notes

This PRD is intentionally business-first. Detailed technical implementation remains in:

- [ARCHITECTURE.md](ARCHITECTURE.md)
- [DEPLOYMENT.md](DEPLOYMENT.md)
