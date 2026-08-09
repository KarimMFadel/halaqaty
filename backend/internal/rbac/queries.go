package rbac

// insertCircleQuery creates the circle row; the owner is stored in teacher_id.
const insertCircleQuery = `
INSERT INTO circles (name, teacher_id, invite_code, description, rules, max_capacity, is_private, gender_restriction, language, grading_policy)
VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10)
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

const searchUsersQuery = `
SELECT u.id::text, COALESCE(p.display_name, p.full_name)
FROM users u
JOIN profiles p ON p.user_id = u.id
WHERE COALESCE(p.display_name, p.full_name) ILIKE '%' || $1 || '%' ESCAPE E'\\'
ORDER BY COALESCE(p.display_name, p.full_name), u.id
LIMIT $2
`

const lockUserQuery = `
SELECT id::text
FROM users
WHERE id = $1::uuid
FOR UPDATE
`

// circleExistsQuery reports whether the circle row exists.
const circleExistsQuery = `
SELECT EXISTS(SELECT 1 FROM circles WHERE id = $1::uuid)
`

const findCircleByInviteCodeQuery = `
SELECT id::text, name, invite_code, description, rules, max_capacity, is_private,
       gender_restriction, language, grading_policy, is_archived, created_at
FROM circles
WHERE invite_code = $1
FOR UPDATE
`

const findCircleByIDQuery = `
SELECT id::text, name, teacher_id::text, invite_code, description, rules, max_capacity, is_private,
       gender_restriction, language, grading_policy, is_archived, created_at
FROM circles WHERE id = $1::uuid
`

const listPublicCirclesQuery = `
SELECT id::text, name, description, max_capacity, gender_restriction, language, created_at
FROM circles
WHERE is_private = FALSE AND is_archived = FALSE
  AND ($1 = '' OR name ILIKE '%' || $1 || '%')
ORDER BY created_at DESC, id DESC
LIMIT $2
`

const updateCircleQuery = `
UPDATE circles
SET name = $2, description = $3, rules = $4, max_capacity = $5, is_private = $6,
    gender_restriction = $7, language = $8, grading_policy = $9, updated_at = NOW()
WHERE id = $1::uuid
RETURNING id::text, name, teacher_id::text, invite_code, description, rules, max_capacity, is_private,
          gender_restriction, language, grading_policy, is_archived, created_at
`

const refreshInviteCodeQuery = `
UPDATE circles SET invite_code = $2, updated_at = NOW() WHERE id = $1::uuid
`

const removeCircleMemberQuery = `
DELETE FROM circle_members WHERE circle_id = $1::uuid AND user_id = $2::uuid
`

const archiveCircleQuery = `
UPDATE circles SET is_archived = TRUE, updated_at = NOW() WHERE id = $1::uuid
`

const listCircleMembersQuery = `
SELECT user_id::text, role FROM circle_members WHERE circle_id = $1::uuid ORDER BY user_id
`

const countActiveMembershipsQuery = `
SELECT COUNT(*)
FROM circle_members cm
JOIN circles c ON c.id = cm.circle_id
WHERE cm.user_id = $1::uuid
  AND c.is_archived = FALSE
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
