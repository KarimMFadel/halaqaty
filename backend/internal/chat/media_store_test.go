package chat

import (
	"context"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

const testOpTimeout = 5 * time.Second

// fakeObjectClient records every object-store call against the objectClient
// seam satisfied by *minio.Client.
type fakeObjectClient struct {
	exists        bool
	existsErr     error
	versioning    minio.BucketVersioningConfiguration
	versioningErr error
	putErr        error
	presignErr    error
	removeErrs    []error // consumed per RemoveObject call; exhausted means nil
	versions      []minio.ObjectInfo

	bucketCtx context.Context

	puts     []recordedPut
	presigns []recordedPresign
	removes  []recordedRemove
}

func (f *fakeObjectClient) ListObjects(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	out := make(chan minio.ObjectInfo, len(f.versions))
	for _, version := range f.versions {
		out <- version
	}
	close(out)
	return out
}

type recordedPut struct {
	ctx    context.Context
	bucket string
	object string
	size   int64
	opts   minio.PutObjectOptions
	body   string
}

type recordedPresign struct {
	ctx       context.Context
	bucket    string
	object    string
	expires   time.Duration
	reqParams url.Values
}

type recordedRemove struct {
	ctx    context.Context
	bucket string
	object string
	opts   minio.RemoveObjectOptions
}

func (f *fakeObjectClient) BucketExists(ctx context.Context, bucketName string) (bool, error) {
	f.bucketCtx = ctx
	return f.exists, f.existsErr
}

func (f *fakeObjectClient) GetBucketVersioning(ctx context.Context, bucketName string) (minio.BucketVersioningConfiguration, error) {
	_ = ctx
	return f.versioning, f.versioningErr
}

func (f *fakeObjectClient) PutObject(ctx context.Context, bucketName, objectName string, reader io.Reader, objectSize int64, opts minio.PutObjectOptions) (minio.UploadInfo, error) {
	raw, err := io.ReadAll(reader)
	if err != nil {
		return minio.UploadInfo{}, err
	}
	f.puts = append(f.puts, recordedPut{ctx: ctx, bucket: bucketName, object: objectName, size: objectSize, opts: opts, body: string(raw)})
	if f.putErr != nil {
		return minio.UploadInfo{}, f.putErr
	}
	return minio.UploadInfo{Bucket: bucketName, Key: objectName, Size: objectSize}, nil
}

func (f *fakeObjectClient) PresignedGetObject(ctx context.Context, bucketName, objectName string, expires time.Duration, reqParams url.Values) (*url.URL, error) {
	f.presigns = append(f.presigns, recordedPresign{ctx: ctx, bucket: bucketName, object: objectName, expires: expires, reqParams: reqParams})
	if f.presignErr != nil {
		return nil, f.presignErr
	}
	return &url.URL{Scheme: "http", Host: "minio.test", Path: "/" + bucketName + "/" + objectName}, nil
}

func (f *fakeObjectClient) RemoveObject(ctx context.Context, bucketName, objectName string, opts minio.RemoveObjectOptions) error {
	f.removes = append(f.removes, recordedRemove{ctx: ctx, bucket: bucketName, object: objectName, opts: opts})
	if len(f.removeErrs) == 0 {
		return nil
	}
	err := f.removeErrs[0]
	f.removeErrs = f.removeErrs[1:]
	return err
}

func newTestMediaStore(fake *fakeObjectClient) *MediaStore {
	return NewMediaStore(fake, "halaqaty-chat", testOpTimeout)
}

// --- Staging -----------------------------------------------------------------

func TestMediaStore_Stage_DerivesObjectKeyFromUploadIDOnly(t *testing.T) {
	fake := &fakeObjectClient{}
	uploadID := uuid.New()

	key, err := newTestMediaStore(fake).Stage(context.Background(), StageInput{
		UploadID:  uploadID,
		Type:      MessageTypeVoice,
		MIMEType:  "audio/ogg",
		SizeBytes: 1024,
		Body:      strings.NewReader("voice-bytes"),
	})

	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if key != "chat/"+uploadID.String() {
		t.Fatalf("object key: got %q want chat/<uploadID>", key)
	}
	if len(fake.puts) != 1 {
		t.Fatalf("puts: got %d want 1", len(fake.puts))
	}
	put := fake.puts[0]
	if put.object != key || put.bucket != "halaqaty-chat" {
		t.Fatalf("put target: got bucket %q object %q", put.bucket, put.object)
	}
	if put.size != 1024 || put.body != "voice-bytes" {
		t.Fatalf("put payload: got size %d body %q", put.size, put.body)
	}
	// The Stage API accepts no filename at all, so a malicious filename can
	// never reach the object path; metadata must also stay empty so no
	// filename or content rides along.
	if len(put.opts.UserMetadata) != 0 {
		t.Fatalf("put user metadata must be empty, got %v", put.opts.UserMetadata)
	}
}

func TestMediaStore_Stage_SanitizesContentTypeFromAllowlist(t *testing.T) {
	cases := []struct {
		name      string
		mediaType MessageType
		mimeType  string
		want      string
		wantErr   error
	}{
		{name: "voice padded uppercase", mediaType: MessageTypeVoice, mimeType: "  AUDIO/OGG ", want: "audio/ogg"},
		{name: "image mixed case", mediaType: MessageTypeImage, mimeType: "Image/PNG", want: "image/png"},
		{name: "pdf exact", mediaType: MessageTypeFile, mimeType: "application/pdf", want: "application/pdf"},
		{name: "mime allowed only for another type", mediaType: MessageTypeImage, mimeType: "audio/ogg", wantErr: ErrUnsupportedMIME},
		{name: "executable mime", mediaType: MessageTypeFile, mimeType: "application/x-msdownload", wantErr: ErrUnsupportedMIME},
		{name: "empty mime", mediaType: MessageTypeVoice, mimeType: "   ", wantErr: ErrUnsupportedMIME},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeObjectClient{}

			key, err := newTestMediaStore(fake).Stage(context.Background(), StageInput{
				UploadID:  uuid.New(),
				Type:      tc.mediaType,
				MIMEType:  tc.mimeType,
				SizeBytes: 16,
				Body:      strings.NewReader("payload"),
			})

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Stage error: got %v want %v", err, tc.wantErr)
				}
				if len(fake.puts) != 0 {
					t.Fatalf("rejected MIME must write no object, wrote %d", len(fake.puts))
				}
				return
			}
			if err != nil {
				t.Fatalf("Stage: %v", err)
			}
			if got := fake.puts[0].opts.ContentType; got != tc.want {
				t.Fatalf("content type: got %q want %q", got, tc.want)
			}
			if key == "" {
				t.Fatal("object key must be returned")
			}
		})
	}
}

func TestMediaStore_Stage_RejectsNonPositiveSize(t *testing.T) {
	fake := &fakeObjectClient{}

	_, err := newTestMediaStore(fake).Stage(context.Background(), StageInput{
		UploadID:  uuid.New(),
		Type:      MessageTypeImage,
		MIMEType:  "image/png",
		SizeBytes: 0,
		Body:      strings.NewReader(""),
	})

	if !errors.Is(err, ErrUploadTooLarge) {
		t.Fatalf("Stage error: got %v want ErrUploadTooLarge", err)
	}
	if len(fake.puts) != 0 {
		t.Fatalf("zero-size upload must write no object, wrote %d", len(fake.puts))
	}
}

func TestMediaStore_Stage_WrapsClientErrors(t *testing.T) {
	underlying := errors.New("object store unreachable")

	_, err := newTestMediaStore(&fakeObjectClient{putErr: underlying}).Stage(context.Background(), StageInput{
		UploadID:  uuid.New(),
		Type:      MessageTypeVoice,
		MIMEType:  "audio/mpeg",
		SizeBytes: 8,
		Body:      strings.NewReader("payload"),
	})

	if !errors.Is(err, underlying) {
		t.Fatalf("Stage error must wrap the client failure, got %v", err)
	}
}

// --- Presigning --------------------------------------------------------------

func TestMediaStore_PresignGet_IsVersionlessAndEchoesTTL(t *testing.T) {
	fake := &fakeObjectClient{}
	objectKey := "chat/" + uuid.NewString()

	signed, err := newTestMediaStore(fake).PresignGet(context.Background(), objectKey, 2*time.Hour)

	if err != nil {
		t.Fatalf("PresignGet: %v", err)
	}
	if len(fake.presigns) != 1 {
		t.Fatalf("presigns: got %d want 1", len(fake.presigns))
	}
	call := fake.presigns[0]
	if call.bucket != "halaqaty-chat" || call.object != objectKey {
		t.Fatalf("presign target: got bucket %q object %q", call.bucket, call.object)
	}
	if call.expires != 2*time.Hour {
		t.Fatalf("expires: got %v want 2h", call.expires)
	}
	// A versionId parameter would freeze the URL to one object version and
	// defeat delete-marker revocation; params must stay empty.
	if _, ok := call.reqParams["versionId"]; ok {
		t.Fatal("presign must never pin a versionId")
	}
	if len(call.reqParams) != 0 {
		t.Fatalf("presign params must be empty, got %v", call.reqParams)
	}
	if signed == nil || signed.Path != "/halaqaty-chat/"+objectKey {
		t.Fatalf("presigned URL must be the client's URL, got %v", signed)
	}
}

func TestMediaStore_PresignGet_WrapsClientErrors(t *testing.T) {
	underlying := errors.New("signing failed")

	_, err := newTestMediaStore(&fakeObjectClient{presignErr: underlying}).PresignGet(context.Background(), "chat/x", time.Hour)

	if !errors.Is(err, underlying) {
		t.Fatalf("PresignGet error must wrap the client failure, got %v", err)
	}
}

// --- Startup versioning enforcement -------------------------------------------

func TestMediaStore_EnsureChatBucketVersioned_FailFast(t *testing.T) {
	cases := []struct {
		name    string
		fake    *fakeObjectClient
		wantErr error
	}{
		{
			name:    "bucket missing",
			fake:    &fakeObjectClient{exists: false},
			wantErr: ErrMediaBucketMissing,
		},
		{
			name:    "versioning suspended",
			fake:    &fakeObjectClient{exists: true, versioning: minio.BucketVersioningConfiguration{Status: minio.Suspended}},
			wantErr: ErrMediaBucketNotVersioned,
		},
		{
			name:    "versioning unset",
			fake:    &fakeObjectClient{exists: true},
			wantErr: ErrMediaBucketNotVersioned,
		},
		{
			name: "versioning enabled",
			fake: &fakeObjectClient{exists: true, versioning: minio.BucketVersioningConfiguration{Status: minio.Enabled}},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := newTestMediaStore(tc.fake).EnsureChatBucketVersioned(context.Background())

			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("EnsureChatBucketVersioned: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error: got %v want %v", err, tc.wantErr)
			}
		})
	}
}

func TestMediaStore_EnsureChatBucketVersioned_WrapsClientErrors(t *testing.T) {
	cases := []struct {
		name   string
		fake   *fakeObjectClient
		wantIn string
	}{
		{
			name:   "bucket exists call fails",
			fake:   &fakeObjectClient{existsErr: errors.New("dial tcp: refused")},
			wantIn: "check chat media bucket",
		},
		{
			name:   "versioning read fails",
			fake:   &fakeObjectClient{exists: true, versioningErr: errors.New("read body")},
			wantIn: "read chat media bucket versioning",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := newTestMediaStore(tc.fake).EnsureChatBucketVersioned(context.Background())

			if err == nil {
				t.Fatal("error: got nil, want wrapped client failure")
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("error: got %q, want it to contain %q", err.Error(), tc.wantIn)
			}
		})
	}
}

// --- Timeout propagation -------------------------------------------------------

func TestMediaStore_ClientCallsReceiveDeadlineBearingContext(t *testing.T) {
	ops := []struct {
		name  string
		run   func(store *MediaStore) error
		ctxOf func(fake *fakeObjectClient) context.Context
	}{
		{
			name: "stage",
			run: func(store *MediaStore) error {
				_, err := store.Stage(context.Background(), StageInput{
					UploadID:  uuid.New(),
					Type:      MessageTypeFile,
					MIMEType:  "application/pdf",
					SizeBytes: 4,
					Body:      strings.NewReader("pdf"),
				})
				return err
			},
			ctxOf: func(fake *fakeObjectClient) context.Context { return fake.puts[0].ctx },
		},
		{
			name: "presign",
			run: func(store *MediaStore) error {
				_, err := store.PresignGet(context.Background(), "chat/x", time.Hour)
				return err
			},
			ctxOf: func(fake *fakeObjectClient) context.Context { return fake.presigns[0].ctx },
		},
		{
			name: "delete marker",
			run: func(store *MediaStore) error {
				return store.ApplyDeleteMarker(context.Background(), "chat/x")
			},
			ctxOf: func(fake *fakeObjectClient) context.Context { return fake.removes[0].ctx },
		},
		{
			name: "startup versioning check",
			run: func(store *MediaStore) error {
				return store.EnsureChatBucketVersioned(context.Background())
			},
			ctxOf: func(fake *fakeObjectClient) context.Context { return fake.ctxDeadlineRef() },
		},
	}
	for _, op := range ops {
		op := op
		t.Run(op.name, func(t *testing.T) {
			fake := &fakeObjectClient{exists: true, versioning: minio.BucketVersioningConfiguration{Status: minio.Enabled}}
			before := time.Now()

			if err := op.run(newTestMediaStore(fake)); err != nil {
				t.Fatalf("%s: %v", op.name, err)
			}

			deadline, ok := op.ctxOf(fake).Deadline()
			if !ok {
				t.Fatalf("%s must propagate a deadline-bearing context to the client", op.name)
			}
			if !deadline.After(before) || deadline.After(before.Add(testOpTimeout+2*time.Second)) {
				t.Fatalf("%s deadline: got %v, want ~%v after %v", op.name, deadline, testOpTimeout, before)
			}
		})
	}
}

// ctxDeadlineRef returns the deadline-bearing ctx captured by the versioning
// check (both BucketExists and GetBucketVersioning see the same derived ctx).
func (f *fakeObjectClient) ctxDeadlineRef() context.Context {
	return f.bucketCtx
}

// --- Delete markers ------------------------------------------------------------

func TestMediaStore_ApplyDeleteMarker_UsesVersionlessRemove(t *testing.T) {
	fake := &fakeObjectClient{}

	if err := newTestMediaStore(fake).ApplyDeleteMarker(context.Background(), "chat/abc"); err != nil {
		t.Fatalf("ApplyDeleteMarker: %v", err)
	}

	if len(fake.removes) != 1 {
		t.Fatalf("removes: got %d want 1", len(fake.removes))
	}
	remove := fake.removes[0]
	if remove.bucket != "halaqaty-chat" || remove.object != "chat/abc" {
		t.Fatalf("remove target: got bucket %q object %q", remove.bucket, remove.object)
	}
	if remove.opts.VersionID != "" {
		t.Fatalf("remove must be versionless so it writes a delete marker, got versionId %q", remove.opts.VersionID)
	}
}

func TestMediaStore_ApplyDeleteMarker_IsIdempotentWhenLatestVersionIsMarker(t *testing.T) {
	fake := &fakeObjectClient{versions: []minio.ObjectInfo{{
		Key: "chat/" + uuid.New().String(), IsDeleteMarker: true, VersionID: "marker-1", LastModified: time.Now(),
	}}}
	key := fake.versions[0].Key
	if err := newTestMediaStore(fake).ApplyDeleteMarker(context.Background(), key); err != nil {
		t.Fatalf("ApplyDeleteMarker: %v", err)
	}
	if len(fake.removes) != 0 {
		t.Fatalf("idempotent marker application issued %d remove calls", len(fake.removes))
	}
}

func TestMediaStore_RemoveDeleteMarker_RemovesExactInternalVersion(t *testing.T) {
	fake := &fakeObjectClient{}
	store := newTestMediaStore(fake)
	key := "chat/" + uuid.New().String()

	if err := store.RemoveDeleteMarker(context.Background(), key, "marker-2"); err != nil {
		t.Fatalf("RemoveDeleteMarker: %v", err)
	}
	if len(fake.removes) != 1 || fake.removes[0].opts.VersionID != "marker-2" {
		t.Fatalf("remove version=%q, want marker-2", fake.removes[0].opts.VersionID)
	}
}

func TestMediaStore_ApplyDeleteMarker_WrapsClientErrors(t *testing.T) {
	underlying := errors.New("object store down")

	err := newTestMediaStore(&fakeObjectClient{removeErrs: []error{underlying}}).ApplyDeleteMarker(context.Background(), "chat/abc")

	if !errors.Is(err, underlying) {
		t.Fatalf("ApplyDeleteMarker error must wrap the client failure, got %v", err)
	}
}

func TestMediaStore_RemoveLatestDeleteMarker_RejectsNonInternalObjectKey(t *testing.T) {
	fake := &fakeObjectClient{versions: []minio.ObjectInfo{{
		Key:            "chat/not-a-server-upload",
		IsDeleteMarker: true,
		LastModified:   time.Now(),
		VersionID:      "marker",
	}}}

	if err := newTestMediaStore(fake).RemoveLatestDeleteMarker(context.Background(), "chat/not-a-server-upload"); err == nil {
		t.Fatal("recovery must reject object keys that are not server-generated chat upload keys")
	}
	if len(fake.removes) != 0 {
		t.Fatal("recovery must not remove a marker for an unverified object key")
	}
}

// --- Staged cleanup ------------------------------------------------------------

type fakeStagedSource struct {
	rows      []Upload
	err       error
	gotCutoff time.Time
	gotLimit  int
	claims    int
}

func (s *fakeStagedSource) ClaimExpiredStaged(ctx context.Context, cutoff time.Time, limit int) ([]Upload, error) {
	_ = ctx
	s.claims++
	s.gotCutoff, s.gotLimit = cutoff, limit
	return s.rows, s.err
}

func (s *fakeStagedSource) ReleaseExpiredStaged(context.Context, uuid.UUID) error  { return nil }
func (s *fakeStagedSource) FinalizeExpiredStaged(context.Context, uuid.UUID) error { return nil }

func newTestCleaner(source StagedUploadSource, fake *fakeObjectClient, now time.Time) *Cleaner {
	cleaner := NewCleaner(source, newTestMediaStore(fake))
	cleaner.now = func() time.Time { return now }
	return cleaner
}

func stagedUpload(id uuid.UUID, key string) Upload {
	return Upload{ID: id, ObjectKey: key, State: UploadStateStaged}
}

func TestCleaner_CleanStaged_ClaimsWithExact24HourCutoff(t *testing.T) {
	source := &fakeStagedSource{}
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	cleaner := newTestCleaner(source, &fakeObjectClient{}, now)

	if err := cleaner.CleanStaged(context.Background(), 100); err != nil {
		t.Fatalf("CleanStaged: %v", err)
	}

	wantCutoff := now.Add(-24 * time.Hour)
	if !source.gotCutoff.Equal(wantCutoff) {
		t.Fatalf("cutoff: got %v want exactly %v", source.gotCutoff, wantCutoff)
	}
	if source.gotLimit != 100 {
		t.Fatalf("limit: got %d want 100", source.gotLimit)
	}
}

func TestCleaner_CleanStaged_DeletesEveryClaimedRow(t *testing.T) {
	fake := &fakeObjectClient{}
	// Claimed rows come back post-transition ('revoked'); the claim query is
	// the selection boundary, so every claimed object is deleted.
	source := &fakeStagedSource{rows: []Upload{
		{ID: uuid.New(), ObjectKey: "chat/older", State: UploadStateRevoked},
		{ID: uuid.New(), ObjectKey: "chat/old", State: UploadStateRevoked},
	}}
	cleaner := newTestCleaner(source, fake, time.Now())

	if err := cleaner.CleanStaged(context.Background(), 50); err != nil {
		t.Fatalf("CleanStaged: %v", err)
	}

	removed := make([]string, 0, len(fake.removes))
	for _, remove := range fake.removes {
		removed = append(removed, remove.object)
	}
	want := "chat/older,chat/old"
	if strings.Join(removed, ",") != want {
		t.Fatalf("deleted objects: got %v want %v", removed, want)
	}
}

func TestCleaner_CleanStaged_ObjectFailuresAreBestEffortAndReported(t *testing.T) {
	fake := &fakeObjectClient{removeErrs: []error{
		errors.New("object store down"),
		nil,
		errors.New("still down"),
	}}
	source := &fakeStagedSource{rows: []Upload{
		stagedUpload(uuid.New(), "chat/f1"),
		stagedUpload(uuid.New(), "chat/ok"),
		stagedUpload(uuid.New(), "chat/f2"),
	}}
	cleaner := newTestCleaner(source, fake, time.Now())

	err := cleaner.CleanStaged(context.Background(), 50)

	if err == nil {
		t.Fatal("batch must report the two object-store failures")
	}
	if len(fake.removes) != 3 {
		t.Fatalf("an object failure must not abort the batch; attempted %d of 3", len(fake.removes))
	}
	for _, key := range []string{"chat/f1", "chat/f2"} {
		if !strings.Contains(err.Error(), key) {
			t.Fatalf("error %q must report failed object %q", err.Error(), key)
		}
	}
	if strings.Contains(err.Error(), "chat/ok") {
		t.Fatalf("error %q must not report the successful deletion", err.Error())
	}
}

func TestCleaner_CleanStaged_SourceErrorIsWrapped(t *testing.T) {
	underlying := errors.New("db down")
	source := &fakeStagedSource{err: underlying}
	cleaner := newTestCleaner(source, &fakeObjectClient{}, time.Now())

	err := cleaner.CleanStaged(context.Background(), 50)

	if !errors.Is(err, underlying) {
		t.Fatalf("error must wrap the source failure, got %v", err)
	}
}

func TestCleaner_CleanStaged_NonPositiveLimitIsNoOp(t *testing.T) {
	source := &fakeStagedSource{}
	cleaner := newTestCleaner(source, &fakeObjectClient{}, time.Now())

	if err := cleaner.CleanStaged(context.Background(), 0); err != nil {
		t.Fatalf("CleanStaged(0): %v", err)
	}
	if source.claims != 0 {
		t.Fatal("zero limit must claim nothing")
	}
}

// --- Failed-finalization cleanup ------------------------------------------------

func TestCleaner_CleanupFailedFinalization(t *testing.T) {
	underlyingOutage := errors.New("temporary outage")
	cases := []struct {
		name      string
		upload    Upload
		removeErr error
		wantErr   error
	}{
		{
			// A missing object is not an error in S3 semantics: DELETE of a
			// nonexistent versionless key succeeds, so best-effort removal
			// stays silent.
			name:   "staged object removed; missing object is graceful",
			upload: stagedUpload(uuid.New(), "chat/doomed"),
		},
		{
			name:   "attached upload never touched",
			upload: Upload{ID: uuid.New(), ObjectKey: "chat/live", State: UploadStateAttached},
		},
		{
			name:   "revoked upload never touched",
			upload: Upload{ID: uuid.New(), ObjectKey: "chat/gone", State: UploadStateRevoked},
		},
		{
			name:      "object store failure is wrapped",
			upload:    stagedUpload(uuid.New(), "chat/flaky"),
			removeErr: underlyingOutage,
			wantErr:   underlyingOutage,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeObjectClient{removeErrs: []error{tc.removeErr}}
			cleaner := newTestCleaner(&fakeStagedSource{}, fake, time.Now())

			err := cleaner.CleanupFailedFinalization(context.Background(), tc.upload)

			wantRemoves := 1
			if tc.upload.State != UploadStateStaged {
				wantRemoves = 0
			}
			if len(fake.removes) != wantRemoves {
				t.Fatalf("removes: got %d want %d", len(fake.removes), wantRemoves)
			}
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("CleanupFailedFinalization: %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error: got %v want it to wrap %v", err, tc.wantErr)
			}
		})
	}
}
