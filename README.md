# حِلْقَتي — Halaqaty

> **One platform for every Quran memorization circle.**

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Flutter](https://img.shields.io/badge/Flutter-3.x-blue.svg)](https://flutter.dev)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://golang.org)
[![LiveKit](https://img.shields.io/badge/Audio-LiveKit-orange.svg)](https://livekit.io)

---

## 📖 Vision

Halaqaty (حِلْقَتي — "My Circle") is a **mobile-first platform built exclusively for Quran memorization circles (حلقات تحفيظ القرآن)**. Today, teachers and students are forced to juggle Telegram or WhatsApp for chat, Zoom or Google Meet for video sessions (often with frustrating time limits), and spreadsheets or notebooks for progress tracking.

Halaqaty replaces all of that with a single, purpose-built application designed around the spiritual and pedagogical needs of Quran memorization.

---

## 🌟 Key Features

| Feature | Description |
|---------|-------------|
| 🕌 **Circle Management** | Create circles, invite students with codes/links, assign supervisors |
| 🎙️ **Recitation Queue System** | The killer feature — ordered queue for live sessions with per-round tracking |
| 💬 **Real-time Chat** | Group and private messages with voice notes, files, and pinning |
| 🎙️ **Live Sessions** | No-time-limit audio-only sessions via LiveKit (WebRTC), optimized for Quran recitation quality (video post-MVP behind feature flag) |
| 📅 **Schedule & Calendar** | Recurring weekly schedules, reminders, attendance tracking |
| 📊 **Progress Tracking** | Session-level memorization logs with grades in MVP; advanced Quran maps/analytics post-MVP |
| 🔔 **Smart Notifications** | FCM push + in-app real-time notifications |
| 🔒 **Privacy-First Sessions** | Live-session recording is disabled in MVP; any future recording requires explicit consent and strict privacy controls |
| 📊 **Student & Teacher Dashboards** | Post-MVP dashboards for students and teachers across members/circles |
| 🏢 **Institutional Platform** *(future)* | Onboard entire Quran schools and organizations |

---

## 🔥 The Recitation Queue System

The most unique feature of Halaqaty. During a live session, the teacher manages an **ordered queue** of students who recite one by one. Each student's status is visible to all:

- ⏳ **Waiting** — hasn't recited yet
- 🎙️ **Currently Reciting** — highlighted for everyone in the session
- ✅ **Completed** — finished, grade recorded
- ⏭️ **Skipped** — absent or skipped by teacher

The queue can be **reset and reused multiple times** in a single session — for example, Round 1 for new memorization (حفظ جديد) from Surah Al-Baqarah, then reset for Round 2 for revision (مراجعة) from Surah Al-Fatiha. Each round tracks the Surah, starting Ayah, and ending Ayah.

---

## 🛠️ Tech Stack

| Layer | Technology | Why |
|-------|-----------|-----|
| **Mobile** | Flutter (Dart) | Single codebase → iOS, Android, Web |
| **Backend** | Go (Golang) | High performance, excellent concurrency for WebSocket/chat |
| **Database** | PostgreSQL | Robust relational data, JSONB for flexible fields |
| **Real-time Chat** | WebSocket (native Go) | Low latency, full control |
| **Audio Sessions** | LiveKit (WebRTC, self-hosted) | Open-source, no time limits, Quran audio optimization (video feature-flagged post-MVP) |
| **File Storage** | MinIO (S3-compatible, self-hosted) | Cost-effective, self-hosted, no vendor lock-in |
| **Push Notifications** | Firebase Cloud Messaging (FCM) | Industry standard, cross-platform |
| **Auth** | Firebase Auth | Social sign-in (Google, Apple), phone OTP |
| **Deployment** | Docker Compose → Kubernetes | Start cheap (~$8/mo), scale to global |

---

## 📁 Project Structure

```
halaqaty/
├── README.md                      ← You are here
├── DEVELOPMENT.md                 ← Developer guide (Spec-Kit, workflow, commands)
├── LICENSE                        ← MIT License
├── .specify/                      ← Spec-Kit configuration
│   └── memory/
│       └── constitution.md        ← Governing document — read before writing any code
├── .github/
│   ├── prompts/                   ← Spec-Kit slash command definitions
│   └── agents/                    ← Copilot agent configurations
├── docs/
│   ├── README.md                  ← Docs overview and navigation
│   ├── management/                ← Business & product strategy
│   │   ├── README.md
│   │   ├── product/               ← PRD, features, user journeys, MVP decisions
│   │   │   ├── README.md
│   │   │   ├── PRD.md
│   │   │   ├── FEATURES.md
│   │   │   ├── JOURNEY.md
│   │   │   └── MVP_DECISION_REGISTER.md
│   │   ├── planning/              ← Master project plan
│   │   │   ├── README.md
│   │   │   └── PLAN.md
│   │   ├── business/              ← Market analysis, competitor research
│   │   │   ├── README.md
│   │   │   └── QURAN_MEMORIZATION_COMPETITOR_ANALYSIS.md
│   │   └── arabic/                ← Arabic business documentation
│   │       ├── README.md
│   │       ├── SYNC_GUIDE.md
│   │       ├── PRD_AR.md
│   │       ├── PLAN_AR.md
│   │       └── FEATURES_AR.md
│   └── engineering/               ← Technical architecture & deployment
│       ├── README.md
│       ├── architecture/          ← System design & ADRs
│       │   ├── README.md
│       │   ├── ARCHITECTURE.md
│       │   └── adr/
│       │       ├── README.md
│       │       ├── ADR-001-modular-monolith.md
│       │       ├── ADR-002-go-framework.md
│       │       ├── ADR-003-flutter-state-management.md
│       │       ├── ADR-004-auth-boundary.md
│       │       ├── ADR-005-feature-flags.md
│       │       └── ADR-006-db-migrations.md
│       ├── deployment/            ← Deployment strategy & infrastructure
│       │   ├── README.md
│       │   └── DEPLOYMENT.md
│       ├── development/           ← Development setup & execution playbook
│       │   ├── README.md
│       │   └── EXECUTION_PLAYBOOK.md
│       ├── collaboration/         ← Agent collaboration & team coordination
│       │   ├── README.md
│       │   ├── AGENT_COLLABORATION_GUIDE.md
│       │   ├── AGENT_SETUP_REFINEMENT_SUMMARY.md
│       │   └── DOCUMENTATION_UPDATE_SUMMARY.md
│       ├── system-design/         ← (Future: Detailed system design)
│       │   └── README.md
│       ├── api-docs/              ← (Future: API reference)
│       │   └── README.md
│       └── guides/                ← (Future: How-to guides)
│           └── README.md
└── specs/                         ← Spec-Kit per-feature specs (generated, do not edit manually)
    └── NNN-feature-name/
        ├── spec.md
        ├── plan.md
        ├── data-model.md
        ├── contracts/
        ├── tasks.md
        └── quickstart.md
```

---

## 🛠️ Development Workflow

Halaqaty uses **[Spec-Kit](https://github.com/github/spec-kit)** (`v0.8.1`) for spec-driven development with GitHub Copilot. All code is generated from frozen specs — not from ad-hoc prompts.

**Five specialized engineering agents collaborate autonomously:**
- **Senior Golang Developer** — Backend services, APIs, concurrency, database
- **Senior Flutter Mobile Engineer** — Mobile UI, state management, RTL/Arabic support
- **Architect** — System design, service boundaries, technology choices
- **Tech Lead** — Code quality, security, performance, testing standards (hard gate)
- **Team Leader** — Coordination, delivery tracking, Spec-Kit enforcement

**Read before writing any code:**

| Document | Purpose |
|---|---|
| [DEVELOPMENT.md](DEVELOPMENT.md) | Full developer guide: setup, Spec-Kit commands, workflow steps, quality gates, agent roles |
| [`.specify/memory/constitution.md`](.specify/memory/constitution.md) | Governing principles — every Copilot agent reads this first |
| [`docs/AGENT_COLLABORATION_GUIDE.md`](docs/AGENT_COLLABORATION_GUIDE.md) | How agents collaborate: roles, clarification protocols, escalation paths |
| [`docs/MVP_DECISION_REGISTER.md`](docs/MVP_DECISION_REGISTER.md) | All frozen business and technical decisions |

**Spec-Kit workflow (7 phases):**
```
1. /speckit.specify      → Create feature spec
2. /speckit.clarify      → Resolve ambiguities (agents ask you clarifying questions)
3. /speckit.checklist    → Validate spec quality
4. /speckit.plan         → Design architecture
5. /speckit.tasks        → Generate implementation tasks
6. /speckit.analyze      → Check consistency
7. /speckit.implement    → Agents execute with tests and reviews
```

See [DEVELOPMENT.md](DEVELOPMENT.md) for the full command table, step-by-step workflow, and PR requirements.

---


This repository is currently in the **planning phase**. All documents below are comprehensive planning artifacts authored from the perspective of the full leadership team.

| Document | Language | Description |
|----------|----------|-------------|
| [PRD.md](docs/PRD.md) | 🇬🇧 English | Product requirements document (business-first) |
| [arabic/PRD_AR.md](docs/arabic/PRD_AR.md) | 🇸🇦 Arabic | وثيقة متطلبات المنتج بالعربية |
| [PLAN.md](docs/PLAN.md) | 🇬🇧 English | Master project plan — vision, features, architecture, roadmap |
| [arabic/PLAN_AR.md](docs/arabic/PLAN_AR.md) | 🇸🇦 Arabic | الخطة التجارية للمشروع |
| [FEATURES.md](docs/FEATURES.md) | 🇬🇧 English | Detailed feature specifications and status tracking |
| [arabic/FEATURES_AR.md](docs/arabic/FEATURES_AR.md) | 🇸🇦 Arabic | مواصفات المميزات التفصيلية |
| [arabic/README_AR.md](docs/arabic/README_AR.md) | 🇸🇦 Arabic | نظرة عامة عربية للمشروع |
| [ARCHITECTURE.md](docs/ARCHITECTURE.md) | 🇬🇧 English | Technical architecture, DB schema, API endpoints |
| [DEPLOYMENT.md](docs/DEPLOYMENT.md) | 🇬🇧 English | Phase-by-phase deployment plan with costs |
| [EXECUTION_PLAYBOOK.md](docs/EXECUTION_PLAYBOOK.md) | 🇬🇧 English | CEO/PM execution system: MVP cut, decision sprint, GTM, KPIs, RACI |
| [SYNC_GUIDE.md](docs/SYNC_GUIDE.md) | 🇬🇧🇸🇦 Bilingual | Guide for keeping Arabic/English docs in sync |

---

## 🚀 Release Roadmap

| Phase | Timeline | Milestone |
|-------|----------|-----------|
| **Phase 1** | Months 1–3 | Android APK (internal testing) |
| **Phase 2** | Months 4–6 | Google Play + Apple TestFlight |
| **Phase 3** | Months 6–8 | Apple App Store public launch |
| **Phase 4** | Months 8–12 | Flutter Web + institutional features |

---

## 🤝 Contributing

Before writing any code, read [DEVELOPMENT.md](DEVELOPMENT.md) for the full Spec-Kit workflow and agent collaboration model.

1. Read `.specify/memory/constitution.md` — the governing document for all decisions.
2. Read `docs/AGENT_COLLABORATION_GUIDE.md` — how agents collaborate and when they ask you clarifying questions.
3. Verify the feature is `🟡 Approved` in `docs/FEATURES.md`.
4. Run `/speckit.specify` in VS Code Copilot Chat to start the 7-phase workflow.
5. Follow the pipeline: **specify → clarify → checklist → plan → tasks → analyze → implement**.
6. Agents will ask you 5-7 clarifying questions if requirements are ambiguous — **answer clearly**.
7. All PRs require green quality gates and **Tech Lead approval** (see [DEVELOPMENT.md](DEVELOPMENT.md)).
8. Arabic documentation mirrors business/product docs only. Technical docs (ARCHITECTURE, ADRs) remain English. See [docs/SYNC_GUIDE.md](docs/SYNC_GUIDE.md).

---

## 📄 License

This project is licensed under the **MIT License** — see [LICENSE](LICENSE) for details.

---

<div align="center">

**بِسْمِ اللَّهِ الرَّحْمَنِ الرَّحِيم**

*Built with ❤️ for the Quran memorization community worldwide*

</div>
