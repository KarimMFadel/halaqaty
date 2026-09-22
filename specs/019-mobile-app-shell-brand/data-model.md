# Compatibility Model: Mobile App Shell and UI/UX Modernization

F-019 introduces no database, API, WebSocket, domain entity, or durable client
model. Existing F-001–F-005 models remain unchanged.

## Presentation Concepts

These are testable UI concepts, not persistent entities:

### Screen Presentation

- **wave**: 0–5 traceability owner
- **screen/route**: existing or newly reachable presentation destination
- **role view**: existing student, teacher, supervisor, archived member, or
  unauthenticated view
- **primary action**: one dominant existing product action
- **action availability**: `implemented` uses its existing behavior;
  `under implementation` invokes the shared localized notice only. This is a
  presentation classification, not persisted product data or controller state.
- **state**: loading, empty, ready/success, recoverable error, terminal/read-only,
  or offline/degraded as exposed by an existing controller
- **direction/theme/scale/width**: verification variants
- **compatibility surfaces**: route, widget key, semantic behavior, controller,
  API, and feature contract that remain stable

### Visual Evidence Record

- wave and screen
- role and state
- locale/direction and theme
- viewport and text scale
- artifact name and review outcome

Evidence is produced by tests/review and is not product data.

## State Mapping Rule

Existing controller/domain state is authoritative. Presentation may map it to a
consistent visual treatment but may not add a lifecycle transition, permission,
retry policy, audio rule, or persisted fact. Activating an `under implementation`
action may show the shared notice but may not navigate, call a controller/API, or
mutate product state.

## Migration Impact

None. No migration or rollback file is permitted for F-019.
