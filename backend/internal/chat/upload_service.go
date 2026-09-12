package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

// MaxUploadsPerRollingHour is the successfully staged upload budget per user
// per rolling hour across devices (FR-022).
const MaxUploadsPerRollingHour = 10

// MediaURLTTL is the presigned chat-media link lifetime (FR-024: seven days).
const MediaURLTTL = 7 * 24 * time.Hour

// MaxFileNameRunes bounds the sanitized display filename to the database
// column width; the stored name is metadata only and never an object path.
const MaxFileNameRunes = 255

var (
	// ErrUploadRateExceeded indicates the uploader's rolling-hour budget of
	// successfully staged uploads is exhausted.
	ErrUploadRateExceeded = errors.New("chat: upload rate exceeded")
	// ErrDMNotEligible indicates the pair shares no active circle whose
	// current roles form a teacher-student or supervisor-student pair.
	ErrDMNotEligible = errors.New("chat: dm pair not eligible")
	// ErrUploadNotAttachable indicates a send attempted to attach an upload
	// owned by another uploader or bound to another circle, DM context, or
	// message family.
	ErrUploadNotAttachable = errors.New("chat: upload not attachable")
	// ErrMessageNotVisible non-enumeratingly denies media renewal for a
	// missing, deleted, text, unattached, or unauthorized message.
	ErrMessageNotVisible = errors.New("chat: message not visible")
)

// uploadStore is the narrow chat-persistence seam the upload service depends
// on; *Repository satisfies it in production and unit tests fake it without
// PostgreSQL (the same seam strategy as StagedUploadSource).
type uploadStore interface {
	// CountRecentUploads counts the uploader's staged or attached uploads
	// created since the given instant.
	CountRecentUploads(ctx context.Context, uploaderID uuid.UUID, since time.Time) (int, error)
	// InsertUpload stages one private attachment row.
	InsertUploadWithinBudget(ctx context.Context, upload Upload, since time.Time) (Upload, error)
	// FindQualifyingDMCircle returns one shared active circle currently
	// authorizing the unordered pair, or uuid.Nil with false when none does.
	FindQualifyingDMCircle(ctx context.Context, userA, userB uuid.UUID) (uuid.UUID, bool, error)
	// FindMessageUpload loads one message together with its attached upload.
	FindMessageUpload(ctx context.Context, messageID uuid.UUID) (Message, Upload, error)
	// WithTx runs fn atomically: fn's mutations commit together or not at all.
	WithTx(ctx context.Context, fn func(*Tx) error) error
}

var _ uploadStore = (*Repository)(nil)

// UploadTarget binds a staged upload to exactly one conversation context: a
// group circle or a direct-message peer, mutually exclusively (FR-023).
type UploadTarget struct {
	CircleID *uuid.UUID
	DMPeerID *uuid.UUID
}

// StageUploadInput is one untrusted upload request: the authenticated
// uploader, the exclusive conversation target, and the declared media
// payload whose type, size, and duration the server re-derives.
type StageUploadInput struct {
	UploaderID       uuid.UUID
	Target           UploadTarget
	MediaType        MessageType
	Data             []byte
	DeclaredFileName string
	DurationSeconds  int
}

// SendGroupMediaInput identifies one idempotent group media send.
type SendGroupMediaInput struct {
	SenderID       uuid.UUID
	CircleID       uuid.UUID
	UploadID       uuid.UUID
	MessageType    MessageType
	IdempotencyKey string
}

// StagedUpload is the accepted-upload response data: the upload identity, the
// legacy object_key compatibility field, and a seven-day presigned preview
// URL with its expiry.
type StagedUpload struct {
	UploadID     uuid.UUID
	ObjectKey    string
	URL          *url.URL
	URLExpiresAt time.Time
}

// MediaAccess is a freshly renewed presigned chat-media URL.
type MediaAccess struct {
	URL                  *url.URL
	ExpiresAt            time.Time
	FileName             string
	VoiceDurationSeconds int
}

// UploadService implements US3 staged chat uploads: server-side media
// validation, group/DM context binding, attach-once media sends, and
// seven-day renewable presigned access. Identity and backend sessions are
// verified by HTTP middleware; authorization is re-evaluated from
// PostgreSQL at operation time (SR-002).
type UploadService struct {
	repo       uploadStore
	membership MembershipReader
	store      *MediaStore
	cleaner    *Cleaner
	metrics    *metrics.ChatMetrics
	audit      *logging.AuditLogger
	now        func() time.Time
	validate   func(context.Context, MessageType, string, []byte) (int, error)
}

// NewUploadService constructs the staged chat upload service. repo is
// satisfied by *Repository; cleaner may be nil to skip best-effort cleanup of
// failed finalizations; chatMetrics and audit are optional.
func NewUploadService(repo uploadStore, membership MembershipReader, store *MediaStore, cleaner *Cleaner, chatMetrics *metrics.ChatMetrics, audit *logging.AuditLogger) *UploadService {
	return &UploadService{
		repo:       repo,
		membership: membership,
		store:      store,
		cleaner:    cleaner,
		metrics:    chatMetrics,
		audit:      audit,
		now:        time.Now,
		validate:   validateMediaPayload,
	}
}

// detectMIME determines the actual media type from the leading bytes.
// Filenames and client Content-Type headers are never authoritative
// (FR-022): OggS→audio/ogg, ID3 or an 11-bit MPEG frame sync→audio/mpeg, an
// ftyp box→audio/mp4, an EBML header→audio/webm, %PDF-→application/pdf, a
// JPEG SOI→image/jpeg, and the PNG signature→image/png. Unknown payloads
// return "" so ValidateUpload rejects them.
func detectMIME(data []byte) string {
	switch {
	case bytes.HasPrefix(data, []byte("OggS")):
		return "audio/ogg"
	case bytes.HasPrefix(data, []byte("ID3")):
		return "audio/mpeg"
	case len(data) >= 2 && data[0] == 0xFF && data[1]&0xE0 == 0xE0:
		return "audio/mpeg"
	case len(data) >= 8 && bytes.Equal(data[4:8], []byte("ftyp")):
		return "audio/mp4"
	case len(data) >= 4 && bytes.Equal(data[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return "audio/webm"
	case bytes.HasPrefix(data, []byte("%PDF-")):
		return "application/pdf"
	case len(data) >= 3 && data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}):
		return "image/png"
	default:
		return ""
	}
}

// sanitizeFileName produces the bounded, path-free, control-free display
// name stored as upload metadata. It never participates in object paths and
// always satisfies the database's trimmed non-empty check.
func sanitizeFileName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == 0x7F {
			continue
		}
		b.WriteRune(r)
	}
	cleaned := strings.TrimSpace(b.String())
	if cleaned == "" {
		return "file"
	}
	if runes := []rune(cleaned); len(runes) > MaxFileNameRunes {
		cleaned = string(runes[:MaxFileNameRunes])
	}
	return strings.TrimSpace(cleaned)
}

// mediaMessageType maps a server-detected MIME type onto the message family
// it may attach as: audio/*→voice, image/*→image, application/pdf→file.
// Types outside the allowlist map to the empty, unattachable message type.
func mediaMessageType(mime string) MessageType {
	switch {
	case strings.HasPrefix(mime, "audio/"):
		return MessageTypeVoice
	case strings.HasPrefix(mime, "image/"):
		return MessageTypeImage
	case mime == "application/pdf":
		return MessageTypeFile
	default:
		return MessageType("")
	}
}

// validateUploadTarget enforces exactly one conversation context and rejects
// self-addressed DM targets before any dependency is touched.
func validateUploadTarget(uploaderID uuid.UUID, target UploadTarget) error {
	if (target.CircleID == nil) == (target.DMPeerID == nil) {
		return ErrInvalidContext
	}
	if target.DMPeerID != nil && *target.DMPeerID == uploaderID {
		return ErrInvalidContext
	}
	return nil
}

// checkAttachableUpload reauthorizes one upload for a group attach at send
// time (edge case: an object cannot be attached by another user or reused
// across an unauthorized circle or DM context). The upload must still be
// staged, belong to the sender, target the same circle as a group upload,
// and match the message family of its MIME type.
func checkAttachableUpload(upload Upload, senderID, circleID uuid.UUID, msgType MessageType) error {
	if upload.UploaderID != senderID {
		return ErrUploadNotAttachable
	}
	if upload.State != UploadStateStaged {
		return ErrUploadNotStaged
	}
	if upload.DMPeerID != nil {
		return ErrUploadNotAttachable
	}
	if upload.AuthorizationCircleID != circleID {
		return ErrUploadNotAttachable
	}
	if mediaMessageType(upload.MIMEType) != msgType {
		return ErrUploadNotAttachable
	}
	return nil
}

// Stage validates and stores one private chat attachment bound to the
// uploader and exactly one authorized conversation context, returning the
// staged identity with a seven-day presigned preview URL. Validation and
// authorization failures create no staged object and consume no upload
// budget; only successfully staged uploads count (FR-022).
func (s *UploadService) Stage(ctx context.Context, in StageUploadInput) (StagedUpload, error) {
	start := time.Now()
	if err := validateUploadTarget(in.UploaderID, in.Target); err != nil {
		return s.rejectUpload(start, err)
	}
	mime := detectMIME(in.Data)
	if err := ValidateUpload(UploadInput{
		Type:            in.MediaType,
		MIMEType:        mime,
		SizeBytes:       int64(len(in.Data)),
		DurationSeconds: in.DurationSeconds,
	}); err != nil {
		return s.rejectUpload(start, err)
	}
	duration, err := s.validate(ctx, in.MediaType, mime, in.Data)
	if err != nil {
		return s.rejectUpload(start, err)
	}
	if err := ValidateUpload(UploadInput{
		Type:            in.MediaType,
		MIMEType:        mime,
		SizeBytes:       int64(len(in.Data)),
		DurationSeconds: duration,
	}); err != nil {
		return s.rejectUpload(start, err)
	}

	authCircle, err := s.authorizeUploadTarget(ctx, in.UploaderID, in.Target, start)
	if err != nil {
		return StagedUpload{}, err
	}
	if err := s.checkUploadBudget(ctx, in.UploaderID, start); err != nil {
		return StagedUpload{}, err
	}

	uploadID := uuid.New()
	key, err := s.store.Stage(ctx, StageInput{
		UploadID:  uploadID,
		Type:      in.MediaType,
		MIMEType:  mime,
		SizeBytes: int64(len(in.Data)),
		Body:      bytes.NewReader(in.Data),
	})
	if err != nil {
		return s.failUpload(start, fmt.Errorf("stage chat upload: %w", err))
	}
	staged, err := s.repo.InsertUploadWithinBudget(ctx, Upload{
		ID:                    uploadID,
		UploaderID:            in.UploaderID,
		AuthorizationCircleID: authCircle,
		DMPeerID:              in.Target.DMPeerID,
		ObjectKey:             key,
		MIMEType:              mime,
		OriginalFileName:      sanitizeFileName(in.DeclaredFileName),
		SizeBytes:             int64(len(in.Data)),
		DurationSeconds:       duration,
	}, s.now().Add(-time.Hour))
	if err != nil {
		s.cleanupStagedObject(ctx, key)
		if errors.Is(err, ErrUploadRateExceeded) {
			return s.rejectUpload(start, err)
		}
		return s.failUpload(start, fmt.Errorf("persist chat upload: %w", err))
	}
	preview, err := s.store.PresignGet(ctx, staged.ObjectKey, MediaURLTTL)
	if err != nil {
		// The upload stays staged: retry, renewal, and the 24-hour staged
		// cleaner own its fate, so no cleanup marker is applied here.
		return s.failUpload(start, fmt.Errorf("presign chat upload preview: %w", err))
	}

	s.metrics.RecordUpload(metrics.ChatUploadAccepted)
	s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeAccepted, time.Since(start))
	if s.audit != nil {
		s.audit.LogChat(ctx, logging.ChatUploadAuditEvent(in.UploaderID.String(), authCircle.String(), staged.ID.String(), logging.ChatOutcomeAccepted))
	}
	return StagedUpload{
		UploadID:     staged.ID,
		ObjectKey:    staged.ObjectKey,
		URL:          preview,
		URLExpiresAt: s.now().Add(MediaURLTTL),
	}, nil
}

// authorizeUploadTarget rechecks the target context from current state and
// returns the authorization circle to bind: a group target needs an active
// membership in that circle; a DM target needs one currently qualifying
// shared active circle as witness while the object belongs to the pair
// conversation (FR-023).
func (s *UploadService) authorizeUploadTarget(ctx context.Context, uploaderID uuid.UUID, target UploadTarget, start time.Time) (uuid.UUID, error) {
	if target.CircleID == nil {
		witness, eligible, err := s.repo.FindQualifyingDMCircle(ctx, uploaderID, *target.DMPeerID)
		if err != nil {
			s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeFailure, time.Since(start))
			return uuid.Nil, fmt.Errorf("authorize chat dm upload: %w", err)
		}
		if !eligible {
			s.recordDenial(ctx, uploaderID, uuid.Nil, metrics.ChatDenialIneligible)
			s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeDenied, time.Since(start))
			return uuid.Nil, ErrDMNotEligible
		}
		return witness, nil
	}
	if err := authorizeActiveCircleMember(ctx, s.membership, uploaderID, *target.CircleID, func(reason metrics.ChatDenial) {
		s.recordDenial(ctx, uploaderID, *target.CircleID, reason)
	}); err != nil {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeDenied, time.Since(start))
		return uuid.Nil, err
	}
	return *target.CircleID, nil
}

// checkUploadBudget enforces the rolling-hour budget of successfully staged
// uploads per uploader across devices before any object is written (FR-022).
func (s *UploadService) checkUploadBudget(ctx context.Context, uploaderID uuid.UUID, start time.Time) error {
	count, err := s.repo.CountRecentUploads(ctx, uploaderID, s.now().Add(-time.Hour))
	if err != nil {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeFailure, time.Since(start))
		return fmt.Errorf("count recent chat uploads: %w", err)
	}
	if count >= MaxUploadsPerRollingHour {
		s.metrics.RecordUpload(metrics.ChatUploadRejected)
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeRejected, time.Since(start))
		return ErrUploadRateExceeded
	}
	return nil
}

// rejectUpload records one bounded rejection that staged no object.
func (s *UploadService) rejectUpload(start time.Time, err error) (StagedUpload, error) {
	s.metrics.RecordUpload(metrics.ChatUploadRejected)
	s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeRejected, time.Since(start))
	return StagedUpload{}, err
}

// failUpload records one storage-side failure of an accepted upload.
func (s *UploadService) failUpload(start time.Time, err error) (StagedUpload, error) {
	s.metrics.RecordUpload(metrics.ChatUploadStorageFailure)
	s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeFailure, time.Since(start))
	return StagedUpload{}, err
}

// SendGroupMedia durably accepts one group media message bound to a staged
// upload, reauthorizing sender, ownership, context, state, and message
// family inside the same transaction that inserts the message: the upload
// must belong to the sender, be still staged, carry no DM binding, target
// the same circle, and match the declared message family. Attachment is
// single-use; a replayed (sender, idempotency key) returns the committed
// original without a second row, event, or attach.
func (s *UploadService) SendGroupMedia(ctx context.Context, in SendGroupMediaInput) (Message, error) {
	start := time.Now()
	if err := ValidateIdempotencyKey(in.IdempotencyKey); err != nil {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationSend, metrics.ChatOutcomeRejected, time.Since(start))
		return Message{}, err
	}
	if err := ValidateMessageInput(MessageInput{
		SenderID: in.SenderID,
		CircleID: &in.CircleID,
		Type:     in.MessageType,
		UploadID: &in.UploadID,
	}); err != nil {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationSend, metrics.ChatOutcomeRejected, time.Since(start))
		return Message{}, err
	}
	if err := authorizeActiveCircleMember(ctx, s.membership, in.SenderID, in.CircleID, func(reason metrics.ChatDenial) {
		s.recordDenial(ctx, in.SenderID, in.CircleID, reason)
	}); err != nil {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationSend, metrics.ChatOutcomeDenied, time.Since(start))
		return Message{}, err
	}

	var sent Message
	err := s.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockActiveCircleMember(ctx, in.CircleID, in.SenderID); err != nil {
			return err
		}
		// The row lock serializes concurrent attach attempts on the upload
		// before any message insert.
		upload, err := tx.LoadUploadForUpdate(ctx, in.UploadID)
		if err != nil {
			return err
		}
		if err := checkAttachableUpload(upload, in.SenderID, in.CircleID, in.MessageType); err != nil {
			if !errors.Is(err, ErrUploadNotStaged) {
				return err
			}
			// A consumed upload still serves one legitimate continuation: the
			// idempotent replay of the send that attached it. Checking before
			// the insert keeps a fresh-key re-attach from ever reaching the
			// messages.upload_id unique constraint as a raw 23505.
			existing, found, lookupErr := tx.FindMessageByIdempotency(ctx, in.SenderID, in.IdempotencyKey)
			if lookupErr != nil {
				return lookupErr
			}
			if !found {
				return ErrUploadNotStaged
			}
			if existing.UploadID == nil || *existing.UploadID != in.UploadID ||
				!sameUUID(existing.CircleID, &in.CircleID) || existing.DMRecipientID != nil || existing.Type != in.MessageType {
				return ErrIdempotencyConflict
			}
			sent = existing
			return nil
		}
		msg, inserted, err := tx.InsertMessage(ctx, MessageInput{
			SenderID:       in.SenderID,
			CircleID:       &in.CircleID,
			Type:           in.MessageType,
			UploadID:       &in.UploadID,
			IdempotencyKey: in.IdempotencyKey,
		})
		if err != nil {
			return err
		}
		sent = msg
		if !inserted {
			// Idempotent replay raced a concurrent same-key send: the
			// original transaction committed the message and its single
			// attach atomically.
			return nil
		}
		if _, err := tx.AttachUpload(ctx, in.UploadID); err != nil {
			return err
		}
		return tx.InsertOutboxEvent(ctx, msg.ID, realtime.EventChatMessage, nil)
	})
	if err != nil {
		if errors.Is(err, ErrUploadNotAttachable) || errors.Is(err, ErrUploadNotStaged) {
			s.metrics.RecordLatencyOutcome(metrics.ChatOperationSend, metrics.ChatOutcomeConflict, time.Since(start))
			return Message{}, err
		}
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationSend, metrics.ChatOutcomeFailure, time.Since(start))
		return Message{}, fmt.Errorf("send chat media message: %w", err)
	}
	s.metrics.RecordLatencyOutcome(metrics.ChatOperationSend, metrics.ChatOutcomeAccepted, time.Since(start))
	if s.audit != nil {
		s.audit.LogChat(ctx, logging.ChatMessageAuditEvent(in.SenderID.String(), in.CircleID.String(), sent.ID.String(), logging.ChatOutcomeAccepted))
	}
	return sent, nil
}

// RenewMediaURL reauthorizes the viewer and returns a fresh seven-day
// presigned URL for one attached media message (FR-024). Group visibility
// follows the viewer's current membership period — retained members of an
// archived circle keep playing retained history (FR-032) — and DM renewal
// re-evaluates current pair eligibility (FR-017). Every denial is
// non-enumerating.
func (s *UploadService) RenewMediaURL(ctx context.Context, viewerID, messageID uuid.UUID) (MediaAccess, error) {
	start := time.Now()
	deny := func(circle uuid.UUID, err error) (MediaAccess, error) {
		s.recordDenial(ctx, viewerID, circle, metrics.ChatDenialIneligible)
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeDenied, time.Since(start))
		return MediaAccess{}, err
	}
	fail := func(err error) (MediaAccess, error) {
		s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeFailure, time.Since(start))
		return MediaAccess{}, err
	}

	msg, upload, err := s.repo.FindMessageUpload(ctx, messageID)
	if err != nil {
		if errors.Is(err, ErrMessageNotVisible) {
			return deny(uuid.Nil, ErrMessageNotVisible)
		}
		return fail(fmt.Errorf("load chat message upload: %w", err))
	}
	if msg.State != MessageStateActive || upload.State != UploadStateAttached {
		return deny(messageCircle(msg), ErrMessageNotVisible)
	}

	switch {
	case msg.CircleID != nil:
		if err := s.authorizeGroupMediaView(ctx, viewerID, msg); err != nil {
			if errors.Is(err, ErrMessageNotVisible) {
				return deny(*msg.CircleID, err)
			}
			return fail(err)
		}
	case msg.DMRecipientID != nil:
		if err := s.authorizeDMMediaView(ctx, viewerID, msg); err != nil {
			if errors.Is(err, ErrDMNotEligible) {
				s.recordDenial(ctx, viewerID, uuid.Nil, metrics.ChatDenialIneligible)
				s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeDenied, time.Since(start))
				return MediaAccess{}, err
			}
			if errors.Is(err, ErrMessageNotVisible) {
				return deny(uuid.Nil, err)
			}
			return fail(err)
		}
	default:
		return deny(uuid.Nil, ErrMessageNotVisible)
	}

	signed, err := s.store.PresignGet(ctx, upload.ObjectKey, MediaURLTTL)
	if err != nil {
		return fail(fmt.Errorf("presign chat media url: %w", err))
	}
	s.metrics.RecordLatencyOutcome(metrics.ChatOperationUpload, metrics.ChatOutcomeAccepted, time.Since(start))
	access := MediaAccess{URL: signed, ExpiresAt: s.now().Add(MediaURLTTL), FileName: sanitizeFileName(upload.OriginalFileName)}
	if msg.Type == MessageTypeVoice {
		access.VoiceDurationSeconds = upload.DurationSeconds
	}
	return access, nil
}

// messageCircle returns the message's circle for denial accounting, or the
// nil UUID for direct messages.
func messageCircle(msg Message) uuid.UUID {
	if msg.CircleID != nil {
		return *msg.CircleID
	}
	return uuid.Nil
}

// authorizeGroupMediaView rechecks group visibility for one renewal: the
// viewer must currently belong to the circle and the message must have been
// accepted within the viewer's current membership period. Archival does not
// revoke retained read access (FR-032). ErrMessageNotVisible denies;
// anything else is an infrastructure failure.
func (s *UploadService) authorizeGroupMediaView(ctx context.Context, viewerID uuid.UUID, msg Message) error {
	if err := authorizeRetainedCircleMember(ctx, s.membership, viewerID, *msg.CircleID, func(metrics.ChatDenial) {}); err != nil {
		if errors.Is(err, ErrCircleNotVisible) {
			return ErrMessageNotVisible
		}
		return err
	}
	periods, ok := s.membership.(membershipPeriodReader)
	if !ok {
		return ErrMessageNotVisible
	}
	joined, err := periods.MembershipStartedAt(ctx, msg.CircleID.String(), viewerID.String())
	if err != nil {
		return fmt.Errorf("load chat membership period: %w", err)
	}
	if msg.SentAt.Before(joined) {
		return ErrMessageNotVisible
	}
	return nil
}

// authorizeDMMediaView rechecks direct-message visibility for one renewal:
// the viewer must be one of the pair's two members and the pair must still
// share a qualifying active circle (FR-017). ErrMessageNotVisible denies an
// outsider; ErrDMNotEligible denies a pair member after eligibility loss.
func (s *UploadService) authorizeDMMediaView(ctx context.Context, viewerID uuid.UUID, msg Message) error {
	var other uuid.UUID
	switch viewerID {
	case msg.SenderID:
		other = *msg.DMRecipientID
	case *msg.DMRecipientID:
		other = msg.SenderID
	default:
		return ErrMessageNotVisible
	}
	_, eligible, err := s.repo.FindQualifyingDMCircle(ctx, viewerID, other)
	if err != nil {
		return fmt.Errorf("authorize chat dm media renewal: %w", err)
	}
	if !eligible {
		return ErrDMNotEligible
	}
	return nil
}

// recordDenial records one bounded authorization denial in metrics and a
// redacted audit event; denial records never carry filenames, object keys,
// or URLs (SR-006).
func (s *UploadService) recordDenial(ctx context.Context, actor, circle uuid.UUID, reason metrics.ChatDenial) {
	recordChatDenial(ctx, s.metrics, s.audit, actor, circle, reason)
}

// cleanupStagedObject applies the revoking delete marker for a staged object
// whose database finalization failed, reusing the staged-upload cleaner seam
// (media_cleanup.go); the best-effort error is dropped because the original
// failure must dominate.
func (s *UploadService) cleanupStagedObject(ctx context.Context, objectKey string) {
	if s.cleaner == nil {
		return
	}
	_ = s.cleaner.CleanupFailedFinalization(ctx, Upload{ObjectKey: objectKey, State: UploadStateStaged})
}
