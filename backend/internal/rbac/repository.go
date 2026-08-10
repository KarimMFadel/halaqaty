package rbac

import (
	"context"
	"errors"
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

// FindCircleByInviteCode returns the circle associated with an invite code.
func (r *Repository) FindCircleByInviteCode(ctx context.Context, inviteCode string) (Circle, error) {
	var circle Circle
	err := r.q.QueryRow(ctx, findCircleByInviteCodeQuery, inviteCode).Scan(
		&circle.ID,
		&circle.Name,
		&circle.InviteCode,
		&circle.Description,
		&circle.Rules,
		&circle.MaxCapacity,
		&circle.IsPrivate,
		&circle.GenderRestriction,
		&circle.Language,
		&circle.GradingPolicy,
		&circle.IsArchived,
		&circle.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Circle{}, ErrCircleNotFound
	}
	if err != nil {
		return Circle{}, fmt.Errorf("find circle by invite code: %w", err)
	}
	return circle, nil
}

// FindCircleByID returns a circle by its stable identifier.
func (r *Repository) FindCircleByID(ctx context.Context, circleID string) (Circle, error) {
	return r.scanCircle(ctx, findCircleByIDQuery, circleID)
}

// FindCircleByIDForUpdate returns a circle while holding its row lock in a transaction.
func (r *Repository) FindCircleByIDForUpdate(ctx context.Context, circleID string) (Circle, error) {
	return r.scanCircle(ctx, findCircleByIDForUpdateQuery, circleID)
}

func (r *Repository) scanCircle(ctx context.Context, query string, args ...any) (Circle, error) {
	var circle Circle
	err := r.q.QueryRow(ctx, query, args...).Scan(&circle.ID, &circle.Name, &circle.TeacherID, &circle.InviteCode, &circle.Description, &circle.Rules, &circle.MaxCapacity, &circle.IsPrivate, &circle.GenderRestriction, &circle.Language, &circle.GradingPolicy, &circle.IsArchived, &circle.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Circle{}, ErrCircleNotFound
	}
	if err != nil {
		return Circle{}, fmt.Errorf("find circle: %w", err)
	}
	return circle, nil
}

// ListPublicCircles returns only redacted active public circle projections.
func (r *Repository) ListPublicCircles(ctx context.Context, query, cursor string, limit int) ([]PublicCircleSummary, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := r.q.Query(ctx, listPublicCirclesQuery, query, cursor, limit)
	if err != nil {
		return nil, fmt.Errorf("list public circles: %w", err)
	}
	defer rows.Close()
	var result []PublicCircleSummary
	for rows.Next() {
		var circle PublicCircleSummary
		if err := rows.Scan(&circle.ID, &circle.Name, &circle.Description, &circle.MaxCapacity, &circle.GenderRestriction, &circle.Language, &circle.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan public circle: %w", err)
		}
		result = append(result, circle)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate public circles: %w", err)
	}
	return result, nil
}

// UpdateCircle persists validated circle settings.
func (r *Repository) UpdateCircle(ctx context.Context, circleID, name string, settings CircleSettings) (Circle, error) {
	return r.scanCircle(ctx, updateCircleQuery, circleID, name, settings.Description, settings.Rules, settings.MaxCapacity, settings.IsPrivate, settings.GenderRestriction, settings.Language, settings.GradingPolicy)
}

// RefreshInviteCode atomically replaces the current invite code.
func (r *Repository) RefreshInviteCode(ctx context.Context, circleID, inviteCode string) error {
	if _, err := r.q.Exec(ctx, refreshInviteCodeQuery, circleID, inviteCode); err != nil {
		return fmt.Errorf("refresh invite code: %w", err)
	}
	return nil
}

// RemoveMember removes only the active membership row and preserves circle history.
func (r *Repository) RemoveMember(ctx context.Context, circleID, userID string) error {
	if _, err := r.q.Exec(ctx, removeCircleMemberQuery, circleID, userID); err != nil {
		return fmt.Errorf("remove circle member: %w", err)
	}
	return nil
}

// ArchiveCircle marks a circle retired without deleting any data.
func (r *Repository) ArchiveCircle(ctx context.Context, circleID string) error {
	if _, err := r.q.Exec(ctx, archiveCircleQuery, circleID); err != nil {
		return fmt.Errorf("archive circle: %w", err)
	}
	return nil
}

// ListMembers returns the current membership projection.
func (r *Repository) ListMembers(ctx context.Context, circleID string) ([]Member, error) {
	rows, err := r.q.Query(ctx, listCircleMembersQuery, circleID)
	if err != nil {
		return nil, fmt.Errorf("list circle members: %w", err)
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

// SearchUsers returns registered users matching a display-name query.
func (r *Repository) SearchUsers(ctx context.Context, query string, limit int) ([]UserSearchResult, error) {
	rows, err := r.q.Query(ctx, searchUsersQuery, query, limit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	defer rows.Close()

	var users []UserSearchResult
	for rows.Next() {
		var user UserSearchResult
		if err := rows.Scan(&user.ID, &user.DisplayName); err != nil {
			return nil, fmt.Errorf("scan user search result: %w", err)
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user search results: %w", err)
	}
	return users, nil
}

// LockUser serializes membership-limit checks for one user.
func (r *Repository) LockUser(ctx context.Context, userID string) error {
	var id string
	if err := r.q.QueryRow(ctx, lockUserQuery, userID).Scan(&id); err != nil {
		return fmt.Errorf("lock user: %w", err)
	}
	return nil
}

// InsertCircle creates one circle owned by ownerID.
func (r *Repository) InsertCircle(ctx context.Context, name, ownerID, inviteCode string, settings CircleSettings) (Circle, error) {
	circle := Circle{Name: name, TeacherID: ownerID, InviteCode: inviteCode, Description: settings.Description, Rules: settings.Rules, MaxCapacity: settings.MaxCapacity, IsPrivate: settings.IsPrivate, GenderRestriction: settings.GenderRestriction, Language: settings.Language, GradingPolicy: settings.GradingPolicy}
	err := r.q.QueryRow(ctx, insertCircleQuery, name, ownerID, inviteCode, settings.Description, settings.Rules, settings.MaxCapacity, settings.IsPrivate, settings.GenderRestriction, settings.Language, settings.GradingPolicy).Scan(&circle.ID, &circle.CreatedAt)
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

// CountActiveMemberships returns the user's active circle count.
func (r *Repository) CountActiveMemberships(ctx context.Context, userID string) (int, error) {
	var count int
	if err := r.q.QueryRow(ctx, countActiveMembershipsQuery, userID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active memberships: %w", err)
	}
	return count, nil
}

// UpdateMemberRole applies a validated role change to one membership.
func (r *Repository) UpdateMemberRole(ctx context.Context, circleID, userID, role string) error {
	if _, err := r.q.Exec(ctx, updateCircleMemberRoleQuery, circleID, userID, role); err != nil {
		return fmt.Errorf("update circle member role: %w", err)
	}
	return nil
}
