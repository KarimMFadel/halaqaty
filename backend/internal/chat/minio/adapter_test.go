package minio

import (
	"context"
	"errors"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"
)

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

func newTestAdapter(fake *fakeObjectClient) *Adapter {
	return &Adapter{client: fake, bucket: "halaqaty-chat"}
}

// --- Presigning --------------------------------------------------------------

func TestAdapter_PresignGet_IsVersionlessAndEchoesTTL(t *testing.T) {
	fake := &fakeObjectClient{}
	objectKey := "chat/" + uuid.NewString()

	signed, err := newTestAdapter(fake).PresignGet(context.Background(), objectKey, 2*time.Hour)

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

func TestAdapter_PresignGet_WrapsClientErrors(t *testing.T) {
	underlying := errors.New("signing failed")

	_, err := newTestAdapter(&fakeObjectClient{presignErr: underlying}).PresignGet(context.Background(), "chat/x", time.Hour)

	if !errors.Is(err, underlying) {
		t.Fatalf("PresignGet error must wrap the client failure, got %v", err)
	}
}

// --- Startup versioning enforcement -------------------------------------------

func TestAdapter_EnsureChatBucketVersioned_FailFast(t *testing.T) {
	cases := []struct {
		name    string
		fake    *fakeObjectClient
		wantErr error
	}{
		{
			name:    "bucket missing",
			fake:    &fakeObjectClient{exists: false},
			wantErr: chat.ErrMediaBucketMissing,
		},
		{
			name:    "versioning suspended",
			fake:    &fakeObjectClient{exists: true, versioning: minio.BucketVersioningConfiguration{Status: minio.Suspended}},
			wantErr: chat.ErrMediaBucketNotVersioned,
		},
		{
			name:    "versioning unset",
			fake:    &fakeObjectClient{exists: true},
			wantErr: chat.ErrMediaBucketNotVersioned,
		},
		{
			name: "versioning enabled",
			fake: &fakeObjectClient{exists: true, versioning: minio.BucketVersioningConfiguration{Status: minio.Enabled}},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := newTestAdapter(tc.fake).EnsureChatBucketVersioned(context.Background())

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

func TestAdapter_EnsureChatBucketVersioned_WrapsClientErrors(t *testing.T) {
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
			err := newTestAdapter(tc.fake).EnsureChatBucketVersioned(context.Background())

			if err == nil {
				t.Fatal("error: got nil, want wrapped client failure")
			}
			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("error: got %q, want it to contain %q", err.Error(), tc.wantIn)
			}
		})
	}
}

// --- Delete markers ------------------------------------------------------------

func TestAdapter_ApplyDeleteMarker_UsesVersionlessRemove(t *testing.T) {
	fake := &fakeObjectClient{}

	if err := newTestAdapter(fake).ApplyDeleteMarker(context.Background(), "chat/abc"); err != nil {
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

func TestAdapter_ApplyDeleteMarker_IsIdempotentWhenLatestVersionIsMarker(t *testing.T) {
	fake := &fakeObjectClient{versions: []minio.ObjectInfo{{
		Key: "chat/" + uuid.New().String(), IsDeleteMarker: true, VersionID: "marker-1", LastModified: time.Now(),
	}}}
	key := fake.versions[0].Key
	if err := newTestAdapter(fake).ApplyDeleteMarker(context.Background(), key); err != nil {
		t.Fatalf("ApplyDeleteMarker: %v", err)
	}
	if len(fake.removes) != 0 {
		t.Fatalf("idempotent marker application issued %d remove calls", len(fake.removes))
	}
}

func TestAdapter_RemoveDeleteMarker_RemovesExactInternalVersion(t *testing.T) {
	fake := &fakeObjectClient{}
	store := newTestAdapter(fake)
	key := "chat/" + uuid.New().String()

	if err := store.RemoveDeleteMarker(context.Background(), key, "marker-2"); err != nil {
		t.Fatalf("RemoveDeleteMarker: %v", err)
	}
	if len(fake.removes) != 1 || fake.removes[0].opts.VersionID != "marker-2" {
		t.Fatalf("remove version=%q, want marker-2", fake.removes[0].opts.VersionID)
	}
}

func TestAdapter_ApplyDeleteMarker_WrapsClientErrors(t *testing.T) {
	underlying := errors.New("object store down")

	err := newTestAdapter(&fakeObjectClient{removeErrs: []error{underlying}}).ApplyDeleteMarker(context.Background(), "chat/abc")

	if !errors.Is(err, underlying) {
		t.Fatalf("ApplyDeleteMarker error must wrap the client failure, got %v", err)
	}
}

func TestAdapter_RemoveLatestDeleteMarker_RecoversExactLatestMatchingMarker(t *testing.T) {
	key := "chat/" + uuid.NewString()
	now := time.Now()
	cases := []struct {
		name        string
		versions    []minio.ObjectInfo
		removeErr   error
		wantVersion string
		wantErr     error
	}{
		{name: "absent"},
		{name: "bytes only", versions: []minio.ObjectInfo{{Key: key, VersionID: "bytes", LastModified: now}}},
		{name: "latest matching marker", versions: []minio.ObjectInfo{
			{Key: key, VersionID: "old-marker", IsDeleteMarker: true, LastModified: now.Add(-time.Hour)},
			{Key: key + "/other", VersionID: "other-marker", IsDeleteMarker: true, LastModified: now.Add(time.Hour)},
			{Key: key, VersionID: "bytes", LastModified: now.Add(time.Minute)},
			{Key: key, VersionID: "latest-marker", IsDeleteMarker: true, LastModified: now},
		}, wantVersion: "latest-marker"},
		{name: "listing fails", versions: []minio.ObjectInfo{{Err: errListing}}, wantErr: errListing},
		{name: "removal fails", versions: []minio.ObjectInfo{{Key: key, VersionID: "marker", IsDeleteMarker: true}}, removeErr: errRemoval, wantVersion: "marker", wantErr: errRemoval},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeObjectClient{versions: tc.versions, removeErrs: []error{tc.removeErr}}
			err := newTestAdapter(fake).RemoveLatestDeleteMarker(context.Background(), key)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error=%v want=%v", err, tc.wantErr)
			}
			if tc.wantVersion == "" {
				if len(fake.removes) != 0 {
					t.Fatal("absence/list failure must retain every version")
				}
				return
			}
			if len(fake.removes) != 1 || fake.removes[0].opts.VersionID != tc.wantVersion {
				t.Fatalf("removals=%v want marker %s", fake.removes, tc.wantVersion)
			}
		})
	}
}

var errListing = errors.New("version listing failed")
var errRemoval = errors.New("marker removal failed")

func TestAdapter_Put_PreservesPayloadAndSanitizedMetadata(t *testing.T) {
	fake := &fakeObjectClient{}
	err := newTestAdapter(fake).Put(context.Background(), chat.ObjectPutInput{ObjectKey: "chat/server-id", MIMEType: "audio/ogg", SizeBytes: 11, Body: strings.NewReader("voice-bytes")})
	if err != nil {
		t.Fatal(err)
	}
	put := fake.puts[0]
	if put.bucket != "halaqaty-chat" || put.object != "chat/server-id" || put.size != 11 || put.body != "voice-bytes" || put.opts.ContentType != "audio/ogg" || len(put.opts.UserMetadata) != 0 {
		t.Fatalf("put=%+v", put)
	}
}
