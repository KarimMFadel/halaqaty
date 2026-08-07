package profile

const getProfileByUserIDQuery = `
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
    u.created_at,
    p.completed_at
FROM users u
LEFT JOIN profiles p ON p.user_id = u.id
WHERE u.id = $1
`

// updateProfileFieldsByUserIDQuery updates only supplied fields via COALESCE.
// Parameters: $1=user_id, $2=full_name, $3=display_name, $4=bio, $5=country,
//
//	$6=avatar_url, $7=phone, $8=preferred_language, $9=completed_at.
//
// Passing NULL for a parameter leaves the existing column value unchanged.
const updateProfileFieldsByUserIDQuery = `
INSERT INTO profiles (
    user_id,
    full_name,
    display_name,
    bio,
    country,
    avatar_url,
    phone,
    preferred_language,
    completed_at,
    updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE($8, 'ar'), $9, NOW())
ON CONFLICT (user_id) DO UPDATE SET
    full_name          = COALESCE($2, profiles.full_name),
    display_name       = COALESCE($3, profiles.display_name),
    bio                = COALESCE($4, profiles.bio),
    country            = COALESCE($5, profiles.country),
    avatar_url         = COALESCE($6, profiles.avatar_url),
    phone              = COALESCE($7, profiles.phone),
    preferred_language = COALESCE($8, profiles.preferred_language),
    completed_at       = COALESCE($9, profiles.completed_at),
    updated_at         = NOW()
`
