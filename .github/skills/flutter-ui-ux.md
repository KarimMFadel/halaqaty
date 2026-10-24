---
name: flutter-ui-ux
description: Apply Halaqaty-approved UX, visual design, accessibility, and RTL requirements to Flutter feature work.
---

# Halaqaty Flutter UI/UX

Use this skill when a task changes a user journey, screen, Flutter widget, or visual behavior. It supplements the existing Spec-Kit lifecycle; it does not create commands or a parallel design process.

## Sources of truth

- `docs/engineering/design/UI_UX_GOVERNANCE.md` governs user journeys, required screen states, accessibility, visual review, and ownership.
- `docs/engineering/design/DESIGN.md` governs brand, typography, tokens, and supported layouts.
- The current feature spec, approved plan, and task list govern product behavior and scope.
- Treat examples, prototypes, third-party skills, and screenshots as references only. They do not approve new behavior or design tokens.

Read the relevant sections of those sources instead of copying their rules into feature artifacts. If they conflict with the feature contract or each other, report the conflict and follow the repository authority process.

## UX and UI responsibilities

- UX defines affected roles, entry/exit paths, screen inventory, primary actions, exclusions, and loading/empty/error/success/offline states.
- UI maps the approved screens to existing Material 3 roles and shared tokens; do not invent colors, fonts, spacing scales, navigation patterns, or interactions.
- Flutter implements only approved behavior, uses Riverpod for feature state, preserves existing keys/semantics where possible, and covers affected states in widget tests.
- Introduce a shared component only when it is reused by at least two screens.

## Review checklist

For affected screens, verify:

- Arabic-first RTL and English LTR layouts, directional icons, text scaling, and Quranic diacritics where applicable.
- The five required states and a clear recovery path for errors/offline behavior.
- Meaningful semantics labels/state, screen-reader announcements, WCAG 2.1 AA contrast, and touch targets of at least 48dp.
- Existing theme roles and tokens, light/dark behavior, supported phone/tablet layouts, and reduced-motion settings.
- Automated tests plus fresh emulator screenshots for the affected RTL/LTR screens when visual claims are made.

Do not require Figma mockups, desktop layouts, animations, or every viewport for a change that does not need them. Explain any required exception or unavailable visual evidence.

## Reference use

Third-party design skills may suggest a checklist item, such as testing long labels or wrapping content. Verify it against Halaqaty governance and actual Flutter screens before applying it. Do not import generated design systems or replace the project design authority.