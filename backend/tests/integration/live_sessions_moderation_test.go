//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	lkauth "github.com/livekit/protocol/auth"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions/livekit"
)

// T029 — moderation integration coverage against real PostgreSQL: duplicate
// webhook delivery, remove/reconnect denial, lock races, duration/idle end,
// audit redaction, and moderation rate limits.

const (
	whIntegKey    = "itest-key"
	whIntegSecret = "itest-secret-not-a-real-credential"
)

// whJoinedBody is a provider-shaped participant_joined delivery.
const whJoinedBody = `{
	"event": "participant_joined",
	"room": {"name": "room-ref-integ"},
	"participant": {"identity": "student-1"},
	"created_at": 1765000000
}`

// The livekit protocol auth import below is provider-side simulation for
// signing test deliveries only; production code keeps provider imports inside
// backend/internal/sessions/livekit/ (ADR-015).

// signWebhook builds the signed Authorization token LiveKit attaches to a
// webhook delivery, bound to the SHA-256 digest of the body.
func signWebhook(t *testing.T, body string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(body))
	digest := base64.StdEncoding.EncodeToString(sum[:])
	token, err := lkauth.NewAccessToken(whIntegKey, whIntegSecret).SetSha256(digest).ToJWT()
	if err != nil {
		t.Fatalf("sign webhook: %v", err)
	}
	return token
}

// deliverWebhook posts one signed provider callback through the production
// session handler's webhook path.
func deliverWebhook(t *testing.T, handler http.Handler, body, authorization string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/livekit", strings.NewReader(body))
	if authorization != "" {
		req.Header.Set(httpconst.HeaderAuthorization, authorization)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

// signedVerifyReq builds a provider delivery with its signed Authorization
// header attached, for direct verifier calls.
func signedVerifyReq(t *testing.T, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/livekit", strings.NewReader(body))
	req.Header.Set(httpconst.HeaderAuthorization, signWebhook(t, body))
	return req
}

// startActiveSession creates and starts one session with the teacher joined.
func startActiveSession(t *testing.T, env *sessionIntegEnv, ctx context.Context) sessions.Session {
	t.Helper()
	created, err := env.svc.CreateAdHocSession(ctx, env.userIDs["teacher"], env.circleID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	started, _, err := env.svc.StartSession(ctx, env.userIDs["teacher"], created.ID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	return started
}

// presenceCount returns the durable participant count and the number of
// currently-present presence rows; they must always agree.
func presenceCount(t *testing.T, env *sessionIntegEnv, ctx context.Context, sessionID string) (int, int) {
	t.Helper()
	var durable, present int
	err := env.pool.QueryRow(ctx, `
		SELECT (SELECT participant_count FROM sessions WHERE id = $1::uuid),
		       (SELECT COUNT(*) FROM session_participant_presence
		        WHERE session_id = $1::uuid AND is_currently_present)`,
		sessionID).Scan(&durable, &present)
	if err != nil {
		t.Fatalf("count presence: %v", err)
	}
	return durable, present
}

// ---- duplicate webhook delivery -------------------------------------------------

func TestModerationIntegration_DuplicateWebhookDelivery(t *testing.T) {
	env := setupSessionIntegEnv(t)
	handler := sessions.NewHandler(env.svc)
	handler.SetWebhookVerifier(livekit.NewHandlerVerifier(whIntegKey, whIntegSecret))
	webhook := http.Handler(handler)

	token := signWebhook(t, whJoinedBody)

	// Identical redelivery is acknowledged identically (at-least-once).
	for i := 0; i < 2; i++ {
		rec := deliverWebhook(t, webhook, whJoinedBody, token)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("delivery %d = %d: %s", i+1, rec.Code, rec.Body.String())
		}
		if rec.Body.Len() != 0 {
			t.Errorf("delivery %d must not echo provider data: %s", i+1, rec.Body.String())
		}
	}

	// The neutral translation keeps a stable dedup identifier for duplicate
	// deliveries and distinct identifiers for distinct events.
	verifier := livekit.NewWebhookVerifier(whIntegKey, whIntegSecret)
	first, err := verifier.Verify(signedVerifyReq(t, whJoinedBody))
	if err != nil {
		t.Fatalf("verify first: %v", err)
	}
	second, err := verifier.Verify(signedVerifyReq(t, whJoinedBody))
	if err != nil {
		t.Fatalf("verify duplicate: %v", err)
	}
	if first.ID == "" || first.ID != second.ID {
		t.Fatalf("duplicate deliveries must share a stable id: %q vs %q", first.ID, second.ID)
	}
	otherBody := strings.Replace(whJoinedBody, `"student-1"`, `"student-2"`, 1)
	other, err := verifier.Verify(signedVerifyReq(t, otherBody))
	if err != nil {
		t.Fatalf("verify other: %v", err)
	}
	if other.ID == first.ID {
		t.Fatal("distinct events must have distinct ids")
	}

	// Invalid signature is rejected with the standard 401 envelope.
	wrongSecretSum := sha256.Sum256([]byte(whJoinedBody))
	wrongToken, err := lkauth.NewAccessToken(whIntegKey, "wrong-secret").SetSha256(base64.StdEncoding.EncodeToString(wrongSecretSum[:])).ToJWT()
	if err != nil {
		t.Fatalf("sign wrong-secret token: %v", err)
	}
	rec := deliverWebhook(t, webhook, whJoinedBody, wrongToken)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature = %d, want 401", rec.Code)
	}
	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != httpconst.ErrorCodeUnauthorized {
		t.Fatalf("bad signature envelope: %s", rec.Body.String())
	}

	// Unsupported and malformed payloads are rejected with 400.
	unsupported := strings.Replace(whJoinedBody, `"participant_joined"`, `"track_published"`, 1)
	rec = deliverWebhook(t, webhook, unsupported, signWebhook(t, unsupported))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unsupported event = %d, want 400", rec.Code)
	}
	rec = deliverWebhook(t, webhook, "{not json", signWebhook(t, "{not json"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("malformed body = %d, want 400", rec.Code)
	}
}

// ---- remove / reconnect denial ---------------------------------------------------

func TestModerationIntegration_RemoveReconnectDenial(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()
	started := startActiveSession(t, env, ctx)

	if _, _, err := env.svc.JoinSession(ctx, env.userIDs["student"], started.ID); err != nil {
		t.Fatalf("student join: %v", err)
	}
	if err := env.svc.SetHand(ctx, env.userIDs["student"], started.ID, true); err != nil {
		t.Fatalf("raise hand: %v", err)
	}

	removed, err := env.svc.RemoveParticipant(ctx, env.userIDs["teacher"], started.ID, env.userIDs["student"])
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed.ParticipantCount != 1 {
		t.Fatalf("count after remove = %d, want 1", removed.ParticipantCount)
	}

	// The removal stands for every re-entry path while the session lives on.
	if _, _, err := env.svc.JoinSession(ctx, env.userIDs["student"], started.ID); !errors.Is(err, sessions.ErrParticipantRemoved) {
		t.Fatalf("rejoin: got %v, want ErrParticipantRemoved", err)
	}
	if _, err := env.repo.ReconnectPresence(ctx, started.ID, env.userIDs["student"]); !errors.Is(err, sessions.ErrParticipantRemoved) {
		t.Fatalf("reconnect: got %v, want ErrParticipantRemoved", err)
	}
	if err := env.svc.SetHand(ctx, env.userIDs["student"], started.ID, true); !errors.Is(err, sessions.ErrParticipantRemoved) {
		t.Fatalf("raise hand after removal: got %v, want ErrParticipantRemoved", err)
	}
	if err := env.svc.AuthorizeSessionTopic(ctx, env.userIDs["student"], started.ID); !errors.Is(err, sessions.ErrParticipantRemoved) {
		t.Fatalf("session topic after removal: got %v, want ErrParticipantRemoved", err)
	}

	// Duplicate removal converges without corrupting the count.
	if _, err := env.svc.RemoveParticipant(ctx, env.userIDs["supervisor"], started.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("duplicate remove must converge: %v", err)
	}

	durable, present := presenceCount(t, env, ctx, started.ID)
	if durable != 1 || present != 1 {
		t.Fatalf("after removal durable=%d present=%d, want 1/1", durable, present)
	}

	// The durable removal marker survives with hand state cleared.
	var removedAt, handRaisedAt *string
	err = env.pool.QueryRow(ctx, `
		SELECT removed_at::text, hand_raised_at::text
		FROM session_participant_presence
		WHERE session_id = $1::uuid AND user_id = $2::uuid`,
		started.ID, env.userIDs["student"]).Scan(&removedAt, &handRaisedAt)
	if err != nil {
		t.Fatalf("load presence row: %v", err)
	}
	if removedAt == nil {
		t.Fatal("removed_at must be durable")
	}
	if handRaisedAt != nil {
		t.Fatal("removal must clear the raised hand")
	}
}

// ---- lock race ---------------------------------------------------------------------

func TestModerationIntegration_LockRace(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()
	started := startActiveSession(t, env, ctx)

	if _, _, err := env.svc.JoinSession(ctx, env.userIDs["student"], started.ID); err != nil {
		t.Fatalf("student join: %v", err)
	}

	// Seed three fresh members that race joins against lock toggles.
	joiners := make([]string, 3)
	conn, _ := env.pool.Acquire(ctx)
	for i := range joiners {
		var id string
		if err := conn.QueryRow(ctx,
			`INSERT INTO users (firebase_uid, email) VALUES ($1, $2) RETURNING id::text`,
			fmt.Sprintf("race-%d", i), fmt.Sprintf("race-%d@test.com", i)).Scan(&id); err != nil {
			t.Fatalf("seed race user %d: %v", i, err)
		}
		if _, err := conn.Exec(ctx,
			`INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1::uuid, $2::uuid, 'student')`,
			env.circleID, id); err != nil {
			t.Fatalf("seed race member %d: %v", i, err)
		}
		joiners[i] = id
	}
	conn.Release()

	const toggles = 5
	type outcome struct {
		err error
	}
	lockErrs := make([]error, 2*toggles)
	joinOutcomes := make([]outcome, len(joiners))
	var wg sync.WaitGroup
	for i := 0; i < toggles; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			_, lockErrs[i] = env.svc.SetLock(ctx, env.userIDs["teacher"], started.ID, i%2 == 0)
		}(i)
		go func(i int) {
			defer wg.Done()
			_, lockErrs[toggles+i] = env.svc.SetLock(ctx, env.userIDs["supervisor"], started.ID, i%2 == 1)
		}(i)
	}
	for i, joiner := range joiners {
		wg.Add(1)
		go func(i int, joiner string) {
			defer wg.Done()
			_, _, joinOutcomes[i].err = env.svc.JoinSession(ctx, joiner, started.ID)
		}(i, joiner)
	}
	wg.Wait()

	joinsOK := 0
	for _, o := range joinOutcomes {
		switch {
		case o.err == nil:
			joinsOK++
		case errors.Is(o.err, sessions.ErrSessionLocked):
			// Legitimate: the lock was on at that instant.
		default:
			t.Fatalf("race join failed unexpectedly: %v", o.err)
		}
	}
	for _, err := range lockErrs {
		if err != nil {
			t.Fatalf("lock toggle failed: %v", err)
		}
	}

	// Final state must be internally consistent no matter who won.
	final, err := env.repo.GetSession(ctx, started.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if final.Status != sessions.SessionStatusActive {
		t.Fatalf("status = %q, want active", final.Status)
	}
	durable, present := presenceCount(t, env, ctx, started.ID)
	if durable != present {
		t.Fatalf("corrupt state: participant_count=%d but present rows=%d", durable, present)
	}
	if want := 2 + joinsOK; durable != want {
		t.Fatalf("final count = %d, want %d (2 initial + %d joined)", durable, want, joinsOK)
	}
}

// ---- duration / idle end -------------------------------------------------------------

func TestModerationIntegration_DurationAndIdleEnd(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()

	// Duration-limited end clears presence and keeps durable history.
	a := startActiveSession(t, env, ctx)
	if _, _, err := env.svc.JoinSession(ctx, env.userIDs["student"], a.ID); err != nil {
		t.Fatalf("student join: %v", err)
	}
	ended, err := env.svc.EndSession(ctx, env.userIDs["teacher"], a.ID, sessions.EndReasonDurationLimit)
	if err != nil {
		t.Fatalf("duration end: %v", err)
	}
	if ended.Status != sessions.SessionStatusEnded || ended.EndReason != sessions.EndReasonDurationLimit {
		t.Fatalf("ended = %q/%q, want ended/duration_limit", ended.Status, ended.EndReason)
	}
	if ended.ActualEnd == nil {
		t.Fatal("actual_end must be recorded")
	}
	if _, present := presenceCount(t, env, ctx, a.ID); present != 0 {
		t.Fatalf("ended session still has %d present participants", present)
	}
	var historyRows int
	if err := env.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM session_participant_presence WHERE session_id = $1::uuid`, a.ID).Scan(&historyRows); err != nil {
		t.Fatalf("count history: %v", err)
	}
	if historyRows != 2 {
		t.Fatalf("history rows = %d, want 2 (history is retained)", historyRows)
	}

	// The CAS rejects a second end.
	if _, err := env.svc.EndSession(ctx, env.userIDs["teacher"], a.ID, sessions.EndReasonManual); !errors.Is(err, sessions.ErrSessionAlreadyEnded) {
		t.Fatalf("second end: got %v, want ErrSessionAlreadyEnded", err)
	}

	// Idle end is attributed distinctly and supervisors have equal rights.
	b := startActiveSession(t, env, ctx)
	idle, err := env.svc.EndSession(ctx, env.userIDs["supervisor"], b.ID, sessions.EndReasonIdleTimeout)
	if err != nil {
		t.Fatalf("idle end: %v", err)
	}
	if idle.EndReason != sessions.EndReasonIdleTimeout {
		t.Fatalf("idle end_reason = %q, want idle_timeout", idle.EndReason)
	}

	// The schema rejects end reasons outside the durable attribution set.
	c := startActiveSession(t, env, ctx)
	if _, err := env.repo.EndSession(ctx, c.ID, sessions.EndReason("bogus")); err == nil {
		t.Fatal("invalid end_reason must be rejected by the end-reason CHECK constraint")
	}
}

// ---- audit redaction -------------------------------------------------------------------

func TestModerationIntegration_AuditRedaction(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()
	started := startActiveSession(t, env, ctx)
	_, teacherConn, err := env.svc.StartSession(ctx, env.userIDs["teacher"], started.ID)
	if err != nil || teacherConn.Credential == "" {
		t.Fatalf("teacher credential must be issued: %v", err)
	}
	_, studentConn, err := env.svc.JoinSession(ctx, env.userIDs["student"], started.ID)
	if err != nil || studentConn.Credential == "" {
		t.Fatalf("student credential must be issued: %v", err)
	}

	// Default formatting of credentials and connections must never leak the
	// raw value into logs or audit output.
	for _, got := range []string{
		fmt.Sprintf("%v", studentConn.Credential),
		fmt.Sprintf("%+v", studentConn),
		fmt.Sprintf("%#v", studentConn),
		fmt.Sprintf("%v", []sessions.MediaConnection{teacherConn, studentConn}),
	} {
		if strings.Contains(got, string(studentConn.Credential)) || strings.Contains(got, string(teacherConn.Credential)) {
			t.Errorf("formatting leaks a credential: %s", got)
		}
		if !strings.Contains(got, "[REDACTED]") {
			t.Errorf("formatting must show the redaction marker: %s", got)
		}
	}

	// The neutral webhook translation carries no provider names or secrets.
	event, err := livekit.NewWebhookVerifier(whIntegKey, whIntegSecret).Verify(signedVerifyReq(t, whJoinedBody))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	encoded, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal neutral event: %v", err)
	}
	for _, banned := range []string{"livekit", whIntegSecret, "api_secret", "api_key"} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(banned)) {
			t.Errorf("neutral event leaks %q: %s", banned, encoded)
		}
	}

	// Verification failures never echo the signing secret.
	wrongSum := sha256.Sum256([]byte(whJoinedBody))
	wrongToken, err := lkauth.NewAccessToken(whIntegKey, "wrong-secret").SetSha256(base64.StdEncoding.EncodeToString(wrongSum[:])).ToJWT()
	if err != nil {
		t.Fatalf("sign wrong token: %v", err)
	}
	badReq := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/livekit", strings.NewReader(whJoinedBody))
	badReq.Header.Set(httpconst.HeaderAuthorization, wrongToken)
	_, err = livekit.NewWebhookVerifier(whIntegKey, whIntegSecret).Verify(badReq)
	if err == nil {
		t.Fatal("wrong secret must fail verification")
	}
	if strings.Contains(err.Error(), whIntegSecret) || strings.Contains(err.Error(), "wrong-secret") {
		t.Fatalf("verification error leaks the secret: %v", err)
	}

	// Moderation responses never carry any participant's credential.
	handler := sessions.NewHandler(env.svc)
	for _, tc := range []struct {
		method string
		target string
	}{
		{http.MethodPost, "/api/v1/sessions/" + started.ID + "/lock"},
		{http.MethodPost, "/api/v1/sessions/" + started.ID + "/participants/mute-all"},
		{http.MethodGet, "/api/v1/sessions/" + started.ID + "/participants"},
	} {
		req := httptest.NewRequest(tc.method, tc.target, strings.NewReader(`{}`))
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: env.userIDs["teacher"]}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		body := rec.Body.String()
		if strings.Contains(body, string(teacherConn.Credential)) || strings.Contains(body, string(studentConn.Credential)) {
			t.Errorf("%s response leaks a credential: %s", tc.target, body)
		}
	}
}

// ---- moderation rate limits -------------------------------------------------------------

// doRateLimitedReq mirrors the router's requireWithUserLimit chain: the
// per-user limit applies after the principal is known.
func doRateLimitedReq(h http.Handler, actor, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: actor}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestModerationIntegration_ModerationRateLimit(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()
	started := startActiveSession(t, env, ctx)

	// Same shape the production router uses: per-user budget of 3/min.
	limiter := middleware.NewRateLimitMiddleware(0, 3)
	handler := limiter.Limit(sessions.NewHandler(env.svc))

	for i := 0; i < 3; i++ {
		rec := doRateLimitedReq(handler, env.userIDs["teacher"], http.MethodPost,
			"/api/v1/sessions/"+started.ID+"/lock", fmt.Sprintf(`{"locked":%t}`, i%2 == 0))
		if rec.Code != http.StatusOK {
			t.Fatalf("lock %d = %d: %s", i+1, rec.Code, rec.Body.String())
		}
	}
	rec := doRateLimitedReq(handler, env.userIDs["teacher"], http.MethodPost,
		"/api/v1/sessions/"+started.ID+"/lock", `{"locked":true}`)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("4th request = %d, want 429: %s", rec.Code, rec.Body.String())
	}
	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != httpconst.ErrorCodeRateLimitExceeded {
		t.Fatalf("429 envelope = %s", rec.Body.String())
	}

	// Per-user budgets are isolated: the student's requests still pass the
	// limiter and hit RBAC (403), not the rate limit (429).
	rec = doRateLimitedReq(handler, env.userIDs["student"], http.MethodPost,
		"/api/v1/sessions/"+started.ID+"/lock", `{"locked":true}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("student lock = %d, want 403: %s", rec.Code, rec.Body.String())
	}

	// The durable state survived the rejected flood.
	final, err := env.repo.GetSession(ctx, started.ID)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !final.IsLocked {
		t.Fatal("the last accepted lock (true) must be durable")
	}
}
