# Halaqaty — Role Definitions & Capability Matrix

> **Status:** Authoritative Reference | **Last Updated:** 2026  
> **Source:** Extracted from PROJECT_PLAN.md §2 as part of documentation deduplication (Phase: Planning)

**Related Documents:** [PRD.md](./PRD.md) · [FEATURES.md](./FEATURES.md) · [MVP_DECISION_REGISTER.md](./MVP_DECISION_REGISTER.md) · [ARCHITECTURE.md](../../engineering/architecture/ARCHITECTURE.md)

---

## Role Overview

| Role | Arabic Title | Description |
|------|-------------|-------------|
| Teacher / Reciter | مُقرئ / محفظ | Creates and manages circles; conducts sessions; evaluates students |
| Student | طالب | Joins circles; recites; tracks progress |
| Supervisor | مُشرف | Assigned during circle creation or by a circle manager; helps manage sessions, queue, and roles |
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
- Assign another member as teacher, supervisor, or student, while retaining at least one teacher
- Grade students' recitations (5-grade scale: Excellent → Good → Acceptable → Needs Review → Repeat)
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
- View own progress via F-007: Quran Map (114 Surahs), attendance vs practiced history, recitation log, and progress analytics
- Receive schedule reminders and turn notifications (when it's their time to recite)
- View Quran map of memorized portions

---

## Supervisor (مُشرف)

A trusted member appointed during circle creation or by a circle manager to help manage sessions. MVP uses this assignable role model; a distinct co-teacher role remains deferred until after pilot.

> **Key design decision:** A supervisor can be assigned at circle creation or later by a teacher or supervisor. Multiple teachers may share the circle's authority.

**Capabilities (granted by a circle manager):**

- Manage the recitation queue (reorder, skip, add late joiners)
- Mute/unmute participants in live sessions
- Track attendance
- Change another member's teacher, supervisor, or student assignment
- Pin messages in circle chat

**Explicit restrictions — enforced by authorization middleware:**

- **Cannot grade students** — grading is teacher-only
- **Cannot change their own role** or leave the circle without a teacher

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
| Circle creation | Any authenticated user may create a circle; their role in an existing circle does not restrict it |
| Multi-circle participation | A user can be a Teacher in one circle and a Student in another |
| Initial assignments | Creator may select existing registered users as one or more teachers and one optional backup supervisor; without a selected teacher, the creator becomes teacher |
| Role management | A teacher or supervisor may change another member's role among teacher, supervisor, and student; they cannot change their own role or remove the final teacher |
| Supervisor timing | Can be assigned before session creation, before session start, or during a live session |
| Teacher count | A circle may have multiple teachers and must always retain at least one |

---

## Role → Feature Access Matrix (MVP)

| Capability | Teacher | Student | Supervisor |
|-----------|---------|---------|------------|
| Create circle | Account-level (any authenticated user) | Account-level (any authenticated user) | Account-level (any authenticated user) |
| Invite members | ✅ | ❌ | ❌ |
| Start live session | ✅ | ❌ | ❌ |
| Manage queue (reorder/skip) | ✅ | ❌ | ✅ |
| Grade recitation | ✅ | ❌ | ❌ |
| Mute/unmute participants | ✅ | ❌ | ✅ |
| Pin messages | ✅ | ❌ | ✅ |
| Track attendance | ✅ | ❌ | ✅ |
| View own grades | ✅ (as teacher) | ✅ | ✅ |
| Manage another member's role | ✅ | ❌ | ✅ |
| Change own role / remove final teacher | ❌ | ❌ | ❌ |
| Set schedule | ✅ | ❌ | ❌ |
| Send messages | ✅ | ✅ | ✅ |
| Send voice notes | ✅ | ✅ | ✅ |
