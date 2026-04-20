# حِلْقَتي — Halaqaty

> **One platform for every Quran memorization circle.**

[![License: MIT](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)
[![Flutter](https://img.shields.io/badge/Flutter-3.x-blue.svg)](https://flutter.dev)
[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8.svg)](https://golang.org)
[![LiveKit](https://img.shields.io/badge/Video-LiveKit-orange.svg)](https://livekit.io)

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
| 📹 **Live Sessions** | No-time-limit audio/video via LiveKit (WebRTC), optimized for Quran audio |
| 📅 **Schedule & Calendar** | Recurring weekly schedules, reminders, attendance tracking |
| 📊 **Progress Tracking** | Per-student memorization logs with grades and visual Quran maps |
| 🔔 **Smart Notifications** | FCM push + in-app real-time notifications |
| 📊 **Student & Teacher Dashboards** | Students track own progress; teachers track all members and each circle |
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
| **Video/Audio** | LiveKit (WebRTC, self-hosted) | Open-source, no time limits, Quran audio optimization |
| **File Storage** | MinIO (S3-compatible, self-hosted) | Cost-effective, self-hosted, no vendor lock-in |
| **Push Notifications** | Firebase Cloud Messaging (FCM) | Industry standard, cross-platform |
| **Auth** | Firebase Auth | Social sign-in (Google, Apple), phone OTP |
| **Deployment** | Docker Compose → Kubernetes | Start cheap (~$8/mo), scale to global |

---

## 📁 Project Structure

```
halaqaty/
├── README.md                  ← You are here
├── LICENSE                    ← MIT License
├── .gitignore
└── docs/
    ├── PRD.md                  ← Product Requirements Document (business)
    ├── PLAN.md                 ← Master project plan (English)
    ├── FEATURES.md             ← Feature spec & status board (English)
    ├── ARCHITECTURE.md         ← Technical architecture (English)
    ├── DEPLOYMENT.md           ← Deployment strategy (English)
    ├── SYNC_GUIDE.md           ← Bilingual document sync guide
    └── arabic/
        ├── README_AR.md        ← Arabic overview
        ├── PLAN_AR.md          ← Arabic business plan
        └── FEATURES_AR.md      ← Arabic feature board
```

---

## 📚 Planning Documents

This repository is currently in the **planning phase**. All documents below are comprehensive planning artifacts authored from the perspective of the full leadership team.

| Document | Language | Description |
|----------|----------|-------------|
| [PRD.md](docs/PRD.md) | 🇬🇧 English | Product requirements document (business-first) |
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

Contributions are welcome once the codebase is established. For now:

1. Read the planning documents in `docs/`
2. Open an issue to discuss ideas or improvements
3. Follow the bilingual documentation standard (see [SYNC_GUIDE.md](docs/SYNC_GUIDE.md))

*Detailed contribution guidelines will be added when development begins.*

---

## 📄 License

This project is licensed under the **MIT License** — see [LICENSE](LICENSE) for details.

---

<div align="center">

**بِسْمِ اللَّهِ الرَّحْمَنِ الرَّحِيم**

*Built with ❤️ for the Quran memorization community worldwide*

</div>
