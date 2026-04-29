# Halaqaty — Role Definitions & Capability Matrix

> **Status:** Authoritative Reference | **Last Updated:** 2026  
> **Source:** Extracted from PLAN.md §2 as part of documentation deduplication (Phase: Planning)

**Related Documents:** [PRD.md](./PRD.md) · [FEATURES.md](./FEATURES.md) · [MVP_DECISION_REGISTER.md](./MVP_DECISION_REGISTER.md) · [ARCHITECTURE.md](../../engineering/architecture/ARCHITECTURE.md)

---

## Role Overview

| Role | Arabic Title | Description |
|------|-------------|-------------|
| Teacher / Reciter | مُقرئ / محفظ | Creates and manages circles; conducts sessions; evaluates students |
| Student | طالب | Joins circles; recites; tracks progress |
| Supervisor | مُشرف | Assigned by teacher; helps manage sessions and queue |
| Institution Admin | مدير مؤسسة | Manages entire institution (future — F-017) |

> **Authorization note:** Role boundaries are enforced server-side via Go backend authorization middleware. The `circle_members.role` column stores the assigned role per circle. See [ARCHITECTURE.md §4](../../engineering/architecture/ARCHITECTURE.md#4-database-schema) for schema detail.

---

## Teacher (مُقرئ / محفظ)

The primary power user of Halaqaty.

**Capabilities:**

- Create and configure circles (name, rules, capacity, privacy settings)
- Generate and share invite codes/links
- Conduct live audio sessions (video is post-MVP behind feature flag)
- Manage the recitation queue during sessions (order, skip, grade)
- Assign and revoke the Supervisor role to any circle member at any time
- Grade students' recitations (Excellent → Repeat scale)
- Send private messages and voice notes to students
- View session-level progress per student in MVP (comprehensive analytics post-MVP)
- Set and manage weekly schedules
- Pin important announcements in circle chat

---

## Student (طالب)

> **Key design decision:** A student can join **multiple circles with different teachers simultaneously.** This is a common real-world scenario — a student may recite new memorization with one teacher and do revision sessions with another.

**Capabilities:**

- Join multiple circles with different teachers
- Participate in live sessions; join recitation queue
- Send and receive messages (group and private)
- View own session-level progress and grades in MVP (advanced analytics post-MVP)
- Receive schedule reminders and turn notifications (when it's their time to recite)
- View Quran map of memorized portions

---

## Supervisor (مُشرف)

A trusted student or assistant appointed by the teacher to help manage sessions. MVP uses this assignable role model; a distinct co-teacher role remains deferred until after pilot.

> **Key design decision:** A supervisor can be assigned at any point — before the session is created, before the session starts, or during a live session. The teacher retains full authority at all times.

**Capabilities (granted by teacher):**

- Manage the recitation queue (reorder, skip, add late joiners)
- Mute/unmute participants in live sessions
- Track attendance
- Pin messages in circle chat

**Explicit restrictions — enforced by authorization middleware:**

- **Cannot grade students** — grading is teacher-only
- **Cannot remove the teacher** from the circle

> **Decision reference:** Co-teacher / Supervisor boundary — see [MVP_DECISION_REGISTER.md](./MVP_DECISION_REGISTER.md) entry PRD-4.

---

## Institution Admin (مدير مؤسسة) — *Future / F-017*

- Manages the entire institution's presence on Halaqaty
- Onboards teachers and students in bulk
- Views institution-wide dashboards and analytics
- Manages institution-level settings and branding
- Scoped to F-017 (Institutional Platform); not part of MVP

> **Future capabilities:** See [FEATURES.md F-017](./FEATURES.md#f-017-institutional-platform) for full institutional platform specification.

---

## Role Assignment Rules

| Rule | Detail |
|------|--------|
| Circle-scoped | All roles (Teacher, Student, Supervisor) are scoped per circle, not globally |
| Multi-circle participation | A user can be a Teacher in one circle and a Student in another |
| Supervisor assignment | Only the circle's Teacher can assign or revoke the Supervisor role |
| Supervisor timing | Can be assigned before session creation, before session start, or during a live session |
| One teacher per circle | Each circle has exactly one teacher (the creator); transferable ownership is post-MVP |

---

## Role → Feature Access Matrix (MVP)

| Capability | Teacher | Student | Supervisor |
|-----------|---------|---------|------------|
| Create circle | ✅ | ❌ | ❌ |
| Invite members | ✅ | ❌ | ❌ |
| Start live session | ✅ | ❌ | ❌ |
| Manage queue (reorder/skip) | ✅ | ❌ | ✅ |
| Grade recitation | ✅ | ❌ | ❌ |
| Mute/unmute participants | ✅ | ❌ | ✅ |
| Pin messages | ✅ | ❌ | ✅ |
| Track attendance | ✅ | ❌ | ✅ |
| View own grades | ✅ (as teacher) | ✅ | ✅ |
| Assign Supervisor role | ✅ | ❌ | ❌ |
| Remove teacher | ❌ | ❌ | ❌ |
| Set schedule | ✅ | ❌ | ❌ |
| Send messages | ✅ | ✅ | ✅ |
| Send voice notes | ✅ | ✅ | ✅ |
