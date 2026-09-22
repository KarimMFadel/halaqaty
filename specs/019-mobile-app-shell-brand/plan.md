# Implementation Plan: Mobile App Shell and UI/UX Modernization

**Branch**: `019-mobile-app-shell-brand` | **Date**: 2026-09-22 | **Spec**: [spec.md](spec.md)  
**Input**: Draft F-019 umbrella specification for Waves 0–5; generated-artifact approval pending

## Summary

Modernize the existing Flutter presentation in six sequential wave batches while
preserving all approved F-001–F-005 behavior. Retain conforming Wave 0 and Wave 1
work, reuse the current Material 3 theme and feature widgets, add shared UI only
after a second concrete use, and verify every changed surface across required
states, Arabic RTL/English LTR, light/dark, accessibility, and responsive widths.

No backend, schema, REST, WebSocket, role, permission, provider, or dependency
change is planned. The HTML sample guides hierarchy only; production values come
from `DESIGN.md`, `UI_UX_GOVERNANCE.md`, and existing theme roles. Intentionally
planned actions without implemented behavior use one shared localized
presentation-only notice and produce no navigation, API/controller call, or
state mutation.

## Technical Context

**Language/Version**: Dart SDK `>=3.4.0 <4.0.0`; Flutter 3.47.4 local, with CI-parity verification through the repository-configured environment where required  
**Primary Dependencies**: Flutter Material 3, Riverpod 2.6, go_router 14.6, dio 5.7, existing Firebase/LiveKit/chat-media packages, flutter_svg  
**Storage**: N/A — no persistence/schema change; existing secure and pending-message state remains owned by F-001–F-005  
**Testing**: `flutter_test`, `integration_test`, existing feature/controller suites, screenshot capture through the existing integration binding  
**Target Platform**: Android and iOS; phone widths 320–599dp and tablet widths 600–1023dp  
**Project Type**: Flutter mobile application in a Go/Flutter monorepo  
**Performance Goals**: Smooth Material transitions; no new blocking work in build methods; loading feedback for waits over 300ms  
**Constraints**: Arabic-first RTL, English LTR, light/dark, WCAG AA, 48dp targets, text scaling, reduced motion, preserved routes/controllers/keys/contracts, no new dependency, no side effects from under-implementation actions  
**Scale/Scope**: Six wave batches covering the shell plus existing auth, profile, circle, session/queue, and chat presentation surfaces

## Constitution Check

*GATE: Product scope passed; generated-artifact approval remains pending.*

| Principle/gate | Result | Evidence |
|---|---|---|
| Product registration | PASS | F-019 is approved as the Waves 0–5 umbrella feature |
| Generated artifact approval | PENDING | `spec.md`, this plan, `tasks.md`, and analysis require Karim's review before production edits |
| Spiritual mission | PASS | Calm, respectful, non-manipulative direction; no engagement mechanics |
| Tech stack | PASS | Existing Flutter/Material/Riverpod/go_router only; no framework/infrastructure change |
| Security invariants | PASS | No auth, authorization, provider, upload, recording, or data-boundary behavior change |
| Audio fidelity | PASS | Open student audio and LiveKit behavior remain governed by frozen decisions/F-003/F-005 |
| Test-first | PASS | Tasks require failing focused tests before production changes and fresh gates before commit |
| MVP/YAGNI | PASS | Existing/native components first; new shared widgets require two consumers; no speculative surface |
| Contract discipline | PASS | No OpenAPI/WebSocket/DB changes; F-001–F-005 contracts remain binding |

**Post-design recheck**: Content checks pass, but generated-artifact approval is
**PENDING**. Research, compatibility model, and contract notes introduce no
constitutional exception or ADR requirement.

## Project Structure

### Documentation (this feature)

```text
specs/019-mobile-app-shell-brand/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/compatibility.md
├── checklists/requirements.md
├── checklists/ui-ux.md
└── tasks.md
```

### Source Code (repository root)

```text
mobile/
├── lib/
│   ├── app/                         # Wave 0 shell, Home, Chats, Welcome
│   │   └── app_locale_controller.dart  # Targeted normalized ar/en state
│   ├── core/
│   │   ├── theme/                   # M3 schemes and typography
│   │   └── design/                  # Shared components used by 2+ screens
│   └── features/
│       ├── auth/presentation/        # Wave 4
│       ├── profile/presentation/     # Wave 4
│       ├── circles/presentation/     # Wave 1 + session entry
│       ├── sessions/presentation/    # Wave 2
│       └── chat/presentation/        # Wave 3
├── test/widget/                       # Wave-focused state/semantics tests
└── integration_test/
    ├── ux_visual_journey_test.dart        # Wave 5 evidence matrix
    └── existing journey tests             # Preserved behavior coverage
```

**Structure Decision**: Keep the existing feature-first Flutter structure. Do
not create a design package, UI framework, parallel navigation layer, or new
localization framework.

## Delivery Strategy

1. **Wave 0 — audit/foundation**: theme, fonts, logo, shell, locale/direction,
   Home, Welcome, state primitives, and preserved keys; fix identified gaps only.
2. **Wave 1 — circles audit**: discovery, detail, join/create, members,
   management, and retirement; expose existing create/join entry points and
   normalize state/token use without changing permissions.
3. **Wave 2 — sessions/queue**: expose the existing circle session
   list/ad-hoc create/start/join flows; restructure hierarchy around connection,
   open audio, current turn, role-specific controls, and recovery.
4. **Wave 3 — chat**: retain the Wave 0 Chats-list loading/empty/retryable/
   offline recovery contract, then modernize group/direct conversation, search,
   reply/pin/delete/read, composer, attachment, voice note, archived, and
   offline presentation without changing F-004. The Chats-list state split is
   not reimplemented in this wave.
5. **Wave 4 — auth/profile**: modernize welcome/forms and group the existing
   profile fields, preferred language, and logout; add no settings capability.
6. **Wave 5 — cross-app audit**: close direction, theme, contrast, text scale,
   semantics/focus, compact/tablet, copy, and screenshot gaps.

Each wave follows red-green-refactor: add/fix the smallest relevant test, observe
the intended failure, make the minimum production change, then run the affected
suite and design reviews before advancing.

## UX Flow Plan

| Journey | Entry | Goal | Tap budget | Recovery |
|---|---|---|---:|---|
| Authenticated navigation | App launch/Home | Any shell destination | 1 | Auth failure returns to welcome; no protected shell leakage |
| Circle access | Home or Circles | Circle details | 1–3 | Load error keeps retry/back path and human copy |
| Create/join circle | Circles | Existing create/join outcome | 3; invite 4 | Invite exception records review/confirmation safety |
| Manage/retire circle | Circle detail/members | Existing authorized outcome | 4–5 | Role/destructive confirmation exception |
| Live recitation | Circle details | List/create/start/join/current queue | ≤3 for join; destructive/create confirmation may exceed | Retryable reconnect retains state; terminal offers safe exit |
| Group conversation | Chats or circle details | Conversation/composer | ≤2 | Draft and pending delivery remain visible offline |
| Authorized direct chat | Circle members | Existing pair conversation | 4 | Exception preserves authorization context; no unified inbox |
| Profile maintenance | Profile tab | Existing form/save | 2 | Inline validation and retained in-context success/failure |

Role gates remain in existing controllers/services; presentation reveals only
actions already authorized by approved feature behavior.

## UI Component and Token Plan

| Surface/pattern | First choice | Rule |
|---|---|---|
| Shell/navigation | Existing `StatefulShellRoute` + M3 `NavigationBar` | Four destinations; selected `secondaryContainer`; label/icon/shape, not color alone |
| Page framing | Existing `Scaffold`, `SafeArea`, scroll widgets | Existing roles; approved phone 4-column/8dp-gutter and tablet 8-column/16dp-gutter grids; no invented max width |
| Actions | Native M3 buttons | Theme-provided 48dp minimum; one dominant action; destructive actions separated |
| Cards/lists | Native `Card`, `ListTile`, existing feature widgets | Existing 12dp card radius/theme roles; no HTML values |
| State treatment | Existing `EmptyStateCard`, `CircleLoadError`, feature status widgets | Extract only after second use; preserve rich retryable/terminal/read-only distinctions |
| Under-implementation notice | One function in existing `halaqaty_components.dart` using native M3 `SnackBar` | Exact localized copy and stable key; hide/replace the prior notice; no navigation, controller/API call, or state mutation |
| Identity rows | Existing profile/member/chat widgets | Promote only when a second real screen has the same contract |
| Typography | Existing Cairo/Poppins bundle and fallback | Frozen role/script mapping below; no Segoe/Inter dependency |
| Status/selection | Existing widgets plus scheme roles | Text/icon/shape accompany color |
| Motion | Existing Material transitions | Purposeful short motion; no hero/long system; honor reduced motion |

### Frozen Typography Mapping

F-019 uses only the bundled families already declared in `pubspec.yaml`:

| M3 role/script | Family/fallback | Weight | Line height |
|---|---|---:|---:|
| English display/headline | Poppins | 400 | Existing M3 role |
| English title/label/button | Poppins SemiBold | 600 (bundled exact) | Existing M3 role |
| English body/supporting copy | Poppins | 400 | Existing M3 role |
| Arabic display/headline | Cairo glyphs through the theme fallback | 400 | At least 1.5 |
| Arabic title/label/button | Cairo Bold through the theme fallback | Request 600; nearest bundled weight 700 | At least 1.5 |
| Arabic body/supporting copy | Cairo glyphs through the theme fallback | 400 | At least 1.5 |
| Mixed Arabic/Latin | Poppins primary with Cairo fallback | 400 for display/headline/body; requested 600 resolves to bundled Poppins 600 and Cairo 700 for emphasis | At least 1.5 when Arabic/Quranic text is present |

`DESIGN.md` mentions Segoe/Inter as general body choices and semantic weight 500
for emphasized roles, but neither those families nor 500-weight files are
bundled. F-019 freezes the existing Poppins/Cairo implementation: emphasized
roles request 600, resolving to bundled Poppins SemiBold 600 and the nearest
bundled Cairo Bold 700. It adds no font dependency or synthesized 500 asset.
Sizes and letter spacing remain the existing M3 roles; screen code may not
invent typography values.

## Responsive, Localization, and Accessibility Plan

- **320–599dp**: one-column, scroll-safe, primary action reachable; no clipping
  at supported text scale.
- **600–1023dp**: use the approved 8-column grid with 16dp gutters. Two-pane is
  permitted only when an existing flow materially benefits and tests cover it;
  no unapproved content max width may be invented.
- **1024dp+**: no new desktop shell; F-016 owns desktop.
- A targeted root `AppLocaleController` owns only normalized `ar`/`en` UI locale.
  It initializes to `ar` for Arabic platform locale, `en` for English, and the
  Arabic-first `ar` fallback for unsupported locales.
- Registration language selection updates it immediately. Successful profile
  load replaces it with normalized `preferred_language`; successful Profile save
  updates it and rebuilds `MaterialApp.router`. Logout restores platform fallback.
- Missing/invalid profile values normalize to `ar`. This adds no language and no
  broad F-012 localization scope.
- Every control exposes name, role, value, selected/enabled state, and logical
  focus order where applicable.
- Dynamic connection, turn, opt-out, send/upload, validation, and save changes
  are announced without repeating high-frequency presence/typing noise.
- Contrast covers text, selected, disabled, error, success, and destructive
  states in light and dark themes.

## State Implementation Plan

Do not build one generic state machine. Preserve each controller's domain states
and map them to a consistent visual grammar:

- Initial waits: branded progress/skeleton after 300ms.
- Empty: explanation plus one useful authorized next action.
- Success: retained selected/connected/saved/delivered state in content.
- Recoverable error: human copy, retry/fallback, preserved context.
- Terminal/read-only: reason and safe exit; unsafe controls unavailable.
- Offline/degraded: last safe context retained; network action explained.
- Intentionally planned but unavailable action: one-tap shared localized notice;
  no fake loading/success, navigation, controller/API call, or product-state
  mutation. Unsupported data and unplanned actions remain omitted.
- Mark a state N/A only when the domain cannot produce it and record why.

## Testing and Evidence Plan

### Per-wave automated coverage

- Update focused widget tests before production changes for each affected state,
  primary action, preserved key, semantics label, direction, and role view.
- Reuse integration journeys for auth, profile, circles, session/queue, chat,
  offline recovery, and role access; add only missing visual-flow coverage.
- Keep controller/API tests unchanged unless a presentation test exposes an
  existing regression; F-019 cannot alter their contracts.
- Add explicit regression coverage that queue state never controls authorized
  student audio publishing.
- Inventory each approved-design action as implemented, intentionally under
  implementation, or unplanned/omitted. Tests for the middle category assert the
  exact Arabic/English notice, stable key, replacement/dismiss behavior, and zero
  navigation/controller/API/state side effects.

### Screenshot matrix

Use the exact matrix in `spec.md` under **Screenshot Acceptance Matrix**: every
affected screen receives all four direction/theme ready-state captures at 390dp,
plus the named state/role variants and 320dp, 600dp, and 200% text captures.
Artifact names include wave, screen, role/state, direction, theme, width, and
text scale.

### Required completion gates

Run from `mobile/` unless stated otherwise:

1. `flutter test test`
2. `flutter test integration_test/` with connected emulator/device and configured backend
3. `flutter analyze`
4. `dart format --set-exit-if-changed .`
5. `git diff --check` from repository root

Unavailable integration prerequisites block the commit; they are not a pass.

## Reviews and Ownership

- **UX Designer**: screen inventory, entry/exit, tap counts, state recovery,
  role behavior, RTL/copy, semantics, and accessibility for each wave.
- **UI Designer**: tokens/components, hierarchy, responsive behavior, light/dark,
  typography, selected/error states, motion, and screenshots.
- **Senior Flutter Mobile Engineer**: TDD implementation within existing app
  boundaries.
- **Tech Lead**: each coherent batch and final diff; Karim retains mandatory
  manual review for any security-sensitive existing flow touched.

## Complexity Tracking

No constitutional violation is planned. No new dependency, abstraction layer,
contract, backend surface, or desktop shell is justified or permitted.
