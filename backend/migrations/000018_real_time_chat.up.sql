-- Feature 004: PostgreSQL-authoritative real-time chat persistence.

CREATE OR REPLACE FUNCTION halaqaty_normalize_arabic(input TEXT)
RETURNS TEXT
LANGUAGE SQL
IMMUTABLE
STRICT
PARALLEL SAFE
AS $$
    SELECT translate(
        regexp_replace(normalize(lower(input), NFC), '[ؐ-ؚـً-ٰٟۖ-ۭ]', '', 'g'),
        'أإآٱى',
        'ااااي'
    );
$$;

CREATE TABLE IF NOT EXISTS chat_uploads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    uploader_id UUID NOT NULL,
    authorization_circle_id UUID NOT NULL,
    dm_peer_id UUID,
    object_key TEXT NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    original_file_name VARCHAR(255) NOT NULL,
    size_bytes BIGINT NOT NULL,
    duration_seconds INTEGER,
    state VARCHAR(16) NOT NULL DEFAULT 'staged',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_chat_uploads_object_key UNIQUE (object_key),
    CONSTRAINT fk_chat_uploads_uploader_id
        FOREIGN KEY (uploader_id) REFERENCES users(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT fk_chat_uploads_authorization_circle_id
        FOREIGN KEY (authorization_circle_id) REFERENCES circles(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT fk_chat_uploads_dm_peer_id
        FOREIGN KEY (dm_peer_id) REFERENCES users(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT ck_chat_uploads_dm_peer CHECK (dm_peer_id IS NULL OR dm_peer_id <> uploader_id),
    CONSTRAINT ck_chat_uploads_object_key CHECK (object_key = btrim(object_key) AND object_key <> ''),
    CONSTRAINT ck_chat_uploads_mime_type CHECK (mime_type = btrim(mime_type) AND mime_type <> ''),
    CONSTRAINT ck_chat_uploads_original_file_name CHECK (
        original_file_name = btrim(original_file_name) AND original_file_name <> ''
    ),
    CONSTRAINT ck_chat_uploads_media_limits CHECK (
        (mime_type IN ('image/jpeg', 'image/png') AND size_bytes BETWEEN 1 AND 5242880 AND duration_seconds IS NULL)
        OR (mime_type = 'application/pdf' AND size_bytes BETWEEN 1 AND 10485760 AND duration_seconds IS NULL)
        OR (mime_type LIKE 'audio/%' AND size_bytes BETWEEN 1 AND 20971520 AND duration_seconds BETWEEN 1 AND 300)
    ),
    CONSTRAINT ck_chat_uploads_state CHECK (state IN ('staged', 'attached', 'revoked'))
);

CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    circle_id UUID,
    dm_recipient_id UUID,
    sender_id UUID NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    message_type VARCHAR(20) NOT NULL,
    content TEXT,
    upload_id UUID,
    reply_to_id UUID,
    is_pinned BOOLEAN NOT NULL DEFAULT FALSE,
    pinned_by UUID,
    pinned_at TIMESTAMPTZ,
    sent_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    search_vector TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('simple'::regconfig, halaqaty_normalize_arabic(COALESCE(content, '')))
    ) STORED,
    CONSTRAINT uq_messages_sender_idempotency_key UNIQUE (sender_id, idempotency_key),
    CONSTRAINT uq_messages_upload_id UNIQUE (upload_id),
    CONSTRAINT fk_messages_circle_id
        FOREIGN KEY (circle_id) REFERENCES circles(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT fk_messages_dm_recipient_id
        FOREIGN KEY (dm_recipient_id) REFERENCES users(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT fk_messages_sender_id
        FOREIGN KEY (sender_id) REFERENCES users(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT fk_messages_upload_id
        FOREIGN KEY (upload_id) REFERENCES chat_uploads(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT fk_messages_reply_to_id
        FOREIGN KEY (reply_to_id) REFERENCES messages(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT fk_messages_pinned_by
        FOREIGN KEY (pinned_by) REFERENCES users(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT ck_messages_context CHECK ((circle_id IS NULL) <> (dm_recipient_id IS NULL)),
    CONSTRAINT ck_messages_no_self_dm CHECK (dm_recipient_id IS NULL OR dm_recipient_id <> sender_id),
    CONSTRAINT ck_messages_idempotency_key CHECK (
        idempotency_key = btrim(idempotency_key) AND idempotency_key <> ''
    ),
    CONSTRAINT ck_messages_type CHECK (message_type IN ('text', 'voice', 'image', 'file')),
    CONSTRAINT ck_messages_payload CHECK (
        (message_type = 'text'
            AND content IS NOT NULL
            AND content = btrim(content)
            AND char_length(content) BETWEEN 1 AND 4000
            AND upload_id IS NULL)
        OR (message_type IN ('voice', 'image', 'file') AND content IS NULL AND upload_id IS NOT NULL)
    ),
    CONSTRAINT ck_messages_pin_state CHECK (
        (NOT is_pinned AND pinned_by IS NULL AND pinned_at IS NULL)
        OR (is_pinned
            AND circle_id IS NOT NULL
            AND pinned_by IS NOT NULL
            AND pinned_at IS NOT NULL
            AND deleted_at IS NULL)
    )
);

CREATE TABLE IF NOT EXISTS message_reads (
    message_id UUID NOT NULL,
    user_id UUID NOT NULL,
    read_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT pk_message_reads PRIMARY KEY (message_id, user_id),
    CONSTRAINT fk_message_reads_message_id
        FOREIGN KEY (message_id) REFERENCES messages(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT fk_message_reads_user_id
        FOREIGN KEY (user_id) REFERENCES users(id) ON UPDATE NO ACTION ON DELETE NO ACTION
);

CREATE TABLE IF NOT EXISTS chat_event_outbox (
    event_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL,
    event_type VARCHAR(48) NOT NULL,
    recipient_id UUID,
    available_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ,
    attempt_count INTEGER NOT NULL DEFAULT 0,
    parked_at TIMESTAMPTZ,
    CONSTRAINT fk_chat_event_outbox_message_id
        FOREIGN KEY (message_id) REFERENCES messages(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT fk_chat_event_outbox_recipient_id
        FOREIGN KEY (recipient_id) REFERENCES users(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT ck_chat_event_outbox_event_type CHECK (
        event_type IN ('chat.message', 'chat.message_read', 'chat.message_deleted')
    ),
    CONSTRAINT ck_chat_event_outbox_attempt_count CHECK (attempt_count BETWEEN 0 AND 5),
    CONSTRAINT ck_chat_event_outbox_terminal_state CHECK (
        NOT (delivered_at IS NOT NULL AND parked_at IS NOT NULL)
        AND (parked_at IS NULL OR attempt_count = 5)
    )
);

CREATE TABLE IF NOT EXISTS message_moderation_audits (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL,
    circle_id UUID NOT NULL,
    actor_id UUID NOT NULL,
    action VARCHAR(32) NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_message_moderation_audits_message_id
        FOREIGN KEY (message_id) REFERENCES messages(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT fk_message_moderation_audits_circle_id
        FOREIGN KEY (circle_id) REFERENCES circles(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT fk_message_moderation_audits_actor_id
        FOREIGN KEY (actor_id) REFERENCES users(id) ON UPDATE NO ACTION ON DELETE NO ACTION,
    CONSTRAINT ck_message_moderation_audits_action CHECK (action = 'teacher_delete')
);

CREATE INDEX IF NOT EXISTS idx_messages_circle_history
    ON messages (circle_id, sent_at DESC, id DESC)
    WHERE deleted_at IS NULL AND circle_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_messages_dm_history
    ON messages (sender_id, dm_recipient_id, sent_at DESC, id DESC)
    WHERE deleted_at IS NULL AND circle_id IS NULL;

CREATE INDEX IF NOT EXISTS idx_messages_reply_to_id ON messages (reply_to_id);

CREATE INDEX IF NOT EXISTS idx_messages_pinned
    ON messages (circle_id, pinned_at DESC, id DESC)
    WHERE is_pinned AND deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_messages_search_vector
    ON messages USING GIN (search_vector)
    WHERE deleted_at IS NULL AND message_type = 'text' AND circle_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_chat_uploads_uploader_created_at
    ON chat_uploads (uploader_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_chat_uploads_staged_created_at
    ON chat_uploads (created_at)
    WHERE state = 'staged';

CREATE INDEX IF NOT EXISTS idx_message_reads_user_id ON message_reads (user_id);

CREATE INDEX IF NOT EXISTS idx_chat_event_outbox_dispatch
    ON chat_event_outbox (available_at, event_id)
    WHERE delivered_at IS NULL AND parked_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_message_moderation_audits_message_id
    ON message_moderation_audits (message_id);
