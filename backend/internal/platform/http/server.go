package http

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// TimeoutMiddleware returns the standard JSON timeout envelope when a request exceeds its deadline.
func TimeoutMiddleware(timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		buffered := &timeoutResponseWriter{header: make(http.Header)}
		done := make(chan struct{})
		go func() {
			defer close(done)
			next.ServeHTTP(buffered, r.WithContext(ctx))
		}()

		select {
		case <-done:
			buffered.writeTo(w)
		case <-ctx.Done():
			WriteError(w, httpconst.ErrorCodeRequestTimeout, httpconst.ErrorMessageRequestTimeout, http.StatusServiceUnavailable)
		}
	})
}

type timeoutResponseWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *timeoutResponseWriter) Header() http.Header {
	return w.header
}

func (w *timeoutResponseWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}

func (w *timeoutResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(body)
}

func (w *timeoutResponseWriter) writeTo(target http.ResponseWriter) {
	for key, values := range w.header {
		target.Header()[key] = append([]string(nil), values...)
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	target.WriteHeader(w.status)
	_, _ = target.Write(w.body.Bytes())
}

// RequestIDContextKey is the context key for the request ID.
type RequestIDContextKey struct{}

// RequestIDFromContext returns the request ID stored in the context, if any.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(RequestIDContextKey{}).(string)
	return id
}

// RequestIDMiddleware reads X-Request-ID from the request or generates a new UUID,
// stores it in the request context, and echoes it in the response header.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(httpconst.HeaderRequestID)
		if requestID == "" {
			requestID = uuid.NewString()
		}
		w.Header().Set(httpconst.HeaderRequestID, requestID)
		ctx := context.WithValue(r.Context(), RequestIDContextKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoggerMiddleware logs each request with request_id, method, path, and status.
func LoggerMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		logger.InfoContext(
			r.Context(),
			"http_request",
			slog.String("request_id", RequestIDFromContext(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", wrapped.statusCode),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

// RecoveryMiddleware recovers from panics, logs them, and returns a 500 error.
func RecoveryMiddleware(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.ErrorContext(
					r.Context(),
					"panic recovered",
					slog.String("request_id", RequestIDFromContext(r.Context())),
					slog.Any("recover", rec),
				)
				WriteError(
					w,
					httpconst.ErrorCodeInternalServerError,
					httpconst.ErrorMessageInternalServerError,
					http.StatusInternalServerError,
				)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (rr *responseRecorder) WriteHeader(code int) {
	rr.statusCode = code
	rr.ResponseWriter.WriteHeader(code)
}

// MaxBytesMiddleware wraps each request body with an http.MaxBytesReader so
// oversized payloads are rejected before reaching handler logic.
// A 1 MiB cap is sufficient for auth/profile JSON; raise if file upload routes
// are added (ponytail: single cap, per-route if limits diverge).
func MaxBytesMiddleware(maxBytes int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
		next.ServeHTTP(w, r)
	})
}
