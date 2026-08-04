-- 000014_circle_members_circle_fk.up.sql
-- Enforces circle membership integrity once circles exists in all environments.
-- Remediation for legacy data: remove orphan memberships created before the FK.
DELETE FROM circle_members cm
WHERE NOT EXISTS (
    SELECT 1
    FROM circles c
    WHERE c.id = cm.circle_id
);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.table_constraints tc
        JOIN information_schema.key_column_usage kcu
          ON tc.constraint_name = kcu.constraint_name
         AND tc.table_schema = kcu.table_schema
        JOIN information_schema.constraint_column_usage ccu
          ON tc.constraint_name = ccu.constraint_name
         AND tc.table_schema = ccu.table_schema
        WHERE tc.table_schema = current_schema()
          AND tc.table_name = 'circle_members'
          AND tc.constraint_type = 'FOREIGN KEY'
          AND kcu.column_name = 'circle_id'
          AND ccu.table_name = 'circles'
          AND ccu.column_name = 'id'
    ) THEN
        ALTER TABLE circle_members
            ADD CONSTRAINT fk_circle_members_circle_id_000014
            FOREIGN KEY (circle_id) REFERENCES circles(id) ON DELETE CASCADE;
    END IF;
END
$$;
