---
name: architecture-review
description: Reviews Halaqaty architecture decisions against scalability, reliability, security, and maintainability standards, then returns actionable findings.
---

# Architecture Review Skill

Use this skill when asked to review solution design, architecture docs, or major technical trade-offs.

## Clarification first
- If the request lacks key context, ask business owner **Karim** exactly **5-7 concise questions** before proceeding.
- Prioritize questions about scope, traffic expectations, critical user journeys, security/privacy constraints, and deployment expectations.

## Inputs to inspect
- `README.md`
- `docs/management/product/PRD.md`
- `docs/management/planning/PROJECT_PLAN.md`
- `docs/management/product/FEATURES.md`
- `docs/engineering/architecture/ARCHITECTURE.md`
- `docs/engineering/deployment/DEPLOYMENT.md`

## Review checklist
1. Domain alignment: architecture supports real Quran circle workflows.
2. Scalability: clear growth path from MVP to higher concurrency.
3. Reliability: failure modes and recovery paths are identified.
4. Security: auth boundaries, data protection, and access control are explicit.
5. Data design: entities and relations support required product behavior.
6. Realtime behavior: queue, chat, and live session consistency are credible.
7. Operability: deployment, monitoring, and backup strategy are practical.
8. Cost realism: infrastructure choices fit stage and budget.

## Output format
- **Strengths**
- **Risks**
- **Gaps / open decisions**
- **Recommended decisions (ordered by impact)**
- **Suggested implementation sequence**

Keep output specific, decision-oriented, and tied to existing docs.
