DROP INDEX IF EXISTS idx_circles_public_active;

ALTER TABLE circles
    DROP CONSTRAINT IF EXISTS circles_rules_length_check,
    DROP CONSTRAINT IF EXISTS circles_description_length_check,
    DROP CONSTRAINT IF EXISTS circles_grading_policy_check,
    DROP CONSTRAINT IF EXISTS circles_gender_restriction_check,
    DROP CONSTRAINT IF EXISTS circles_max_capacity_check;

ALTER TABLE circles
    DROP COLUMN IF EXISTS grading_policy,
    DROP COLUMN IF EXISTS language,
    DROP COLUMN IF EXISTS gender_restriction,
    DROP COLUMN IF EXISTS is_private,
    DROP COLUMN IF EXISTS max_capacity,
    DROP COLUMN IF EXISTS rules,
    DROP COLUMN IF EXISTS description;
