# Requirements Checklist: Circle Management

**Purpose**: Validate that the Feature 002 specification is complete, clear, consistent, and ready for planning.
**Created**: 2026-08-07
**Feature**: [../spec.md](../spec.md)

## Scope and traceability

- [x] CHK001 Feature identity, branch, status, and source input are recorded.
- [x] CHK002 F-002 scope is separated from F-003 queue, scheduling, LiveKit, chat, payments, and notifications.
- [x] CHK003 Feature 001 authentication/session behavior is explicitly reused and not redesigned.
- [x] CHK004 Public discovery/join and invite-link join are both represented.
- [x] CHK005 Circle retirement is represented as archive/soft state; hard deletion is explicitly prohibited.
- [x] CHK006 User decisions from clarification are recorded in the spec.

## User stories and acceptance criteria

- [x] CHK007 Each user story has a priority, rationale, independent test, and Given/When/Then scenarios.
- [x] CHK008 Circle creation covers name, description, rules, capacity, privacy, language, circle gender (`male|female|mixed|unspecified`), invite code, and initial memberships.
- [x] CHK009 Joining covers public circles, private circles, invite links, duplicate membership, capacity, archived circles, and the five-circle limit.
- [x] CHK010 Circle/member reads define member visibility and non-member denial behavior.
- [x] CHK011 Role management covers teacher/supervisor authorization, self-change denial, cross-circle denial, and final-teacher protection.
- [x] CHK012 Retirement covers preserved history, blocked new activity, teacher-only authorization, and hard-delete prohibition.
- [x] CHK013 Member removal is explicit scope: teacher-only, no self/final-teacher removal, and historical records retained.
- [x] CHK014 Acceptance scenarios do not require queue, session, chat, notification, or payment behavior.

## Functional requirements and data

- [x] CHK015 Functional requirements are uniquely numbered and use testable MUST statements.
- [x] CHK016 Validation limits match the approved baseline: name 100, description 500, rules 1000, capacity default 50/max 200, and circle gender `male|female|mixed|unspecified` with default `unspecified`.
- [x] CHK017 Role behavior references OQ-036 and ADR-010 instead of inventing global roles.
- [x] CHK018 Invite regeneration invalidates the previous code and preserves uniqueness.
- [x] CHK019 The five-circle limit, circle capacity, and duplicate-membership constraints are explicit.
- [x] CHK020 Key entities cover Circle, CircleMember, and InviteCode without prescribing an unsupported table design.
- [x] CHK021 Persistence and reporting requirements preserve archived circle history.

## Security, reliability, and testability

- [x] CHK022 Protected endpoints retain Firebase ID-token and opaque backend-session validation from Feature 001.
- [x] CHK023 Per-circle authorization and standard error responses are explicit.
- [x] CHK024 Atomic creation and join behavior prevents partial writes.
- [x] CHK025 Edge cases cover invalid input, invalid invites, capacity, archive state, role escalation, self-change, and final-teacher loss.
- [x] CHK026 Success criteria include automated authorization, uniqueness, consistency, and historical-retention verification.
- [x] CHK027 Mobile, REST, migrations, OpenAPI, unit, contract, and integration work are identified without prescribing implementation details.

## Canonical-document alignment findings

- [x] CHK028 `docs/management/product/FEATURES.md` F-002 defines archive/retirement with history retention and prohibits hard deletion.
- [x] CHK029 `docs/management/product/JOURNEY.md` T-05 defines public discovery/join, private invite-only behavior, and invite links for both.
- [x] CHK030 `docs/contracts/openapi.yaml` defines public discovery, public direct join, and invite-code refresh operations.
- [x] CHK031 `docs/engineering/architecture/adr/ADR-012-audit-logging-persistence.md` exists with Accepted status and defines the MVP audit sink and constraints.

## Checklist result

**Result**: PASS — specification quality and canonical-document alignment are ready for implementation.

**Gate**: Resolve CHK028–CHK031 before implementation. `/speckit.plan` may proceed only after the source-document decisions are amended, the API contract is aligned, and the audit-persistence ADR is accepted.
