//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// circleRoleUsers are the actors exercised against the real role middleware chain.
var circleRoleUsers = []string{"creator", "creator2", "teacher_a", "teacher_b", "backup_sup", "student", "outsider"}

// circleTokenVerifier maps one bearer token per test user to a Firebase identity.
type circleTokenVerifier struct {
	tokens map[string]*auth.DecodedToken
}

func (v *circleTokenVerifier) Verify(_ context.Context, bearerToken string) (*auth.DecodedToken, error) {
	decoded, ok := v.tokens[bearerToken]
	if !ok {
		return nil, errors.New("invalid token")
	}
	return decoded, nil
}

// circleRoleEnv holds the wired app and per-user credentials.
type circleRoleEnv struct {
	mux      *http.ServeMux
	svc      *rbac.Service
	repo     *rbac.Repository
	pool     *pgxpool.Pool
	userIDs  map[string]string
	sessions map[string]string
	tokens   map[string]string
}

// setupCircleRoleEnv builds the full HTTP chain on a fresh schema: real
// PostgreSQL repositories, real auth+role middleware, Firebase stubbed at the
// token-verifier boundary.
func setupCircleRoleEnv(t *testing.T) *circleRoleEnv {
	t.Helper()
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	adminPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open admin pool: %v", err)
	}

	conn := acquireConn(t, adminPool, ctx)
	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	for _, file := range []string{
		"000010_auth_roles_profile.up.sql",
		"000011_auth_roles_profile_alignment.up.sql",
		"000012_auth_profiles_display_name.up.sql",
		"000013_create_circles.up.sql",
		"000014_circle_members_circle_fk.up.sql",
		"000015_circle_management.up.sql",
	} {
		runMigrationFile(t, conn, ctx, file)
	}
	conn.Release()

	poolCfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	poolCfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		dropSchema(t, adminPool, ctx, schema)
		adminPool.Close()
	})

	sessionRepo := auth.NewSessionRepository(pool)
	rbacRepo := rbac.NewRepository(pool)
	rbacService := rbac.NewService(rbacRepo, nil)
	rbacHandler := rbac.NewHandler(rbacService)
	authService := auth.NewService(sessionRepo, nil, 24*time.Hour)
	authHandler := auth.NewHandler(authService)

	verifier := &circleTokenVerifier{tokens: make(map[string]*auth.DecodedToken)}
	for _, user := range circleRoleUsers {
		verifier.tokens["Bearer "+user+"-token"] = &auth.DecodedToken{
			UID:   "firebase-" + user,
			Email: user + "@halaqaty.app",
		}
	}

	sessionService := auth.NewSessionService(24 * time.Hour)
	authMW := middleware.NewAuthMiddleware(verifier, sessionService, sessionRepo)
	roleMW := middleware.NewRoleMiddleware(sessionRepo)

	mux := http.NewServeMux()
	mux.Handle("POST /auth/register", authMW.RequireVerifiedFirebase(http.HandlerFunc(authHandler.Register)))
	mux.Handle("POST /circles", authMW.Require(http.HandlerFunc(rbacHandler.CreateCircle)))
	mux.Handle("POST /circles/{circleId}/join", authMW.Require(http.HandlerFunc(rbacHandler.JoinPublicCircle)))
	mux.Handle("POST /circles/join", authMW.Require(http.HandlerFunc(rbacHandler.JoinCircle)))
	mux.Handle(
		"PUT /circles/{circleId}/members/{userId}/role",
		authMW.Require(roleMW.RequireAny("supervisor", "teacher")(http.HandlerFunc(rbacHandler.AssignRole))),
	)

	env := &circleRoleEnv{
		mux:      mux,
		svc:      rbacService,
		repo:     rbacRepo,
		pool:     pool,
		userIDs:  make(map[string]string),
		sessions: make(map[string]string),
		tokens:   make(map[string]string),
	}
	for _, user := range circleRoleUsers {
		env.registerUser(t, user)
	}
	return env
}

func (e *circleRoleEnv) registerUser(t *testing.T, user string) {
	t.Helper()
	token := "Bearer " + user + "-token"
	resp := doJSONRequest(t, e.mux, http.MethodPost, "/auth/register", `{"display_name":"`+user+`"}`, map[string]string{
		httpconst.HeaderAuthorization: token,
		httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("register %s: got %d body=%s", user, resp.Code, resp.Body.String())
	}
	var session auth.BackendSessionResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode register response for %s: %v", user, err)
	}
	e.tokens[user] = token
	e.sessions[user] = session.SessionID
	e.userIDs[user] = session.User.ID
}

func (e *circleRoleEnv) createCircle(t *testing.T, creator, body string) rbac.CircleResponse {
	t.Helper()
	resp := doJSONRequest(t, e.mux, http.MethodPost, "/circles", body, map[string]string{
		httpconst.HeaderAuthorization: e.tokens[creator],
		httpconst.HeaderSessionID:     e.sessions[creator],
		httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("create circle as %s: got %d body=%s", creator, resp.Code, resp.Body.String())
	}
	var circle rbac.CircleResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &circle); err != nil {
		t.Fatalf("decode circle response: %v", err)
	}
	return circle
}

func (e *circleRoleEnv) putRole(t *testing.T, actor, circleID, targetUserID, role string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSONRequest(t, e.mux, http.MethodPut,
		"/circles/"+circleID+"/members/"+targetUserID+"/role",
		`{"role":"`+role+`"}`,
		map[string]string{
			httpconst.HeaderAuthorization: e.tokens[actor],
			httpconst.HeaderSessionID:     e.sessions[actor],
			httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
		})
}

func TestCircleRoleAccess(t *testing.T) {
	env := setupCircleRoleEnv(t)
	ctx := context.Background()

	circle := env.createCircle(t, "creator", fmt.Sprintf(
		`{"name":"Access Circle","teacher_user_ids":[%q,%q],"backup_supervisor_user_id":%q}`,
		env.userIDs["teacher_a"], env.userIDs["teacher_b"], env.userIDs["backup_sup"],
	))
	if circle.ID == "" || circle.InviteCode == "" {
		t.Fatalf("incomplete circle response: %+v", circle)
	}

	// Invitee-student membership is idempotent.
	if err := env.svc.AddStudentMember(ctx, circle.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("add student member: %v", err)
	}
	if err := env.svc.AddStudentMember(ctx, circle.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("add student member replay: %v", err)
	}

	studentID := env.userIDs["student"]
	teacherAID := env.userIDs["teacher_a"]

	cases := []struct {
		name       string
		actor      string
		targetID   string
		role       string
		wantStatus int
	}{
		{"teacher changes another member", "teacher_a", studentID, rbac.RoleTeacher, http.StatusOK},
		{"supervisor changes another member", "backup_sup", studentID, rbac.RoleStudent, http.StatusOK},
		{"student actor rejected", "student", teacherAID, rbac.RoleStudent, http.StatusForbidden},
		{"non-member actor rejected", "outsider", teacherAID, rbac.RoleStudent, http.StatusForbidden},
		{"self change rejected", "teacher_a", teacherAID, rbac.RoleStudent, http.StatusForbidden},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := env.putRole(t, tc.actor, circle.ID, tc.targetID, tc.role)
			if resp.Code != tc.wantStatus {
				t.Fatalf("status: got %d want %d body=%s", resp.Code, tc.wantStatus, resp.Body.String())
			}
		})
	}

	// Final-teacher protection: creator2 is the only teacher; the backup
	// supervisor cannot demote them.
	solo := env.createCircle(t, "creator2", fmt.Sprintf(
		`{"name":"Solo Circle","backup_supervisor_user_id":%q}`, env.userIDs["backup_sup"],
	))
	resp := env.putRole(t, "backup_sup", solo.ID, env.userIDs["creator2"], rbac.RoleStudent)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("final teacher demotion: got %d want %d body=%s", resp.Code, http.StatusForbidden, resp.Body.String())
	}
}

func TestCircleRoleAccess_ConcurrentFinalTeacherProtection(t *testing.T) {
	env := setupCircleRoleEnv(t)

	circle := env.createCircle(t, "creator", fmt.Sprintf(
		`{"name":"Race Circle","teacher_user_ids":[%q,%q]}`,
		env.userIDs["teacher_a"], env.userIDs["teacher_b"],
	))

	// Two concurrent demotions of the last two teachers: exactly one succeeds.
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, target := range []string{env.userIDs["teacher_a"], env.userIDs["teacher_b"]} {
		wg.Add(1)
		go func(targetID string) {
			defer wg.Done()
			resp := env.putRole(t, "creator", circle.ID, targetID, rbac.RoleStudent)
			statuses <- resp.Code
		}(target)
	}
	wg.Wait()
	close(statuses)

	got := make([]int, 0, 2)
	for code := range statuses {
		got = append(got, code)
	}
	sort.Ints(got)
	want := []int{http.StatusOK, http.StatusForbidden}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("concurrent demotions: got statuses %v want exactly one 200 and one 403", got)
	}
}
