package rbac

import "time"

// Circle roles supported by circle_members.
const (
	RoleStudent    = "student"
	RoleSupervisor = "supervisor"
	RoleTeacher    = "teacher"
)

// CreateCircleRequest is the payload for POST /circles.
type CreateCircleRequest struct {
	Name                   string   `json:"name"`
	TeacherUserIDs         []string `json:"teacher_user_ids"`
	BackupSupervisorUserID *string  `json:"backup_supervisor_user_id"`
}

// AssignCircleRoleRequest is the payload for PUT /circles/{circleId}/members/{userId}/role.
type AssignCircleRoleRequest struct {
	Role string `json:"role"`
}

// Circle is the persistence record for one circle.
type Circle struct {
	ID         string
	Name       string
	TeacherID  string
	InviteCode string
	CreatedAt  time.Time
}

// Member is one circle membership projection.
type Member struct {
	UserID string
	Role   string
}

// CircleResponse is the POST /circles 201 response body.
type CircleResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	InviteCode string    `json:"invite_code"`
	CreatedAt  time.Time `json:"created_at"`
}

// RoleAssignment is the PUT role 200 response body.
type RoleAssignment struct {
	CircleID string `json:"circle_id"`
	UserID   string `json:"user_id"`
	Role     string `json:"role"`
}
