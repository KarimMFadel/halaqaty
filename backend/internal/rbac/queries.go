package rbac

// circleMemberRoleQuery returns one active membership role, including
// membership in an archived circle. It backs membership-only authorization
// such as moderation and reads of a lingering active session.
const circleMemberRoleQuery = `
SELECT role
FROM circle_members
WHERE circle_id = $1::uuid AND user_id = $2::uuid
LIMIT 1`

// activeCircleMemberRoleQuery returns the membership role only while the
// circle has not been archived; archived circles answer with no row so
// new-activity authorization fails closed (ADR-011, F-005 edge cases).
const activeCircleMemberRoleQuery = `
SELECT cm.role
FROM circle_members cm
JOIN circles c ON c.id = cm.circle_id
WHERE cm.circle_id = $1::uuid AND cm.user_id = $2::uuid
  AND c.is_archived = FALSE
LIMIT 1`

const circleIDsForUserQuery = `
SELECT circle_id::text
FROM circle_members
WHERE user_id = $1::uuid
ORDER BY circle_id`

// insertCircleQuery creates the circle row; the owner is stored in teacher_id.
const insertCircleQuery = `
INSERT INTO circles (name, teacher_id, invite_code, description, rules, max_capacity, is_private, gender_restriction, language, grading_policy)
VALUES ($1, $2::uuid, $3, $4, $5, $6, $7, $8, $9, $10)
RETURNING id::text, created_at
`

const listUserCirclesQuery = `
SELECT c.id::text, c.name, c.description, c.max_capacity,
       c.gender_restriction, c.language, c.created_at
FROM circle_members cm
JOIN circles c ON c.id = cm.circle_id
WHERE cm.user_id = $1::uuid
  AND c.is_archived = FALSE
  AND ($2 = '' OR cm.role = $2)
ORDER BY cm.joined_at ASC, c.id ASC
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
WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL
`

const searchUsersQuery = `
SELECT u.id::text, COALESCE(p.display_name, p.full_name)
FROM users u
JOIN profiles p ON p.user_id = u.id
WHERE u.deleted_at IS NULL
  AND COALESCE(p.display_name, p.full_name) ILIKE '%' || $1 || '%' ESCAPE E'\\'
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

const findCircleByIDForUpdateQuery = `
SELECT id::text, name, teacher_id::text, invite_code, description, rules, max_capacity, is_private,
       gender_restriction, language, grading_policy, is_archived, created_at
FROM circles WHERE id = $1::uuid FOR UPDATE
`

const listPublicCirclesQuery = `
SELECT id::text, name, description, max_capacity, gender_restriction, language, created_at
FROM circles
WHERE is_private = FALSE AND is_archived = FALSE
  AND ($1 = '' OR name ILIKE '%' || $1 || '%' ESCAPE E'\\')
  AND ($2 = '' OR id < $2::uuid)
ORDER BY id DESC
LIMIT $3
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

// isMemberQuery checks if a user belongs to a circle, returning their role.
const isMemberQuery = `
SELECT role
FROM circle_members
WHERE circle_id = $1::uuid AND user_id = $2::uuid
LIMIT 1
`

const membershipStartedAtQuery = `SELECT joined_at FROM circle_members WHERE circle_id = $1::uuid AND user_id = $2::uuid LIMIT 1`

const listCircleMembersQuery = `
SELECT
    cm.user_id::text,
    COALESCE(NULLIF(BTRIM(p.display_name), ''), NULLIF(BTRIM(p.full_name), ''), 'Member'),
    cm.role,
    cm.joined_at
FROM circle_members cm
LEFT JOIN profiles p ON p.user_id = cm.user_id
WHERE cm.circle_id = $1::uuid
ORDER BY cm.joined_at ASC
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

const setCircleTeacherIDQuery = `
UPDATE circles
SET teacher_id = $2::uuid, updated_at = NOW()
WHERE id = $1::uuid
`

const supersedePendingCircleInvitationQuery = `
UPDATE circle_invitations
SET state = 'superseded', resolved_at = NOW()
WHERE circle_id = $1::uuid
  AND recipient_user_id = $2::uuid
  AND state = 'pending'
`

const insertCircleInvitationQuery = `
INSERT INTO circle_invitations (circle_id, inviter_user_id, recipient_user_id, role)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4)
RETURNING id::text, circle_id::text, inviter_user_id::text, recipient_user_id::text,
          role, state, created_at, resolved_at
`

const listPendingCircleInvitationsQuery = `
SELECT id::text, circle_id::text, inviter_user_id::text, recipient_user_id::text,
       role, state, created_at, resolved_at
FROM circle_invitations
WHERE circle_id = $1::uuid AND state = 'pending'
ORDER BY created_at DESC, id DESC
`

const listPendingUserInvitationsQuery = `
SELECT i.id::text, i.circle_id::text, i.inviter_user_id::text, i.recipient_user_id::text,
       i.role, i.state, i.created_at, i.resolved_at
FROM circle_invitations i
JOIN circles c ON c.id = i.circle_id
WHERE i.recipient_user_id = $1::uuid AND i.state = 'pending' AND c.is_archived = FALSE
ORDER BY i.created_at DESC, i.id DESC
`

const findCircleInvitationQuery = `
SELECT id::text, circle_id::text, inviter_user_id::text, recipient_user_id::text,
       role, state, created_at, resolved_at
FROM circle_invitations
WHERE id = $1::uuid
`

const lockCircleInvitationQuery = `
SELECT id::text, circle_id::text, inviter_user_id::text, recipient_user_id::text,
       role, state, created_at, resolved_at
FROM circle_invitations
WHERE circle_id = $1::uuid AND id = $2::uuid
FOR UPDATE
`

const setCircleInvitationStateQuery = `
UPDATE circle_invitations
SET state = $2, resolved_at = NOW()
WHERE id = $1::uuid AND state = 'pending'
`
