# Halaqaty — Execution Playbook

> Version: 1.0  
> Status: Active operating playbook  
> Related: [../../management/product/PRD.md](../../management/product/PRD.md) · [../../management/planning/PROJECT_PLAN.md](../../management/planning/PROJECT_PLAN.md) · [../../management/product/FEATURES.md](../../management/product/FEATURES.md) · [../../../DEVELOPMENT.md](../../../DEVELOPMENT.md) · [AGENT_COLLABORATION_GUIDE.md](../collaboration/AGENT_COLLABORATION_GUIDE.md)

---

## 0) Agent-Driven Development

Halaqaty features are built by **5 specialized engineering agents** working collaboratively:

- **Senior Golang Developer** → Backend services, APIs, concurrency, database
- **Senior Flutter Mobile Engineer** → Mobile UI, state management, RTL/Arabic support
- **Architect** → System design, service boundaries, technology choices
- **Tech Lead** → Code quality, security, performance, testing (hard gate before merge)
- **Team Leader** → Coordination, delivery tracking, Spec-Kit workflow enforcement

**Key principle**: When ambiguous, agents ask 5-7 clarifying questions rather than guessing. See [AGENT_COLLABORATION_GUIDE.md](../collaboration/AGENT_COLLABORATION_GUIDE.md) for detailed collaboration patterns.

---

This playbook defines **how we execute** week by week:
- what ships now (MVP cut),
- how decisions are made quickly (decision sprint),
- how we go to market (GTM),
- how performance is measured (KPIs),
- who owns what (RACI).

---

## 2) MVP Cut

> **Migrated:** MVP scope definition and out-of-scope items have been moved to [`PRD.md Â§6`](../../management/product/PRD.md#6-scope).

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

> **Migrated:** GTM phases (Pilot → Early Public → Trust Expansion) have been moved to [`PRD.md §10`](../../management/product/PRD.md#10-go-to-market-gtm).

---

## 6) KPI System

> **Migrated:** North Star metric and supporting KPIs have been moved to [`PRD.md §8`](../../management/product/PRD.md#8-success-metrics-north-star--supporting).

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

## 9) Update Policy

When a decision changes:
1. update the source document (PRD/PLAN/FEATURES),
2. update this playbook if execution behavior changes,
3. keep English/Arabic documentation aligned.


