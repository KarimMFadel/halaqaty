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
		&m.UploadID, &m.ReplyToID, &m.PinnedBy, &m.PinnedAt, &m.State, &m.SentAt, &m.DeletedAt)
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
		if !matchesMessageInput(existing, in) {
			return Message{}, false, ErrIdempotencyConflict
		}
		return existing, false, nil
	}
	if err != nil {
		return Message{}, false, fmt.Errorf("insert chat message: %w", err)
	}
	return msg, true, nil
}

func matchesMessageInput(existing Message, in MessageInput) bool {
	return sameUUID(existing.CircleID, in.CircleID) && sameUUID(existing.DMRecipientID, in.DMRecipientID) && existing.Type == in.Type &&
		existing.Content == in.Content && sameUUID(existing.UploadID, in.UploadID) && sameUUID(existing.ReplyToID, in.ReplyToID)
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

// LockVisibleGroupReplyTarget locks an active group message visible in the
// actor's current membership period, without disclosing why it is unavailable.
func (t *Tx) LockVisibleGroupReplyTarget(ctx context.Context, circleID, actorID, messageID uuid.UUID) (Message, error) {
	message, err := scanMessage(t.tx.QueryRow(ctx, lockVisibleGroupReplyTargetQuery, circleID, actorID, messageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrMessageNotVisible
	}
	if err != nil {
		return Message{}, fmt.Errorf("lock visible group reply target: %w", err)
	}
	return message, nil
}

// LockVisibleDirectReplyTarget locks an active message belonging to exactly
// the current unordered direct pair.
func (t *Tx) LockVisibleDirectReplyTarget(ctx context.Context, userA, userB, messageID uuid.UUID) (Message, error) {
	message, err := scanMessage(t.tx.QueryRow(ctx, lockVisibleDirectReplyTargetQuery, userA, userB, messageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, ErrMessageNotVisible
	}
	if err != nil {
		return Message{}, fmt.Errorf("lock visible direct reply target: %w", err)
	}
	return message, nil
}

// LockCirclePinActor serializes pin state with other circle mutations and
// returns the actor's current role.
func (t *Tx) LockCirclePinActor(ctx context.Context, circleID, actorID uuid.UUID) (string, error) {
	var archived bool
	var role string
	err := t.tx.QueryRow(ctx, lockCirclePinActorQuery, circleID, actorID).Scan(&archived, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrCircleNotVisible
	}
	if err != nil {
		return "", fmt.Errorf("lock circle pin actor: %w", err)
	}
	if archived {
		return "", ErrCircleArchived
	}
	return role, nil
}

// PinVisibleGroupMessage pins one visible, active group message.
func (t *Tx) PinVisibleGroupMessage(ctx context.Context, circleID, actorID, messageID uuid.UUID) (bool, error) {
	var id uuid.UUID
	err := t.tx.QueryRow(ctx, pinVisibleGroupMessageQuery, circleID, actorID, messageID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("pin visible group message: %w", err)
	}
	return true, nil
}

// UnpinVisibleGroupMessage removes one visible active group message pin.
func (t *Tx) UnpinVisibleGroupMessage(ctx context.Context, circleID, actorID, messageID uuid.UUID) (bool, error) {
	var id uuid.UUID
	err := t.tx.QueryRow(ctx, unpinVisibleGroupMessageQuery, circleID, actorID, messageID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("unpin visible group message: %w", err)
	}
	return true, nil
}

// CountPinnedMessages counts active pins while the caller holds the circle row.
func (t *Tx) CountPinnedMessages(ctx context.Context, circleID uuid.UUID) (int, error) {
	var count int
	if err := t.tx.QueryRow(ctx, countPinnedMessagesQuery, circleID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count pinned group messages: %w", err)
	}
	return count, nil
}

// LockVisibleGroupMessagePinState locks one eligible pin target and reports
// whether it is already pinned, preserving retry idempotence at the limit.
func (t *Tx) LockVisibleGroupMessagePinState(ctx context.Context, circleID, actorID, messageID uuid.UUID) (bool, error) {
	var pinned bool
	err := t.tx.QueryRow(ctx, lockVisibleGroupMessagePinStateQuery, circleID, actorID, messageID).Scan(&pinned)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrMessageNotVisible
	}
	if err != nil {
		return false, fmt.Errorf("lock group message pin state: %w", err)
	}
	return pinned, nil
}

// hydrateReplyPreviews loads current reply-target projections; deleted target
// content is never retained in a response or realtime payload.
func (r *Repository) hydrateReplyPreviews(ctx context.Context, viewerID uuid.UUID, messages []Message) error {
	for i := range messages {
		if messages[i].ReplyToID == nil {
			continue
		}
		preview := ReplyPreview{ID: *messages[i].ReplyToID}
		var content *string
		query := findReplyPreviewQuery
		args := []any{preview.ID}
		if messages[i].CircleID != nil {
			query = findVisibleGroupReplyPreviewQuery
			args = append(args, viewerID, *messages[i].CircleID)
		} else if messages[i].DMRecipientID != nil {
			peerID := messages[i].SenderID
			if peerID == viewerID {
				peerID = *messages[i].DMRecipientID
			}
			query = findVisibleDirectReplyPreviewQuery
			args = append(args, viewerID, peerID)
		}
		err := r.pool.QueryRow(ctx, query, args...).Scan(&preview.ID, &content, &preview.Deleted)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("load reply preview: %w", err)
		}
		if !preview.Deleted && content != nil {
			preview.Preview = safeReplyPreview(Message{ID: preview.ID, Content: *content}).Preview
		}
		messages[i].ReplyPreview = &preview
	}
	return nil
}

// hydrateDeletedReplyPreviews avoids leaking an active target's content in a
// shared realtime event whose recipients can have different join times.
func (r *Repository) hydrateDeletedReplyPreviews(ctx context.Context, messages []Message) error {
	for i := range messages {
		if messages[i].ReplyToID == nil {
			continue
		}
		preview := ReplyPreview{ID: *messages[i].ReplyToID, Deleted: true}
		err := r.pool.QueryRow(ctx, findDeletedReplyPreviewQuery, preview.ID).Scan(&preview.ID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("load deleted reply preview: %w", err)
		}
		messages[i].ReplyPreview = &preview
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
func (t *Tx) LockEligibleDirectMessageForRead(ctx context.Context, readerID, peerID, messageID uuid.UUID) (uuid.UUID, error) {
	var senderID uuid.UUID
	err := t.tx.QueryRow(ctx, lockEligibleDirectMessageForReadQuery, readerID, peerID, messageID).Scan(&senderID)
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

// FindMessageRead loads one authoritative read fact for realtime projection.
func (r *Repository) FindMessageRead(ctx context.Context, messageID, userID uuid.UUID) (MessageRead, error) {
	var read MessageRead
	if err := r.pool.QueryRow(ctx, findMessageReadQuery, messageID, userID).Scan(&read.MessageID, &read.UserID, &read.ReadAt); err != nil {
		return MessageRead{}, fmt.Errorf("load chat read receipt: %w", err)
	}
	return read, nil
}

// LoadSenderReadReceipts returns only currently authorized readers for a
// sender-owned message. Callers pass only messages already visible to the
// current viewer, so the query never creates a new enumeration surface.
func (r *Repository) LoadSenderReadReceipts(ctx context.Context, message Message) ([]MessageRead, error) {
	if message.CircleID == nil && message.DMRecipientID == nil {
		return nil, ErrInvalidContext
	}
	query := groupMessageReadReceiptsQuery
	if message.DMRecipientID != nil {
		query = directMessageReadReceiptsQuery
	}
	rows, err := r.pool.Query(ctx, query, message.ID, message.SenderID)
	if err != nil {
		return nil, fmt.Errorf("load chat read receipts: %w", err)
	}
	defer rows.Close()
	receipts := []MessageRead{}
	for rows.Next() {
		var receipt MessageRead
		if err := rows.Scan(&receipt.MessageID, &receipt.UserID, &receipt.ReadAt); err != nil {
			return nil, fmt.Errorf("scan chat read receipt: %w", err)
		}
		receipts = append(receipts, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate chat read receipts: %w", err)
	}
	return receipts, nil
}

func (r *Repository) hydrateSenderReadReceipts(ctx context.Context, viewerID uuid.UUID, messages []Message) error {
	for i := range messages {
		if messages[i].SenderID != viewerID {
			continue
		}
		receipts, err := r.LoadSenderReadReceipts(ctx, messages[i])
		if err != nil {
			return err
		}
		messages[i].ReadReceipts = receipts
	}
	return nil
}

// LockQualifyingDMCircle locks one qualifying circle and its pair memberships
// for a direct-message mutation.
func (t *Tx) LockQualifyingDMCircle(ctx context.Context, userA, userB uuid.UUID) error {
	var circleID uuid.UUID
	err := t.tx.QueryRow(ctx, lockQualifyingDMCircleQuery, userA, userB).Scan(&circleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrDMNotEligible
	}
	if err != nil {
		return fmt.Errorf("lock qualifying direct circle: %w", err)
	}
	return nil
}

// LoadDirectMessageUploadForDelete locks one direct attachment for the
// marker-before-commit deletion sequence.
func (t *Tx) LoadDirectMessageUploadForDelete(ctx context.Context, messageID, senderID, peerID uuid.UUID) (Message, Upload, error) {
	var message Message
	var upload Upload
	var content *string
	var duration *int
	err := t.tx.QueryRow(ctx, findDirectMessageUploadForDeleteQuery, messageID, senderID, peerID).Scan(
		&message.ID, &message.CircleID, &message.DMRecipientID, &message.SenderID, &message.Type, &content,
		&message.UploadID, &message.ReplyToID, &message.PinnedBy, &message.PinnedAt, &message.State, &message.SentAt, &message.DeletedAt,
		&upload.ID, &upload.UploaderID, &upload.AuthorizationCircleID, &upload.DMPeerID, &upload.ObjectKey,
		&upload.MIMEType, &upload.OriginalFileName, &upload.SizeBytes, &duration, &upload.State, &upload.CreatedAt, &upload.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Message{}, Upload{}, ErrMessageNotVisible
	}
	if err != nil {
		return Message{}, Upload{}, fmt.Errorf("load direct message upload for delete: %w", err)
	}
	if content != nil {
		message.Content = *content
	}
	if duration != nil {
		upload.DurationSeconds = *duration
	}
	return message, upload, nil
}

// DeleteOwnDirectMessage soft-deletes a sender's recent direct message and
// reports whether this call changed the row. A prior successful deletion is a
// safe idempotent replay.
func (t *Tx) DeleteOwnDirectMessage(ctx context.Context, messageID, senderID, peerID uuid.UUID) (bool, error) {
	var deletedAt *time.Time
	var sentAt time.Time
	err := t.tx.QueryRow(ctx, lockOwnDirectMessageForDeleteQuery, messageID, senderID, peerID).Scan(&deletedAt, &sentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrMessageNotVisible
	}
	if err != nil {
		return false, fmt.Errorf("lock direct message for delete: %w", err)
	}
	if deletedAt != nil {
		return false, nil
	}
	if sentAt.Before(time.Now().UTC().Add(-10 * time.Minute)) {
		return false, ErrDirectDeleteConflict
	}
	var deletedID uuid.UUID
	err = t.tx.QueryRow(ctx, softDeleteOwnDirectMessageQuery, messageID, senderID, peerID).Scan(&deletedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrDirectDeleteConflict
	}
	if err != nil {
		return false, fmt.Errorf("delete own direct message: %w", err)
	}
	return true, nil
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

// LockMessageForModeration locks a message before checking authority and time.
func (t *Tx) LockMessageForModeration(ctx context.Context, messageID uuid.UUID) (Message, error) {
	message, err := scanMessage(t.tx.QueryRow(ctx, lockMessageForModerationQuery, messageID))
	if errors.Is(err, pgx.ErrNoRows) { return Message{}, ErrMessageNotVisible }
	if err != nil { return Message{}, fmt.Errorf("lock message for moderation: %w", err) }
	return message, nil
}

// LoadMessageUploadForDelete locks an attached upload for marker-before-commit deletion.
func (t *Tx) LoadMessageUploadForDelete(ctx context.Context, messageID uuid.UUID) (Upload, error) {
	var message Message
	var upload Upload
	var content *string
	var duration *int
	err := t.tx.QueryRow(ctx, findMessageUploadForDeleteQuery, messageID).Scan(
		&message.ID, &message.CircleID, &message.DMRecipientID, &message.SenderID, &message.Type, &content,
		&message.UploadID, &message.ReplyToID, &message.PinnedBy, &message.PinnedAt, &message.State, &message.SentAt, &message.DeletedAt,
		&upload.ID, &upload.UploaderID, &upload.AuthorizationCircleID, &upload.DMPeerID, &upload.ObjectKey, &upload.MIMEType,
		&upload.OriginalFileName, &upload.SizeBytes, &duration, &upload.State, &upload.CreatedAt, &upload.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) { return Upload{}, ErrMessageNotVisible }
	if err != nil { return Upload{}, fmt.Errorf("load message upload for deletion: %w", err) }
	return upload, nil
}

// SoftDeleteMessage applies the authoritative database-time deadline and clears pins.
func (t *Tx) SoftDeleteMessage(ctx context.Context, messageID, actorID uuid.UUID, teacher bool) (bool, error) {
	var deletedID uuid.UUID
	err := t.tx.QueryRow(ctx, softDeleteMessageQuery, messageID, actorID, teacher).Scan(&deletedID)
	if errors.Is(err, pgx.ErrNoRows) { return false, nil }
	if err != nil { return false, fmt.Errorf("soft-delete chat message: %w", err) }
	return true, nil
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
	_, anchorID, err := r.messageCursor(ctx, before)
	if err != nil {
		return nil, err
	}
	rows, err := r.pool.Query(ctx, groupSearchPageQuery, circleID, viewerID, query, anchorID, limit)
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

// ListPinnedMessages returns the visible, active pinned bar for one group.
func (r *Repository) ListPinnedMessages(ctx context.Context, circleID, viewerID uuid.UUID) ([]Message, error) {
	rows, err := r.pool.Query(ctx, listPinnedMessagesQuery, circleID, viewerID)
	if err != nil {
		return nil, fmt.Errorf("list pinned group messages: %w", err)
	}
	defer rows.Close()
	var messages []Message
	for rows.Next() {
		message, err := scanMessage(rows)
		if err != nil {
			return nil, fmt.Errorf("scan pinned group message: %w", err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pinned group messages: %w", err)
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
		&m.UploadID, &m.ReplyToID, &m.PinnedBy, &m.PinnedAt, &m.State, &m.SentAt, &m.DeletedAt,
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
