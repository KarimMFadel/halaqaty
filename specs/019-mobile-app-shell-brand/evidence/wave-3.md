# Wave 3 Evidence — Chat (US4)

Date: 2026-09-23 · Branch: `019-mobile-app-shell-brand` · Scope: T029–T036

Wave 3 retains the Wave 0 Chats-list loading/empty/retryable/offline recovery
contract (T029/T030, carried forward — see `wave-0.md`) and re-verifies the
existing F-004 group/direct conversation and composer/media implementation
(T031–T034) without altering authorization or message behavior. The wave's
production-adjacent changes are test coverage, a timing-flake fix, the
presentation-only T036 review fixes (UX-W3-1..4/7/8, UI-W3-1/2/7 — see the
review sections below; no F-004 authorization or mutation-rule change), and
this evidence.

## T035 — Chat verification

Fresh results on the final code (after the T036 review fixes):

| Gate | Command | Result |
|---|---|---|
| Chat + shell widget suites | `flutter test test/widget/chat test/widget_test.dart` | **87/87 passed** |
| Full unit/widget suite | `flutter test test` | **513/513 passed** ([log](log-test-wave3.txt)) |
| Chat device journeys | `flutter test integration_test/chat_direct_flow_test.dart integration_test/chat_discovery_flow_test.dart integration_test/chat_group_flow_test.dart integration_test/chat_lifecycle_flow_test.dart integration_test/chat_media_flow_test.dart integration_test/chat_moderation_flow_test.dart integration_test/chat_offline_recovery_test.dart integration_test/chat_presence_flow_test.dart -d emulator-5554` | **15 passed, 2 skipped** ([log](log-test-wave3-integration.txt)) |
| Wave 3 visual harness | `flutter test tool/wave3_visual_capture_test.dart` | **All tests passed**; 64 PNGs captured |
| Static analysis | `flutter analyze` | **No issues found** ([log](log-analyze-wave3.txt)) |
| Format | `dart format --output=none --set-exit-if-changed .` | wave-3 files clean ([log](log-format-wave3.txt)) |

Integration skips are environmental, not failures: `chat_direct_flow_test.dart`
(T064) and `chat_media_flow_test.dart` (T052) self-skip because their
`T064_*`/`T052_*` env vars (pre-provisioned Firebase tokens and backend
sessions) are unavailable in this session — the same skip pattern recorded in
Wave 2.

As in Wave 2, the Windows Java selector fails with the default long temporary
path; the device command above ran with process-local `TEMP`/`TMP` set to
`C:\jtmp`. No repository or machine configuration was changed.

### Visual harness note (device → host capture)

The Wave 0–2 `flutter drive` recipe was attempted first
(`integration_test` harness + screenshot driver on `emulator-5554`), but the
device screenshot channel stalled twice — the `takeScreenshot` VM-service
request never completed (35+ and 15+ minutes, zero PNGs, emulator otherwise
responsive). Wave 4 already established the host-side precedent for exactly
this situation, so the matrix was captured with
`mobile/tool/wave3_visual_capture_test.dart`, a plain widget test that
rasterizes each state off a `RepaintBoundary` with the bundled Poppins/Cairo
fonts loaded. The device-path harness was removed; the host harness is the
recorded Wave 3 evidence path.

Harness truths surfaced while bringing the matrix up (the never-executed
device harness had them wrong): the attachment chooser opens from the
"إرفاق"/"Attach" text action (there is no `Icons.attach_file` affordance);
`GroupChatScreen` re-applies its `readOnly` constructor flag onto the
controller after mount, so the archived capture must pass `readOnly: true` to
the widget, not only seed the controller state; and the chats-list captures
stub the discovery controller so the list renders from chat state alone.

### Concurrency note

A parallel session began Wave 5 work in this worktree during Wave 3
verification (touching `halaqaty_theme.dart`, `chats_screen.dart`,
`home_screen.dart`, circle files, and adding `tool/wave5_visual_capture_test.dart`
+ `test/widget/wave5/`). The Wave 3 full-suite log was refreshed after those
edits and again after the T036 review fixes; the chat-scoped suites and the
visual harness were also re-run on the final tree (87/87 passed, 64 PNGs
recaptured) so the chat evidence reflects current code. The repo-wide
`dart format` check flags only the parallel
session's unformatted `tool/wave5_visual_capture_test.dart`, which is outside
Wave 3 scope and was left untouched.

## T031/T033 — Added regression coverage

- Group/direct conversation entry, hierarchy, own/other, delivery/read,
  search/reply/pin/delete, archived/read-only, access-lost, terminal/retryable,
  semantics, targets, and large-text coverage in `mobile/test/widget/chat/`
  (T031) — focused group/direct/entry suite 18/18.
- Voice-upload pending state: `chat_media_widgets_test.dart` now proves the
  send announces "sending" copy in a live region while the upload is in flight
  and returns to idle only after acceptance — no fabricated success
  (T033, RTL + LTR).
- `chat_presence_flow_test.dart`: read-receipt expiry window 40 ms → 1 s and
  the typing-expiry assertion now waits on the actual state instead of a fixed
  80 ms pump — device-timing flake fixes, no behavior change.

## T035 — RTL/LTR × light/dark screenshot matrix

Captured host-side at 390dp logical width (780px at pixelRatio 2; 320dp/600dp
responsive variants at their stated sizes) using fresh provider stubs for every
state; each capture waits for its sentinel to remain visible for three
consecutive frames before rasterizing.

64 PNGs are stored under [screenshots/wave3/](screenshots/wave3/) — sixteen
states × Arabic RTL / English LTR × light / dark:

| State | Files | Verified content |
|---|---|---|
| Chats empty | `wave3_chats_empty_{ar,en}_{light,dark}.png` | Retained Wave 0 empty-state copy |
| Chats load error | `wave3_chats_error_{ar,en}_{light,dark}.png` | Retryable error, not a fake empty list (Wave 0 contract) |
| Chats loaded | `wave3_chats_loaded_{ar,en}_{light,dark}.png` | Client-derived circle group chat row; RTL chevron mirrored |
| Group empty | `wave3_group_empty_{ar,en}_{light,dark}.png` | Localized "no messages yet" empty state |
| Group history | `wave3_group_history_{ar,en}_{light,dark}.png` | Own/other bubbles, sender names, read indicator, composer |
| Direct empty | `wave3_direct_empty_{ar,en}_{light,dark}.png` | Direct composer present without fabricated history |
| Direct history | `wave3_direct_history_{ar,en}_{light,dark}.png` | Authorized pair history, own/other split |
| Attachment sheet | `wave3_attachment_sheet_{ar,en}_{light,dark}.png` | Chooser with photo/PDF options only — no invented types |
| Voice preview | `wave3_voice_preview_{ar,en}_{light,dark}.png` | Duration, waveform, listen/discard/send actions |
| Failed send | `wave3_failed_send_{ar,en}_{light,dark}.png` | Terminal failure copy with edit/discard/retry; draft retained |
| Offline draft | `wave3_offline_draft_{ar,en}_{light,dark}.png` | Draft text retained in the composer |
| Archived read-only | `wave3_archived_read_only_{ar,en}_{light,dark}.png` | Read-only banner; composer and pin actions hidden |
| Access lost | `wave3_access_lost_{ar,en}_{light,dark}.png` | Safe history-error copy with retry; no raw errors |
| 320dp width | `wave3_conversation_320dp_{ar,en}_{light,dark}.png` | Compact phone width, no overflow |
| 600dp width | `wave3_conversation_600dp_{ar,en}_{light,dark}.png` | Wide layout, no stretching defects |
| 200% text | `wave3_conversation_text_200pct_{ar,en}_{light,dark}.png` | Large text scale, no clipping |

All 64 files were spot-checked for clipping, overflow, incorrect
directionality, raw identifiers, placeholder/developer copy, and theme
regressions. The matrix contains no debug banner or visual-test terminology.
Captures reflect the amended contrast tokens (Wave 2 UI-M2/M3).

## T036 — UX Designer review

Verdict: **APPROVE-WITH-FIXES**. The reviewer verified tap budgets (Chats tab →
circle tile = 2 taps; direct chat's 4-tap members path remains the recorded
authorization-context exception), own/other bubble distinction with icon-plus-
text delivery states (never color-only), correct RTL mirroring, retained Wave 0
chats-list states, responsive captures without clipped primary actions, and
zero notice-classified chat actions (all four under-implementation notices are
Wave 4 — FR-032/033 compliant). Findings and their resolution:

- **UX-W3-1 (major) — fixed.** Access-lost mapped to the same retryable error
  as a transient failure. Now terminal localized copy ("You no longer have
  access to this conversation") with a Back exit instead of Retry, per FR-009,
  in both group and direct screens (`accessLost`/`back` labels).
- **UX-W3-2 (major) — fixed.** Archived read-only hid pin but left delete
  tappable. `canDelete` is now also gated on `!state.readOnly`, and the
  read-only banner states the archived reason (`readOnlyArchived` label). The
  F-004 moderation rules themselves are untouched.
- **UX-W3-3 (major) — fixed.** Direct chat collapsed error and access-lost
  into one text-only line with no recovery, and its empty state was a blank
  body. Error vs access-lost are now split (retry vs safe exit), and the empty
  state shows the localized instructional no-messages copy.
- **UX-W3-4 (minor) — fixed.** A failed message showed a "Sending" badge above
  a "Send failed" strip. `ChatMessageBubble` now takes `hasTerminalFailure`
  and suppresses the pending badge when a terminal failure exists.
- **UX-W3-5 (minor) — deferred.** Standalone centered pin button detached from
  its bubble; consolidating it into the long-press affordance changes F-004
  interaction → recorded for the Wave 5 audit (T043).
- **UX-W3-6 (minor) — deferred.** Icon-only destructive delete without
  confirmation; changing it alters F-004 behavior → F-004 follow-up, flagged
  for T043.
- **UX-W3-7 (minor) — fixed.** Direct composer/bubbles sat flush to screen
  edges; the direct body now uses the same 8dp padding as the group screen.
- **UX-W3-8 (minor) — fixed.** Direct send was always enabled and rejections
  were silent; send is disabled while the field is empty and a rejected send
  announces the existing `actionFailed` live-region label.
- **UX-W3-9 (minor) — harness notes.** Attachment-sheet clipping, undimmed
  composer text, and the stray glyph in `direct_empty` are bare-Scaffold
  harness artifacts, not product defects; annotated here, no product change.

All fixes are presentation-only: no F-004 behavior change, no new endpoint,
no new dependency. New/updated widget tests cover each fix (see the T035 gate
table — 87/87 chat+shell, 513/513 full suite).

## T036 — UI Designer review

Verdict: **APPROVE-WITH-FIXES** — one in-scope WCAG fix required; token
conformance otherwise good (zero raw hex in chat presentation, all color via
`colorScheme` roles, typography via `textTheme`, spacing on the 8px scale,
RTL mirroring correct, 64-PNG light/dark parity verified). Measured contrast
highlights: own-bubble text ≈6.4:1 light / ≈7.5:1 dark; "Send failed" copy
5.39:1 light / 5.68:1 dark; media-bar labels 4.95:1 / 7.50:1. Findings:

- **UI-W3-1 (major) — fixed.** The own-bubble delete `IconButton` inherited
  `onSurfaceVariant`, measuring **2.35:1** on the light `primaryContainer`
  fill (below the 3:1 interactive-component floor). Now
  `foregroundColor: colorScheme.onSurface` → 6.45:1 light / 7.54:1 dark. The
  reviewer's warning was heeded: `onPrimaryContainer` on `primaryContainer`
  measures only 3.34:1 in both modes and was not used.
- **UI-W3-2 (minor) — fixed.** Three sibling error labels used bare
  `TextStyle(color: colorScheme.error)`; aligned to
  `textTheme...copyWith(color: colorScheme.error)` for consistency.
- **UI-W3-3 (minor) — deferred to T043.** Pin buttons centered under every
  bubble (visual clutter, ambiguous ownership) — a widget-tree change outside
  Wave 3's boundary.
- **UI-W3-4 (token note) — deferred to T043 / DESIGN.md amendment.**
  `primaryContainer`↔`onPrimaryContainer` is 3.34:1 in both schemes; the
  current near-black/near-white bubble foreground is the accessible choice and
  should be formalized as a documented chat-bubble rule before a future
  implementer "corrects" it.
- **UI-W3-5 (minor) — deferred.** Attachment-sheet drag handle/row icons are
  cosmetic widget-tree changes; the narrow overlapping sheet in captures is
  the known harness artifact.
- **UI-W3-6 (evidence note) — deferred to T043.** The hard black card stroke
  in light-mode captures is an offscreen-rasterizer shadow artifact (absent
  from Wave 1 emulator captures); one on-emulator elevation spot check is
  recommended in T043.
- **UI-W3-7 (minor nit) — fixed.** `chat_status_widgets.dart` off-scale
  `SizedBox(width: 3)` → 4.
- **UI-W3-8 (behavior note) — resolved by UX-W3-2** (delete now gated in
  read-only).

Wave 3 is approved by both roles with the fixes above landed and re-gated;
UX-W3-5/6 and UI-W3-3..6 are recorded for the Wave 5 cross-app audit (T043).
