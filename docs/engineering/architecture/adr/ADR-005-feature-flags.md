# ADR-005: Feature Flag Strategy

**Status:** Accepted  
**Date:** 2026-04-26  
**Deciders:** Karim (product owner)

---

## Context

Several Halaqaty features will be built but not enabled in the MVP: video, recording, AI Tajweed analysis. We need a feature flag mechanism that:

1. Allows the backend to enable/disable features without a code deployment.
2. Allows the Flutter app to conditionally render UI based on which features are active.
3. Does not require a third-party feature flag service (LaunchDarkly, Split.io) with its associated cost and data-sharing implications.
4. Scales to per-tier flags for the monetization model (Free vs Pro vs Institution).

---

## Decision

We will use **environment variable flags on the backend** surfaced to the Flutter app via a dedicated config endpoint.

**Flag naming convention:**
```
FEATURE_RECORDING_ENABLED=false
FEATURE_VIDEO_ENABLED=false
FEATURE_AI_TAJWEED_ENABLED=false
FEATURE_ANALYTICS_ENABLED=false
```

**Backend config endpoint:**
```
GET /api/v1/config/features
```

Returns a JSON object of active flags, filtered by the authenticated user's tier:
```json
{
  "recording": false,
  "video": false,
  "ai_tajweed": false,
  "analytics": false
}
```

**Flutter app behavior:**
- Fetches `GET /api/v1/config/features` at app startup (after auth).
- Stores the result in a `featuresProvider` (Riverpod).
- All feature-gated UI watches `featuresProvider` before rendering.
- The Flutter app never reads env vars directly. It trusts the API response.

**Per-tier logic (post-MVP):**
When video is enabled globally (`FEATURE_VIDEO_ENABLED=true`), the endpoint returns `video: true` only for Pro and Institution tier users. Free users get `video: false` regardless of the global flag.

---

## Consequences

**Positive:**
- Zero external service dependency. One env var change + restart enables a feature.
- Per-tier filtering is handled by the backend — the Flutter app has no billing logic.
- Flags are auditable in deployment configuration (`.env` files, Docker Compose, Kubernetes secrets).
- Adding a new flag requires one env var definition, one entry in the endpoint response, and one watch in Flutter. No DSL to learn.

**Negative:**
- No real-time flag push — Flutter app needs to restart (or call the endpoint again) to see a flag change. Acceptable for features that change rarely.
- No user-level targeting (e.g., "enable recording for circle X only"). If needed post-MVP, replace env vars with a DB-backed `feature_flags` table. The API contract stays the same.
- No rollback capability per se — reverting requires re-deploying with the flag set to `false`. Acceptable at MVP scale.

---

## Alternatives Considered

| Option | Reason Rejected |
|---|---|
| **LaunchDarkly / Split.io** | Adds ~$50-200/month cost and sends user context (UID, email) to a third party. Privacy concern for a Quran learning app. Overkill for MVP. |
| **DB-backed `feature_flags` table** | More flexible than env vars (no restart required; per-circle targeting). Added complexity is not justified at MVP scale with 2-3 flags. Recommended migration path post-MVP. |
| **Compile-time flags in Flutter** | Can't change a flag without a new app release. Not usable. |
| **Firebase Remote Config** | Google-hosted config service. Would work, but sends app context to Google. Inconsistent with our "Firebase for identity only" boundary (ADR-004). |

---

## References

- `../../management/product/MVP_DECISION_REGISTER.md` — PRD-5 (video flag rollout model), OQ-017 (recording disabled)
- `.specify/memory/constitution.md` — Feature flag strategy section
