package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrUploadNotStaged indicates an attach targeted an upload that is no longer
// staged (already attached or revoked), making the transition single-use.
var ErrUploadNotStaged = errors.New("chat: upload is not staged")

// Repository persists F-004 chat truth in PostgreSQL.
type Repository struct{ pool *pgxpool.Pool }

// Tx is one chat mutation transaction.
type Tx struct{ tx pgx.Tx }

// NewRepository constructs a chat repository from a PostgreSQL pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// WithTx runs fn atomically: fn's mutations commit together or not at all.
// Domain errors returned by fn are propagated unwrapped.
func (r *Repository) WithTx(ctx context.Context, fn func(*Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin chat transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(&Tx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit chat transaction: %w", err)
	}
	return nil
}

func scanMessage(row pgx.Row) (Message, error) {
	var m Message
	var content *string
	err := row.Scan(&m.ID, &m.CircleID, &m.DMRecipientID, &m.SenderID, &m.Type, &content,
		&m.UploadID, &m.ReplyToID, &m.State, &m.SentAt, &m.DeletedAt)
	if content != nil {
		m.Content = *content
	}
	return m, err
}

func scanUpload(row pgx.Row) (Upload, error) {
	var u Upload
	var duration *int
	err := row.Scan(&u.ID, &u.UploaderID, &u.AuthorizationCircleID, &u.DMPeerID, &u.ObjectKey,
		&u.MIMEType, &u.OriginalFileName, &u.SizeBytes, &duration, &u.State, &u.CreatedAt, &u.UpdatedAt)
	if duration != nil {
		u.DurationSeconds = *duration
	}
	return u, err
}

func scanOutboxEvent(row pgx.Row) (OutboxEvent, error) {
	var e OutboxEvent
	err := row.Scan(&e.EventID, &e.MessageID, &e.EventType, &e.RecipientID,
		&e.AvailableAt, &e.DeliveredAt, &e.AttemptCount, &e.ParkedAt)
	if err == nil {
		e.State = outboxState(e.DeliveredAt, e.ParkedAt)
	}
	return e, err
}

func outboxState(deliveredAt, parkedAt *time.Time) OutboxState {
	switch {
	case deliveredAt != nil:
		return OutboxStateDelivered
	case parkedAt != nil:
		return OutboxStateParked
	default:
		return OutboxStatePending
	}
}

// InsertMessage persists one message idempotently inside the enclosing
// transaction. A replayed (sender_id, idempotency_key) returns the committed
// original with inserted=false; a fresh insert returns inserted=true. The
// caller emits outbox events only for fresh inserts.
func (t *Tx) InsertMessage(ctx context.Context, in MessageInput) (Message, bool, error) {
	var content any
	if in.Content != "" {
		content = in.Content
	}
	msg, err := scanMessage(t.tx.QueryRow(ctx, insertMessageQuery, in.CircleID, in.DMRecipientID,
		in.SenderID, in.IdempotencyKey, in.Type, content, in.UploadID, in.ReplyToID))
	if errors.Is(err, pgx.ErrNoRows) {
		existing, err := scanMessage(t.tx.QueryRow(ctx, findMessageByIdempotencyQuery, in.SenderID, in.IdempotencyKey))
		if err != nil {
			return Message{}, false, fmt.Errorf("load existing chat message: %w", err)
		}
		return existing, false, nil
	}
	if err != nil {
		return Message{}, false, fmt.Errorf("insert chat message: %w", err)
	}
	return msg, true, nil
}

// InsertOutboxEvent persists one identifier-only chat event in the same
// transaction as the mutation it describes.
func (t *Tx) InsertOutboxEvent(ctx context.Context, messageID uuid.UUID, eventType string, recipientID *uuid.UUID) error {
	if _, err := t.tx.Exec(ctx, insertChatOutboxEventQuery, messageID, eventType, recipientID); err != nil {
		return fmt.Errorf("insert chat outbox event: %w", err)
	}
	return nil
}

// InsertMessageRead records one read fact idempotently: a repeated
// (message_id, user_id) reports inserted=false and leaves the original fact
// untouched.
func (t *Tx) InsertMessageRead(ctx context.Context, messageID, userID uuid.UUID) (bool, error) {
	tag, err := t.tx.Exec(ctx, insertMessageReadQuery, messageID, userID)
	if err != nil {
		return false, fmt.Errorf("insert chat read receipt: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// AttachUpload applies the single-use staged→attached transition. An upload
// that is no longer staged returns ErrUploadNotStaged.
func (t *Tx) AttachUpload(ctx context.Context, uploadID uuid.UUID) (Upload, error) {
	attached, err := scanUpload(t.tx.QueryRow(ctx, attachUploadQuery, uploadID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Upload{}, ErrUploadNotStaged
	}
	if err != nil {
		return Upload{}, fmt.Errorf("attach chat upload: %w", err)
	}
	return attached, nil
}

// InsertModerationAudit appends one content-free moderation fact.
func (t *Tx) InsertModerationAudit(ctx context.Context, audit ModerationAudit) (ModerationAudit, error) {
	recorded := audit
	if err := t.tx.QueryRow(ctx, insertModerationAuditQuery, audit.MessageID, audit.CircleID,
		audit.ActorID, audit.Action).Scan(&recorded.ID, &recorded.OccurredAt); err != nil {
		return ModerationAudit{}, fmt.Errorf("insert chat moderation audit: %w", err)
	}
	return recorded, nil
}

// GroupHistoryPage returns one circle history page for a current member in
// (sent_at, id) DESC order. Non-members get an empty page; messages sent
// before the member's joined_at and soft-deleted messages are excluded. The
// before anchor paginates at the referenced message; nil starts at newest.
func (r *Repository) GroupHistoryPage(ctx context.Context, circleID, viewerID uuid.UUID, before *uuid.UUID, limit int) ([]Message, error) {
	anchorSentAt, anchorID, err := r.messageCursor(ctx, before)
	if err != nil {
		return nil, err
	}
	return r.queryMessagePage(ctx, groupHistoryPageQuery, circleID, viewerID, anchorSentAt, anchorID, limit)
}

// DMHistoryPage returns one unordered-pair DM history page in (sent_at, id)
// DESC order, excluding deleted messages. before anchors pagination at the
// referenced message; nil starts at newest.
func (r *Repository) DMHistoryPage(ctx context.Context, userA, userB uuid.UUID, before *uuid.UUID, limit int) ([]Message, error) {
	anchorSentAt, anchorID, err := r.messageCursor(ctx, before)
	if err != nil {
		return nil, err
	}
	return r.queryMessagePage(ctx, dmHistoryPageQuery, userA, userB, anchorSentAt, anchorID, limit)
}

// messageCursor resolves the anchor message's (sent_at, id) pair. Soft-deleted
// anchors remain valid; unknown ids return ErrInvalidCursor.
func (r *Repository) messageCursor(ctx context.Context, before *uuid.UUID) (*time.Time, *uuid.UUID, error) {
	if before == nil {
		return nil, nil, nil
	}
	var sentAt time.Time
	var id uuid.UUID
	if err := r.pool.QueryRow(ctx, findMessageCursorQuery, before).Scan(&sentAt, &id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrInvalidCursor
		}
		return nil, nil, fmt.Errorf("load chat cursor: %w", err)
	}
	return &sentAt, &id, nil
}

func (r *Repository) queryMessagePage(ctx context.Context, query string, contextA, contextB uuid.UUID, anchorSentAt *time.Time, anchorID *uuid.UUID, limit int) ([]Message, error) {
	if limit < 1 {
		return []Message{}, nil
	}
	rows, err := r.pool.Query(ctx, query, contextA, contextB, anchorSentAt, anchorID, limit)
	if err != nil {
		return nil, fmt.Errorf("query chat history page: %w", err)
	}
	defer rows.Close()
	msgs := []Message{}
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan chat message: %w", err)
		}
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat history page: %w", err)
	}
	return msgs, nil
}

// ClaimOutboxEvents leases due, undelivered, unparked events for dispatch.
// Each claim increments attempt_count and pushes available_at into a 30-second
// lease window so concurrent or immediate re-claims stay disjoint.
func (r *Repository) ClaimOutboxEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	if limit < 1 {
		return []OutboxEvent{}, nil
	}
	var events []OutboxEvent
	err := r.WithTx(ctx, func(tx *Tx) error {
		rows, err := tx.tx.Query(ctx, claimOutboxEventsQuery, limit)
		if err != nil {
			return fmt.Errorf("claim chat outbox events: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			event, err := scanOutboxEvent(rows)
			if err != nil {
				return fmt.Errorf("scan claimed chat outbox event: %w", err)
			}
			events = append(events, event)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate claimed chat outbox events: %w", err)
		}
		return nil
	})
	return events, err
}

// InsertUpload stages one private attachment row and returns the persisted
// projection with its server-generated identity.
func (r *Repository) InsertUpload(ctx context.Context, upload Upload) (Upload, error) {
	var duration any
	if upload.DurationSeconds != 0 {
		duration = upload.DurationSeconds
	}
	staged, err := scanUpload(r.pool.QueryRow(ctx, insertUploadQuery, upload.UploaderID,
		upload.AuthorizationCircleID, upload.DMPeerID, upload.ObjectKey, upload.MIMEType,
		upload.OriginalFileName, upload.SizeBytes, duration))
	if err != nil {
		return Upload{}, fmt.Errorf("stage chat upload: %w", err)
	}
	return staged, nil
}
