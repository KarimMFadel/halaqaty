package rbac

import "time"

// CircleSettings contains validated persisted circle configuration.
type CircleSettings struct {
	Description       *string
	Rules             *string
	MaxCapacity       int
	IsPrivate         bool
	GenderRestriction string
	Language          string
	GradingPolicy     string
}

// Circle roles supported by circle_members.
const (
	RoleStudent    = "student"
	RoleSupervisor = "supervisor"
	RoleTeacher    = "teacher"
)

// CreateCircleRequest is the payload for POST /circles.
type CreateCircleRequest struct {
	Name                   string   `json:"name"`
	Description            *string  `json:"description"`
	Rules                  *string  `json:"rules"`
	MaxCapacity            int      `json:"max_capacity"`
	IsPrivate              bool     `json:"is_private"`
	GenderRestriction      string   `json:"gender_restriction"`
	Language               string   `json:"language"`
	GradingPolicy          string   `json:"grading_policy"`
	TeacherUserIDs         []string `json:"teacher_user_ids"`
	BackupSupervisorUserID *string  `json:"backup_supervisor_user_id"`
}

// JoinCircleRequest is the payload for POST /circles/join.
type JoinCircleRequest struct {
	InviteCode string `json:"invite_code"`
}

// AssignCircleRoleRequest is the payload for PUT /circles/{circleId}/members/{userId}/role.
type AssignCircleRoleRequest struct {
	Role string `json:"role"`
}

// Circle is the persistence record for one circle.
type Circle struct {
	ID                string
	Name              string
	TeacherID         string
	InviteCode        string
	Description       *string
	Rules             *string
	MaxCapacity       int
	IsPrivate         bool
	GenderRestriction string
	Language          string
	GradingPolicy     string
	IsArchived        bool
	CreatedAt         time.Time
}

// Member is one circle membership projection.
type Member struct {
	UserID string
	Role   string
}

// CircleResponse is the POST /circles 201 response body.
type CircleResponse struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	InviteCode        string    `json:"invite_code"`
	InviteLink        string    `json:"invite_link"`
	Description       *string   `json:"description,omitempty"`
	Rules             *string   `json:"rules,omitempty"`
	MaxCapacity       int       `json:"max_capacity"`
	IsPrivate         bool      `json:"is_private"`
	GenderRestriction string    `json:"gender_restriction"`
	Language          string    `json:"language"`
	GradingPolicy     string    `json:"grading_policy"`
	IsArchived        bool      `json:"is_archived"`
	CreatedAt         time.Time `json:"created_at"`
}

// PublicCircleSummary is the redacted projection used by public discovery.
// It intentionally has no invite code or membership fields.
type PublicCircleSummary struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	Description       *string   `json:"description,omitempty"`
	MaxCapacity       int       `json:"max_capacity"`
	GenderRestriction string    `json:"gender_restriction"`
	Language          string    `json:"language"`
	CreatedAt         time.Time `json:"created_at"`
}

// RoleAssignment is the PUT role 200 response body.
type RoleAssignment struct {
	CircleID string `json:"circle_id"`
	UserID   string `json:"user_id"`
	Role     string `json:"role"`
}

// UserSearchResult is the minimal registered-user projection used during circle creation.
type UserSearchResult struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
}

// UserSearchResponse is the authenticated user-search response body.
type UserSearchResponse struct {
	Data []UserSearchResult `json:"data"`
}
