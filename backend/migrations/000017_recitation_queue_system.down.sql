-- 000017_recitation_queue_system.down.sql
-- Removes only F-003 objects: queue/progress tables (children first; entries
-- and rounds reference each other, so they drop in one statement), the Surah
-- seed, and the six sessions queue-policy columns (their CHECK constraints
-- drop automatically with the columns). F-001/F-002/F-005 tables and data are
-- untouched.

DROP TABLE IF EXISTS memorization_progress;
DROP TABLE IF EXISTS queue_event_outbox;
DROP TABLE IF EXISTS queue_opt_out_requests;
DROP TABLE IF EXISTS queue_command_receipts;
DROP TABLE IF EXISTS recitation_queue_preorder;
DROP TABLE IF EXISTS recitation_queue_entries, recitation_queue;
DROP TABLE IF EXISTS quran_surahs;

ALTER TABLE sessions DROP COLUMN IF EXISTS queue_population_policy;
ALTER TABLE sessions DROP COLUMN IF EXISTS queue_finalization_policy;
ALTER TABLE sessions DROP COLUMN IF EXISTS queue_opt_out_policy;
ALTER TABLE sessions DROP COLUMN IF EXISTS queue_grade_visibility;
ALTER TABLE sessions DROP COLUMN IF EXISTS queue_grade_correction;
ALTER TABLE sessions DROP COLUMN IF EXISTS queue_policy_version;
