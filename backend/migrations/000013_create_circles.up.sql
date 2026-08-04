-- 000013_create_circles.up.sql
-- Minimal circles parent table for circle role management (US3, ADR-010).
-- Only the columns this feature needs are created here; the remaining columns
-- from ARCHITECTURE.md (description, rules, max_capacity, is_private,
-- gender_restriction, language, grading_policy) arrive with their own features.
CREATE TABLE circles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    teacher_id UUID NOT NULL REFERENCES users(id),
    invite_code VARCHAR(20) NOT NULL UNIQUE,
    is_archived BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
