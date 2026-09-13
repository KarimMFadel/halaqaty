package chat

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUploadReviewErrorResponses(t *testing.T) {
	for _, tc := range []struct {
		name  string
		write func(http.ResponseWriter, error)
		err   error
		want  int
	}{
		{"media retry conflict", writeMediaSendError, ErrIdempotencyConflict, http.StatusConflict},
		{"text retry conflict", writeSendError, ErrIdempotencyConflict, http.StatusConflict},
		{"archived upload", writeUploadError, ErrCircleArchived, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			tc.write(w, tc.err)
			if w.Code != tc.want {
				t.Fatalf("status=%d want %d", w.Code, tc.want)
			}
		})
	}
}
