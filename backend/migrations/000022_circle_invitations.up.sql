CREATE TABLE circle_invitations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    circle_id UUID NOT NULL REFERENCES circles(id) ON DELETE RESTRICT,
    inviter_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    recipient_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    role VARCHAR(20) NOT NULL,
    state VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_at TIMESTAMPTZ,
    CONSTRAINT ck_circle_invitations_role CHECK (role IN ('teacher', 'student')),
    CONSTRAINT ck_circle_invitations_state CHECK (state IN ('pending', 'accepted', 'revoked', 'superseded')),
    CONSTRAINT ck_circle_invitations_distinct_users CHECK (inviter_user_id <> recipient_user_id),
    CONSTRAINT ck_circle_invitations_resolution CHECK (
        (state = 'pending' AND resolved_at IS NULL)
        OR (state <> 'pending' AND resolved_at IS NOT NULL)
    )
);

CREATE UNIQUE INDEX uq_circle_invitations_pending_recipient
    ON circle_invitations (circle_id, recipient_user_id)
    WHERE state = 'pending';

CREATE INDEX idx_circle_invitations_pending_circle
    ON circle_invitations (circle_id, created_at, id)
    WHERE state = 'pending';

CREATE INDEX idx_circle_invitations_pending_recipient
    ON circle_invitations (recipient_user_id, created_at, id)
    WHERE state = 'pending';
