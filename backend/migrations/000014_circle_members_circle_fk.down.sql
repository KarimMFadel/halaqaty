-- 000014_circle_members_circle_fk.down.sql
ALTER TABLE circle_members
    DROP CONSTRAINT IF EXISTS fk_circle_members_circle_id_000014;
