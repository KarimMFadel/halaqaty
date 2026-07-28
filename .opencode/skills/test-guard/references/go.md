# Go Testing Reference

Patterns and conventions for testing Go code in the Halaqaty backend.

## Test file conventions

- Test files live alongside the code they test: `circle_service.go` → `circle_service_test.go`
- Integration tests use the build tag `//go:build integration` as the **first line** of the file, before the `package` declaration
- Unit tests run with `go test ./...`; integration tests run with `go test -tags=integration ./...`
- Use **package `foo_test`** (external test package) for black-box tests; use **package `foo`** only when testing unexported internals that genuinely cannot be exercised externally

```go
//go:build integration

package postgres_test
```

## Table-driven tests (canonical pattern)

Table-driven tests are the idiomatic Go approach for testing multiple scenarios of the same behavior. Use them whenever two or more test cases share the same setup and assertion shape.

```go
func TestCreateCircle_Validation(t *testing.T) {
    t.Parallel()

    tests := []struct {
        name    string
        input   CreateCircleRequest
        wantErr error
    }{
        {
            name:    "empty name returns validation error",
            input:   CreateCircleRequest{Name: ""},
            wantErr: ErrInvalidCircleName,
        },
        {
            name:    "name over 100 chars returns validation error",
            input:   CreateCircleRequest{Name: strings.Repeat("a", 101)},
            wantErr: ErrInvalidCircleName,
        },
        {
            name:  "valid name succeeds",
            input: CreateCircleRequest{Name: "Surah Al-Baqarah Circle"},
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            err := tc.input.Validate()
            if tc.wantErr != nil {
                require.ErrorIs(t, err, tc.wantErr)
                return
            }
            require.NoError(t, err)
        })
    }
}
```

## Testify usage

Use `github.com/stretchr/testify`. Prefer `require` over `assert` when a failure makes subsequent assertions meaningless.

```go
import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// require stops the test immediately on failure
require.NoError(t, err)
require.NotNil(t, circle)

// assert continues and collects failures
assert.Equal(t, "Al-Baqarah Circle", circle.Name)
assert.Equal(t, domain.RoleTeacher, membership.Role)
```

**Error assertion**: always use `errors.Is`-based checks:
```go
require.ErrorIs(t, err, domain.ErrCircleNotFound)
// NOT: require.EqualError(t, err, "circle not found")
```

## Mocking with gomock

Use `github.com/golang/mock/gomock` (or `go.uber.org/mock/gomock`) for interface boundaries. Generate mocks with `mockgen`.

```go
// 1. Define the interface in the consuming package (service layer)
type CircleRepository interface {
    Create(ctx context.Context, circle *Circle) error
    FindByID(ctx context.Context, id uuid.UUID) (*Circle, error)
}

// 2. Generate: go generate ./...
//go:generate mockgen -destination=mocks/mock_circle_repo.go -package=mocks . CircleRepository

// 3. Use in tests
func TestCircleService_Create_DuplicateName(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    repo := mocks.NewMockCircleRepository(ctrl)
    repo.EXPECT().
        Create(gomock.Any(), gomock.Any()).
        Return(domain.ErrDuplicateCircleName)

    svc := NewCircleService(repo)
    _, err := svc.Create(context.Background(), CreateCircleRequest{Name: "Existing"})

    require.ErrorIs(t, err, domain.ErrDuplicateCircleName)
}
```

**Key rule**: set expectations only for the *outcome you care about*, not every method call.

## HTTP handler testing with httptest

```go
func TestCircleHandler_Create_InvalidBody(t *testing.T) {
    t.Parallel()

    ctrl := gomock.NewController(t)
    svc := mocks.NewMockCircleService(ctrl)
    // no expectations set — handler should fail before calling the service

    h := NewCircleHandler(svc)
    router := chi.NewRouter()
    router.Post("/circles", h.Create)

    body := `{"name": ""}` // invalid: empty name
    req := httptest.NewRequest(http.MethodPost, "/circles", strings.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()

    router.ServeHTTP(w, req)

    require.Equal(t, http.StatusBadRequest, w.Code)

    var resp ErrorResponse
    require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
    assert.Equal(t, "ERR_INVALID_INPUT", resp.Error.Code)
}
```

## Integration tests with real PostgreSQL

Use `testcontainers-go` or `dockertest` to spin up a real PostgreSQL instance. Apply `golang-migrate` migrations before the test suite.

```go
//go:build integration

package postgres_test

import (
    "context"
    "testing"

    "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/stretchr/testify/require"
    "github.com/stretchr/testify/suite"
)

type CircleRepositoryTestSuite struct {
    suite.Suite
    container *postgres.PostgresContainer
    repo      CircleRepository
}

func (s *CircleRepositoryTestSuite) SetupSuite() {
    ctx := context.Background()

    container, err := postgres.Run(ctx, "postgres:16-alpine",
        postgres.WithDatabase("halaqaty_test"),
        postgres.WithUsername("test"),
        postgres.WithPassword("test"),
    )
    s.Require().NoError(err)
    s.container = container

    dsn, err := container.ConnectionString(ctx)
    s.Require().NoError(err)

    // apply migrations
    runMigrations(s.T(), dsn)

    pool := connectPool(s.T(), dsn)
    s.repo = NewPostgresCircleRepository(pool)
}

func (s *CircleRepositoryTestSuite) TearDownSuite() {
    s.container.Terminate(context.Background())
}

func (s *CircleRepositoryTestSuite) TestCreate_ThenFindByID() {
    ctx := context.Background()
    circle := &Circle{Name: "Test Circle", TeacherID: uuid.New()}

    err := s.repo.Create(ctx, circle)
    s.Require().NoError(err)
    s.Require().NotEqual(uuid.Nil, circle.ID)

    found, err := s.repo.FindByID(ctx, circle.ID)
    s.Require().NoError(err)
    s.Equal(circle.Name, found.Name)
}

func TestCircleRepositoryTestSuite(t *testing.T) {
    suite.Run(t, new(CircleRepositoryTestSuite))
}
```

## Testing WebSocket handlers

Use `github.com/gorilla/websocket` (or `nhooyr.io/websocket`) test helpers or `net/http/httptest` with a `websocket.Dialer`:

```go
//go:build integration

func TestSessionHub_JoinAndBroadcast(t *testing.T) {
    srv := httptest.NewServer(buildWSHandler(t))
    defer srv.Close()

    wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

    conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
    require.NoError(t, err)
    defer conn1.Close()

    // join event
    sendJSON(t, conn1, JoinEvent{Type: "join", CircleID: testCircleID})

    // assert broadcast received
    var event BroadcastEvent
    require.NoError(t, conn1.ReadJSON(&event))
    assert.Equal(t, "member_joined", event.Type)
}
```

## Testing LiveKit token generation

LiveKit tokens are generated server-side (Constitution §IV.1). Test that the generated token contains the correct claims — do not mock the LiveKit SDK's token builder; test the claims directly.

```go
func TestTokenService_GenerateReciterToken_CanPublishTrue(t *testing.T) {
    svc := NewTokenService(testLiveKitAPIKey, testLiveKitAPISecret)
    token, err := svc.GenerateReciterToken(context.Background(), circleID, userID)
    require.NoError(t, err)

    claims := parseTokenClaims(t, token) // parse without verification for unit tests
    assert.True(t, claims.Video.CanPublish)
    assert.False(t, claims.Video.CanPublishVideo) // always false per constitution
    assert.Equal(t, circleID.String(), claims.Video.Room)
}

func TestTokenService_GenerateListenerToken_CanPublishFalse(t *testing.T) {
    svc := NewTokenService(testLiveKitAPIKey, testLiveKitAPISecret)
    token, err := svc.GenerateListenerToken(context.Background(), circleID, userID)
    require.NoError(t, err)

    claims := parseTokenClaims(t, token)
    assert.False(t, claims.Video.CanPublish)
}
```

## Testing Firebase Auth middleware

Mock the Firebase token verifier at the interface boundary, not the SDK itself:

```go
type TokenVerifier interface {
    VerifyIDToken(ctx context.Context, idToken string) (*auth.Token, error)
}

// In tests:
verifier := mocks.NewMockTokenVerifier(ctrl)
verifier.EXPECT().
    VerifyIDToken(gomock.Any(), "valid-token").
    Return(&auth.Token{UID: "user-123"}, nil)

mw := NewAuthMiddleware(verifier)
```

## Running tests — Makefile commands

Use the project Makefiles. Run from the repo root or from `backend/` directly:

```bash
# Unit tests only (no database required)
make test
# or directly: go test -short ./...

# Integration tests (requires DATABASE_URL)
export DATABASE_URL="postgres://user:pass@localhost:5432/halaqaty?sslmode=disable"
make test-integration
# or directly: go test -tags=integration ./...

# Lint (must be clean before PR — zero violations)
make lint
# or directly: golangci-lint run ./...

# Build the API binary
make build
# or directly: go build -o bin/api ./cmd/api

# Start Docker Compose services (PostgreSQL, MinIO, LiveKit) for integration tests
make up
make down
```

**Migration targets** (used in integration test setup):
```bash
make migrate-up                        # apply all pending migrations
make migrate-fresh                     # ⚠️ drop + recreate + migrate (test env only)
make migrate-create NAME=create_foo    # create a new migration pair
make migrate-status                    # check current migration version
```

## Coverage enforcement

The project targets **≥80% coverage for `backend/internal/`**. Check coverage with:

```bash
go test -coverprofile=coverage.out ./internal/...
go tool cover -func=coverage.out | grep total
```

Do not pad coverage with trivial tests. If coverage is below 80%, identify which behaviors are untested, not which lines are uncovered.

## Test helper conventions

- Put shared test builders and fixtures in `backend/internal/testutil/`
- Name builder functions `New<Type>For<Context>` or `Make<Type>` (e.g., `MakeCircle`, `MakeUserWithRole`)
- Never put test helpers in production packages — use `_test.go` files or the `testutil` package
- Use `t.Cleanup` for resource teardown instead of `defer` when inside subtests
