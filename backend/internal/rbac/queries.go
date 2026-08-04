package rbac

// insertCircleQuery creates the circle row; the owner is stored in teacher_id.
const insertCircleQuery = `
INSERT INTO circles (name, teacher_id, invite_code)
VALUES ($1, $2::uuid, $3)
RETURNING id::text, created_at
`

// insertCircleMemberQuery adds one membership; replays keep the existing role.
const insertCircleMemberQuery = `
INSERT INTO circle_members (circle_id, user_id, role)
VALUES ($1::uuid, $2::uuid, $3)
ON CONFLICT (circle_id, user_id) DO NOTHING
`

// usersExistQuery resolves which of the candidate user IDs are registered.
const usersExistQuery = `
SELECT id::text
FROM users
WHERE id = ANY($1::uuid[])
`

// circleExistsQuery reports whether the circle row exists.
const circleExistsQuery = `
SELECT EXISTS(SELECT 1 FROM circles WHERE id = $1::uuid)
`

// lockCircleMembersQuery locks the full membership set of one circle for a
// role-change transaction. ORDER BY keeps lock acquisition deterministic so
// concurrent role changes serialize instead of deadlocking.
const lockCircleMembersQuery = `
SELECT user_id::text, role
FROM circle_members
WHERE circle_id = $1::uuid
ORDER BY user_id
FOR UPDATE
`

// updateCircleMemberRoleQuery applies a validated role change.
const updateCircleMemberRoleQuery = `
UPDATE circle_members
SET role = $3, updated_at = NOW()
WHERE circle_id = $1::uuid
  AND user_id = $2::uuid
`
