# Halaqaty — Master Project Plan

> **Version:** 1.0 | **Status:** Planning Phase | **Last Updated:** 2026

**Related Documents:** [product/PRD.md](../product/PRD.md) · [arabic/PLAN_AR.md](../arabic/PLAN_AR.md) · [product/FEATURES.md](../product/FEATURES.md) · [../engineering/architecture/ARCHITECTURE.md](../engineering/architecture/ARCHITECTURE.md) · [../engineering/deployment/DEPLOYMENT.md](../engineering/deployment/DEPLOYMENT.md) · [../../DEVELOPMENT.md](../../DEVELOPMENT.md) · [../engineering/collaboration/AGENT_COLLABORATION_GUIDE.md](../engineering/collaboration/AGENT_COLLABORATION_GUIDE.md)

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

See [ARCHITECTURE.md](../engineering/architecture/ARCHITECTURE.md) for the complete technical specification.

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

---

## 9. Sprint Execution Plan

Sprints are **2 weeks** long. Each sprint has a single goal, a defined feature slice from [`product/FEATURES.md`](../product/FEATURES.md), an explicit task list, and acceptance gates that must all be green before the sprint is considered done. No sprint closes with failing gates — the scope is reduced before quality is sacrificed.

### Sprint Overview

| Sprint | Goal | Feature Slice | Duration |
|--------|------|---------------|----------|
| [Sprint 1](#sprint-1--project-bootstrap--auth) | Working Go server + Firebase Auth + PostgreSQL + Flutter login end-to-end | F-001 (Auth) — partial | Weeks 1–2 |
| [Sprint 2](#sprint-2--circles--membership) | Teacher creates circle, invites students, student joins | F-002 (Circle Management) — core CRUD + membership | Weeks 3–4 |

---

### Sprint 1 — Project Bootstrap & Auth

**Goal:** Deliver a working Go server with Firebase Auth middleware and PostgreSQL connectivity, paired with a Flutter login/register screen that issues a real Firebase JWT validated end-to-end by the backend.

**Feature slice:** F-001 (Authentication) — register, login, token refresh, logout

#### Tasks

**Backend**
- Set up Go module, Echo v4 scaffold, project directory layout per ADR-007 (`backend/cmd/api/`, `backend/internal/`, `backend/migrations/`)
- Implement `GET /health` endpoint (returns `{"status":"ok"}`) — used by Docker health check
- Integrate Firebase Admin SDK for JWT verification; implement `backend/internal/auth` middleware
- Write migration `000001_create_users.up.sql` / `.down.sql` (users table: `id`, `firebase_uid`, `email`, `display_name`, `timezone`, `created_at`, `updated_at`)
- Implement `POST /api/v1/auth/register` — create user record on first Firebase sign-in
- Implement `POST /api/v1/auth/login` — return user profile for an authenticated Firebase UID
- Provision Hetzner CX22, configure DNS/TLS via Cloudflare, deploy Docker Compose (`api`, `postgres`)

**Mobile**
- Scaffold Flutter project under `mobile/` with Riverpod 2.x, `go_router`, and `firebase_auth` dependencies in `pubspec.yaml`
- Implement login screen and register screen (email + Google Sign-In; Apple Sign-In for iOS)
- Wire Firebase Auth SDK to login/register screens; store and refresh `idToken` via Riverpod `AsyncNotifier`
- Call `POST /api/v1/auth/register` on first sign-in; call `POST /api/v1/auth/login` on subsequent logins
- Implement basic home screen (placeholder) reached after successful authentication

#### Acceptance Gates

- [ ] `go test ./...` passes with **0 failures**
- [ ] `go test -tags=integration ./...` passes — includes an integration test that issues a real Firebase test token and validates it through the Go middleware end-to-end
- [ ] `flutter test` passes with **0 failures**
- [ ] `docker compose up -d` succeeds on a **fresh clone** with no pre-existing volumes (backend reaches `healthy`, postgres reaches `healthy`)
- [ ] `make migrate-fresh` runs all migrations on a fresh PostgreSQL schema with **0 errors**
- [ ] Firebase JWT issued by the test client is accepted by `POST /api/v1/auth/login` and returns `200 OK` with a user profile
- [ ] `golangci-lint run ./...` reports **0 violations**
- [ ] `flutter analyze` reports **0 issues**
- [ ] `gitleaks detect --source .` reports **0 secrets detected**
- [ ] OpenAPI spec `docs/contracts/openapi.yaml` documents `/health`, `/api/v1/auth/register`, and `/api/v1/auth/login`; `spectral lint` passes

---

### Sprint 2 — Circles & Membership

**Goal:** A teacher can create a Halaqaty circle, generate an invite code, and a student can join the circle using that code. Role-based access control (RBAC) is enforced — students cannot call teacher-only endpoints.

**Feature slice:** F-002 (Circle Management) — core CRUD + membership (create, invite, join)

#### Tasks

**Backend**
- Write migration `000002_create_circles.up.sql` / `.down.sql` (circles table: `id`, `name`, `description`, `invite_code`, `owner_id`, `created_at`, `updated_at`)
- Write migration `000003_create_circle_members.up.sql` / `.down.sql` (circle_members table: `circle_id`, `user_id`, `role` (`teacher`|`supervisor`|`student`), `joined_at`)
- Implement `POST /api/v1/circles` — create circle (teacher role required); generate unique `invite_code`
- Implement `GET /api/v1/circles/:id` — return circle details + member count (authenticated member only)
- Implement `POST /api/v1/circles/:id/invite` — regenerate invite code (teacher role required)
- Implement `POST /api/v1/circles/:id/join` — student joins circle by providing `invite_code` in request body
- Implement RBAC middleware in `backend/internal/shared/middleware`: reads `circle_members` for the authenticated user and circle ID; attaches `CircleRole` to request context; returns `403` if role check fails

**Mobile**
- Implement circle creation screen: form for name and description, calls `POST /api/v1/circles`, navigates to circle detail on success
- Implement invite code screen: displays the circle's `invite_code` with a copy-to-clipboard button; calls `POST /api/v1/circles/:id/invite` to regenerate
- Implement circle join screen: text field for invite code, calls `POST /api/v1/circles/:id/join`, navigates to circle detail on success

#### Acceptance Gates

- [ ] Teacher can create a circle, view the invite code, and a student can join using that code — verified by an **E2E integration test** that exercises all four endpoints in sequence against a real PostgreSQL database
- [ ] `POST /api/v1/circles` called by a student (role: `student`) returns **`403 Forbidden`** — tested via integration test
- [ ] `POST /api/v1/circles/:id/invite` called by a non-teacher member returns **`403 Forbidden`** — tested via integration test
- [ ] `make migrate-down` rolls back `000003` then `000002` cleanly on a fresh schema with **0 errors**
- [ ] `go test ./...` and `go test -tags=integration ./...` pass with **0 failures**
- [ ] `flutter test` passes with **0 failures**
- [ ] `golangci-lint run ./...` reports **0 violations**
- [ ] `flutter analyze` reports **0 issues**
- [ ] `gitleaks detect --source .` reports **0 secrets detected**
- [ ] OpenAPI spec updated for all four new endpoints; `spectral lint` passes

