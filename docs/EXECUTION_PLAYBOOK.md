# Halaqaty — Execution Playbook

> Version: 1.0  
> Status: Active operating playbook  
> Related: [PRD.md](PRD.md) · [PLAN.md](PLAN.md) · [FEATURES.md](FEATURES.md) · [DEVELOPMENT.md](../DEVELOPMENT.md) · [docs/AGENT_COLLABORATION_GUIDE.md](AGENT_COLLABORATION_GUIDE.md)

---

## 0) Agent-Driven Development

Halaqaty features are built by **5 specialized engineering agents** working collaboratively:

- **Senior Golang Developer** → Backend services, APIs, concurrency, database
- **Senior Flutter Mobile Engineer** → Mobile UI, state management, RTL/Arabic support
- **Architect** → System design, service boundaries, technology choices
- **Tech Lead** → Code quality, security, performance, testing (hard gate before merge)
- **Team Leader** → Coordination, delivery tracking, Spec-Kit workflow enforcement

**Key principle**: When ambiguous, agents ask 5-7 clarifying questions rather than guessing. See [`docs/AGENT_COLLABORATION_GUIDE.md`](AGENT_COLLABORATION_GUIDE.md) for detailed collaboration patterns.

---

This playbook defines **how we execute** week by week:
- what ships now (MVP cut),
- how decisions are made quickly (decision sprint),
- how we go to market (GTM),
- how performance is measured (KPIs),
- who owns what (RACI).

---

## 2) MVP Cut (What we ship first)

### MVP Definition
**MVP (Minimum Viable Product)** = the smallest version that delivers real value.

For Halaqaty MVP:
1. Create circle
2. Run live session
3. Use queue + grading
4. View basic session-level progress

### In MVP
- Authentication + role-based access
- Circle creation and membership
- Recitation queue with rounds/statuses
- Real-time chat + voice notes
- Live sessions (audio-only)
- Schedule, reminders, attendance basics
- Basic session-level progress visibility (history + grades)

### Out of MVP (deferred)
- Session video (post-MVP, feature-flagged rollout)
- Session recording (post-MVP, privacy-sensitive/high-risk feature)
- AI tajweed scoring
- AI memorization planning
- Institutional control center
- Desktop app
- Advanced progress analytics (Quran map, trend charts, comparative dashboards)

### Privacy and trust policy for deferred recording
- Recording remains disabled in MVP for privacy reasons.
- Any future recording rollout requires explicit participant consent UX, retention policy, and access-control rules.

---

## 3) Decision Sprint (Fast product decisions)

Every week, unresolved items are handled in one short decision sprint:

1. **Collect** open items (from PRD/FEATURES open questions)
2. **Evaluate** user impact, effort, and launch risk
3. **Decide** now / later / reject
4. **Record** final decision in source docs
5. **Assign** owner + due date

Decision rule: if a decision blocks MVP delivery, resolve it in the current sprint.
Policy rule: `live_session_video` and `session_recording` feature flags must remain OFF in MVP and require PM + architect sign-off before activation.

---

## 4) Weekly Operating Cadence

### Monday — Plan
- Confirm weekly objectives
- Freeze this week’s MVP scope

### Midweek — Execute
- Deliver priority items
- Surface blockers early

### Friday — Review
- KPI review
- Decision review
- Scope corrections for next week

---

## 5) GTM (Go-To-Market) Execution

### Phase A: Pilot
- 10–20 teachers in a controlled cohort
- Weekly feedback loop

### Phase B: Early Public
- Android-first expansion
- Referral and teacher ambassador motion

### Phase C: Trust Expansion
- Strengthen dashboards and retention
- Start selective institution pilots

---

## 6) KPI System

### KPI Definition
**KPI (Key Performance Indicator)** = measurable numbers used to track progress.

### North Star
- Weekly completed recitation rounds

### Core KPI Set
- Teacher activation rate
- Student WAU/MAU ratio
- Session attendance rate
- Queue completion rate
- 90-day teacher retention

KPI rule: each KPI must have an owner, target, and review frequency.

---

## 7) Team Accountability (RACI)

### RACI Definition
- **R** = Responsible (does the work)
- **A** = Accountable (final owner)
- **C** = Consulted (gives input)
- **I** = Informed (kept updated)

| Workstream | R | A | C | I |
|---|---|---|---|---|
| MVP scope control | Product Manager | CEO/Founder | Tech Lead | Team |
| Queue + session flow | Tech Lead | Product Manager | Teacher advisors | Team |
| KPI dashboard and review | Product Analyst | Product Manager | Tech Lead | Team |
| GTM pilot operations | Growth Lead | CEO/Founder | Product Manager | Team |

---

## 8) Role Clarification: Co-Teacher (Pending)

A **co-teacher role** is an assistant role concept for session operations (queue, attendance, moderation) without full owner/admin power.

Current Halaqaty policy:
- MVP operations run with teacher + assignable supervisor permissions,
- final co-teacher model remains open and is deferred until after pilot outcomes,
- teacher remains the final authority.

---

## 9) Abbreviation Glossary

| Abbreviation | Meaning | Practical meaning in Halaqaty |
|---|---|---|
| MVP | Minimum Viable Product | Smallest useful launch scope |
| KPI | Key Performance Indicator | Metrics to evaluate execution |
| GTM | Go-To-Market | Pilot-to-public launch strategy |
| RACI | Responsible, Accountable, Consulted, Informed | Ownership model for decisions and delivery |
| PRD | Product Requirements Document | Business and product requirements |
| WAU | Weekly Active Users | Weekly recurring usage |
| MAU | Monthly Active Users | Monthly recurring usage |
| JTBD | Jobs To Be Done | User goals/tasks the product must solve |
| B2B | Business to Business | Institution-facing business model |
| P0 / P1 / P2 / P3 | Priority levels | Launch-now to future backlog |
| OQ | Open Question | Unresolved product decision |
| DD | Design Decision | Documented technical/product decision |
| BR | Business Requirement | Business-level requirement |
| API | Application Programming Interface | Backend endpoints and integrations |
| JWT | JSON Web Token | Session/auth token format |
| OTP | One-Time Password | Phone verification code |
| FCM | Firebase Cloud Messaging | Push notification service |
| WS | WebSocket | Real-time update channel |
| DB | Database | Persistent application data |
| ERD | Entity Relationship Diagram | Database relationship map |

---

## 10) Update Policy

When a decision changes:
1. update the source document (PRD/PLAN/FEATURES),
2. update this playbook if execution behavior changes,
3. keep English/Arabic documentation aligned.

