package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// --- Fakes ---------------------------------------------------------------------

// stubMembershipReader answers the MembershipReader and membershipPeriodReader
// seams from fixed state; production uses *rbac.Repository. It is distinct
// from realtime_projector_test.go's fakeMembershipReader because renewal tests
// also need the membership-period seam.
type stubMembershipReader struct {
	circle    rbac.Circle
	circleErr error
	member    bool
	joinedAt  time.Time
}

func (f *stubMembershipReader) IsMember(context.Context, string, string) (bool, error) {
	return f.member, nil
}

func (f *stubMembershipReader) FindCircleByID(context.Context, string) (rbac.Circle, error) {
	return f.circle, f.circleErr
}

func (f *stubMembershipReader) MembershipStartedAt(context.Context, string, string) (time.Time, error) {
	return f.joinedAt, nil
}

// fakeUploadStore fakes the uploadStore seam satisfied by *Repository.
type fakeUploadStore struct {
	recentCount  int
	gotUploader  uuid.UUID
	gotSince     time.Time
	inserts      []Upload
	insertErr    error
	witness      uuid.UUID
	eligible     bool
	gotDMUserA   uuid.UUID
	gotDMUserB   uuid.UUID
	found        bool
	foundMessage Message
	foundUpload  Upload
	txErr        error
}

func (f *fakeUploadStore) CountRecentUploads(_ context.Context, uploaderID uuid.UUID, since time.Time) (int, error) {
	f.gotUploader, f.gotSince = uploaderID, since
	return f.recentCount, nil
}

func (f *fakeUploadStore) InsertUpload(_ context.Context, upload Upload) (Upload, error) {
	if f.insertErr != nil {
		return Upload{}, f.insertErr
	}
	if upload.ID == uuid.Nil {
		// Mirrors the database's gen_random_uuid() default.
		upload.ID = uuid.New()
	}
	upload.State = UploadStateStaged
	upload.CreatedAt = time.Now().UTC()
	upload.UpdatedAt = upload.CreatedAt
	f.inserts = append(f.inserts, upload)
	return upload, nil
}

func (f *fakeUploadStore) InsertUploadWithinBudget(ctx context.Context, upload Upload, _ time.Time) (Upload, error) {
	return f.InsertUpload(ctx, upload)
}

func (f *fakeUploadStore) FindQualifyingDMCircle(_ context.Context, userA, userB uuid.UUID) (uuid.UUID, bool, error) {
	f.gotDMUserA, f.gotDMUserB = userA, userB
	return f.witness, f.eligible, nil
}

func (f *fakeUploadStore) FindMessageUpload(context.Context, uuid.UUID) (Message, Upload, error) {
	if !f.found {
		return Message{}, Upload{}, ErrMessageNotVisible
	}
	return f.foundMessage, f.foundUpload, nil
}

// WithTx never executes its closure: the transactional send path requires
// PostgreSQL and is covered by the integration suites. Tests drive only the
// pre-transaction outcomes plus the wrapped transaction failure.
func (f *fakeUploadStore) WithTx(context.Context, func(*Tx) error) error {
	return f.txErr
}

// newTestUploadService wires a Stage/RenewMediaURL-testable service over the
// fakes with a pinned clock.
func newTestUploadService(repo uploadStore, membership MembershipReader, fake *fakeObjectClient, chatMetrics *metrics.ChatMetrics, now time.Time) *UploadService {
	svc := NewUploadService(repo, membership, newTestMediaStore(fake), newTestCleaner(&fakeStagedSource{}, fake, now), chatMetrics, nil)
	svc.now = func() time.Time { return now }
	svc.validate = func(_ context.Context, kind MessageType, mime string, data []byte) (int, error) {
		if !plausibleMediaFixture(mime, data) {
			return 0, ErrUnsupportedMIME
		}
		if kind == MessageTypeVoice {
			return 30, nil
		}
		return 0, nil
	}
	return svc
}

func plausibleMediaFixture(mime string, data []byte) bool {
	return detectMIME(data) == mime && len(data) >= 4
}

// --- Byte fixtures --------------------------------------------------------------

var (
	oggMagic  = append([]byte("OggS"), bytes.Repeat([]byte{0x01}, 60)...)
	mp3ID3    = []byte("ID3\x03\x00\x00\x00\x00\x00\x00")
	mp3Sync   = []byte{0xFF, 0xFB, 0x90, 0x00, 0x00, 0x00, 0x00, 0x00}
	mp4Magic  = append([]byte{0x00, 0x00, 0x00, 0x20, 'f', 't', 'y', 'p', 'M', '4', 'A', ' '}, bytes.Repeat([]byte{0x01}, 52)...)
	webmMagic = []byte{0x1A, 0x45, 0xDF, 0xA3, 0x9F, 0x42, 0x86, 0x81}
	pdfMagic  = append(append([]byte("%PDF-1.7\n%âãÏÓ"), bytes.Repeat([]byte{0x25}, 48)...), []byte("%%EOF")...)
	jpegMagic = append([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}, append(bytes.Repeat([]byte{0x01}, 52), 0xff, 0xd9)...)
	pngMagic  = append([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}, bytes.Repeat([]byte{0x01}, 54)...)
)

// mediaWithPrefix builds a size-byte payload whose leading bytes are prefix,
// so size-limit tests keep a valid magic signature.
func mediaWithPrefix(prefix []byte, size int64) []byte {
	data := make([]byte, size)
	copy(data, prefix)
	return data
}

func groupStageInput(uploader uuid.UUID, circleID uuid.UUID, mediaType MessageType, data []byte, duration int) StageUploadInput {
	return StageUploadInput{
		UploaderID:       uploader,
		Target:           UploadTarget{CircleID: &circleID},
		MediaType:        mediaType,
		Data:             data,
		DeclaredFileName: "note.ext",
		DurationSeconds:  duration,
	}
}

// --- Magic-byte sniffing ----------------------------------------------------------

// TestDetectMIME pins the server-side MIME truth: signatures, not filenames
// or client Content-Type, decide the detected type (FR-022).
func TestDetectMIME(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want string
	}{
		{name: "ogg container", data: oggMagic, want: "audio/ogg"},
		{name: "mp3 id3 tag", data: mp3ID3, want: "audio/mpeg"},
		{name: "mp3 raw frame sync", data: mp3Sync, want: "audio/mpeg"},
		{name: "mp3 sync at minimum 0xE0", data: []byte{0xFF, 0xE0, 0x00}, want: "audio/mpeg"},
		{name: "mp4 ftyp box", data: mp4Magic, want: "audio/mp4"},
		{name: "webm ebml header", data: webmMagic, want: "audio/webm"},
		{name: "pdf header", data: pdfMagic, want: "application/pdf"},
		{name: "jpeg soi", data: jpegMagic, want: "image/jpeg"},
		{name: "png signature", data: pngMagic, want: "image/png"},
		{name: "plain text is unknown", data: []byte("hello world"), want: ""},
		{name: "gif is not supported", data: []byte("GIF89a"), want: ""},
		{name: "empty payload", data: nil, want: ""},
		{name: "truncated ogg", data: []byte("Ogg"), want: ""},
		{name: "truncated ebml", data: []byte{0x1A, 0x45, 0xDF}, want: ""},
		{name: "ftyp at wrong offset", data: append([]byte("nope"), mp4Magic...), want: ""},
		{name: "jpeg soi without third ff byte", data: []byte{0xFF, 0xD8, 0xE0}, want: ""},
		{name: "mpeg sync needs eleven set bits", data: []byte{0xFF, 0x10, 0x00}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectMIME(tt.data); got != tt.want {
				t.Fatalf("detectMIME() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestMediaMessageType pins the MIME-family to message-type mapping used at
// attach time: audio uploads attach as voice, images as image, PDFs as file.
// The mapping is family-prefix based; only allowlisted mimes can ever be
// stored, so unreachable inputs like image/gif still resolve to their family.
func TestMediaMessageType(t *testing.T) {
	tests := []struct {
		mime string
		want MessageType
	}{
		{mime: "audio/ogg", want: MessageTypeVoice},
		{mime: "audio/mpeg", want: MessageTypeVoice},
		{mime: "audio/mp4", want: MessageTypeVoice},
		{mime: "audio/webm", want: MessageTypeVoice},
		{mime: "image/jpeg", want: MessageTypeImage},
		{mime: "image/png", want: MessageTypeImage},
		{mime: "application/pdf", want: MessageTypeFile},
		{mime: "text/plain", want: MessageType("")},
		{mime: "", want: MessageType("")},
	}
	for _, tt := range tests {
		t.Run(tt.mime, func(t *testing.T) {
			if got := mediaMessageType(tt.mime); got != tt.want {
				t.Fatalf("mediaMessageType(%q) = %q, want %q", tt.mime, got, tt.want)
			}
		})
	}
}

// --- Filename sanitization ---------------------------------------------------------

// TestSanitizeFileName pins the display-name contract: path components,
// control runes, and surrounding whitespace never reach the stored metadata,
// and the result always satisfies the non-empty trimmed database check.
func TestSanitizeFileName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "windows path stripped", input: `C:\evil\path\recitation.ogg`, want: "recitation.ogg"},
		{name: "posix path stripped", input: "/tmp/hidden/x.png", want: "x.png"},
		{name: "control runes removed", input: "re\x00ci\x1ftation.mp3", want: "recitation.mp3"},
		{name: "tabs and newlines removed", input: "note\t\n", want: "note"},
		{name: "trimmed", input: "  spaced  ", want: "spaced"},
		{name: "empty falls back", input: "", want: "file"},
		{name: "path-only falls back", input: `///`, want: "file"},
		{name: "whitespace-only falls back", input: "   ", want: "file"},
		{name: "bounded to 255 runes", input: strings.Repeat("م", 300), want: strings.Repeat("م", 255)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeFileName(tt.input)
			if got != tt.want {
				t.Fatalf("sanitizeFileName(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if strings.TrimSpace(got) != got {
				t.Fatalf("sanitizeFileName(%q) = %q must be trimmed for the database check", tt.input, got)
			}
		})
	}
}

// --- Stage: target context ---------------------------------------------------------

// TestUploadService_StageRejectsInvalidTargets proves the mutually exclusive
// context rule is enforced before any dependency is touched: the service is
// built from nil seams, so an ordering violation would panic instead of
// returning ErrInvalidContext.
func TestUploadService_StageRejectsInvalidTargets(t *testing.T) {
	svc := NewUploadService(nil, nil, nil, nil, nil, nil)
	uploader := uuid.New()
	circle := uuid.New()
	peer := uploader // self-DM case

	tests := []struct {
		name   string
		target UploadTarget
		want   error
	}{
		{name: "no context", target: UploadTarget{}, want: ErrInvalidContext},
		{name: "both contexts", target: UploadTarget{CircleID: &circle, DMPeerID: &peer}, want: ErrInvalidContext},
		{name: "self dm", target: UploadTarget{DMPeerID: &peer}, want: ErrInvalidContext},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := groupStageInput(uploader, circle, MessageTypeVoice, oggMagic, 30)
			in.Target = tt.target
			_, err := svc.Stage(context.Background(), in)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Stage() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestUploadService_StageValidatesMediaBeforePersistence proves every media
// limit rejects before any persistence, authorization, or object-store
// dependency is touched (nil seams panic on misuse) and therefore consumes no
// upload budget (FR-022).
func TestUploadService_StageValidatesMediaBeforePersistence(t *testing.T) {
	svc := NewUploadService(nil, nil, nil, nil, nil, nil)
	uploader := uuid.New()
	circle := uuid.New()

	tests := []struct {
		name     string
		media    MessageType
		data     []byte
		duration int
		want     error
	}{
		{name: "unknown magic is unsupported", media: MessageTypeVoice, data: []byte("just talking"), duration: 30, want: ErrUnsupportedMIME},
		{name: "empty payload is unsupported", media: MessageTypeVoice, data: nil, duration: 30, want: ErrUnsupportedMIME},
		{name: "gif is unsupported", media: MessageTypeImage, data: []byte("GIF89a more bytes"), want: ErrUnsupportedMIME},
		{name: "voice over 20 MB", media: MessageTypeVoice, data: mediaWithPrefix(oggMagic, MaxVoiceSizeBytes+1), duration: 30, want: ErrUploadTooLarge},
		{name: "image over 5 MB", media: MessageTypeImage, data: mediaWithPrefix(pngMagic, MaxImageSizeBytes+1), want: ErrUploadTooLarge},
		{name: "pdf over 10 MB", media: MessageTypeFile, data: mediaWithPrefix(pdfMagic, MaxFileSizeBytes+1), want: ErrUploadTooLarge},
		{name: "voice over 300 seconds", media: MessageTypeVoice, data: oggMagic, duration: MaxVoiceDurationSeconds + 1, want: ErrInvalidDuration},
		{name: "voice without duration", media: MessageTypeVoice, data: oggMagic, duration: 0, want: ErrInvalidDuration},
		{name: "image with duration", media: MessageTypeImage, data: jpegMagic, duration: 5, want: ErrInvalidDuration},
		{name: "text type has no allowlist", media: MessageTypeText, data: oggMagic, want: ErrUnsupportedMIME},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.Stage(context.Background(), groupStageInput(uploader, circle, tt.media, tt.data, tt.duration))
			if !errors.Is(err, tt.want) {
				t.Fatalf("Stage() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// --- Stage: authorization ----------------------------------------------------------

func memberMembership(t *testing.T, circleID uuid.UUID) *stubMembershipReader {
	t.Helper()
	return &stubMembershipReader{circle: rbac.Circle{ID: circleID.String()}, member: true}
}

// TestUploadService_StageAuthorizesGroupTarget proves a group target requires
// a visible, unarchived circle and current membership, and that every denial
// writes no object, no row, and issues no budget query.
func TestUploadService_StageAuthorizesGroupTarget(t *testing.T) {
	uploader := uuid.New()
	circle := uuid.New()
	tests := []struct {
		name       string
		membership *stubMembershipReader
		want       error
	}{
		{name: "unknown circle is not visible", membership: &stubMembershipReader{circleErr: rbac.ErrCircleNotFound}, want: ErrCircleNotVisible},
		{name: "non-member is not visible", membership: &stubMembershipReader{circle: rbac.Circle{ID: circle.String()}}, want: ErrCircleNotVisible},
		{name: "archived circle rejects upload", membership: &stubMembershipReader{circle: rbac.Circle{ID: circle.String(), IsArchived: true}, member: true}, want: ErrCircleArchived},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeObjectClient{}
			repo := &fakeUploadStore{}

			_, err := newTestUploadService(repo, tt.membership, fake, nil, time.Now()).
				Stage(context.Background(), groupStageInput(uploader, circle, MessageTypeVoice, oggMagic, 30))

			if !errors.Is(err, tt.want) {
				t.Fatalf("Stage() error = %v, want %v", err, tt.want)
			}
			if len(fake.puts) != 0 || len(repo.inserts) != 0 {
				t.Fatalf("denied upload must stage nothing: puts=%d inserts=%d", len(fake.puts), len(repo.inserts))
			}
		})
	}
}

// TestUploadService_StageAuthorizesDMTarget proves a DM target requires a
// currently qualifying shared active circle and stages nothing otherwise.
func TestUploadService_StageAuthorizesDMTarget(t *testing.T) {
	uploader := uuid.New()
	peer := uuid.New()
	fake := &fakeObjectClient{}
	repo := &fakeUploadStore{eligible: false}
	in := StageUploadInput{
		UploaderID:       uploader,
		Target:           UploadTarget{DMPeerID: &peer},
		MediaType:        MessageTypeVoice,
		Data:             oggMagic,
		DeclaredFileName: "note.ogg",
		DurationSeconds:  30,
	}

	_, err := newTestUploadService(repo, memberMembership(t, uuid.New()), fake, nil, time.Now()).
		Stage(context.Background(), in)

	if !errors.Is(err, ErrDMNotEligible) {
		t.Fatalf("Stage() error = %v, want ErrDMNotEligible", err)
	}
	if len(fake.puts) != 0 || len(repo.inserts) != 0 {
		t.Fatalf("ineligible DM upload must stage nothing: puts=%d inserts=%d", len(fake.puts), len(repo.inserts))
	}
	if repo.gotDMUserA != uploader || repo.gotDMUserB != peer {
		t.Fatalf("eligibility checked (%s, %s), want (%s, %s)", repo.gotDMUserA, repo.gotDMUserB, uploader, peer)
	}
}

// --- Stage: rolling-hour budget ------------------------------------------------------

// TestUploadService_StageEnforcesRollingHourBudget proves the tenth
// successfully staged upload in the rolling hour blocks the next attempt
// before any object write, while the tenth attempt itself (nine staged) still
// succeeds.
func TestUploadService_StageEnforcesRollingHourBudget(t *testing.T) {
	uploader := uuid.New()
	circle := uuid.New()
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)

	fake := &fakeObjectClient{}
	repo := &fakeUploadStore{recentCount: MaxUploadsPerRollingHour}
	metricsLog := &metrics.ChatMetrics{}

	_, err := newTestUploadService(repo, memberMembership(t, circle), fake, metricsLog, now).
		Stage(context.Background(), groupStageInput(uploader, circle, MessageTypeVoice, oggMagic, 30))

	if !errors.Is(err, ErrUploadRateExceeded) {
		t.Fatalf("Stage() error = %v, want ErrUploadRateExceeded", err)
	}
	if len(fake.puts) != 0 || len(repo.inserts) != 0 {
		t.Fatalf("rate-limited upload must write nothing: puts=%d inserts=%d", len(fake.puts), len(repo.inserts))
	}
	if !repo.gotSince.Equal(now.Add(-time.Hour)) {
		t.Fatalf("budget window: got since %v, want exactly %v", repo.gotSince, now.Add(-time.Hour))
	}
	if repo.gotUploader != uploader {
		t.Fatalf("budget counted uploader %s, want %s", repo.gotUploader, uploader)
	}
	if got := metricsLog.Summary().Uploads[metrics.ChatUploadRejected]; got != 1 {
		t.Fatalf("rate-limited upload outcome: got %d want 1 rejected", got)
	}

	// Nine staged uploads in the window: the tenth is still allowed.
	repo = &fakeUploadStore{recentCount: MaxUploadsPerRollingHour - 1}
	fake = &fakeObjectClient{}
	if _, err := newTestUploadService(repo, memberMembership(t, circle), fake, nil, now).
		Stage(context.Background(), groupStageInput(uploader, circle, MessageTypeVoice, oggMagic, 30)); err != nil {
		t.Fatalf("tenth staged upload in window must succeed, got %v", err)
	}
	if len(repo.inserts) != 1 {
		t.Fatalf("tenth staged upload must persist one row, got %d", len(repo.inserts))
	}
}

// --- Stage: success -------------------------------------------------------------------

// TestUploadService_StageSuccessGroupBindsCircle pins the group happy path:
// server-detected MIME, circle-bound staged row, chat/<id> object key, and a
// seven-day presigned preview URL whose expiry is derived from the pinned now.
func TestUploadService_StageSuccessGroupBindsCircle(t *testing.T) {
	uploader := uuid.New()
	circle := uuid.New()
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	fake := &fakeObjectClient{}
	repo := &fakeUploadStore{}
	metricsLog := &metrics.ChatMetrics{}
	in := groupStageInput(uploader, circle, MessageTypeVoice, oggMagic, 30)
	in.DeclaredFileName = `C:\tmp\my note.OGG`

	staged, err := newTestUploadService(repo, memberMembership(t, circle), fake, metricsLog, now).
		Stage(context.Background(), in)

	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if len(fake.puts) != 1 || fake.puts[0].object != "chat/"+staged.UploadID.String() {
		t.Fatalf("object key: got puts %v, want chat/<uploadID>", fake.puts)
	}
	if staged.ObjectKey != "chat/"+staged.UploadID.String() {
		t.Fatalf("response object key: got %q", staged.ObjectKey)
	}
	if len(repo.inserts) != 1 {
		t.Fatalf("staged rows: got %d want 1", len(repo.inserts))
	}
	row := repo.inserts[0]
	if row.UploaderID != uploader || row.AuthorizationCircleID != circle || row.DMPeerID != nil {
		t.Fatalf("staged binding: got uploader %s circle %s peer %v", row.UploaderID, row.AuthorizationCircleID, row.DMPeerID)
	}
	if row.MIMEType != "audio/ogg" {
		t.Fatalf("stored MIME must be server-detected, got %q", row.MIMEType)
	}
	if row.OriginalFileName != "my note.OGG" {
		t.Fatalf("stored filename: got %q want path-stripped original", row.OriginalFileName)
	}
	if row.SizeBytes != int64(len(oggMagic)) || row.DurationSeconds != 30 || row.State != UploadStateStaged {
		t.Fatalf("stored metadata: got size %d duration %d state %q", row.SizeBytes, row.DurationSeconds, row.State)
	}
	if len(fake.presigns) != 1 || fake.presigns[0].expires != MediaURLTTL || fake.presigns[0].object != staged.ObjectKey {
		t.Fatalf("preview presign: got %v, want 7-day URL for the staged key", fake.presigns)
	}
	if staged.URL == nil {
		t.Fatal("staged upload must return the presigned preview URL")
	}
	if !staged.URLExpiresAt.Equal(now.Add(MediaURLTTL)) {
		t.Fatalf("url expiry: got %v want %v", staged.URLExpiresAt, now.Add(MediaURLTTL))
	}
	if got := metricsLog.Summary().Uploads[metrics.ChatUploadAccepted]; got != 1 {
		t.Fatalf("upload outcome: got %d want 1 accepted", got)
	}
}

// TestUploadService_StageSuccessDMBindsPair proves a DM upload records the
// qualifying witness circle as its authorization while the object belongs to
// the pair conversation via dm_peer_id.
func TestUploadService_StageSuccessDMBindsPair(t *testing.T) {
	uploader := uuid.New()
	peer := uuid.New()
	witness := uuid.New()
	fake := &fakeObjectClient{}
	repo := &fakeUploadStore{witness: witness, eligible: true}
	in := StageUploadInput{
		UploaderID:       uploader,
		Target:           UploadTarget{DMPeerID: &peer},
		MediaType:        MessageTypeImage,
		Data:             jpegMagic,
		DeclaredFileName: "page.jpg",
	}

	staged, err := newTestUploadService(repo, memberMembership(t, uuid.New()), fake, nil, time.Now()).
		Stage(context.Background(), in)

	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if len(repo.inserts) != 1 {
		t.Fatalf("staged rows: got %d want 1", len(repo.inserts))
	}
	row := repo.inserts[0]
	if row.AuthorizationCircleID != witness {
		t.Fatalf("DM witness circle: got %s want %s", row.AuthorizationCircleID, witness)
	}
	if row.DMPeerID == nil || *row.DMPeerID != peer {
		t.Fatalf("DM peer binding: got %v want %s", row.DMPeerID, peer)
	}
	if row.MIMEType != "image/jpeg" || row.DurationSeconds != 0 {
		t.Fatalf("DM image metadata: got mime %q duration %d", row.MIMEType, row.DurationSeconds)
	}
	if staged.UploadID == uuid.Nil {
		t.Fatal("staged upload must be identified")
	}
}

// --- Stage: failure hygiene -------------------------------------------------------------

// TestUploadService_StageCleansUpObjectWhenRowInsertFails proves the 24-hour
// staged-cleanup seam is reused immediately: a database failure after a
// successful object put applies the revoking delete marker so the orphan never
// becomes attachable, and reports a storage failure.
func TestUploadService_StageCleansUpObjectWhenRowInsertFails(t *testing.T) {
	uploader := uuid.New()
	circle := uuid.New()
	dbFailure := errors.New("connection refused")
	fake := &fakeObjectClient{}
	repo := &fakeUploadStore{insertErr: dbFailure}
	metricsLog := &metrics.ChatMetrics{}

	_, err := newTestUploadService(repo, memberMembership(t, circle), fake, metricsLog, time.Now()).
		Stage(context.Background(), groupStageInput(uploader, circle, MessageTypeVoice, oggMagic, 30))

	if !errors.Is(err, dbFailure) {
		t.Fatalf("Stage() error = %v, want wrapped database failure", err)
	}
	if len(fake.puts) != 1 {
		t.Fatalf("object was staged before the database failure, got %d puts", len(fake.puts))
	}
	if len(fake.removes) != 1 || fake.removes[0].object != fake.puts[0].object {
		t.Fatalf("failed finalization must apply the delete marker to %q, got %v", fake.puts[0].object, fake.removes)
	}
	if got := metricsLog.Summary().Uploads[metrics.ChatUploadStorageFailure]; got != 1 {
		t.Fatalf("upload outcome: got %d want 1 storage failure", got)
	}
}

// TestUploadService_StagePresignFailureLeavesStagedUpload proves a preview
// presign failure after a successful stage leaves the object and row staged:
// the upload remains retryable and the 24-hour cleaner owns its fate, so no
// delete marker is applied here.
func TestUploadService_StagePresignFailureLeavesStagedUpload(t *testing.T) {
	uploader := uuid.New()
	circle := uuid.New()
	signFailure := errors.New("signing unavailable")
	fake := &fakeObjectClient{presignErr: signFailure}
	repo := &fakeUploadStore{}
	metricsLog := &metrics.ChatMetrics{}

	_, err := newTestUploadService(repo, memberMembership(t, circle), fake, metricsLog, time.Now()).
		Stage(context.Background(), groupStageInput(uploader, circle, MessageTypeVoice, oggMagic, 30))

	if !errors.Is(err, signFailure) {
		t.Fatalf("Stage() error = %v, want wrapped signing failure", err)
	}
	if len(repo.inserts) != 1 {
		t.Fatalf("upload must remain staged after a presign failure, got %d rows", len(repo.inserts))
	}
	if len(fake.removes) != 0 {
		t.Fatalf("presign failure must not delete the staged object, got %v", fake.removes)
	}
	if got := metricsLog.Summary().Uploads[metrics.ChatUploadStorageFailure]; got != 1 {
		t.Fatalf("upload outcome: got %d want 1 storage failure", got)
	}
}

// --- Attach reauthorization -------------------------------------------------------------

// TestCheckAttachableUpload pins the attach-once reauthorization matrix
// (edge case: an object cannot be attached by another user or reused across
// an unauthorized circle or DM context).
func TestCheckAttachableUpload(t *testing.T) {
	uploader := uuid.New()
	other := uuid.New()
	circle := uuid.New()
	uploadID := uuid.New()

	stagedGroupVoice := func() Upload {
		return Upload{
			ID:                    uploadID,
			UploaderID:            uploader,
			AuthorizationCircleID: circle,
			ObjectKey:             "chat/" + uploadID.String(),
			MIMEType:              "audio/ogg",
			State:                 UploadStateStaged,
		}
	}

	tests := []struct {
		name    string
		mutate  func(*Upload)
		sender  uuid.UUID
		msgType MessageType
		want    error
	}{
		{name: "matching staged group voice attaches", sender: uploader, msgType: MessageTypeVoice},
		{name: "foreign uploader is not attachable", sender: other, msgType: MessageTypeVoice, want: ErrUploadNotAttachable},
		{name: "attached upload is single-use", mutate: func(u *Upload) { u.State = UploadStateAttached }, sender: uploader, msgType: MessageTypeVoice, want: ErrUploadNotStaged},
		{name: "revoked upload is not attachable", mutate: func(u *Upload) { u.State = UploadStateRevoked }, sender: uploader, msgType: MessageTypeVoice, want: ErrUploadNotStaged},
		{name: "dm-bound upload is not attachable to a group", mutate: func(u *Upload) { u.DMPeerID = &other }, sender: uploader, msgType: MessageTypeVoice, want: ErrUploadNotAttachable},
		{name: "cross-circle reuse is not attachable", mutate: func(u *Upload) { u.AuthorizationCircleID = uuid.New() }, sender: uploader, msgType: MessageTypeVoice, want: ErrUploadNotAttachable},
		{name: "audio upload cannot attach as image", sender: uploader, msgType: MessageTypeImage, want: ErrUploadNotAttachable},
		{name: "audio upload cannot attach as file", sender: uploader, msgType: MessageTypeFile, want: ErrUploadNotAttachable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upload := stagedGroupVoice()
			if tt.mutate != nil {
				tt.mutate(&upload)
			}
			err := checkAttachableUpload(upload, tt.sender, circle, tt.msgType)
			if !errors.Is(err, tt.want) {
				t.Fatalf("checkAttachableUpload() error = %v, want %v", err, tt.want)
			}
		})
	}
}

// TestUploadService_SendGroupMediaPreTxValidation proves send-time
// reauthorization ordering: payload and key validation precede every
// persistence and authorization dependency (nil seams panic on misuse), and
// the active-member recheck and transaction failures are propagated.
func TestUploadService_SendGroupMediaPreTxValidation(t *testing.T) {
	sender := uuid.New()
	circle := uuid.New()
	uploadID := uuid.New()

	tests := []struct {
		name    string
		svc     *UploadService
		msgType MessageType
		key     string
		want    error
	}{
		{name: "missing idempotency key", svc: NewUploadService(nil, nil, nil, nil, nil, nil), msgType: MessageTypeVoice, key: "", want: ErrInvalidIdempotencyKey},
		{name: "text type cannot carry an upload", svc: NewUploadService(nil, nil, nil, nil, nil, nil), msgType: MessageTypeText, key: "k", want: ErrInvalidPayload},
		{name: "unknown type", svc: NewUploadService(nil, nil, nil, nil, nil, nil), msgType: MessageType("video"), key: "k", want: ErrInvalidPayload},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.svc.SendGroupMedia(context.Background(), SendGroupMediaInput{
				SenderID:       sender,
				CircleID:       circle,
				UploadID:       uploadID,
				MessageType:    tt.msgType,
				IdempotencyKey: tt.key,
			})
			if !errors.Is(err, tt.want) {
				t.Fatalf("SendGroupMedia() error = %v, want %v", err, tt.want)
			}
		})
	}

	t.Run("unknown circle is not visible", func(t *testing.T) {
		repo := &fakeUploadStore{}
		_, err := newTestUploadService(repo, &stubMembershipReader{circleErr: rbac.ErrCircleNotFound}, &fakeObjectClient{}, nil, time.Now()).
			SendGroupMedia(context.Background(), SendGroupMediaInput{
				SenderID:       sender,
				CircleID:       circle,
				UploadID:       uploadID,
				MessageType:    MessageTypeVoice,
				IdempotencyKey: "k",
			})
		if !errors.Is(err, ErrCircleNotVisible) {
			t.Fatalf("SendGroupMedia() error = %v, want ErrCircleNotVisible", err)
		}
	})

	t.Run("transaction failure is wrapped", func(t *testing.T) {
		txErr := errors.New("commit lost")
		repo := &fakeUploadStore{txErr: txErr}
		_, err := newTestUploadService(repo, memberMembership(t, circle), &fakeObjectClient{}, nil, time.Now()).
			SendGroupMedia(context.Background(), SendGroupMediaInput{
				SenderID:       sender,
				CircleID:       circle,
				UploadID:       uploadID,
				MessageType:    MessageTypeVoice,
				IdempotencyKey: "k",
			})
		if !errors.Is(err, txErr) || !strings.Contains(err.Error(), "send chat media message") {
			t.Fatalf("SendGroupMedia() error = %v, want wrapped commit failure", err)
		}
	})
}

// --- Media URL renewal ---------------------------------------------------------------------

// TestUploadService_RenewMediaURL pins the seven-day renewal authorization:
// only currently authorized viewers of an attached media message renew, group
// visibility follows the membership period (retained archived members
// included), DM renewal re-evaluates pair eligibility, and every denial is
// non-enumerating.
func TestUploadService_RenewMediaURL(t *testing.T) {
	viewer := uuid.New()
	sender := uuid.New()
	peer := uuid.New()
	circle := uuid.New()
	uploadID := uuid.New()
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	attachedUpload := Upload{ID: uploadID, ObjectKey: "chat/" + uploadID.String(), State: UploadStateAttached}

	groupVoiceMessage := func() Message {
		return Message{ID: uuid.New(), CircleID: &circle, SenderID: sender, Type: MessageTypeVoice, UploadID: &uploadID, State: MessageStateActive, SentAt: now}
	}
	dmImageMessage := func() Message {
		return Message{ID: uuid.New(), DMRecipientID: &peer, SenderID: sender, Type: MessageTypeImage, UploadID: &uploadID, State: MessageStateActive, SentAt: now.Add(-time.Minute)}
	}

	tests := []struct {
		name       string
		message    Message
		upload     Upload
		membership *stubMembershipReader
		repo       *fakeUploadStore
		want       error
	}{
		{
			name:   "missing message is not visible",
			upload: attachedUpload,
			want:   ErrMessageNotVisible,
		},
		{
			name:    "deleted message is not visible",
			message: func() Message { m := groupVoiceMessage(); m.State = MessageStateDeleted; return m }(),
			upload:  attachedUpload,
			want:    ErrMessageNotVisible,
		},
		{
			name:    "revoked upload is not visible",
			message: groupVoiceMessage(),
			upload:  Upload{ID: uploadID, State: UploadStateRevoked},
			want:    ErrMessageNotVisible,
		},
		{
			name:    "unattached upload is not visible",
			message: groupVoiceMessage(),
			upload:  Upload{ID: uploadID, State: UploadStateStaged},
			want:    ErrMessageNotVisible,
		},
		{
			name:       "group non-member cannot renew",
			message:    groupVoiceMessage(),
			upload:     attachedUpload,
			membership: &stubMembershipReader{circle: rbac.Circle{ID: circle.String()}},
			want:       ErrMessageNotVisible,
		},
		{
			name:       "message sent before joining is not visible",
			message:    groupVoiceMessage(),
			upload:     attachedUpload,
			membership: &stubMembershipReader{circle: rbac.Circle{ID: circle.String()}, member: true, joinedAt: now.Add(time.Minute)},
			want:       ErrMessageNotVisible,
		},
		{
			name:       "retained member of archived circle renews retained media",
			message:    groupVoiceMessage(),
			upload:     attachedUpload,
			membership: &stubMembershipReader{circle: rbac.Circle{ID: circle.String(), IsArchived: true}, member: true, joinedAt: now.Add(-time.Hour)},
		},
		{
			name:    "dm viewer outside the pair cannot renew",
			message: dmImageMessage(),
			upload:  attachedUpload,
			want:    ErrMessageNotVisible,
		},
		{
			name:    "dm pair without qualifying circle cannot renew",
			message: func() Message { m := dmImageMessage(); m.SenderID = viewer; return m }(),
			upload:  attachedUpload,
			repo:    &fakeUploadStore{},
			want:    ErrDMNotEligible,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeObjectClient{}
			repo := tt.repo
			if repo == nil {
				repo = &fakeUploadStore{}
			}
			// A zero-ID message models the missing message: the store never
			// finds it. Every other case loads its fixture.
			if tt.message.ID != uuid.Nil {
				repo.found = true
				repo.foundMessage = tt.message
				repo.foundUpload = tt.upload
			}
			membership := tt.membership
			if membership == nil {
				if tt.message.CircleID != nil {
					membership = &stubMembershipReader{circle: rbac.Circle{ID: circle.String()}, member: true, joinedAt: tt.message.SentAt.Add(-time.Hour)}
				} else {
					membership = memberMembership(t, uuid.New())
				}
			}

			access, err := newTestUploadService(repo, membership, fake, nil, now).
				RenewMediaURL(context.Background(), viewer, tt.message.ID)

			if tt.want != nil {
				if !errors.Is(err, tt.want) {
					t.Fatalf("RenewMediaURL() error = %v, want %v", err, tt.want)
				}
				if len(fake.presigns) != 0 {
					t.Fatalf("denied renewal must presign nothing, got %d", len(fake.presigns))
				}
				return
			}
			if err != nil {
				t.Fatalf("RenewMediaURL: %v", err)
			}
			if len(fake.presigns) != 1 || fake.presigns[0].expires != MediaURLTTL {
				t.Fatalf("renewal presign: got %v, want one 7-day URL", fake.presigns)
			}
			if access.URL == nil || !access.ExpiresAt.Equal(now.Add(MediaURLTTL)) {
				t.Fatalf("renewed access: got %+v, want URL expiring %v", access, now.Add(MediaURLTTL))
			}
		})
	}
}

// TestUploadService_RenewMediaURLDMSuccess proves an eligible pair member
// renews the pair's media.
func TestUploadService_RenewMediaURLDMSuccess(t *testing.T) {
	viewer := uuid.New()
	peer := uuid.New()
	uploadID := uuid.New()
	now := time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC)
	message := Message{ID: uuid.New(), DMRecipientID: &peer, SenderID: viewer, Type: MessageTypeFile, UploadID: &uploadID, State: MessageStateActive, SentAt: now.Add(-time.Minute)}
	upload := Upload{ID: uploadID, ObjectKey: "chat/" + uploadID.String(), State: UploadStateAttached}
	fake := &fakeObjectClient{}
	repo := &fakeUploadStore{found: true, foundMessage: message, foundUpload: upload, witness: uuid.New(), eligible: true}

	access, err := newTestUploadService(repo, memberMembership(t, uuid.New()), fake, nil, now).
		RenewMediaURL(context.Background(), viewer, message.ID)

	if err != nil {
		t.Fatalf("RenewMediaURL: %v", err)
	}
	if repo.gotDMUserA != viewer || repo.gotDMUserB != peer {
		t.Fatalf("eligibility checked (%s, %s), want (%s, %s)", repo.gotDMUserA, repo.gotDMUserB, viewer, peer)
	}
	if access.URL == nil || !access.ExpiresAt.Equal(now.Add(MediaURLTTL)) {
		t.Fatalf("renewed DM access: got %+v", access)
	}
}

// TestUploadService_StageReturnsURLOfPresigner proves the returned preview URL
// is exactly the store's presigned URL, never a server-composed one.
func TestUploadService_StageReturnsURLOfPresigner(t *testing.T) {
	circle := uuid.New()
	fake := &fakeObjectClient{}
	svc := newTestUploadService(&fakeUploadStore{}, memberMembership(t, circle), fake, nil, time.Now())

	staged, err := svc.Stage(context.Background(), groupStageInput(uuid.New(), circle, MessageTypeFile, pdfMagic, 0))
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	want := &url.URL{Scheme: "http", Host: "storage.test", Path: fmt.Sprintf("/halaqaty-chat/%s", staged.ObjectKey)}
	if *staged.URL != *want {
		t.Fatalf("preview URL: got %v want %v", staged.URL, want)
	}
}
