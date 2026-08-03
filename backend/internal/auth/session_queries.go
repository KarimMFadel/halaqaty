package auth

// upsertUserByFirebaseUIDQuery inserts a new user or refreshes the email on
// replay. The inserted flag uses the xmax = 0 idiom: a freshly inserted row
// has no transaction ID stamped on it, an updated (conflict) row does.
const upsertUserByFirebaseUIDQuery = `
INSERT INTO users (firebase_uid, email) VALUES ($1, $2)
ON CONFLICT (firebase_uid) DO UPDATE SET
    email = EXCLUDED.email,
    updated_at = NOW()
RETURNING id, firebase_uid, email, created_at, updated_at, (xmax = 0) AS inserted
`

// upsertProfileOnRegisterQuery writes the registration profile fields once.
// Replays (same Firebase UID) must not overwrite an existing profile.
const upsertProfileOnRegisterQuery = `
INSERT INTO profiles (user_id, display_name, preferred_language)
VALUES ($1, $2, $3)
ON CONFLICT (user_id) DO NOTHING
`

// getUserProfileByUserIDQuery projects the API UserProfile from users+profiles.
// The LEFT JOIN keeps pre-profile users readable; preferred_language falls
// back to the contract default.
const getUserProfileByUserIDQuery = `
SELECT
    u.id,
    u.firebase_uid,
    p.full_name,
    p.display_name,
    p.bio,
    p.country,
    p.avatar_url,
    p.phone,
    COALESCE(p.preferred_language, 'ar') AS preferred_language,
    u.created_at
FROM users u
LEFT JOIN profiles p ON p.user_id = u.id
WHERE u.id = $1
`

const getUserByFirebaseUIDQuery = `
SELECT id, firebase_uid, email, created_at, updated_at
FROM users
WHERE firebase_uid = $1
`

const getUserByEmailQuery = `
SELECT id, firebase_uid, email, created_at, updated_at
FROM users
WHERE email = $1
`

const createEmptyProfileQuery = `
INSERT INTO profiles (user_id) VALUES ($1)
ON CONFLICT (user_id) DO NOTHING
`

const createSessionQuery = `
INSERT INTO user_sessions (
    session_id,
    user_id,
    device_name,
    last_activity_at,
    expires_at,
    revoked_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, NULL, NOW())
`

const getSessionByIDQuery = `
SELECT
    session_id,
    user_id,
    device_name,
    last_activity_at,
    expires_at,
    revoked_at,
    created_at,
    updated_at
FROM user_sessions
WHERE session_id = $1
`

const getSessionByIDAndUserIDQuery = `
SELECT
    session_id,
    user_id,
    device_name,
    last_activity_at,
    expires_at,
    revoked_at,
    created_at,
    updated_at
FROM user_sessions
WHERE session_id = $1
  AND user_id = $2
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
