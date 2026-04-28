# Halaqaty — Project Plan Review

> **Reviewers:** Product Manager (Alex) · Senior Project Manager (SeniorProjectManager)  
> **Date:** 2026-04-28  
> **Scope:** All files in `docs/`, `README.md`, `DEVELOPMENT.md`, `.specify/memory/`  
> **Verdict:** 🟡 **Strong foundation — address 6 critical gaps before coding begins**

---

## Executive Summary

This is one of the most thoroughly planned pre-code repositories we've reviewed. The documentation suite is **unusually comprehensive** for a planning-phase project: 12+ English documents, 4 Arabic mirrors, 6 ADRs, a competitor analysis, a full user journey, and a governing constitution. The team has clearly invested in thinking before building.

**However, thorough is not the same as ready.** Our review surfaces **23 specific findings** — 6 critical, 9 important, 8 minor — that should be resolved before the first line of production code is written. The most significant gaps are around **team capacity planning, testing strategy, concrete acceptance criteria dates, and a missing OpenAPI contract file**.

---

## Scorecard

| # | Checklist Area | Grade | Summary |
|---|----------------|-------|---------|
| 1 | Documentation Completeness | 🟢 A | Exceptional breadth; one missing artifact (OpenAPI spec) |
| 2 | Process Flow | 🟢 A- | Strong Spec-Kit pipeline; minor sequencing issue in PLAN.md |
| 3 | Logic & Architecture | 🟢 A | Solid architecture; minor schema inconsistencies |
| 4 | Dependencies & Risks | 🟡 B+ | Dependencies listed; mitigation strategies present but incomplete |
| 5 | Roles & Responsibilities | 🟠 B- | RACI exists but is abstract; no named humans except "Karim" |
| 6 | Milestones & Deliverables | 🟠 B- | Timeline exists but milestones lack acceptance gates |
| 7 | Questions & Assumptions | 🟢 A | All 26 OQs resolved; competitor analysis strong |

**Overall: B+ / Strong — needs targeted hardening, not a rewrite.**

---

## 1. Documentation Completeness

### What's Present ✅

| Document | Purpose | Quality |
|----------|---------|---------|
| [README.md](../../README.md) | Project overview, tech stack, structure, roadmap | ⭐⭐⭐⭐⭐ |
| [PRD.md](../management/product/PRD.md) | Business-first product requirements | ⭐⭐⭐⭐⭐ |
| [PLAN.md](../management/planning/PLAN.md) | Master project plan (features, timeline, business model) | ⭐⭐⭐⭐ |
| [FEATURES.md](../management/product/FEATURES.md) | Detailed feature specs with acceptance criteria | ⭐⭐⭐⭐⭐ |
| [ARCHITECTURE.md](../engineering/architecture/ARCHITECTURE.md) | DB schema, API endpoints, security model | ⭐⭐⭐⭐⭐ |
| [DEPLOYMENT.md](../engineering/deployment/DEPLOYMENT.md) | Phase-by-phase infra plan with costs | ⭐⭐⭐⭐⭐ |
| [JOURNEY.md](../management/product/JOURNEY.md) | Screen-by-screen user journey (teacher + student) | ⭐⭐⭐⭐⭐ |
| [MVP_DECISION_REGISTER.md](../management/product/MVP_DECISION_REGISTER.md) | All 26 frozen product decisions with rationale | ⭐⭐⭐⭐⭐ |
| [EXECUTION_PLAYBOOK.md](../engineering/development/EXECUTION_PLAYBOOK.md) | Weekly cadence, GTM, KPIs, RACI | ⭐⭐⭐⭐ |
| [SYNC_GUIDE.md](../management/arabic/SYNC_GUIDE.md) | Bilingual doc sync policy | ⭐⭐⭐⭐ |
| [COMPETITOR_ANALYSIS.md](../management/business/QURAN_MEMORIZATION_COMPETITOR_ANALYSIS.md) | 12-app competitive landscape | ⭐⭐⭐⭐ |
| [DEVELOPMENT.md](../../DEVELOPMENT.md) | Developer guide, Spec-Kit workflow, quality gates | ⭐⭐⭐⭐⭐ |
| [6 ADRs](../engineering/architecture/adr) | Architecture Decision Records | ⭐⭐⭐⭐ |
| Arabic mirrors (4 files) | PRD, PLAN, FEATURES, README in Arabic | ⭐⭐⭐⭐ |

### What's Missing ❌

> [!CAUTION]
> **F-01 (Critical): `docs/contracts/` directory is referenced but does not exist.**
> DEVELOPMENT.md line 299 references `docs/contracts/` for "OpenAPI spec + WebSocket event catalog." ARCHITECTURE.md defines 50+ API endpoints. But no `openapi.yaml` or WebSocket event schema file exists anywhere in the repo. This is the single most important missing artifact — without it, the Spec-Kit pipeline has no contract to validate against.

> [!WARNING]
> **F-02 (Important): No `TESTING_STRATEGY.md` or equivalent.**
> DEVELOPMENT.md defines quality gates (unit tests, integration tests, linting) but there is no document describing *what* gets tested, coverage targets, test data strategy, or E2E testing approach. For a real-time system with WebSocket + WebRTC + queue state machines, a testing strategy is not optional.

> [!NOTE]
> **F-03 (Minor): No `CONTRIBUTING.md` as a standalone file.**
> README.md has a contributing section, but it's minimal (6 bullet points). For an open-source MIT project, a standalone CONTRIBUTING.md with PR template, code style guide, and issue template would reduce friction for external contributors.

---

## 2. Process Flow

### Strengths ✅

- **Spec-Kit pipeline is exceptionally well-defined.** The 9-step workflow in DEVELOPMENT.md (specify → clarify → plan → checklist → tasks → analyze → implement → verify → PR) is the most rigorous AI-assisted dev workflow I've seen in a planning-phase project.
- **Pre-flight checklist** ensures no feature enters development without being Approved, having all OQs resolved, and having a documented user journey.
- **Quality gates** are concrete and enforceable (9 specific commands with pass/fail criteria).
- **Feature status lifecycle** is clear: Proposed → Approved → In Progress → Shipped → Frozen.

### Issues Found

> [!WARNING]
> **F-04 (Important): PLAN.md timeline has a sequencing problem.**
> Month 5 = Recitation Queue System, but Month 4 = Live Sessions (LiveKit). The queue is designed to operate *within* a live session (F-003 depends on F-005). However, the FEATURES.md status table assigns both to Phase 2, and the dependency list in FEATURES.md F-003 correctly lists F-005 as a dependency. The PLAN.md monthly breakdown should make it explicit that LiveKit integration (Month 4) must reach a functional state before queue work (Month 5) can begin — or acknowledge that Month 4-5 are overlapping with staggered starts.

> [!NOTE]
> **F-05 (Minor): FEATURES.md status table shows all P0 features as 🟡 Approved, but the detailed sections below still show 🔵 Proposed.**
> The status table at the top of FEATURES.md correctly marks F-001 through F-006 as `🟡 Approved`, but the detailed discussion sections (e.g., line 53: `**Status:** 🔵 Proposed`) still say `🔵 Proposed`. This inconsistency will confuse the Spec-Kit pre-flight checklist, which checks for `🟡 Approved`.

> [!NOTE]
> **F-06 (Minor): Design Decision IDs are duplicated.**
> DD-005 appears in both F-002 (Circle Management, line 112) and F-003 (Queue System, line 212) with different content. DD-006 is similarly duplicated. DD-009 appears in both F-003 (line 216) and F-004 (line 254). These should have unique sequential IDs.

---

## 3. Logic & Architecture

### Strengths ✅

- **Database schema is production-grade.** 12 tables with proper FK constraints, UNIQUE constraints, CHECK constraints, UUIDs, and timestamptz. This is not a sketch — it's implementable.
- **API design is RESTful and comprehensive.** 50+ endpoints across 8 resource groups, with proper HTTP verbs and nested resource patterns.
- **Security model is thoughtful.** Per-turn audio publish permissions, teacher-only grading, LiveKit token scoping, rate limiting, and input validation are all documented.
- **LiveKit integration flow** is exceptionally detailed — from room creation to token generation to Flutter connection, with actual Go and Dart code patterns.
- **ADRs provide rationale** for every major technology choice (modular monolith, Echo v4, Riverpod, Firebase auth boundary, feature flags, golang-migrate).

### Issues Found

> [!CAUTION]
> **F-07 (Critical): `users.fcm_token` is a single TEXT column, but users can have multiple devices.**
> ARCHITECTURE.md line 443 defines `fcm_token TEXT` as a single field. A teacher using both a phone and tablet needs two FCM tokens. This should be a separate `device_tokens` table or a JSONB array, with token registration/deregistration logic. FEATURES.md F-001 acceptance criteria line 67 says "Device token registration for FCM push notifications" — the schema doesn't support this properly.

> [!WARNING]
> **F-08 (Important): `messages` table has `circle_id NOT NULL` but also supports DMs.**
> ARCHITECTURE.md line 530: `circle_id UUID FK → circles.id NOT NULL` combined with line 531: `dm_recipient_id UUID FK → users.id` for direct messages. If circle_id is NOT NULL, DMs (which don't belong to a circle) cannot be stored. The constraint should be nullable, or DMs need a separate table.

> [!WARNING]
> **F-09 (Important): No `grading_policy` column on `circles` table.**
> FEATURES.md F-003 acceptance criteria states "Grading mode configurable per circle (required or optional per completed turn)." The `circles` table in ARCHITECTURE.md has no column for this setting. Add a `grading_policy VARCHAR(20) CHECK IN ('required','optional') DEFAULT 'required'` column.

> [!NOTE]
> **F-10 (Minor): `schedules` table stores `start_time TIME` and `end_time TIME` separately from timezone.**
> This is correct per OQ-019 (UTC in DB), but `TIME` type in PostgreSQL is timezone-unaware. The `timezone` column stores the IANA string, but the conversion logic from local time to UTC is not documented. Clarify whether `start_time`/`end_time` are stored in local time (with timezone for conversion) or in UTC.

---

## 4. Dependencies & Risks

### Strengths ✅

- **External dependencies are listed**: Firebase Auth, FCM, LiveKit, MinIO, PostgreSQL, Hetzner, Cloudflare.
- **PRD.md Section 11** has a 6-item risk register with mitigations.
- **DEPLOYMENT.md** has phase-specific risk tables with likelihood/impact/mitigation.
- **MVP Decision Register** acts as a binding constraint document — this is excellent for preventing scope creep.
- **Feature flags** (ADR-005) provide a safety net for premature feature activation.

### Issues Found

> [!CAUTION]
> **F-11 (Critical): No dependency on a specific LiveKit SDK version or compatibility matrix.**
> The project depends on `livekit_client` (Flutter) and `livekit-server-sdk-go` (Go backend), but no version pinning or compatibility requirements are documented. LiveKit is a fast-moving project — breaking changes between versions are common. Pin versions in the plan, and document the minimum LiveKit server version required.

> [!CAUTION]
> **F-12 (Critical): Firebase Auth is a single point of failure with no fallback documented.**
> If Firebase Auth has an outage (it has had them), users cannot register, log in, or refresh tokens. No fallback or degraded-mode strategy is documented. At minimum, document: (a) what happens to active sessions during a Firebase outage, (b) whether cached tokens allow continued use, (c) whether a migration path away from Firebase Auth exists.

> [!WARNING]
> **F-13 (Important): Hetzner Nuremberg location may have latency issues for Egypt-first pilot.**
> DEPLOYMENT.md specifies Nuremberg, Germany. The primary pilot market is Egypt (PRD Section 10). Nuremberg → Cairo latency is ~40-60ms, which is acceptable for REST/WebSocket but may affect LiveKit audio quality. Consider Hetzner Helsinki (closer to Middle East via Turkey) or Hetzner Falkenstein, or document the latency acceptance threshold.

> [!NOTE]
> **F-14 (Minor): No risk entry for App Store / Play Store rejection.**
> Apple's review process is notoriously unpredictable for apps with audio/video features, especially religious content apps. A risk entry with mitigation (e.g., pre-submission review checklist, TestFlight beta period) would be prudent.

---

## 5. Roles & Responsibilities

### Strengths ✅

- **RACI matrix exists** in EXECUTION_PLAYBOOK.md with 4 workstreams.
- **In-app roles** (Teacher, Student, Supervisor, Institution Admin) are extremely well-defined with capabilities and limitations.
- **Approval authority** is clear: "PRs reviewed and merged by Karim only."

### Issues Found

> [!CAUTION]
> **F-15 (Critical): No team composition or capacity plan.**
> The RACI matrix references roles (Product Manager, CEO/Founder, Tech Lead, Growth Lead, Product Analyst, Teacher Advisors) but no actual team members are named except "Karim" (implied as founder). Key questions:
> - How many developers are working on this? Is Karim the sole developer?
> - If solo, is the 12-month timeline realistic for Go backend + Flutter app + LiveKit integration + PostgreSQL + DevOps?
> - Who handles the Arabic localization and teacher pilot operations?
>
> **This is the single biggest risk to the project.** A brilliant plan with no capacity to execute it is just a document.

> [!WARNING]
> **F-16 (Important): No code review process beyond "merged by Karim only."**
> DEVELOPMENT.md says PRs are "opened by Copilot, reviewed and merged by Karim only." If Karim is the sole reviewer and sole developer, there is no independent code review. This is acceptable for a solo founder MVP, but it should be explicitly acknowledged as a risk with a mitigation (e.g., "Copilot reviews are supplemented by manual security review of auth, payment, and privacy-sensitive code").

---

## 6. Milestones & Deliverables

### Strengths ✅

- **12-month timeline** broken into monthly chunks with specific deliverables.
- **4-phase release strategy** (Internal Alpha → Beta → iOS Public → Web) is realistic.
- **4-phase deployment strategy** with specific cost thresholds and scaling triggers.
- **PRD milestones** (M1–M4) define outcomes, not just dates.

### Issues Found

> [!CAUTION]
> **F-17 (Critical): Milestones have no acceptance gates or success criteria.**
> PRD Section 12 lists 4 milestones:
> - M1: "MVP scope sign-off + pilot readiness"
> - M2: "Pilot launch with queue-centric workflows"
> - M3: "Public beta with retention instrumentation"
> - M4: "Monetization experiment + institution pilot package"
>
> None of these have measurable acceptance criteria. What does "pilot readiness" mean? How many features must be shipped? What's the quality bar? Compare with the PRD Section 3 goals (300+ MAT, 60% WAU/MAU, etc.) — those are measurable but not tied to specific milestones.
>
> **Recommendation:** Map each milestone to specific:
> - Features that must be `✅ Shipped` (by F-ID)
> - Quality gates that must be green
> - KPI baselines that must be established

> [!WARNING]
> **F-18 (Important): Month 7 includes P1 features (Progress Tracking, Quran Map, PDF Export) but the timeline positions these after the MVP cut.**
> EXECUTION_PLAYBOOK.md clearly states MVP = P0 features only. PLAN.md Month 7 includes "Visual Quran map" and "Basic PDF report export" — both are P1/P2 in FEATURES.md. Either:
> (a) Move these to Month 9-10 (post-beta), or
> (b) Promote them to P0 in the MVP cut with justification.
> Currently they create a scope inconsistency.

> [!WARNING]
> **F-19 (Important): No buffer time in the 12-month plan.**
> Every month is packed with deliverables. There is zero slack for: bug fixing, pilot feedback integration, unexpected technical challenges, App Store review delays, or personal time off. Industry standard is 15-20% buffer. A solo or small-team project should have 25%+.
>
> **Recommendation:** Either extend the timeline to 14-15 months or explicitly deprioritize some Month 9-10 items as "stretch goals."

---

## 7. Questions for Clarification

### Assumptions Not Documented

> [!WARNING]
> **F-20 (Important): Internet connectivity assumption.**
> The entire product assumes always-on internet for core features (live sessions, queue, chat). JOURNEY.md has an offline behavior section, but it's thin — "Join live session: Blocked." For the Egypt pilot market, mobile data reliability varies significantly. Document the minimum bandwidth requirement for a LiveKit audio session and whether the app will work on 3G/EDGE.

> [!NOTE]
> **F-21 (Minor): Quran data model assumption.**
> The queue system references `surah_name VARCHAR(100)` as a string. There is no Quran reference data table (114 Surahs, their Ayah counts, Juz boundaries). Validation of Ayah ranges ("Surah Al-Baqarah has 286 Ayahs, not 300") requires this data. ARCHITECTURE.md line 760 says "Ayah numbers validated against known Surah lengths" but doesn't specify where that data lives.

> [!NOTE]
> **F-22 (Minor): Monetization timing assumption.**
> PRD Section 9 lists pricing tiers but no activation date. The MVP is free. At what point does the paywall activate? Before or after the 300-teacher target? This affects feature flag architecture (ADR-005) and the `users` table (no `subscription_tier` or `billing` columns exist in the current schema).

> [!NOTE]
> **F-23 (Minor): Competitor analysis is not cross-referenced with feature decisions.**
> The competitor analysis recommends "Now: Hifz mode UX + script profile parity" and "Now: Free correction quota." Neither of these maps to any feature in FEATURES.md. The competitor analysis should either be reconciled with the feature backlog or marked as "strategic input, not committed scope."

---

## Prioritized Action Plan

### 🔴 Before Any Code (Critical — Block implementation)

| # | Finding | Action | Owner | Effort |
|---|---------|--------|-------|--------|
| F-01 | Missing `docs/contracts/` | Create `openapi.yaml` from ARCHITECTURE.md Section 5 endpoints + WebSocket event schema from Section 2.2 | Tech Lead | 2-3 days |
| F-07 | Single FCM token column | Redesign to `device_tokens` table (user_id, token, platform, created_at) | Tech Lead | 1 hour |
| F-15 | No team capacity plan | Document team size, velocity assumptions, and validate 12-month timeline against available capacity | Karim / PM | 1 day |
| F-17 | Milestones lack gates | Add acceptance criteria to M1-M4 with specific F-IDs, quality gates, and KPI baselines | PM | Half day |
| F-11 | No SDK version pinning | Pin `livekit_client`, `livekit-server-sdk-go`, `firebase_auth`, Echo v4 versions | Tech Lead | 1 hour |
| F-12 | Firebase SPOF | Document degraded-mode behavior and cached-token policy | Tech Lead | Half day |

### 🟡 Before Pilot Launch (Important — Must fix before users see it)

| # | Finding | Action | Owner | Effort |
|---|---------|--------|-------|--------|
| F-02 | No testing strategy | Create `TESTING_STRATEGY.md` covering unit, integration, E2E, and WebSocket/LiveKit testing | Tech Lead | 1 day |
| F-04 | Timeline sequencing | Clarify Month 4-5 overlap for LiveKit → Queue dependency | PM | 1 hour |
| F-05 | Status inconsistency | Update FEATURES.md detailed sections to match status table (🟡 Approved for P0s) | PM | 30 min |
| F-08 | Messages table DM bug | Make `circle_id` nullable or create separate `direct_messages` table | Tech Lead | 1 hour |
| F-09 | Missing grading_policy | Add column to `circles` schema | Tech Lead | 15 min |
| F-16 | No code review beyond Karim | Document the solo-developer review policy and security-sensitive review checklist | PM | 1 hour |
| F-18 | P1 features in MVP timeline | Move Month 7 P1/P2 items to Month 9-10 or promote with justification | PM | 30 min |
| F-19 | No buffer time | Add 2-3 months buffer or mark specific items as stretch goals | PM | 1 hour |
| F-20 | Bandwidth assumption | Document minimum bandwidth for LiveKit audio session + 3G/EDGE testing plan | Tech Lead | 1 hour |

### 🟢 Housekeeping (Minor — Fix when convenient)

| # | Finding | Action | Owner | Effort |
|---|---------|--------|-------|--------|
| F-03 | No CONTRIBUTING.md | Create standalone file with PR template and code style | PM | 1 hour |
| F-06 | Duplicate DD IDs | Re-number DD-005 through DD-010 to be unique across all features | PM | 30 min |
| F-10 | Schedule timezone docs | Clarify TIME storage semantics in ARCHITECTURE.md | Tech Lead | 15 min |
| F-13 | Hetzner latency for Egypt | Benchmark Nuremberg → Cairo latency; document threshold | Tech Lead | 1 hour |
| F-14 | App Store rejection risk | Add risk entry to PRD risk register | PM | 15 min |
| F-21 | Quran reference data | Add `quran_surahs` reference table or document static validation source | Tech Lead | 1 hour |
| F-22 | Monetization timing | Define paywall activation trigger in PRD | PM | 30 min |
| F-23 | Competitor reconciliation | Cross-reference competitor recs with FEATURES.md or mark as strategic input | PM | 1 hour |

---

## What's Excellent (Don't Change This)

These elements are **best-in-class** and should be preserved:

1. **MVP Decision Register** — Every product decision is frozen, rationale'd, and has an amendment process. This is rare and extremely valuable.
2. **User Journey document** — Screen-by-screen flows with error states AND offline behavior. Most projects don't have this until post-launch.
3. **Spec-Kit workflow** — The 9-step pipeline with pre-flight checklist is the most disciplined AI-assisted dev process we've encountered.
4. **Privacy-first recording policy** — Keeping recording disabled until a full consent framework is approved shows mature product thinking.
5. **Queue state machine** — The detailed state diagram, round system, and real-time sync requirements are implementation-ready.
6. **Deployment cost modeling** — $8/month → $500+/month phased plan with specific scaling triggers is exactly how a bootstrapped product should plan infra.
7. **Arabic documentation mirrors** — Bilingual docs for a bilingual market, with clear sync policy.
8. **ADR discipline** — 6 ADRs covering every major technology choice, with status, context, decision, and consequences.

---

## Final Recommendation

> **From the Product Manager (Alex):**
> This project has the best pre-code documentation I've seen for a solo/small-team product. The product thinking is sharp — the queue system as a wedge, audio-only MVP, teacher-first strategy, and no-ads commitment are all correct calls. The biggest product risk is not the plan quality — it's **execution capacity**. Resolve F-15 (team capacity) honestly before committing to the timeline. If this is a solo build, either extend the timeline to 16-18 months or cut F-006 (Scheduling) from MVP and handle it with manual coordination in the pilot.

> **From the Project Manager (SeniorProjectManager):**
> The documentation is comprehensive enough to hand to a development team today — almost. The 6 critical findings (F-01, F-07, F-11, F-12, F-15, F-17) are standard pre-implementation gaps that take 3-5 days total to close. The process flow (Spec-Kit) is excellent, but it only works if the contract artifacts it validates against actually exist (F-01). Fix the critical items, add buffer to the timeline, and this project is **ready to start Sprint 1**.
