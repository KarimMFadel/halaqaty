package chat

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"
)

const testOpTimeout = 5 * time.Second

// fakeObjectClient models the external storage boundary using only chat types.
type fakeObjectClient struct {
	putErr     error
	presignErr error
	removeErrs []error
	ensureErr  error
	recoverErr error
	puts       []recordedPut
	presigns   []recordedPresign
	removes    []recordedRemove
	recoveries []recordedRemove
	bucketCtx  context.Context
}
type recordedPut struct {
	ctx    context.Context
	object string
	size   int64
	mime   string
	body   string
}
type recordedPresign struct {
	ctx     context.Context
	object  string
	expires time.Duration
}
type recordedRemove struct {
	ctx    context.Context
	object string
}

func (f *fakeObjectClient) Put(ctx context.Context, in ObjectPutInput) error {
	raw, err := io.ReadAll(in.Body)
	if err != nil {
		return err
	}
	f.puts = append(f.puts, recordedPut{ctx: ctx, object: in.ObjectKey, size: in.SizeBytes, mime: in.MIMEType, body: string(raw)})
	return f.putErr
}
func (f *fakeObjectClient) PresignGet(ctx context.Context, key string, ttl time.Duration) (*url.URL, error) {
	f.presigns = append(f.presigns, recordedPresign{ctx: ctx, object: key, expires: ttl})
	if f.presignErr != nil {
		return nil, f.presignErr
	}
	return &url.URL{Scheme: "http", Host: "storage.test", Path: "/halaqaty-chat/" + key}, nil
}
func (f *fakeObjectClient) ApplyDeleteMarker(ctx context.Context, key string) error {
	f.removes = append(f.removes, recordedRemove{ctx: ctx, object: key})
	if len(f.removeErrs) == 0 {
		return nil
	}
	err := f.removeErrs[0]
	f.removeErrs = f.removeErrs[1:]
	return err
}
func (f *fakeObjectClient) RemoveLatestDeleteMarker(ctx context.Context, key string) error {
	f.recoveries = append(f.recoveries, recordedRemove{ctx: ctx, object: key})
	return f.recoverErr
}
func (f *fakeObjectClient) EnsureChatBucketVersioned(ctx context.Context) error {
	f.bucketCtx = ctx
	return f.ensureErr
}
func newTestMediaStore(fake *fakeObjectClient) *MediaStore { return NewMediaStore(fake, testOpTimeout) }

func TestMediaStore_Stage_DerivesObjectKeyFromUploadIDOnly(t *testing.T) {
	fake := &fakeObjectClient{}
	id := uuid.MustParse("03c79072-9eec-48e0-befa-25ec0a187a2a")
	key, err := newTestMediaStore(fake).Stage(context.Background(), StageInput{UploadID: id, Type: MessageTypeVoice, MIMEType: "audio/ogg", SizeBytes: 11, Body: strings.NewReader("voice-bytes")})
	if err != nil {
		t.Fatal(err)
	}
	if key != "chat/03c79072-9eec-48e0-befa-25ec0a187a2a" || len(fake.puts) != 1 {
		t.Fatalf("key=%q puts=%v", key, fake.puts)
	}
	put := fake.puts[0]
	if put.object != key || put.size != 11 || put.body != "voice-bytes" || put.mime != "audio/ogg" {
		t.Fatalf("put=%+v", put)
	}
}
func TestMediaStore_Stage_SanitizesContentTypeFromAllowlist(t *testing.T) {
	cases := []struct {
		name    string
		typ     MessageType
		mime    string
		want    string
		wantErr error
	}{
		{"voice padded uppercase", MessageTypeVoice, "  AUDIO/OGG ", "audio/ogg", nil},
		{"image mixed case", MessageTypeImage, "Image/PNG", "image/png", nil},
		{"pdf", MessageTypeFile, "application/pdf", "application/pdf", nil},
		{"wrong media type", MessageTypeImage, "audio/ogg", "", ErrUnsupportedMIME},
		{"executable", MessageTypeFile, "application/x-msdownload", "", ErrUnsupportedMIME},
		{"empty", MessageTypeVoice, " ", "", ErrUnsupportedMIME},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeObjectClient{}
			_, err := newTestMediaStore(fake).Stage(context.Background(), StageInput{UploadID: uuid.New(), Type: tc.typ, MIMEType: tc.mime, SizeBytes: 16, Body: strings.NewReader("payload")})
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err=%v want=%v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				if len(fake.puts) != 0 {
					t.Fatal("rejected MIME wrote an object")
				}
				return
			}
			if fake.puts[0].mime != tc.want {
				t.Fatalf("mime=%s want=%s", fake.puts[0].mime, tc.want)
			}
		})
	}
}
func TestMediaStore_Stage_RejectsNonPositiveSize(t *testing.T) {
	for _, size := range []int64{0, -1} {
		t.Run(string(rune(size+65)), func(t *testing.T) {
			fake := &fakeObjectClient{}
			_, err := newTestMediaStore(fake).Stage(context.Background(), StageInput{UploadID: uuid.New(), Type: MessageTypeImage, MIMEType: "image/png", SizeBytes: size, Body: strings.NewReader("")})
			if !errors.Is(err, ErrUploadTooLarge) || len(fake.puts) != 0 {
				t.Fatalf("err=%v puts=%v", err, fake.puts)
			}
		})
	}
}
func TestMediaStore_PresignGet_PreservesSignedURLAndTTL(t *testing.T) {
	fake := &fakeObjectClient{}
	key := "chat/" + uuid.NewString()
	signed, err := newTestMediaStore(fake).PresignGet(context.Background(), key, 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if signed == nil || signed.Path != "/halaqaty-chat/"+key || len(fake.presigns) != 1 || fake.presigns[0].object != key || fake.presigns[0].expires != 2*time.Hour {
		t.Fatalf("signed=%v calls=%v", signed, fake.presigns)
	}
}
func TestMediaStore_StorageFailuresRemainRecognizable(t *testing.T) {
	failure := errors.New("storage unavailable")
	cases := []struct {
		name string
		run  func(*MediaStore) error
	}{
		{"stage", func(s *MediaStore) error {
			_, err := s.Stage(context.Background(), StageInput{UploadID: uuid.New(), Type: MessageTypeVoice, MIMEType: "audio/ogg", SizeBytes: 1, Body: strings.NewReader("a")})
			return err
		}},
		{"presign", func(s *MediaStore) error {
			_, err := s.PresignGet(context.Background(), "chat/x", time.Hour)
			return err
		}},
		{"marker", func(s *MediaStore) error { return s.ApplyDeleteMarker(context.Background(), "chat/x") }},
		{"recovery", func(s *MediaStore) error {
			return s.RemoveLatestDeleteMarker(context.Background(), "chat/"+uuid.NewString())
		}},
		{"startup", func(s *MediaStore) error { return s.EnsureChatBucketVersioned(context.Background()) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeObjectClient{putErr: failure, presignErr: failure, removeErrs: []error{failure}, recoverErr: failure, ensureErr: failure}
			if err := tc.run(newTestMediaStore(fake)); !errors.Is(err, failure) {
				t.Fatalf("err=%v must wrap failure", err)
			}
		})
	}
}
func TestMediaStore_ClientCallsReceiveDeadlineBearingContext(t *testing.T) {
	cases := []struct {
		name  string
		run   func(*MediaStore) error
		ctxOf func(*fakeObjectClient) context.Context
	}{
		{"stage", func(s *MediaStore) error {
			_, err := s.Stage(context.Background(), StageInput{UploadID: uuid.New(), Type: MessageTypeFile, MIMEType: "application/pdf", SizeBytes: 3, Body: strings.NewReader("pdf")})
			return err
		}, func(f *fakeObjectClient) context.Context { return f.puts[0].ctx }},
		{"presign", func(s *MediaStore) error {
			_, err := s.PresignGet(context.Background(), "chat/x", time.Hour)
			return err
		}, func(f *fakeObjectClient) context.Context { return f.presigns[0].ctx }},
		{"marker", func(s *MediaStore) error { return s.ApplyDeleteMarker(context.Background(), "chat/x") }, func(f *fakeObjectClient) context.Context { return f.removes[0].ctx }},
		{"recovery", func(s *MediaStore) error {
			return s.RemoveLatestDeleteMarker(context.Background(), "chat/"+uuid.NewString())
		}, func(f *fakeObjectClient) context.Context { return f.recoveries[0].ctx }},
		{"startup", func(s *MediaStore) error { return s.EnsureChatBucketVersioned(context.Background()) }, func(f *fakeObjectClient) context.Context { return f.bucketCtx }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeObjectClient{}
			before := time.Now()
			if err := tc.run(newTestMediaStore(fake)); err != nil {
				t.Fatal(err)
			}
			deadline, ok := tc.ctxOf(fake).Deadline()
			if !ok || !deadline.After(before) || deadline.After(before.Add(testOpTimeout+time.Second)) {
				t.Fatalf("deadline=%v present=%t", deadline, ok)
			}
		})
	}
}
func TestMediaStore_RemoveLatestDeleteMarker_RejectsNonInternalObjectKey(t *testing.T) {
	for _, key := range []string{"chat/not-a-server-upload", "other/" + uuid.NewString(), ""} {
		t.Run(key, func(t *testing.T) {
			fake := &fakeObjectClient{}
			if err := newTestMediaStore(fake).RemoveLatestDeleteMarker(context.Background(), key); err == nil {
				t.Fatal("recovery must reject noninternal object key")
			}
			if len(fake.recoveries) != 0 {
				t.Fatal("invalid key reached storage recovery")
			}
		})
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
