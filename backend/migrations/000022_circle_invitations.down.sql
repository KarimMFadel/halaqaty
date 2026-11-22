DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM circle_invitations) THEN
        RAISE EXCEPTION 'cannot roll back circle invitations while invitation history exists';
    END IF;
END
$$;

DROP TABLE circle_invitations;
