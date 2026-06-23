# Flutter UI Design System — Implementation Guide

> **Status:** Ready | **Created:** 2026-06-23

---

## ✅ What Was Completed

### 1. Age Range Updated
- Updated to **(ages 10–60)** in all documents
- Reflects realistic user demographic for Quran memorization platform

### 2. Flutter UI/UX Skill Created
- Location: `.github/skills/flutter-ui-ux.md`
- 7 commands for Spec-Kit phases
- Embedded UI design into your workflow (no separate process)

### 3. DESIGN.md Created
- Location: `docs/engineering/design/DESIGN.md`
- **Colors:** Islamic Green `#1B7E3C`, Quranic Gold `#D4A574`
- **Typography:** Cairo (Arabic), Poppins/Inter (English)
- **Spacing:** 8px base grid
- **Accessibility:** WCAG 2.1 AA standards
- **Responsive:** 3 breakpoints (320px, 600px, 1024px+)

### 4. UI Design Integrated with Spec-Kit
- UI design at **each of your 7 Spec-Kit phases**
- **Phase 8 (NEW):** UI Polish — Final validation before launch
- **Avoids:** AI hallucination, inconsistent designs, conflicting approaches

---

## 🚀 How to Use

### For Each Feature:

```
Phase 1 (Specify):
  /speckit.specify <feature-name>
  /flutter-ui-ux spec-ui-requirements <feature-name>
  → Add UI mockups, responsive breakpoints, RTL requirements

Phase 2 (Clarify):
  /speckit.clarify
  → Ask UI questions: colors? animations? responsive? RTL?

Phase 3 (Checklist):
  /speckit.checklist
  /flutter-ui-ux component-checklist <feature-name>
  → Validate UI spec is complete

Phase 4 (Plan):
  /speckit.plan
  /flutter-ui-ux design-architecture <feature-name>
  → Design component hierarchy, responsive strategy

Phase 5 (Tasks):
  /speckit.tasks
  /flutter-ui-ux responsive-patterns <feature-name>
  /flutter-ui-ux animation-guide <feature-name>
  → Create UI implementation tasks

Phase 6 (Analyze):
  /speckit.analyze
  /flutter-ui-ux accessibility-audit <feature-name>
  → Verify WCAG 2.1 AA, responsive, RTL compliance

Phase 7 (Implement):
  /speckit.implement
  /flutter-ui-ux accessibility-audit <feature-name>
  → Build UI + tests, verify before merge

Phase 8 (UI Polish — End of Project):
  Audit all features against DESIGN.md
  Final validation before launch
```

---

## 📋 Skill Commands Quick Reference

| Command | Phase | Purpose |
|---------|-------|---------|
| `spec-ui-requirements` | 1 | Add UI requirements to spec |
| `design-architecture` | 4 | Design component layout |
| `responsive-patterns` | 5 | Responsive layout examples |
| `animation-guide` | 5 | Animation duration/types |
| `accessibility-audit` | 6–7 | WCAG 2.1 AA verification |
| `component-checklist` | 3/6 | Quality validation |
| `design-tokens` | Any | Reference colors, spacing, fonts |

---

## 🎨 Design System Quick Facts

| Item | Value |
|------|-------|
| **Primary Color** | Islamic Green `#1B7E3C` |
| **Secondary Color** | Quranic Gold `#D4A574` |
| **Arabic Font** | Cairo |
| **English Font** | Poppins or Inter |
| **Spacing Base** | 8px (4, 8, 12, 16, 24, 32, 48) |
| **Mobile Breakpoint** | 320–599px |
| **Tablet Breakpoint** | 600–1023px |
| **Desktop Breakpoint** | 1024px+ |
| **Minimum Contrast** | 4.5:1 (normal text) |
| **Minimum Touch Target** | 48×48dp |
| **Animation Speed** | 100ms–400ms (respectful) |

---

## ✅ Pre-Merge Accessibility Checklist

Before submitting any UI PR:

- [ ] **Contrast:** ≥ 4.5:1 (use [WCAG checker](https://webaim.org/resources/contrastchecker/))
- [ ] **Touch Targets:** ≥ 48×48dp
- [ ] **Responsive:** Tested on 320px (mobile), 768px (tablet), 1366px (desktop)
- [ ] **Dark Mode:** Works correctly
- [ ] **Arabic Locale:** RTL layout, Cairo font, diacritics render
- [ ] **Animations:** ≤ 400ms, 60fps (no jank)
- [ ] **Semantics:** Screen reader labels added (`Semantics(label: "...")`)
- [ ] **Colors:** From DESIGN.md palette (not ad-hoc)
- [ ] **Typography:** Cairo or Poppins only (not random fonts)
- [ ] **Spacing:** 8px grid (not magic numbers)

---

## 📁 Key Files

| File | Purpose |
|------|---------|
| `docs/engineering/design/DESIGN.md` | **System design** — colors, typography, spacing, accessibility rules |
| `.github/skills/flutter-ui-ux.md` | **Skill commands** — UI design at each Spec-Kit phase |
| `DEVELOPMENT.md` | **Spec-Kit workflow** — 7-phase process |

---

## 🎯 Project Timeline with UI Design

| Phase | Timeline | Features | UI Design Gate |
|---|---|---|---|
| **Phase 1** | Month 1–2 | Auth (F-001), Circles (F-002) | Setup theme.dart, colors.dart |
| **Phase 2** | Month 3–5 | Queue (F-003), Chat (F-004), Sessions (F-005) | Build component library |
| **Phase 3** | Month 6–8 | Tracking (F-007), Dashboards (F-010) | Responsive dashboard layouts |
| **Phase 4** | Month 9–10 | Mushaf (F-009), Flutter Web | Web optimization |
| **Phase 5** | Month 11–12 | AI (F-013, F-014), Institutional (F-017) | Advanced UI features |
| **Phase 8** | Month 12.5 | All Features | **UI Polish — Final validation** |

---

## 💡 Pro Tips

### Use Design Tokens (Not Magic Values)
```dart
// ✅ Good
padding: 16,  // From DESIGN.md spacing grid
color: Theme.of(context).colorScheme.primary,

// ❌ Avoid
padding: EdgeInsets.all(24),
color: Color(0xFF1B7E3C),
```

### Always Test RTL (Arabic)
```dart
// Test with Arabic locale
Directionality(textDirection: TextDirection.rtl, ...)

// Verify:
// - Layout flips horizontally
// - Icons flip (back arrow, etc.)
// - Arabic fonts render with diacritics
// - Screen reader works with RTL
```

### Animation Best Practices
- **Button press:** 100ms scale
- **Fade in/out:** 200–300ms
- **Screen transitions:** 300–400ms
- **Loading spinners:** 1s loop
- **Never:** >500ms (too slow for users)

### Common Responsive Patterns
```dart
// Mobile (320–599px)
Single-column stacked layout
Bottom navigation bar

// Tablet (600–1023px)
Two-column grid
Side drawer for navigation

// Desktop (1024px+)
Three-column layout
Full sidebar navigation
```

---

## ❓ Common Questions

**Q: Where do I find color values?**
A: `docs/engineering/design/DESIGN.md` or run `/flutter-ui-ux design-tokens`

**Q: What if I need a color not in DESIGN.md?**
A: Update DESIGN.md first, then use the new token (avoid ad-hoc colors)

**Q: How do I test responsive layout?**
A: Flutter Device Preview or test on 3 breakpoints: 320px, 768px, 1366px

**Q: Does Arabic need special handling?**
A: Yes — Cairo font, RTL layout flip, verify diacritics, test TalkBack screen reader

**Q: Which animation duration should I use?**
A: 100ms (micro), 200–300ms (short), 300–400ms (medium). Never >500ms.

**Q: Can I use a different color scheme?**
A: No. Colors are locked to DESIGN.md (Islamic Green + Quranic Gold). This ensures consistency across all features.

---

## 🔗 Resources

- **DESIGN.md:** `docs/engineering/design/DESIGN.md`
- **Skill:** `.github/skills/flutter-ui-ux.md`
- **Material Design 3:** https://m3.material.io/
- **Flutter Accessibility:** https://flutter.dev/docs/development/accessibility-and-localization/accessibility
- **WCAG 2.1:** https://www.w3.org/WAI/WCAG21/quickref/
- **Spec-Kit Guide:** `DEVELOPMENT.md`

---

## 🎓 Getting Started

### For New Team Members:
1. Read this file (you are here! ✓)
2. Read `docs/engineering/design/DESIGN.md` (system design)
3. Review `.github/skills/flutter-ui-ux.md` (skill commands)
4. Reference during feature implementation

### For Next Feature:
```bash
/speckit.specify <feature-name>
/flutter-ui-ux spec-ui-requirements <feature-name>
# ... follow 7 phases with UI design at each phase
```

### For Tech Lead / QA:
- Verify all colors from DESIGN.md (no ad-hoc colors)
- Verify responsive tested on 3 breakpoints
- Verify WCAG 2.1 AA accessibility checks passed
- Verify animations ≤ 400ms, 60fps
- Before Phase 7 merge: `/flutter-ui-ux accessibility-audit`

---

## 📞 Support

- **Design system questions?** → Read `DESIGN.md`
- **Spec-Kit workflow?** → Read `DEVELOPMENT.md`
- **UI design at phases?** → Run `/flutter-ui-ux` commands
- **Accessibility help?** → Run `/flutter-ui-ux accessibility-audit`

---

<div align="center">

**Ready to build beautiful, accessible Quran memorization UI! 🕌**

</div>

