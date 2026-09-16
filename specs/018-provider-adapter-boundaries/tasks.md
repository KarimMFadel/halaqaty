# Tasks: Provider Adapter Boundaries

**Input**: Approved [spec.md](spec.md), [plan.md](plan.md), research, data model and internal contracts.  
**Tests**: Required task-first red/green under constitution §VI. Implementation tasks remain unchecked until named deliverables and fresh verification exist.

## Phase 1: Setup

- [X] T001 Verify F-018 approval, frozen PROVIDER-BOUNDARY decisions, branch/worktree and baseline evidence against specs/018-provider-adapter-boundaries/spec.md and docs/management/product/FEATURES.md; record verification without altering F-004 artifacts.

## Phase 2: Foundation

- [X] T002 Extend the approved architecture guard in backend/tests/contract/livekit_boundary_contract_test.go to scan backend/cmd/api plus backend/internal, and add production MinIO confinement in backend/tests/contract/minio_boundary_contract_test.go; run focused contract tests and record the current composition/storage violations as red evidence (FR-001, FR-005, FR-009).

## Phase 3: US1 - Private Attachment Lifecycle

**Goal**: Preserve validation, access revocation, retained bytes and crash recovery behind a neutral chat contract.  
**Independent test**: Existing real-MinIO chat media/revocation scenarios plus chat policy and concrete adapter unit suites.

- [X] T003 [US1] Adapt backend/internal/chat/media_store_test.go to a neutral ObjectStore fake and preserve key derivation, MIME normalization/allowlist, nonpositive-size rejection, deadlines and error propagation; add a runnable failing interface test before production edits (FR-001, FR-002, FR-007).
- [X] T004 [US1] Add adapter behavior tests in backend/internal/chat/minio/adapter_test.go by relocating relevant SDK fakes/assertions from backend/internal/chat/media_store_test.go; cover bucket existence/versioning failures without new bucket-policy behavior, signing TTL/versionlessness, marker idempotency, exact marker removal, latest-marker recovery/no-op and listing/removal failures (FR-003, FR-007).
- [X] T005 [US1] Implement chat.ObjectStore/ObjectPutInput and retain chat.MediaStore policy in backend/internal/chat/media_store.go; add backend/internal/chat/minio/adapter.go implementing the proposed contract with current SDK/config behavior, internal version IDs and retained-byte semantics; update existing chat test helpers as needed and run affected unit suites (FR-001–FR-004, FR-007).
- [X] T006 [US1] Replace MinIO SDK construction in backend/cmd/api/main.go with direct storage-adapter construction and policy-wrapper injection, preserving config validation, disabled behavior, startup error paths and cleanup/reconciliation workers (FR-001, FR-007, FR-008).
- [X] T007 [US1] Adapt only required fixtures in backend/tests/integration/chat_media_test.go and chat_media_revocation_test.go; retain all existing upload, versionless-link revocation, deletion retry, cleanup and marker-before-commit/reconciler recovery checks, and run the affected real PostgreSQL/MinIO scenarios (FR-003, FR-004, FR-008; SC-001).

## Phase 4: US2 - Authorized Live Audio

**Goal**: Complete backend/mobile media dependency direction without changing lifecycle or user flows.  
**Independent test**: Existing adapter/session/model/controller/lifecycle suites and strengthened boundary guards.

- [X] T008 [US2] Preserve injected room-client behavior coverage in backend/internal/sessions/livekit/adapter_test.go and document the T002 composition violation as the failing architectural check before moving production construction (FR-005, FR-007).
- [X] T009 [US2] Add NewConfiguredAdapter inside backend/internal/sessions/livekit/adapter.go and remove LiveKit SDK construction/imports from backend/cmd/api/main.go; preserve direct injection, existing config/audio policy, webhook verification and lifecycle recovery; run affected session/adapter suites and the extended contract guard (FR-005, FR-007, FR-008).
- [X] T010 [US2] Strengthen the existing mobile/test/features/sessions/livekit_boundary_test.dart to cover interface/type import direction (including transitive API-client dependencies) and reuse existing connection serialization and session controller/lifecycle tests; run a meaningful failing guard before mobile production edits (FR-006, FR-009).
- [X] T011 [US2] Extract MediaConnection into mobile/lib/features/sessions/domain/media_connection.dart with compatibility reexports in session_models.dart; leave application/media_session.dart interface-only, move its unchanged Riverpod declaration to application/media_session_provider.dart, update adapter/provider consumer imports, and preserve serialization/lifecycle behavior; run affected mobile tests (FR-006, FR-008; SC-002).

## Phase 5: Cross-cutting Verification and Review

- [X] T012 Run clean-code-guard and test-guard on the completed production/test diff and docs-guard on docs/engineering/architecture/adr/ADR-023-provider-adapter-boundaries.md plus specs/018-provider-adapter-boundaries/; confirm canonical docs/contracts/openapi.yaml, ws_events.md, persistence and deployment configuration behavior are unchanged; obtain Tech Lead review and resolve verified findings (FR-004, FR-007–FR-010; SC-003).
- [X] T013 Run all final applicable gates in specs/018-provider-adapter-boundaries/plan.md on the final tree: Go unit/full contract/full integration/combined coverage/lint/fmt; Flutter full unit-widget/integration/analyze/format; OpenAPI lint and secret scan; record commands, final exits and blocked/skipped distinctions (SC-001–SC-004).
- [ ] T014 Obtain Karim's mandatory manual storage/upload/deletion security review before merge and record its approval separately in the F-018 review handoff; no automatic merge or completion claim while this gate is pending (SC-004; docs/engineering/architecture/adr/ADR-023-provider-adapter-boundaries.md).

## Dependencies and Parallel Opportunities

T001 → T002 → storage batch T003–T005 → composition/real-test batch T006–T007. US2 T008–T011 is behaviorally independent of US1 but T009 must follow T006 to avoid shared main.go ownership conflicts. T010–T011 may run independently of backend implementation with exclusively mobile paths once T001 completes. No [P] markers are assigned inside the serial canonical sequence: test-before-production ordering and shared files require explicit coordinator scheduling. For optional parallel work, mobile T010–T011 and backend T003–T007 have disjoint paths; they need one owner each and their own red/green evidence. T012 follows both stories; T013 follows any review fixes; T014 follows successful gates and remains approval-gated.

## Implementation Strategy

Use the smallest coherent batches and one implementer/one reviewer per batch. First validate US1's complete attachment lifecycle, then US2's authorized live audio seam, then final guards/gates/manual review. US1 alone is a useful independent storage increment; F-018 as a whole requires both stories. Do not add vendor abstractions, migration machinery, dependencies or a duplicate Superpowers plan. Do not commit Flutter edits until all four fresh Flutter gates pass. Generated checklist completion is requirement validation, never evidence that these tasks are complete.
