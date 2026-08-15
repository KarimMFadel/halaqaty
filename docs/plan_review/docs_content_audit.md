# 📋 Halaqaty `docs/` Content Audit — Panel Review & Action Plan

> **Original Auditors:** Product Manager + Senior Project Manager  
> **Panel Review:** Architect · Tech Lead · Team Leader  
> **Date:** 2026-04-29  
> **Status:** ✅ Panel review complete · Karim decisions captured · Pre-conditions executed  
> **Next Step:** Product Manager + Project Manager to execute remaining cleanup actions (see Action Plan below)

---

## ⚡ Pre-Conditions Already Executed (by Engineering Panel)

The following content migrations were completed **before** the audit cleanup begins, to prevent data loss:

| # | Action | File Created/Updated | Content Migrated From |
|---|--------|---------------------|----------------------|
| **PC-1a** | ✅ Done | `docs/management/product/FEATURES.md` F-003 | Grading scale table (6 grades + Arabic labels) added as `#### Grading Scale` section between Open Questions and Design Decisions (now moved before Acceptance Criteria per Tech Lead review) |
| **PC-1b** | ✅ Done | `docs/engineering/architecture/ARCHITECTURE.md` §4.0 | Recitation Grade domain enum table added before §4.1 ERD |
| **PC-2** | ✅ Done | `docs/management/product/ROLES.md` *(new file)* | Full role capability matrix (Teacher/Student/Supervisor/Institution Admin + access matrix) extracted from PLAN.md §2 |
| **PC-3** | ✅ Done | `docs/management/product/PRD.md` §4 | Institutional platform vision (7 bullets) confirmed present in PRD.md §4 (lines 65–79); source content was PLAN.md §1.5 |
| **PC-4/5** | ⚠️ **Pending PM/PjM** | `docs/management/product/PRD.md` §6 | Feature flag sign-off rule + recording privacy statement must be migrated from EXECUTION_PLAYBOOK.md §2/§3 before those sections are removed |
| **Q4** | ✅ Done | `docs/management/planning/PLAN.md` §6 | Retitled "Release Strategy" → "Release Channel Strategy" to clarify uniqueness |

---

## Audit Methodology

Every `.md` file was read in full. Each finding is categorized as:

| Severity | Meaning |
|----------|---------|
| 🔴 **CRITICAL** | Content exists in 3+ places or a file is fundamentally misscoped |
| 🟠 **IMPORTANT** | Content duplicated in 2 places or clearly belongs elsewhere |
| 🟡 **MINOR** | Small redundancy, stale reference, or cosmetic scope issue |

---

## Finding Summary

| # | Severity | Category | File(s) Involved | Issue |
|---|----------|----------|-------------------|-------|
| D-01 | 🔴 | Duplication | PLAN.md, FEATURES.md | Feature specs F-001 through F-017 are fully written in **both** files |
| D-02 | 🔴 | Duplication | PLAN.md, PRD.md | Problem statement, vision, target users repeated across 2 files — ⚠️ FEATURES.md incorrectly listed in original audit (see Error 1) |
| D-03 | 🔴 | Duplication | FEATURES.md, MVP_DECISION_REGISTER.md | Open Questions table in FEATURES.md duplicates the entire Decision Register |
| D-04 | 🟠 | Misplacement | PLAN.md §4, §5, §6, §7 | Technical architecture, deployment, release, and business model sections belong in ARCHITECTURE.md, DEPLOYMENT.md, PRD.md respectively |
| D-05 | 🟠 | Duplication | PLAN.md §5, DEPLOYMENT.md | Deployment phase table repeated nearly verbatim |
| D-06 | 🟠 | Duplication | PLAN.md §4.2, ARCHITECTURE.md §1 | System overview diagram repeated in both files |
| D-07 | 🟠 | Duplication | FEATURES.md §F-005, ARCHITECTURE.md §3 | LiveKit integration flow, audio config table duplicated nearly verbatim |
| D-08 | 🟠 | Duplication | PLAN.md §F-005, ARCHITECTURE.md §3 | LiveKit integration flow, audio config also in PLAN.md |
| D-09 | 🟠 | Misplacement | EXECUTION_PLAYBOOK.md | MVP scope, GTM, KPIs, RACI, co-teacher, glossary — all belong in PRD.md or PLAN.md |
| D-10 | 🟠 | Duplication | EXECUTION_PLAYBOOK.md §2, PRD.md §6 | MVP In/Out scope list nearly identical |
| D-11 | 🟠 | Stale/Misplaced | AGENT_SETUP_REFINEMENT_SUMMARY.md | One-time change log — not a living reference doc; belongs in git history or changelog |
| D-12 | 🟠 | Stale/Misplaced | DOCUMENTATION_UPDATE_SUMMARY.md | One-time update log — not a living reference; belongs in git history |
| D-13 | 🟡 | Duplication | PRD.md §4, PLAN.md §2 | Target users and JTBD repeated |
| D-14 | 🟡 | Duplication | PRD.md §11, PLAN.md §F-003 grading | Grading scale partially duplicated |
| D-15 | 🟡 | Misplacement | SYNC_GUIDE.md | References old paths (`docs/arabic/`) instead of current `docs/management/arabic/` |
| D-16 | 🟡 | Scope creep | PLAN.md §7 | Business model pricing table belongs in PRD.md (where it also exists) |
| D-17 | 🟡 | Duplication | EXECUTION_PLAYBOOK.md §5, PRD.md §10 | GTM phases A/B/C repeated verbatim |
| D-18 | 🟡 | Scope creep | PLAN.md §6 | Release strategy table duplicates/overlaps DEPLOYMENT.md Phase 1-4 timelines |

---

## Detailed Findings & Recommendations

---

### D-01 🔴 CRITICAL — Feature Specs Duplicated Across PLAN.md and FEATURES.md

**What's wrong:** PLAN.md Section 3 (lines 143–516, **374 lines**) contains the **complete** feature specifications for F-001 through F-017 — user stories, acceptance criteria, queue diagrams, grading scales, notification matrices, and all. These are **also** fully specified in FEATURES.md with even more detail (open questions, design decisions, dependencies, edge cases).

**Impact:** Any update to a feature must be made in two places. They will inevitably drift. FEATURES.md has already evolved beyond PLAN.md (it has DD-020 through DD-026, dependency maps, and resolved OQs that PLAN.md lacks).

> [!CAUTION]
> This is the single largest source of duplication in your docs. PLAN.md's Section 3 is 374 lines that are a stale subset of FEATURES.md.

**Recommendation:**
- **Remove** PLAN.md Section 3 entirely (lines 143–516)
- **Replace** with a short reference section:

```markdown
## 3. Feature Specifications

All feature specifications, acceptance criteria, open questions, and design decisions 
are maintained in [FEATURES.md](../management/product/FEATURES.md).

**MVP (P0) Features:** F-001 through F-006 (Auth, Circles, Queue, Chat, Sessions, Schedule)  
**See:** [Feature Status Table](../management/product/FEATURES.md#feature-status-table)
```

---

### D-02 🔴 CRITICAL — Vision/Problem/Users Repeated in 3 Files

**What's wrong:** The following content appears in nearly identical form across three files:

| Content Block | PLAN.md | PRD.md | FEATURES.md |
|---------------|---------|--------|-------------|
| Vision statement | §1.1 (3 lines) | §1 (3 lines) | — |
| Problem statement + pain point table | §1.2 (12 lines + table) | §2 (6 lines) | — |
| Solution overview | §1.3 (8 lines) | §5 (6 lines) | — |
| Target users + roles | §2 (60 lines, full role details) | §4 (15 lines, JTBD) | — |
| Target market | §1.4 (6 lines) | — | — |
| Institutional future | §1.5 (12 lines) | — | F-017 (20 lines) |

**Impact:** Three files compete to be the "source of truth" for business context. When the vision or user roles change, you must update all three.

**Recommendation:**
- **PRD.md** is the canonical home for Vision, Problem, Users, and Value Prop — it already has the right business-first framing.
- **Remove** PLAN.md Sections 1 and 2 entirely (lines 22–141, ~120 lines).
- **Replace** with a short reference:

```markdown
## 1. Project Context

See [PRD.md](../management/product/PRD.md) for:
- Vision statement (§1)
- Business problem (§2)
- Target users and JTBD (§4)
- Value proposition (§5)

See [FEATURES.md](../management/product/FEATURES.md) for detailed role capabilities and permissions.
```

---

### D-03 🔴 CRITICAL — Open Questions Table Duplicated Between FEATURES.md and MVP_DECISION_REGISTER.md

**What's wrong:** FEATURES.md lines 595–627 contain an "Open Questions Log" table with 26 entries (OQ-001 through OQ-026), all marked "Decided" with resolutions. MVP_DECISION_REGISTER.md contains **the same 26 decisions** organized by domain (Auth, Circle, Queue, etc.) with identical resolutions plus rationale.

**Impact:** Two files claim to be the decision record. Any new decision must be added to both. The MVP Decision Register has rationale that FEATURES.md lacks, making it the richer source.

**Recommendation:**
- **Remove** the "Open Questions Log" table from FEATURES.md (lines 595–627).
- **Replace** with a short reference:

```markdown
## Open Questions Log

All open questions have been resolved and frozen in the 
[MVP Decision Register](../management/product/MVP_DECISION_REGISTER.md).
```

- Keep the individual `#### Open Questions` sections within each feature (e.g., OQ-007 under F-003) as contextual references, but remove their `Status` and `Decision` columns — just link to the Decision Register for the resolution.

---

### D-04 🟠 IMPORTANT — PLAN.md Contains Content That Belongs in Other Files

**What's wrong:** PLAN.md has become a "mega-document" that repeats content from 4+ other files:

| PLAN.md Section | Belongs In | Why |
|-----------------|------------|-----|
| §4 Technical Architecture Overview (lines 518–570) | ARCHITECTURE.md | System diagram and protocol table are a subset of ARCHITECTURE.md §1-2 |
| §5 Deployment Strategy (lines 574–586) | DEPLOYMENT.md | Phase table is a subset of DEPLOYMENT.md's four-phase plan |
| §6 Release Strategy (lines 589–600) | PRD.md §12 or DEPLOYMENT.md §10 | Release channels are deployment/GTM concerns |
| §7 Business Model (lines 603–614) | PRD.md §9 | Pricing tiers already exist in PRD.md §9 |

**Recommendation:**
- **Remove** PLAN.md Sections 4, 5, 6, and 7 entirely.
- **Replace** each with a one-line cross-reference:

```markdown
## 4. Technical Architecture
See [ARCHITECTURE.md](../engineering/architecture/ARCHITECTURE.md)

## 5. Deployment Strategy
See [DEPLOYMENT.md](../engineering/deployment/DEPLOYMENT.md)

## 6. Release Strategy
See [PRD.md](../management/product/PRD.md#12-milestones) and [DEPLOYMENT.md](../engineering/deployment/DEPLOYMENT.md#10-cicd-pipeline)

## 7. Business Model
See [PRD.md](../management/product/PRD.md#9-pricing-and-business-model-future)
```

**After this cleanup, PLAN.md's remaining content is:**
1. Cross-references to PRD, FEATURES, ARCHITECTURE, DEPLOYMENT
2. **Section 8: Timeline — 12-Month Plan** (the actual planning content)
3. Timeline realism notes

This makes PLAN.md a lean **timeline and execution schedule** document — which is exactly what a project plan should be.

---

### D-05 🟠 IMPORTANT — Deployment Phase Table in PLAN.md and DEPLOYMENT.md

**What's wrong:** PLAN.md §5 (lines 578-586) contains a 4-row deployment phase summary table that is a direct subset of DEPLOYMENT.md's detailed four sections (each with architecture diagrams, capacity estimates, and risk tables).

**Recommendation:** Already covered by D-04. Remove PLAN.md §5 and link to DEPLOYMENT.md.

---

### D-06 🟠 IMPORTANT — System Overview Diagram in PLAN.md and ARCHITECTURE.md

**What's wrong:** PLAN.md §4.2 (lines 533-570) has an ASCII system diagram showing Flutter → Go Backend → PostgreSQL/LiveKit/MinIO/Firebase. ARCHITECTURE.md §1 (lines 24-87) has a **more detailed version** of the exact same diagram with Data & Services layer.

**Recommendation:** Already covered by D-04. The PLAN.md version is a stale subset — remove it.

---

### D-07 🟠 IMPORTANT — LiveKit Config Duplicated in FEATURES.md and ARCHITECTURE.md

**What's wrong:** 
- FEATURES.md F-005 (lines 283-295) contains a LiveKit audio config YAML block and integration flow diagram.
- ARCHITECTURE.md §3 contains the same config with additional context.
- PLAN.md F-005 (lines 349-358) has the same audio config comparison table.

**Impact:** LiveKit audio settings are specified in **three places**. If you change the bitrate target, you must update three files.

**Recommendation:**
- **ARCHITECTURE.md** is the canonical home for all technical configuration.
- In FEATURES.md F-005, **remove** the audio config YAML and integration flow. Keep the acceptance criteria and user stories. Add a reference:

```markdown
> **Audio Configuration:** See [ARCHITECTURE.md §3](../engineering/architecture/ARCHITECTURE.md#3-livekit--flutter-integration) for LiveKit audio settings and integration flow.
```

- In PLAN.md, this section is already being removed per D-01.

---

### D-09 🟠 IMPORTANT — EXECUTION_PLAYBOOK.md Scope Creep

**What's wrong:** EXECUTION_PLAYBOOK.md (196 lines) contains:

| Section | Already Exists In |
|---------|-------------------|
| §2 MVP Cut (in/out scope) | PRD.md §6, FEATURES.md status table |
| §5 GTM phases A/B/C | PRD.md §10 |
| §6 KPI System | PRD.md §8 |
| §7 RACI table | Unique — but belongs in PLAN.md or a standalone governance doc |
| §8 Co-teacher clarification | MVP_DECISION_REGISTER.md (OQ-004/PRD-4) |
| §9 Abbreviation glossary | Unique — useful but could be in docs/README.md |

**Recommendation:**
- **Slim down** EXECUTION_PLAYBOOK.md to contain **only execution-specific content** not found elsewhere:
  - §0 Agent-Driven Development ✅ keep (unique)
  - §3 Decision Sprint ✅ keep (unique process)
  - §4 Weekly Operating Cadence ✅ keep (unique process)
  - §7 RACI ✅ keep (unique)
  - §10 Update Policy ✅ keep (unique)
- **Remove** §2 (MVP Cut) — link to PRD.md §6
- **Remove** §5 (GTM) — link to PRD.md §10
- **Remove** §6 (KPIs) — link to PRD.md §8
- **Remove** §8 (Co-teacher) — link to MVP_DECISION_REGISTER.md
- **Move** §9 (Glossary) to `docs/README.md` where all newcomers land

---

### D-11 🟠 IMPORTANT — AGENT_SETUP_REFINEMENT_SUMMARY.md is a Stale Changelog

**What's wrong:** This 294-line file documents the one-time changes made when the agent system was configured (Point 1: file location, Point 2: missing phases, Point 3: clarification protocol). It is marked "✅ Complete" and contains no living content. It's a snapshot of a past conversation.

**Impact:** It clutters the `collaboration/` directory with historical noise. The actual reference is `AGENT_COLLABORATION_GUIDE.md`, which is the living document.

**Recommendation:**
- **Delete** `AGENT_SETUP_REFINEMENT_SUMMARY.md`
- The changes it documents are now permanently captured in the files it modified (constitution.md, agent files, collaboration guide). Git history preserves the change record.

---

### D-12 🟠 IMPORTANT — DOCUMENTATION_UPDATE_SUMMARY.md is a Stale Changelog

**What's wrong:** Same issue as D-11. This 273-line file lists every documentation change made when the agent system was integrated. It's a one-time update log marked "✅ All documentation updates complete."

**Impact:** Redundant with git history. The cross-reference map it contains is already built into `docs/README.md`.

**Recommendation:**
- **Delete** `DOCUMENTATION_UPDATE_SUMMARY.md`
- If you want to preserve the cross-reference map diagram, move it into `docs/README.md` (where a Navigation Map already exists).

---

### D-15 🟡 MINOR — SYNC_GUIDE.md Has Stale Paths

**What's wrong:** SYNC_GUIDE.md references `docs/arabic/` as the Arabic documentation location, but the actual path is `docs/management/arabic/`. The document map table links to `arabic/README_AR.md` and similar relative paths that don't resolve correctly from the file's current location.

**Recommendation:**
- **Update** all paths in SYNC_GUIDE.md to use correct relative paths from `docs/management/arabic/SYNC_GUIDE.md`.

---

### D-16 🟡 MINOR — Business Model in Both PLAN.md and PRD.md

**What's wrong:** PLAN.md §7 (lines 603-614) has a 3-row pricing tier table. PRD.md §9 (lines 135-149) has the same table with additional paywall activation policy.

**Recommendation:** Already covered by D-04. Remove from PLAN.md.

---

## Action Plan — Ordered by Impact

> [!IMPORTANT]
> The following actions are ordered by impact. Completing items 1-4 removes ~600 lines of duplicated content and eliminates the three worst drift risks.

### Phase 1: Critical Deduplication (Remove ~600 lines)

| # | Action | File | Lines Removed |
|---|--------|------|---------------|
| 1 | Replace PLAN.md §3 (Features) with cross-reference to FEATURES.md | PLAN.md | ~374 lines |
| 2 | Replace PLAN.md §1-2 (Vision/Users) with cross-reference to PRD.md | PLAN.md | ~120 lines |
| 3 | Remove PLAN.md §4.2 system diagram and §7 business model — **§4.1 protocol table and §6 Release Channel Strategy are KEPT** (panel decision: §4.1 is unique planning summary, §6 is unique app-store channel content — see Error 2 and Q4) | PLAN.md | ~50 lines |
| 4 | Remove FEATURES.md Open Questions Log, link to MVP_DECISION_REGISTER.md | FEATURES.md | ~33 lines |

**Result:** PLAN.md shrinks from **709 lines → ~120 lines** (timeline + cross-references). FEATURES.md loses 33 lines of duplicate decisions.

### Phase 2: Scope Cleanup (Remove ~100 lines of misplaced content)

| # | Action | File | Lines Removed |
|---|--------|------|---------------|
| 5 | Remove EXECUTION_PLAYBOOK.md §2, §5, §6, §8 (duplicates PRD.md) | EXECUTION_PLAYBOOK.md | ~60 lines |
| 6 | Remove LiveKit config YAML from FEATURES.md F-005 (lives in ARCHITECTURE.md) | FEATURES.md | ~30 lines |

### Phase 3: Housekeeping (Delete stale files, fix paths)

| # | Action | File |
|---|--------|------|
| 7 | **Delete** AGENT_SETUP_REFINEMENT_SUMMARY.md (stale one-time changelog) | collaboration/ |
| 8 | **Delete** DOCUMENTATION_UPDATE_SUMMARY.md (stale one-time changelog) | collaboration/ |
| 9 | Fix SYNC_GUIDE.md relative paths to match current directory structure | arabic/ |
| 10 | Move Abbreviation Glossary from EXECUTION_PLAYBOOK.md to docs/README.md | Both files |

---

## Post-Cleanup File Responsibilities

After all actions are completed, each file has a **single, clear responsibility**:

| File | Single Responsibility | Lines (est.) |
|------|----------------------|--------------|
| **PRD.md** | Business context: vision, problem, users, scope, pricing, milestones, risks, GTM | ~213 (unchanged) |
| **FEATURES.md** | Feature specs: acceptance criteria, design decisions, dependencies, status | ~600 (from 660) |
| **PLAN.md** | **Timeline only:** 12-month schedule + cross-references to other docs | ~120 (from 709) |
| **MVP_DECISION_REGISTER.md** | **Single source** for all frozen decisions with rationale | ~119 (unchanged) |
| **JOURNEY.md** | User journey flows (screen-by-screen MVP walkthrough) | ~412 (unchanged) |
| **ARCHITECTURE.md** | Technical architecture, schema, LiveKit config, security | ~900 (unchanged) |
| **DEPLOYMENT.md** | Infrastructure phases, Docker, monitoring, backups, CI/CD | ~596 (unchanged) |
| **TESTING_STRATEGY.md** | Test pyramid, coverage targets, CI gates | ~219 (unchanged) |
| **EXECUTION_PLAYBOOK.md** | Execution process: agent workflow, decision sprint, cadence, RACI | ~100 (from 196) |
| **AGENT_COLLABORATION_GUIDE.md** | Agent roles, responsibilities, Spec-Kit phases | ~408 (unchanged) |
| **COMPETITOR_ANALYSIS.md** | Market research (read-only reference) | ~158 (unchanged) |
| **SYNC_GUIDE.md** | Arabic documentation sync policy | ~70 (unchanged) |

---

---

## ⚠️ Audit Errors Found by Engineering Panel

> **IMPORTANT for PM/PjM:** The following items in the original audit contained factual errors. Do NOT execute the original recommendation for these items — use the corrected guidance below.

### Error 1 — D-02 overcounts affected files

**Original claim:** "Vision/problem repeated in 3 files including FEATURES.md"  
**Correction:** FEATURES.md does NOT contain vision or problem statement content. The duplication is only PLAN.md vs PRD.md. Corrected recommendation: only two files need consolidation.

### Error 2 — D-04 §6 recommendation is wrong

**Original claim:** "PLAN.md §6 (Release Strategy) duplicates DEPLOYMENT.md §10"  
**Correction:** DEPLOYMENT.md §10 is the **CI/CD Pipeline** — it has nothing to do with release channels. PLAN.md §6 covers App Store channels (Internal Alpha → Google Play → TestFlight → Web → Institutional) which is **unique content not found in DEPLOYMENT.md**.  
**Karim's decision:** Keep PLAN.md §6 — retitled to **"Release Channel Strategy"** ✅ Already done.

### Error 3 — D-07 LiveKit YAML is technically incorrect

**Original claim:** "FEATURES.md has duplicate LiveKit YAML — remove it"  
**Correction:** The LiveKit YAML in FEATURES.md F-005 is technically incorrect:
- `auto_gain_control` is a Flutter platform-level audio constraint, NOT a LiveKit room option — it cannot appear in `RoomOptions`
- Bitrate format is inconsistent with the SDK (`audioBitrate: 30000` vs SDK's kbps integer)

**Updated recommendation:** Remove the YAML block from FEATURES.md F-005 (D-07 action still stands), but ALSO remove or correct the YAML in ARCHITECTURE.md §3. ARCHITECTURE.md should have the correct SDK configuration, not copy-pasted incorrect code.

---

## 🗺️ Three-Agent Panel Verdict Table

Panel: **Architect** (A) · **Tech Lead** (TL) · **Team Leader** (TM)  
Legend: ✅ Accept · 🔄 Partial · ❌ Reject · ✋ Precondition Required

| Finding | A | TL | TM | Consensus | Notes |
|---------|---|----|----|-----------|-------|
| D-01 Feature specs duplication | ✅ | ✅ | ✅ | **ACCEPT** | ✋ Grading scale must migrate first (PC-1 ✅ done) |
| D-02 Vision/problem in 3 files | 🔄 | ✅ | ✅ | **PARTIAL** | Only 2 files affected (see Error 1). §1.5 institutional vision is UNIQUE — must migrate to PRD.md §4 first (PC-3 pending) |
| D-03 OQ table in FEATURES.md | 🔄 | 🔄 | ✅ | **PARTIAL** | Remove ONLY the consolidated table (lines 595–627). Keep per-feature inline OQ subsections. OQ stub format deferred to PM/PjM (Q5) |
| D-04 PLAN.md §4/5/6/7 cleanup | 🔄 | 🔄 | ✅ | **PARTIAL** | §6 is KEEP (unique, retitled ✅). §4.1 protocol table is KEEP (useful planning summary). §5 is KEEP reference. §4.2 system diagram and §7 business model: remove |
| D-05 Deployment table dup | ✅ | ✅ | ✅ | **ACCEPT** | Covered by D-04 |
| D-06 System diagram dup | ✅ | ✅ | ✅ | **ACCEPT** | ARCHITECTURE.md has richer version |
| D-07 LiveKit YAML dup | ✅ | ✅ | ✅ | **ACCEPT** | Technically incorrect YAML — remove from FEATURES.md F-005; verify/fix in ARCHITECTURE.md §3 |
| D-08 LiveKit in PLAN.md | ✅ | ✅ | ✅ | **ACCEPT** | Covered by D-01 |
| D-09 EXECUTION_PLAYBOOK scope | 🔄 | 🔄 | 🔄 | **PARTIAL** | §9 Glossary: KEEP in playbook (do NOT move to docs/README.md). §2/§5/§6/§8 removals require PC-4/5 first (pending) |
| D-10 MVP scope duplication | ✅ | ✅ | ✅ | **ACCEPT** | After PC-4 migration |
| D-11 AGENT_SETUP_REFINEMENT_SUMMARY | ✅ | ✅ | ✅ | **ACCEPT** | ✋ Check constitution.md first (Q7 — see below) |
| D-12 DOCUMENTATION_UPDATE_SUMMARY | ✅ | ✅ | ✅ | **ACCEPT** | ✋ Check constitution.md first; also all paths are wrong (pre-dates current dir structure) |
| D-13 Target users in PLAN.md | ✅ | ✅ | ✅ | **ACCEPT** | Covered by D-02. ROLES.md now handles capability detail (PC-2 ✅ done) |
| D-14 Grading scale partial dup | ✅ | ✅ | ✅ | **ACCEPT** | PC-1 ✅ done — grading table now in FEATURES.md F-003 and ARCHITECTURE.md §4.0 |
| D-15 SYNC_GUIDE.md stale paths | ✅ | 🔄 | ✅ | **PARTIAL** | 3-part fix needed: (1) prose path refs, (2) relative link targets in doc map, (3) broken inbound links from FEATURES.md line 659 and PLAN.md line 708 |
| D-16 Business model dup | ✅ | ✅ | ✅ | **ACCEPT** | Remove PLAN.md §7 |
| D-17 GTM phases dup | ✅ | ✅ | ✅ | **ACCEPT** | After PC-5 migration |
| D-18 Release strategy scope | 🔄 | ❌ | 🔄 | **PARTIAL** | §6 now retitled "Release Channel Strategy" and KEPT (Tech Lead: unique content). Remove cross-ref confusion only. ✅ Done |

---

## 💬 Karim's Decisions (Q1–Q7)

| Q | Question | Decision |
|---|---------|---------|
| **Q1** | Where does the grading scale go? | **Both A and B** — FEATURES.md F-003 AND ARCHITECTURE.md §4 ✅ Done |
| **Q2** | Where does the role capability matrix go? | **Option C — New `ROLES.md`** reference file ✅ Done (`docs/management/product/ROLES.md`) |
| **Q3** | Should PLAN.md be renamed? | **Deferred to PM/PjM** — see open items below |
| **Q4** | What happens to PLAN.md §6 Release Strategy? | **Option A — Keep, retitle to "Release Channel Strategy"** ✅ Done |
| **Q5** | How to handle inline OQ sections after the consolidated table is removed? | **Deferred to PM/PjM** — see open items below |
| **Q6** | EXECUTION_PLAYBOOK.md §8 Co-teacher section? | **Option B — Condense to privacy statement + reference link** to MVP_DECISION_REGISTER.md PRD-4 |
| **Q7** | constitution.md check before D-11/D-12 deletion? | **Include in PM/PjM summary** — see pre-delete check below |

---

## 📋 PM/PjM Action Plan (Ordered Execution)

### Phase 0 — Remaining Pre-Conditions (must do BEFORE any deletions)

| # | Action | Owner | File | Blocks |
|---|--------|-------|------|--------|
| **PC-3** | Migrate PLAN.md §1.5 institutional platform vision (lines 63–76, 7 bullets) to PRD.md §4. This is **unique content** not in PRD.md. | PM | `PRD.md §4` ← `PLAN.md §1.5` | D-02 |
| **PC-4** | Migrate feature flag sign-off rule from EXECUTION_PLAYBOOK.md §3 ("live_session_video and session_recording must remain OFF in MVP; require PM + architect sign-off") to PRD.md §6. | PM | `PRD.md §6` ← `EXECUTION_PLAYBOOK.md §3` | D-09/D-10 |
| **PC-5** | Migrate recording privacy statement from EXECUTION_PLAYBOOK.md §2 to PRD.md §6 or §11. | PM | `PRD.md §6 or §11` ← `EXECUTION_PLAYBOOK.md §2` | D-09 |

### Phase 1 — Critical Deduplication (✅ Executed)

| # | Action | File | Status |
|---|--------|------|--------|
| 1 | Replace PLAN.md §3 (Features) with one cross-reference paragraph pointing to FEATURES.md | PLAN.md | ✅ Done |
| 2 | Replace PLAN.md §1 and §2 (Vision/Users) with cross-reference to PRD.md and ROLES.md | PLAN.md | ✅ Done |
| 3a | Remove PLAN.md §4.2 system diagram (stale subset of ARCHITECTURE.md §4.0). Keep §4.1 protocol table. Keep §6 Release Channel Strategy. | PLAN.md | ✅ Done |
| 3b | Remove PLAN.md §7 business model table — replaced with reference to PRD.md §9 | PLAN.md | ✅ Done |
| 4 | Remove FEATURES.md consolidated OQ Log table (lines 595–627). | FEATURES.md | ✅ Done |

### Phase 2 — Scope Cleanup (✅ Executed)

| # | Action | File | Status |
|---|--------|------|--------|
| 5 | Remove EXECUTION_PLAYBOOK.md §2 (MVP Cut) — keep privacy consent statement, link to PRD.md §6 | EXECUTION_PLAYBOOK.md | ✅ Done |
| 6 | Remove EXECUTION_PLAYBOOK.md §5 (GTM) — link to PRD.md §10 | EXECUTION_PLAYBOOK.md | ✅ Done |
| 7 | Remove EXECUTION_PLAYBOOK.md §6 (KPIs) — keep KPI ownership sentence, link to PRD.md §8 | EXECUTION_PLAYBOOK.md | ✅ Done |
| 8 | Condense EXECUTION_PLAYBOOK.md §8 (Co-teacher) to privacy statement + reference | EXECUTION_PLAYBOOK.md | ✅ Done |
| 9 | Remove FEATURES.md F-005 LiveKit YAML block (technically incorrect; ARCHITECTURE.md §3 is canonical) | FEATURES.md | ✅ Done |
| 10 | Verify/fix ARCHITECTURE.md §3 LiveKit YAML (removed `auto_gain_control`) | ARCHITECTURE.md §3 | ✅ Done |

### Phase 3 — Housekeeping (✅ Executed)

| # | Action | File | Status |
|---|--------|------|--------|
| 11 | Fix SYNC_GUIDE.md — 3 changes: prose refs, relative link targets, update inbound links | SYNC_GUIDE.md + FEATURES.md + PLAN.md | ✅ Done |
| 12 | **Pre-delete check ⚠️:** Search `docs/management/arabic/constitution.md` and `.specify/memory/constitution.md` for references | constitution.md | ✅ Done |
| 13 | Delete `docs/engineering/collaboration/AGENT_SETUP_REFINEMENT_SUMMARY.md` | — | ✅ Done |
| 14 | Delete `docs/engineering/collaboration/DOCUMENTATION_UPDATE_SUMMARY.md` | — | ✅ Done |

---

## ❓ Open Items Deferred to PM/PjM

### Q3 — PLAN.md Naming
After cleanup, PLAN.md will be ~120 lines (timeline + cross-references). Should it be:
- A: Keep as `PLAN.md`
- B: Rename to `TIMELINE.md`
- C: Rename to `ROADMAP.md`

The content will be the 12-month execution schedule — a timeline, not a full plan. Recommend deciding after Phase 1 cleanup is complete.

### Q5 — OQ Inline Sections After Consolidated Table Removed
Each feature (e.g., F-003) has an `#### Open Questions` subsection with OQ entries. After removing the consolidated table at lines 595–627:
- **Option A:** Keep inline OQ sections as-is (they provide useful per-feature context even if decisions are frozen in MVP_DECISION_REGISTER.md)
- **Option B:** Strip inline OQ sections down to ID + Status + "See [MVP Decision Register](../management/product/MVP_DECISION_REGISTER.md)" stub

Recommend Option A (minimal disruption) unless the goal is a fully slim FEATURES.md.

---

## 📌 Additional Panel Findings (Not in Original Audit)

These items were discovered during the panel review and must be addressed as part of the cleanup:

| # | Finding | Severity | Action Required |
|---|---------|---------|----------------|
| **NF-1** | EXECUTION_PLAYBOOK.md §3 contains a feature flag enforcement rule ("live_session_video and session_recording must remain OFF; require PM + architect sign-off") that exists NOWHERE else in the docs. | 🔴 CRITICAL | Migrate to PRD.md §6 before §3 cleanup (PC-4) |
| **NF-2** | FEATURES.md line 659 used the obsolete relative target `SYNC_GUIDE.md`, which resolved from the wrong directory | 🟠 IMPORTANT | Fix as part of D-15 (step 11) |
| **NF-3** | PLAN.md had a broken inbound link to SYNC_GUIDE.md (original line 708 in 709-line file; that section has since been removed). Current PLAN.md footer uses `../arabic/SYNC_GUIDE.md` which resolves correctly from `docs/management/planning/`. | 🟠 IMPORTANT | ✅ Resolved — link verified correct at current line ~175 |
| **NF-4** | PLAN.md §2 role matrix contains Supervisor authorization constraints ("Cannot grade students," "Cannot remove the teacher") that are **not present in PRD.md** — unique content | 🔴 CRITICAL | Migrated to ROLES.md (PC-2 ✅ done). Do NOT delete PLAN.md §2 before ROLES.md was verified |
| **NF-5** | F-005 (FEATURES.md lines 283–295): LiveKit YAML is technically incorrect — `auto_gain_control` is a Flutter platform audio constraint, not a LiveKit `RoomOptions` field | 🟠 IMPORTANT | Remove YAML from FEATURES.md. Fix in ARCHITECTURE.md §3 |

---

*Panel review conducted by Architect, Tech Lead, and Team Leader agents. Pre-condition actions PC-1, PC-2, and Q4 have been executed. Remaining actions handed off to Product Manager and Project Manager.*
