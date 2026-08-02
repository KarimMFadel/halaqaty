-- 000012_auth_profiles_display_name.down.sql
-- Reverses only the objects introduced by 000012_auth_profiles_display_name.up.sql.

-- 1. Drop the preferred-language check constraint.
ALTER TABLE profiles
    DROP CONSTRAINT IF EXISTS chk_profiles_preferred_language;

-- 2. Drop the columns added by this migration.
ALTER TABLE profiles
    DROP COLUMN IF EXISTS preferred_language;

ALTER TABLE profiles
    DROP COLUMN IF EXISTS display_name;
