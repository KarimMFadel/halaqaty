-- Roll back only Feature 004-owned chat objects.

DROP TABLE IF EXISTS message_moderation_audits;
DROP TABLE IF EXISTS chat_event_outbox;
DROP TABLE IF EXISTS message_reads;
DROP TABLE IF EXISTS messages;
DROP TABLE IF EXISTS chat_uploads;

DROP FUNCTION IF EXISTS halaqaty_normalize_arabic(TEXT);
