# Wave 5 — Cross-App Audit Evidence

**Scope:** US6 / T043–T046 on `019-mobile-app-shell-brand`. This record is
limited to the Wave 5 audit paths; Wave 3 evidence and its in-progress changes
are deliberately excluded.

## Result summary

- The focused cross-app audit is implemented in
  `mobile/test/widget/wave5/cross_app_surfaces_audit_test.dart` (15 grouped
  widget tests covering the specified direction, target, semantic, typography,
  notice-only, and responsive assertions).
- The retained complete widget run passed **114/114**; see
  `log-test-wave5.txt`.
- The off-device screenshot harness passed **5/5** in 22 seconds; see
  `log-test-wave5-capture.txt`.
- `flutter analyze` reported no issues; see `log-analyze-wave5.txt`.
- No API, controller, model, backend contract, migration, or product capability
  was added. The only new planned action is Circle Detail's Schedule row, which
  invokes the shared localized under-implementation notice and has regression
  coverage for zero navigation/state/controller/API side effects.

## Screenshot acceptance matrix

`mobile/tool/wave5_visual_capture_test.dart` rasterizes the representative
Wave 0–4 surfaces at 2x pixel ratio. It produced **68 PNGs** under
`evidence/screenshots/wave5/`:

| Representative surface/state | Direction/theme | Responsive variants | Artifact prefix |
|---|---|---|---|
| Authenticated Home (loaded) | Arabic/light, English/dark | 320dp, 600dp, 200% text | `wave5_home_loaded_` |
| Circle discovery (loaded) | Arabic/light, English/dark | 320dp, 600dp, 200% text | `wave5_discovery_loaded_` |
| Circle Detail (student) | Arabic/light, English/dark | 320dp, 600dp, 200% text | `wave5_circle_detail_student_` |
| Student turn and manager grading | Arabic/light, English/dark | 320dp, 600dp, 200% text | `wave5_session_student_turn_`, `wave5_session_manager_grading_` |
| Group conversation | Arabic/light, English/dark | 320dp, 600dp, 200% text | `wave5_chat_conversation_` |
| Login, registration ready/validation, profile | Arabic/light, English/dark | 320dp, 600dp, 200% text | `wave5_login_ready_`, `wave5_register_`, `wave5_profile_ready_` |
| Corrected schedule notice and RTL chevrons | Arabic/light and English/light where applicable | 390dp x 844dp (Circle Detail: 390dp x 1600dp) | `wave5_circle_detail_`, `wave5_home_chevron_`, `wave5_chats_chevron_` |
| Keyboard focus | Arabic/light, English/light | 390dp x 844dp | `wave5_profile_save_focus_` |

Normal ready-state light/dark and RTL/LTR captures for the pre-existing Waves
0–4 remain in their respective `evidence/screenshots/wave*/` directories.
Wave 5 adds their previously deferred compact-width and 200% text-scale
coverage, plus the corrected audit findings above.

## Findings resolved by T045

1. Set shared 48dp minimum targets for elevated, text, and icon buttons.
2. Removed manually reversed chevrons so Material mirrors each directional icon
   once in RTL.
3. Made terminal chat access loss honest and safely escapable; removed mutation
   affordances from archived/read-only chats.
4. Replaced the planned Schedule affordance with the shared notice-only action;
   it has no side effect beyond displaying localized feedback.
5. Used theme text roles for error copy and corrected the message-delete icon
   contrast role.

## Visual inspection

Manual artifact inspection covered the Arabic/light Profile at 200% text scale
and the English/dark compact conversation. The profile remains vertically
scrollable with its save action reachable, and the conversation preserves
legible bubbles, labels, and composer controls. No clipping or color-only
meaning was observed in those inspected captures.

## T047 — UX Designer / UI Designer final review

Both roles inspected the complete 68-PNG matrix (all direction/theme,
320dp/600dp, and 200% text variants, plus the notice, chevron, and focus
captures). Verdict on first pass: **APPROVE-WITH-FIXES**, two findings:

1. **UI-W5-1 (raw database values in copy)**: Circle Detail's language row and
   the discovery card's Language/Audience lines rendered raw codes (`ar`,
   `mixed`) in one or both locales, violating the no-raw-identifiers rule.
   Fixed minimally: `circleLanguageLabel()` in `circle_ui_labels.dart` maps
   `ar`/`en` to localized names (unknown codes fall back to the code);
   discovery's `_genderText` now localizes the English branch as well.
2. **UX-W5-1 / UI-W5-2 (invisible keyboard focus)**: the focused primary button
   showed only a ~10% state-layer lightening — indistinguishable in capture and
   short of WCAG 2.4.7. Fixed at theme level per DESIGN.md's focus-state token:
   all four button themes gain a 2dp focused border (light `#0F5627`
   Primary Dark, dark `#4CB368` lifted primary). No screen-level changes.

Resolution evidence: regression coverage in
`cross_app_surfaces_audit_test.dart` ("T047 review fixes" group) asserts the
localized language row in both directions and the 2dp focused border in both
schemes; the capture harness was re-run (5/5, see `log-test-wave5-capture.txt`)
so all 68 PNGs reflect the fixes — the focus captures now show a visible ring
and the detail/discovery captures show `العربية`/`Arabic`. Post-fix verdict:
**APPROVED** by both roles.

This record makes no device/integration-test claim; that gate is T049.
