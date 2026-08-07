-- 000011_auth_roles_profile_alignment.down.sql
-- Reverses only the objects introduced by 000011_auth_roles_profile_alignment.up.sql.

-- 1. Restore user_sessions.session_id to a plain text primary key.
ALTER TABLE user_sessions
    ALTER COLUMN session_id DROP DEFAULT;

ALTER TABLE user_sessions
    ALTER COLUMN session_id SET DATA TYPE TEXT
    USING session_id::text;

-- 2. Remove the device label added by the alignment migration.
ALTER TABLE user_sessions
    DROP COLUMN IF EXISTS device_name;

-- 3. Allow absolute expiry to be nullable again (matches pre-alignment model).
ALTER TABLE user_sessions
    ALTER COLUMN expires_at DROP NOT NULL;

ALTER TABLE user_sessions
    ALTER COLUMN expires_at DROP DEFAULT;

-- 4. Drop alignment indexes.
DROP INDEX IF EXISTS idx_user_sessions_expires_at;
DROP INDEX IF EXISTS idx_user_sessions_revoked_at;
DROP INDEX IF EXISTS idx_user_sessions_user_id_revoked;
DROP INDEX IF EXISTS idx_circle_members_circle_role;

-- 5. Drop only the migration-specific circle_members foreign key if it was added.
ALTER TABLE circle_members
    DROP CONSTRAINT IF EXISTS fk_circle_members_circle_id_000011;
