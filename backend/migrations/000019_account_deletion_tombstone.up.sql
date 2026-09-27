ALTER TABLE users
    ALTER COLUMN firebase_uid DROP NOT NULL,
    ALTER COLUMN email DROP NOT NULL,
    ADD COLUMN deleted_at TIMESTAMPTZ,
    ADD COLUMN deleted_firebase_uid_hash BYTEA UNIQUE;
