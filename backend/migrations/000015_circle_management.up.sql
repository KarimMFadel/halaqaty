-- Additive Circle Management fields. Existing circle rows keep their identity
-- and receive the approved MVP defaults.
ALTER TABLE circles
    ADD COLUMN IF NOT EXISTS description TEXT,
    ADD COLUMN IF NOT EXISTS rules TEXT,
    ADD COLUMN IF NOT EXISTS max_capacity INTEGER NOT NULL DEFAULT 50,
    ADD COLUMN IF NOT EXISTS is_private BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS gender_restriction VARCHAR(20) NOT NULL DEFAULT 'unspecified',
    ADD COLUMN IF NOT EXISTS language VARCHAR(10) NOT NULL DEFAULT 'ar',
    ADD COLUMN IF NOT EXISTS grading_policy VARCHAR(20) NOT NULL DEFAULT 'required';

ALTER TABLE circles
    DROP CONSTRAINT IF EXISTS circles_max_capacity_check,
    ADD CONSTRAINT circles_max_capacity_check CHECK (max_capacity BETWEEN 2 AND 200),
    DROP CONSTRAINT IF EXISTS circles_gender_restriction_check,
    ADD CONSTRAINT circles_gender_restriction_check
        CHECK (gender_restriction IN ('male', 'female', 'mixed', 'unspecified')),
    DROP CONSTRAINT IF EXISTS circles_grading_policy_check,
    ADD CONSTRAINT circles_grading_policy_check
        CHECK (grading_policy IN ('required', 'optional')),
    DROP CONSTRAINT IF EXISTS circles_description_length_check,
    ADD CONSTRAINT circles_description_length_check CHECK (description IS NULL OR char_length(description) <= 500),
    DROP CONSTRAINT IF EXISTS circles_rules_length_check,
    ADD CONSTRAINT circles_rules_length_check CHECK (rules IS NULL OR char_length(rules) <= 1000);

CREATE INDEX IF NOT EXISTS idx_circles_public_active
    ON circles (created_at DESC)
    WHERE is_private = FALSE AND is_archived = FALSE;
