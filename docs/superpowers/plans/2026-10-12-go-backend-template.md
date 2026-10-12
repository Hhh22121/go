# Go Backend Template Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A reusable Go REST backend template (Gin + Postgres + JWT) with one example `user` module, tests, Docker and tooling.

**Architecture:** Feature-module layout under `internal/`. `cmd/api/main.go` only wires. Domain packages return sentinel errors from `internal/apperr`; `internal/httpx` maps them to HTTP. `user` never imports `auth` — the auth middleware is injected by `main`, and the caller ID travels through `httpx.SetUserID/UserID`.

**Tech Stack:** Go 1.27 (brew), Gin, pgx/v5 + pgxpool, sqlc (pgx/v5), goose v3 (embedded SQL, Provider API), golang-jwt/jwt/v5, bcrypt, google/uuid, gin-contrib/cors, testcontainers-go (postgres module), golangci-lint v2.

**Spec:** `docs/superpowers/specs/2026-10-12-go-backend-template-design.md`

**Conventions for every task:**
- Repo root: `/Users/hoangvx/Go/go-backend-template` (its own git repo, branch `main`). Never commit to the parent `~` repo.
- Every commit message ends with a blank line then `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Run unit tests with `GIN_MODE=test go test ./...` (quiets Gin debug output).
- Deviations from spec (intentional): goose `NewProvider` instead of global `SetBaseFS`; `TokenService.Issue` returns `(string, error)` and `TTL()` supplies `expires_in`; password helpers are plain functions; Postgres image is `postgres:17-alpine`.

---

## File Map

| File | Responsibility |
|------|----------------|
| `go.mod` | module `go-backend-template`, `tool` directives for sqlc + goose |
| `cmd/api/main.go` | load config, logger, pool, migrate, build modules, run server |
| `internal/apperr/apperr.go` | sentinel errors |
| `internal/httpx/errors.go` | `WriteError`, JSON envelope, `RequestIDKey` |
| `internal/httpx/bind.go` | `BindJSON` → 400 on bind/validation errors |
| `internal/httpx/context.go` | `SetUserID`, `UserID` |
| `internal/httpx/httpxtest/httpxtest.go` | test helpers: `Do`, `DecodeJSON` |
| `internal/config/config.go` | env → `Config` |
| `migrations/embed.go`, `migrations/00001_create_users.sql` | embedded goose migrations |
| `queries/users.sql`, `sqlc.yaml` | sqlc inputs |
| `internal/store/*` | sqlc output (generated, committed) |
| `internal/database/database.go` | `NewPool`, `Migrate` |
| `internal/testutil/postgres.go` | `//go:build integration` testcontainers Postgres helper |
| `internal/user/{user,repository,service,handler}.go` | example module |
| `internal/user/usertest/memory.go` | in-memory `user.Repository` for tests |
| `internal/auth/{password,token,middleware,handler}.go` | auth |
| `internal/server/{middleware,server}.go` | engine, middleware, health, `Run` |
| `Makefile`, `Dockerfile`, `docker-compose.yml`, `.golangci.yml`, `.env.example`, `.gitignore`, `.dockerignore`, `README.md` | tooling |

---

### Task 1: Toolchain upgrade and module scaffold

**Files:**
- Create: `go.mod`, `.gitignore`, `.env.example`, `docker-compose.yml`

- [ ] **Step 1: Install latest Go via brew**

Run: `brew install go && /opt/homebrew/bin/go version`
Expected: `go version go1.27.x darwin/arm64`

- [ ] **Step 2: Make sure the brew Go wins on PATH**

Run: `zsh -lc 'which go; go version'`
Expected: `/opt/homebrew/bin/go` and `go1.27.x`.
If it still prints `/usr/local/go/bin/go` (1.21.1), the old official install is first via `/etc/paths.d/go`. Ask the user before touching `/usr/local/go` or `/etc/paths.d` (needs sudo). As a non-destructive fallback, add `export PATH="/opt/homebrew/bin:$PATH"` to the end of `~/.zshrc` after confirming with the user. All later commands in this plan assume `go` resolves to 1.27.x; if PATH isn't fixed yet, use `/opt/homebrew/bin/go` explicitly.

- [ ] **Step 3: Init the module**

```bash
cd /Users/hoangvx/Go/go-backend-template
go mod init go-backend-template
```

Expected: `go.mod` created with a `go 1.2x` directive (recent Go may write the previous minor version — that's fine; later `go get` may raise it).

- [ ] **Step 4: Write `.gitignore`**

```gitignore
/bin/
.env
*.out
coverage.*
.DS_Store
```

- [ ] **Step 5: Write `.env.example`**

```dotenv
APP_ENV=dev
HTTP_ADDR=:8080
DATABASE_URL=postgres://app:app@localhost:5432/app?sslmode=disable
JWT_SECRET=change-me-to-a-random-string-of-32-bytes-or-more
JWT_TTL=15m
SHUTDOWN_TIMEOUT=10s
CORS_ORIGINS=http://localhost:3000
```

- [ ] **Step 6: Write `docker-compose.yml`**

```yaml
services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: app
      POSTGRES_PASSWORD: app
      POSTGRES_DB: app
    ports:
      - "${POSTGRES_PORT:-5432}:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U app -d app"]
      interval: 2s
      timeout: 3s
      retries: 15
    volumes:
      - pgdata:/var/lib/postgresql/data

  api:
    build: .
    profiles: ["api"]
    environment:
      APP_ENV: dev
      HTTP_ADDR: ":8080"
      DATABASE_URL: postgres://app:app@postgres:5432/app?sslmode=disable
      JWT_SECRET: ${JWT_SECRET:-change-me-to-a-random-string-of-32-bytes-or-more}
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy

volumes:
  pgdata:
```

- [ ] **Step 7: Validate compose file**

Run: `docker compose config --quiet && echo ok`
Expected: `ok`

- [ ] **Step 8: Commit**

```bash
git add go.mod .gitignore .env.example docker-compose.yml
git commit -m "chore: init go module, env example, compose postgres"
```

---

### Task 2: apperr + httpx (errors, bind, user-ID context)

**Files:**
- Create: `internal/apperr/apperr.go`, `internal/httpx/errors.go`, `internal/httpx/bind.go`, `internal/httpx/context.go`, `internal/httpx/httpxtest/httpxtest.go`
- Test: `internal/httpx/httpx_test.go`

- [ ] **Step 1: Write `internal/apperr/apperr.go`**

```go
// Package apperr defines domain-level sentinel errors shared by all modules.
// Wrap them with fmt.Errorf("...%w...") and classify with errors.Is.
package apperr

import "errors"

var (
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("already exists")
	ErrUnauthorized = errors.New("unauthorized")
	ErrValidation   = errors.New("invalid input")
)
```

- [ ] **Step 2: Add dependencies**

Run: `go get github.com/gin-gonic/gin github.com/google/uuid`

- [ ] **Step 3: Write test helpers `internal/httpx/httpxtest/httpxtest.go`**

```go
// Package httpxtest holds helpers for HTTP handler tests.
package httpxtest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Do sends a request with an optional JSON body and bearer token.
func Do(t *testing.T, h http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// DecodeJSON decodes the recorder body into T or fails the test.
func DecodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return v
}

// ErrorBody mirrors the httpx error envelope.
type ErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}
```

- [ ] **Step 4: Write the failing test `internal/httpx/httpx_test.go`**

```go
package httpx_test

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"go-backend-template/internal/apperr"
	"go-backend-template/internal/httpx"
	"go-backend-template/internal/httpx/httpxtest"
)

func TestWriteError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
		msg    string
	}{
		{"not found", fmt.Errorf("user %w", apperr.ErrNotFound), http.StatusNotFound, "not_found", "user not found"},
		{"conflict", fmt.Errorf("email %w", apperr.ErrConflict), http.StatusConflict, "conflict", "email already exists"},
		{"unauthorized", apperr.ErrUnauthorized, http.StatusUnauthorized, "unauthorized", "unauthorized"},
		{"validation", fmt.Errorf("%w: bad", apperr.ErrValidation), http.StatusBadRequest, "validation_error", "invalid input: bad"},
		{"internal", errors.New("db exploded"), http.StatusInternalServerError, "internal_error", "internal server error"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := gin.New()
			r.GET("/", func(c *gin.Context) { httpx.WriteError(c, tc.err) })
			rec := httpxtest.Do(t, r, http.MethodGet, "/", "", "")
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			body := httpxtest.DecodeJSON[httpxtest.ErrorBody](t, rec)
			if body.Error.Code != tc.code || body.Error.Message != tc.msg {
				t.Fatalf("body = %+v, want code %q msg %q", body.Error, tc.code, tc.msg)
			}
		})
	}
}

func TestBindJSON(t *testing.T) {
	type req struct {
		Name string `json:"name" binding:"required"`
	}
	r := gin.New()
	r.POST("/", func(c *gin.Context) {
		var in req
		if !httpx.BindJSON(c, &in) {
			return
		}
		c.JSON(http.StatusOK, in)
	})

	if rec := httpxtest.Do(t, r, http.MethodPost, "/", `{"name":"a"}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("valid body: status = %d", rec.Code)
	}
	for _, body := range []string{`{}`, `not json`, ``} {
		rec := httpxtest.Do(t, r, http.MethodPost, "/", body, "")
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %q: status = %d, want 400", body, rec.Code)
		}
		if got := httpxtest.DecodeJSON[httpxtest.ErrorBody](t, rec).Error.Code; got != "validation_error" {
			t.Fatalf("body %q: code = %q", body, got)
		}
	}
}

func TestUserID(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	if _, ok := httpx.UserID(c); ok {
		t.Fatal("UserID on empty context returned ok")
	}
	id := uuid.New()
	httpx.SetUserID(c, id)
	got, ok := httpx.UserID(c)
	if !ok || got != id {
		t.Fatalf("UserID = %v, %v; want %v, true", got, ok, id)
	}
}
```

- [ ] **Step 5: Run test to verify it fails**

Run: `GIN_MODE=test go test ./internal/httpx/...`
Expected: FAIL — `undefined: httpx.WriteError` (and BindJSON, UserID, SetUserID).

- [ ] **Step 6: Write `internal/httpx/errors.go`**

```go
// Package httpx contains HTTP-edge helpers shared by all modules.
package httpx

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"go-backend-template/internal/apperr"
)

// RequestIDKey is the Gin context key holding the request ID.
const RequestIDKey = "request_id"

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteError aborts the request with a JSON error envelope. Known apperr
// sentinels map to 4xx with their message; anything else is logged and
// returned as a generic 500.
func WriteError(c *gin.Context, err error) {
	status, code := classify(err)
	msg := err.Error()
	if status == http.StatusInternalServerError {
		slog.ErrorContext(c.Request.Context(), "internal error",
			"err", err, "request_id", c.GetString(RequestIDKey))
		msg = "internal server error"
	}
	abort(c, status, code, msg)
}

func classify(err error) (int, string) {
	switch {
	case errors.Is(err, apperr.ErrNotFound):
		return http.StatusNotFound, "not_found"
	case errors.Is(err, apperr.ErrConflict):
		return http.StatusConflict, "conflict"
	case errors.Is(err, apperr.ErrUnauthorized):
		return http.StatusUnauthorized, "unauthorized"
	case errors.Is(err, apperr.ErrValidation):
		return http.StatusBadRequest, "validation_error"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

func abort(c *gin.Context, status int, code, msg string) {
	c.AbortWithStatusJSON(status, gin.H{"error": errorBody{Code: code, Message: msg}})
}
```

- [ ] **Step 7: Write `internal/httpx/bind.go`**

```go
package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BindJSON binds and validates the JSON body into dst. On failure it writes a
// 400 validation_error response and returns false.
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		abort(c, http.StatusBadRequest, "validation_error", err.Error())
		return false
	}
	return true
}
```

- [ ] **Step 8: Write `internal/httpx/context.go`**

```go
package httpx

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const userIDKey = "httpx.user_id"

// SetUserID stores the authenticated user's ID in the Gin context.
func SetUserID(c *gin.Context, id uuid.UUID) {
	c.Set(userIDKey, id)
}

// UserID returns the authenticated user's ID; ok is false when no
// authentication middleware ran for this request.
func UserID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(userIDKey)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok && id != uuid.Nil
}
```

- [ ] **Step 9: Run tests to verify they pass**

Run: `go mod tidy && GIN_MODE=test go test ./internal/...`
Expected: `ok  go-backend-template/internal/httpx`

- [ ] **Step 10: Commit**

```bash
git add go.mod go.sum internal/apperr internal/httpx
git commit -m "feat: add apperr sentinels and httpx error/bind/context helpers"
```

---

### Task 3: config

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test `internal/config/config_test.go`**

```go
package config

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

var validSecret = strings.Repeat("s", 32)

func TestLoadDefaults(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"DATABASE_URL": "postgres://x",
		"JWT_SECRET":   validSecret,
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := Config{
		Env:             "dev",
		HTTPAddr:        ":8080",
		DatabaseURL:     "postgres://x",
		JWTSecret:       validSecret,
		JWTTTL:          15 * time.Minute,
		ShutdownTimeout: 10 * time.Second,
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Fatalf("cfg = %+v\nwant %+v", cfg, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := load(env(map[string]string{
		"APP_ENV":          "prod",
		"HTTP_ADDR":        ":9000",
		"DATABASE_URL":     "postgres://x",
		"JWT_SECRET":       validSecret,
		"JWT_TTL":          "1h",
		"SHUTDOWN_TIMEOUT": "3s",
		"CORS_ORIGINS":     " http://a.com , ,http://b.com",
	}))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Env != "prod" || cfg.HTTPAddr != ":9000" || cfg.JWTTTL != time.Hour || cfg.ShutdownTimeout != 3*time.Second {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
	if want := []string{"http://a.com", "http://b.com"}; !reflect.DeepEqual(cfg.CORSOrigins, want) {
		t.Fatalf("CORSOrigins = %q, want %q", cfg.CORSOrigins, want)
	}
}

func TestLoadReportsAllErrors(t *testing.T) {
	_, err := load(env(map[string]string{
		"APP_ENV":    "staging",
		"JWT_SECRET": "short",
		"JWT_TTL":    "soon",
	}))
	if err == nil {
		t.Fatal("expected error")
	}
	for _, want := range []string{"APP_ENV", "DATABASE_URL", "JWT_SECRET", "JWT_TTL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/`
Expected: FAIL — `undefined: load`, `undefined: Config`.

- [ ] **Step 3: Write `internal/config/config.go`**

```go
// Package config loads application configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

type Config struct {
	Env             string // "dev" or "prod"
	HTTPAddr        string
	DatabaseURL     string
	JWTSecret       string
	JWTTTL          time.Duration
	ShutdownTimeout time.Duration
	CORSOrigins     []string // empty disables CORS middleware
}

// Load reads configuration from the process environment.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	var errs []error
	cfg := Config{
		Env:         withDefault(getenv("APP_ENV"), "dev"),
		HTTPAddr:    withDefault(getenv("HTTP_ADDR"), ":8080"),
		DatabaseURL: getenv("DATABASE_URL"),
		JWTSecret:   getenv("JWT_SECRET"),
		CORSOrigins: splitList(getenv("CORS_ORIGINS")),
	}
	if cfg.Env != "dev" && cfg.Env != "prod" {
		errs = append(errs, fmt.Errorf("APP_ENV must be dev or prod, got %q", cfg.Env))
	}
	if cfg.DatabaseURL == "" {
		errs = append(errs, errors.New("DATABASE_URL is required"))
	}
	if len(cfg.JWTSecret) < 32 {
		errs = append(errs, errors.New("JWT_SECRET must be at least 32 bytes"))
	}
	var err error
	if cfg.JWTTTL, err = duration(getenv, "JWT_TTL", 15*time.Minute); err != nil {
		errs = append(errs, err)
	}
	if cfg.ShutdownTimeout, err = duration(getenv, "SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return Config{}, fmt.Errorf("config: %w", errors.Join(errs...))
	}
	return cfg, nil
}

func withDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func duration(getenv func(string) string, key string, def time.Duration) (time.Duration, error) {
	v := getenv(key)
	if v == "" {
		return def, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, got %q", key, v)
	}
	return d, nil
}

func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/`
Expected: `ok`

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat: add env-based config loader"
```

---

### Task 4: Migrations, sqlc queries, generated store

**Files:**
- Create: `migrations/embed.go`, `migrations/00001_create_users.sql`, `queries/users.sql`, `sqlc.yaml`
- Generated: `internal/store/db.go`, `internal/store/models.go`, `internal/store/users.sql.go`

- [ ] **Step 1: Write `migrations/00001_create_users.sql`**

```sql
-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email         citext UNIQUE NOT NULL,
    password_hash text NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE users;
```

- [ ] **Step 2: Write `migrations/embed.go`**

```go
// Package migrations embeds the goose SQL migrations.
package migrations

import "embed"

// FS holds every *.sql migration at its root.
//
//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 3: Write `queries/users.sql`**

```sql
-- name: CreateUser :one
INSERT INTO users (email, password_hash)
VALUES ($1, $2)
RETURNING id, email, password_hash, created_at, updated_at;

-- name: GetUserByID :one
SELECT id, email, password_hash, created_at, updated_at
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT id, email, password_hash, created_at, updated_at
FROM users
WHERE email = $1;
```

- [ ] **Step 4: Write `sqlc.yaml`**

```yaml
version: "2"
sql:
  - engine: postgresql
    schema: migrations
    queries: queries
    gen:
      go:
        package: store
        out: internal/store
        sql_package: pgx/v5
        overrides:
          - db_type: uuid
            go_type: github.com/google/uuid.UUID
          - db_type: citext
            go_type: string
          - db_type: timestamptz
            go_type: time.Time
```

- [ ] **Step 5: Pin sqlc and goose as go tools**

```bash
go get -tool github.com/sqlc-dev/sqlc/cmd/sqlc@latest
go get -tool github.com/pressly/goose/v3/cmd/goose@latest
go tool sqlc version
go tool goose --version
```

Expected: both print a version. sqlc builds `pg_query_go` with cgo — the first build takes a few minutes. If `go get -tool` for sqlc fails to build, fall back to `brew install sqlc`, replace `go tool sqlc` with `sqlc` in the Makefile (Task 10), and note the version in the README.

- [ ] **Step 6: Generate the store**

Run: `go tool sqlc generate && ls internal/store`
Expected: `db.go  models.go  users.sql.go`

- [ ] **Step 7: Verify generated types**

Run: `grep -A6 'type User struct' internal/store/models.go`
Expected:

```go
type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}
```

If `CreatedAt` is `pgtype.Timestamptz` or `Email` is `pgtype.Text`, change the override `db_type` to `pg_catalog.timestamptz` / `public.citext` respectively, regenerate, and recheck.

- [ ] **Step 8: Build**

Run: `go mod tidy && go build ./...`
Expected: no output.

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum migrations queries sqlc.yaml internal/store
git commit -m "feat: add users migration, sqlc queries and generated store"
```

---

### Task 5: database package + integration test helper

**Files:**
- Create: `internal/database/database.go`, `internal/testutil/postgres.go`
- Test: `internal/database/database_integration_test.go`

- [ ] **Step 1: Add dependencies**

Run: `go get github.com/jackc/pgx/v5 github.com/pressly/goose/v3 github.com/testcontainers/testcontainers-go github.com/testcontainers/testcontainers-go/modules/postgres`

- [ ] **Step 2: Write `internal/testutil/postgres.go`**

```go
//go:build integration

// Package testutil provides helpers for integration tests.
package testutil

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"go-backend-template/internal/database"
)

// NewPostgres starts a throwaway Postgres container, runs migrations and
// returns a pool. Everything is cleaned up when the test ends.
func NewPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	ctr, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("app"),
		postgres.WithUsername("app"),
		postgres.WithPassword("app"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })

	url, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := database.NewPool(ctx, url)
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return pool
}
```

- [ ] **Step 3: Write the failing test `internal/database/database_integration_test.go`**

```go
//go:build integration

package database_test

import (
	"context"
	"testing"

	"go-backend-template/internal/database"
	"go-backend-template/internal/testutil"
)

func TestMigrateIsIdempotent(t *testing.T) {
	pool := testutil.NewPostgres(t) // already migrated once
	ctx := context.Background()

	if err := database.Migrate(ctx, pool); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var n int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Fatalf("users table missing: %v", err)
	}
}
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test -tags integration ./internal/database/`
Expected: FAIL — `undefined: database.NewPool` / `database.Migrate`.

- [ ] **Step 5: Write `internal/database/database.go`**

```go
// Package database owns the Postgres connection pool and migrations.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"go-backend-template/migrations"
)

// NewPool connects to Postgres and verifies the connection with a ping.
func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies all pending embedded migrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	db := stdlib.OpenDBFromPool(pool) // closing db does not close pool
	defer func() { _ = db.Close() }()

	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		return fmt.Errorf("goose provider: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go mod tidy && go test -tags integration ./internal/database/`
Expected: `ok` (needs the Docker daemon running; first run pulls `postgres:17-alpine`).

- [ ] **Step 7: Confirm unit build ignores integration files**

Run: `go vet ./... && go test ./...`
Expected: passes; `internal/testutil` is excluded without the tag.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/database internal/testutil
git commit -m "feat: add pgx pool, embedded goose migrations, testcontainers helper"
```

---

### Task 6: user module

**Files:**
- Create: `internal/user/user.go`, `internal/user/repository.go`, `internal/user/service.go`, `internal/user/handler.go`, `internal/user/usertest/memory.go`
- Test: `internal/user/service_test.go`, `internal/user/handler_test.go`, `internal/user/repository_integration_test.go`

- [ ] **Step 1: Write `internal/user/user.go`**

```go
// Package user is the example module: repository → service → handler.
package user

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}
```

- [ ] **Step 2: Write `internal/user/repository.go`**

```go
package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go-backend-template/internal/apperr"
	"go-backend-template/internal/store"
)

// Repository persists users. Implementations return apperr.ErrNotFound and
// apperr.ErrConflict (wrapped) for missing rows and duplicate emails.
type Repository interface {
	Create(ctx context.Context, email, passwordHash string) (User, error)
	GetByID(ctx context.Context, id uuid.UUID) (User, error)
	GetByEmail(ctx context.Context, email string) (User, error)
}

type PostgresRepository struct {
	q *store.Queries
}

func NewPostgresRepository(db store.DBTX) *PostgresRepository {
	return &PostgresRepository{q: store.New(db)}
}

func (r *PostgresRepository) Create(ctx context.Context, email, passwordHash string) (User, error) {
	row, err := r.q.CreateUser(ctx, store.CreateUserParams{Email: email, PasswordHash: passwordHash})
	if err != nil {
		return User{}, mapErr(err)
	}
	return fromRow(row), nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (User, error) {
	row, err := r.q.GetUserByID(ctx, id)
	if err != nil {
		return User{}, mapErr(err)
	}
	return fromRow(row), nil
}

func (r *PostgresRepository) GetByEmail(ctx context.Context, email string) (User, error) {
	row, err := r.q.GetUserByEmail(ctx, email)
	if err != nil {
		return User{}, mapErr(err)
	}
	return fromRow(row), nil
}

const uniqueViolation = "23505"

func mapErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("user %w", apperr.ErrNotFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolation {
		return fmt.Errorf("email %w", apperr.ErrConflict)
	}
	return fmt.Errorf("user repository: %w", err)
}

func fromRow(row store.User) User {
	return User{
		ID:           row.ID,
		Email:        row.Email,
		PasswordHash: row.PasswordHash,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}
```

- [ ] **Step 3: Write `internal/user/usertest/memory.go`**

```go
// Package usertest provides an in-memory user.Repository for tests.
package usertest

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"go-backend-template/internal/apperr"
	"go-backend-template/internal/user"
)

type MemoryRepository struct {
	mu    sync.Mutex
	users map[uuid.UUID]user.User
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{users: map[uuid.UUID]user.User{}}
}

func (r *MemoryRepository) Create(_ context.Context, email, passwordHash string) (user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if strings.EqualFold(u.Email, email) {
			return user.User{}, fmt.Errorf("email %w", apperr.ErrConflict)
		}
	}
	now := time.Now().UTC()
	u := user.User{ID: uuid.New(), Email: email, PasswordHash: passwordHash, CreatedAt: now, UpdatedAt: now}
	r.users[u.ID] = u
	return u, nil
}

func (r *MemoryRepository) GetByID(_ context.Context, id uuid.UUID) (user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.users[id]
	if !ok {
		return user.User{}, fmt.Errorf("user %w", apperr.ErrNotFound)
	}
	return u, nil
}

func (r *MemoryRepository) GetByEmail(_ context.Context, email string) (user.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, u := range r.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return user.User{}, fmt.Errorf("user %w", apperr.ErrNotFound)
}

// Delete removes a user (test-only, e.g. to simulate a deleted account).
func (r *MemoryRepository) Delete(id uuid.UUID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.users, id)
}
```

- [ ] **Step 4: Write the failing service test `internal/user/service_test.go`**

```go
package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"go-backend-template/internal/apperr"
	"go-backend-template/internal/user"
	"go-backend-template/internal/user/usertest"
)

func TestServiceCreateNormalizesEmail(t *testing.T) {
	svc := user.NewService(usertest.NewMemoryRepository())
	u, err := svc.Create(context.Background(), "  Alice@Example.COM ", "hash")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if u.Email != "alice@example.com" {
		t.Fatalf("email = %q", u.Email)
	}
	got, err := svc.GetByEmail(context.Background(), "ALICE@example.com")
	if err != nil || got.ID != u.ID {
		t.Fatalf("GetByEmail = %v, %v", got, err)
	}
}

func TestServiceCreateDuplicate(t *testing.T) {
	svc := user.NewService(usertest.NewMemoryRepository())
	ctx := context.Background()
	if _, err := svc.Create(ctx, "a@b.com", "h"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, "A@B.com", "h"); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

func TestServiceGetByIDNotFound(t *testing.T) {
	svc := user.NewService(usertest.NewMemoryRepository())
	if _, err := svc.GetByID(context.Background(), uuid.New()); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 5: Run test to verify it fails**

Run: `go test ./internal/user/...`
Expected: FAIL — `undefined: user.NewService`.

- [ ] **Step 6: Write `internal/user/service.go`**

```go
package user

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// Create stores a new user. passwordHash must already be hashed.
func (s *Service) Create(ctx context.Context, email, passwordHash string) (User, error) {
	return s.repo.Create(ctx, normalizeEmail(email), passwordHash)
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (User, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) GetByEmail(ctx context.Context, email string) (User, error) {
	return s.repo.GetByEmail(ctx, normalizeEmail(email))
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
```

- [ ] **Step 7: Run service tests**

Run: `go test ./internal/user/...`
Expected: `ok`

- [ ] **Step 8: Write the failing handler test `internal/user/handler_test.go`**

The test injects a fake auth middleware — proving `user` does not depend on `auth`.

```go
package user_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"go-backend-template/internal/httpx"
	"go-backend-template/internal/httpx/httpxtest"
	"go-backend-template/internal/user"
	"go-backend-template/internal/user/usertest"
)

func newRouter(svc *user.Service, mw gin.HandlerFunc) *gin.Engine {
	r := gin.New()
	user.RegisterRoutes(r.Group("/api/v1"), svc, mw)
	return r
}

func TestMe(t *testing.T) {
	repo := usertest.NewMemoryRepository()
	svc := user.NewService(repo)
	u, err := svc.Create(context.Background(), "me@example.com", "secret-hash")
	if err != nil {
		t.Fatal(err)
	}
	asUser := func(id uuid.UUID) gin.HandlerFunc {
		return func(c *gin.Context) { httpx.SetUserID(c, id) }
	}

	t.Run("ok", func(t *testing.T) {
		rec := httpxtest.Do(t, newRouter(svc, asUser(u.ID)), http.MethodGet, "/api/v1/users/me", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
		}
		body := httpxtest.DecodeJSON[map[string]map[string]any](t, rec)
		if body["user"]["email"] != "me@example.com" {
			t.Fatalf("body = %v", body)
		}
		if _, leaked := body["user"]["password_hash"]; leaked {
			t.Fatal("password_hash leaked")
		}
	})

	t.Run("no user id in context", func(t *testing.T) {
		noop := func(c *gin.Context) {}
		rec := httpxtest.Do(t, newRouter(svc, noop), http.MethodGet, "/api/v1/users/me", "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("user deleted", func(t *testing.T) {
		ghost := uuid.New()
		rec := httpxtest.Do(t, newRouter(svc, asUser(ghost)), http.MethodGet, "/api/v1/users/me", "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}
```

- [ ] **Step 9: Run test to verify it fails**

Run: `GIN_MODE=test go test ./internal/user/...`
Expected: FAIL — `undefined: user.RegisterRoutes`.

- [ ] **Step 10: Write `internal/user/handler.go`**

```go
package user

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"go-backend-template/internal/apperr"
	"go-backend-template/internal/httpx"
)

type handler struct {
	svc *Service
}

// RegisterRoutes mounts the user routes. requireAuth is injected by the
// caller so this package does not depend on the auth package.
func RegisterRoutes(rg *gin.RouterGroup, svc *Service, requireAuth gin.HandlerFunc) {
	h := &handler{svc: svc}
	g := rg.Group("/users", requireAuth)
	g.GET("/me", h.me)
}

func (h *handler) me(c *gin.Context) {
	id, ok := httpx.UserID(c)
	if !ok {
		httpx.WriteError(c, apperr.ErrUnauthorized)
		return
	}
	u, err := h.svc.GetByID(c.Request.Context(), id)
	if errors.Is(err, apperr.ErrNotFound) {
		// Valid token for a user that no longer exists.
		httpx.WriteError(c, fmt.Errorf("%w: user no longer exists", apperr.ErrUnauthorized))
		return
	}
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": u})
}
```

- [ ] **Step 11: Run unit tests**

Run: `go mod tidy && GIN_MODE=test go test ./internal/user/...`
Expected: `ok`

- [ ] **Step 12: Write repository integration test `internal/user/repository_integration_test.go`**

```go
//go:build integration

package user_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"go-backend-template/internal/apperr"
	"go-backend-template/internal/testutil"
	"go-backend-template/internal/user"
)

func TestPostgresRepository(t *testing.T) {
	repo := user.NewPostgresRepository(testutil.NewPostgres(t))
	ctx := context.Background()

	created, err := repo.Create(ctx, "bob@example.com", "hash")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == uuid.Nil || created.CreatedAt.IsZero() {
		t.Fatalf("defaults not populated: %+v", created)
	}

	byID, err := repo.GetByID(ctx, created.ID)
	if err != nil || byID.Email != "bob@example.com" {
		t.Fatalf("GetByID = %+v, %v", byID, err)
	}

	byEmail, err := repo.GetByEmail(ctx, "BOB@example.com") // citext
	if err != nil || byEmail.ID != created.ID {
		t.Fatalf("GetByEmail = %+v, %v", byEmail, err)
	}

	if _, err := repo.Create(ctx, "Bob@Example.com", "hash"); !errors.Is(err, apperr.ErrConflict) {
		t.Fatalf("duplicate err = %v, want ErrConflict", err)
	}

	if _, err := repo.GetByID(ctx, uuid.New()); !errors.Is(err, apperr.ErrNotFound) {
		t.Fatalf("missing err = %v, want ErrNotFound", err)
	}
}
```

- [ ] **Step 13: Run integration tests**

Run: `go test -tags integration ./internal/user/...`
Expected: `ok`

- [ ] **Step 14: Commit**

```bash
git add go.mod go.sum internal/user
git commit -m "feat: add user module (repo, service, /users/me) with tests"
```

---

### Task 7: auth — password hashing and JWT tokens

**Files:**
- Create: `internal/auth/password.go`, `internal/auth/token.go`
- Test: `internal/auth/password_test.go`, `internal/auth/token_test.go`

- [ ] **Step 1: Add dependencies**

Run: `go get github.com/golang-jwt/jwt/v5 golang.org/x/crypto`

- [ ] **Step 2: Write the failing tests**

`internal/auth/password_test.go`:

```go
package auth

import (
	"errors"
	"strings"
	"testing"

	"go-backend-template/internal/apperr"
)

func TestPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "correct horse") {
		t.Fatal("correct password rejected")
	}
	if CheckPassword(hash, "wrong horse") {
		t.Fatal("wrong password accepted")
	}
}

func TestHashPasswordTooLong(t *testing.T) {
	// 25 runes × 3 bytes = 75 bytes: passes a rune-count max=72 but exceeds bcrypt's byte limit.
	_, err := HashPassword(strings.Repeat("ữ", 25))
	if !errors.Is(err, apperr.ErrValidation) {
		t.Fatalf("err = %v, want ErrValidation", err)
	}
}
```

`internal/auth/token_test.go`:

```go
package auth

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"go-backend-template/internal/apperr"
)

var secret = strings.Repeat("k", 32)

func TestTokenRoundTrip(t *testing.T) {
	ts := NewTokenService(secret, 15*time.Minute)
	id := uuid.New()
	tok, err := ts.Issue(id)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ts.Verify(tok)
	if err != nil || got != id {
		t.Fatalf("Verify = %v, %v; want %v", got, err, id)
	}
	if ts.TTL() != 15*time.Minute {
		t.Fatalf("TTL = %v", ts.TTL())
	}
}

func TestTokenRejected(t *testing.T) {
	ts := NewTokenService(secret, 15*time.Minute)
	id := uuid.New()

	expired := NewTokenService(secret, time.Minute)
	expired.now = func() time.Time { return time.Now().Add(-time.Hour) }
	expiredTok, _ := expired.Issue(id)

	otherSecretTok, _ := NewTokenService(strings.Repeat("x", 32), time.Minute).Issue(id)

	sign := func(m jwt.SigningMethod, sub string, key any) string {
		s, err := jwt.NewWithClaims(m, jwt.RegisteredClaims{
			Subject:   sub,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
		}).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	cases := map[string]string{
		"garbage":       "not-a-token",
		"expired":       expiredTok,
		"wrong secret":  otherSecretTok,
		"HS512":         sign(jwt.SigningMethodHS512, id.String(), []byte(secret)),
		"alg none":      sign(jwt.SigningMethodNone, id.String(), jwt.UnsafeAllowNoneSignatureType),
		"bad subject":   sign(jwt.SigningMethodHS256, "not-a-uuid", []byte(secret)),
		"empty subject": sign(jwt.SigningMethodHS256, "", []byte(secret)),
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := ts.Verify(tok); !errors.Is(err, apperr.ErrUnauthorized) {
				t.Fatalf("err = %v, want ErrUnauthorized", err)
			}
		})
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/auth/`
Expected: FAIL — `undefined: HashPassword`, `undefined: NewTokenService`.

- [ ] **Step 4: Write `internal/auth/password.go`**

```go
package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"go-backend-template/internal/apperr"
)

const maxPasswordBytes = 72 // bcrypt limit

// HashPassword returns a bcrypt hash of pw.
func HashPassword(pw string) (string, error) {
	if len(pw) > maxPasswordBytes {
		return "", fmt.Errorf("%w: password longer than %d bytes", apperr.ErrValidation, maxPasswordBytes)
	}
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(b), nil
}

// CheckPassword reports whether pw matches hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}
```

- [ ] **Step 5: Write `internal/auth/token.go`**

```go
// Package auth issues and verifies JWT access tokens and serves the
// register/login endpoints.
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"go-backend-template/internal/apperr"
)

// TokenService issues and verifies HS256 access tokens.
type TokenService struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func NewTokenService(secret string, ttl time.Duration) *TokenService {
	return &TokenService{secret: []byte(secret), ttl: ttl, now: time.Now}
}

func (s *TokenService) TTL() time.Duration { return s.ttl }

// Issue returns a signed token whose subject is userID.
func (s *TokenService) Issue(userID uuid.UUID) (string, error) {
	now := s.now()
	claims := jwt.RegisteredClaims{
		Subject:   userID.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return "", fmt.Errorf("sign token: %w", err)
	}
	return tok, nil
}

// Verify checks signature, algorithm and expiry and returns the subject.
func (s *TokenService) Verify(token string) (uuid.UUID, error) {
	var claims jwt.RegisteredClaims
	_, err := jwt.ParseWithClaims(token, &claims,
		func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.now),
	)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: invalid token", apperr.ErrUnauthorized)
	}
	id, err := uuid.Parse(claims.Subject)
	if err != nil || id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("%w: invalid token subject", apperr.ErrUnauthorized)
	}
	return id, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go mod tidy && go test ./internal/auth/`
Expected: `ok`

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/auth
git commit -m "feat: add bcrypt password helpers and HS256 token service"
```

---

### Task 8: auth — middleware and register/login handlers

**Files:**
- Create: `internal/auth/middleware.go`, `internal/auth/handler.go`
- Test: `internal/auth/handler_test.go`

- [ ] **Step 1: Write the failing test `internal/auth/handler_test.go`**

This wires auth + user exactly like `main` will, so it also covers `RequireAuth` on `/users/me`.

```go
package auth_test

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"go-backend-template/internal/auth"
	"go-backend-template/internal/httpx/httpxtest"
	"go-backend-template/internal/user"
	"go-backend-template/internal/user/usertest"
)

type env struct {
	router *gin.Engine
	repo   *usertest.MemoryRepository
	tokens *auth.TokenService
}

func newEnv() env {
	repo := usertest.NewMemoryRepository()
	users := user.NewService(repo)
	tokens := auth.NewTokenService(strings.Repeat("s", 32), 15*time.Minute)
	r := gin.New()
	api := r.Group("/api/v1")
	auth.RegisterRoutes(api, users, tokens)
	user.RegisterRoutes(api, users, auth.RequireAuth(tokens))
	return env{router: r, repo: repo, tokens: tokens}
}

const creds = `{"email":"a@example.com","password":"password123"}`

type loginResp struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func login(t *testing.T, e env, body string) *loginResp {
	t.Helper()
	rec := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/login", body, "")
	if rec.Code != http.StatusOK {
		return nil
	}
	r := httpxtest.DecodeJSON[loginResp](t, rec)
	return &r
}

func TestRegister(t *testing.T) {
	e := newEnv()

	rec := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/register", creds, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
	body := httpxtest.DecodeJSON[map[string]map[string]any](t, rec)
	if body["user"]["email"] != "a@example.com" {
		t.Fatalf("body = %v", body)
	}
	if _, leaked := body["user"]["password_hash"]; leaked {
		t.Fatal("password_hash leaked")
	}

	dup := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/register", `{"email":"A@example.com","password":"password123"}`, "")
	if dup.Code != http.StatusConflict {
		t.Fatalf("duplicate status = %d", dup.Code)
	}

	for name, bad := range map[string]string{
		"bad email":      `{"email":"nope","password":"password123"}`,
		"short password": `{"email":"b@example.com","password":"short"}`,
		"missing fields": `{}`,
		"too many bytes": `{"email":"c@example.com","password":"` + strings.Repeat("ữ", 25) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/register", bad, "")
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestLoginAndMe(t *testing.T) {
	e := newEnv()
	if rec := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/register", creds, ""); rec.Code != http.StatusCreated {
		t.Fatalf("register: %d", rec.Code)
	}

	lr := login(t, e, creds)
	if lr == nil {
		t.Fatal("login failed")
	}
	if lr.TokenType != "Bearer" || lr.ExpiresIn != 900 || lr.AccessToken == "" {
		t.Fatalf("login resp = %+v", lr)
	}

	if login(t, e, `{"email":"a@example.com","password":"wrong-password"}`) != nil {
		t.Fatal("wrong password accepted")
	}
	rec := httpxtest.Do(t, e.router, http.MethodPost, "/api/v1/auth/login", `{"email":"nobody@example.com","password":"password123"}`, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown email status = %d", rec.Code)
	}

	me := httpxtest.Do(t, e.router, http.MethodGet, "/api/v1/users/me", "", lr.AccessToken)
	if me.Code != http.StatusOK {
		t.Fatalf("me status = %d body = %s", me.Code, me.Body)
	}

	for name, tok := range map[string]string{"no token": "", "bad token": "abc.def.ghi"} {
		t.Run(name, func(t *testing.T) {
			rec := httpxtest.Do(t, e.router, http.MethodGet, "/api/v1/users/me", "", tok)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d", rec.Code)
			}
		})
	}

	t.Run("deleted user", func(t *testing.T) {
		id, err := e.tokens.Verify(lr.AccessToken)
		if err != nil {
			t.Fatal(err)
		}
		e.repo.Delete(id)
		rec := httpxtest.Do(t, e.router, http.MethodGet, "/api/v1/users/me", "", lr.AccessToken)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `GIN_MODE=test go test ./internal/auth/`
Expected: FAIL — `undefined: auth.RegisterRoutes`, `undefined: auth.RequireAuth`.

- [ ] **Step 3: Write `internal/auth/middleware.go`**

```go
package auth

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"go-backend-template/internal/apperr"
	"go-backend-template/internal/httpx"
)

// RequireAuth rejects requests without a valid bearer token and stores the
// caller's ID via httpx.SetUserID.
func RequireAuth(tokens *TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, ok := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !ok || raw == "" {
			httpx.WriteError(c, fmt.Errorf("%w: missing bearer token", apperr.ErrUnauthorized))
			return
		}
		id, err := tokens.Verify(raw)
		if err != nil {
			httpx.WriteError(c, err)
			return
		}
		httpx.SetUserID(c, id)
		c.Next()
	}
}
```

- [ ] **Step 4: Write `internal/auth/handler.go`**

```go
package auth

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"go-backend-template/internal/apperr"
	"go-backend-template/internal/httpx"
	"go-backend-template/internal/user"
)

var errInvalidCredentials = fmt.Errorf("%w: invalid credentials", apperr.ErrUnauthorized)

// dummyHash is compared against when the email is unknown so that login
// takes roughly the same time whether or not the account exists.
var dummyHash, _ = HashPassword("dummy-password-for-timing")

type handler struct {
	users  *user.Service
	tokens *TokenService
}

func RegisterRoutes(rg *gin.RouterGroup, users *user.Service, tokens *TokenService) {
	h := &handler{users: users, tokens: tokens}
	g := rg.Group("/auth")
	g.POST("/register", h.register)
	g.POST("/login", h.login)
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

func (h *handler) register(c *gin.Context) {
	var req registerRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	hash, err := HashPassword(req.Password)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	u, err := h.users.Create(c.Request.Context(), req.Email, hash)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": u})
}

type loginRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required,max=72"`
}

func (h *handler) login(c *gin.Context) {
	var req loginRequest
	if !httpx.BindJSON(c, &req) {
		return
	}
	u, err := h.users.GetByEmail(c.Request.Context(), req.Email)
	if errors.Is(err, apperr.ErrNotFound) {
		CheckPassword(dummyHash, req.Password)
		httpx.WriteError(c, errInvalidCredentials)
		return
	}
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	if !CheckPassword(u.PasswordHash, req.Password) {
		httpx.WriteError(c, errInvalidCredentials)
		return
	}
	tok, err := h.tokens.Issue(u.ID)
	if err != nil {
		httpx.WriteError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"access_token": tok,
		"token_type":   "Bearer",
		"expires_in":   int(h.tokens.TTL().Seconds()),
	})
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `GIN_MODE=test go test ./internal/...`
Expected: all `ok`.

- [ ] **Step 6: Commit**

```bash
git add internal/auth
git commit -m "feat: add RequireAuth middleware and register/login endpoints"
```

---

### Task 9: server — engine, middleware, health, graceful shutdown

**Files:**
- Create: `internal/server/middleware.go`, `internal/server/server.go`
- Test: `internal/server/server_test.go`

- [ ] **Step 1: Add dependency**

Run: `go get github.com/gin-contrib/cors`

- [ ] **Step 2: Write the failing test `internal/server/server_test.go`**

```go
package server_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"go-backend-template/internal/httpx/httpxtest"
	"go-backend-template/internal/server"
)

type pinger struct{ err error }

func (p pinger) Ping(context.Context) error { return p.err }

func newEngine(db server.Pinger, origins ...string) *gin.Engine {
	return server.NewEngine(server.Options{
		Logger:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		CORSOrigins: origins,
		DB:          db,
	})
}

func TestHealth(t *testing.T) {
	if rec := httpxtest.Do(t, newEngine(pinger{}), http.MethodGet, "/healthz", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", rec.Code)
	}
	if rec := httpxtest.Do(t, newEngine(pinger{}), http.MethodGet, "/readyz", "", ""); rec.Code != http.StatusOK {
		t.Fatalf("readyz ok = %d", rec.Code)
	}
	if rec := httpxtest.Do(t, newEngine(pinger{errors.New("down")}), http.MethodGet, "/readyz", "", ""); rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz down = %d", rec.Code)
	}
}

func TestRequestID(t *testing.T) {
	e := newEngine(pinger{})

	rec := httpxtest.Do(t, e, http.MethodGet, "/healthz", "", "")
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("request id not generated")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Request-ID", "abc-123")
	rec = httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if got := rec.Header().Get("X-Request-ID"); got != "abc-123" {
		t.Fatalf("request id = %q, want passthrough", got)
	}
}

func TestPanicRecovered(t *testing.T) {
	e := newEngine(pinger{})
	e.GET("/boom", func(*gin.Context) { panic("boom") })
	rec := httpxtest.Do(t, e, http.MethodGet, "/boom", "", "")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	if code := httpxtest.DecodeJSON[httpxtest.ErrorBody](t, rec).Error.Code; code != "internal_error" {
		t.Fatalf("code = %q", code)
	}
}

func TestCORS(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "http://app.local")

	rec := httptest.NewRecorder()
	newEngine(pinger{}, "http://app.local").ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://app.local" {
		t.Fatalf("allow-origin = %q", got)
	}

	rec = httptest.NewRecorder()
	newEngine(pinger{}).ServeHTTP(rec, req)
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("CORS disabled but allow-origin = %q", got)
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Run(ctx, "127.0.0.1:0", http.NotFoundHandler(), time.Second) }()

	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunListenError(t *testing.T) {
	if err := server.Run(context.Background(), "bad::addr::", http.NotFoundHandler(), time.Second); err == nil {
		t.Fatal("expected listen error")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `GIN_MODE=test go test ./internal/server/`
Expected: FAIL — `undefined: server.NewEngine`, `server.Options`, `server.Pinger`, `server.Run`.

- [ ] **Step 4: Write `internal/server/middleware.go`**

```go
package server

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"go-backend-template/internal/httpx"
)

const requestIDHeader = "X-Request-ID"

func recovery() gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, rec any) {
		httpx.WriteError(c, fmt.Errorf("panic: %v", rec))
	})
}

// requestID reuses an incoming X-Request-ID (if sane) or generates one.
func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(requestIDHeader)
		if id == "" || len(id) > 128 {
			id = uuid.NewString()
		}
		c.Set(httpx.RequestIDKey, id)
		c.Header(requestIDHeader, id)
		c.Next()
	}
}

func accessLog(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.LogAttrs(c.Request.Context(), slog.LevelInfo, "http request",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Duration("latency", time.Since(start)),
			slog.String("request_id", c.GetString(httpx.RequestIDKey)),
		)
	}
}
```

Note: `gin.CustomRecoveryWithWriter(nil, …)` disables Gin's own stderr dump; `httpx.WriteError` logs the panic via slog. If the installed Gin version panics on a nil writer, use `io.Discard` instead.

- [ ] **Step 5: Write `internal/server/server.go`**

```go
// Package server builds the HTTP engine and runs it with graceful shutdown.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// Pinger is satisfied by *pgxpool.Pool.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Options struct {
	Logger      *slog.Logger
	CORSOrigins []string // empty disables CORS
	DB          Pinger
}

// NewEngine returns a Gin engine with middleware and health routes. Callers
// mount module routes on it (e.g. engine.Group("/api/v1")).
func NewEngine(opts Options) *gin.Engine {
	r := gin.New()
	r.Use(recovery(), requestID(), accessLog(opts.Logger))
	if len(opts.CORSOrigins) > 0 {
		r.Use(cors.New(cors.Config{
			AllowOrigins:  opts.CORSOrigins,
			AllowMethods:  []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:  []string{"Authorization", "Content-Type", requestIDHeader},
			ExposeHeaders: []string{requestIDHeader},
			MaxAge:        12 * time.Hour,
		}))
	}
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/readyz", readyz(opts.DB))
	return r
}

func readyz(db Pinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	}
}

// Run serves h on addr until ctx is cancelled, then shuts down gracefully,
// waiting at most shutdownTimeout for in-flight requests.
func Run(ctx context.Context, addr string, h http.Handler, shutdownTimeout time.Duration) error {
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}

	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go mod tidy && GIN_MODE=test go test ./internal/server/`
Expected: `ok`

- [ ] **Step 7: Commit**

```bash
git add go.mod go.sum internal/server
git commit -m "feat: add server engine, middleware, health checks, graceful shutdown"
```

---

### Task 10: main wiring, Docker, lint, Makefile, README, smoke test

**Files:**
- Create: `cmd/api/main.go`, `Dockerfile`, `.dockerignore`, `.golangci.yml`, `Makefile`, `README.md`

- [ ] **Step 1: Write `cmd/api/main.go`**

```go
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"

	"go-backend-template/internal/auth"
	"go-backend-template/internal/config"
	"go-backend-template/internal/database"
	"go-backend-template/internal/server"
	"go-backend-template/internal/user"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.Env)
	slog.SetDefault(logger)
	if cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := database.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		return err
	}

	users := user.NewService(user.NewPostgresRepository(pool))
	tokens := auth.NewTokenService(cfg.JWTSecret, cfg.JWTTTL)

	engine := server.NewEngine(server.Options{Logger: logger, CORSOrigins: cfg.CORSOrigins, DB: pool})
	api := engine.Group("/api/v1")
	auth.RegisterRoutes(api, users, tokens)
	user.RegisterRoutes(api, users, auth.RequireAuth(tokens))

	logger.Info("listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
	return server.Run(ctx, cfg.HTTPAddr, engine, cfg.ShutdownTimeout)
}

func newLogger(env string) *slog.Logger {
	if env == "prod" {
		return slog.New(slog.NewJSONHandler(os.Stdout, nil))
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
}
```

- [ ] **Step 2: Build**

Run: `go build ./... && go vet ./...`
Expected: no output.

- [ ] **Step 3: Write `Dockerfile`**

Set `GO_VERSION` to the major.minor of `go env GOVERSION` (e.g. `1.27`).

```dockerfile
ARG GO_VERSION=1.27
FROM golang:${GO_VERSION}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/api /api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
```

- [ ] **Step 4: Write `.dockerignore`**

```
.git
bin
.env
docs
*.out
```

- [ ] **Step 5: Write `.golangci.yml`**

```yaml
version: "2"
linters:
  enable:
    - bodyclose
    - errorlint
    - gosec
    - misspell
    - unconvert
  exclusions:
    paths:
      - internal/store
formatters:
  enable:
    - gofmt
    - goimports
```

- [ ] **Step 6: Pin golangci-lint version**

Run: `gh release view -R golangci/golangci-lint --json tagName -q .tagName`
Use that tag (e.g. `v2.x.y`) for `GOLANGCI_LINT_VERSION` in the Makefile below. It must be built with a Go ≥ the `go` directive in `go.mod`; if `make lint` errors with "Go language version … is higher", pick the newest release whose notes mention the current Go version.

- [ ] **Step 7: Write `Makefile`** (recipe lines are TAB-indented)

```make
GOLANGCI_LINT_VERSION ?= v2.x.y
LOAD_ENV = set -a; [ -f .env ] && . ./.env; set +a;

.PHONY: run build test test-integration lint sqlc migrate-up migrate-down migrate-new docker-up docker-down

run:
	@$(LOAD_ENV) go run ./cmd/api

build:
	CGO_ENABLED=0 go build -trimpath -o bin/api ./cmd/api

test:
	GIN_MODE=test go test -race ./...

test-integration:
	GIN_MODE=test go test -race -tags integration ./...

lint:
	docker run --rm -v $(CURDIR):/app -v $(shell go env GOMODCACHE):/go/pkg/mod -w /app golangci/golangci-lint:$(GOLANGCI_LINT_VERSION) golangci-lint run

sqlc:
	go tool sqlc generate

migrate-up:
	@$(LOAD_ENV) go tool goose -dir migrations postgres "$$DATABASE_URL" up

migrate-down:
	@$(LOAD_ENV) go tool goose -dir migrations postgres "$$DATABASE_URL" down

migrate-new:
	@test -n "$(name)" || (echo "usage: make migrate-new name=add_things" && exit 1)
	go tool goose -dir migrations -s create $(name) sql

docker-up:
	docker compose up -d postgres

docker-down:
	docker compose down
```

- [ ] **Step 8: Run full test suites and lint**

```bash
make test
make test-integration
make lint
```

Expected: tests `ok` everywhere; lint `0 issues.` Fix any lint findings in the source (do not blanket-disable linters). If gosec flags `dummy-password-for-timing` as G101, add `//nolint:gosec // not a credential, timing equalizer` on that line.

- [ ] **Step 9: Smoke test against compose Postgres**

```bash
cp .env.example .env
make docker-up
(make run > /tmp/claude-api.log 2>&1 &) ; sleep 4
curl -s localhost:8080/readyz
curl -s -X POST localhost:8080/api/v1/auth/register -H 'Content-Type: application/json' -d '{"email":"smoke@example.com","password":"password123"}'
TOKEN=$(curl -s -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' -d '{"email":"smoke@example.com","password":"password123"}' | sed -E 's/.*"access_token":"([^"]+)".*/\1/')
curl -s localhost:8080/api/v1/users/me -H "Authorization: Bearer $TOKEN"
pkill -INT -f 'go-build.*/api' ; pkill -INT -f 'go run ./cmd/api'
make docker-down
```

Expected: `{"status":"ok"}`, a `{"user":{...}}` 201 body, then `{"user":{"email":"smoke@example.com",...}}`. If port 5432 is taken, set `POSTGRES_PORT=5433` and use that port in `.env`'s `DATABASE_URL`.

- [ ] **Step 10: Docker image build check**

Run: `docker build -t go-backend-template:dev . && docker image ls go-backend-template:dev`
Expected: build succeeds; image listed.

- [ ] **Step 11: Write `README.md`**

````markdown
# go-backend-template

Reusable Go REST backend: Gin, Postgres (pgx + sqlc + goose), JWT auth, slog, Docker.

## Quick start

```bash
cp .env.example .env
make docker-up     # Postgres on :5432
make run           # API on :8080, migrations run on startup
```

```bash
curl -X POST localhost:8080/api/v1/auth/register -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","password":"password123"}'
curl -X POST localhost:8080/api/v1/auth/login -H 'Content-Type: application/json' \
  -d '{"email":"me@example.com","password":"password123"}'
curl localhost:8080/api/v1/users/me -H "Authorization: Bearer <access_token>"
```

## Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | /healthz | – | liveness |
| GET | /readyz | – | DB ping |
| POST | /api/v1/auth/register | – | create account |
| POST | /api/v1/auth/login | – | get access token |
| GET | /api/v1/users/me | Bearer | current user |

Errors: `{"error":{"code":"not_found|conflict|unauthorized|validation_error|internal_error","message":"..."}}`

## Config (env)

`APP_ENV` (dev|prod), `HTTP_ADDR`, `DATABASE_URL`*, `JWT_SECRET`* (≥32 bytes), `JWT_TTL`, `SHUTDOWN_TIMEOUT`, `CORS_ORIGINS` (comma list, empty = CORS off). * required.

## Layout

```
cmd/api            wiring only
internal/apperr    sentinel errors (domain packages import this, not HTTP)
internal/httpx     error → HTTP mapping, bind, user-ID context
internal/config    env loader
internal/database  pgx pool + goose migrate
internal/store     sqlc-generated (do not edit)
internal/auth      JWT, bcrypt, RequireAuth, register/login
internal/user      example module: repository → service → handler
internal/server    engine, middleware, health, graceful shutdown
migrations/        goose SQL (embedded)
queries/           sqlc SQL
```

## Adding a module

1. Migration: `make migrate-new name=create_things`, write SQL under `-- +goose Up/Down`.
2. Queries in `queries/things.sql`, then `make sqlc`.
3. `internal/thing/`: `Repository` interface + Postgres impl (map `pgx.ErrNoRows` → `apperr.ErrNotFound`), `Service`, `RegisterRoutes(rg, svc, requireAuth)`.
4. Wire it in `cmd/api/main.go`.
5. Unit-test the service with an in-memory repo; integration-test the repo with `testutil.NewPostgres` (`//go:build integration`).

## Make targets

`run build test test-integration lint sqlc migrate-up migrate-down migrate-new docker-up docker-down`

Integration tests need Docker (testcontainers).
````

- [ ] **Step 12: Commit**

```bash
git add cmd Dockerfile .dockerignore .golangci.yml Makefile README.md go.mod go.sum
git commit -m "feat: wire main, add Dockerfile, lint config, Makefile and README"
```
