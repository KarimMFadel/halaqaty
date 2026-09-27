package auth

// upsertUserByFirebaseUIDQuery inserts a new user or refreshes the email on
// replay. The inserted flag uses the xmax = 0 idiom: a freshly inserted row
// has no transaction ID stamped on it, an updated (conflict) row does.
const upsertUserByFirebaseUIDQuery = `
INSERT INTO users (firebase_uid, email)
SELECT $1, $2
WHERE NOT EXISTS (SELECT 1 FROM users WHERE deleted_firebase_uid_hash = $3)
ON CONFLICT (firebase_uid) DO UPDATE SET
    email = EXCLUDED.email,
    updated_at = NOW()
WHERE users.deleted_at IS NULL
RETURNING id, firebase_uid, email, created_at, updated_at, (xmax = 0) AS inserted
`

const lockActiveAccountQuery = `
SELECT firebase_uid
FROM users
WHERE id = $1::uuid AND deleted_at IS NULL
FOR UPDATE
`

const lockFirebaseUIDQuery = `
SELECT pg_advisory_xact_lock(hashtextextended($1, 0))
`

const getFirebaseUIDForActiveAccountQuery = `
SELECT firebase_uid
FROM users
WHERE id = $1::uuid AND deleted_at IS NULL
`

const hasActiveManagerCircleQuery = `
SELECT EXISTS (
    SELECT 1
    FROM circle_members cm
    JOIN circles c ON c.id = cm.circle_id
    WHERE cm.user_id = $1::uuid
      AND NOT c.is_archived
      AND cm.role IN ('teacher', 'supervisor')
) OR EXISTS (
    SELECT 1 FROM circles
    WHERE teacher_id = $1::uuid AND NOT is_archived
)
`

const hasActiveSessionParticipantQuery = `
SELECT EXISTS (
    SELECT 1
    FROM session_participant_presence p
    JOIN sessions s ON s.id = p.session_id
    WHERE p.user_id = $1::uuid
      AND p.removed_at IS NULL
      AND p.is_currently_present
      AND s.status = 'active'
)
`

const closeAccountQuery = `
UPDATE users
SET deleted_at = NOW(), email = NULL, deleted_firebase_uid_hash = $2, updated_at = NOW()
WHERE id = $1::uuid AND deleted_at IS NULL
`

const scrubClosedProfileQuery = `
UPDATE profiles
SET full_name = NULL,
    country = NULL,
    phone = NULL,
    bio = NULL,
    avatar_url = NULL,
    completed_at = NULL,
    preferred_language = 'ar',
    updated_at = NOW()
WHERE user_id = $1::uuid
`

const deleteUserSessionsQuery = `
DELETE FROM user_sessions WHERE user_id = $1::uuid
`

const clearDeletedAccountFirebaseUIDQuery = `
UPDATE users
SET firebase_uid = NULL, updated_at = NOW()
WHERE id = $1::uuid AND firebase_uid = $2 AND deleted_at IS NOT NULL
`

const listPendingFirebaseDeletionsQuery = `
SELECT id::text, firebase_uid
FROM users
WHERE deleted_at IS NOT NULL AND firebase_uid IS NOT NULL
  AND ($1::uuid IS NULL OR id > $1::uuid)
ORDER BY id
LIMIT $2
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
  AND u.deleted_at IS NULL
`

const getUserByFirebaseUIDQuery = `
SELECT id, firebase_uid, email, created_at, updated_at
FROM users
WHERE firebase_uid = $1 AND deleted_at IS NULL
`

const getUserByEmailQuery = `
SELECT id, firebase_uid, email, created_at, updated_at
FROM users
WHERE email = $1 AND deleted_at IS NULL
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
WHERE firebase_uid = $1 AND deleted_at IS NULL
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
SELECT cm.role
FROM circle_members cm
JOIN users u ON u.id = cm.user_id
WHERE cm.circle_id = $1::uuid
  AND cm.user_id = $2::uuid
  AND u.deleted_at IS NULL
`
