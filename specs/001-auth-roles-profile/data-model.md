# Data Model: Authentication, Roles, and User Profile

## 1) User
- **Purpose**: Authenticated platform account.
- **Core fields**:
  - `id` (UUID, PK)
  - `firebase_uid` (string, unique)
  - `email` (string, unique, normalized lowercase)
  - `account_type` (enum: `student | teacher`)
  - `status` (enum: `active | disabled`)
  - `created_at`, `updated_at` (UTC timestamps)
- **Validation**:
  - Email unique and valid format
  - `account_type` can only be `student` or `teacher` during self-registration

## 2) Profile (user-owned)
- **Purpose**: User identity/profile details used in app experiences.
- **Core fields**:
  - `user_id` (UUID, PK/FK -> User.id)
  - `full_name` (string, required for first completion)
  - `display_name` (string, optional)
  - `bio` (string, optional)
  - `country` (ISO country code, required for first completion)
  - `avatar_url` (string, optional)
  - `profile_completed_at` (timestamp, nullable until completed)
  - `updated_at` (UTC timestamp)
- **Validation**:
  - First completion requires both `full_name` and `country`
  - Country must be valid ISO code
  - Field length and trimming rules enforced server-side

## 3) UserSession
- **Purpose**: Durable backend session activity state for inactivity timeout and logout invalidation.
- **Core fields**:
  - `id` (UUID, PK)
  - `user_id` (UUID, FK -> User.id)
  - `device_id` (string, nullable)
  - `last_activity_at` (UTC timestamp)
  - `revoked_at` (UTC timestamp, nullable)
  - `created_by_ip` (string, nullable)
  - `created_by_user_agent` (string, nullable)
  - `created_at` (UTC timestamp)
- **Validation**:
  - Session is invalid after 30 days of inactivity
  - Revoked session cannot access protected endpoints

## 4) CircleMember
- **Purpose**: Per-circle authorization source of truth.
- **Core fields**:
  - `circle_id` (UUID, PK/FK)
  - `user_id` (UUID, PK/FK)
  - `role` (enum: `student | teacher | supervisor`)
  - `status` (enum: `active | removed`)
  - `joined_at` (UTC timestamp)
  - `updated_at` (UTC timestamp)
- **Validation**:
  - Role checks for protected actions query this table
  - Non-members and insufficient roles are rejected with `403`

## Relationships
- User **1:1** Profile
- User **1:N** UserSession
- User **N:M** Circle via CircleMember

## State Transitions

### UserSession
1. `active` (not revoked and activity within 30 days)
2. `revoked` (explicit logout or admin disablement)
3. `expired` (30-day inactivity timeout)

### CircleMember Role Authorization
1. `member` (role assigned in circle_members)
2. `authorized` (role meets endpoint requirement)
3. `forbidden` (missing membership or insufficient role)

### Profile Completion
1. `incomplete` (missing required fields for first completion)
2. `complete` (`full_name` and `country` present)
3. `updated` (subsequent edits preserve completion unless required fields removed, which is rejected)
