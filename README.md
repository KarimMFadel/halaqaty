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
| 📊 **Progress Tracking** | Session-level memorization logs with grades, followed by the approved Phase 3 Quran map and analytics work |
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
| **Auth** | Firebase Auth | Email/password, Google, and Apple sign-in; phone is supplementary verification only |
| **Deployment** | Docker Compose on Hetzner CX22 | Start cheap (~$8/mo), single-server MVP; no Kubernetes in MVP ([ADR-014](docs/engineering/architecture/adr/ADR-014-mvp-deployment.md)) |

---

## 📁 Project Structure

```
halaqaty/
├── README.md                       ← You are here
├── AGENTS.md                       ← Agent harness rules (read alongside DEVELOPMENT.md)
├── DEVELOPMENT.md                  ← Developer guide (Spec-Kit, workflow, commands)
├── CONTRIBUTING.md                 ← Contribution guidelines
├── SECURITY.md                     ← Security policy
├── LICENSE                         ← MIT License
├── Makefile                        ← Root Makefile (delegates to backend/ and mobile/)
├── opencode.json                   ← OpenCode configuration
├── .spectral.yaml                  ← Spectral OAS lint config (used by `make api-lint`)
├── .specify/                       ← Spec-Kit configuration and governing memory
│   └── memory/constitution.md       ← Governing document — read before writing any code
├── .github/                        ← Copilot agents, skills, prompts, workflows
├── .opencode/                      ← OpenCode agent and skill definitions
├── backend/                        ← Go 1.22 service (module halaqaty/backend) — see backend/ layout
├── mobile/                         ← Flutter app (package halaqaty_mobile) — see mobile/ layout
├── specs/                          ← Spec-Kit per-feature specs (generated, do not edit manually)
│                                   ← each subdirectory is a feature (e.g., 001-auth-roles-profile)
└── docs/                           ← Documentation hub — see docs/README.md for full navigation
    ├── README.md                   ← Docs overview, navigation map, glossary
    ├── contracts/                  ← REST & WS API contracts (source of truth for the API surface)
    │   ├── openapi.yaml            ← OpenAPI 3.0 spec for /api/v1/* (Constitution §III)
    │   └── ws_events.md            ← WebSocket event catalogue
    ├── engineering/                ← Technical architecture & development
    │   ├── architecture/           ← System design, ARCHITECTURE.md, and the ADR index (see architecture/README.md)
    │   ├── deployment/             ← Deployment strategy & infrastructure
    │   ├── development/            ← Execution playbook & testing strategy
    │   ├── collaboration/          ← Agent collaboration guide & workflow harness
    │   ├── design/                 ← Per-feature design docs (e.g., F-007 progress tracking)
    │   ├── system-design/          ← Runtime lifecycle flows (queue, session, WebSocket)
    │   ├── api-docs/               ← API auth flow and endpoint quick-reference index
    │   └── guides/                 ← Troubleshooting runbooks and common dev tasks
    ├── management/                 ← Business & product strategy
    │   ├── product/                ← PRD, FEATURES, JOURNEY, ROLES, MVP_DECISION_REGISTER
    │   ├── planning/               ← Master project plan
    │   ├── business/               ← Market analysis & competitor research
    │   └── arabic/                 ← Arabic business/product docs + bilingual sync guide
    └── plan_review/                ← Historical plan reviews & enhancement tracker
```

> **Live contents:** this tree shows the stable top-level structure only. Each subdirectory's `README.md` (where present) holds the authoritative file-level index for that area. Run `Get-ChildItem -Recurse <dir>` (PowerShell) or `ls -R <dir>` (bash) for the current filesystem snapshot — the hand-maintained list below the directory level is intentionally omitted to avoid drift.

---

## 🛠️ Development Workflow

Halaqaty uses **[Spec-Kit](https://github.com/github/spec-kit)** (`v0.8.1`) for spec-driven development with GitHub Copilot. All code is generated from frozen specs — not from ad-hoc prompts.

**Specialized engineering agents collaborate through the project workflow harness:**
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
| [`docs/engineering/collaboration/AGENT_COLLABORATION_GUIDE.md`](docs/engineering/collaboration/AGENT_COLLABORATION_GUIDE.md) | How agents collaborate: roles, clarification protocols, escalation paths |
| [`docs/management/product/MVP_DECISION_REGISTER.md`](docs/management/product/MVP_DECISION_REGISTER.md) | All frozen business and technical decisions |

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
| [PRD.md](docs/management/product/PRD.md) | 🇬🇧 English | Product requirements document (business-first) |
| [arabic/PRD_AR.md](docs/management/arabic/PRD_AR.md) | 🇸🇦 Arabic | وثيقة متطلبات المنتج بالعربية |
| [PROJECT_PLAN.md](docs/management/planning/PROJECT_PLAN.md) | 🇬🇧 English | Master project plan — vision, features, architecture, roadmap |
| [arabic/PLAN_AR.md](docs/management/arabic/PLAN_AR.md) | 🇸🇦 Arabic | الخطة التجارية للمشروع |
| [FEATURES.md](docs/management/product/FEATURES.md) | 🇬🇧 English | Detailed feature specifications and status tracking |
| [arabic/FEATURES_AR.md](docs/management/arabic/FEATURES_AR.md) | 🇸🇦 Arabic | مواصفات المميزات التفصيلية |
| [arabic/README_AR.md](docs/management/arabic/README_AR.md) | 🇸🇦 Arabic | نظرة عامة عربية للمشروع |
| [ARCHITECTURE.md](docs/engineering/architecture/ARCHITECTURE.md) | 🇬🇧 English | Technical architecture, DB schema, API endpoints |
| [DEPLOYMENT.md](docs/engineering/deployment/DEPLOYMENT.md) | 🇬🇧 English | Phase-by-phase deployment plan with costs |
| [EXECUTION_PLAYBOOK.md](docs/engineering/development/EXECUTION_PLAYBOOK.md) | 🇬🇧 English | CEO/PM execution system: MVP cut, decision sprint, GTM, KPIs, RACI |
| [SYNC_GUIDE.md](docs/management/arabic/SYNC_GUIDE.md) | 🇬🇧🇸🇦 Bilingual | Guide for keeping Arabic/English docs in sync |

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
2. Read `docs/engineering/collaboration/AGENT_COLLABORATION_GUIDE.md` — how agents collaborate and when they ask you clarifying questions.
3. Verify the feature is `🟡 Approved` in `docs/management/product/FEATURES.md`.
4. Run `/speckit.specify` in VS Code Copilot Chat to start the 7-phase workflow.
5. Follow the pipeline: **specify → clarify → checklist → plan → tasks → analyze → implement**.
6. Agents will ask you 5-7 clarifying questions if requirements are ambiguous — **answer clearly**.
7. All PRs require green quality gates and **Tech Lead approval** (see [DEVELOPMENT.md](DEVELOPMENT.md)).
8. Arabic documentation mirrors business/product docs only. Technical docs (ARCHITECTURE, ADRs) remain English. See [docs/management/arabic/SYNC_GUIDE.md](docs/management/arabic/SYNC_GUIDE.md).

---

## 📄 License

This project is licensed under the **MIT License** — see [LICENSE](LICENSE) for details.

---

<div align="center">

**بِسْمِ اللَّهِ الرَّحْمَنِ الرَّحِيم**

*Built with ❤️ for the Quran memorization community worldwide*

</div>

