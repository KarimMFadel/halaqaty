package rbac

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repository is the PostgreSQL persistence boundary for circle RBAC data.
type Repository struct {
	pool *pgxpool.Pool
	q    querier
}

// NewRepository constructs a circle RBAC repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: pool}
}

// WithinTransaction runs fn against a transaction-scoped store and commits on
// success. It must only be called on the root repository, never inside fn.
func (r *Repository) WithinTransaction(ctx context.Context, fn func(Store) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin circle transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(&Repository{q: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit circle transaction: %w", err)
	}
	return nil
}

// UsersExist resolves which candidate IDs belong to registered users.
func (r *Repository) UsersExist(ctx context.Context, userIDs []string) (map[string]bool, error) {
	existing := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		existing[id] = false
	}
	rows, err := r.q.Query(ctx, usersExistQuery, userIDs)
	if err != nil {
		return nil, fmt.Errorf("query users existence: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan user id: %w", err)
		}
		existing[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user ids: %w", err)
	}
	return existing, nil
}

// InsertCircle creates one circle owned by ownerID.
func (r *Repository) InsertCircle(ctx context.Context, name, ownerID, inviteCode string) (Circle, error) {
	circle := Circle{Name: name, TeacherID: ownerID, InviteCode: inviteCode}
	err := r.q.QueryRow(ctx, insertCircleQuery, name, ownerID, inviteCode).Scan(&circle.ID, &circle.CreatedAt)
	if err != nil {
		return Circle{}, fmt.Errorf("insert circle: %w", err)
	}
	return circle, nil
}

// InsertMember adds one membership; existing memberships keep their role.
func (r *Repository) InsertMember(ctx context.Context, circleID, userID, role string) error {
	if _, err := r.q.Exec(ctx, insertCircleMemberQuery, circleID, userID, role); err != nil {
		return fmt.Errorf("insert circle member: %w", err)
	}
	return nil
}

// CircleExists reports whether the circle row exists.
func (r *Repository) CircleExists(ctx context.Context, circleID string) (bool, error) {
	var exists bool
	if err := r.q.QueryRow(ctx, circleExistsQuery, circleID).Scan(&exists); err != nil {
		return false, fmt.Errorf("check circle existence: %w", err)
	}
	return exists, nil
}

// LockMembers locks and returns the full membership set of one circle.
func (r *Repository) LockMembers(ctx context.Context, circleID string) ([]Member, error) {
	rows, err := r.q.Query(ctx, lockCircleMembersQuery, circleID)
	if err != nil {
		return nil, fmt.Errorf("lock circle members: %w", err)
	}
	defer rows.Close()

	var members []Member
	for rows.Next() {
		var member Member
		if err := rows.Scan(&member.UserID, &member.Role); err != nil {
			return nil, fmt.Errorf("scan circle member: %w", err)
		}
		members = append(members, member)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate circle members: %w", err)
	}
	return members, nil
}

// UpdateMemberRole applies a validated role change to one membership.
func (r *Repository) UpdateMemberRole(ctx context.Context, circleID, userID, role string) error {
	if _, err := r.q.Exec(ctx, updateCircleMemberRoleQuery, circleID, userID, role); err != nil {
		return fmt.Errorf("update circle member role: %w", err)
	}
	return nil
}
