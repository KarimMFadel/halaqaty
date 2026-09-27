DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE deleted_at IS NOT NULL) THEN
        RAISE EXCEPTION 'cannot roll back account deletion migration while tombstones exist';
    END IF;
END
$$;

ALTER TABLE users
    DROP COLUMN deleted_firebase_uid_hash,
    DROP COLUMN deleted_at,
    ALTER COLUMN firebase_uid SET NOT NULL,
    ALTER COLUMN email SET NOT NULL;
