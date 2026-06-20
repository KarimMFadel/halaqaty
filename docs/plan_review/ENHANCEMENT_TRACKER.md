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
| E-06 | Docs | Quran data source not documented — `ARCHITECTURE.md` defines a `quran_surahs` table but never explains where canonical Surah/Ayah data originates (static JSON seed, third-party API, bundled asset, etc.) | Implementing agents will make incompatible assumptions; data consistency across environments is undefined | ⬜ Pending | — | — |
| E-07 | Docs | Rate limiting not documented in `ARCHITECTURE.md §6` Security section — the constitution §IV.8 mandates rate limits (per-IP, per-user, WebSocket 30 msg/min) but these are absent from the architecture document | Implementing agents may omit rate limiting middleware entirely; referenced in constitution but not actionable in code contracts | ⬜ Pending | — | — |
| E-08 | Product | No offline / low-bandwidth strategy documented — the primary user base (Middle East, Africa, South Asia) frequently operates on unstable mobile connections | No graceful degradation means a bad connection silently breaks sessions; no guidance for Flutter caching, retry logic, or WebSocket reconnection backoff | ⬜ Pending | — | — |
| E-09 | Planning | Capacity plan missing — the plan review scored "Roles & Responsibilities" B+ because this gap remained; a solo developer shipping 8 features across 12 months needs explicit monthly hour-budget or per-sprint velocity targets | Without a capacity plan, the 15% buffer warning in `PROJECT_PLAN.md §8` has no actionable trigger; risk of silent timeline slippage | ⬜ Pending | — | — |
| E-10 | Docs | `PRD.md` Owner field lists `"GPT-5.3-Codex acting as Project Manager"` — a model name is not an appropriate owner for a living product document | Confuses contributors about accountability; model versions change; a person or role name (e.g. "Karim Fadel — Product Owner") is required | ⬜ Pending | — | — |
| E-17 | Planning | Add Sprint 3-6 skeletons to `PROJECT_PLAN.md` | Even one-liner sprint goals would help with execution tracking | ⬜ Pending | — | — |
| E-18 | Docs | Add error code registry | Centralized error codes prevent fragmented API responses | ⬜ Pending | — | — |
| E-19 | Architecture | Add database indexing strategy | Prevents slow queries at scale, guides developers | ⬜ Pending | — | — |
| E-20 | Architecture | Create a data migration runbook | Needed for safe schema changes in production | ⬜ Pending | — | — |
| E-21 | Architecture | Add `updated_at` column to tables that lack it | Needed for cache invalidation and conflict detection | ⬜ Pending | — | — |
| E-22 | Architecture | Add pagination strategy | Affects every list endpoint | ⬜ Pending | — | — |
| E-23 | Docs | Clean up `EXECUTION_PLAYBOOK.md` RACI roles | Replaces phantom roles with reality for the solo phase | ⬜ Pending | — | — |
| E-24 | Architecture | Add observability design | Necessary for structured logging and request tracing | ⬜ Pending | — | — |

---

## Priority 3 — Nice to Have

*Housekeeping, polish, and long-term maintainability improvements.*

| ID | Category | Enhancement | Impact | Status | Resolved By | Resolved On |
|----|----------|-------------|--------|--------|-------------|-------------|
| E-11 | Spec-Kit | `specs/001-auth` is incomplete — only `spec.md` and a checklist exist; `plan.md` and `tasks.md` (Spec-Kit phases 4 and 5) have not been generated | Implementation agents cannot start Sprint 1 without the plan and tasks artifacts; the 7-phase pipeline is stalled at phase 3 | ⬜ Pending | — | — |
| E-12 | Docs | Three placeholder-only directories exist with stub READMEs and no content: `docs/engineering/system-design/`, `docs/engineering/api-docs/`, `docs/engineering/guides/` | Sparse documentation tree signals incomplete planning to contributors and agents; stubs create false impression of coverage | ⬜ Pending | — | — |
| E-13 | Docs | No `CHANGELOG.md` — no history of documentation or planning changes beyond git blame | Once implementation begins and releases are cut, the absence of a changelog makes it hard to communicate changes to pilot users or contributors | ⬜ Pending | — | — |
| E-14 | Docs | Arabic README (`docs/management/arabic/README_AR.md`) is not linked from the main `README.md` top section | Arabic-speaking contributors or teachers landing on the repo see no indication of Arabic documentation; discoverability is zero | ⬜ Pending | — | — |
| E-15 | Docs | `TESTING_STRATEGY.md` exists in `docs/engineering/development/` but is not referenced or linked from `DEVELOPMENT.md` | Implementing agents and contributors following the developer guide will miss the testing strategy entirely; test patterns will be inconsistent | ⬜ Pending | — | — |
| E-16 | Planning | Timeline optimism — the 300+ Monthly Active Teachers target at 12 months has no concrete deferral trigger; `PROJECT_PLAN.md` correctly notes 14–15 months is realistic but provides no decision criteria for when to officially defer a milestone | Without explicit deferral criteria, milestone slippage goes unacknowledged and quality gates get quietly relaxed under time pressure | ⬜ Pending | — | — |
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
| P2 — Before Beta | 13 | 0 | 0 | **13** |
| P3 — Nice to Have | 22 | 0 | 0 | **22** |
| **Total** | **40** | **5** | **0** | **35** |

---

## Amendment Log

*Append one row every time an enhancement is resolved, a new one is added, or the rating changes. Never delete rows.*

| Date | Enhancement(s) | Action | Model / Agent | Notes |
|------|----------------|--------|---------------|-------|
| 2026-06-20 | E-01 through E-16 | Initial review and tracker created | Claude Sonnet 4.6 (`claude-sonnet-4.6`) | Full docs + plan review. Overall rating: 8.5/10. Sixteen enhancements identified across 3 priority levels. |
| 2026-06-20 | E-01, E-02, E-03, E-04, E-05 | Resolved — Priority 1 complete | Claude Sonnet 4.6 (`claude-sonnet-4.6`) | Created `.github/workflows/lint.yml`, `.spectral.yaml`, `SECURITY.md`, `Makefile`. Fixed `PROJECT_PLAN.md` ToC (added §2, §7, §9). Replaced `EXECUTION_PLAYBOOK.md` §5 and §6 duplicated content with "Migrated" notices. |
| 2026-06-20 | E-17 through E-40 | Additional review points added | Antigravity AI (Gemini 3.1 Pro) | Conducted a comprehensive documentation review; added 24 new enhancements across P2 and P3. Overall rating bumped to 9.5/10. Noted usage of Antigravity AI chat. |
