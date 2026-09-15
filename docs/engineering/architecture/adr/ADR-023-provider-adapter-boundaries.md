# ADR-023: Complete Feature-local Provider Adapter Boundaries

**Status:** Accepted scope; implementation pending  
**Date:** 2026-09-16  
**Decider:** Karim (product owner)

## Context

ADR-015 already places session media behind `SessionMediaGateway` and mobile `MediaSession`. Current `backend/cmd/api/main.go` nevertheless constructs LiveKit and MinIO SDK clients. Current `chat.MediaStore` contains MinIO request/result types, signing, version listing, and delete marker operations alongside chat key/MIME policy. Mobile `media_session.dart` imports the API-client reexport and constructs the concrete adapter in its Riverpod declaration; `session_models.dart` also imports a circle API client for its role type. These inspected dependency gaps make future replacement broader than the intended feature-local seams.

## Decision

Karim approved the reviewed F-018 refactor and six PROVIDER-BOUNDARY decisions: global deployment configuration, no settings UI, seams now and alternatives later, replacement before live deployment with no migration/failover work now, current MinIO/LiveKit only, and capability-first business wording retaining current provider names.

Retain `chat.MediaStore` as the validation/key policy wrapper and give it a chat-owned neutral object-store contract. Provider mechanics and SDK construction move into a feature-local MinIO adapter. Preserve `StageInput`, deterministic server-generated object keys, server MIME/size validation, existing startup bucket/version validation, versionless signed GETs, idempotent deletion markers, retained physical versions, and complete deletion-before-commit recovery through `RemoveLatestDeleteMarker`. Exact marker version IDs remain adapter-private. Existing upload/moderation/cleaner/reconciler consumers keep the policy wrapper.

Move LiveKit SDK room-client construction into its existing backend adapter; inject it directly from main using unchanged validated deployment config. Keep `SessionMediaGateway`, webhook verification, backend-only credential issuance, audio-only fidelity, and lifecycle reconciliation unchanged.

Separate mobile `MediaSession` from Riverpod concrete wiring. Extract the existing `MediaConnection` declaration to a neutral model file and preserve compatibility reexports so the interface/type import graph does not pull API clients. Preserve serialization, lifecycle and user flows. The existing single mobile adapter remains the only SDK importer.

Extend backend import guards to include application composition and enforce MinIO confinement as well as LiveKit. Strengthen the existing mobile guard; complement source-policy checks with behavior and real-storage revocation/recovery verification. Proposed file paths and signatures are specified in [F-018 plan](../../../../specs/018-provider-adapter-boundaries/plan.md); they describe the approved implementation target, not already delivered code.

## Compatibility and Constitutional Alignment

Constitution §§I/VI require completed Spec-Kit artifacts, task-first tests and current quality gates. §§III–V retain MinIO/LiveKit, PostgreSQL authority, backend-only credentials, authorization, validation, recording prohibition and Quran audio policy. §VII requires narrow MVP scope. This decision amends no constitution or schema.

Keep public REST/WS and persistence unchanged, including internal object keys only; signed URLs and version IDs are never persisted as identity. Keep current environment names, defaults, validation, disabled configuration behavior, redaction and deadlines. No second vendor, generic registry, provider flags, dependency, DB migration, live replacement, migration, or failover is added. ADR-015's separately approved future rollout requirements remain applicable if future work introduces another provider after live deployment.

## Consequences

- SDK changes are confined to concrete adapters and direct deployment wiring.
- Retaining the chat policy wrapper limits consumer churn and protects existing deletion/recovery behavior.
- Narrow mobile type extraction avoids an unrelated session/circle model refactor.
- This seam is not proof that a second vendor can satisfy retained-version revocation semantics; any replacement requires separate acceptance and governance.
- Karim's mandatory manual review of storage/upload/deletion paths and all applicable final gates remain required before merge.

## Alternatives Considered

| Alternative | Reason rejected |
|---|---|
| Keep SDK clients/types in main and chat | Leaves the observed ownership gaps |
| Move upload key/validation into storage adapter | Makes provider code own chat policy |
| Replace every chat consumer with a new generic gateway | More churn than preserving the current policy wrapper |
| Import all session models into the mobile interface | Transitively imports the circle API client |
| Registry, settings UI, provider flags, second vendor | Outside approved scope with one implementation |
| Schema/provider migration and automatic failover | No live replacement is required in F-018 |

## References

- [ADR-015](ADR-015-session-media-provider-boundary.md)
- [ADR-021](ADR-021-chat-persistence-media-and-delivery.md)
- [Constitution](../../../../.specify/memory/constitution.md)
- [F-018 specification](../../../../specs/018-provider-adapter-boundaries/spec.md)
