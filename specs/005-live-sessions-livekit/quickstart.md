# Quickstart: Live Sessions (LiveKit)

1. Confirm the `005-live-sessions-livekit` branch, read the spec, checklist, ADR-015, ADR-016, and canonical contracts.
2. Complete Phase 0: synchronize canonical OpenAPI/WebSocket contracts with both feature-local contracts, apply the docs-guard checklist manually, and run `make api-lint`.
3. Add and test the paired session/presence migration on fresh and upgrade schemas; do not modify applied migrations.
4. Implement the sessions domain and persistence first, then the narrow LiveKit adapter/reconciler, realtime ticket hub, REST handlers, and Flutter `MediaSession` shell.
5. Keep the token/credential boundary private: issue only from Go, use `Cache-Control: no-store`, store in Flutter memory only, and never log/broadcast it.
6. Verify focused Go unit/contract/integration tests, then Flutter widget and integration tests using the repository’s approved Docker workflow.
7. Run `make api-lint`, backend `go test -short ./...`, Flutter test/analyze/format gates, formatting, lint, and secrets checks before requesting Tech Lead and Karim review.
