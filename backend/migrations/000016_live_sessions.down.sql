-- 000016_live_sessions.down.sql
-- Removes only F-005 objects: presence first (it references sessions), then
-- the sessions table. Users and circles data is untouched.
DROP TABLE IF EXISTS session_participant_presence;
DROP TABLE IF EXISTS sessions;
