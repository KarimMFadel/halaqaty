# Halaqaty Agent Workflow Harness

**Status:** Authoritative execution policy

**Applies to:** Codex, OpenCode, GitHub Copilot, Spec-Kit agents, and the Superpowers or project quality-guard skills available in the active harness

**Purpose:** Preserve Spec-Kit traceability while gaining Superpowers implementation discipline without duplicated artifacts, agents, reviews, or token-heavy context.

---

## 1. Responsibility Boundaries

Halaqaty uses one lifecycle with complementary tools:

- **Spec-Kit decides what is built.** It owns feature requirements, clarification, quality checklists, technical plans, tasks, cross-artifact analysis, and implementation scope.
- **Superpowers controls how approved work is executed.** It supplies test-first development, systematic debugging, bounded delegation, code-review procedure, worktree safety, and verification before completion.
- **Role-based agents supply domain judgment.** They participate only when their specialty is needed.
- **Ponytail controls implementation restraint.** It prefers the smallest correct diff, existing code, standard libraries, native platform features, and already-installed dependencies inside approved scope.
- **Project quality guards audit changed artifacts.** They do not create a second review organization.
- **Karim retains approval.** Mandatory manual review remains required for auth, RBAC, deletion paths, Firebase Auth, MinIO/upload, and other security-sensitive work.

Superpowers is installed in the coding harness, not vendored into this repository. This file configures how an installed harness must behave inside Halaqaty.

### Harness capability detection

- Before invoking a named skill or agent workflow, confirm that the active harness exposes it.
- Codex uses the installed Superpowers plugin. OpenCode and GitHub Copilot require their own Superpowers installation; do not assume a Codex installation is shared with them.
- Ponytail is installed per harness. OpenCode and GitHub Copilot use their own Ponytail configuration; Codex uses its Codex plugin. If a Ponytail command or skill is unavailable, apply the Ponytail rules manually and report the fallback.
- OpenCode role definitions live under `.opencode/agents/`; GitHub Copilot role definitions live under `.github/agents/`.
- In Codex, use a callable matching role agent when available. Otherwise read the matching `.github/agents/<role>.agent.md` and apply it as the role brief for inline work or a bounded subagent.
- OpenCode project guards live under `.opencode/skills/`; GitHub Copilot project guards live under `.github/skills/`.
- If a project guard is not registered as a callable skill, read its repository `SKILL.md` and apply its checklist manually. Report the fallback accurately instead of claiming the skill was invoked.
- If Superpowers is unavailable, preserve the equivalent discipline directly: test first, investigate failures before fixes, delegate only bounded independent work, use the Tech Lead review gate, and run fresh verification before completion.
- If multi-agent support is unavailable, execute inline with the appropriate role instructions. Never block compliant work merely because optional delegation tooling is missing.

## 2. Authority and Conflict Resolution

Use this order when deciding what governs work:

1. The Halaqaty constitution
2. Karim's explicit current instruction, when consistent with the constitution and its amendment process
3. Frozen product decisions, approved feature status, `DEVELOPMENT.md`, architecture, ADRs, and canonical API/WebSocket contracts
4. Current feature artifacts under `specs/NNN-feature/`: `spec.md`, `plan.md`, contracts, and `tasks.md`
5. Root `AGENTS.md`, this harness, and the collaboration guide
6. Superpowers process skills, role-agent defaults, and Ponytail defaults

Do not silently resolve a contradiction between levels 1–4. Stop, quote the conflicting paths and requirements concisely, and ask Karim which artifact must change. If a current instruction changes constitution-governed or frozen behavior, update the governing artifact through its required approval or amendment process before implementation. Lower-level workflow guidance must never rewrite higher-level scope.

Ponytail is a restraint layer, not an authority layer. If Ponytail suggests skipping work that Spec-Kit, the constitution, security requirements, accessibility, contracts, or tests require, build the required work. If Ponytail can reduce code without weakening those requirements, prefer the smaller path.

## 3. Spec-Kit Cycle With Agents and Skills

| Phase | Authoritative mechanism | Agents used | Superpowers and project skills |
|---|---|---|---|
| Pre-flight | Approved `FEATURES.md`, frozen decision register, journey, constitution | `team-leader` only unless a domain concern exists | No Superpowers planning artifact |
| `specify` | `/speckit.specify` | Team Leader coordinates; domain agents give bounded feasibility input | Do not run Superpowers `brainstorming` to create a parallel feature spec |
| `clarify` | `/speckit.clarify` | Only agents needed for unresolved questions | Batch related questions; do not ask every agent the same question |
| `checklist` | `/speckit.checklist` | Spec-Kit checklist agent; Tech Lead only for material security/quality gaps | No implementation review yet |
| `plan` | `/speckit.plan` | Architect leads; Go, Flutter, DB, DevOps, or SRE agents join only for affected areas | Do not run Superpowers `writing-plans`; `plan.md` is canonical |
| `tasks` | `/speckit.tasks` | Team Leader sequences ownership and dependencies | Do not generate a second task list |
| `analyze` | `/speckit.analyze` | Spec-Kit analyze agent; Architect/Tech Lead only for flagged risks | Resolve artifact conflicts before coding |
| `implement` | `/speckit.implement` and current `tasks.md` | Appropriate domain implementer; Team Leader coordinates cross-domain dependencies | Use TDD, systematic debugging when triggered, focused review, and verification |
| Finish | Spec-Kit git workflow, CI gates, PR | Tech Lead review, then Karim's required manual review | Use verification-before-completion; use finishing-a-development-branch only after gates pass |

### Explicit planning override

For an approved Spec-Kit feature, this file explicitly scopes Superpowers planning skills:

- `superpowers:brainstorming` may help refine an idea **before** `/speckit.specify`, but its output must feed Spec-Kit and must not become a competing feature specification.
- After a feature spec exists, use `/speckit.clarify` rather than Superpowers brainstorming for feature requirements.
- After `plan.md` exists, do not use `superpowers:writing-plans`; use `/speckit.tasks` and `tasks.md`.
- Superpowers may still require a tiny operational brief for agent dispatch. That brief contains task-local context only and is not a new project plan.

## 4. Agent Routing

Use the smallest sufficient team:

| Work | Primary agent | Consult or review only when needed |
|---|---|---|
| Phase coordination, task sequencing, blockers | `team-leader` | Relevant domain owner |
| Architecture, boundaries, ADR implications | `architect` | Tech Lead and affected implementers |
| Go backend, REST, WebSocket, LiveKit backend | `senior-golang-developer` | Database Optimizer for query/schema risk; Tech Lead for review |
| Flutter, Riverpod, RTL/Arabic UI | `senior-flutter-mobile-engineer` | Architect for contract/state-boundary changes; Tech Lead for review |
| PostgreSQL query/index optimization | `database-optimizer` | Go Developer and Architect |
| CI/CD and deployment automation | `devops-automator` | SRE for operational risk |
| Reliability, observability, incidents | `sre` | DevOps and affected domain owner |
| Code, security, and performance review | `tech-lead` | Karim for mandatory manual deep-review areas |

Rules:

- Collaboration means the responsible agents can be consulted; it does not require all agents to run in every phase.
- One task has one owner. Never dispatch two agents to edit the same files concurrently.
- The Team Leader coordinates but does not reimplement domain work.
- The Tech Lead is the reviewer used by the Superpowers review procedure; do not add a duplicate generic reviewer for the same diff.

## 5. Skill Composition Without Duplication

### Implementation path

1. `superpowers:test-driven-development` drives red-green-refactor for production behavior changes and bug fixes.
2. The domain agent implements the task against its Spec-Kit acceptance criteria, applying Ponytail to reuse existing code and avoid speculative abstraction.
3. `$test-guard` audits test quality after test files change; it does not repeat the TDD cycle.
4. `$clean-code-guard` audits the completed production-code batch.
5. `$docs-guard` runs only when OpenAPI, WebSocket events, or related contract documentation changes.
6. `superpowers:requesting-code-review` routes the prepared diff and acceptance criteria to `tech-lead` once per coherent batch.
7. Ponytail review is applied during review to flag avoidable code, unused abstraction, unnecessary dependencies, and missed standard-library/native-platform reuse.
8. `superpowers:verification-before-completion` validates current evidence before any completion claim.

### Ponytail boundaries

- Use Ponytail during `/speckit.implement`, review, and fix waves; do not use it to replace `/speckit.specify`, `/speckit.plan`, `/speckit.tasks`, or `/speckit.analyze`.
- In OpenCode implementation, Ponytail may use its native plugin commands and hooks, but the Halaqaty harness remains authoritative for scope and gates.
- In GitHub Copilot implementation or review, apply Ponytail through the installed Copilot plugin or manually from this harness when plugin commands are unavailable.
- In Codex review, use the normal review skill/procedure and also apply Ponytail review. Findings should still prioritize correctness, security, regressions, and missing tests before over-engineering concerns.
- Never simplify away trust-boundary validation, authorization checks, persistence safety, error handling that prevents data loss, accessibility basics, localization/RTL requirements, contract compatibility, or required tests.

### Conditional skills

- Use `superpowers:systematic-debugging` only after a bug, failed test, or unexpected behavior appears. Do not invoke it for routine implementation.
- Use `superpowers:dispatching-parallel-agents` only for at least two independent tasks with disjoint file ownership and no ordering dependency, and only outside a subagent-driven-development run.
- Use `superpowers:subagent-driven-development` only for a sufficiently large approved `tasks.md` whose tasks justify fresh agent context and review overhead. Its implementer/reviewer loop is sequential; never dispatch parallel implementers inside that workflow.
- Prefer inline execution for a small change, a tightly coupled task, or work within one package or feature slice.
- Use `superpowers:using-git-worktrees` only after detecting the current Git environment. Reuse the Spec-Kit feature branch or an existing managed worktree; never create a nested or duplicate worktree automatically.
- Use `superpowers:finishing-a-development-branch` only after all applicable Halaqaty gates pass. It must not bypass Karim's PR review.
- Use `$steno-mode` for compact status, task briefs, agent returns, and final reports when detail is already stored in files.

## 6. Token-Efficient Execution Rules

### A. Context loading

- The controller reads the constitution, `DEVELOPMENT.md`, current feature `spec.md`, `plan.md`, `tasks.md`, and only the canonical docs relevant to the active task.
- Read each stable artifact once per phase. Re-read only after the file changes or context compaction makes verification necessary.
- Agents receive paths and exact task IDs instead of pasted full documents.
- An implementer brief contains only: task IDs, goal, acceptance criteria, binding constraints, relevant paths, dependencies already produced, and verification commands.
- A reviewer receives: acceptance criteria, base/head or prepared diff path, implementation report, and relevant constraints. It does not receive the whole conversation.
- Store large diffs, logs, and reports in files or tool output; do not paste them repeatedly between agents.

### B. Task batching and delegation

- Default to inline work for one small coherent task.
- Batch approximately 2–5 closely related Spec-Kit tasks when they share setup, files, and one meaningful review boundary.
- Split a batch when tasks have different owners, independently rejectable outcomes, or disjoint verification.
- Default maximum per batch: one implementer and one Tech Lead reviewer.
- Parallelism optimizes elapsed time, not token usage. Use it only when tasks are truly independent and waiting would otherwise block progress.
- When parallel work is justified outside subagent-driven development, use the bounded parallel-agent workflow with at most two implementation agents and explicitly disjoint paths. Keep one concurrency slot available for coordination or review.
- Resume the original implementer for fixes while its context remains useful. Start a fresh agent only when the prior agent is stuck, context is polluted, or independent judgment is required.

### C. Model and reasoning economy

- Use the least expensive model capable of completing the task reliably.
- Small, exact, one- or two-file mechanical work may use a fast model.
- Multi-file integration, debugging, security, concurrency, architecture, and final review require stronger reasoning.
- Do not use a weak model when repeated turns are likely to cost more than one correct stronger-model pass.
- Do not run an agent solely to restate a result already established by deterministic tools.

### D. Questions and communication

- Ask only questions that materially change scope, architecture, contracts, security, or user-visible behavior.
- Batch questions that share one decision context, while keeping the final request concise.
- Do not make every agent ask its own version of the same question; the Team Leader consolidates cross-domain clarification.
- Status updates should report only new evidence, decisions, blockers, or completed boundaries.
- Agent returns should be compact: status, changed paths, verification command/result, commit if any, and unresolved concerns.

### E. Tests and reviews

- During red-green-refactor, run the smallest focused test that proves the behavior.
- At a coherent batch boundary, run the affected package or feature suite.
- Run full applicable quality gates once before PR or completion. After a fix, rerun the affected focused checks and any full gate invalidated by the change.
- Do not make reviewers rerun unchanged tests when a trustworthy report contains the command and result. Reviewers rerun tests only when evidence is missing, stale, suspicious, or affected by later changes.
- Combine final-review fixes into one bounded fix wave when safe instead of starting one agent per finding.
- A finding is closed only after its implementation and smallest useful regression coverage are present and the affected checks have been rerun. Do not restate an assigned finding as if that completed the fix.
- Review endpoint changes at all affected boundaries: service behavior, HTTP handler, production router/middleware, response projection, canonical contract, and feature-local contract when present.
- Never claim success from old output; completion evidence must match the current tree.

### F. State and recovery

- Treat the Spec-Kit artifacts and Git history as durable state; chat memory is not authoritative.
- Mark a `tasks.md` item `[X]` only when its named deliverables exist and current evidence supports completion. A skipped environment-dependent test is reported as residual verification risk, never described as a passing test.
- When Superpowers subagent-driven development is active, use its plan-specific gitignored ledger and resume from the first incomplete task. Do not create a competing committed plan.
- After context compaction, inspect the ledger, current task state, `git status`, and `git log` before redispatching work.
- Preserve user changes and untracked files. Never include unrelated changes in an agent brief, patch, stage, or commit.

## 7. Implementation Runbook

For `/speckit.implement`:

1. Team Leader validates that analysis passed and selects the next dependency-ready task or coherent batch.
2. Select the primary domain agent using the routing table.
3. Decide inline versus delegated execution using Section 6B.
4. Give the implementer the smallest sufficient file-based brief.
5. Apply TDD for behavior changes; use systematic debugging only when an actual failure occurs.
6. Run focused tests during work and the affected suite at the batch boundary.
7. Run the applicable project guards once on the completed batch.
8. Send one review package to the Tech Lead.
9. Resume the implementer for verified findings; avoid duplicate fix agents.
10. Before completion, run current quality gates and Superpowers verification.
11. Preserve the mandatory Karim review and Spec-Kit git traceability.

## 8. Definition of a Healthy Harness

The integration is working correctly when:

- Every production change traces to an approved Spec-Kit requirement and task.
- No Superpowers document competes with `spec.md`, `plan.md`, or `tasks.md`.
- Ponytail reduces implementation size only after approved scope and safety requirements are satisfied.
- Only relevant agents are dispatched, with non-overlapping ownership.
- TDD, guards, Tech Lead review, verification, and Karim's manual gate each occur once at the appropriate level.
- Full documents and conversation history are not copied into agent prompts.
- Test evidence is current, failures are debugged systematically, and completion claims are evidence-backed.
