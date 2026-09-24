# F-019 Compatibility Contract

F-019 changes presentation only. The following surfaces are invariant:

- REST and WebSocket contracts in `docs/contracts/`
- F-001–F-005 feature contracts and role behavior
- Existing route locations and protected-route redirects
- Riverpod controller inputs, states, and actions
- Existing API client behavior and error meanings
- Widget keys and semantic behavior relied on by tests
- Chat idempotency, draft, media, archive, and offline behavior
- Session/queue retryable, terminal, role, moderation, and audio behavior
- Authentication/profile fields and identity/session boundaries

The frozen open-audio decision is authoritative: an authorized student's audio
publishing does not depend on queue turn state.

## Prohibited Contract Changes

No new endpoint, schema field/table, WebSocket event, permission, role, provider
control, media capability, conversation-discovery API, or runtime configuration
is part of F-019.

If a prototype visual cannot be populated through an invariant surface, omit or
adapt its unsupported data. Omit unplanned actions. An intentionally planned
action with no implemented behavior may remain visible only when it invokes the
shared localized under-implementation notice and causes no navigation,
controller/API call, state mutation, or implied success. Do not expand this
contract during implementation without returning to product approval and
Spec-Kit analysis.
