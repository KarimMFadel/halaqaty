-- 000012_auth_profiles_display_name.up.sql
-- Aligns profiles with the canonical API contract (RegisterRequest/UserProfile):
--   * display_name — public label chosen at registration (nullable until set)
--   * preferred_language — UI language preference, NOT NULL DEFAULT 'ar',
--     constrained to the contract enum ('ar', 'en')

-- 1. Public display name supplied during registration.
ALTER TABLE profiles
    ADD COLUMN IF NOT EXISTS display_name TEXT;

-- 2. Preferred UI language with a contract-aligned default.
ALTER TABLE profiles
    ADD COLUMN IF NOT EXISTS preferred_language TEXT NOT NULL DEFAULT 'ar';

-- 3. Restrict preferred_language to the values allowed by the API contract.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.table_constraints
        WHERE table_schema = current_schema()
          AND table_name = 'profiles'
          AND constraint_name = 'chk_profiles_preferred_language'
    ) THEN
        ALTER TABLE profiles
            ADD CONSTRAINT chk_profiles_preferred_language
            CHECK (preferred_language IN ('ar', 'en'));
    END IF;
END
$$;
