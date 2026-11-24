DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM notification_device_attempts)
       OR EXISTS (SELECT 1 FROM notifications)
       OR EXISTS (SELECT 1 FROM device_tokens)
       OR EXISTS (SELECT 1 FROM notification_preferences) THEN
        RAISE EXCEPTION 'cannot roll back notification migration while notification data exists';
    END IF;
END
$$;

DROP TRIGGER IF EXISTS trg_remove_closed_account_device_tokens ON users;
DROP FUNCTION IF EXISTS remove_closed_account_device_tokens();
DROP TABLE notification_device_attempts;
DROP TABLE notifications;
DROP TABLE device_tokens;
DROP TABLE notification_preferences;
