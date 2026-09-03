# Implementation Plan: Recitation Queue System

**Branch**: `003-recitation-queue-system` | **Date**: 2026-08-23 (regenerated) | **Spec**: [spec.md](spec.md) (Approved 2026-08-23)
**Input**: Feature specification from `specs/003-recitation-queue-system/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/plan-template.md` for the execution workflow.

## Summary

F-003 adds durable, sequential recitation rounds inside an active F-005 live session: a focused `backend/internal/queue` Go domain plus an additive `/api/v1` REST surface, `queue.*` WebSocket projections, one paired migration (`000017`), and Arabic-first Flutter queue screens inside the existing sessions feature. PostgreSQL is authoritative for rounds, positions, displayed turn state, grading, policy, and completed-turn practice records; delivery is at-least-once through a transactional outbox. Per ADR-020, students publish audio freely in authorized F-005 sessions; F-003 never grants, revokes, or otherwise changes media permission.

## Technical Context

**Language/Version**: Go 1.22 (backend), Dart 3 / Flutter (mobile)
**Primary Dependencies**: Echo v4, pgx/v5, golang-migrate, golang-jwt/v5 (existing — no new dependencies); Flutter: Riverpod 2.x, go_router, dio, livekit_client (sessions feature only)
**Storage**: PostgreSQL 16 (sole source of truth; golang-migrate paired `000017_*.up/.down.sql`)
**Testing**: `go test` (unit/contract `-tags=contract`/integration `-tags=integration`), `flutter test` + `integration_test/`
**Target Platform**: Linux server (single Docker Compose host), Android/iOS
**Project Type**: web-service backend + mobile app (two-stack monorepo)
**Performance Goals**: SC-008 — p95 ≤ 500 ms from PG queue-mutation commit to event dispatch to connected authorized clients (≥100 actions/scenario, local-network env); SC-007 — session-end queue convergence ≤ 10 s
**Constraints**: MVP ≤50 participants/session, ≤10 simultaneous live sessions; audio-only media; ADR-015 media boundary; no FCM in F-003
**Scale/Scope**: 3 P1 user stories; ~11 queue operations; 7 new/extended tables; Arabic-first RTL UI

No `NEEDS CLARIFICATION` items remain — all unknowns were resolved by the approved clarifications (A1, A2a–d, B1, B2, I1, I3, I4) and the reconciled ARCHITECTURE.md.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Gate | Status | Evidence |
|---|---|---|
| I. Spec-First — approved spec before plan | ✅ PASS | spec.md Status: Approved 2026-08-23; checklist 23 PASS / 21 DEFER / 0 FAIL |
| III. Tech stack — no new library/infra without ADR | ✅ PASS | No new dependencies; migration + REST/WS on existing stack |
| IV. Security invariants (backend-only media tokens, per-circle roles, parameterized SQL, rate limits) | ✅ PASS | Design D8/D9; F-003 has no media control; all SQL parameterized in `*_queries.go`; platform rate limits apply (§Security map) |
| V. Audio fidelity (Opus ≥48 kbps, no DSP, video off) | ✅ PASS | F-003 never touches provider config; ADR-015 boundary; `CanPublishVideo` always false |
| VI. Test-first | ✅ PASS | §Testing strategy; every migration/test matrix defined before code |
| VII. MVP scope discipline (YAGNI) | ✅ PASS | No FCM, no history UI, no timers, no dashboards; single new domain package |
| New tables/columns require ADR + ARCHITECTURE match | ✅ PASS | Session policy columns: ADR-018. Queue tables: canonical ARCHITECTURE §4. One constraint-line divergence found and routed as a canonical-sync delta (D2) — not a schema invention |

**Post-design re-check (after Phase 1)**: ✅ PASS — data-model.md matches canonical ARCHITECTURE.md `memorization_progress` exactly (ADR-019 no-cascade, `queue_entry_id NOT NULL UNIQUE`, `notes VARCHAR(500)`, ADR-013 five-grade CHECKs, deprecated `surah_name` until v1.1); contracts carry no FCM/history surface; no provider types cross the F-003 boundary.

## Project Structure

### Documentation (this feature)

```text
specs/003-recitation-queue-system/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (revised 2026-08-23)
├── data-model.md        # Phase 1 output (regenerated from canonical ARCHITECTURE.md)
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   ├── recitation-queue.openapi.yaml
│   └── recitation-queue.ws_events.md
├── checklists/          # /speckit.checklist output (input to this plan)
└── tasks.md             # regenerated 2026-08-23 (authoritative; supersedes the staleness note in Risks #5)
```

### Source Code (repository root)

```text
backend/
├── migrations/
│   ├── 000017_recitation_queue_system.up.sql     # F-003 tables + session policy columns + surah seed
│   └── 000017_recitation_queue_system.down.sql
├── cmd/api/routes.go                             # centralized route patterns (extended)
└── internal/
    ├── queue/                                    # F-003 domain (no LiveKit imports)
    │   ├── queue_types.go                        # rounds, entries, policy, opt-out, progress, enums
    │   ├── queue_validation.go / _test.go        # closed enums, Quran range, transitions, versions
    │   ├── queue_queries.go                      # package-level SQL statements (no inline SQL)
    │   ├── queue_repository.go / _test.go        # transactions, locks, receipts, projections
    │   ├── round_service.go / _test.go           # prepare/activation/reset/population/order/move
    │   ├── turn_service.go / _test.go            # advance/start/skip/complete/correction
    │   ├── optout_service.go / _test.go          # request/approve/decline/auto-approve
    │   ├── policy_service.go / _test.go          # prospective policy changes + audit
    │   ├── outbox.go / _test.go                  # dispatcher + visibility-aware projector
    │   └── convergence.go / _test.go             # session-end finalization + restart reconciliation
    ├── sessions/
    │   ├── queue_observer.go                     # narrow F-005→F-003 lifecycle/presence hooks
    │   └── livekit/adapter.go                    # F-005 media and explicit moderation implementation
    └── platform/
        ├── httpconst/                            # shared error codes/messages (queue additions)
        ├── logging/audit_logger.go               # redacted queue audit actions
        └── metrics/queue_metrics.go              # bounded-cardinality F-003 metrics

mobile/
└── lib/features/sessions/
    ├── data/queue_api_client.dart                # REST DTOs (current-queue surface only)
    ├── application/queue_controller.dart         # Riverpod state, dedup, version-gap re-fetch
    └── presentation/queue/                       # Arabic-first RTL student & manager queue screens

backend/tests/integration/                        # migration, round-flow, concurrency, convergence tests
mobile/test/widget/sessions/queue/                # widget tests (both text directions)
mobile/integration_test/queue_flow_test.dart      # end-to-end queue journey
```

**Structure Decision**: Single new backend domain package `backend/internal/queue` following the existing feature layout (mirrors `auth`/`profile`); Flutter work extends the existing sessions feature — no new top-level modules.

## Complexity Tracking

No constitution violations to justify.

---

## Design Decisions

### D1 — Domain placement and integration
New `backend/internal/queue` package. Reuses F-001 identity/current-device sessions, F-002 `circle_members` roles, and F-005 lifecycle/presence/realtime. Queue code imports no media boundary or provider type (ADR-020; SC-005). F-005 exposes a narrow, optional queue observer (`sessions/queue_observer.go`) that notifies F-003 of committed session-start/participant-join/session-end facts; observer callbacks are bounded and can never block or roll back F-005 lifecycle transactions.

### D2 — Round lifecycle: stacking + automatic activation (B1)
Several `prepared` rounds stack per session; at most one `active` round (partial unique `(session_id) WHERE lifecycle = 'active'`); `UNIQUE (session_id, round_number)` with max+1 allocation under the per-session round lock (CHK036). **Activation invariant**: *while a session is live, if no round is active and prepared rounds exist, the lowest-numbered prepared round is active* — restored by session start, round creation on a live session, and finalization-via-reset. No activate endpoint/command/UI. Reset finalizes the active round, creates the next sequential round, and activates the next prepared round in round-number order. Session end suppresses the invariant permanently: convergence finalizes the active round **and every never-activated prepared round** (permanently inert, retained, `activated_at` NULL — FR-014).

### D3 — Population and ordering (B2, FR-002/003)
Pre-set candidates persist in `recitation_queue_preorder` (manager surface only — CHK008). At activation, entries materialize in one transaction:
- `present_at_activation`: pre-set students **who are present** keep relative order first; then present active student members without a pre-set position by F-005 `first_joined_at`, tie-break user UUID (closes B2's deferred tie-break); absent students get no entry.
- `all_active_students`: pre-set relative order first; then all remaining active student members by `circle_members.joined_at`, tie-break user UUID.

Late/admitted joins: on a committed F-005 join fact, an eligible active student member with no entry in the active round is appended once at the end (both policies; under `all_active_students` this fires only for members added after activation). `(queue_id, student_id)` UNIQUE is the backstop.

**Mid-round membership loss (CHK002)**: the entry stays in place with no automatic transition; a manager resolves it via the existing skip (FR-004); the removed member loses queue visibility through the standard operation-time membership check (§Safety).

**Empty/ineligible populations (CHK031)**: a round with zero eligible students still activates (empty); `advance` then rejects cleanly per FR-004/A2b; ineligible candidates simply reduce the activated set (population = present eligible ∩ policy).

### D4 — Controls: advance/start separation, move, reorder (I1, A2a, A2b)
- **advance**: selects the next waiting entry durably (`selected_entry_id`); does NOT make it `reciting` or change media permission. Replace-selection on repeat advance; zero waiting entries → clean rejection (409, no mutation); while an entry is `reciting` → rejected (409). Selection clears on the entry's terminal transition and at finalization.
- **start**: `PUT .../entries/{id}/status {status: reciting}` applies only to the round's currently selected waiting entry; it records the displayed current turn only. The partial unique one-reciter index is the final queue-state guard.
- **move**: `POST .../entries/{entryId}/move {new_position, expected_version}` — repositions exactly one `waiting` entry in the active round; permitted while another entry recites; `reciting`/terminal entries immovable (409). Emits `queue.reordered (order_kind: entry_move)`.
- **reorder (full-list)**: `PUT .../order {ordered_ids, expected_version}` — pre-set candidates only, valid only while the round is `prepared`. The former `order_kind: queue_entries` full-list value is removed: entries do not exist pre-activation and full-list entry reordering is forbidden post-activation (A2a), so it could never be legal.
- **skip**: `waiting|reciting → skipped` with no media operation.
- Reset requires the live session's active round.

### D5 — Opt-out (FR-005, CHK005)
`POST .../opt-out` (student-only, idempotent). Under `approval_required`: one pending `queue_opt_out_requests` row (partial unique per entry), managers notified via targeted `queue.opt_out_requested`; any current teacher/supervisor may decide. Approve → `waiting|reciting → opted_out`, durably logged, no penalty/progress. **Decline → request terminally closed and logged; entry remains `waiting`** — the transition table permits no other outcome. Under `auto_approve`: direct idempotent transition to `opted_out`, no pending state. Pending requests become non-actionable when the round finalizes (later decision → clean 409).

### D6 — Grading, completion, correction, progress (FR-007/008/013, A2c, CHK035)
Grading-required round: `completed` requires one of the five ADR-013 grades + optional ≤500-char note, atomically, in the same transaction that inserts the single `memorization_progress` row. Grading-optional round: `completed` carries **no grade and no note**; both may be added/changed later **only** via `POST .../grade` (FR-013 correction): replaces grade and/or sets/clears the note (`notes: null` clears), updates entry + the same one progress row atomically (`queue_entry_id NOT NULL UNIQUE` makes retries and re-grades safe), emits a redacted audit event. Repeated identical corrections converge idempotently to one record state (upsert semantics; CHK035). Correction boundaries: `audited_any_time` (default) / `before_round_finalization` / `immutable`, enforced at correction time. Skipped/opted-out entries never create a progress row (SC-004). `test`-type completions retain their practice record; F-007 excludes them from Quran-map derivation.

### D7 — Policy (FR-012, ADR-018, A2d)
Five CHECK-constrained columns + `queue_policy_version` on `sessions`. Manager-only (current teacher/supervisor), scheduled-or-active sessions, every change audited. Workflow policies (population, finalization, opt-out, correction) apply to subsequent actions only. **Grade/note visibility applies immediately and prospectively** to new snapshots and events; delivered history is never rewritten; clients re-fetch on `queue.policy_changed` (FR-009 pattern), which also self-heals stale/out-of-order visibility events (CHK034 — resolved in spec).

### D8 — Voluntary queue / F-005 media boundary (FR-010, ADR-020)
F-003 imports no media-control interface and never changes participant publishing permission. It stores and projects manager-set queue order and displayed turn state only. F-005 grants audio publishing to every authorized student connection and retains its independent explicit moderation controls. No room reference, endpoint, credential, provider identifier, or URL crosses into F-003 code, persistence, events, logs, caches, or URLs (SC-005); `CanPublishVideo` remains always false.

### D9 — Concurrency, idempotency, conflicts (CHK032)
Mutations serialize on the per-session round lock (`SELECT ... FOR UPDATE` on the round row / session advisory lock) + expected-version optimistic checks (`version` on round and entries) + PostgreSQL constraints as the final barrier (one active round, one entry/student, one position/entry, one displayed reciting entry, one progress/completed entry). Optional client `Idempotency-Key` stored in `queue_command_receipts`; replays return/reconstruct the committed resource; key reuse with another command is a conflict. Racing pairs and their serializer (CHK032): advance/start — round lock + displayed-reciter partial unique; reset/complete — round lock, reset requires active round, complete requires non-finalized; reorder/late-join — preorder lock pre-activation vs append-at-end (disjoint rows, `(queue_id, position)` rewritten transactionally); move/late-join — same; opt-out/skip — entry lock + version, first terminal transition wins, second gets 409; policy-change/action — policy changes only affect subsequent actions by design, no rollback needed; correction/reset — correction requires a completed entry in a non-finalized round under `before_round_finalization` (round lock orders them). Duplicate/concurrent requests converge to exactly one durable outcome (§Safety).

### D10 — Realtime, outbox, recovery (FR-009, A1, CHK025/033)
Every committed queue mutation inserts a redacted-metadata row into `queue_event_outbox` in the same transaction. Client rows are delivered to authorized connected clients through the existing hub, whose worker reconstructs visibility-sensitive fields (grades/notes/names) from PostgreSQL at send time. Delivery: initial attempt + **5 retries, exponential backoff (+1/+2/+4/+8/+16 s, jittered), then parked** (`parked_at` set, metric+alert fires; operator replay — never silently dropped). Clients deduplicate client events by `event_id`, ignore stale versions, and re-fetch `GET /sessions/{id}/queue` on reconnect/gap/unknown event. **Restart recovery (CHK033)**: on startup the dispatcher replays pending + parked client outbox entries and the wider convergence pass finalizes rounds of ended sessions. No FCM/device-token work anywhere in F-003 (I4) — F-008 later projects the stable durable event IDs.

### D11 — Session-end convergence (FR-014, A1, SC-007)
F-005 session end commits and returns independently. After observing end, F-003 finalizes the active round under the finalization policy (`mark_unfinished_skipped` default / `preserve_last_state`), finalizes every never-activated prepared round as permanently inert, and clears selection — all idempotently, retried until finalized, **convergence target ≤ 10 s** after observing session end. Failure never changes the already-ended session result; parked-retry exhaustion is the observable terminal outcome when convergence cannot complete.

### D12 — History preservation (FR-006, I3)
Finalized rounds and their entries are immutable, retained rows. `GET /sessions/{id}/queue` returns the latest round's snapshot (including a finalized one, read-only) — the **only** F-003 read surface. No history-list REST endpoints, no mobile history UI (F-007 owns projections). Corrections update the current projection only through the audited flow.

### D13 — Canonical sync strategy (FR-015, CHK017/018/044)
Feature-local contracts (regenerated above) are the F-003 truth for implementation. Canonical `docs/contracts/openapi.yaml`, `docs/contracts/ws_events.md`, and one ARCHITECTURE.md line are **not** edited in the plan phase; every pending delta is listed in §Canonical sync and the first implementation task applies them with `$docs-guard` + `make api-lint` before any handler/DTO code. Enum verification (CHK018): 5 entry states · 4 round types · 5 grades · 5 policy dimensions — identical across spec, data-model.md, feature-local contracts, and (post-sync) canonical contracts.

---

## Reliability parameters (A1 — Architect-confirmed)

| Parameter | Value | Governs |
|---|---|---|
| Outbox delivery retries | initial attempt + **5 retries**, exponential backoff (+1/+2/+4/+8/+16 s, jitter) | queue-event delivery |
| Retry exhaustion | **parked** (`parked_at`, alert, operator replay) — never silent drop | queue-event delivery |
| Session-end finalization | idempotent retry until finalized; **≤ 10 s** convergence target after observing end; never blocks/alters F-005 end | SC-007 |
| SC-008 latency | **p95 ≤ 500 ms** PG-commit → dispatch to connected authorized clients; ≥100 committed actions/scenario; standard local-network test env; disconnected clients excluded (recover via FR-009 re-fetch) | realtime delivery |

## Security map (CHK037 — per surface)

All surfaces require Firebase bearer + `X-Halaqaty-Session-ID` (F-001), active circle membership (F-002), operation-time role checks; all run behind platform REST rate limits (per-IP + per-user) and WS limits (3 connections/user, 30 msgs/min/user/circle). SQL is parameterized and defined in `queue_queries.go`; routes centralized in `cmd/api/routes.go`; HTTP constants from `platform/httpconst`.

| Surface | AuthZ | Validation |
|---|---|---|
| `GET /sessions/{id}/queue` | active member; visibility-filtered projection; preorder managers-only | session/round existence |
| `POST .../rounds`, `POST .../reset`, `PATCH .../policy` | current teacher/supervisor; scheduled-or-live session | round type/surah range/ayahs/grading flag/order IDs/policy enums/expected versions |
| `POST .../advance`, `PUT .../order`, `POST .../entries/{id}/move` | current teacher/supervisor | version, waiting/prepared state, position bounds, order membership+uniqueness |
| `PUT .../entries/{id}/status` | current teacher/supervisor | transition table, selected-entry rule, conditional grade/note, version |
| `POST .../entries/{id}/grade` | current teacher/supervisor + correction policy | completed-entry, policy boundary, grade enum, note ≤ 500 |
| `POST .../opt-out` | student, own entry | entry state, idempotency |
| `POST .../opt-out-requests/{id}/decision` | current teacher/supervisor | pending request, decision enum, version |
| `queue.*` WS events | authorized session-topic subscribers; grade/note events visibility-filtered at send time | server-built payloads only |

Error codes (centralized in `httpconst`): queue conflict cases map to 409 (stale version, invalid transition, no waiting entry, entry reciting/terminal, finalized/inert round, duplicate command); input-shape failures to 400; enum/range/order/grade/note failures to 422. F-003 has no audio-convergence error because it performs no media operation.

## Privacy map (CHK038)

| Data class | Stance |
|---|---|
| Queue identity/position/state (incl. opted-out facts) | authorized session participants only (FR-009, SC-008 "authorized clients") |
| Grades & notes | `queue_grade_visibility`-gated projections; redacted audit events; never in delivery logs or metrics |
| Pending opt-out requests | current teachers/supervisors + requester only |
| Audit metadata | actor/resource IDs + timestamps in PG audit rows; no notes/grades in audit payloads |
| Metrics/labels | UUIDs and closed enums only — no PII, no names, bounded cardinality (extends SC-005; ties to CHK040) |
| Media material | never present in any F-003 artifact (SC-005) |

## Audit model (CHK007/015)

"Durably logged"/"audited" = **PostgreSQL-persisted business facts**, not ops logs (operational logs are non-authoritative telemetry). Discrete durable audit records (structured, redacted, actor-attributed): policy changes, opt-out requests + decisions, grade/note corrections (prior/current values, no note text), and manager attribution on every queue mutation (`created_by`, `resolved_by`, `decided_by`, `added_by`, receipt actor). Durable **state history** (no separate audit row needed — the rows themselves are the preserved record): turn transitions, skips, completions, resets, finalizations (queue tables + outbox are append-mostly and retained per FR-006). Every manager mutation carries actor attribution. No MVP retention policy contradicts preservation.

## Observability (CHK040)

Metrics (bounded labels: command/event/outcome enums + IDs; no PII): `queue_command_duration`, `queue_command_conflicts_total`, `outbox_pending`, `outbox_parked_total` (alert), `event_delivery_lag` (the SC-008 commit→dispatch metric, p95), `session_end_finalization_lag` (SC-007, alert past 10 s), `invariant_violations_total` (alert — one-active-round/one-displayed-reciting-entry/one-progress guards), rate-limit counters on queue surfaces. Logs carry request/user/session/round IDs; never payload secrets, grades, or notes.

## Testing strategy

Test-first per constitution §VI. Evidence matrix extends the SCs with the deferred scenarios:

| Criterion | Evidence (tests) |
|---|---|
| SC-001 one displayed `reciting` entry / one position | acceptance + concurrency; **+ stale-version replays and process-restart reruns** (CHK025) |
| SC-002 convergence | duplicate/retry + reconnect re-fetch; **+ outbox replay after restart; invariant re-check after crash windows** (CHK025) |
| SC-003 invalid input rejection | unauthenticated/non-member/unauthorized-role, invalid Quran ranges/types/grades/notes; **+ policy enum values, order membership/uniqueness, opt-out decision values, expected-version conflicts** (CHK026) |
| SC-004 progress exactly-once | every grade × completion; grading-optional completion; insert-vs-update separation for corrections; skip/opt-out → zero rows |
| SC-005 no media material | contract/security scans of events, persistence, logs, cache inputs, URLs; **+ outbox metadata, audit metadata, error details, metrics labels, notification payloads, client diagnostics** (CHK028) |
| SC-006 policy coverage | every value of all five policy dimensions; correction under all three boundaries incl. repeated-identical + note-clear; policy change never rewrites history |
| SC-007 session-end | forced finalization/media failure; idempotent retry convergence ≤10 s; parked exhaustion observable; F-005 end result unmodified |
| SC-008 latency | ≥100 committed actions per scenario, local-network env, commit→dispatch p95 ≤ 500 ms, disconnected excluded |
| A2 edges | advance replace-selection / no-waiting / while-reciting; move while reciting; move-reciting-entry rejected; preorder reorder post-activation rejected |
| B1/B2 | several prepared rounds activate in round-number order; never-activated prepared rounds inert at end; pre-set-present-then-join-order placement; UUID tie-break |
| CHK033 | commit-then-event-failure (retry→park), Reset writes revoke barrier before next-round activation, grant/revoke failure (5 s timeout, truth intact), app-restart replay, closed/missing media room |

Migration tests run on fresh schema (up/down/up) per constitution; contract tests pin REST/WS shapes against the canonical files after sync.

## Mobile experience (FR-016, CHK039)

Arabic-first RTL-aware queue screens (verified in both text directions) inside `mobile/lib/features/sessions/` with Riverpod controllers (no `setState` in feature code). Enumerated UI states: **loading** (skeleton), **empty** (no round prepared — guidance for students, prepare CTA for managers), **reconnecting** (banner + FR-009 re-fetch on version gap), **recoverable errors** (retry: advance/start/complete failures), **terminal** (round finalized / session ended — read-only). Manager reset, skip, policy change, and F-005 end controls stay available when a queue operation or delivery path fails. Accessibility: semantic labels for positions/statuses/grades (Arabic), minimum touch targets, screen-reader-friendly turn announcements. No history UI, no FCM handling.

## Canonical sync (completed by T001–T003; barrier reconciliation included)

Canonical REST, WebSocket, and architecture surfaces match the feature-local contracts: several prepared rounds may stack; activation is automatic in round-number order, including after reset; queue actions never change participant audio permission; no activate endpoint exists; and never-activated rounds are inert at session end. The canonical partial unique is `WHERE lifecycle = 'active'`.

Feature-local files already align with canonical schema names (`StartRoundRequest`, `GradeRequest`); canonical `surah_name` fields in `queue.your_turn`/`queue.round_started` are kept (public reference data, not sensitive) and mirrored in the feature-local catalog.

## DEFER ledger traceability (21/21 closed by this plan)

| CHK | Closed by |
|---|---|
| CHK002 eligibility/mid-round membership loss | D3 (entry stays; manager skip; membership check gates visibility) |
| CHK005 opt-out decline path; pending at finalization | D5 |
| CHK007 audit classes (discrete vs state history) | §Audit model |
| CHK008 prepared-order manager-surface only | D3 + data-model visibility projection + QueueState preorder note |
| CHK015 durable audit = PG, ops logs non-authoritative | §Audit model |
| CHK017 five states consistent across artifacts | data-model.md state machines + both contracts (this regeneration) |
| CHK018 enums identical across artifacts | D13 verification + data-model CHECKs + contract enums |
| CHK025 stale-version + restart evidence | §Testing strategy (SC-001/002 rows) + D10 restart replay |
| CHK026 SC-003 enumeration extension | §Testing strategy (SC-003 row) + §Security map |
| CHK028 SC-005 extension surfaces | §Testing strategy (SC-005 row) + §Privacy map + §Observability |
| CHK031 empty population / ineligible / end-before-activation | D3 (empty round) + D2 (inert rounds) |
| CHK032 conflict serialization per racing pair | D9 |
| CHK033 restart + closed/missing media room recovery | D10 + D8 |
| CHK034 (PASS — already resolved in spec) | noted: A2d re-fetch pattern in D7/contracts |
| CHK035 correction idempotency + note clearing | D6 |
| CHK036 round-number allocation | D2 + data-model (`UNIQUE (session_id, round_number)`, max+1 under lock) |
| CHK037 per-surface authN/authZ/validation/rate limits | §Security map |
| CHK038 privacy per data class | §Privacy map |
| CHK039 mobile UX states + accessibility | §Mobile experience |
| CHK040 observability metrics set | §Observability |
| CHK042 queue has no media-control dependency | D8 |
| CHK044 exclusion lists consistent | D12/D13 + contracts (no history/FCM/timer surfaces anywhere) |

(CHK034 was PASS in the checklist; listed for completeness of the 2026-08-23 defer set.)

## Risks & notes for the tasks phase

1. **Canonical sync is required** — regenerate canonical session-admission, queue, and event documentation from ADR-020 before implementation.
2. **`move` is the one operation this regeneration adds** to the previously-drafted contract set — derived 1:1 from approved A2a; everything else reuses existing paths.
3. Task dependencies: canonical sync (CS-1..7) → migration → domain/validation → services → outbox/convergence → handlers/routes → mobile. Flutter DTO work must not start before canonical sync lands.
4. Tech Lead deep-review surface (mandatory Karim review): queue mutation authorization (role checks on all 10 operations), one displayed-reciting-entry invariant and proof that queue code never controls audio, correction/audit redaction, session-end independence, open student-audio admission, explicit moderator controls, and the migration (constraints/partial indexes) — schedule the review after handlers + convergence exist.
5. `tasks.md` is regenerated for ADR-020; do not mark a reopened audio-coupled task complete without fresh open-audio evidence.
6. SC-008 measurement harness (commit→dispatch timestamps) is backend-only tooling; keep it out of production paths (metrics-based, not bespoke tracing).
