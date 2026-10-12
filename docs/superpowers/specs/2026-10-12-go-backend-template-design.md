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
| Auth           | JWT `golang-jwt/jwt/v5` (HS256, access only) + bcrypt |
| Validation     | Gin binding (`go-playground/validator`)            |
| Tests          | `testing` + `httptest`; `testcontainers-go` for DB |
| Lint           | golangci-lint                                      |
| Dev env        | Docker Compose (Postgres), Makefile                |
| Tool pinning   | sqlc, goose, golangci-lint via `tool` directives in go.mod |

Module path: `github.com/Hhh22121/go-backend-template`.

## Layout (feature/module-based)

```
cmd/api/main.go        # wiring only: config → db → migrate → modules → server
internal/
  config/              # Load() reads env, validates required vars, returns Config
  server/              # Gin engine, middleware, route registration, graceful shutdown
  database/            # pgxpool constructor, goose migrate-up using embedded migrations
  store/               # sqlc-generated code (do not edit)
  apperr/              # shared sentinel errors (no HTTP dependency)
  httpx/               # apperr → HTTP mapping, JSON error envelope, user-ID context helpers
  auth/                # token service, password hashing, RequireAuth middleware, handlers
  user/                # example module: repo → service → handler
migrations/            # goose SQL migrations + embed.go (`package migrations; var FS embed.FS`)
queries/               # sqlc query files
sqlc.yaml  docker-compose.yml  Dockerfile  Makefile
.golangci.yml  .env.example  README.md
```

Each module owns `handler.go`, `service.go`, `repository.go` and exposes a
`RegisterRoutes(rg *gin.RouterGroup, ...)` function. Services depend on a
repository **interface** declared in the same package so they can be unit
tested with fakes.

### Package dependencies (no cycles)

```
apperr  ←  httpx  ←  user  ←  auth
                      ↑         ↑
store ────────────────┘         │
cmd/api ── wires everything ────┘
```

- `user` must **not** import `auth`. `user.RegisterRoutes(rg, svc,
  requireAuth gin.HandlerFunc)` receives the middleware from `main`, and reads
  the caller with `httpx.UserID(c)`.
- `auth` imports `user` (uses `user.Service` for create/lookup) and sets the
  ID via `httpx.SetUserID(c, id)` in `RequireAuth`.
- IDs are `github.com/google/uuid.UUID` everywhere (sqlc override).

## Components

### config
Fields: `Env` (dev|prod), `HTTPAddr` (default `:8080`), `DatabaseURL`
(required), `JWTSecret` (required, ≥32 bytes), `JWTTTL` (default `15m`),
`ShutdownTimeout` (default `10s`), `CORSOrigins` (comma list; empty → CORS
middleware not installed). `Load()`
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
`NewPool(ctx, url)` with ping; `Migrate(ctx, pool)` opens a `*sql.DB` via
`stdlib.OpenDBFromPool(pool)`, sets `goose.SetBaseFS(migrations.FS)` +
`SetDialect("postgres")` and runs up. Initial migration:

```sql
CREATE EXTENSION IF NOT EXISTS citext;
CREATE TABLE users (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email         citext UNIQUE NOT NULL,
  password_hash text NOT NULL,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
```

`updated_at` is set explicitly by any UPDATE query (no trigger).

`sqlc.yaml`: engine postgresql, `schema: migrations`, `queries: queries`,
out `internal/store`, `sql_package: pgx/v5`, override `uuid` →
`github.com/google/uuid.UUID`, `citext` → `string`. Makefile goose targets use
`-dir migrations`.

### auth
- `PasswordHasher`: bcrypt hash/compare.
- `TokenService`: `Issue(userID) (token, expiresAt)`, `Verify(token) (userID)`;
  claims `sub`, `iat`, `exp`. `Verify` accepts only HS256
  (`jwt.WithValidMethods`), rejects expired tokens and missing/invalid `sub`.
- `RequireAuth(tokens)` middleware: parses `Authorization: Bearer`, calls
  `httpx.SetUserID`; failures → 401 envelope.
- Register validation: `email` `binding:"required,email"`, `password`
  `binding:"required,min=8,max=72"` (bcrypt 72-byte limit).
- Handlers call `user.Service` for create/lookup. Login returns the same
  `ErrUnauthorized` for unknown email and wrong password.

### user
`Service.Create(ctx, email, passwordHash)`, `GetByID`, `GetByEmail`.
Repository wraps sqlc queries and maps `pgx.ErrNoRows` → `ErrNotFound`,
unique violation (23505) → `ErrConflict`. The `password_hash` is never
serialized.

### apperr / httpx
`apperr` defines sentinels `ErrNotFound`, `ErrConflict`, `ErrUnauthorized`,
`ErrValidation` (domain packages import only this). `httpx.WriteError(c, err)`
maps them to 404/409/401/400, Gin bind errors → 400, anything else → 500
(logged, generic message). `httpx.SetUserID` / `httpx.UserID` store the
authenticated `uuid.UUID` in the Gin context. Envelope:
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
`docker-down`. Dockerfile: multi-stage, `CGO_ENABLED=0` →
`gcr.io/distroless/static:nonroot`. Compose: `postgres:17` with healthcheck + optional `api` service.

## Out of scope (YAGNI)

Refresh tokens, RBAC, Redis, OpenAPI/Swagger, metrics/tracing, rate limiting.
