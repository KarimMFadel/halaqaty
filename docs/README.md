# 📚 Halaqaty Documentation Hub

Welcome to the Halaqaty project documentation. This folder is organized into two main categories:

---

## 🎯 **Management** — Business & Product Strategy

For product managers, stakeholders, and business decision-makers.

### 📖 Quick Links

- **[Product Documentation](./management/product/)** — PRD, features, user journeys, MVP decisions
  - [`PRD.md`](./management/product/PRD.md) — Product Requirements Document
  - [`FEATURES.md`](./management/product/FEATURES.md) — Feature status board
  - [`JOURNEY.md`](./management/product/JOURNEY.md) — Complete user journey (teacher-first)
  - [`MVP_DECISION_REGISTER.md`](./management/product/MVP_DECISION_REGISTER.md) — All frozen MVP decisions

- **[Planning](./management/planning/)** — Project plan & timeline
  - [`PROJECT_PLAN.md`](./management/planning/PROJECT_PLAN.md) — 12-month master project plan

- **[Business](./management/business/)** — Market analysis & research
  - [`QURAN_MEMORIZATION_COMPETITOR_ANALYSIS.md`](./management/business/QURAN_MEMORIZATION_COMPETITOR_ANALYSIS.md) — Competitive landscape

- **[Arabic Documentation](./management/arabic/)** — العربية
  - [`PRD_AR.md`](./management/arabic/PRD_AR.md) — وثيقة متطلبات المنتج
  - [`PLAN_AR.md`](./management/arabic/PLAN_AR.md) — خطة المشروع
  - [`FEATURES_AR.md`](./management/arabic/FEATURES_AR.md) — مواصفات الميزات
  - [`SYNC_GUIDE.md`](./management/arabic/SYNC_GUIDE.md) — Bilingual documentation sync guide

---

## 🔧 **Engineering** — Technical Architecture & Development

For engineers, architects, and technical leads.

### 📖 Quick Links

- **[Architecture](./engineering/architecture/)** — System design & decisions
  - [`ARCHITECTURE.md`](./engineering/architecture/ARCHITECTURE.md) — Technical architecture & schema
  - [`adr/`](./engineering/architecture/adr/) — Architecture Decision Records (live index in [architecture/README.md](./engineering/architecture/README.md))

- **[Deployment](./engineering/deployment/)** — Infrastructure & scaling
  - [`DEPLOYMENT.md`](./engineering/deployment/DEPLOYMENT.md) — Deployment strategy (MVP to global)

- **[Development](./engineering/development/)** — Setup & workflows
  - [`EXECUTION_PLAYBOOK.md`](./engineering/development/EXECUTION_PLAYBOOK.md) — Step-by-step execution workflow
  - See [`DEVELOPMENT.md`](../DEVELOPMENT.md) at root for complete developer guide

- **[Collaboration](./engineering/collaboration/)** — Agent coordination
  - [`AGENT_COLLABORATION_GUIDE.md`](./engineering/collaboration/AGENT_COLLABORATION_GUIDE.md) — How 5 engineering agents work together

- **[System Design](./engineering/system-design/)** *(future)* — Detailed system design docs
- **[API Documentation](./engineering/api-docs/)** *(future)* — API reference & endpoints
- **[Guides](./engineering/guides/)** *(future)* — How-to guides & walkthroughs

- **[Plan Review & Quality Audits](./plan_review/)** — Historical reviews and enhancement tracking
  - [`ENHANCEMENT_TRACKER.md`](./plan_review/ENHANCEMENT_TRACKER.md) — Living list of all detected enhancements with priority, status, and amendment history
  - [`project_plan_review.md`](./plan_review/project_plan_review.md) — Original A- plan review (23 findings, 18 resolved)
  - [`docs_content_audit.md`](./plan_review/docs_content_audit.md) — Docs content audit (18 findings, D-01 through D-18)

---

## 🚀 Start Here

**First time here?**

1. Read [`.specify/memory/constitution.md`](../.specify/memory/constitution.md) — The governing principles
2. For **Product** → Start with [`PRD.md`](./management/product/PRD.md)
3. For **Engineering** → Start with [`ARCHITECTURE.md`](./engineering/architecture/ARCHITECTURE.md)
4. For **Development** → Start with [`DEVELOPMENT.md`](../DEVELOPMENT.md)

---

## 📋 Navigation Map

```
docs/
├── management/          ← Business & product strategy
│   ├── product/         ← PRD, features, journeys
│   ├── planning/        ← Master plan
│   ├── business/        ← Market analysis
│   └── arabic/          ← Arabic translations
│
├── engineering/         ← Technical docs
│   ├── architecture/    ← System design & ADRs
│   ├── deployment/      ← Infrastructure
│   ├── development/     ← Execution workflow & testing strategy
│   ├── collaboration/   ← Agent coordination
│   ├── system-design/   ← (reserved for future)
│   ├── api-docs/        ← (reserved for future)
│   └── guides/          ← (reserved for future)
│
└── plan_review/         ← Quality audits & enhancement tracking
    ├── ENHANCEMENT_TRACKER.md   ← ⭐ Living enhancement tracker
    ├── project_plan_review.md   ← Original A- plan review
    └── docs_content_audit.md   ← Docs content audit
```

## Abbreviation Glossary

A quick reference for the abbreviations used in this repository:

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

## 🔗 Related Files at Root

- [`README.md`](../README.md) — Project overview
- [`DEVELOPMENT.md`](../DEVELOPMENT.md) — Complete developer guide (Spec-Kit, workflow, commands)
- [`.specify/memory/constitution.md`](../.specify/memory/constitution.md) — Governing principles

---

**Last updated:** 2026-06-20  
**Maintained by:** Halaqaty Team
