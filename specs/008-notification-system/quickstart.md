# F-008 Quickstart and Verification

This feature is not implemented yet. Use these checks after the corresponding tasks land; they do not replace full repository gates in `AGENTS.md`.

1. Read `plan.md`, `data-model.md`, `contracts/service.md`, ADR-027, and source-specific handoff notes before implementation.
2. Set `SPECIFY_FEATURE_DIRECTORY=specs/008-notification-system` in the current PowerShell process for Spec-Kit scripts.
3. Apply migration 000021 to a fresh PostgreSQL database, verify down/up rollback behavior, and test account closure cleanup. Never use `migrate-fresh` on valued data.
4. Run focused Go package tests for each changed domain, then unfiltered backend unit and contract suites. Run integration and combined coverage only with configured PostgreSQL per `DEVELOPMENT.md`.
5. Run Flutter format, analyze, widget/unit, integration, and visual suites using the local runbooks; record unavailable device/backend gates as blocked, not passed.
6. Verify OpenAPI and WebSocket contracts, notification matrix coverage across app foreground/background/terminated states, F-006 stale reminder suppression, preference/quiet-hours transitions, unread reconciliation, and invalid token removal.
7. Test manager deletion with no/missing supervisor, transactional intent failure, successful atomic commit, and post-commit FCM outage. Keep the route disabled until operational readiness evidence and Karim's required security review.
8. Confirm no push payload or logs contain chat bodies, grade details, media credentials/endpoints/room refs, or raw tokens; confirm all queries are recipient-scoped.

The current database migration sequence ends at 000020; F-008's additive schema is planned as 000021 with a paired down migration.
