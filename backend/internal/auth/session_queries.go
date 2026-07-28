package auth

const upsertSessionQuery = `
INSERT INTO user_sessions (
    session_id,
    user_id,
    last_activity_at,
    expires_at,
    revoked_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, NOW())
ON CONFLICT (session_id)
DO UPDATE SET
    last_activity_at = EXCLUDED.last_activity_at,
    expires_at = EXCLUDED.expires_at,
    revoked_at = EXCLUDED.revoked_at,
    updated_at = NOW()
`

const getSessionByIDQuery = `
SELECT
    session_id,
    user_id,
    last_activity_at,
    COALESCE(expires_at, TIMESTAMPTZ 'epoch'),
    revoked_at
FROM user_sessions
WHERE session_id = $1
`

const getLocalUserIDByFirebaseUIDQuery = `
SELECT id::text
FROM users
WHERE firebase_uid = $1
`

const touchSessionQuery = `
UPDATE user_sessions
SET
    last_activity_at = $2,
    updated_at = NOW()
WHERE session_id = $1
`

const revokeSessionQuery = `
UPDATE user_sessions
SET
    revoked_at = $2,
    updated_at = NOW()
WHERE session_id = $1
`

const getCircleMemberRoleQuery = `
SELECT role
FROM circle_members
WHERE circle_id = $1::uuid
  AND user_id = $2::uuid
`
