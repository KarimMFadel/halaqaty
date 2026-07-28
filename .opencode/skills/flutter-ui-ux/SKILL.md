# Flutter UI/UX Design Skill

> **Purpose:** Integrate Material Design 3 and responsive Flutter patterns into Halaqaty's Spec-Kit workflow.

---

## Skill Overview

This skill brings **Flutter UI/UX best practices** into your feature development pipeline. It ensures every feature spec includes UI design requirements, responsive patterns, animations, and accessibility standards aligned with `docs/engineering/design/DESIGN.md`.

### When to Use This Skill

- **During `/speckit.specify`** — Ensure feature spec includes UI requirements (user interface, interactions, responsive breakpoints)
- **During `/speckit.plan`** — Design UI architecture (component hierarchy, layout patterns, theme integration)
- **During `/speckit.tasks`** — Include UI implementation tasks (build components, responsive layouts, animations)
- **During `/speckit.implement`** — Execute UI tasks with accessibility and design system compliance
- **During `/speckit.analyze`** — Verify UI consistency against DESIGN.md

---

## Skill Commands

Invoke from VS Code Copilot Chat or GitHub Copilot Agent:

```
/flutter-ui-ux <command> <feature-name>
```

### Available Commands

| Command | Use Case | Output |
|---------|----------|--------|
| `spec-ui-requirements` | During `/speckit.specify` — add UI requirements to feature spec | UI requirements checklist for spec.md |
| `design-architecture` | During `/speckit.plan` — design component layout and patterns | Component hierarchy, responsive breakpoints |
| `responsive-patterns` | Find responsive layout patterns for your feature | LayoutBuilder examples, breakpoint strategy |
| `animation-guide` | Design purposeful animations for your feature | Animation durations, types, micro-interactions |
| `accessibility-audit` | Verify WCAG 2.1 AA compliance for your feature | Contrast check, touch target, RTL, semantics |
| `component-checklist` | Before submitting PR — verify component quality | Design system alignment checklist |
| `design-tokens` | Reference color, spacing, typography tokens | Token values, usage examples |

---

## Integration with Spec-Kit Phases

### Phase 1: `/speckit.specify`
**Include UI Requirements**
```
Run: /flutter-ui-ux spec-ui-requirements <feature-name>

Add to spec.md:
- User interface mockups (Figma link, or ASCII sketch)
- Responsive breakpoints affected (mobile 320px, tablet 600px, desktop 1024px+)
- Button states, card layouts, input fields
- RTL support requirements (Arabic text, directional icons)
- Accessibility requirements (contrast, touch targets, screen readers)
- Animation expectations (which interactions should animate?)
```

### Phase 2: `/speckit.clarify`
**Clarify UI Details**
- Confirm breakpoint handling strategy
- Clarify animation timing and purpose
- Confirm color/typography choices from DESIGN.md
- Ask about Arabic/English text handling if multilingual

### Phase 3: `/speckit.checklist`
**Validate UI Specifications**
```
Run: /flutter-ui-ux component-checklist <feature-name>

Checklist:
- [ ] UI mockups attached or described
- [ ] All breakpoints documented (mobile, tablet, desktop)
- [ ] RTL handling confirmed
- [ ] Accessibility requirements clear (WCAG 2.1 AA)
- [ ] Animation timings specified (100ms–1s range)
- [ ] Colors from DESIGN.md palette
- [ ] Fonts: Cairo (Arabic), Poppins/Inter (English)
```

### Phase 4: `/speckit.plan`
**Design UI Architecture**
```
Run: /flutter-ui-ux design-architecture <feature-name>

Deliverables (add to plan.md):
- Component hierarchy (widget tree structure)
- Layout patterns (stacked, grid, flex)
- State management (StatelessWidget vs StatefulWidget)
- Theme integration (use Material Design 3 ColorScheme)
- Responsive breakpoint strategy
```

### Phase 5: `/speckit.tasks`
**Include UI Implementation Tasks**
```
Run: /flutter-ui-ux responsive-patterns <feature-name>
Run: /flutter-ui-ux animation-guide <feature-name>

Add to tasks.md:
- Task: Build [ComponentName] widget (responsive, accessible)
- Task: Implement responsive layouts (LayoutBuilder for mobile/tablet/desktop)
- Task: Add animations (micro-interactions, 200-400ms durations)
- Task: Accessibility audit (contrast, touch targets, semantics)
```

### Phase 6: `/speckit.analyze`
**Verify UI Consistency**
```
Run: /flutter-ui-ux component-checklist <feature-name>
Run: /flutter-ui-ux accessibility-audit <feature-name>

Check:
- All colors match DESIGN.md palette
- Typography uses Cairo/Poppins only
- Spacing uses 8px grid (4, 8, 12, 16, 24, 32, 48)
- Components follow Material Design 3 standards
- RTL support in place for all layouts
- Accessibility: WCAG 2.1 AA passes
```

### Phase 7: `/speckit.implement`
**Execute with UI Review**
```
Before merging PR:
1. Run: /flutter-ui-ux accessibility-audit <feature-name>
2. Verify:
   - Responsive layout works on 320px, 768px, 1366px widths
   - Dark mode works
   - Arabic locale (RTL, fonts, diacritics) works
   - Animations are smooth (60fps, no jank)
   - Contrast ratio ≥ 4.5:1
   - Touch targets ≥ 48x48dp
3. Tech Lead approval before merge
```

---

## Quick Reference: Design System Tokens

### Colors (from DESIGN.md)

**Primary:** `#1B7E3C` (Islamic Green)
**Secondary:** `#D4A574` (Quranic Gold)
**Success:** `#2E7D32`
**Warning:** `#F57C00`
**Error:** `#C62828`

```dart
// Use Material 3 ColorScheme, not hard-coded colors:
Theme.of(context).colorScheme.primary
Theme.of(context).colorScheme.secondary
Theme.of(context).colorScheme.error
```

### Spacing (8px base unit)

```
xs: 4px
sm: 8px
md: 12px
lg: 16px
xl: 24px
xxl: 32px
xxxl: 48px
```

### Typography

```dart
// Arabic: Cairo
// English: Poppins or Inter

// Use Material 3 TextTheme:
Theme.of(context).textTheme.displayLarge    // 57px, headers
Theme.of(context).textTheme.headlineSmall   // 24px, card titles
Theme.of(context).textTheme.bodyLarge       // 16px, body text
Theme.of(context).textTheme.bodySmall       // 12px, captions
```

### Responsive Breakpoints

```dart
// Mobile: 320–599px
// Tablet: 600–1023px
// Desktop: 1024px+

if (constraints.maxWidth > 1024) {
  // Desktop layout
} else if (constraints.maxWidth > 600) {
  // Tablet layout
} else {
  // Mobile layout
}
```

---

## Accessibility Checklist

Every feature must pass these checks before merge:

- [ ] **Contrast:** Text on background ≥ 4.5:1 (normal) or 3:1 (large)
- [ ] **Touch Targets:** All buttons/interactive elements ≥ 48x48dp
- [ ] **RTL:** Test with Arabic locale — layout flips, icons flip, text reads right-to-left
- [ ] **Arabic Typography:** Cairo font, supports diacritics (ـ ٰ ُ ِ)
- [ ] **Semantics:** Interactive elements have labels: `Semantics(label: "...")`
- [ ] **Screen Reader:** Test with TalkBack (Android) or VoiceOver (iOS)
- [ ] **Dark Mode:** Works correctly with adjusted colors
- [ ] **Animations:** 60fps (no jank), ≤ 400ms duration
- [ ] **Responsive:** Works on 320px (mobile), 768px (tablet), 1366px (desktop)

---

## References

- **DESIGN.md:** `/docs/engineering/design/DESIGN.md` (brand, colors, typography, accessibility standards)
- **Spec-Kit Guide:** `/DEVELOPMENT.md` (7-phase workflow)
- **Material Design 3:** https://m3.material.io/
- **Flutter Widgets:** https://flutter.dev/docs/development/ui/widgets

---

## Example: Using the Skill for a Feature

### Scenario: F-003 Recitation Queue System

**Phase 1: Specify**
```
Run: /flutter-ui-ux spec-ui-requirements recitation-queue

Add to spec.md:
- UI: Queue displays students with status badges (Waiting, Reciting, Completed, Skipped)
- Responsive: Mobile (1 column), Tablet (2 columns), Desktop (sidebar + queue)
- Accessibility: WCAG 2.1 AA, screen reader announcements for status changes
- RTL: Arabic names, RTL layout support
```

**Phase 4: Plan**
```
Run: /flutter-ui-ux design-architecture recitation-queue

Add to plan.md:
- Components: QueueStudentCard, QueueList
- Layout: Mobile stacked, tablet grid, desktop sidebar
- Animations: Slide when status changes (300ms)
- State: GetX for queue state management
```

**Phase 5: Tasks**
```
Run: /flutter-ui-ux responsive-patterns recitation-queue
Run: /flutter-ui-ux animation-guide recitation-queue

Add to tasks.md:
- Build QueueStudentCard (responsive, accessible)
- Implement QueueList with LayoutBuilder
- Add SlideTransition animations
- Accessibility audit
```

---

## Contributing

Found a better pattern? New accessibility insight? Please update this file and create a PR.

