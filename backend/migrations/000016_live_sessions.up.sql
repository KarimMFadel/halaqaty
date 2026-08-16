-- 000016_live_sessions.up.sql
-- F-005-owned sessions table and session_participant_presence (ADR-015, ADR-016).
-- F-005 creates only ad-hoc rows (scheduled_at IS NULL); F-006 owns scheduling
-- and attendance, F-003 owns queue concerns, each via later paired migrations.

CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    circle_id UUID NOT NULL,
    created_by UUID NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'scheduled',
    scheduled_at TIMESTAMPTZ NULL,
    actual_start TIMESTAMPTZ NULL,
    actual_end TIMESTAMPTZ NULL,
    end_reason VARCHAR(20) NULL,
    media_mode VARCHAR(20) NOT NULL DEFAULT 'audio_only',
    media_room_ref VARCHAR(200) NULL,
    is_locked BOOLEAN NOT NULL DEFAULT FALSE,
    participant_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_sessions_circle_id FOREIGN KEY (circle_id) REFERENCES circles(id) ON DELETE RESTRICT,
    CONSTRAINT fk_sessions_created_by FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT ck_sessions_status CHECK (status IN ('scheduled', 'active', 'ended')),
    CONSTRAINT ck_sessions_end_reason
        CHECK (end_reason IS NULL OR end_reason IN ('manual', 'duration_limit', 'idle_timeout')),
    CONSTRAINT ck_sessions_media_mode CHECK (media_mode IN ('audio_only', 'audio_video')),
    CONSTRAINT ck_sessions_participant_count CHECK (participant_count BETWEEN 0 AND 50),
    CONSTRAINT uq_sessions_media_room_ref UNIQUE (media_room_ref)
);

CREATE TABLE IF NOT EXISTS session_participant_presence (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL,
    user_id UUID NOT NULL,
    first_joined_at TIMESTAMPTZ NULL,
    last_joined_at TIMESTAMPTZ NULL,
    last_left_at TIMESTAMPTZ NULL,
    reconnect_count INTEGER NOT NULL DEFAULT 0,
    is_currently_present BOOLEAN NOT NULL DEFAULT FALSE,
    removed_at TIMESTAMPTZ NULL,
    hand_raised_at TIMESTAMPTZ NULL,
    CONSTRAINT fk_session_participant_presence_session_id
        FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE RESTRICT,
    CONSTRAINT fk_session_participant_presence_user_id
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE RESTRICT,
    CONSTRAINT uq_session_participant_presence_session_user UNIQUE (session_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_sessions_circle_status ON sessions (circle_id, status);
CREATE INDEX IF NOT EXISTS idx_session_presence_current
    ON session_participant_presence (session_id)
    WHERE is_currently_present;
