# ADR-017: Session Recovery and Reconciliation

**Status:** Accepted  
**Date:** 2026-08-19  
**Deciders:** Karim (product owner)

## Context

F-005 crosses a PostgreSQL lifecycle transaction with LiveKit room creation,
credential issuance, participant presence, and room closure. A process can stop
between either side of those operations. The existing F-005 artifacts also
contained contradictory recovery wording: provider outage was terminal in the
specification but retryable in the product journey and canonical `503` REST
contract; the current implementation generated a new random room reference on
each start attempt even though ADR-015 requires deterministic reconciliation;
and a provider close failure could be returned after `ended` had already been
committed.

## Decision

- PostgreSQL remains authoritative for lifecycle, membership, presence, lock,
  removal, and participant count.
- The provider room reference is a stable, opaque, non-guessable value derived
  from the session UUID with a backend-only HMAC key. It is never a literal
  session/circle/user ID, public response field, event field, log field, or
  metric label.
- A session-scoped PostgreSQL advisory lock covers start, join/reconnect through
  credential issuance, end, and reconciliation. Foreground operations take the
  blocking lock; the background worker uses a transaction-scoped try-lock and
  skips busy sessions.
- The reconciler runs once at startup and every 30 seconds thereafter. It scans
  at most 25 scheduled, active, and ended candidates per state. Each candidate
  receives one provider attempt with a 3-second child context; failures are
  retried on a later sweep. No recovery table or retry columns are added in MVP.
- Scheduled rows with no persisted room reference are checked for the
  deterministic orphan candidate; active rows are ensured against their
  persisted reference; ended rows are closed idempotently. A missing provider
  room is treated as an idempotent close success.
- Provider outage is recoverable, not a lifecycle terminal state. Start/join
  return `503 ERR_MEDIA_UNAVAILABLE` with no credential and no presence/count
  mutation. The client offers Retry/Leave and never performs an unbounded REST
  retry loop.
- End persists `ended` and its allowed reason before cleanup. The end response
  returns the committed ended session even if provider close fails; cleanup is
  redacted telemetry plus background reconciliation.
- WebSocket transport retries at 1s, 2s, and 4s. After those attempts the
  client stops automatic retries and offers “Tap to rejoin.” A media credential
  within two minutes of expiry is refreshed through authenticated start/join.
- The server selects a fresh join or an eligible pre-lock reconnect from
  durable presence. Clients do not choose the path with a recovery flag.

## Alternatives rejected

- A recovery table or outbox in MVP: adds durable state and replay machinery
  before scale requires it; repeated idempotent sweeps are sufficient.
- Literal session IDs as room names: conflicts with the non-guessable room
  security requirement.
- Random room references per retry: makes crash orphan identification
  impossible.
- Automatic ending on provider outage: invents a lifecycle reason and loses the
  distinction between infrastructure failure and session policy.
- Infinite synchronous retries or a job framework: risks request hangs and
  speculative infrastructure.

## Consequences

The implementation must add the HMAC room-reference derivation, shared
session-lock boundary, durable reconnect selection, bounded reconciler, `503`
error mapping, and cleanup tests before Phase 4 can pass. Repeated scans and
metrics for lock contention, provider outcomes, reconciliation latency, and
oldest pending cleanup are accepted MVP costs. Credentials, room references,
Firebase tokens, backend session IDs, and user IDs remain excluded from logs
and metric labels.
