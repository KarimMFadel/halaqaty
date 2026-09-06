# Quickstart: Real-time Chat

Use this as an immutable validation recipe. Store command output and human approvals in the PR/check run, not by rewriting this file.

1. Confirm branch `004-real-time-chat`; read `spec.md`, the requirements-quality checklist, ADR-010/012/016/019/021, and both canonical contracts.
2. Verify ADR-021 is accepted and Constitution 1.2.0 distinguishes permitted chat voice notes from disabled live-session recording.
3. Synchronize `contracts/chat.openapi.yaml` and `contracts/chat.ws_events.md` into `docs/contracts/openapi.yaml` and `docs/contracts/ws_events.md`; run the docs guard and `make api-lint` before implementation.
4. Add and test paired migration `000018_real_time_chat` against fresh, upgrade, rollback, and reapply schemas; use `DATABASE_URL=postgres://postgres:postgres@localhost:5432/halaqaty?sslmode=disable` with the existing `halaqaty-pg` container.
5. Implement PostgreSQL repository/service behavior before handlers: membership-period visibility, DM role matrix, idempotent send/read, sender-only read receipts, pinned retrieval, circle-row pin serialization, soft deletion/audit, normalized search, upload binding, and chat outbox.
6. Start the version-pinned MinIO Compose service and verify the private chat bucket has versioning enabled. Never persist/log a signed URL, object key, version ID, message body, or original filename. Prove marker failure rejection, active-message access restoration by removing only the latest internal marker after marker-before-commit crash, and marker repair for deleted messages.
7. Verify upload routes accept at most 21 MB while all other routes retain the 1 MiB default; cover `415`, 60-second upload timeout, and 24-hour inaccessible staged-upload cleanup.
8. Extend the existing generic WebSocket hub for per-client/session reauthorization, circle-topic group delivery, and direct authenticated-user DM delivery. Build the Flutter Riverpod chat feature with `record`, `just_audio`, `image_picker`, `file_picker`, persistent pending envelopes, bounded retry, Arabic-first RTL, accessible status semantics, and no FCM dependency.
9. Fetch sender-only read receipts and the dedicated pinned-list endpoint; reject mark-read on archived circles and confirm REST/WebSocket server projections never emit local-only `pending` or `sent` status.
10. Run the fixed performance fixture: 50 authenticated users across 10 circles, 10,000 messages/circle with 10% deleted and two membership periods, 100 warm-ups, then 1,000 history, search, and delivery samples. Require p95 ≤2 seconds and repeat delivery with intentional realtime suppression to prove REST recovery.
11. Run focused Go unit/contract/integration and Flutter unit/widget/integration tests, then full formatting, lint, coverage, OpenAPI, secret-scan, docs-guard, Tech Lead, and Karim manual security review gates. Store the evidence in the PR/review record.

Expected failure boundaries: PostgreSQL failure rejects mutation; MinIO failure rejects upload/renewal but does not affect text chat; WebSocket failure leaves the durable message available through REST reconciliation; F-008 absence has no F-004 effect.
