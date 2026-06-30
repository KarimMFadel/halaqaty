# F-007 — Enhanced Student Progress Tracking: Technical Specification

> **Version:** 1.0 | **Status:** Approved | **Date:** 2026-06-30
> **Feature:** [FEATURES.md F-007](../../management/product/FEATURES.md#f-007-enhanced-student-progress-tracking)
> **Architecture:** [ARCHITECTURE.md](../architecture/ARCHITECTURE.md)

---

## Table of Contents

1. [Locked Decisions](#1-locked-decisions)
2. [DB Schema Changes](#2-db-schema-changes)
3. [SQL Views & Materialized View](#3-sql-views--materialized-view)
4. [New API Endpoints](#4-new-api-endpoints)
5. [Auto-population on Grade Submit](#5-auto-population-on-grade-submit)
6. [Performance & Indexes](#6-performance--indexes)
7. [Sprint Plan](#7-sprint-plan)
8. [Files & Components Affected](#8-files--components-affected)

---

## 1. Locked Decisions

| ID | Decision | Answer |
|----|----------|--------|
| OQ-027 | Surah status thresholds | Fixed globally — same rules for all circles |
| OQ-028 | "Practiced" definition | Only `completed` turns count; `skipped` and `opted_out` do NOT |
| OQ-029 | Teacher cross-circle visibility | Yes — teacher can see student's full cross-circle progress |
| OQ-030 | Needs Attention flag threshold | ≥7 consecutive sessions attended with zero recitation turns → 🚩 yellow flag |
| OQ-031 | Memorized stale badge threshold | 30 days without any revision → ⚠️ badge on memorized Surah |
| OQ-032 | `surah_id` FK in `memorization_progress` | Add `surah_id INT FK`; keep `surah_name` deprecated until v1.1 |
| OQ-033 | Memorized degradation | Soft degradation — stays `memorized` status but shows ⚠️ badge |
| OQ-034 | `quran_divisions` seed standard | Medina Mushaf — 240 Rub' divisions |
| GRADE-ENUM | Grade scale | 5-grade: `excellent / good / acceptable / needs_review / repeat` |
| CROSS-CIRCLE | Global map conflict resolution | Most recent update wins (full history preserved) |

---

## 2. DB Schema Changes

> **Migration file numbering:** The file numbers below (`0006`–`0012`) are placeholders. Before writing actual migration files, run `ls backend/migrations/` to find the current highest sequence number and continue from there. The order of the 7 migrations must be preserved relative to each other.

### 2.1 Grade Enum — Update to 5-Grade Scale

**Affects:** `recitation_queue_entries.grade`, `memorization_progress.grade`

```sql
-- FILE: migrations/0009_grade_enum_5grade.up.sql
-- Replaces the 4-grade scale (excellent/good/needs_improvement/repeat)
-- with the canonical 5-grade scale.

-- Step 1: Rename existing value if present (old dev data only)
UPDATE recitation_queue_entries
  SET grade = 'needs_review'
  WHERE grade = 'needs_improvement';

UPDATE memorization_progress
  SET grade = 'needs_review'
  WHERE grade = 'needs_improvement';

-- Step 2: Drop and recreate the CHECK constraint on recitation_queue_entries
ALTER TABLE recitation_queue_entries
  DROP CONSTRAINT IF EXISTS recitation_queue_entries_grade_check;

ALTER TABLE recitation_queue_entries
  ADD CONSTRAINT recitation_queue_entries_grade_check
  CHECK (grade IN ('excellent','good','acceptable','needs_review','repeat'));

-- Step 3: Same for memorization_progress
ALTER TABLE memorization_progress
  DROP CONSTRAINT IF EXISTS memorization_progress_grade_check;

ALTER TABLE memorization_progress
  ADD CONSTRAINT memorization_progress_grade_check
  CHECK (grade IN ('excellent','good','acceptable','needs_review','repeat'));

-- FILE: migrations/0009_grade_enum_5grade.down.sql
-- ⚠️  SAFETY WARNING: Rolling back the 5-grade constraint is only safe if
-- no rows contain the value 'acceptable'. If 'acceptable' grades were inserted,
-- the constraint below will CONFLICT with existing data. Run this guard first:
--   SELECT COUNT(*) FROM recitation_queue_entries WHERE grade = 'acceptable';
--   SELECT COUNT(*) FROM memorization_progress WHERE grade = 'acceptable';
-- Both must return 0 before proceeding with the rollback.

UPDATE recitation_queue_entries SET grade = 'needs_improvement' WHERE grade = 'needs_review';
UPDATE memorization_progress    SET grade = 'needs_improvement' WHERE grade = 'needs_review';
-- 'acceptable' has no equivalent in the old 4-grade scale — map to 'good' as best approximation
UPDATE recitation_queue_entries SET grade = 'good' WHERE grade = 'acceptable';
UPDATE memorization_progress    SET grade = 'good' WHERE grade = 'acceptable';

ALTER TABLE recitation_queue_entries
  DROP CONSTRAINT IF EXISTS recitation_queue_entries_grade_check;
ALTER TABLE recitation_queue_entries
  ADD CONSTRAINT recitation_queue_entries_grade_check
  CHECK (grade IN ('excellent','good','needs_improvement','repeat'));

ALTER TABLE memorization_progress
  DROP CONSTRAINT IF EXISTS memorization_progress_grade_check;
ALTER TABLE memorization_progress
  ADD CONSTRAINT memorization_progress_grade_check
  CHECK (grade IN ('excellent','good','needs_improvement','repeat'));
```

---

### 2.2 Normalize `memorization_progress.surah_id` (Two-Phase Migration)

#### Phase 1 — Expand (non-breaking, deploy first)

```sql
-- FILE: migrations/0006_normalize_mp_surah.up.sql

-- Add nullable surah_id (no downtime — existing code ignores it)
ALTER TABLE memorization_progress
  ADD COLUMN surah_id INT REFERENCES quran_surahs(id);

-- Backfill via queue entry chain (~95% of rows)
UPDATE memorization_progress mp
SET    surah_id = rq.surah_id
FROM   recitation_queue_entries rqe
JOIN   recitation_queue rq ON rq.id = rqe.queue_id
WHERE  rqe.id = mp.queue_entry_id
  AND  mp.surah_id IS NULL;

-- Backfill remainder by name matching
UPDATE memorization_progress mp
SET    surah_id = qs.id
FROM   quran_surahs qs
WHERE  mp.surah_id IS NULL
  AND (LOWER(TRIM(mp.surah_name)) = LOWER(qs.name_transliterated)
   OR  mp.surah_name = qs.name_arabic);

-- Reconciliation check — must return 0 before Phase 2:
-- SELECT COUNT(*) FROM memorization_progress WHERE surah_id IS NULL;

-- FILE: migrations/0006_normalize_mp_surah.down.sql
ALTER TABLE memorization_progress DROP COLUMN IF EXISTS surah_id;
```

#### Phase 2 — Contract (after backfill verified)

```sql
-- FILE: migrations/0007_enforce_surah_id_not_null.up.sql
ALTER TABLE memorization_progress
  ALTER COLUMN surah_id SET NOT NULL;

COMMENT ON COLUMN memorization_progress.surah_name IS
  'DEPRECATED — use surah_id. Scheduled for removal after v1.1.';

-- FILE: migrations/0007_enforce_surah_id_not_null.down.sql
ALTER TABLE memorization_progress
  ALTER COLUMN surah_id DROP NOT NULL;
```

---

### 2.3 Add Idempotency + `updated_at` to `memorization_progress`

```sql
-- FILE: migrations/0008_mp_idempotency.up.sql

-- Unique constraint enables safe ON CONFLICT upsert on re-grade
ALTER TABLE memorization_progress
  ADD CONSTRAINT uq_mp_queue_entry_id UNIQUE (queue_entry_id);

-- updated_at for re-grade traceability
ALTER TABLE memorization_progress
  ADD COLUMN updated_at TIMESTAMPTZ;

-- Allow nullable grade (for rounds where grading_required = false)
ALTER TABLE memorization_progress
  ALTER COLUMN grade DROP NOT NULL;

-- FILE: migrations/0008_mp_idempotency.down.sql
ALTER TABLE memorization_progress DROP CONSTRAINT IF EXISTS uq_mp_queue_entry_id;
ALTER TABLE memorization_progress DROP COLUMN IF EXISTS updated_at;
ALTER TABLE memorization_progress ALTER COLUMN grade SET NOT NULL;
```

---

### 2.4 New Table: `quran_divisions`

Reference table mapping every Surah's Ayah ranges to the Medina Mushaf Juz/Hizb/Rub' divisions. Static seed data — never modified by the application.

```sql
-- FILE: migrations/0012_quran_divisions.up.sql

CREATE TABLE quran_divisions (
  id           SERIAL PRIMARY KEY,
  surah_id     INT  NOT NULL REFERENCES quran_surahs(id),
  from_ayah    INT  NOT NULL CHECK (from_ayah >= 1),
  to_ayah      INT  NOT NULL,
  juz_number   INT  NOT NULL CHECK (juz_number  BETWEEN 1 AND 30),
  hizb_number  INT  NOT NULL CHECK (hizb_number BETWEEN 1 AND 60),
  rub_number   INT  NOT NULL CHECK (rub_number  BETWEEN 1 AND 240),
  UNIQUE (surah_id, from_ayah),
  CHECK (to_ayah >= from_ayah)
);

-- Pre-seeded with all 240 standard Medina Mushaf Rub' boundaries.
-- Source: standard Uthmani mushaf (30 juz × 2 hizb × 4 rub' = 240 segments).
-- Seed file: migrations/seeds/quran_divisions.sql

CREATE INDEX idx_qd_surah ON quran_divisions(surah_id);
CREATE INDEX idx_qd_juz   ON quran_divisions(juz_number);

-- FILE: migrations/0012_quran_divisions.down.sql
DROP TABLE IF EXISTS quran_divisions;
```

**Purpose:** Allows teacher UI to offer a "grade by Rub'/Hizb/Juz" picker, and allows the Quran Map to correctly display segment-level coverage for long Surahs like Al-Baqarah.

---

## 3. SQL Views & Materialized View

### 3.1 View: `v_student_session_history`

Powers `GET /students/me/sessions/history`.

```sql
-- FILE: migrations/0011_progress_views.up.sql

CREATE OR REPLACE VIEW v_student_session_history AS
SELECT
  sa.user_id                                                        AS student_id,
  sa.session_id,
  s.circle_id,
  c.name                                                            AS circle_name,
  s.actual_start                                                    AS session_date,
  s.actual_end,
  sa.status                                                         AS attendance_status,
  sa.joined_at,
  sa.left_at,

  -- "practiced" = at least one completed turn (skipped/opted_out excluded)
  COALESCE(COUNT(rqe.id) FILTER (WHERE rqe.status = 'completed') > 0, FALSE)
                                                                    AS practiced,
  COUNT(rqe.id) FILTER (WHERE rqe.status = 'completed')            AS completed_turns,
  COUNT(rqe.id) FILTER (WHERE rqe.status = 'skipped')              AS skipped_turns,
  COUNT(rqe.id) FILTER (WHERE rqe.status = 'opted_out')            AS opted_out_turns,

  AVG(
    CASE rqe.grade
      WHEN 'excellent'    THEN 5.0
      WHEN 'good'         THEN 4.0
      WHEN 'acceptable'   THEN 3.0
      WHEN 'needs_review' THEN 2.0
      WHEN 'repeat'       THEN 1.0
      ELSE NULL
    END
  ) FILTER (WHERE rqe.status = 'completed')                         AS avg_grade_numeric

FROM   session_attendance sa
JOIN   sessions s  ON s.id  = sa.session_id
JOIN   circles  c  ON c.id  = s.circle_id
LEFT   JOIN recitation_queue rq
         ON rq.session_id = sa.session_id
LEFT   JOIN recitation_queue_entries rqe
         ON rqe.queue_id  = rq.id
        AND rqe.student_id = sa.user_id

WHERE  sa.status IN ('present', 'late')
  AND  s.status  = 'ended'

GROUP BY
  sa.user_id, sa.session_id, s.circle_id, c.name,
  s.actual_start, s.actual_end, sa.status, sa.joined_at, sa.left_at;
```

---

### 3.2 View: `v_student_circle_summary`

Powers `GET /students/me/circles/history`.

```sql
CREATE OR REPLACE VIEW v_student_circle_summary AS
SELECT
  sh.student_id,
  sh.circle_id,
  sh.circle_name,
  COUNT(DISTINCT sh.session_id)                                         AS sessions_attended,
  COUNT(DISTINCT sh.session_id) FILTER (WHERE sh.practiced)            AS sessions_practiced,
  COUNT(DISTINCT sh.session_id) FILTER (WHERE NOT sh.practiced)        AS sessions_attended_only,
  SUM(sh.completed_turns)                                               AS total_completed_turns,
  MAX(sh.session_date)                                                  AS last_session_date,
  AVG(sh.avg_grade_numeric) FILTER (WHERE sh.avg_grade_numeric IS NOT NULL)
                                                                        AS overall_avg_grade
FROM   v_student_session_history sh
GROUP  BY sh.student_id, sh.circle_id, sh.circle_name;
```

---

### 3.3 Materialized View: `mv_student_surah_status`

Powers `GET /students/me/progress` (global Quran Map). Refreshed async after every grade submission.

**Status derivation rules (fixed globally):**

| Condition on most recent record | Status |
|---|---|
| Latest grade = `needs_review` or `repeat` | `needs_recap` |
| Latest type = `new_memorization` AND grade IN (`excellent`, `good`) | `memorized` |
| Latest type IN (`revision`, `old_revision`) | `in_revision` |
| Has records but none match above | `in_progress` |
| No records | `not_started` *(gap-filled in Go at query time)* |

**`memorized_stale` is computed in Go** at read time, not stored: status = `memorized` AND `last_practiced_date < NOW() - 30 days`.

```sql
CREATE MATERIALIZED VIEW mv_student_surah_status AS
WITH ranked AS (
  SELECT
    mp.student_id,
    mp.surah_id,
    mp.grade          AS latest_grade,
    mp.type           AS latest_type,
    mp.date           AS last_practiced_date,
    mp.circle_id      AS last_circle_id,
    COUNT(*)          OVER (PARTITION BY mp.student_id, mp.surah_id) AS total_records,
    COUNT(*)          FILTER (WHERE mp.grade IN ('excellent','good'))
                      OVER (PARTITION BY mp.student_id, mp.surah_id) AS good_grade_count,
    ROW_NUMBER()      OVER (
      PARTITION BY mp.student_id, mp.surah_id
      ORDER BY mp.date DESC, mp.created_at DESC
    ) AS rn
  FROM memorization_progress mp
  WHERE mp.surah_id IS NOT NULL
)
SELECT
  r.student_id,
  r.surah_id,
  r.latest_grade,
  r.latest_type,
  r.last_practiced_date,
  r.last_circle_id,
  r.total_records,
  r.good_grade_count,
  CASE
    WHEN r.latest_grade IN ('needs_review', 'repeat')             THEN 'needs_recap'
    WHEN r.latest_type  = 'new_memorization'
     AND r.latest_grade IN ('excellent', 'good')                  THEN 'memorized'
    WHEN r.latest_type  IN ('revision', 'old_revision')           THEN 'in_revision'
    ELSE 'in_progress'
  END AS status,
  NOW() AS computed_at
FROM ranked r
WHERE r.rn = 1;

CREATE UNIQUE INDEX ON mv_student_surah_status(student_id, surah_id);
CREATE INDEX        ON mv_student_surah_status(student_id);
```

**Go refresh pattern (fire-and-forget after grade submit):**

```go
// internal/store/progress.go
func (s *Store) RefreshQuranMap(ctx context.Context) error {
    // CONCURRENTLY — no read lock; stale reads acceptable for up to 30 min
    _, err := s.db.Exec(ctx,
        `REFRESH MATERIALIZED VIEW CONCURRENTLY mv_student_surah_status`)
    return err
}
```

**Rollback:**
```sql
-- FILE: migrations/0011_progress_views.down.sql
DROP MATERIALIZED VIEW IF EXISTS mv_student_surah_status;
DROP VIEW IF EXISTS v_student_circle_summary;
DROP VIEW IF EXISTS v_student_session_history;
```

---

## 4. New API Endpoints

All endpoints require `Authorization: Bearer <firebase-jwt>`.
All new endpoints belong to the new `Progress` tag in `openapi.yaml`.

---

### `GET /students/me/circles/history`

```
Auth: any authenticated user (student reading own data)
Query params: limit (int, default 20), cursor (string), active_only (bool)
```

**Response 200:**
```json
{
  "data": [{
    "circle_id": "uuid",
    "circle_name": "حلقة الشيخ عبدالله",
    "my_role": "student",
    "joined_at": "2024-01-10T00:00:00Z",
    "is_active_member": true,
    "sessions_attended": 45,
    "sessions_practiced": 42,
    "sessions_attended_only": 3,
    "total_completed_turns": 210,
    "last_session_date": "2026-06-15T18:00:00Z",
    "overall_avg_grade": 3.8,
    "overall_avg_grade_label": "good"
  }],
  "next_cursor": "<opaque>",
  "total_circles": 3
}
```

---

### `GET /students/me/progress`

```
Auth: student (own data only)
Query params: circle_id (UUID, optional — if set, uses live per-circle query instead of mat-view)
```

**Response 200 (per-surah item):**
```json
{
  "student_id": "uuid",
  "last_computed_at": "2026-06-30T21:00:00Z",
  "summary": {
    "memorized_count": 15,
    "in_revision_count": 8,
    "needs_recap_count": 2,
    "in_progress_count": 5,
    "not_started_count": 84
  },
  "surahs": [{
    "surah_id": 1,
    "name_arabic": "الفاتحة",
    "name_transliterated": "Al-Fatihah",
    "ayah_count": 7,
    "juz_start": 1,
    "status": "memorized",
    "is_stale": false,
    "days_since_revision": 12,
    "latest_grade": "excellent",
    "latest_type": "new_memorization",
    "last_practiced_date": "2026-06-18",
    "total_sessions": 14,
    "coverage_pct": 100.0,
    "segments": []
  }, {
    "surah_id": 2,
    "name_arabic": "البقرة",
    "ayah_count": 286,
    "status": "in_progress",
    "is_stale": false,
    "latest_grade": "good",
    "coverage_pct": 22.0,
    "ayahs_covered": 63,
    "segments": [
      { "from_ayah": 1,  "to_ayah": 20,  "grade": "excellent",   "type": "new_memorization", "rub_number": 1 },
      { "from_ayah": 21, "to_ayah": 40,  "grade": "good",        "type": "new_memorization", "rub_number": 2 },
      { "from_ayah": 41, "to_ayah": 63,  "grade": "needs_review","type": "revision",          "rub_number": 3 }
    ]
  }]
}
```

> **`is_stale`:** computed in Go — `status == 'memorized' AND last_practiced_date < NOW() - 30 days`
> **`segments`:** populated only when the Surah has >1 recited range; empty for short fully-memorized Surahs

---

### `GET /students/me/sessions/history`

```
Query params: circle_id, practiced_only (bool), from_date, to_date, limit, cursor
```

**Response 200 (per-session item):**
```json
{
  "session_id": "uuid",
  "circle_name": "حلقة الشيخ عبدالله",
  "session_date": "2026-06-15T18:00:00Z",
  "attendance_status": "present",
  "practiced": true,
  "completed_turns": 2,
  "turns": [{
    "surah_id": 18,
    "surah_name_arabic": "الكهف",
    "from_ayah": 1,
    "to_ayah": 10,
    "round_type": "new_memorization",
    "grade": "excellent",
    "grade_label": "ممتاز",
    "teacher_notes": "ماشاء الله، تجويد ممتاز"
  }]
}
```

---

### `GET /students/me/progress/stats`

```
Query params: period (week|month|year, default month), circle_id, from_date, to_date
```

**Response 200:**
```json
{
  "period": "week",
  "buckets": [{
    "bucket_start": "2026-06-09",
    "ayahs_recited": 47,
    "sessions_attended": 3,
    "sessions_practiced": 3,
    "completed_turns": 9,
    "avg_grade": 4.2
  }],
  "totals": { "ayahs_recited": 420, "sessions_attended": 18, "avg_grade": 4.0 }
}
```

---

### `GET /circles/{circleId}/progress`

```
Auth: Teacher or Supervisor of the circle
```

**Response 200 (per-student summary):**
```json
{
  "data": [{
    "student_id": "uuid",
    "display_name": "أحمد محمد",
    "attendance_pct": 95.0,
    "practice_pct": 93.0,
    "last_practiced_date": "2026-06-28",
    "needs_attention_flag": false,
    "consecutive_attended_no_recitation": 0,
    "active_surahs": [{ "surah_id": 2, "name_arabic": "البقرة", "status": "in_progress" }]
  }]
}
```

> **`needs_attention_flag`:** true when `consecutive_attended_no_recitation >= 7`

---

### `GET /circles/{circleId}/progress/{userId}`

```
Auth: Teacher of any circle that includes userId as a member
Response: Full student profile — same shape as GET /students/me/progress but for any student
          includes cross-circle surah map (teacher can see all circles)
```

---

### `GET /circles/{circleId}/surah-insights`

```
Auth: Teacher of circleId
Query params: days (int, default 30)
```

**Response 200:**
```json
{
  "period_days": 30,
  "insights": [{
    "surah_id": 18,
    "name_arabic": "الكهف",
    "weak_grade_count": 24,
    "student_count": 8,
    "most_common_grade": "needs_review"
  }]
}
```

---

## 5. Auto-population on Grade Submit

### Where: `internal/service/queue.go` → `SubmitGrade()`

The service owns the transaction. After updating the queue entry grade, it upserts `memorization_progress`, then fires an async mat-view refresh.

**Decision table for edge cases:**

| Scenario | Action |
|---|---|
| Entry `status = 'completed'` | ✅ Insert/upsert `memorization_progress` |
| Entry `status = 'skipped'` | ❌ Return early — no progress record |
| Entry `status = 'opted_out'` | ❌ Return early — no progress record |
| `grading_required = false` | ✅ Insert with `grade = NULL` |
| Re-grade (teacher corrects) | ✅ `ON CONFLICT (queue_entry_id) DO UPDATE grade, notes, updated_at` |
| Mat-view refresh fails | ⚠️ Log warning, never fail the HTTP response |

**SQL upsert:**

```sql
-- store/queries/progress.sql
INSERT INTO memorization_progress (
  id, student_id, circle_id, session_id, queue_entry_id,
  surah_id, surah_name, from_ayah, to_ayah,
  type, grade, notes, date, created_at
) VALUES (
  $1, $2, $3, $4, $5,
  $6, $7, $8, $9,
  $10, $11, $12, $13, NOW()
)
ON CONFLICT (queue_entry_id)
  DO UPDATE SET
    grade      = EXCLUDED.grade,
    notes      = EXCLUDED.notes,
    updated_at = NOW()
RETURNING id, created_at, updated_at;
```

---

## 6. Performance & Indexes

```sql
-- FILE: migrations/0010_progress_indexes.up.sql

-- memorization_progress
CREATE INDEX CONCURRENTLY idx_mp_student_surah
  ON memorization_progress(student_id, surah_id);

CREATE INDEX CONCURRENTLY idx_mp_student_circle
  ON memorization_progress(student_id, circle_id);

CREATE INDEX CONCURRENTLY idx_mp_student_date
  ON memorization_progress(student_id, date DESC);

CREATE INDEX CONCURRENTLY idx_mp_session_id
  ON memorization_progress(session_id);

-- session_attendance — partial index (present rows only)
CREATE INDEX CONCURRENTLY idx_sa_user_present
  ON session_attendance(user_id, session_id)
  WHERE status IN ('present', 'late');

-- recitation_queue_entries — partial index (completed turns only)
CREATE INDEX CONCURRENTLY idx_rqe_student_completed
  ON recitation_queue_entries(student_id, queue_id)
  WHERE status = 'completed';

-- sessions — circle history pagination
CREATE INDEX CONCURRENTLY idx_sessions_circle_ended
  ON sessions(circle_id, actual_end DESC NULLS LAST)
  WHERE status = 'ended';

-- FILE: migrations/0010_progress_indexes.down.sql
DROP INDEX CONCURRENTLY IF EXISTS idx_mp_student_surah;
DROP INDEX CONCURRENTLY IF EXISTS idx_mp_student_circle;
DROP INDEX CONCURRENTLY IF EXISTS idx_mp_student_date;
DROP INDEX CONCURRENTLY IF EXISTS idx_mp_session_id;
DROP INDEX CONCURRENTLY IF EXISTS idx_sa_user_present;
DROP INDEX CONCURRENTLY IF EXISTS idx_rqe_student_completed;
DROP INDEX CONCURRENTLY IF EXISTS idx_sessions_circle_ended;
```

**Expected query latency at MVP scale (≤50 students, ≤1,560 records/student):**

| Endpoint | Expected Latency |
|---|---|
| Quran Map (global, mat-view) | < 5ms |
| Session history (page 1, 20 items) | < 10ms |
| Circle history | < 15ms |
| Stats/charts | < 25ms |
| Per-circle progress (live query) | < 15ms |

---

## 7. Sprint Plan

### Critical Path

```
A (Grade enum) ──► B (Migrations) ──┬──► C (Surah map logic + mat-view)
                                    └──► D (Backend API 8 endpoints)
                                                └──► E (OpenAPI contract)
                                                          └──► F (Flutter screens)
                                                                    └──► G (Tech Lead gate)
```

### Stories

| Story | Title | Effort | Owner | Blocks |
|---|---|---|---|---|
| **A** | Grade enum ADR + migration (5-grade) | S | Tech Lead + PM | Everything |
| **B** | DB migrations (0006–0012) + quran_divisions seed | M | Backend | C, D |
| **C** | Surah map logic: mat-view, status state machine, stale badge, 🚩 flag | M | Backend | D |
| **D** | Backend API: 8 handlers + repository + service | L | Backend | E |
| **E** | OpenAPI contract: Progress tag + 8 endpoints + 8 schemas | M | Backend + Tech Lead | F |
| **F** | Flutter: 4 screens + providers + widgets (SurahMapGrid, AttendancePill, etc.) | L | Mobile | G |
| **G** | Tech Lead gate: RBAC review, query plan analysis, code review | S | Tech Lead | Merge |

### Definition of Done (Full Feature)

- [ ] `make migrate-up` and `make migrate-down` clean
- [ ] All 8 endpoints return correct data with RBAC enforced
- [ ] `go test -tags=integration ./...` — zero failures
- [ ] `golangci-lint` — zero violations
- [ ] `spectral lint docs/contracts/openapi.yaml` — zero violations
- [ ] `flutter test` — zero failures, `flutter analyze` — zero issues
- [ ] All Flutter screens render correctly in Arabic RTL
- [ ] FEATURES.md F-007 and F-010 status updated to `🟢 Done`
- [ ] ARCHITECTURE.md §4 updated with new tables + views

---

## 8. Files & Components Affected

### Backend (Go)

| Path | Change |
|---|---|
| `backend/internal/progress/handler.go` | NEW — 8 HTTP handlers |
| `backend/internal/progress/repository.go` | NEW — all DB queries (pgx/v5) |
| `backend/internal/progress/model.go` | NEW — Go structs |
| `backend/internal/progress/service.go` | NEW — surah status logic, stale badge, 🚩 flag |
| `backend/internal/store/progress.go` | NEW — `RefreshQuranMap()` |
| `backend/internal/service/queue.go` | MODIFY — add progress upsert + async refresh after grade |
| `backend/cmd/api/main.go` | MODIFY — register `/progress` route group |
| `backend/migrations/0006_normalize_mp_surah.up/down.sql` | NEW |
| `backend/migrations/0007_enforce_surah_id_not_null.up/down.sql` | NEW |
| `backend/migrations/0008_mp_idempotency.up/down.sql` | NEW |
| `backend/migrations/0009_grade_enum_5grade.up/down.sql` | NEW |
| `backend/migrations/0010_progress_indexes.up/down.sql` | NEW |
| `backend/migrations/0011_progress_views.up/down.sql` | NEW |
| `backend/migrations/0012_quran_divisions.up/down.sql` | NEW |
| `backend/migrations/seeds/quran_divisions.sql` | NEW — 240 Rub' seed rows |

### Flutter (Mobile)

| Path | Change |
|---|---|
| `mobile/lib/features/progress/screens/student_progress_screen.dart` | NEW — 4-tab container |
| `mobile/lib/features/progress/screens/quran_map_screen.dart` | NEW — 114-surah RTL grid |
| `mobile/lib/features/progress/screens/surah_detail_sheet.dart` | NEW — drill-down per surah |
| `mobile/lib/features/progress/screens/attendance_history_screen.dart` | NEW — session timeline |
| `mobile/lib/features/progress/screens/teacher_circle_progress_screen.dart` | NEW |
| `mobile/lib/features/progress/providers/progress_provider.dart` | NEW — Riverpod AsyncNotifiers |
| `mobile/lib/features/progress/models/*.dart` | NEW — Dart models |
| `mobile/lib/features/progress/widgets/surah_map_grid.dart` | NEW — scrollable RTL grid |
| `mobile/lib/features/progress/widgets/surah_tile.dart` | NEW — color-coded tile |
| `mobile/lib/features/progress/widgets/attendance_pill.dart` | NEW — حضر / تلا / غائب |
| `mobile/lib/features/progress/widgets/grade_badge.dart` | NEW — grade display |
| `mobile/lib/features/progress/widgets/ayah_coverage_bar.dart` | NEW — segment bar for long surahs |
| `mobile/lib/core/router/app_router.dart` | MODIFY — add progress routes |

### Documentation

| Path | Change |
|---|---|
| `docs/contracts/openapi.yaml` | ADD — Progress tag + 8 endpoints + 8 schemas |
| `docs/engineering/architecture/ARCHITECTURE.md` | UPDATE — §4.0 grade enum, §4.2 new tables/views, §5 endpoint table |
| `docs/management/product/FEATURES.md` | UPDATED — F-007 expanded, F-010 cross-ref, status → Approved |
| `docs/engineering/design/F-007-SPEC.md` | THIS FILE |
