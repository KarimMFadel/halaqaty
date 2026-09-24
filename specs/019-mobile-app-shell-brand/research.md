# Research: Mobile App Shell and UI/UX Modernization

**Date**: 2026-09-22  
**Status**: Complete for planning

## Repository Findings

- The app already has a `StatefulShellRoute` with Home, Circles, Chats, and
  Profile; navigation behavior and `appNavigationBar` are tested.
- Light/dark Material 3 schemes, Cairo/Poppins fonts, stable brand assets, and
  shared logo/header/empty/error primitives already exist.
- Wave 0 and much of Wave 1 are present but need audit: some loads are spinner
  only, some state distinctions are incomplete, and some success is snackbar-only.
- Existing circle create and F-005 session screens/APIs are implemented but do
  not have complete production entry paths from the current shell/circle flow.
- Session/queue and chat controllers expose rich retryable, terminal, read-only,
  access-lost, pending, and recovery states that must not be collapsed.
- The Chats tab derives circle group chats from memberships. There is no unified
  conversations endpoint, so a direct-message inbox is outside approved scope.
- Existing screenshot coverage is limited and must expand to the approved matrix.
- Older F-005 listen-only wording and prototype turn-muted copy are stale. The
  frozen 2026-08-28 decision and current F-003 open-audio rule are authoritative.

## Decisions

### R-001 — Preserve current application boundaries

Restyle/recompose existing presentation widgets while preserving routes,
controllers, API clients, keys, tested semantics, and F-001–F-005 behavior.

### R-002 — Production tokens override prototype values

Use `DESIGN.md`, governance, and theme roles for color, type, spacing, radius,
contrast, and targets. HTML controls hierarchy/visual direction only.

### R-003 — Reuse before extraction

Prefer native Material 3 and existing feature/shared components. Promote a new
shared component only after two screens require the same contract. Reject a new
design package, wrappers for every M3 control, and a generic state machine.

### R-004 — Preserve typography/localization mechanisms

Keep bundled Cairo/Poppins and direction-aware copy. Wire existing `ar`/`en`
preferred language to authenticated direction and use registration/platform
locale before profile load. Add no language or localization framework.

### R-005 — Expose existing flows, not new behavior

Add production presentation entry points for existing circle create/join and
F-005 circle-session list/ad-hoc create/start/join. Do not add scheduling,
attendance, progress, notification, or account-setting capability.

### R-006 — Mobile/tablet only

Verify 320–1023dp and retain the four-tab shell. F-016 owns desktop.

### R-007 — No long-motion system

Keep existing Material transitions, purposeful short motion, and reduced-motion
compliance. Add no hero or 800ms–1s animation flow.

### R-008 — Evidence is wave-scoped

Each wave closes after focused tests, UX/UI review, and screenshot subset. Full
gates run on the final current tree.

### R-009 — One presentation-only notice for planned unavailable actions

Inventory actions in the approved design against current routes, controllers,
and APIs. Implemented actions keep their real behavior; intentionally planned
actions with no implementation call one shared localized Snackbar helper and do
nothing else. Unsupported data and unplanned prototype actions remain omitted.
This adds no backend contract, controller state, dependency, or generic feature
availability framework.

## Risks and Mitigations

| Risk | Mitigation |
|---|---|
| Visual rewrite breaks behavior/keys | Tests first; preserve controller/route boundaries |
| Prototype implies unavailable data/actions | Omit unsupported data/unplanned actions; route intentionally planned unavailable actions to the shared notice with no side effects |
| Stale turn-muted copy changes audio policy | Explicit open-audio regression requirement/test |
| Generic state hides domain recovery | Map controller states individually to shared visual grammar |
| Dark/RTL/text-scale fix regresses another wave | Wave 5 matrix plus representative screenshots per wave |
| Shared layer becomes speculative | Require two consumers and prefer native M3 |
| Integration environment unavailable | Record blocked and stop before commit; never mark skipped passing |
