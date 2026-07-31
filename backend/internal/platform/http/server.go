package http

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"time"

	"github.com/google/uuid"
	"halaqaty/backend/internal/platform/httpconst"
)

// RequestIDContextKey is the context key for the request ID.
type RequestIDContextKey struct{}

// RequestIDFromContext returns the request ID stored in the context, if any.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(RequestIDContextKey{}).(string)
	return id
}

// IdempotencyKeyContextKey is the context key for the idempotency key.
type IdempotencyKeyContextKey struct{}

// IdempotencyKeyFromContext returns the idempotency key stored in the context, if any.
func IdempotencyKeyFromContext(ctx context.Context) string {
	key, _ := ctx.Value(IdempotencyKeyContextKey{}).(string)
	return key
}

// IdempotencyKeyMiddleware reads the Idempotency-Key header and stores it in context.
func IdempotencyKeyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(httpconst.HeaderIdempotencyKey)
		if key != "" {
			ctx := context.WithValue(r.Context(), IdempotencyKeyContextKey{}, key)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
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

// RetryPolicy configures retry behavior for safe HTTP requests.
type RetryPolicy struct {
	MaxRetries    int
	BaseDelay     time.Duration
	MaxDelay      time.Duration
	RetryStatuses map[int]struct{}
}

// DefaultRetryPolicy is a conservative policy for safe auth/profile requests.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   2 * time.Second,
		RetryStatuses: map[int]struct{}{
			http.StatusTooManyRequests:     {},
			http.StatusInternalServerError: {},
			http.StatusBadGateway:          {},
			http.StatusServiceUnavailable:  {},
			http.StatusGatewayTimeout:      {},
		},
	}
}

// NewRetryableClient returns an HTTP client with timeouts and a retry transport.
// It is intended for safe, idempotent outbound calls (e.g. Firebase/LiveKit SDKs).
func NewRetryableClient(timeout time.Duration, policy RetryPolicy) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: &retryTransport{base: http.DefaultTransport, policy: policy},
	}
}

type retryTransport struct {
	base   http.RoundTripper
	policy RetryPolicy
}

func (rt *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	lastResp, lastErr := rt.base.RoundTrip(req)
	if !isRetryableRequest(req) {
		return lastResp, lastErr
	}

	for attempt := 1; attempt <= rt.policy.MaxRetries; attempt++ {
		if !rt.shouldRetry(lastResp, lastErr) {
			break
		}
		drainAndClose(lastResp)
		time.Sleep(rt.backoff(attempt))
		newReq, err := cloneRequest(req)
		if err != nil {
			return nil, err
		}
		lastResp, lastErr = rt.base.RoundTrip(newReq)
	}

	return lastResp, lastErr
}

func (rt *retryTransport) shouldRetry(resp *http.Response, err error) bool {
	if err != nil {
		return true
	}
	if resp == nil {
		return false
	}
	_, ok := rt.policy.RetryStatuses[resp.StatusCode]
	return ok
}

func (rt *retryTransport) backoff(attempt int) time.Duration {
	jitter := time.Duration(rand.Int63n(int64(rt.policy.BaseDelay)))
	delay := rt.policy.BaseDelay*time.Duration(1<<attempt) + jitter
	if delay > rt.policy.MaxDelay {
		delay = rt.policy.MaxDelay
	}
	return delay
}

func isRetryableRequest(req *http.Request) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodPut, http.MethodDelete:
		return true
	default:
		return false
	}
}

func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func cloneRequest(req *http.Request) (*http.Request, error) {
	newReq := req.Clone(req.Context())
	if req.Body == nil || req.GetBody == nil {
		return newReq, nil
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, fmt.Errorf("retry get body: %w", err)
	}
	newReq.Body = body
	return newReq, nil
}
