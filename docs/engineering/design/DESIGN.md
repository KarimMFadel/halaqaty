# Halaqaty Design System

> **Our Design Philosophy:** *Islamic-first, spiritually-minded, modern, accessible, and purposeful.*

Built on **Material Design 3** with Halaqaty's unique brand for Quran memorization circles (حلقات تحفيظ القرآن).

---

## 🎨 Brand Identity

### Core Values
- **Spiritual & Respectful** — Honors Islamic education traditions
- **Clear & Purposeful** — Every design decision serves Quran memorization
- **Accessible to All** — Works for Arabic, English; screen readers; low-vision users
- **Culturally Sensitive** — RTL-first, supports Arabic typography
- **Performance-First** — Responsive from 320px phone to 1920px desktop

### Target Audience
- 🧒 **Students** (ages 10–60) — Learning to memorize Quran
- 👨‍🏫 **Teachers** — Managing circles and tracking progress
- 👥 **Parents** — Monitoring their children's progress
- 🏢 **Institutions** — Managing multiple circles

---

## 🌈 Color Palette

### Primary Colors
| Name | Hex | Usage |
|------|-----|-------|
| **Primary (Islamic Green)** | `#1B7E3C` | Buttons, accents, CTA, progress indicators |
| **Primary Light** | `#4CB368` | Hover states, secondary highlights |
| **Primary Dark** | `#0F5627` | Focus states, dark mode primary |
| **On Primary** | `#FFFFFF` | Text on primary background |

**Rationale:** Green is sacred in Islamic tradition. The shade `#1B7E3C` balances professionalism with cultural significance.

### Secondary Colors
| Name | Hex | Usage |
|------|-----|-------|
| **Secondary (Quranic Gold)** | `#D4A574` | Supporting accents, badges, highlights |
| **Secondary Light** | `#E8C8A0` | Hover states for secondary actions |
| **Secondary Dark** | `#8B6F47` | Focus states, dark mode secondary |
| **On Secondary** | `#FFFFFF` | Text on secondary background |

**Rationale:** Gold reflects the value of Quran memorization. Used sparingly for achievements, badges, and celebrations.

### Semantic Colors
| Purpose | Light Mode | Dark Mode | Usage |
|---------|-----------|----------|-------|
| **Success** | `#2E7D32` | `#66BB6A` | Completed recitations, passed rounds |
| **Warning** | `#F57C00` | `#FFB74D` | Attention needed, incomplete tasks |
| **Error** | `#C62828` | `#EF5350` | Failed attempts, connection issues |
| **Info** | `#0277BD` | `#29B6F6` | Session info, helpful tooltips |
| **Neutral** | `#616161` | `#BDBDBD` | Disabled state, secondary text |

### Neutral Palette
| Name | Light | Dark | Usage |
|------|-------|------|-------|
| **Surface** | `#FFFFFF` | `#121212` | Card backgrounds, main content |
| **Surface Variant** | `#F5F5F5` | `#2C2C2C` | Secondary surfaces, sections |
| **Background** | `#FAFAFA` | `#0A0A0A` | App background |
| **Outline** | `#79747E` | `#928F96` | Borders, dividers |
| **Outline Variant** | `#CAC7D0` | `#49454E` | Secondary borders |

---

## 📝 Typography

### Typefaces
- **Primary (Body & UI):** `Segoe UI` (Android), `Inter` (fallback for web)
- **Arabic (Body & UI):** `Cairo` or `Droid Arabic Naskh` (must support Arabic diacritics `ـ ٰ ُ ِ`)
- **English Headings:** `Poppins` (modern, friendly)
- **Arabic Headings:** `Cairo Bold` (clear hierarchy)

**Why:** Arabic typography must support Quranic diacritics. `Cairo` is widely available and readable at small sizes.

### Type Scale

| Role | Size | Weight | Line Height | Letter Spacing | Usage |
|------|------|--------|-------------|----------------|-------|
| **Display Large** | 57px | 400 | 64px | -0.25px | App title, hero sections |
| **Display Medium** | 45px | 400 | 52px | 0px | Feature titles |
| **Display Small** | 36px | 400 | 44px | 0px | Page titles |
| **Headline Large** | 32px | 400 | 40px | 0px | Circle name, section headers |
| **Headline Medium** | 28px | 400 | 36px | 0px | Subsection headers |
| **Headline Small** | 24px | 400 | 32px | 0px | Card titles, modal headers |
| **Title Large** | 22px | 500 | 28px | 0px | Strong emphasis, labels |
| **Title Medium** | 16px | 500 | 24px | 0.15px | Card subtitles, buttons |
| **Title Small** | 14px | 500 | 20px | 0.1px | Small labels, chips |
| **Body Large** | 16px | 400 | 24px | 0.5px | Main body text, descriptions |
| **Body Medium** | 14px | 400 | 20px | 0.25px | Secondary body, form inputs |
| **Body Small** | 12px | 400 | 16px | 0.4px | Captions, helper text, footnotes |
| **Label Large** | 14px | 500 | 20px | 0.1px | Button text, labels |
| **Label Medium** | 12px | 500 | 16px | 0.5px | Tags, badges, small labels |
| **Label Small** | 11px | 500 | 16px | 0.5px | Micro labels, version numbers |

**RTL Considerations:**
- Ensure `letter-spacing` is mirrored for Arabic text
- Use proper font features for Arabic diacritics
- Test with Arabic text at all sizes (diacritics should not clip)

---

## 🎯 Spacing & Layout

### Spacing Scale (8px base unit)
```
4px    → Extra tight (x-small gaps between elements)
8px    → Tight (default padding inside small components)
12px   → Comfortable (padding inside medium components)
16px   → Generous (default padding, card spacing)
24px   → Large (section spacing)
32px   → XL (major section breaks)
48px   → 2XL (top-level screen padding)
```

### Grid System
- **Desktop (1024px+):** 12-column grid, 24px gutters
- **Tablet (600px–1023px):** 8-column grid, 16px gutters
- **Mobile (320px–599px):** 4-column grid, 8px gutters

### Breakpoints
| Device | Width | Breakpoint |
|--------|-------|-----------|
| Mobile | 320–599px | `mobile` |
| Tablet | 600–1023px | `tablet` |
| Desktop | 1024px+ | `desktop` |

---

## 🧩 Component Library

### Button States
- **Enabled** — Full opacity, interactive cursor
- **Hovered** — 8% darker tint, elevation lift
- **Pressed** — Primary color overlay
- **Disabled** — 38% opacity, no interaction
- **Loading** — Spinner overlay, disabled state

### Button Variants
1. **Filled (Primary)** — Main CTA ("Start Recitation", "Save")
2. **Outlined** — Secondary actions ("Cancel", "Skip")
3. **Elevated** — Tertiary actions with subtle depth
4. **Text** — Lowest priority ("Learn More", "Dismiss")
5. **Icon** — For floating action buttons (FAB)

### Card Design
- **Corner Radius:** 12px (Material Design 3 standard)
- **Elevation:** 1dp (subtle shadow)
- **Padding:** 16px
- **Divider:** `Outline Variant` color when grouping content

### Input Fields
- **Border Radius:** 8px
- **Focus State:** Green border (`#1B7E3C`), error red for validation
- **Helper Text:** `Body Small`, `Outline` color
- **Placeholder:** Gray (70% opacity)
- **Label:** Above input, `Title Small` weight

### Navigation
- **Bottom Navigation Bar** — Up to 5 items (Mobile)
- **Side Navigation** — Desktop view
- **Tab Bar** — For category switching (e.g., "Waiting", "Reciting", "Completed")

---

## ♿ Accessibility Standards (WCAG 2.1 AA)

### Color Contrast
- **Minimum:** 4.5:1 for normal text (14px+)
- **Minimum:** 3:1 for large text (18px+ or 14px bold+)
- **Minimum:** 3:1 for graphics and UI components

### Touch Targets
- **Minimum:** 48x48dp (Material Design standard)
- **Recommended:** 56x56dp (easier for children)
- **Spacing:** At least 8px between interactive elements

### Text & Readability
- **Minimum Font Size:** 12px (body text)
- **Recommended Font Size:** 14px (optimal for all ages)
- **Line Height:** 1.5 minimum (better for Arabic diacritics)
- **Letter Spacing:** 0.15px minimum for clarity

### RTL Support
- All layouts must work in RTL and LTR
- Directional icons must flip (e.g., back arrow)
- RTL mirrors all horizontal layouts automatically in Flutter
- Test with `Localizations.localeOf(context).languageCode == 'ar'`

### Screen Reader (Semantic)
- All interactive elements must have labels: `Semantics(label: "Add student")`
- Form inputs must have labels (not just placeholders)
- Announce dynamic changes properly

### Motion
- No auto-playing videos
- Animations duration: 200–400ms (respectful pace)
- Never use strobing or flashing (risk of seizures)

---

## ✨ Animation Guidelines

### Principles
- **Purposeful** — Every animation explains or clarifies interaction
- **Respectful** — No excessive motion; honors Islamic sensibilities
- **Performant** — 60fps minimum; no jank on mid-range devices
- **Consistent** — Same type of animation for similar interactions

### Animation Durations
| Type | Duration | Use Case |
|------|----------|----------|
| **Micro** | 100ms | Button press, checkbox toggle |
| **Short** | 200–300ms | Fade in/out, small transitions |
| **Medium** | 400–500ms | Screen transitions, navigation |
| **Long** | 800ms–1s | Hero animations, loading states |

### Animation Types
1. **Fade In/Out** — Content appearing/disappearing
2. **Slide** — Modal entry/exit, card transitions
3. **Scale** — Button presses, expand/collapse
4. **Rotate** — Loading spinner, icon state change
5. **Hero** — Shared element transition between screens

---

## 🌙 Dark Mode

### Palette Adjustments
- **Primary** → Lighter shade (`#4CB368`)
- **Surface** → `#121212` (true black reduces OLED burn-in)
- **Text** → `#FFFFFF` (white, not gray)

---

## 📱 Responsive Breakpoints

### Mobile-First Approach
1. **Design for 320px first** (smallest phones)
2. **Scale up** to 600px (tablet)
3. **Optimize** for 1024px+ (web, desktop)

---

## 🔧 Implementation Rules

### All New Components Must:
- [ ] Pass WCAG contrast check
- [ ] Have minimum 48dp touch target
- [ ] Support RTL (test with Arabic locale)
- [ ] Work on all 3 breakpoints
- [ ] Include semantic labels for screen readers
- [ ] Animate smoothly (no jank) at 60fps

---

## 📚 References

### Material Design 3
- [Material 3 Spec](https://m3.material.io/)
- [Flutter Material Widgets](https://flutter.dev/docs/development/ui/widgets/material)

### Accessibility
- [WCAG 2.1 Guidelines](https://www.w3.org/WAI/WCAG21/quickref/)
- [Flutter Accessibility](https://flutter.dev/docs/development/accessibility-and-localization/accessibility)

