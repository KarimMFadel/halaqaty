# Future UI Follow-ups

**Status:** Open follow-up inventory
**Updated:** 2026-09-30

This list records visible differences between the approved screen sample and the
current product UI. The sample at [screen-samples-a-v2.html](screen-samples-a-v2.html)
is a visual and interaction reference; it does not authorize fabricated data,
new API behavior, or changes to approved feature contracts. Product behavior
must be approved and specified before implementation.

## Home

- [ ] When the data contract supports it, show the sample's next-session
  countdown, teacher, and attendee count, and provide a direct **Enter
  session** action. Current Home shows **View calendar**; do not route a
  scheduled occurrence to a live room unless a valid live-session identity is
  available.
- [ ] Add study-progress and recent-activity subtitles to circle rows only
  after authoritative progress/activity data is available.

## Circles

- [ ] Review whether the current **My circles / Public circles** discovery
  sections should become the sample's segmented **My circles / Discover**
  control. Preserve current membership and discovery behaviors.
- [ ] Add teacher and schedule details to joined-circle cards when the
  existing APIs expose those values.
- [ ] Add **Suggested for you / See more** only with an approved
  recommendation rule and supporting API; do not present arbitrary circles as
  personalized suggestions.

## Circle details

- [ ] Add the sample's next-session summary and direct join action when the
  session data includes an eligible live session and safe route identity.
- [ ] Add learning progress and recent activity when supported by approved
  progress/activity contracts. These must be real, authorized user data.
- [ ] Review the current metadata and management-action emphasis against the
  sample hierarchy without removing required manager capabilities.

## Profile and preferences

- [ ] Review the sample's profile overview (avatar/contact summary) and
  separate **Edit profile** action against the current editable profile form.
- [ ] Decide and specify in-app appearance selection; dark/light currently
  follows the system theme. Keep the Garden dusk token palette from
  [DESIGN.md](DESIGN.md) when adding a setting.
- [ ] Decide and specify notification, help, and privacy actions currently
  shown as under-implementation notices.
- [ ] Reconcile profile sample actions with current approved account
  capabilities before changing or documenting deletion behavior. F-001 and
  ADR-024 govern account deletion; F-019 does not override them.

## Live session and recitation queue

- [ ] Conduct a visual review of the current combined session-status and queue
  layout against the sample's distinct focus/position cards. Treat this as a
  hierarchy/layout follow-up; current join/start, student-turn, teacher-control,
  and recovery behavior remains governed by F-005 and F-006.

## Chats and authentication

- [ ] Capture current device screens and compare chat attachment, voice-note,
  and offline-recovery interactions with the sample. Preserve the existing
  approved F-004 behavior and contracts.
- [ ] Keep authentication behavior aligned with approved routes and validation;
  no additional core action was identified in the current page audit.
- [ ] Do not add a unified direct-message inbox based on the sample; it
  explicitly excludes one.

## Verification and closure

- [ ] Capture current emulator screenshots for Home, Circles, Circle Details,
  and Profile in Arabic RTL and compare each with the sample and this follow-up
  list.
- [ ] Before closing an item, identify its approved Spec-Kit scope/contract,
  implement only supported behavior, and record fresh widget/device evidence.

Items marked open are follow-up candidates, not authorization to invent
requirements. Update this file and the relevant approved feature artifacts when
product decisions are made.
