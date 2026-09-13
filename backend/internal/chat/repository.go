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

func sameUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

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

// scanReplayOutboxEvent scans the nine-column replay-claim row: the shared
// outbox columns plus was_parked, which carries the pre-claim parked state
// that the post-update RETURNING values can no longer express.
func scanReplayOutboxEvent(row pgx.Row) (OutboxEvent, error) {
	var e OutboxEvent
	err := row.Scan(&e.EventID, &e.MessageID, &e.EventType, &e.RecipientID,
		&e.AvailableAt, &e.DeliveredAt, &e.AttemptCount, &e.ParkedAt, &e.WasParked)
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
		existing, found, err := t.FindMessageByIdempotency(ctx, in.SenderID, in.IdempotencyKey)
		if err != nil {
			return Message{}, false, err
		}
		if !found {
			// The insert already reported this (sender, key) as replayed, so
			// the committed row must exist; report the anomaly instead of
			// returning a zero message as a successful replay.
			return Message{}, false, fmt.Errorf("resolve chat message replay for sender %s: %w", in.SenderID, ErrIdempotencyConflict)
		}
		if !sameUUID(existing.CircleID, in.CircleID) || !sameUUID(existing.DMRecipientID, in.DMRecipientID) || existing.Type != in.Type ||
			existing.Content != in.Content || !sameUUID(existing.UploadID, in.UploadID) ||
			!sameUUID(existing.ReplyToID, in.ReplyToID) {
			return Message{}, false, ErrIdempotencyConflict
		}
		return existing, false, nil
	}
	if err != nil {
		return Message{}, false, fmt.Errorf("insert chat message: %w", err)
	}
	return msg, true, nil
}

// FindMessageByIdempotency loads the committed message for one (sender,
// idempotency key) pair without inserting; found is false when no committed
// message exists for the pair.
func (t *Tx) FindMessageByIdempotency(ctx context.Context, senderID uuid.UUID, idempotencyKey string) (Message, bool, error) {
	msg, err := scanMessage(t.tx.QueryRow(ctx, findMessageByIdempotencyQuery, senderID, idempotencyKey))
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, false, nil
	}
	if err != nil {
		return Message{}, false, fmt.Errorf("load chat message by idempotency: %w", err)
	}
	return msg, true, nil
}

// LockActiveCircleMember verifies the actor's current membership while
// locking the circle and membership rows against archive and removal races.
func (t *Tx) LockActiveCircleMember(ctx context.Context, circleID, userID uuid.UUID) error {
	var archived bool
	err := t.tx.QueryRow(ctx, lockActiveCircleMemberQuery, circleID, userID).Scan(&archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrCircleNotVisible
	}
	if err != nil {
		return fmt.Errorf("lock chat circle membership: %w", err)
	}
	if archived {
		return ErrCircleArchived
	}
	return nil
}

// LockVisibleGroupMessageForRead returns the sender of an active message in
// circleID that is visible in readerID's current membership period and sent by
// a different user. The message row remains locked through the transaction.
func (t *Tx) LockVisibleGroupMessageForRead(ctx context.Context, circleID, readerID, messageID uuid.UUID) (uuid.UUID, error) {
	var senderID uuid.UUID
	err := t.tx.QueryRow(ctx, lockVisibleGroupMessageForReadQuery, circleID, readerID, messageID).Scan(&senderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrMessageNotVisible
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lock visible group message for read: %w", err)
	}
	return senderID, nil
}

// LockEligibleDirectMessageForRead returns the sender of one visible direct
// message while rechecking the reader's current qualifying relationship.
func (t *Tx) LockEligibleDirectMessageForRead(ctx context.Context, readerID, messageID uuid.UUID) (uuid.UUID, error) {
	var senderID uuid.UUID
	err := t.tx.QueryRow(ctx, lockEligibleDirectMessageForReadQuery, readerID, messageID).Scan(&senderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrDMNotEligible
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("lock eligible direct message for read: %w", err)
	}
	return senderID, nil
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

// DeleteOwnDirectMessage soft-deletes a sender's recent direct message.
func (t *Tx) DeleteOwnDirectMessage(ctx context.Context, messageID, senderID uuid.UUID) error {
	var deletedID uuid.UUID
	err := t.tx.QueryRow(ctx, softDeleteOwnDirectMessageQuery, messageID, senderID).Scan(&deletedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrMessageNotVisible
	}
	if err != nil {
		return fmt.Errorf("delete own direct message: %w", err)
	}
	return nil
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

// LoadUploadForUpdate loads one upload row locked by the enclosing
// transaction for attach-time reauthorization; a missing upload is denied
// non-enumeratingly with ErrUploadNotAttachable.
func (t *Tx) LoadUploadForUpdate(ctx context.Context, uploadID uuid.UUID) (Upload, error) {
	upload, err := scanUpload(t.tx.QueryRow(ctx, findUploadForUpdateQuery, uploadID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Upload{}, ErrUploadNotAttachable
	}
	if err != nil {
		return Upload{}, fmt.Errorf("lock chat upload for attach: %w", err)
	}
	return upload, nil
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

// GroupSearchPage returns one retained, non-deleted group-message search page.
func (r *Repository) GroupSearchPage(ctx context.Context, circleID, viewerID uuid.UUID, query string, before *uuid.UUID, limit int) ([]Message, error) {
	anchorSentAt, anchorID, err := r.messageCursor(ctx, before)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, groupSearchPageQuery, circleID, viewerID, query, anchorSentAt, anchorID, limit)
	if err != nil {
		return nil, fmt.Errorf("query chat search page: %w", err)
	}
	defer rows.Close()
	messages := []Message{}
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan chat search message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat search page: %w", err)
	}
	return messages, nil
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
// projection. A caller-pinned ID (the upload identity the object key derives
// from) is preserved; a nil ID is database-generated.
func (r *Repository) InsertUpload(ctx context.Context, upload Upload) (Upload, error) {
	var id, duration any
	if upload.ID != uuid.Nil {
		id = upload.ID
	}
	if upload.DurationSeconds != 0 {
		duration = upload.DurationSeconds
	}
	staged, err := scanUpload(r.pool.QueryRow(ctx, insertUploadQuery, id, upload.UploaderID,
		upload.AuthorizationCircleID, upload.DMPeerID, upload.ObjectKey, upload.MIMEType,
		upload.OriginalFileName, upload.SizeBytes, duration))
	if err != nil {
		return Upload{}, fmt.Errorf("stage chat upload: %w", err)
	}
	return staged, nil
}

// CountRecentUploads counts the uploader's successfully staged uploads
// (staged or attached) created since the given instant, for the rolling-hour
// upload budget (FR-022).
func (r *Repository) CountRecentUploads(ctx context.Context, uploaderID uuid.UUID, since time.Time) (int, error) {
	var count int
	if err := r.pool.QueryRow(ctx, countRecentUploadsQuery, uploaderID, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("count recent chat uploads: %w", err)
	}
	return count, nil
}

// FindQualifyingDMCircle returns one shared active circle that currently
// authorizes the unordered pair's direct conversation (teacher-student or
// supervisor-student roles in either direction), or uuid.Nil with false when
// no qualifying circle exists (FR-016). The pair conversation is not owned by
// the returned witness (FR-023).
func (r *Repository) FindQualifyingDMCircle(ctx context.Context, userA, userB uuid.UUID) (uuid.UUID, bool, error) {
	var circleID uuid.UUID
	err := r.pool.QueryRow(ctx, findQualifyingDMCircleQuery, userA, userB).Scan(&circleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, fmt.Errorf("find qualifying dm circle: %w", err)
	}
	return circleID, true, nil
}

// FindMessageUpload loads one message together with its attached upload for
// media renewal. A missing message or a message without an attached upload
// (for example a text message) is denied non-enumeratingly with
// ErrMessageNotVisible.
func (r *Repository) FindMessageUpload(ctx context.Context, messageID uuid.UUID) (Message, Upload, error) {
	var m Message
	var u Upload
	var content *string
	var duration *int
	err := r.pool.QueryRow(ctx, findMessageUploadQuery, messageID).Scan(
		&m.ID, &m.CircleID, &m.DMRecipientID, &m.SenderID, &m.Type, &content,
		&m.UploadID, &m.ReplyToID, &m.State, &m.SentAt, &m.DeletedAt,
		&u.ID, &u.UploaderID, &u.AuthorizationCircleID, &u.DMPeerID, &u.ObjectKey,
		&u.MIMEType, &u.OriginalFileName, &u.SizeBytes, &duration, &u.State, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, Upload{}, ErrMessageNotVisible
	}
	if err != nil {
		return Message{}, Upload{}, fmt.Errorf("load chat message upload: %w", err)
	}
	if content != nil {
		m.Content = *content
	}
	if duration != nil {
		u.DurationSeconds = *duration
	}
	return m, u, nil
}
