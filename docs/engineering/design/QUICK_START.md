# Flutter Design System — Quick Start for Teams

> **Share this with your team!**

---

## 📋 What Changed

✅ **DESIGN.md** — System design with Islamic aesthetics, colors, typography, accessibility
✅ **Flutter UI/UX Skill** — Commands for each Spec-Kit phase
✅ **Age range updated** — Ages 10–60 (realistic for your platform)
✅ **UI + Spec-Kit integrated** — No separate UI process (embedded at each phase)

---

## 🚀 Start Your First Feature

```bash
Step 1:
/speckit.specify authentication

Step 2:
/flutter-ui-ux spec-ui-requirements authentication

Step 3: Follow Spec-Kit phases 1–7
(use /flutter-ui-ux commands at each phase)

Step 4: Before merge, run
/flutter-ui-ux accessibility-audit authentication
```

---

## 🎨 Color Palette (Memorize These)

```
Primary (Use most):      #1B7E3C (Islamic Green)
Secondary (Accents):     #D4A574 (Quranic Gold)
Success:                 #2E7D32 (Green)
Warning:                 #F57C00 (Orange)
Error:                   #C62828 (Red)
```

---

## 📝 Fonts

- **Arabic:** Cairo (must support diacritics)
- **English:** Poppins or Inter

---

## 📱 Responsive Breakpoints

- **Mobile:** 320–599px (stacked layout, bottom nav)
- **Tablet:** 600–1023px (two-column, side drawer)
- **Desktop:** 1024px+ (three-column, sidebar)

---

## ✅ Pre-Merge Checklist (Copy & Paste)

```
[ ] Contrast ≥ 4.5:1
[ ] Touch targets ≥ 48×48dp
[ ] Responsive: 320px, 768px, 1366px widths
[ ] Dark mode works
[ ] Arabic (RTL, Cairo font, diacritics)
[ ] Animations ≤ 400ms, 60fps
[ ] Semantic labels for screen readers
[ ] Colors from DESIGN.md only
[ ] Typography: Cairo or Poppins only
[ ] Spacing: 8px grid (4, 8, 12, 16, 24, 32, 48)
```

---

## 📁 Where to Find Things

| Item | Location |
|------|----------|
| **System Design** | `docs/engineering/design/DESIGN.md` |
| **Skill Commands** | `.github/skills/flutter-ui-ux.md` |
| **This Guide** | `docs/engineering/design/IMPLEMENTATION_GUIDE.md` |
| **Spec-Kit Workflow** | `DEVELOPMENT.md` |

---

## 💡 Key Skill Commands

```bash
/flutter-ui-ux spec-ui-requirements <feature>
/flutter-ui-ux design-architecture <feature>
/flutter-ui-ux accessibility-audit <feature>
/flutter-ui-ux component-checklist <feature>
```

---

## 🎯 Phase 8: UI Polish (End of Project)

Before launch:
- Audit all colors match DESIGN.md
- Audit all fonts are Cairo/Poppins
- Audit all spacing is 8px grid
- Audit responsive on 3 breakpoints
- Audit accessibility WCAG 2.1 AA
- Audit animations ≤ 400ms, 60fps

---

**Questions?** Read `IMPLEMENTATION_GUIDE.md` or `DESIGN.md`

