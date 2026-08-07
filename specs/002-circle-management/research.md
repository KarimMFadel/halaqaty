# Research: Circle Management

## Decision 1: Reuse the existing circle foundation

- **Decision**: Extend the existing `circles` and `circle_members` tables and Feature 001 auth/RBAC patterns. Do not replace the existing circle foreign-key migrations or create parallel authorization tables.
- **Rationale**: Feature 001 already established `circle_members` as the per-circle authorization source and migrations `000013`/`000014` as the deployed circle foundation.
- **Constraints**: Applied migrations remain immutable; all schema changes use the next additive migration.

## Decision 2: Circle role lifecycle

- **Decision**: Follow ADR-010. Circle creation is transactional: selected existing users may become teachers, one optional registered user may become backup supervisor, and the creator becomes teacher only when no teacher is selected; otherwise the creator becomes supervisor. Invite joins create students.
- **Rationale**: Preserves the approved multi-teacher and delegated-management model.
- **Alternatives rejected**: Global roles, creator-only ownership, client-side role enforcement.

## Decision 3: Public and private joining

- **Decision**: Support both public discovery/join and invite-link joining. Public circles are discoverable and joinable; private circles require an invite. Both modes retain an invite link and invite-code regeneration.
- **Rationale**: Matches the clarified product decision while preserving private-circle access control.
- **Compatibility**: Keep the existing invite-code join endpoint and add public discovery/direct-join operations additively.

## Decision 4: Invite-code format and rotation

- **Decision**: Use a unique 8-character total code, including the `HLQ-` prefix (for example, `HLQ-7X2K`). Refresh is transactional: the old code becomes unusable before the new code is returned.
- **Rationale**: Matches F-002 and the architecture example; corrects the existing contract example/pattern that describes a longer code.
- **Reliability**: Join and refresh are idempotent at the state boundary and must not create duplicate membership or accept a superseded code.

## Decision 5: Retirement instead of hard deletion

- **Decision**: A circle is retired by setting its archived state. Hard deletion of circles, memberships, or circle history is prohibited. Archived circles remain readable for reporting and historical use, while new activity and new joins are blocked.
- **Rationale**: Prevents irreversible data loss and preserves audit/reporting history.
- **Required alignment**: Amend F-002 product wording and record the retirement decision through the repository's ADR/amendment process before implementation.

## Decision 6: Contract and API compatibility

- **Decision**: Preserve existing v1 circle routes and response shapes. Add only the missing public discovery, public direct-join, invite refresh, and complete request/response fields needed by F-002. `DELETE /circles/{circleId}` remains archive/retirement, never hard delete.
- **Rationale**: Keeps existing clients compatible while making the feature contract complete.
- **Required alignment**: Synchronize `docs/contracts/openapi.yaml` with the feature contract and run Spectral before implementation.

## Decision 7: Security and reliability baseline

- **Decision**: Require Feature 001 dual credentials on protected routes, validate all request fields server-side, enforce per-circle authorization, rate-limit mutation endpoints, write audit events for create/join/role/member-removal/invite/archive mutations, use request IDs and structured logs, and retry only safe idempotent reads.
- **Rationale**: Protects trust boundaries and prevents inconsistent membership/role state under retries or concurrency.

## Decision 8: Circle and personal gender are separate concepts

- **Decision**: Circle `gender_restriction` accepts `male`, `female`, `mixed`, or `unspecified` and defaults to `unspecified`. It describes the student audience and does not restrict teacher gender. Personal gender, if introduced by its owning profile feature, is limited to `male` or `female`.
- **Rationale**: A male teacher may teach female students, so teacher gender cannot determine a circle's audience setting.
- **Migration consequence**: Existing rows without a value may safely use the documented `unspecified` default.

## Decision 9: Persistent audit design requires an ADR

- **Decision**: Add and accept `ADR-012-audit-logging-persistence.md` before implementing audit persistence. The ADR must decide the sink/table, schema, retention, sensitive-field redaction, transaction/failure behavior, indexing, and access policy.
- **Rationale**: The plan requires durable security evidence, but the current architecture does not define a persistence mechanism.
