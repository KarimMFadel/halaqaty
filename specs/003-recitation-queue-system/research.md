# Research: Recitation Queue System

## 1. Ownership and integration

**Decision**: Add a focused `backend/internal/queue` domain. Reuse F-001 identity, F-002 circle roles, and F-005 session presence/realtime/media. Queue code calls only `sessions.ReciterAudioControl`; F-005 resolves private media state.

**Rationale**: This keeps F-003 cohesive without rebuilding session lifecycle or leaking LiveKit types.

**Alternatives considered**: Put queue behavior in `sessions` (blurs ownership); add a general media/provider layer (prohibited by ADR-015); call LiveKit directly (security violation).

## 2. Round lifecycle and pre-set ordering

**Decision**: Use `prepared`, `active`, and `finalized` round lifecycle values, distinct from the five queue-entry states. Persist pre-set candidate order separately; materialize entries at activation according to policy. An active-session round activates in the creation transaction. A scheduled-session round remains prepared until F-005 start is observed.

**Rationale**: A default `present_at_activation` round cannot know its eligible population before presence exists, while pre-set order must still be durable.

At activation, explicitly pre-ordered eligible students retain their relative
order. Under `present_at_activation`, remaining present students follow by
`first_joined_at` then user ID; later joiners append. Under
`all_active_students`, remaining active student members follow by membership
`joined_at` then user ID. The UUID tie-breaker makes concurrent timestamps
deterministic.

**Alternatives considered**: Create absent students as `waiting` (violates default population); delete prepared entries at activation (destroys history); add another queue-entry state (explicitly forbidden).

## 3. Queue control API

**Decision**: Keep existing GET, round creation, grade correction, and opt-out paths. Add reset, advance, status transition, order, opt-out decision, and policy paths. `advance` durably selects the next waiting entry but does not make it `reciting`; `PUT .../status` performs allowed transitions. The existing grade path is the audited correction path after completion.

**Rationale**: This maps every manager control to one explicit command while preserving existing path identities and atomic grade-required completion.

**Alternatives considered**: A generic action endpoint (weak contract); grade endpoint that also completes (cannot represent ungraded completion cleanly); delete/add entry controls (outside accepted scope).

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

**Decision**: Reuse request deadlines and global REST/WS rate limits; bound database calls by request context and media operations by short adapter timeouts. Retry only idempotent event/media reconciliation with capped exponential backoff and jitter. Emit metrics for queue mutation latency/conflicts, event lag/failure, media convergence failure, cleanup backlog, and invariant violations; logs include request/user/session/round IDs and never payload secrets or grade notes.

**Rationale**: This meets the reliability baseline with existing infrastructure and keeps sensitive educational content out of logs.

**Alternatives considered**: Unbounded request retries (duplicate/latency risk); logging full payloads (privacy risk); new queue dashboards (explicitly out of scope).
