# Data Model: Preserved State

No tables, columns, migrations, or new durable models are introduced.

| Existing concept | Preserved identity and behavior |
|---|---|
| Upload | Server-generated UUID and deterministic internal `chat/<UUID>` object key; existing staged/attached/revoked transitions and authorization binding |
| Message | Existing active/deleted state, transaction ordering, moderation audit and outbox behavior |
| Storage deletion marker | Provider-private exact version ID; never public/persisted; versionless marker revokes links, latest-marker recovery restores active-message access without deleting bytes |
| Session | Existing opaque `media_room_ref`, lifecycle, audio-only mode, and authoritative membership/presence/reconciliation |
| MediaConnection | Existing endpoint, credential and expiry; ephemeral memory only; extracting the Dart declaration changes no serialized field |

Cleanup claims/release/finalization and message-deletion recovery retain the F-004 baseline. Session reconciliation retains ADR-015/ADR-017. PostgreSQL remains authoritative; no signed URL, provider version identity, registry state, or client storage key is added.
