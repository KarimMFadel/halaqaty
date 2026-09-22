# Halaqaty UI/UX Governance and Screen Modernization

**Status:** Approved project standard
**Date:** 2026-09-21
**Scope:** Flutter mobile UI/UX rules, review ownership, and staged modernization

## 1. Intent and success criteria

Halaqaty should feel calm, modern, trustworthy, and comfortable for repeated
Quran-learning use. The interface must help students, teachers, parents, and
institutions complete important actions with minimal navigation while remaining
Arabic-first, RTL-correct, accessible, and respectful of the product's mission.

Success means:

- The app uses one recognizable visual language across current and future screens.
- The main action is understandable within a few seconds and reachable within
  two to three taps from the app shell whenever the backend flow permits it.
- Every screen has explicit loading, empty, error, success, and offline states.
- Arabic RTL and English LTR are both supported without separate ad-hoc layouts.
- A Flutter implementation cannot introduce arbitrary colors, spacing, fonts,
  navigation patterns, or unreviewed interaction behavior.
- Visual quality is verified with emulator screenshots as well as automated
  tests and static analysis.

## 2. Approved visual direction

The approved personality is **Calm Contemporary**. Its visual language is:

- Warm ivory page surfaces for long-session comfort.
- Restrained emerald for primary actions, progress, focus, and trusted status.
- Soft gold for positive emphasis and the selected bottom-navigation indicator.
- Muted neutrals for secondary information and inactive navigation destinations.
- Spacious, calm layouts with fewer framed boxes and stronger content hierarchy.
- Arabic-first typography using the bundled Cairo family with a deliberate mixed
  script fallback for Latin text.

The reference sample is [screen-samples-a-v2.html](screen-samples-a-v2.html).
It is a visual reference, not a production implementation or a source of
backend data.

### 2.1 Token rules

- Screens consume Material 3 `ColorScheme` roles and shared spacing/type tokens.
- Screen-level hard-coded hex colors are prohibited.
- `secondaryContainer`/`onSecondaryContainer` are the default roles for the
  soft-gold selected navigation indicator and similar low-area emphasis.
- Gold is an accent, not a large page fill or the only status signal.
- Status meaning always includes text, icon, shape, or another non-color cue.
- Shared components are introduced only when reused by at least two screens.
- Touch targets are at least 48dp; spacing must preserve comfortable separation.

## 3. UX principles and measurable rules

### 3.1 Navigation and task completion

- Use the approved four-tab shell: Home, Circles, Chats, Profile.
- Each screen has one dominant primary action; secondary actions are visually
  subordinate.
- Core tasks target a maximum of three taps from the app shell. If a role,
  confirmation, or security boundary requires more, the UX brief records why.
- Every screen has a clear back path and no dead-end state.
- The Home screen prioritizes the next relevant session and the user's circles;
  it is not a speculative analytics dashboard.
- Navigation labels and action text are human-facing Arabic/English copy, never
  raw database identifiers, test fixture names, or provider terms.

### 3.2 State coverage

Each screen's UX brief must define:

- Loading: branded progress or skeleton behavior for waits over 300ms.
- Empty: what is absent, why it matters, and the next useful action.
- Error: safe, human-readable explanation and a recoverable retry/fallback path.
- Success: confirmation that does not require a transient snackbar alone for a
  consequential action.
- Offline/degraded: what remains available and what is temporarily disabled.

### 3.3 Accessibility and localization

- WCAG 2.1 AA contrast: at least 4.5:1 for normal text and 3:1 for large text
  and essential UI graphics.
- Every interactive control has a meaningful semantics label and state.
- Dynamic state changes are announced appropriately to screen readers.
- Arabic is designed RTL-first; directional icons, progress, gestures, and
  ordering are verified in both directions.
- Quranic text and Arabic diacritics must not clip at supported text scales.
- Motion is purposeful, 200–400ms by default, and honors reduced-motion settings.

## 4. Required delivery workflow

The UI/UX gate is part of the existing Spec-Kit lifecycle and does not create a
parallel feature process:

```text
UX brief → UI specification → Flutter implementation → visual/technical review
```

### UX Designer deliverable

For each feature, produce:

- User journey and entry/exit points per affected role.
- Screen inventory with purpose, data, actions, and explicit exclusions.
- Primary action and expected tap count for each core task.
- Loading/empty/error/success/offline state matrix.
- RTL, semantics, copy, and accessibility notes.

### UI Designer deliverable

For each approved screen inventory, produce:

- Component-to-token mapping; no invented values.
- Layout and responsive notes for the supported phone/tablet widths.
- Typography hierarchy and Arabic mixed-script behavior.
- Light/dark treatment, selected/inactive states, and error/empty visuals.
- Motion notes and a concise visual review checklist.

### Senior Flutter Mobile Engineer deliverable

- Implement only the approved flow and visual mapping.
- Reuse shared components and `Theme.of(context).colorScheme` roles.
- Preserve existing widget-test keys and behavior unless a separate UX change is
  approved.
- Add or update widget tests for the defined states and semantics.
- Attach emulator screenshot evidence for the affected RTL/LTR screens.

### Tech Lead review

The reviewer checks:

- Flow and tap-count compliance.
- State coverage and recovery paths.
- Token/component reuse and absence of screen-level magic values.
- Contrast, touch targets, semantics, RTL/LTR, and responsive behavior.
- No backend contract or role behavior was invented by the UI work.

## 5. Modernization roadmap

### Wave 0 — foundation (completed)

Theme, bundled fonts, logo, app shell, shared components, and selected gold
navigation state. Existing work is retained and audited against this document.

### Wave 1 — circles (completed)

Discover, circle list, circle detail, membership actions, human-readable names,
and complete loading/empty/error/offline states.

### Wave 2 — sessions and recitation queue

Upcoming session, join/start session, queue state, teacher controls, and the
student's primary recitation path. This wave receives the strictest tap-count
and live-state review.

### Wave 3 — chat

Circle conversation entry, message states, attachment/voice-note affordances,
and recovery for offline or unavailable media.

### Wave 4 — profile and authentication

Welcome, sign-in/register, profile editing, preferences, and account actions.

### Wave 5 — cross-app audit

Dark mode, RTL/LTR screenshot matrix, screen-reader checks, text scaling,
emulator review, and removal of remaining developer-facing copy or placeholders.

F-019 is the approved umbrella Spec-Kit feature for Waves 0–5, replacing the
previous F-020–F-023 allocation. Each wave remains a coherent implementation and
review batch within F-019. No wave adds backend tables, endpoints, role rules, or
provider capabilities without the normal contract and architecture approvals.

## 6. Definition of done

A screen is complete only when:

- Its UX and UI deliverables exist and are reviewed.
- Its primary task and tap count are recorded.
- All five required states are implemented and tested where applicable.
- Arabic RTL and English LTR have been checked.
- Semantics and 48dp touch targets are present.
- Colors, type, spacing, selection, and motion come from approved tokens/rules.
- Widget tests, Flutter analysis, formatting, and required screenshot evidence
  are fresh for the changed surface.
- The Tech Lead review is complete and any security-sensitive behavior remains
  subject to Karim's required manual review.

## 7. Out of scope

This design does not introduce a new UI framework, runtime theme/plugin system,
backend API, database schema, analytics dashboard, unified direct-message inbox,
or speculative provider controls. It does not replace the existing Spec-Kit
artifacts for individual product features.

## 8. Governance and change control

This document is the permanent UI/UX standard for current and future Halaqaty
screens. `DESIGN.md` remains the token catalogue; this document governs how
those tokens, flows, states, accessibility rules, and agent handoffs are applied.

Material changes to the visual direction, shell navigation, accessibility floor,
required state coverage, or wave boundaries require Karim's approval before the
governing document and affected Spec-Kit artifacts are updated.
