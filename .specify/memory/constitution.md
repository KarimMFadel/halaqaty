# Halaqaty Constitution
<!-- حِلْقَتي — Mobile-first Quran memorization circle platform -->

## Core Principles

### I. Spec-First (NON-NEGOTIABLE)
No code is written without a completed and approved Spec-Kit spec. Every feature must follow the **complete Spec-Kit workflow** before `/speckit.implement`:

1. **`/speckit.specify`** — Product requirements → Technical specifications
2. **`/speckit.clarify`** — Ask 5-7 targeted questions; resolve ambiguities
3. **`/speckit.checklist`** — Unit test spec quality (completeness, clarity, consistency)
4. **`/speckit.plan`** — Architecture design, schema, API contracts, testing strategy
5. **`/speckit.tasks`** — Break design into actionable, testable tasks with dependencies
6. **`/speckit.analyze`** — Cross-artifact consistency check before implementation
7. **`/speckit.implement`** — Execute tasks with tests, reviews, quality gates

**Key Principle**: Agents (Golang Developer, Flutter Engineer, Architect, Tech Lead, Team Leader) collaborate throughout all phases. When ambiguous, **agents ask Karim clarifying questions** rather than guess. See `docs/engineering/collaboration/AGENT_COLLABORATION_GUIDE.md` for full agent collaboration model.

### II. Spiritual Mission Above All
Halaqaty is a Quran memorization platform. Every decision — UX, monetization, data retention — must respect the spiritual and educational nature of the product. **No advertisements. Ever.** No dark patterns. No engagement-maximizing tricks. The platform serves the memorizers and their teachers, not the other way around.

### III. Tech Stack (NON-NEGOTIABLE)
- **Backend**: Go — lean, single binary, deployed as a single modular service in MVP
- **Mobile**: Flutter (Dart) — single codebase for Android and iOS
- **Database**: PostgreSQL via `pgx` driver — the sole source of truth for all persistent state
- **Realtime delivery**: WebSocket (Go) for pushing events to clients; PostgreSQL is the source of truth (NOT in-memory maps or caches)
- **Voice sessions**: LiveKit (self-hosted, audio-only in MVP) — WebRTC SFU
- **Identity**: Firebase Auth — for token issuance and verification only
- **Authorization**: PostgreSQL `circle_members` table — roles are per-circle, not global
- **File storage**: MinIO (S3-compatible, self-hosted) for voice notes and media attachments
- **Push notifications**: Firebase Cloud Messaging (FCM)
- **DNS/TLS**: Cloudflare
- **Deployment (MVP)**: Docker Compose on a single Hetzner CX22 (~$8–12/month)

No library, framework, or infrastructure change may be introduced without a new ADR in `docs/engineering/architecture/adr/`.

### IV. Security Invariants (NON-NEGOTIABLE)
These rules are never broken, in any environment, under any circumstances:

1. **LiveKit tokens are generated exclusively by the Go backend**. The Flutter client never calls LiveKit APIs directly and never generates tokens.
2. **Firebase Auth is for identity only**. All authorization checks query PostgreSQL `circle_members`. A valid Firebase JWT does not grant any action without a matching role record.
3. **Roles are per-circle**. A user can be teacher in one circle and student in another simultaneously.
4. **Student audio publishing is open within authorized live sessions**. Authorized students receive audio-publish permission for the session; F-003 queue actions never grant, revoke, mute, or otherwise change it. Teachers and supervisors retain explicit F-005 moderation controls, and video publishing remains disabled ([ADR-020](../../docs/engineering/architecture/adr/ADR-020-voluntary-recitation-queue.md)).
5. **Recording is DISABLED in MVP**. The `FEATURE_RECORDING_ENABLED` flag must stay `false` until a privacy/legal framework is formally documented and approved. This is not negotiable.
6. **All input is validated server-side**. The Flutter client is never trusted. Ayah numbers, file types, MIME types, and sizes are re-validated on the Go backend.
7. **Parameterized queries only**. No string-interpolated SQL. Use `pgx` named or positional parameters exclusively.
8. **Rate limiting is enforced**. REST API: per IP and per user ID. WebSocket: max 3 active connections per user. Messages: max 30/min per user per circle.

### V. Audio Fidelity for Quran (NON-NEGOTIABLE)
Quran recitation demands pristine, unprocessed audio. Every LiveKit room configuration must:
- Use **Opus codec at 48 kbps or higher**
- **Disable noise suppression** (preserves tajweed phonetics)
- **Disable automatic gain control** (preserves authentic recitation dynamics)
- **Disable echo cancellation** where possible (preserves natural voice)
- Never enable video publishing in MVP. LiveKit token `CanPublishVideo` is always `false`.

### VI. Test-First Development
- Unit tests are written alongside implementation, not after. Red-green-refactor cycle.
- Integration tests are required for every API endpoint and every WebSocket event handler.
- No PR is merged with any failing test.
- Every database migration must be tested against a fresh schema before merging.
- Go test coverage target: ≥80% aggregate for `backend/internal/` packages, measured from the combined unit + contract + integration profile (`make coverage` in `backend/`). A bare unit-only profile undercounts because contract/integration tests are behind build tags.

### VII. MVP Scope Discipline (YAGNI)
- Scale target: **50 concurrent users, ≤10 simultaneous live sessions** in the first 6 months.
- Infrastructure: **single Docker Compose server**. No Kubernetes. No Redis. No multi-region.
- Code must be architecturally clean enough to split into 2–3 services later — but do NOT over-engineer for scale that does not exist yet.
- If a feature is not in the approved spec for the current sprint, it does not get implemented. No "while I'm here" additions.

---

## Technology Constraints

### Database
- PostgreSQL is the **only** data store in MVP. No Redis, no in-memory session maps.
- All session queue state is persisted in PostgreSQL — not held in goroutines or application memory.
- DB migrations use **golang-migrate** with sequential versioning: `000001_create_users.up.sql` / `.down.sql`.
- Every migration must have a corresponding rollback `.down.sql` file.
- Schema changes are never applied manually in production — always through migration files in CI.

### API Design
- REST endpoints are documented in `docs/contracts/openapi.yaml` (OpenAPI 3.x). No endpoint may be implemented that is not in that contract.
- All error responses follow the standard envelope:
  ```json
  { "error": { "code": "ERR_CIRCLE_NOT_FOUND", "message": "Circle does not exist" } }
  ```
- Standard HTTP semantics apply: `400` bad input, `401` unauthenticated, `403` forbidden, `404` not found, `409` conflict, `422` unprocessable, `500` internal.
- WebSocket events follow the schema catalog in `docs/contracts/ws_events.md`.
- All timestamps in API payloads are UTC ISO 8601. Clients convert to local timezone using the user's stored IANA timezone string.

### Feature Flags
- Post-MVP capabilities live behind boolean feature flags in Go configuration (environment variables):
  - `FEATURE_VIDEO_ENABLED` (default: `false`)
  - `FEATURE_RECORDING_ENABLED` (default: `false`)
  - `FEATURE_AI_TAJWEED_ENABLED` (default: `false`)
  - `FEATURE_ANALYTICS_ENABLED` (default: `false`)
  - `FEATURE_WEB_ENABLED` (default: `false`)
- Activating `FEATURE_RECORDING_ENABLED` requires a signed-off privacy framework document merged to `main`.
- Flutter clients must read feature flags from a backend endpoint — never hardcoded in the app binary.

---

## Development Workflow

### Spec-Kit Pipeline (per feature — this is the only valid path to production)

```
/speckit.specify [feature description]     → specs/NNN-feature-name/spec.md
/speckit.clarify  (optional)               → de-risks ambiguous areas
/speckit.plan [tech choices]               → plan.md, data-model.md, contracts/
/speckit.checklist (optional)              → validates completeness
/speckit.tasks                             → tasks.md with [P] parallelization hints
/speckit.analyze (recommended)             → cross-artifact consistency check
/speckit.implement                         → code + tests + migrations
/speckit.git.commit                        → structured commit with spec traceability
```

### PR Requirements
- PR title format: `HLQ-NNN: description` or `#NNN: description` (enforced by GitHub Actions).
- PR description must include: `Implements: specs/NNN-feature-name/`.
- PRs are opened by Copilot; reviewed and merged by Karim only.
- PRs must not be merged until all quality gates are green.

### Quality Gates (all must be green before merge)
| Gate | Tool |
|---|---|
| All unit tests pass | `go test ./...` / `flutter test` |
| All integration tests pass | `go test -tags=integration ./...` |
| Go coverage floor | `make coverage` (from `backend/`) | ≥80% aggregate over `backend/internal/`, combined profile |
| DB migration tested on fresh schema | CI step with `golang-migrate` |
| No linter violations | `golangci-lint` (Go) / `flutter analyze` (Dart) |
| Formatter clean | `gofmt` / `dart format` |
| OpenAPI contract validated | No undocumented endpoints |
| No hardcoded secrets | `gitleaks` or equivalent |
| Security checklist | Auth on all protected routes, no missing role checks |

---

## Key Business Rules (Frozen for MVP)

These are final decisions. Do not re-open or work around them during implementation.

| Topic | Rule |
|---|---|
| Phone auth | No phone-only accounts. Require email or social provider. Phone is supplementary only. |
| Token expiry | Firebase 1hr auto-refresh. Backend enforces 30-day inactivity logout. |
| Circle teaching roles | Multiple teachers are allowed per circle; a supervisor is a distinct per-circle role. See ADR-010. |
| Queue timer | No per-student timer in MVP. Teacher manages timing verbally. |
| Pre-set queue | Teacher can pre-order queue before starting the session. |
| Double-queue | No. One position per student per round. Use sequential rounds. |
| Announcements | No announcement channel. Pinned messages serve this purpose. |
| Voice max length | 5 minutes (300s). Max file size: 20 MB. |
| Emoji reactions | Deferred to P2. Not in MVP. |
| Session max duration | 4 hours. 30-minute idle room timeout after last participant leaves. |
| One-off sessions | Allowed. Not required to link to a recurring schedule. |
| Timezone storage | UTC in DB. IANA timezone string per user profile. Display in local tz. |
| Student self-log | Not allowed in MVP. Progress comes from session recitations only. |
| Multiple passes | Allowed. Each queue entry creates a new progress record. Full history kept. |
| Mushaf text | Tanzil.net (CC BY 3.0). No licensing cost. |
| Mushaf audio playback | No in MVP. Mushaf is reading-only. Deferred to P2. |
| Monthly email reports | No in MVP. Manual PDF export only. |
| AI / Tajweed analysis | Fully deferred. Not until recording is unblocked. |
| Circle on account deletion | Archived. Members notified. Teacher must designate supervisor first. |

For full rationale, see `docs/management/product/MVP_DECISION_REGISTER.md`.

---

## Governance

This constitution supersedes all other practices, preferences, or conventions in the Halaqaty codebase. Any amendment requires:

1. A new or updated ADR in `docs/engineering/architecture/adr/` explaining the change and rationale.
2. An update to this constitution with the version bump and amendment noted.
3. Explicit approval from Karim (the product owner) before any implementation proceeds.

All Copilot agents must verify constitutional compliance before generating code. When uncertain, do less and ask. Complexity must be justified. Simplicity is the default.

**Version**: 1.1.0 | **Ratified**: 2026-04-26 | **Last Amended**: 2026-08-30
