# Halaqaty — Master Project Plan

> **Version:** 1.0 | **Status:** Planning Phase | **Last Updated:** 2026

**Related Documents:** [product/PRD.md](./product/PRD.md) · [arabic/PLAN_AR.md](./arabic/PLAN_AR.md) · [product/FEATURES.md](./product/FEATURES.md) · [../engineering/architecture/ARCHITECTURE.md](../engineering/architecture/ARCHITECTURE.md) · [../engineering/deployment/DEPLOYMENT.md](../engineering/deployment/DEPLOYMENT.md) · [../../DEVELOPMENT.md](../../DEVELOPMENT.md) · [../engineering/collaboration/AGENT_COLLABORATION_GUIDE.md](../engineering/collaboration/AGENT_COLLABORATION_GUIDE.md)

---

## Table of Contents

1. [Executive Summary & Target Users](#1-executive-summary--target-users)
3. [Detailed Feature Specifications](#3-detailed-feature-specifications)
4. [Technical Architecture Overview](#4-technical-architecture-overview)
5. [Deployment Strategy](#5-deployment-strategy)
6. [Release Channel Strategy](#6-release-channel-strategy)
8. [Timeline — 12-Month Plan](#8-timeline--12-month-plan)

---

## 1. Executive Summary & Target Users

> **Migrated:** Vision, Problem Statement, and Roles have been moved to [`product/PRD.md`](../product/PRD.md) and [`product/ROLES.md`](../product/ROLES.md).

---

## 3. Detailed Feature Specifications

> **Migrated:** All feature specifications and user stories have been moved to [`product/FEATURES.md`](../product/FEATURES.md).

---

## 4. Technical Architecture Overview

See [ARCHITECTURE.md](ARCHITECTURE.md) for the complete technical specification.

### 4.1 Communication Protocols Summary

| Protocol | Used For |
|----------|---------|
| HTTPS / REST | All standard CRUD: auth, circle management, user profiles, progress records |
| WebSocket (Go) | Real-time chat, presence/online status, queue updates, in-app notifications |
| WebRTC (via LiveKit) | Audio streaming in live sessions (video is post-MVP, feature-flagged) |
| FCM | Push notifications when app is in background or closed |

---

## 5. Deployment Strategy

> **Migrated:** The deployment strategy has been moved to [`../engineering/deployment/DEPLOYMENT.md`](../../engineering/deployment/DEPLOYMENT.md).

---

## 6. Release Channel Strategy

| Phase | Timeline | Channel | Target |
|-------|----------|---------|--------|
| **Internal Alpha** | Month 1–3 | Android APK (sideloaded) | Core team + 5–10 pilot teachers |
| **Beta** | Month 4–6 | Google Play (Open Beta) + Apple TestFlight | 50–200 early adopters |
| **iOS Public** | Month 6–8 | Apple App Store | Full iOS launch |
| **Web** | Month 8–12 | Flutter Web (Progressive Web App) | Desktop users, institutions |
| **Institutional** | Month 12+ | B2B outreach | Quran schools and centers |

**Note on iOS:** Apple Sign-In is **mandatory** when Google Sign-In is offered on iOS. This will be implemented from the start.

---

## 7. Business Model

> **Migrated:** Business model and pricing tiers have been moved to [`product/PRD.md §9`](../product/PRD.md#9-pricing-and-business-model-future).

---

## 8. Timeline — 12-Month Plan

### Month 1–2: Foundation
- [ ] Finalize all planning documents (this phase)
- [ ] Set up development environment: Go backend scaffold, Flutter project scaffold
- [ ] PostgreSQL schema design and migrations
- [ ] Firebase Auth integration (email, Google, Apple)
- [ ] Basic user registration/login (Flutter UI)
- [ ] Circle creation and invitation (backend API)
- [ ] Hetzner server provisioning + Docker Compose setup

### Month 3: Core Chat & Circles
- [ ] Circle member management (join, leave, roles)
- [ ] WebSocket server (Go) for real-time messaging
- [ ] Group chat — text messages, image attachments
- [ ] Voice note recording and playback
- [ ] Push notifications via FCM (basic)

### Month 4: Live Sessions (LiveKit)
- [ ] LiveKit server deployment on Hetzner
- [ ] Go backend: LiveKit room creation + JWT token generation
- [ ] Flutter: `livekit_client` integration
- [ ] Basic audio session (teacher-controlled mute, hand raise)
- [ ] Audio-only hardening (no video publish paths in MVP clients/tokens)

### Month 5: Recitation Queue System

> **Dependency note:** Queue work starts after Month 4 LiveKit basic session (audio connect, teacher mute control) is functional end-to-end. Months 4–5 may overlap by 2–3 weeks: LiveKit core stabilises in Month 4 while queue backend work begins in parallel in early Month 5.

- [ ] Queue backend: real-time queue state via WebSocket
- [ ] Queue ordering modes (join order, manual)
- [ ] Student status (waiting/reciting/completed/skipped)
- [ ] Queue reset and round management
- [ ] Grading system
- [ ] Turn notification ("You're next!", "Your turn!")

### Month 6: Scheduling & Attendance
- [ ] Weekly schedule per circle
- [ ] Push notification reminders
- [ ] Auto-attendance from LiveKit room join events
- [ ] Manual attendance override

### Month 7: Progress Tracking (Session-Level)
- [ ] Memorization log linked to queue history (session-level visibility)
- [ ] Teacher notes per student per session
- [ ] Session history view: past sessions with grades per student

> **Scope note:** Visual Quran map, progress charts, and PDF export are P1/P2 features moved to Month 9–10. See FEATURES.md F-007 and F-011 for priority classification.

### Month 8: Beta Launch Preparation
- [ ] Google Play Beta deployment
- [ ] Apple TestFlight deployment
- [ ] Onboarding flow polish
- [ ] Bug bash and performance optimization
- [ ] Admin dashboard (teacher web view)

### Month 9–10: Post-Beta Improvements
- [ ] User feedback integration
- [ ] Performance tuning (WebSocket scalability)
- [ ] Student + teacher dashboards (P2)
- [ ] Multi-language: English + Arabic complete
- [ ] Visual Quran map — color-coded memorized portions (F-007, P1)
- [ ] Progress charts: weekly/monthly trend views (F-007, P1)
- [ ] Basic PDF report export (F-011, P2)

### Month 11: App Store Launch
- [ ] Apple App Store submission and review
- [ ] Marketing materials (Arabic + English)
- [ ] Pilot program with 3–5 Quran schools

### Month 12: Foundation for Growth
- [ ] Kubernetes migration planning
- [ ] Institution platform architecture design
- [ ] AI Tajweed assessment research spike
- [ ] Flutter Web deployment (PWA)
- [ ] Analytics dashboard

### ⚠️ Timeline Realism — Solo Developer

This 12-month plan is aggressive for a solo build. Calibration notes:
- **15–20% buffer** should be expected for debugging, App Store review, pilot feedback, and integration surprises.
- **Month 10–12 items are stretch goals.** If earlier phases slip, these are the first to be deferred.
- **Hard milestones:** M1 (internal alpha) and M2 (pilot launch with 5–10 teachers) are non-negotiable targets. M3 and M4 are aspirational.
- **Realistic range:** 12 months if velocity is strong; 14–15 months is equally valid and preferred over shipping incomplete features.

If in doubt, extend the timeline — do not cut quality gates.

---

*This document is the source of truth for project planning. Business-facing updates should be reflected in [arabic/PLAN_AR.md](arabic/PLAN_AR.md).*

*See [SYNC_GUIDE.md](../arabic/SYNC_GUIDE.md) for the documentation synchronization policy.*

