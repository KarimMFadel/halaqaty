//go:build contract

package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/api"
	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// T045 — behavioral REST contract tests for the F-004 US3 chat upload and
// media-renewal surface, pinned to specs/004-real-time-chat/contracts/
// chat.openapi.yaml (uploadVoice, uploadImage, uploadFile,
// renewMessageMediaUrl) and the canonical docs/contracts/openapi.yaml copies
// of those operations.
//
// The suite runs the production chat.UploadHandler and chat.MediaHandler
// through the production api.Router with the real auth middleware, so route
// registration, middleware order, the route-specific 21 MB body allowance,
// the 60-second upload timeout, status codes, error envelopes, non-enumerating
// denials, sanitized filenames, and response safety are proven without
// PostgreSQL or MinIO. The service behaviors behind the seam (magic-byte
// detection, budget accounting, renewal reauthorization) are proven against
// real infrastructure in backend/internal/chat/upload_service_test.go and
// backend/tests/integration/chat_media_test.go; this file pins only what the
// wire contract declares.

const (
	chatMediaVoicePath = "/api/v1/uploads/voice"
	chatMediaImagePath = "/api/v1/uploads/image"
	chatMediaFilePath  = "/api/v1/uploads/file"

	chatMediaCircleID = "44444444-4444-4444-4444-444444444444"
	chatMediaDMPeerID = "55555555-5555-5555-5555-555555555555"
)

// Media payloads prefixed with the magic bytes the server detects; the stub
// seam does not re-validate, but realistic prefixes keep the fixtures honest.
var (
	chatVoiceMagic = append([]byte("OggS"), bytes.Repeat([]byte{0x00}, 1024)...)
	chatImageMagic = append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, bytes.Repeat([]byte{0x00}, 1024)...)
	chatFileMagic  = append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{0x25}, 1024)...)
)

// chatUploadStageCall records one Stage call reaching the upload stub.
type chatUploadStageCall struct {
	input chat.StageUploadInput
}

// chatUploadServiceStub is a deterministic chat.UploadStagingService.
type chatUploadServiceStub struct {
	stageCalls []chatUploadStageCall
	stageErr   error
	delay      time.Duration
	result     chat.StagedUpload
}

func (s *chatUploadServiceStub) Stage(ctx context.Context, in chat.StageUploadInput) (chat.StagedUpload, error) {
	s.stageCalls = append(s.stageCalls, chatUploadStageCall{input: in})
	if s.delay > 0 {
		timer := time.NewTimer(s.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return chat.StagedUpload{}, ctx.Err()
		}
	}
	if s.stageErr != nil {
		return chat.StagedUpload{}, s.stageErr
	}
	if s.result.UploadID != uuid.Nil {
		return s.result, nil
	}
	return chatUploadStagedFixture(), nil
}

// chatUploadStagedFixture is one accepted staged upload.
func chatUploadStagedFixture() chat.StagedUpload {
	uploadID := uuid.MustParse("aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa")
	return chat.StagedUpload{
		UploadID:     uploadID,
		ObjectKey:    "chat/" + uploadID.String(),
		URL:          &url.URL{Scheme: "https", Host: "media.example.test", Path: "/halaqaty-chat/" + uploadID.String()},
		URLExpiresAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// chatMediaRenewCall records one RenewMediaURL call reaching the media stub.
type chatMediaRenewCall struct {
	viewerID  uuid.UUID
	messageID uuid.UUID
}

// chatMediaServiceStub is a deterministic chat.MediaRenewalService.
type chatMediaServiceStub struct {
	renewCalls []chatMediaRenewCall
	renewErr   error
	delay      time.Duration
	result     chat.MediaAccess
}

func (s *chatMediaServiceStub) RenewMediaURL(ctx context.Context, viewerID, messageID uuid.UUID) (chat.MediaAccess, error) {
	s.renewCalls = append(s.renewCalls, chatMediaRenewCall{viewerID: viewerID, messageID: messageID})
	if s.delay > 0 {
		timer := time.NewTimer(s.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			return chat.MediaAccess{}, ctx.Err()
		}
	}
	if s.renewErr != nil {
		return chat.MediaAccess{}, s.renewErr
	}
	if s.result.URL != nil {
		return s.result, nil
	}
	return chat.MediaAccess{
		URL:       &url.URL{Scheme: "https", Host: "media.example.test", Path: "/halaqaty-chat/renewed"},
		ExpiresAt: time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC),
	}, nil
}

// chatMediaRouter wires the production router with the real auth middleware
// (alwaysOKVerifier + the shared session stub authenticate as
// testLocalUserID) and the handlers under test, so contract cases exercise
// the exact deployed middleware order including the upload body-limit and
// timeout chain.
func chatMediaRouter(upload chat.UploadStagingService, media chat.MediaRenewalService, configure ...func(*api.MiddlewareSet)) http.Handler {
	authMW := middleware.NewAuthMiddleware(
		&alwaysOKVerifier{},
		auth.NewSessionService(30*24*time.Hour),
		&stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID},
	)
	mw := api.MiddlewareSet{
		Auth:              authMW,
		ChatUploadHandler: chat.NewUploadHandler(upload),
		ChatMediaHandler:  chat.NewMediaHandler(media),
		// The group send handler registers only so the standard-chain body
		// cap can be probed on a JSON route.
		ChatHandler: chat.NewGroupHandler(&chatGroupServiceStub{}),
	}
	for _, apply := range configure {
		apply(&mw)
	}
	return api.NewRouter(mw).Handler()
}

// chatMultipartBody builds one authenticated multipart upload request. A nil
// data with empty fileName omits the file part entirely.
func chatMultipartRequest(path string, fields map[string]string, fileName string, data []byte) *http.Request {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			panic(fmt.Sprintf("write field %q: %v", key, err))
		}
	}
	if fileName != "" {
		part, err := writer.CreateFormFile(httpconst.FieldFile, fileName)
		if err != nil {
			panic(fmt.Sprintf("create file field: %v", err))
		}
		if _, err := part.Write(data); err != nil {
			panic(fmt.Sprintf("write file field: %v", err))
		}
	}
	if err := writer.Close(); err != nil {
		panic(fmt.Sprintf("close multipart writer: %v", err))
	}

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set(httpconst.HeaderContentType, writer.FormDataContentType())
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	return req
}

// chatMediaRenewalRequest builds one authenticated media renewal request.
func chatMediaRenewalRequest(messageID string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/messages/"+messageID+"/media-url", nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	return req
}

func TestChatUploadContract(t *testing.T) {
	t.Parallel()

	t.Run("voice upload returns the documented UploadResponse", func(t *testing.T) {
		t.Parallel()
		stub := &chatUploadServiceStub{}
		rec := httptest.NewRecorder()
		req := chatMultipartRequest(chatMediaVoicePath, map[string]string{
			"circle_id":        chatMediaCircleID,
			"duration_seconds": "30",
		}, "recitation.ogg", chatVoiceMagic)
		chatMediaRouter(stub, nil).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		assertJSONKeySet(t, rec.Body.Bytes(), []string{"object_key", "url", "upload_id", "url_expires_at"})
		fixture := chatUploadStagedFixture()
		var response struct {
			ObjectKey    string `json:"object_key"`
			URL          string `json:"url"`
			UploadID     string `json:"upload_id"`
			URLExpiresAt string `json:"url_expires_at"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
		}
		if response.ObjectKey != fixture.ObjectKey {
			t.Fatalf("object_key: got %q, want the staged key %q", response.ObjectKey, fixture.ObjectKey)
		}
		if response.URL != fixture.URL.String() {
			t.Fatalf("url: got %q, want %q", response.URL, fixture.URL.String())
		}
		if response.UploadID != fixture.UploadID.String() {
			t.Fatalf("upload_id: got %q, want %q", response.UploadID, fixture.UploadID.String())
		}
		if _, err := time.Parse(time.RFC3339Nano, response.URLExpiresAt); err != nil {
			t.Fatalf("url_expires_at: %v", err)
		}

		if len(stub.stageCalls) != 1 {
			t.Fatalf("stage calls: got %d, want 1", len(stub.stageCalls))
		}
		call := stub.stageCalls[0].input
		if call.UploaderID.String() != testLocalUserID {
			t.Fatalf("uploader: got %s, want the authenticated principal %s", call.UploaderID, testLocalUserID)
		}
		if call.Target.CircleID == nil || call.Target.CircleID.String() != chatMediaCircleID || call.Target.DMPeerID != nil {
			t.Fatalf("target: got %+v, want circle %s", call.Target, chatMediaCircleID)
		}
		if call.MediaType != chat.MessageTypeVoice {
			t.Fatalf("media type: got %q, want voice", call.MediaType)
		}
		if !bytes.Equal(call.Data, chatVoiceMagic) {
			t.Fatalf("data: got %d bytes, want the uploaded %d bytes", len(call.Data), len(chatVoiceMagic))
		}
		if call.DeclaredFileName != "recitation.ogg" {
			t.Fatalf("declared filename: got %q, want the multipart filename (sanitization is service-side)", call.DeclaredFileName)
		}
		if call.DurationSeconds != 30 {
			t.Fatalf("duration: got %d, want 30", call.DurationSeconds)
		}
	})

	t.Run("image upload accepts a dm peer target", func(t *testing.T) {
		t.Parallel()
		stub := &chatUploadServiceStub{}
		rec := httptest.NewRecorder()
		chatMediaRouter(stub, nil).ServeHTTP(rec, chatMultipartRequest(chatMediaImagePath, map[string]string{
			"dm_peer_id": chatMediaDMPeerID,
		}, "photo.jpg", chatImageMagic))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		call := stub.stageCalls[0].input
		if call.MediaType != chat.MessageTypeImage {
			t.Fatalf("media type: got %q, want image", call.MediaType)
		}
		if call.Target.DMPeerID == nil || call.Target.DMPeerID.String() != chatMediaDMPeerID || call.Target.CircleID != nil {
			t.Fatalf("target: got %+v, want dm peer %s", call.Target, chatMediaDMPeerID)
		}
		if call.DurationSeconds != 0 {
			t.Fatalf("duration: got %d, want 0 for a non-voice upload", call.DurationSeconds)
		}
	})

	t.Run("file upload returns the documented UploadResponse", func(t *testing.T) {
		t.Parallel()
		stub := &chatUploadServiceStub{}
		rec := httptest.NewRecorder()
		chatMediaRouter(stub, nil).ServeHTTP(rec, chatMultipartRequest(chatMediaFilePath, map[string]string{
			"circle_id": chatMediaCircleID,
		}, "lesson.pdf", chatFileMagic))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if stub.stageCalls[0].input.MediaType != chat.MessageTypeFile {
			t.Fatalf("media type: got %q, want file", stub.stageCalls[0].input.MediaType)
		}
	})

	cases := []struct {
		name        string
		path        string
		fields      map[string]string
		fileName    string
		data        []byte
		noBearer    bool
		noSession   bool
		stubErr     error
		wantStatus  int
		wantCode    string
		wantMessage string
		wantField   string
		notContains string
	}{
		{
			name:       "missing bearer returns 401",
			path:       chatMediaVoicePath,
			fields:     map[string]string{"circle_id": chatMediaCircleID, "duration_seconds": "30"},
			fileName:   "recitation.ogg",
			data:       chatVoiceMagic,
			noBearer:   true,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeUnauthorized,
		},
		{
			name:       "missing session returns 401",
			path:       chatMediaVoicePath,
			fields:     map[string]string{"circle_id": chatMediaCircleID, "duration_seconds": "30"},
			fileName:   "recitation.ogg",
			data:       chatVoiceMagic,
			noSession:  true,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionMissing,
		},
		{
			name:        "unsupported detected media returns 415",
			path:        chatMediaVoicePath,
			fields:      map[string]string{"circle_id": chatMediaCircleID, "duration_seconds": "30"},
			fileName:    "notes.txt",
			data:        chatVoiceMagic,
			stubErr:     chat.ErrUnsupportedMIME,
			wantStatus:  http.StatusUnsupportedMediaType,
			wantCode:    httpconst.ErrorCodeUnsupportedMediaType,
			wantMessage: httpconst.ErrorMessageUnsupportedMediaType,
		},
		{
			name:        "oversized file returns 413",
			path:        chatMediaVoicePath,
			fields:      map[string]string{"circle_id": chatMediaCircleID, "duration_seconds": "30"},
			fileName:    "recitation.ogg",
			data:        chatVoiceMagic,
			stubErr:     chat.ErrUploadTooLarge,
			wantStatus:  http.StatusRequestEntityTooLarge,
			wantCode:    httpconst.ErrorCodeUploadTooLarge,
			wantMessage: httpconst.ErrorMessageUploadTooLarge,
		},
		{
			name:       "missing voice duration returns 422",
			path:       chatMediaVoicePath,
			fields:     map[string]string{"circle_id": chatMediaCircleID},
			fileName:   "recitation.ogg",
			data:       chatVoiceMagic,
			stubErr:    chat.ErrInvalidDuration,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldDurationSeconds,
		},
		{
			name:       "over-limit voice duration returns 422",
			path:       chatMediaVoicePath,
			fields:     map[string]string{"circle_id": chatMediaCircleID, "duration_seconds": "301"},
			fileName:   "recitation.ogg",
			data:       chatVoiceMagic,
			stubErr:    chat.ErrInvalidDuration,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldDurationSeconds,
		},
		{
			name:       "malformed duration returns 400",
			path:       chatMediaVoicePath,
			fields:     map[string]string{"circle_id": chatMediaCircleID, "duration_seconds": "soon"},
			fileName:   "recitation.ogg",
			data:       chatVoiceMagic,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldDurationSeconds,
		},
		{
			name:       "both targets rejected 422",
			path:       chatMediaVoicePath,
			fields:     map[string]string{"circle_id": chatMediaCircleID, "dm_peer_id": chatMediaDMPeerID, "duration_seconds": "30"},
			fileName:   "recitation.ogg",
			data:       chatVoiceMagic,
			stubErr:    chat.ErrInvalidContext,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldCircleID,
		},
		{
			name:       "no target rejected 422",
			path:       chatMediaVoicePath,
			fields:     map[string]string{"duration_seconds": "30"},
			fileName:   "recitation.ogg",
			data:       chatVoiceMagic,
			stubErr:    chat.ErrInvalidContext,
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldCircleID,
		},
		{
			name:       "invalid circle id returns 400",
			path:       chatMediaVoicePath,
			fields:     map[string]string{"circle_id": "not-a-uuid", "duration_seconds": "30"},
			fileName:   "recitation.ogg",
			data:       chatVoiceMagic,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldCircleID,
		},
		{
			name:       "invalid dm peer id returns 400",
			path:       chatMediaImagePath,
			fields:     map[string]string{"dm_peer_id": "not-a-uuid"},
			fileName:   "photo.jpg",
			data:       chatImageMagic,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldDMPeerID,
		},
		{
			name:       "missing file field returns 400",
			path:       chatMediaVoicePath,
			fields:     map[string]string{"circle_id": chatMediaCircleID, "duration_seconds": "30"},
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldFile,
		},
		{
			name:       "non-member and unknown target circle return 403",
			path:       chatMediaVoicePath,
			fields:     map[string]string{"circle_id": chatMediaCircleID, "duration_seconds": "30"},
			fileName:   "recitation.ogg",
			data:       chatVoiceMagic,
			stubErr:    chat.ErrCircleNotVisible,
			wantStatus: http.StatusForbidden,
			wantCode:   httpconst.ErrorCodeForbidden,
		},
		{
			name:       "ineligible dm pair returns 403",
			path:       chatMediaImagePath,
			fields:     map[string]string{"dm_peer_id": chatMediaDMPeerID},
			fileName:   "photo.jpg",
			data:       chatImageMagic,
			stubErr:    chat.ErrDMNotEligible,
			wantStatus: http.StatusForbidden,
			wantCode:   httpconst.ErrorCodeForbidden,
		},
		{
			name:        "exhausted upload budget returns 429",
			path:        chatMediaVoicePath,
			fields:      map[string]string{"circle_id": chatMediaCircleID, "duration_seconds": "30"},
			fileName:    "recitation.ogg",
			data:        chatVoiceMagic,
			stubErr:     chat.ErrUploadRateExceeded,
			wantStatus:  http.StatusTooManyRequests,
			wantCode:    httpconst.ErrorCodeRateLimitExceeded,
			wantMessage: httpconst.ErrorMessageRateLimitExceeded,
		},
		{
			name:        "storage failure returns 500 without internals",
			path:        chatMediaFilePath,
			fields:      map[string]string{"circle_id": chatMediaCircleID},
			fileName:    "lesson.pdf",
			data:        chatFileMagic,
			stubErr:     fmt.Errorf("persist chat upload: %w", errors.New("connection refused")),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    httpconst.ErrorCodeInternalServerError,
			notContains: "connection refused",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			stub := &chatUploadServiceStub{stageErr: tc.stubErr}
			req := chatMultipartRequest(tc.path, tc.fields, tc.fileName, tc.data)
			if tc.noBearer {
				req.Header.Del(httpconst.HeaderAuthorization)
			}
			if tc.noSession {
				req.Header.Del(httpconst.HeaderSessionID)
			}
			rec := httptest.NewRecorder()
			chatMediaRouter(stub, nil).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != tc.wantCode {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, tc.wantCode)
			}
			if tc.wantMessage != "" && envelope.Error.Message != tc.wantMessage {
				t.Fatalf("error message: got %q, want %q", envelope.Error.Message, tc.wantMessage)
			}
			if tc.wantField != "" {
				if _, ok := envelope.Error.Fields[tc.wantField]; !ok {
					t.Fatalf("expected field %q in Fields=%v", tc.wantField, envelope.Error.Fields)
				}
			}
			if tc.wantStatus == http.StatusForbidden && len(envelope.Error.Fields) != 0 {
				t.Fatalf("forbidden envelope must carry no field details: %v", envelope.Error.Fields)
			}
			if tc.notContains != "" && strings.Contains(rec.Body.String(), tc.notContains) {
				t.Fatalf("error body leaks internal detail %q: %s", tc.notContains, rec.Body.String())
			}
		})
	}

	t.Run("authorization denials are byte-identical across target kinds", func(t *testing.T) {
		t.Parallel()
		bodies := make([]string, 0, 2)
		for _, tc := range []struct {
			name    string
			fields  map[string]string
			stubErr error
		}{
			{"group target", map[string]string{"circle_id": chatMediaCircleID, "duration_seconds": "30"}, chat.ErrCircleNotVisible},
			{"dm target", map[string]string{"dm_peer_id": chatMediaDMPeerID}, chat.ErrDMNotEligible},
		} {
			stub := &chatUploadServiceStub{stageErr: tc.stubErr}
			rec := httptest.NewRecorder()
			chatMediaRouter(stub, nil).ServeHTTP(rec, chatMultipartRequest(chatMediaVoicePath, tc.fields, "recitation.ogg", chatVoiceMagic))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s: status: got %d, want %d body=%s", tc.name, rec.Code, http.StatusForbidden, rec.Body.String())
			}
			bodies = append(bodies, rec.Body.String())
		}
		if bodies[0] != bodies[1] {
			t.Fatalf("denial bodies differ, want byte-identical responses: %q vs %q", bodies[0], bodies[1])
		}
	})

	t.Run("hostile declared filename never reaches the upload response", func(t *testing.T) {
		t.Parallel()
		stub := &chatUploadServiceStub{}
		// Header-safe but hostile: path traversal fragments and an oversized
		// name. (A NUL byte would make the multipart header itself
		// unparseable, which the 400 malformed-request path already covers.)
		hostile := `..\..\etc\passwd\\\\` + strings.Repeat("x", 600)
		rec := httptest.NewRecorder()
		chatMediaRouter(stub, nil).ServeHTTP(rec, chatMultipartRequest(chatMediaFilePath, map[string]string{
			"circle_id": chatMediaCircleID,
		}, hostile, chatFileMagic))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		assertJSONKeySet(t, rec.Body.Bytes(), []string{"object_key", "url", "upload_id", "url_expires_at"})
		for _, marker := range []string{"passwd", `..\`, "xxxx"} {
			if strings.Contains(rec.Body.String(), marker) {
				t.Fatalf("upload body leaks filename fragment %q: %s", marker, rec.Body.String())
			}
		}
	})
}

func TestChatUploadBodyLimitContract(t *testing.T) {
	t.Parallel()

	t.Run("multipart body over 1 MiB is accepted on the image upload route", func(t *testing.T) {
		t.Parallel()
		stub := &chatUploadServiceStub{}
		oversize := append(append([]byte{}, chatImageMagic...), bytes.Repeat([]byte{0xAB}, (3<<20)-len(chatImageMagic))...)
		rec := httptest.NewRecorder()
		chatMediaRouter(stub, nil).ServeHTTP(rec, chatMultipartRequest(chatMediaImagePath, map[string]string{
			"circle_id": chatMediaCircleID,
		}, "photo.jpg", oversize))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (route-specific cap must exceed the 1 MiB default) body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if len(stub.stageCalls) != 1 {
			t.Fatalf("stage calls: got %d, want 1 (an in-limit body must reach the service)", len(stub.stageCalls))
		}
	})

	t.Run("multipart body over 21 MB is rejected 413 on the voice upload route", func(t *testing.T) {
		t.Parallel()
		stub := &chatUploadServiceStub{}
		over := append(append([]byte{}, chatVoiceMagic...), bytes.Repeat([]byte{0x00}, int(chat.ChatUploadMaxBodyBytes))...)
		rec := httptest.NewRecorder()
		chatMediaRouter(stub, nil).ServeHTTP(rec, chatMultipartRequest(chatMediaVoicePath, map[string]string{
			"circle_id":        chatMediaCircleID,
			"duration_seconds": "30",
		}, "recitation.ogg", over))

		if rec.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
		}
		envelope := decodeErrorEnvelope(t, rec)
		if envelope.Error.Code != httpconst.ErrorCodeUploadTooLarge {
			t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeUploadTooLarge)
		}
		if len(stub.stageCalls) != 0 {
			t.Fatalf("stage calls: got %d, want 0 (an over-cap body must not reach the service)", len(stub.stageCalls))
		}
	})

	t.Run("json routes keep the 1 MiB global cap", func(t *testing.T) {
		t.Parallel()
		router := chatMediaRouter(nil, nil)
		body := `{"message_type":"text","content":"` + strings.Repeat("a", 2<<20) + `"}`
		req := httptest.NewRequest(http.MethodPost, chatGroupMessagesPath(chatGroupCircleID), strings.NewReader(body))
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
		req.Header.Set(httpconst.HeaderSessionID, testSessionID)
		req.Header.Set(httpconst.HeaderIdempotencyKey, "body-limit-key")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d, want %d (over-cap JSON bodies stay rejected) body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
		envelope := decodeErrorEnvelope(t, rec)
		if envelope.Error.Code != httpconst.ErrorCodeValidationFailed {
			t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeValidationFailed)
		}
	})
}

func TestChatUploadTimeoutContract(t *testing.T) {
	t.Parallel()

	if api.DefaultChatUploadTimeout != 60*time.Second {
		t.Fatalf("default upload timeout: got %s, want 60s (plan §Phase 1: 60-second upload timeout)", api.DefaultChatUploadTimeout)
	}

	t.Run("slow upload exceeds its route timeout envelope", func(t *testing.T) {
		t.Parallel()
		stub := &chatUploadServiceStub{delay: 150 * time.Millisecond}
		router := chatMediaRouter(stub, nil, func(mw *api.MiddlewareSet) {
			mw.ChatUploadTimeout = 40 * time.Millisecond
		})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, chatMultipartRequest(chatMediaVoicePath, map[string]string{
			"circle_id":        chatMediaCircleID,
			"duration_seconds": "30",
		}, "recitation.ogg", chatVoiceMagic))

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
		}
		envelope := decodeErrorEnvelope(t, rec)
		if envelope.Error.Code != httpconst.ErrorCodeRequestTimeout {
			t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeRequestTimeout)
		}
	})

	t.Run("upload route is not governed by the shorter global timeout", func(t *testing.T) {
		t.Parallel()
		stub := &chatUploadServiceStub{delay: 100 * time.Millisecond}
		router := chatMediaRouter(stub, nil, func(mw *api.MiddlewareSet) {
			mw.Timeout = 30 * time.Millisecond
			mw.ChatUploadTimeout = 5 * time.Second
		})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, chatMultipartRequest(chatMediaFilePath, map[string]string{
			"circle_id": chatMediaCircleID,
		}, "lesson.pdf", chatFileMagic))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (uploads carry their own 60-second budget) body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		if len(stub.stageCalls) != 1 {
			t.Fatalf("stage calls: got %d, want 1", len(stub.stageCalls))
		}
	})

	t.Run("media renewal keeps the global timeout", func(t *testing.T) {
		t.Parallel()
		stub := &chatMediaServiceStub{delay: 150 * time.Millisecond}
		router := chatMediaRouter(nil, stub, func(mw *api.MiddlewareSet) {
			mw.Timeout = 40 * time.Millisecond
		})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, chatMediaRenewalRequest(uuid.New().String()))

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
		}
		envelope := decodeErrorEnvelope(t, rec)
		if envelope.Error.Code != httpconst.ErrorCodeRequestTimeout {
			t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeRequestTimeout)
		}
	})
}

func TestChatMediaRenewalContract(t *testing.T) {
	t.Parallel()

	t.Run("renewal returns the documented MediaAccess", func(t *testing.T) {
		t.Parallel()
		messageID := uuid.New()
		stub := &chatMediaServiceStub{}
		rec := httptest.NewRecorder()
		chatMediaRouter(nil, stub).ServeHTTP(rec, chatMediaRenewalRequest(messageID.String()))

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
		}
		assertJSONKeySet(t, rec.Body.Bytes(), []string{"url", "expires_at"})
		var response struct {
			URL       string `json:"url"`
			ExpiresAt string `json:"expires_at"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("decode response: %v body=%s", err, rec.Body.String())
		}
		if !strings.HasPrefix(response.URL, "https://") {
			t.Fatalf("url: got %q, want an absolute https URL", response.URL)
		}
		if _, err := time.Parse(time.RFC3339Nano, response.ExpiresAt); err != nil {
			t.Fatalf("expires_at: %v", err)
		}
		if len(stub.renewCalls) != 1 {
			t.Fatalf("renew calls: got %d, want 1", len(stub.renewCalls))
		}
		call := stub.renewCalls[0]
		if call.viewerID.String() != testLocalUserID {
			t.Fatalf("viewer: got %s, want the authenticated principal %s", call.viewerID, testLocalUserID)
		}
		if call.messageID != messageID {
			t.Fatalf("message id: got %s, want %s", call.messageID, messageID)
		}
	})

	t.Run("missing bearer returns 401", func(t *testing.T) {
		t.Parallel()
		req := chatMediaRenewalRequest(uuid.New().String())
		req.Header.Del(httpconst.HeaderAuthorization)
		rec := httptest.NewRecorder()
		chatMediaRouter(nil, &chatMediaServiceStub{}).ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		envelope := decodeErrorEnvelope(t, rec)
		if envelope.Error.Code != httpconst.ErrorCodeUnauthorized {
			t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeUnauthorized)
		}
	})

	t.Run("missing session returns 401 with the session code", func(t *testing.T) {
		t.Parallel()
		req := chatMediaRenewalRequest(uuid.New().String())
		req.Header.Del(httpconst.HeaderSessionID)
		rec := httptest.NewRecorder()
		chatMediaRouter(nil, &chatMediaServiceStub{}).ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
		}
		envelope := decodeErrorEnvelope(t, rec)
		if envelope.Error.Code != httpconst.ErrorCodeSessionMissing {
			t.Fatalf("error code: got %q, want %q (auth and session denials stay distinguishable)", envelope.Error.Code, httpconst.ErrorCodeSessionMissing)
		}
	})

	t.Run("invalid message id returns 400", func(t *testing.T) {
		t.Parallel()
		rec := httptest.NewRecorder()
		chatMediaRouter(nil, &chatMediaServiceStub{}).ServeHTTP(rec, chatMediaRenewalRequest("not-a-uuid"))

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
		}
		envelope := decodeErrorEnvelope(t, rec)
		if envelope.Error.Code != httpconst.ErrorCodeValidationFailed {
			t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeValidationFailed)
		}
		if _, ok := envelope.Error.Fields[httpconst.FieldMessageID]; !ok {
			t.Fatalf("expected field %q in Fields=%v", httpconst.FieldMessageID, envelope.Error.Fields)
		}
	})

	// Every renewal denial is the same non-enumerating 403: missing, deleted,
	// text, unattached, and unauthorized messages are indistinguishable, and
	// an eligibility-lost pair member learns nothing more.
	t.Run("renewal denials are non-enumerating", func(t *testing.T) {
		t.Parallel()
		denials := []struct {
			name    string
			stubErr error
		}{
			{"missing message", chat.ErrMessageNotVisible},
			{"deleted message", chat.ErrMessageNotVisible},
			{"text message", chat.ErrMessageNotVisible},
			{"unauthorized viewer", chat.ErrMessageNotVisible},
			{"eligibility-lost pair member", chat.ErrDMNotEligible},
		}
		bodies := make([]string, 0, len(denials))
		for _, denial := range denials {
			stub := &chatMediaServiceStub{renewErr: denial.stubErr}
			rec := httptest.NewRecorder()
			chatMediaRouter(nil, stub).ServeHTTP(rec, chatMediaRenewalRequest(uuid.New().String()))
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s: status: got %d, want %d body=%s", denial.name, rec.Code, http.StatusForbidden, rec.Body.String())
			}
			envelope := decodeErrorEnvelope(t, rec)
			if envelope.Error.Code != httpconst.ErrorCodeForbidden {
				t.Fatalf("%s: error code: got %q, want %q", denial.name, envelope.Error.Code, httpconst.ErrorCodeForbidden)
			}
			if len(envelope.Error.Fields) != 0 {
				t.Fatalf("%s: forbidden envelope must carry no field details: %v", denial.name, envelope.Error.Fields)
			}
			bodies = append(bodies, rec.Body.String())
		}
		for _, body := range bodies[1:] {
			if body != bodies[0] {
				t.Fatalf("denial bodies differ, want byte-identical responses: %q vs %q", bodies[0], body)
			}
		}
	})

	t.Run("infrastructure failure returns 500 without internals", func(t *testing.T) {
		t.Parallel()
		stub := &chatMediaServiceStub{renewErr: fmt.Errorf("presign chat media url: %w", errors.New("connection refused"))}
		rec := httptest.NewRecorder()
		chatMediaRouter(nil, stub).ServeHTTP(rec, chatMediaRenewalRequest(uuid.New().String()))

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status: got %d, want %d", rec.Code, http.StatusInternalServerError)
		}
		envelope := decodeErrorEnvelope(t, rec)
		if envelope.Error.Code != httpconst.ErrorCodeInternalServerError {
			t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeInternalServerError)
		}
		if strings.Contains(rec.Body.String(), "connection refused") {
			t.Fatalf("error body leaks internal detail: %s", rec.Body.String())
		}
	})

	t.Run("per-user rate limit guards renewal", func(t *testing.T) {
		t.Parallel()
		stub := &chatMediaServiceStub{}
		router := chatMediaRouter(nil, stub, func(mw *api.MiddlewareSet) {
			mw.RateLimit = middleware.NewRateLimitMiddleware(0, 1)
		})
		messageID := uuid.New().String()

		first := httptest.NewRecorder()
		router.ServeHTTP(first, chatMediaRenewalRequest(messageID))
		if first.Code != http.StatusOK {
			t.Fatalf("first renewal: got %d, want %d body=%s", first.Code, http.StatusOK, first.Body.String())
		}

		second := httptest.NewRecorder()
		router.ServeHTTP(second, chatMediaRenewalRequest(messageID))
		if second.Code != http.StatusTooManyRequests {
			t.Fatalf("second renewal: got %d, want %d body=%s", second.Code, http.StatusTooManyRequests, second.Body.String())
		}
		envelope := decodeErrorEnvelope(t, second)
		if envelope.Error.Code != httpconst.ErrorCodeRateLimitExceeded {
			t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeRateLimitExceeded)
		}
	})
}
