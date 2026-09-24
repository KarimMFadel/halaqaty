# UI/UX Requirements Checklist: Mobile App Shell and UI/UX Modernization

**Purpose**: Unit-test the written requirements before technical planning  
**Created**: 2026-09-22  
**Feature**: [spec.md](../spec.md)

## Scope and Traceability

- [x] CHK001 Are Waves 0–5 explicitly contained in F-019 as one lifecycle with coherent wave-level batches?
- [x] CHK002 Is every wave mapped to screens, affected roles, and explicit exclusions?
- [x] CHK003 Is the HTML sample constrained to visual hierarchy rather than product behavior?
- [x] CHK004 Are F-001–F-005 behavior and contracts identified as unchanged authorities?
- [x] CHK005 Are Wave 0 and Wave 1 treated as preserve-and-audit work rather than automatic rewrites?

## Flow and Navigation

- [x] CHK006 Is the four-tab shell fixed and are core task tap counts measurable?
- [x] CHK007 Does every affected screen require one primary action and a clear exit/back path?
- [x] CHK008 Are exceptions above three taps limited to role, confirmation, or security boundaries?
- [x] CHK009 Is Home prevented from becoming an unsupported analytics or session-data surface?
- [x] CHK010 Is the unified direct-message inbox explicitly excluded?
- [x] CHK026 Are intentionally planned but unimplemented actions distinguished from unplanned prototype actions?

## States and Recovery

- [x] CHK011 Are loading, empty, success, error, and offline/degraded states required per wave?
- [x] CHK012 Are waits over 300ms and spinner-only loading addressed?
- [x] CHK013 Are recoverable and terminal errors distinguishable with safe next actions?
- [x] CHK014 Must consequential success remain visible beyond a transient snackbar?
- [x] CHK015 Are draft/context preservation and realtime duplicate edge cases covered?

## Accessibility and Localization

- [x] CHK016 Are Arabic RTL and English LTR direction, icons, order, dialogs, and gestures required?
- [x] CHK017 Are WCAG AA text/UI contrast thresholds stated for light and dark themes?
- [x] CHK018 Are 48x48dp controls, semantics, dynamic announcements, and focus behavior required?
- [x] CHK019 Are increased text scale, compact width, Arabic diacritics, and reduced motion covered?
- [x] CHK020 Is human-facing copy required to exclude raw IDs, provider names, fixtures, and diagnostics?

## Compatibility and Testability

- [x] CHK021 Are existing routes, controllers, API clients, widget keys, semantics, and localization patterns preserved?
- [x] CHK022 Are widget, integration, screenshot, analysis, format, and diff verification expectations explicit?
- [x] CHK023 Are screenshot variants stated for direction, theme, text scale, and compact width?
- [x] CHK024 Are new dependencies, frameworks, runtime theming, and speculative abstractions excluded?
- [x] CHK025 Can every success criterion be checked from user-visible behavior or current verification evidence?
- [x] CHK027 Does the shared under-implementation notice have exact localized copy, a stable key, accessible dismissal, replacement behavior, and a zero-side-effect requirement?

## Clarification Outcome

- [x] No material ambiguity remains for planning.
- [x] HTML-vs-token precedence is resolved in favor of production tokens and accessibility.
- [x] Existing background roles, Cairo/Poppins mapping, mobile/tablet shell, and short-motion policy are frozen for F-019.

## Notes

- The checklist passed after `/speckit.clarify` and was revalidated after the
  plan, task list, per-screen state matrix, tap budgets, and screenshot matrix
  were completed.
- UX Designer review confirmed flow, locale ownership, state, tap-budget, and
  task traceability alignment.
- UI Designer concerns about token precedence, typography weights and
  fallbacks, desktop exclusion, responsive layouts, and motion were resolved
  without expanding product scope.
- Final UI review confirmed the implementable bundled-font mapping: regular
  roles request 400; emphasis requests 600, resolving to Poppins 600 and the
  nearest bundled Cairo 700, with no new or synthesized 500 asset.
- Final UX and UI reviews reported no remaining blocking or high-severity
  cross-artifact inconsistencies.
