package notifications

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultHistoryLimit = 20
	maxHistoryLimit     = 50
)

// Repository reads recipient-scoped notification history from PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a notification repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// List returns a bounded 30-day history page and unread count from one snapshot.
func (r *Repository) List(ctx context.Context, userID string, before *Cursor, limit int) (Page, error) {
	if limit <= 0 {
		limit = defaultHistoryLimit
	} else if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Page{}, fmt.Errorf("begin notification history read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var beforeTime, beforeID any
	if before != nil {
		beforeTime, beforeID = before.CreatedAt, before.ID
	}
	rows, err := tx.Query(ctx, listNotificationsQuery, userID, beforeTime, beforeID, limit+1)
	if err != nil {
		return Page{}, fmt.Errorf("list notification history: %w", err)
	}
	page := Page{Items: make([]Item, 0, limit)}
	for rows.Next() {
		var item Item
		if err := rows.Scan(
			&item.ID,
			&item.Type,
			&item.SourceRef.Type,
			&item.SourceRef.ID,
			&item.Title,
			&item.Body,
			&item.CreatedAt,
			&item.IsRead,
			&item.ReadAt,
		); err != nil {
			rows.Close()
			return Page{}, fmt.Errorf("scan notification history: %w", err)
		}
		page.Items = append(page.Items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Page{}, fmt.Errorf("iterate notification history: %w", err)
	}
	rows.Close()

	if err := tx.QueryRow(ctx, countUnreadNotificationsQuery, userID).Scan(&page.UnreadCount); err != nil {
		return Page{}, fmt.Errorf("count unread notifications: %w", err)
	}
	// hasMore must be captured before trimming: when exactly limit rows
	// remain, no extra row was fetched and a cursor would page into emptiness.
	hasMore := len(page.Items) > limit
	if hasMore {
		page.Items = page.Items[:limit]
		last := page.Items[len(page.Items)-1]
		page.NextCursor = &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	if err := tx.Commit(ctx); err != nil {
		return Page{}, fmt.Errorf("commit notification history read: %w", err)
	}
	return page, nil
}
