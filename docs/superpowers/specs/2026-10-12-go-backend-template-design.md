# Go Backend Template — Design Spec

Date: 2026-10-12
Status: Approved (brainstorming)

## Goal

A reusable, domain-agnostic Go backend boilerplate. Cloning it should give a
working REST API with auth, Postgres, migrations, tests, Docker and lint, plus
one example module (`user`) that shows the pattern for adding new modules.

## Stack

| Concern        | Choice                                             |
|----------------|----------------------------------------------------|
| Go             | Latest stable (upgrade local toolchain via brew)   |
| HTTP           | Gin                                                |
| DB driver      | pgx v5 (`pgxpool`)                                 |
| Queries        | sqlc (generated, type-safe)                        |
| Migrations     | goose (SQL files, embedded, run on startup)        |
| Config         | Environment variables → typed struct               |
| Logging        | `log/slog` (JSON in prod, text in dev)             |
| Auth           | JWT (HS256, access token only) + bcrypt            |
| Validation     | Gin binding (`go-playground/validator`)            |
| Tests          | `testing` + `httptest`; `testcontainers-go` for DB |
| Lint           | golangci-lint                                      |
| Dev env        | Docker Compose (Postgres), Makefile                |

Module path: `github.com/Hhh22121/go-backend-template`.

## Layout (feature/module-based)

```
cmd/api/main.go        # wiring only: config → db → migrate → modules → server
internal/
  config/              # Load() reads env, validates required vars, returns Config
  server/              # Gin engine, middleware, route registration, graceful shutdown
  database/            # pgxpool constructor, goose migrate-up using embedded migrations
  db/                  # sqlc-generated code (do not edit)
  httpx/               # error → HTTP mapping, JSON error envelope, bind helper
  auth/                # token service, password hashing, RequireAuth middleware, handlers
  user/                # example module: repo → service → handler
migrations/            # goose SQL migrations (embedded via embed.FS)
queries/               # sqlc query files
sqlc.yaml  docker-compose.yml  Dockerfile  Makefile
.golangci.yml  .env.example  README.md
```

Each module owns `handler.go`, `service.go`, `repository.go` and exposes a
`RegisterRoutes(rg *gin.RouterGroup, ...)` function. Services depend on a
repository **interface** declared in the same package so they can be unit
tested with fakes.

## Components

### config
Fields: `Env` (dev|prod), `HTTPAddr` (default `:8080`), `DatabaseURL`
(required), `JWTSecret` (required, ≥32 bytes), `JWTTTL` (default `15m`),
`ShutdownTimeout` (default `10s`), `CORSOrigins` (comma list). `Load()`
returns an error listing all missing/invalid vars.

### server
Middleware order: recover → request-id (`X-Request-ID`, generated if absent)
→ slog access log (method, path, status, latency, request_id) → CORS.
`Run(ctx)` starts `http.Server` and on context cancel (SIGINT/SIGTERM) calls
`Shutdown` with `ShutdownTimeout`.

Routes:
- `GET /healthz` → 200 `{"status":"ok"}` (no dependencies)
- `GET /readyz` → 200 if DB ping succeeds, else 503
- `POST /api/v1/auth/register` `{email,password}` → 201 `{user}`
- `POST /api/v1/auth/login` `{email,password}` → 200 `{access_token,token_type,expires_in}`
- `GET /api/v1/users/me` (auth) → 200 `{user}`

### database
`NewPool(ctx, url)` with ping; `Migrate(ctx, pool)` runs goose up from
embedded `migrations/`. Initial migration: `users(id uuid pk default
gen_random_uuid(), email citext unique not null, password_hash text not null,
created_at timestamptz, updated_at timestamptz)`.

### auth
- `PasswordHasher`: bcrypt hash/compare.
- `TokenService`: `Issue(userID) (token, expiresAt)`, `Verify(token) (userID)`;
  claims `sub`, `iat`, `exp`.
- `RequireAuth(tokens)` middleware: parses `Authorization: Bearer`, puts
  user ID in Gin context; helper `UserID(c)`.
- Handlers call `user.Service` for create/lookup. Login returns the same
  `ErrUnauthorized` for unknown email and wrong password.

### user
`Service.Create(ctx, email, passwordHash)`, `GetByID`, `GetByEmail`.
Repository wraps sqlc queries and maps `pgx.ErrNoRows` → `ErrNotFound`,
unique violation (23505) → `ErrConflict`. The `password_hash` is never
serialized.

### httpx
Sentinel errors: `ErrNotFound`, `ErrConflict`, `ErrUnauthorized`,
`ErrValidation`. `WriteError(c, err)` maps them to 404/409/401/400, anything
else → 500 (logged, generic message). Envelope:
`{"error":{"code":"not_found","message":"..."}}`.

## Error handling

Errors are wrapped with `%w` and classified via `errors.Is` at the HTTP edge
only. Internal errors are logged with request_id; clients never see internal
details.

## Testing

- Unit: config loading, token issue/verify/expiry, hasher, user service with
  fake repo, httpx mapping.
- Handler: `httptest` against a Gin engine with fake services (register,
  login, me, auth middleware rejections).
- Integration (`//go:build integration`): user repository against real
  Postgres via testcontainers-go, including migrations and conflict mapping.
- `make test` runs unit + handler; `make test-integration` adds the tag.

## Tooling

Makefile targets: `run`, `build`, `test`, `test-integration`, `lint`, `sqlc`,
`migrate-up`, `migrate-down`, `migrate-new name=...`, `docker-up`,
`docker-down`. Dockerfile: multi-stage build → `gcr.io/distroless/static`
non-root. Compose: `postgres:17` with healthcheck + optional `api` service.

## Out of scope (YAGNI)

Refresh tokens, RBAC, Redis, OpenAPI/Swagger, metrics/tracing, rate limiting.
