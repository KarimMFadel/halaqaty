# Research: Provider Adapter Boundaries

## Storage seam

**Decision**: Preserve `chat.MediaStore` as a policy wrapper and replace its SDK-shaped client with a chat-owned `ObjectStore`. Put provider mechanics in `backend/internal/chat/minio/adapter.go`.

**Rationale**: Existing upload, moderation, cleaner, reconciler, and direct-chat consumers can retain their `*MediaStore` dependency. Chat still derives `chat/<upload UUID>` and validates type/MIME/size. The adapter owns bucket configuration, SDK options, version listing, signing, delete markers, and exact version removal.

**Alternatives considered**: Move all staging into the adapter (leaks chat policy); replace every consumer with a new gateway (larger diff without needed behavior); generic storage registry (one approved provider).

## Session setup

**Decision**: Add production client construction to the existing LiveKit adapter; preserve its injected-client constructor for adapter tests.

**Rationale**: `cmd/api/main.go` currently constructs SDK clients despite ADR-015. Moving that line into the adapter completes the already accepted direction.

**Alternatives considered**: Separate general factory package or resolver (unneeded); retain SDK in main (boundary gap).

## Mobile dependency direction

**Decision**: Extract only `MediaConnection` from `domain/session_models.dart` into `domain/media_connection.dart`; retain a compatibility reexport. Move Riverpod concrete wiring to `application/media_session_provider.dart`.

**Rationale**: The current `session_models.dart` imports `circle_api_client.dart` for `CircleRole`, so merely importing that whole model file from the interface would leave an indirect API-client dependency. A narrow extracted type avoids unrelated circle-model changes.

**Alternatives considered**: Move CircleRole and all session models (unrelated scope); duplicate MediaConnection (identity/serialization drift); keep the interface importing the API client reexport (dependency gap).

## Verification scope

**Decision**: Extend existing LiveKit contract scan to `backend/cmd/api` and add MinIO confinement for production backend imports. Reuse and strengthen `mobile/test/features/sessions/livekit_boundary_test.dart`; retain complementary lifecycle and real-MinIO revocation tests.

**Rationale**: The existing backend scan walks only internal packages and misses the observed composition leak. Source guards are explicitly authorized architecture requirements, not substitutes for privacy/recovery behavior.

**Alternatives considered**: A new general architecture framework or duplicated mobile test harness (unneeded).

All scope choices are resolved by approved decisions and inspected code; no dependency research or new vendor is needed.
