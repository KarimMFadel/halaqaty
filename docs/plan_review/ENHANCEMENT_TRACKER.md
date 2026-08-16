# Halaqaty — Enhancement Tracker

<!-- ─────────────────────────────────────────────────────────────────────────
     HOW TO USE THIS FILE (read before editing)
     ─────────────────────────────────────────────────────────────────────────
     This is a living document. When you resolve an enhancement:
       1. Change its Status cell to ✅ Done
       2. Fill in "Resolved By" (model name) and "Resolved On" (date)
       3. Add one line to the Amendment Log at the bottom of this file
     When you identify NEW enhancements, add rows to the appropriate section
     and append an entry to the Amendment Log.
     ──────────────────────────────────────────────────────────────────────── -->

> **Initial Review Date:** 2026-06-20  
> **Reviewed By:** Claude Sonnet 4.6 (`claude-sonnet-4.6`)  
> **Overall Project Rating:** 8.5 / 10
>
> **Second Review Date:** 2026-06-20  
> **Reviewed By:** Antigravity AI (Gemini 3.1 Pro)  
> **Overall Project Rating:** 9.5 / 10

---

## Status Legend

| Symbol | Meaning |
|--------|---------|
| ⬜ Pending | Not yet addressed |
| 🔄 In Progress | Being actively worked on |
| ✅ Done | Resolved and verified |
| 🚫 Won't Fix | Deliberately deferred or out of scope |

---

## Priority 1 — Must Fix Before Sprint 1

*These items block or significantly risk the development workflow.*

| ID | Category | Enhancement | Impact | Status | Resolved By | Resolved On |
|----|----------|-------------|--------|--------|-------------|-------------|
| E-01 | CI/CD | No lint/security CI pipeline — `golangci-lint`, `flutter analyze`, `gitleaks`, and `spectral lint` were not automated | Sprint acceptance gates reference these tools but they had no automation, meaning failing code could be merged undetected | ✅ Done | Claude Sonnet 4.6 | 2026-06-20 |
| E-02 | Security | No `SECURITY.md` — `CONTRIBUTING.md` mentioned responsible disclosure but no policy existed | External researchers had no formal channel; security invariants were only in the constitution | ✅ Done | Claude Sonnet 4.6 | 2026-06-20 |
| E-03 | Tooling | No `Makefile` — sprints reference `make migrate-fresh` and `make migrate-down` but the file didn't exist | Sprint 1 acceptance gates would fail immediately with "command not found" | ✅ Done | Claude Sonnet 4.6 | 2026-06-20 |
| E-04 | Docs | `PROJECT_PLAN.md` ToC was broken — sections 2, 7, and 9 missing from Table of Contents; body also missing stubs for sections 2 and 7 | Any agent or contributor navigating by ToC would miss target user context and business model references | ✅ Done | Claude Sonnet 4.6 | 2026-06-20 |
| E-05 | Docs | `EXECUTION_PLAYBOOK.md` §5 GTM and §6 KPIs still contained full duplicated content from `PRD.md §10` and `PRD.md §8` (audit findings D-09, D-17) | Two sources of truth for GTM and KPI data will inevitably drift | ✅ Done | Claude Sonnet 4.6 | 2026-06-20 |

---

## Priority 2 — Fix Before Beta Launch

*These items carry product risk, architectural gaps, or data accuracy issues.*

| ID | Category | Enhancement | Impact | Status | Resolved By | Resolved On |
|----|----------|-------------|--------|--------|-------------|-------------|
| E-06 | Docs | Quran data source not documented — `ARCHITECTURE.md` defines a `quran_surahs` table but never explains where canonical Surah/Ayah data originates (static JSON seed, third-party API, bundled asset, etc.) | Canonical data source is now documented through the seeded `quran_surahs` reference table and test seed flow; remaining gap is only if a second source-of-truth is introduced later | ✅ Done | Copilot | 2026-08-16 |
| E-07 | Docs | Rate limiting was missing from the architecture security section | `ARCHITECTURE.md §6.4` now defines REST, WebSocket, message, and upload rate-limit policies | ✅ Done | Codex | 2026-08-09 |
| E-08 | Product | No offline / low-bandwidth strategy documented — the primary user base (Middle East, Africa, South Asia) frequently operates on unstable mobile connections | Offline/low-bandwidth strategy now fully documented with Dio config, cache TTLs, offline capabilities/restrictions, reconnection backoff, and WebSocket recovery | ✅ Done | Copilot | 2026-08-16 |
| E-09 | Planning | Capacity plan missing — the plan review scored "Roles & Responsibilities" B+ because this gap remained; a solo developer shipping 8 features across 12 months needs explicit monthly hour-budget or per-sprint velocity targets | Capacity plan now added to `PROJECT_PLAN.md` with hours/week, velocity assumptions, contingency triggers, and solo-dev buffer guidance | ✅ Done | Copilot | 2026-08-16 |
| E-10 | Docs | `PRD.md` Owner field lists `"GPT-5.3-Codex acting as Project Manager"` — a model name is not an appropriate owner for a living product document | Confuses contributors about accountability; model versions change; a person or role name (e.g. "Karim Fadel — Product Owner") is required | ⬜ Pending | — | — |
| E-17 | Planning | Add Sprint 3-6 skeletons to `PROJECT_PLAN.md` | Even one-liner sprint goals would help with execution tracking | ⬜ Pending | — | — |
| E-18 | Docs | Add error code registry | Centralized error codes prevent fragmented API responses | ⬜ Pending | — | — |
| E-19 | Architecture | Add database indexing strategy | Canonical indexing policy, naming conventions, review checklist, and periodic audit process now documented in `ARCHITECTURE.md` §6.4.1 | ✅ Done | Copilot | 2026-08-16 |
| E-20 | Architecture | Create a data migration runbook | Needed for safe schema changes in production | ⬜ Pending | — | — |
| E-21 | Architecture | Add `updated_at` column to tables that lack it | `updated_at` coverage audit, policy, and migration plan now documented in `ARCHITECTURE.md` §6.4.2 with table-by-table status | ✅ Done | Copilot | 2026-08-16 |
| E-22 | Architecture | Add pagination strategy | Canonical cursor-based pagination policy, request/response formats, and implementation notes now documented in `docs/engineering/api-docs/README.md` | ✅ Done | Copilot | 2026-08-16 |
| E-23 | Docs | Clean up `EXECUTION_PLAYBOOK.md` RACI roles | The solo-phase RACI is now present and internally consistent; remaining: trim any redundant legacy framing if the playbook is simplified further | ✅ Done | Copilot | 2026-08-16 |
| E-24 | Architecture | Add observability design | Structured logging, request tracing, metric naming, and dashboard guidance now fully documented in `docs/engineering/deployment/DEPLOYMENT.md` §7.5–7.6 | ✅ Done | Copilot | 2026-08-16 |

---

## Priority 3 — Nice to Have

*Housekeeping, polish, and long-term maintainability improvements.*

| ID | Category | Enhancement | Impact | Status | Resolved By | Resolved On |
|----|----------|-------------|--------|--------|-------------|-------------|
| E-11 | Spec-Kit | Feature 001 lacked `plan.md` and `tasks.md` | The full `specs/001-auth-roles-profile/` artifact set now exists, including plan, tasks, contracts, quickstart, and validation report | ✅ Done | Codex | 2026-08-09 |
| E-12 | Docs | Engineering system-design, API-docs, and guides directories contained placeholder READMEs | All three READMEs now contain substantive contract-anchored guidance and clearly label proposals | ✅ Done | Codex | 2026-08-09 |
| E-13 | Docs | No `CHANGELOG.md` — no history of documentation or planning changes beyond git blame | Once implementation begins and releases are cut, the absence of a changelog makes it hard to communicate changes to pilot users or contributors | ⬜ Pending | — | — |
| E-14 | Docs | Arabic README (`docs/management/arabic/README_AR.md`) is not linked from the main `README.md` top section | Discoverability issue is now closed because the top-level README links the Arabic docs entry point | ✅ Done | Copilot | 2026-08-16 |
| E-15 | Docs | `TESTING_STRATEGY.md` exists in `docs/engineering/development/` but is not referenced or linked from `DEVELOPMENT.md` | The developer guide now points to the testing strategy, but the relationship between strategy, contracts, and execution is still worth keeping visible | ✅ Done | Copilot | 2026-08-16 |
| E-16 | Planning | Timeline optimism — the 300+ Monthly Active Teachers target at 12 months has no concrete deferral trigger; `PROJECT_PLAN.md` correctly notes 14–15 months is realistic but provides no decision criteria for when to officially defer a milestone | Milestone deferral criteria, decision owners, and authorization dates now documented in `PROJECT_PLAN.md` §8 "Milestone Deferral Criteria" table with explicit decision authority and contingency trigger conditions | ✅ Done | Copilot | 2026-08-16 |
| E-25 | Docs | Add sequence diagrams to `ws_events.md` | Clarifies complex multi-event flows | ⬜ Pending | — | — |
| E-26 | Architecture | Define health check contract | The `/health` endpoint is missing from `openapi.yaml` | ⬜ Pending | — | — |
| E-27 | Architecture | Add `soft_deleted_at` column to `users` table | Soft delete enables grace period recovery | ⬜ Pending | — | — |
| E-28 | Docs | Document file upload flow | Clarifies MinIO presigned URL workflow | ⬜ Pending | — | — |
| E-29 | Docs | Create a glossary of Islamic/Quranic terms | Helps non-Arabic-speaking contributors | ⬜ Pending | — | — |
| E-30 | Policy | Add a data retention policy document | Clarifies how long messages and logs are kept | ⬜ Pending | — | — |
| E-31 | Architecture | Document the feature flag storage | ADR-005 lacks specification on where flags are stored | ⬜ Pending | — | — |
| E-32 | Product | Add onboarding metrics definition | Missing instrumentation plan for onboarding requirements | ⬜ Pending | — | — |
| E-33 | Policy | Create API versioning policy | Clarifies breaking change policy | ⬜ Pending | — | — |
| E-34 | Architecture | Add `display_order` to `circle_members` | Member list ordering is currently undefined | ⬜ Pending | — | — |
| E-35 | Architecture | Document voice note encoding format | Affects mobile recording and chat playback | ⬜ Pending | — | — |
| E-36 | Docs | Add system limits table | Consolidates all limits in one place | ⬜ Pending | — | — |
| E-37 | Architecture | Add `schedule_id` FK to `sessions` table | No foreign key connecting a session instance to its recurring schedule | ⬜ Pending | — | — |
| E-38 | Compliance | Add Apple privacy nutrition labels | iOS App Store requirement | ⬜ Pending | — | — |
| E-39 | Product | Create a pilot feedback collection plan | Clarifies how feedback will be collected | ⬜ Pending | — | — |
| E-40 | CI/CD | Arabic `SYNC_GUIDE` could use automation | The manual sync process will inevitably drift | ⬜ Pending | — | — |

---

## Summary Scorecard

| Priority | Total | ✅ Done | 🔄 In Progress | ⬜ Pending |
|----------|-------|---------|----------------|-----------|
| P1 — Before Sprint 1 | 5 | **5** | 0 | 0 |
| P2 — Before Beta | 13 | **9** | 0 | 4 |
| P3 — Nice to Have | 22 | **5** | 0 | 17 |
| **Total** | **40** | **19** | **0** | **21** |

> **2026-08-16 update:** All In Progress items resolved by adding content to suitable existing docs. E-08 (Offline/low-bandwidth strategy) → guides/README.md. E-09 (Capacity plan) → PROJECT_PLAN.md. E-16 (Deferral triggers) → PROJECT_PLAN.md. E-19 (Indexing strategy) → ARCHITECTURE.md §6.4.1. E-21 (`updated_at` coverage) → ARCHITECTURE.md §6.4.2. E-22 (Pagination policy) → api-docs/README.md. E-24 (Observability design) → DEPLOYMENT.md §7.5–7.6. No new .md files created; all content integrated into existing suitable locations.

---

## Amendment Log

*Append one row every time an enhancement is resolved, a new one is added, or the rating changes. Never delete rows.*

| Date | Enhancement(s) | Action | Model / Agent | Notes |
|------|----------------|--------|---------------|-------|
| 2026-06-20 | E-01 through E-16 | Initial review and tracker created | Claude Sonnet 4.6 (`claude-sonnet-4.6`) | Full docs + plan review. Overall rating: 8.5/10. Sixteen enhancements identified across 3 priority levels. |
| 2026-06-20 | E-01, E-02, E-03, E-04, E-05 | Resolved — Priority 1 complete | Claude Sonnet 4.6 (`claude-sonnet-4.6`) | Created `.github/workflows/lint.yml`, `.spectral.yaml`, `SECURITY.md`, `Makefile`. Fixed `PROJECT_PLAN.md` ToC (added §2, §7, §9). Replaced `EXECUTION_PLAYBOOK.md` §5 and §6 duplicated content with "Migrated" notices. |
| 2026-06-20 | E-17 through E-40 | Additional review points added | Antigravity AI (Gemini 3.1 Pro) | Conducted a comprehensive documentation review; added 24 new enhancements across P2 and P3. Overall rating bumped to 9.5/10. Noted usage of Antigravity AI chat. |
| 2026-06-30 | E-19 (partial) | Grade enum conflict resolved — 4-grade vs 6-grade mismatch across ARCHITECTURE.md and FEATURES.md fixed | Claude Sonnet 4.6 | F-003/F-007 grade scale unified to 5-grade canonical: `excellent/good/acceptable/needs_review/repeat`. Updated ARCHITECTURE.md §4.0, FEATURES.md F-003, openapi.yaml GradeRequest + QueueEntry, ws_events.md. |
| 2026-06-30 | E-21 (partial) | Added `updated_at` to `memorization_progress` as part of F-007 spec | Claude Sonnet 4.6 | Migration 0008 adds `updated_at TIMESTAMPTZ` to `memorization_progress`. |
| 2026-06-30 | E-19, E-22 (partial) | F-007 Enhanced Student Progress Tracking approved and fully specced | Claude Sonnet 4.6 | F-007 upgraded from Proposed → Approved. New file `F-007-SPEC.md` created. 7 new Progress API endpoints added to `openapi.yaml` with full schema. 16 new OpenAPI schemas. DB: 2 new tables (`quran_divisions`), 1 materialized view (`mv_student_surah_status`), 2 views, 7 indexes. 10 decisions locked (OQ-027 to OQ-034, GRADE-ENUM, CROSS-CIRCLE). |
| 2026-08-09 | E-11 | Resolved stale Spec-Kit artifact gap | Codex | Verified Feature 001 has `spec.md`, `plan.md`, `tasks.md`, research, data model, quickstart, contract, checklist, and validation report. |
| 2026-08-09 | E-07, E-12 | Closed stale documentation gaps | Codex | Verified architecture rate-limit policy and substantive system-design, API-docs, and guides content. |
| 2026-08-16 | E-06, E-14, E-15, E-23 | Reclassified solved findings as Done | Copilot | Revalidated current docs tree; quran data source, Arabic README linking, testing strategy linking, and RACI cleanup are now treated as resolved in the tracker. |
| 2026-08-16 | E-08, E-09, E-16, E-19, E-21, E-22, E-24 | Completed In Progress items by integrating content into existing docs | Copilot | E-08 added offline/low-bandwidth strategy to guides/README.md; E-09 & E-16 capacity plan and deferral triggers to PROJECT_PLAN.md; E-19 & E-21 indexing strategy and updated_at audit to ARCHITECTURE.md; E-22 pagination policy to api-docs/README.md; E-24 observability design to DEPLOYMENT.md. Zero new .md files created; all enhancements integrated into suitable existing locations. Scorecard updated: 19 Done (47.5%), 0 In Progress (0%), 21 Pending (52.5%). |
