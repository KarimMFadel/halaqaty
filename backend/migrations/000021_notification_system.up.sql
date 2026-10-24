-- F-008 recipient history, preferences, and durable FCM dispatch state (ADR-027).

CREATE TABLE device_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    token TEXT NOT NULL,
    platform VARCHAR(16) NOT NULL,
    device_label VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_device_tokens_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT uq_device_tokens_user_token UNIQUE (user_id, token),
    CONSTRAINT ck_device_tokens_token CHECK (token = btrim(token) AND token <> ''),
    CONSTRAINT ck_device_tokens_platform CHECK (platform IN ('android', 'ios', 'web')),
    CONSTRAINT ck_device_tokens_device_label CHECK (
        device_label IS NULL OR (device_label = btrim(device_label) AND device_label <> '')
    )
);

CREATE FUNCTION remove_closed_account_device_tokens()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF OLD.deleted_at IS NULL AND NEW.deleted_at IS NOT NULL THEN
        DELETE FROM device_tokens WHERE user_id = NEW.id;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER trg_remove_closed_account_device_tokens
AFTER UPDATE OF deleted_at ON users
FOR EACH ROW
EXECUTE FUNCTION remove_closed_account_device_tokens();

CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    type VARCHAR(40) NOT NULL,
    source_event_id VARCHAR(200) NOT NULL,
    source_ref_type VARCHAR(32) NOT NULL,
    source_ref_id UUID NOT NULL,
    title VARCHAR(120) NOT NULL,
    body VARCHAR(240) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    push_expires_at TIMESTAMPTZ NOT NULL,
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    read_at TIMESTAMPTZ,
    preference_override BOOLEAN NOT NULL DEFAULT FALSE,
    delivery_state VARCHAR(32) NOT NULL DEFAULT 'pending_no_device',
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempt_count SMALLINT NOT NULL DEFAULT 0,
    last_error_class VARCHAR(64),
    CONSTRAINT uq_notifications_user_type_source UNIQUE (user_id, type, source_event_id),
    CONSTRAINT fk_notifications_user_id FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT ck_notifications_type CHECK (type IN (
        'session_reminder', 'group_message', 'private_message', 'grade_posted',
        'circle_invitation', 'queue_next_soon', 'queue_your_turn',
        'student_joined_session', 'student_joined_circle', 'circle_archived',
        'session_started', 'session_ended'
    )),
    CONSTRAINT ck_notifications_source_event_id CHECK (
        source_event_id = btrim(source_event_id) AND source_event_id <> ''
    ),
    CONSTRAINT ck_notifications_source_ref_type CHECK (
        source_ref_type IN ('session', 'message', 'recitation_log', 'circle', 'invitation', 'queue')
    ),
    CONSTRAINT ck_notifications_display_projection CHECK (
        title = btrim(title) AND title <> '' AND body = btrim(body) AND body <> ''
    ),
    CONSTRAINT ck_notifications_read_state CHECK (
        (NOT is_read AND read_at IS NULL) OR (is_read AND read_at IS NOT NULL)
    ),
    CONSTRAINT ck_notifications_delivery_state CHECK (delivery_state IN (
        'pending_no_device', 'pending_attempt', 'accepted_by_provider',
        'suppressed_preference', 'suppressed_quiet_hours', 'expired', 'exhausted'
    )),
    CONSTRAINT ck_notifications_attempt_count CHECK (attempt_count BETWEEN 0 AND 5)
);

CREATE INDEX idx_notifications_user_history
    ON notifications (user_id, created_at DESC, id DESC);
CREATE INDEX idx_notifications_user_unread
    ON notifications (user_id, created_at DESC) WHERE NOT is_read;
CREATE INDEX idx_notifications_dispatch_due
    ON notifications (next_attempt_at, created_at)
    WHERE delivery_state IN ('pending_no_device', 'pending_attempt');
CREATE INDEX idx_notifications_retention
    ON notifications (created_at);

CREATE TABLE notification_device_attempts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    device_token_id UUID NOT NULL REFERENCES device_tokens(id) ON DELETE CASCADE,
    state VARCHAR(32) NOT NULL DEFAULT 'pending',
    attempt_count SMALLINT NOT NULL DEFAULT 0,
    next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_error_class VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_notification_device_attempts_notification_device
        UNIQUE (notification_id, device_token_id),
    CONSTRAINT ck_notification_device_attempts_state CHECK (state IN (
        'pending', 'accepted_by_provider', 'suppressed_preference',
        'suppressed_quiet_hours', 'invalid_token', 'exhausted'
    )),
    CONSTRAINT ck_notification_device_attempts_count CHECK (attempt_count BETWEEN 0 AND 5)
);

CREATE INDEX idx_notification_device_attempts_due
    ON notification_device_attempts (next_attempt_at, created_at)
    WHERE state = 'pending';

CREATE TABLE notification_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    notification_type VARCHAR(40),
    enabled BOOLEAN,
    quiet_hours_start TIME,
    quiet_hours_end TIME,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_notification_preferences_user_id
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    CONSTRAINT ck_notification_preferences_shape CHECK (
        (notification_type IS NULL AND enabled IS NOT NULL
            AND quiet_hours_start IS NOT NULL AND quiet_hours_end IS NOT NULL
            AND quiet_hours_start <> quiet_hours_end)
        OR
        (notification_type IS NOT NULL AND enabled IS NOT NULL
            AND quiet_hours_start IS NULL AND quiet_hours_end IS NULL
            AND notification_type IN (
                'session_reminder', 'group_message', 'grade_posted',
                'student_joined_session', 'student_joined_circle', 'circle_archived',
                'session_started', 'session_ended'
            ))
    )
);

CREATE UNIQUE INDEX uq_notification_preferences_user_type
    ON notification_preferences (user_id, notification_type)
    WHERE notification_type IS NOT NULL;
CREATE UNIQUE INDEX uq_notification_preferences_user_quiet_hours
    ON notification_preferences (user_id)
    WHERE notification_type IS NULL;
