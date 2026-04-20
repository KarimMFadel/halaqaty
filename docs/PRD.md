# Halaqaty — Product Requirements Document (PRD)

> Version: 1.0  
> Status: Draft for alignment  
> Owner: Product Management (**GPT-5.3-Codex acting as Project Manager**)  
> Related: [PLAN.md](PLAN.md) · [FEATURES.md](FEATURES.md) · [ARCHITECTURE.md](ARCHITECTURE.md) · [DEPLOYMENT.md](DEPLOYMENT.md)

---

## 1) Product Vision

Halaqaty (حِلْقَتي) is a mobile-first platform built specifically for Quran memorization circles, replacing fragmented tools (WhatsApp/Telegram + Zoom/Meet + manual tracking) with one unified experience.

**Vision:** Become the default platform for Quran circles globally, from individual teachers to institutions.

---

## 2) Business Problem

Today’s Quran circles suffer from:
- fragmented communication and meeting tools
- time-limited free video platforms
- no structured recitation workflow
- poor progress visibility for teachers and students

This creates low retention, operational friction, and limited scalability for teachers and organizations.

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
- Live sessions (audio-first, optional video)
- Scheduling + reminders + attendance basics

### Out of Scope (MVP)
- AI tajweed scoring
- AI memorization planning
- Full institution control center
- Session recording
- Desktop applications

---

## 7) Business Requirements

### BR-1 Growth & Adoption
- onboarding must allow first circle creation in <10 minutes
- invite flow must support link/code sharing in common messaging apps

### BR-2 Retention
- teachers must get weekly progress visibility for each student
- students must receive clear turn reminders and session reminders

### BR-3 Monetization Readiness
- architecture and packaging must support a free tier + paid tier transition
- premium flags must be definable per feature (recording, advanced analytics, future AI)

### BR-4 Trust & Brand Fit
- no ads model
- respectful UX for religious/educational context
- clear privacy boundaries for teacher/student data

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
| Premium | Active teachers | recordings, advanced reports, future AI | subscription |
| Institutional | schools/centers | centralized management + branding + analytics | annual contract |

**Policy:** No ads in any tier.

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
| Audio quality complaints | High | audio-first defaults, device guidance, session diagnostics |
| Multi-circle scheduling conflicts | Medium | conflict warnings, unified student calendar |
| Slow institutional sales cycle | Medium | prove teacher traction first, then package B2B pilots |

---

## 12) Milestones

| Milestone | Outcome |
|---|---|
| M1 | MVP scope sign-off + pilot readiness |
| M2 | Pilot launch with queue-centric workflows |
| M3 | Public beta with retention instrumentation |
| M4 | Monetization experiment + institution pilot package |

---

## 13) Open Product Decisions

1. free tier caps (students/circles) before paywall
2. premium packaging sequence once recording returns post-MVP (recording vs analytics first)
3. institution onboarding model (self-serve vs assisted)
4. co-teacher model details (distinct role vs supervisor permissions)

---

## 14) PM Notes

This PRD is intentionally business-first. Detailed technical implementation remains in:
- [ARCHITECTURE.md](ARCHITECTURE.md)
- [DEPLOYMENT.md](DEPLOYMENT.md)
