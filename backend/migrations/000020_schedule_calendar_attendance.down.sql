-- 000020_schedule_calendar_attendance.down.sql
-- Disposable-schema rollback for the additive F-006 migration.
--
-- WARNING (data-model.md "Migration sequence and rollback" §3): do NOT run
-- this file blindly in production. It destroys every F-006 table, including
-- attendance history and the append-only correction audit. Production
-- rollback must instead disable F-006 routes, roll back code, and leave the
-- additive tables in place until F-006 data (especially attendance history)
-- is preserved and migrated deliberately. This down migration is safe only
-- on disposable schemas where no F-006 data must be retained.

-- Dependency-safe order: corrections reference attendance; details,
-- exceptions, selected dates and revisions reference schedules/sessions.
DROP TABLE IF EXISTS attendance_corrections;
DROP TABLE IF EXISTS session_attendance;
DROP TABLE IF EXISTS planned_session_details;
DROP TABLE IF EXISTS schedule_occurrence_exceptions;
DROP TABLE IF EXISTS schedule_selected_dates;
DROP TABLE IF EXISTS schedule_revisions;
DROP TABLE IF EXISTS schedule_request_replays;
DROP TABLE IF EXISTS schedules;
DROP FUNCTION IF EXISTS schedule_weekdays_unique(SMALLINT[]);

ALTER TABLE profiles
    DROP CONSTRAINT IF EXISTS ck_profiles_timezone;

ALTER TABLE profiles
    DROP COLUMN IF EXISTS timezone;
