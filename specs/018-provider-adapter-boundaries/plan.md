# Implementation Plan: Provider Adapter Boundaries

**Branch**: `018-provider-adapter-boundaries` | **Date**: 2026-09-16 | **Spec**: [spec.md](spec.md)

## Summary

Complete feature-local storage and session-media adapter seams while preserving the accepted attachment and live-audio behavior. Keep the chat policy wrapper, move SDK construction into adapters, and split mobile interface/types from Riverpod composition. ADR-023 records this bounded refactor.

## Technical Context

**Language/Version**: Existing Go 1.26; Flutter/Dart versions pinned by the repository and authorized Docker runtime.
**Primary Dependencies**: Existing MinIO SDK, LiveKit server/mobile SDKs, Riverpod, pgx; no additions or upgrades.
**Storage**: Existing PostgreSQL and versioned private MinIO bucket; no schema change.
**Testing**: Go unit/contract/integration; Flutter unit/widget/integration, existing architecture guards.
**Target Platform**: Current Docker Compose backend and Android/iOS Flutter app; Linux/Xvfb fallback for authorized verification.
**Project Type**: Modular Go service and Flutter mobile app.
**Performance Goals**: Preserve current timeout/capacity behavior; no new performance claim or benchmark work.
**Constraints**: Public REST/WS, persistence, auth, revocation, recovery, config, secret redaction, audio policy unchanged.
**Scale/Scope**: MVP 50 concurrent users and ≤10 live sessions; MinIO/LiveKit only.

## Constitution Check

Pre-research and post-design checks pass: §§I and VI require this Spec-Kit pipeline plus task-first red/green verification; §§III–V keep current providers, backend-only credentials, authorization, server validation, audio-only fidelity and recording prohibition; §VII excludes speculative provider machinery. No constitutional amendment, DB migration, new dependency, or public contract change is proposed. Merge remains Karim-owned and manual security review is required for storage/upload/deletion paths.

## Project Structure

### Documentation (this feature)

`spec.md`, `checklists/requirements.md`, `checklists/boundaries.md`, `research.md`, `plan.md`, `data-model.md`, `contracts/provider-boundaries.md`, `quickstart.md`, and `tasks.md` under `specs/018-provider-adapter-boundaries/`.

### Source Code (repository root)

- `backend/internal/chat/media_store.go`: neutral contract plus unchanged policy wrapper.
- `backend/internal/chat/minio/adapter.go` and `adapter_test.go`: new concrete storage adapter and SDK fakes moved from chat tests.
- `backend/internal/chat/media_store_test.go`: neutral fake and chat key/MIME/size policy coverage; adjust existing chat fixtures only as needed.
- `backend/internal/sessions/livekit/adapter.go`, `adapter_test.go`: production client construction and existing test injection.
- `backend/cmd/api/main.go`: direct adapter injection; no SDK imports or SDK construction.
- `backend/tests/contract/livekit_boundary_contract_test.go`, `minio_boundary_contract_test.go`: bounded production import guards.
- `backend/tests/integration/chat_media_test.go`, `chat_media_revocation_test.go`: current real-storage acceptance fixtures use the new adapter; preserve cleanup/recovery scenarios.
- `mobile/lib/features/sessions/domain/media_connection.dart`: extracted existing type; `session_models.dart` imports and reexports it for compatibility.
- `mobile/lib/features/sessions/application/media_session.dart`: interface only, importing extracted neutral type.
- `mobile/lib/features/sessions/application/media_session_provider.dart`: direct Riverpod wiring; update existing provider consumers.
- `mobile/lib/features/sessions/data/livekit_media_session.dart`: SDK adapter imports extracted type directly.
- `mobile/test/features/sessions/livekit_boundary_test.dart`: strengthen existing guard; reuse session controller/lifecycle/model tests.

**Structure Decision**: Feature-local adapter packages implement feature-owned contracts; no shared provider framework.

## Phase 0: Research

[research.md](research.md) records inspected seams and rejected alternatives. All decisions are resolved.

## Phase 1: Design and Contracts

[contracts/provider-boundaries.md](contracts/provider-boundaries.md) defines proposed internal signatures; [data-model.md](data-model.md) records unchanged identities/transitions. Keep current `StageInput` and `MediaStore.Stage` API. Construct the MinIO adapter with existing validated `config.ChatMediaConfig`; compose `chat.NewMediaStore(adapter, cfg.OperationTimeout)`. The wrapper owns Stage validation, object-key derivation, and per-call deadline contexts; adapter configuration supplies startup deadline/bucket mechanics. Adapter exact version removal stays internal or adapter-local for test use, never on the neutral contract.

LiveKit gains proposed `NewConfiguredAdapter(cfg config.LiveKitConfig, policy config.AudioPolicy) *Adapter`, which delegates to current `NewAdapter` using an SDK room client constructed inside the adapter. Existing injected-client constructor remains adapter-local in purpose; no SDK types appear in application composition.

Mobile preserves `MediaConnection` constructors/serialization and existing reexports. No lifecycle or disposal policy changes accompany moving the provider declaration. Interface and extracted type transitively import no concrete adapter, API client, Riverpod composition, or SDK; pure existing protocol constants may remain.

## Phase 2: Execution Strategy

Generate tasks by story, with tests before each implementation. US1 storage and US2 media have independent acceptance; serialize the `main.go` composition edits. Small batches: storage guard/policy+adapter; storage composition/real tests; LiveKit+mobile seam; final gates/review. Do not mark tasks done from generated deliverables alone.

## Verification and Review

Use focused tests for red/green, then affected package suites once per batch. Final gates on the final tree: Go unit `go test -short ./...`; full contract `make test-contract`; full integration `go test -tags=integration ./...`; combined ≥80% coverage `make coverage` (all from backend with DATABASE_URL as needed); Go lint and fmt; Flutter `flutter test test`, `flutter test integration_test/`, `flutter analyze`, Dart format; canonical OpenAPI lint and secret scan. Read the local environment runbooks before Docker Flutter integration/Spectral. Log unavailable/skipped gates explicitly; focused results are not full gates. Apply clean-code-guard, test-guard, docs-guard and Tech Lead review, followed by mandatory Karim manual review before merge. No commit containing Flutter changes until all four Flutter gates pass freshly.

## Complexity Tracking

No constitutional violations or additional complexity exceptions. This plan is the sole implementation plan; no competing Superpowers plan is created. Branch/auto-commit hooks are suppressed because the approved branch exists and staging/committing is coordinator-owned. Setup template is populated through apply_patch; agent-context update is coordinator-owned outside this artifact generator's allowed paths.
