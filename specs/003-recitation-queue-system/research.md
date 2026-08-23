# Research: Recitation Queue System

> Revised 2026-08-23 after spec approval and the A1/A2/B1/B2 clarifications.
> Sections 2, 3, and 10 were updated; sections 11-13 were added. All decisions
> below are Karim-approved via the approved spec text or the Architect-confirmed
> reliability parameters.

## 1. Ownership and integration

**Decision**: Add a focused `backend/internal/queue` domain. Reuse F-001 identity, F-002 circle roles, and F-005 session presence/realtime/media. Queue code calls only `sessions.ReciterAudioControl`; F-005 resolves private media state.

**Rationale**: This keeps F-003 cohesive without rebuilding session lifecycle or leaking LiveKit types.

**Alternatives considered**: Put queue behavior in `sessions` (blurs ownership); add a general media/provider layer (prohibited by ADR-015); call LiveKit directly (security violation).

## 2. Round lifecycle and pre-set ordering

**Decision**: Use `prepared`, `active`, and `finalized` round lifecycle values, distinct from the five queue-entry states. Persist pre-set candidate order separately; materialize entries at activation according to policy. Several prepared rounds may stack per session with sequential numbers (clarification B1: activation order is round-number order when several are prepared). Activation is governed by one invariant — *while a session is live, if no round is active and a prepared round exists, the lowest-numbered prepared round is active* — restored by: F-005 session start, round creation on a live session, round finalization via reset, and a reconciliation pass after restart/crash. No activate endpoint, command, or UI exists. Session end suppresses the invariant permanently: the convergence worker finalizes the active round and every never-activated prepared round (retained, `activated_at` stays NULL, never activatable — FR-014 "permanently inert").

**Rationale**: A default `present_at_activation` round cannot know its eligible population before presence exists, while pre-set order must still be durable. One invariant covers session-start activation, mid-session first prepare, reset chaining, and crash recovery without a special case per trigger.

**Alternatives considered**: One prepared-or-active round per session (contradicts B1 "several prepared"); an explicit manager activate action (explicitly removed by B1); create absent students as `waiting` (violates default population); delete prepared entries at activation (destroys history); add another queue-entry state (explicitly forbidden).

At activation, explicitly pre-ordered eligible students retain their relative
order. Under `present_at_activation`, remaining present students follow by
`first_joined_at` then user ID; later joiners append. Under
`all_active_students`, remaining active student members follow by membership
`joined_at` then user ID. The UUID tie-breaker makes concurrent timestamps
deterministic (closes the B2 deferred tie-break).

## 3. Queue control API

**Decision**: Keep existing GET, round creation, grade correction, and opt-out paths. Add reset, advance, status transition, order, move, opt-out decision, and policy paths. `advance` durably selects the next waiting entry but does not make it `reciting`; `PUT .../status` performs allowed transitions and `start` applies only to the currently selected entry. The existing grade path is the audited correction path after completion. Order control splits by clarification A2a: full-list reorder (`PUT .../order`) applies only to pre-set candidates while the round is prepared; after activation the only order mutation is `move` (`POST .../entries/{entryId}/move`), which repositions exactly one `waiting` entry to an arbitrary slot, is permitted while another entry is `reciting`, and must reject moving the `reciting` entry.

**Rationale**: This maps every manager control in FR-004 to one explicit command while preserving existing path identities and atomic grade-required completion. Entries do not exist before activation, so a full-list entry reorder can never be legal after activation — a single-entry move is the minimal post-activation control the approved spec requires.

**Alternatives considered**: A generic action endpoint (weak contract); grade endpoint that also completes (cannot represent ungraded completion cleanly); delete/add entry controls (outside accepted scope); expressing move as a full-list order replace (that is the full-list reorder A2a forbids after activation).

## 4. Policy persistence

**Decision**: Add five closed, CHECK-constrained policy columns plus `queue_policy_version` to `sessions`. Apply defaults to existing rows. Emit redacted structured audit events after each committed change.

**Rationale**: One policy per session is a small fixed projection; columns are simpler and safer than a rules engine. ADR-018 authorizes the additive session extension.

**Alternatives considered**: JSON/key-value policy (allows unsupported combinations); global defaults only (rejects clarification); separate current-policy table (unneeded join and lifecycle).

## 5. Concurrency and idempotency

**Decision**: Serialize mutations by locking the current round, use expected round/entry versions, enforce invariants with PostgreSQL unique/partial indexes, and store supplied idempotency keys in `queue_command_receipts`. Replays return or reconstruct the committed resource; operations without a key still converge through state checks and constraints.

**Rationale**: At-least-once clients need durable deduplication, while database constraints remain the final race barrier.

**Alternatives considered**: In-memory mutexes/deduplication (not authoritative and unsafe across restarts); Redis locks (outside MVP); optimistic checks without constraints (race-prone).

## 6. Completion, progress, and corrections

**Decision**: Entry completion and the one `memorization_progress` insert occur in one transaction. A unique `queue_entry_id` makes retries safe. Allowed corrections update both entry and the same progress row atomically; prior/current values and actor attribution are emitted in redacted audit metadata. `test` records are retained but later F-007 derivation excludes them.

**Rationale**: This meets completed-only progress truth and preserves F-007's documented input contract.

**Alternatives considered**: Asynchronous progress creation (can lose records); a second F-003 practice table (duplicates F-007's canonical source); insert-on-grade separate from completion (breaks atomicity).

## 7. Media convergence

**Decision**: PostgreSQL commits authoritative turn state. A start grant is attempted immediately and retried with bounded backoff when unavailable. A current reciter must be revoked before another entry can start. Ended-session cleanup is idempotent asynchronous convergence and never delays F-005's committed end.

**Rationale**: PostgreSQL and LiveKit cannot share a transaction. This ordering never grants a non-reciter and favors safety over temporary availability.

**Alternatives considered**: Grant before DB commit (temporary unauthorized publisher); distributed transaction (unsupported/over-engineered); blocking F-005 end (rejected by ADR-018).

## 8. Realtime, FCM, and recovery

**Decision**: REST is authoritative; WebSocket and in-app projections are at-least-once and versioned. Clients deduplicate by `event_id`, reject stale versions, and re-fetch after reconnect/gaps. F-003 does not create FCM/device-token infrastructure; F-008 may later project the stable targeted turn event through Firebase with the same deduplication key.

**Rationale**: No FCM foundation exists, and F-008 owns general notifications. This preserves feature ownership without changing F-003 truth or later delivery semantics.

**Alternatives considered**: Implement general Firebase Messaging in F-003 (scope conflict); trust event ordering (unsafe after reconnect); persist queue state in the hub (constitutional violation).

## 9. Backward compatibility

**Decision**: Keep `/api/v1` and all existing queue paths; add fields and controls before implementation. The existing queue contract is an unimplemented placeholder, so completing required round fields does not break a deployed F-003 client. Existing F-001/F-002/F-005 schemas and session/media responses are unchanged.

**Rationale**: Contract-first correction occurs before any F-003 production behavior or mobile consumer exists.

**Alternatives considered**: Create `/api/v2` (no deployed incompatibility warrants it); encode missing fields through hidden defaults (would invent product rules).

## 10. Timeouts, retries, and observability

**Decision**: Reuse request deadlines and global REST/WS rate limits; bound database calls by request context and media operations by a 5-second adapter call timeout. Retry only idempotent event/media reconciliation with capped exponential backoff and jitter. Outbox delivery: 5 retries with exponential backoff, then parked for operator replay (parked rows alert; never silently dropped). Session-end queue finalization: idempotent retry until finalized, convergence target ≤ 10 seconds after observing session end, never blocking or altering the F-005 session-end result. SC-008 latency: p95 ≤ 500 ms from PostgreSQL queue-mutation commit to dispatch to connected authorized clients, ≥ 100 committed actions per scenario, standard local-network test environment; disconnected clients are excluded and recover via the FR-009 re-fetch. Emit metrics for queue mutation latency/conflicts, event lag/failure, media convergence failure, cleanup backlog, and invariant violations; logs include request/user/session/round IDs and never payload secrets or grade notes.

**Rationale**: These are the Architect-confirmed A1 parameter values; they meet the reliability baseline with existing infrastructure and keep sensitive educational content out of logs.

**Alternatives considered**: Unbounded request retries (duplicate/latency risk); logging full payloads (privacy risk); new queue dashboards (explicitly out of scope).

## 11. Round stacking vs. the one-current-round constraint

**Decision**: Allow several `prepared` rounds per session; enforce at most one `active` round via a partial unique index on `(session_id) WHERE lifecycle = 'active'` (not `IN ('prepared','active')`). Sequential numbering stays enforced by `UNIQUE (session_id, round_number)`.

**Rationale**: Approved clarification B1 explicitly names "which prepared round activates when several are prepared", which is impossible if only one prepared-or-active round may exist. The reconciled ARCHITECTURE.md queue section still carries the older `IN ('prepared','active')` wording; the spec is the approved authority, so the plan adopts the active-only partial unique and records a one-line ARCHITECTURE.md sync delta for the implementation phase.

**Alternatives considered**: Keep the one-prepared-or-active constraint (makes B1's rule and mid-live prepare-stacking unimplementable); a separate "next round" pointer table (extra state for no benefit).

## 12. Canonical contract sync strategy

**Decision**: Regenerate the feature-local contracts to the final F-003 shape now; leave `docs/contracts/openapi.yaml`, `docs/contracts/ws_events.md`, and the one ARCHITECTURE.md constraint line untouched in the plan phase. Record every pending canonical delta as an explicit list in `plan.md`; the first implementation task applies them to the canonical files with `$docs-guard` review and `make api-lint` before any handler or mobile DTO is written.

**Rationale**: FR-015 makes canonical reconciliation a precondition of implementation, but canonical docs are repo-level sources of truth edited in their own reviewed change — not silently inside `/speckit.plan`.

**Alternatives considered**: Edit canonical contracts during planning (mixes doc-authority phases, unreviewed); defer reconciliation to whenever handlers land (invites drift, violates FR-015 ordering).

## 13. Delivery scope boundary (F-008/F-007)

**Decision**: F-003 delivers REST/WebSocket/in-app projections only, all at-least-once with stable durable `event_id`s. No FCM, device-token, or push code, payload, or metric exists in F-003; `queue.your_turn` is documented as the stable future FCM deduplication source (I4). No history REST surface beyond `GET /sessions/{id}/queue` returning the latest round (finalized rounds included, read-only); historical-round projections and UI are F-007 work consuming preserved server-side history (I3).

**Rationale**: Feature-ownership boundaries frozen by the approved clarifications; durable IDs and preserved rows are the complete handoff contract.

**Alternatives considered**: Ship minimal FCM plumbing now (scope conflict with F-008); expose round history list endpoints (explicitly deferred to F-007).
