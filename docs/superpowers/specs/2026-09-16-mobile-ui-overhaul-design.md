# Mobile UI/UX Overhaul — Design (Approved 2026-09-16, by Karim)

## Problem

App looks like "buttons and links": `main.dart:24` uses default deepPurple theme
(DESIGN.md brand never implemented); root is a dev-bridge index
(`ImplementedFeaturesScreen` in `mobile/lib/app/implemented_features_app.dart`);
no app shell, no logo, no assets (`mobile/assets/` does not exist).

## Decisions (frozen, approved by Karim)

1. **Logo**: original SVG now (8-point Islamic star/khatam + open-book/crescent
   motif, green `#1B7E3C` + gold `#D4A574`, flat, legible at 48px) at
   `mobile/assets/brand/` (logo.svg + PNG 1024/512/192/48 + monochrome variant).
   Designer files swap in later at SAME paths.
2. **Navigation**: bottom `NavigationBar`, 4 tabs — Home, Circles, Chats, Profile.
   Home = branded overview (greeting, my circles horizontal cards, active/
   upcoming sessions via existing `SessionApiClient.list()`, quick actions:
   discover + invite link). NOT F-010 dashboard.
   Chats = group chats of my circles (client-derived). NO backend changes —
   no conversations-list endpoint exists; unified DM inbox deferred (would
   need new API + ADR).
3. **Depth**: FULL redesign of all screens, delivered in waves (below).
4. **Fonts**: bundle Cairo (Arabic) + Poppins (Latin) TTFs in `mobile/assets/fonts/`,
   `fontFamilyFallback` for mixed script. NO google_fonts (offline-safe).
   Dark mode: Material 3 dark ColorScheme wired now, polished later (light-first).

## Structure — Approach A: foundation wave + per-feature waves

**Wave 0 — feature branch `019-mobile-app-shell-brand`** (via superpowers
design-doc precedent: `2026-09-16-mobile-implemented-features-navigation`):

1. `mobile/lib/core/theme/` — M3 light+dark ColorScheme from
   `docs/engineering/design/DESIGN.md` tokens (primary `#1B7E3C`,
   secondary `#D4A574`, semantic, neutrals). `main.dart` drops deepPurple.
2. Fonts bundled + declared in pubspec.
3. Logo assets; add `flutter_svg` dep; `flutter_launcher_icons` dev-dep;
   Android/iOS launcher icons; splash while Firebase initializes
   (Android 12+ splash = launcher icon natively).
4. `go_router` (ALREADY in pubspec, unused today): auth redirect +
   ShellRoute + NavigationBar 4 tabs. DELETE `ImplementedFeaturesScreen`.
5. Redesign welcome + login/register (branded, Arabic-first RTL).
6. `mobile/lib/core/design/` shared components — only if reused ≥2 screens:
   HalaqatyAppBar, SectionHeader, branded Card, Input, EmptyState,
   ErrorState, LoadingState.
7. PRESERVE all existing widget-test `Key`s (suites must survive redesign).
   Update root-nav tests (tests reference `implemented_features_app.dart`).

Constraints: no backend/contract changes, no new tables. Labels stay
inline `rtl ? 'عربي' : 'English'` ternaries (existing pattern, no intl yet).

**Waves 1–4 — each its own Spec-Kit feature, order:**
`020-circles-ui`, `021-sessions-ui`, `022-chat-ui`, `023-profile-ui`.
Per wave: ux-designer agent (flows/IA/RTL/WCAG-AA/state coverage) →
ui-designer agent (tokens/components/layouts) → senior-flutter-mobile-engineer
implements → tech-lead review.

## New agents (`.github/agents/`, matching repo convention; replaceable files)

- `ux-designer.agent.md` — flows, navigation, IA, RTL-first, WCAG AA (touch
  targets ≥48dp, semantics labels, contrast), empty/loading/error states.
  Runs during speckit.specify/clarify.
- `ui-designer.agent.md` — DESIGN.md tokens, typography, components, layouts,
  logo/brand usage, dark values. Runs during speckit.plan/tasks.
- Register both in `docs/engineering/collaboration/AGENT_WORKFLOW_HARNESS.md`
  and AGENTS.md role table. Note: opencode loads agent types at session
  start (usable next session).

## Process & verification

- Every commit with Flutter changes: fresh gates via Docker fallback
  (`ghcr.io/cirruslabs/flutter:stable`): `flutter test test`,
  `flutter analyze` (zero), `dart format --set-exit-if-changed .`
  Integration: `flutter test integration_test/` (ephemeral Linux + xvfb recipe
  in `docs/engineering/development/LOCAL_ENVIRONMENT_RUNBOOKS.md`).
- Run on `emulator-5554` vs `http://10.0.2.2:8080/api/v1` for Karim's visual review.
- Decision-register entry appended to
  `docs/management/product/MVP_DECISION_REGISTER.md` (app shell + chats-tab scope).
- RTL + LTR verified on every directional screen; tests assert content,
  not only status codes.

## Immediate next actions (execution order)

1. Save this doc to `docs/superpowers/specs/2026-09-16-mobile-ui-overhaul-design.md` + commit.
2. Create the two agents + harness/AGENTS.md registration + commit.
3. Branch `019-mobile-app-shell-brand`; implement Wave 0.
4. Docker Flutter gates green; emulator review by Karim.
5. Schedule Waves 1–4 via Spec-Kit.

## Rejected alternatives

- Big-bang single redesign (giant PRs, weeks to visible value, regression risk).
- True unified chat inbox now (needs new conversations endpoint + ADR — deferred).
