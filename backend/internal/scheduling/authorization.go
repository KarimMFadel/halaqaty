package scheduling

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrCircleNotFoundOrDenied hides circle existence from non-members.
	ErrCircleNotFoundOrDenied = errors.New("circle not found or access denied")
	// ErrInsufficientCircleRole means a member lacks the required role.
	ErrInsufficientCircleRole = errors.New("insufficient circle role")
	// ErrCircleArchived means an archived circle cannot be changed.
	ErrCircleArchived = errors.New("circle is archived")
)

// Querier is a database transaction used for authorization and command replay.
// Mutation callers must pass the same transaction for checks and writes.
type Querier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// CircleAccess is a current membership projection.
type CircleAccess struct {
	UserID   string
	Role     string
	Archived bool
}

// CanManage reports whether this member can manage plans in an active circle.
func (a CircleAccess) CanManage() bool {
	return !a.Archived && (a.Role == "teacher" || a.Role == "supervisor")
}

// CanReadAttendanceOf applies student self-only visibility.
func (a CircleAccess) CanReadAttendanceOf(userID string) bool {
	return a.Role == "teacher" || a.Role == "supervisor" || a.UserID == userID
}

// RequireCircleRead checks current membership; the caller supplies its transaction.
func RequireCircleRead(ctx context.Context, tx Querier, circleID, userID string) (CircleAccess, error) {
	a := CircleAccess{UserID: userID}
	if err := tx.QueryRow(ctx, getCircleAccessQuery, circleID, userID).Scan(&a.Role, &a.Archived); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CircleAccess{}, ErrCircleNotFoundOrDenied
		}
		return CircleAccess{}, fmt.Errorf("read circle access: %w", err)
	}
	return a, nil
}

// RequireCircleManage checks current role and active state inside the command transaction.
func RequireCircleManage(ctx context.Context, tx Querier, circleID, userID string) (CircleAccess, error) {
	a, err := RequireCircleRead(ctx, tx, circleID, userID)
	if err != nil {
		return CircleAccess{}, err
	}
	if a.Archived {
		return CircleAccess{}, ErrCircleArchived
	}
	if !a.CanManage() {
		return CircleAccess{}, ErrInsufficientCircleRole
	}
	return a, nil
}
