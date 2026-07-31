-- 000011_auth_roles_profile_alignment.up.sql
-- Aligns the Phase 1/2 foundation with the clarified session and role model:
--   * opaque UUID session identifiers
--   * non-null absolute expiry on every session
--   * device_name for current-device labeling
--   * FK from circle_members.circle_id to circles.id when the parent table exists
--   * supporting indexes for auth/session lookups

-- 1. Convert user_sessions.session_id to an opaque UUID with a safe backfill.
ALTER TABLE user_sessions
    ALTER COLUMN session_id SET DATA TYPE UUID
    USING (
        CASE
            WHEN session_id ~ '^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$'
                THEN session_id::uuid
            ELSE gen_random_uuid()
        END
    );

ALTER TABLE user_sessions
    ALTER COLUMN session_id SET DEFAULT gen_random_uuid();

-- 2. Add a user-visible device label. Not a credential.
ALTER TABLE user_sessions
    ADD COLUMN IF NOT EXISTS device_name VARCHAR(100);

-- 3. Enforce absolute expiry on every session and backfill existing rows.
UPDATE user_sessions
SET expires_at = NOW() + INTERVAL '90 days'
WHERE expires_at IS NULL;

ALTER TABLE user_sessions
    ALTER COLUMN expires_at SET NOT NULL;

ALTER TABLE user_sessions
    ALTER COLUMN expires_at SET DEFAULT (NOW() + INTERVAL '90 days');

-- 4. Add indexes to support the session validation paths.
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at
    ON user_sessions(expires_at);

CREATE INDEX IF NOT EXISTS idx_user_sessions_revoked_at
    ON user_sessions(revoked_at);

CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id_revoked
    ON user_sessions(user_id, revoked_at);

-- 5. Add a role-based index for circle authorization checks.
CREATE INDEX IF NOT EXISTS idx_circle_members_circle_role
    ON circle_members(circle_id, role);

-- 6. Conditionally validate circle_members.circle_id against circles.id
--    when the parent table exists (added by a later migration or already present).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.tables
        WHERE table_schema = current_schema()
          AND table_name = 'circles'
    ) THEN
        IF NOT EXISTS (
            SELECT 1
            FROM information_schema.table_constraints
            WHERE table_schema = current_schema()
              AND table_name = 'circle_members'
              AND constraint_name = 'fk_circle_members_circle_id_000011'
        ) THEN
            ALTER TABLE circle_members
                ADD CONSTRAINT fk_circle_members_circle_id_000011
                FOREIGN KEY (circle_id) REFERENCES circles(id) ON DELETE CASCADE;
        END IF;
    END IF;
END
$$;
