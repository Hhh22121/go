# go-backend-template

[![CI](https://github.com/Hhh22121/go/actions/workflows/ci.yml/badge.svg)](https://github.com/Hhh22121/go/actions/workflows/ci.yml)

Reusable Go REST backend: Gin, Postgres (pgx + sqlc + goose), JWT auth, slog, Docker.

## Quick start

```bash
cp .env.example .env
make docker-up     # Postgres on :5432
make run           # API on :8080, migrations run on startup
```

Generate a real JWT secret with `openssl rand -base64 48`. To run the whole stack (API + Postgres) in containers: `docker compose --profile api up --build`.

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

## Operational notes

- Migrations run on API startup under a Postgres advisory lock, so concurrent replicas are safe. In prod you may prefer running them separately (`make migrate-up`).
- The `citext` extension is created by a migration; the database user needs `CREATE EXTENSION` privilege (or create it ahead of time as a superuser).
- 4xx error messages are returned to clients verbatim. Do not wrap sensitive context into `apperr` errors.
- The `tool` directives for sqlc/goose in `go.mod` add many indirect dependencies; that is expected.
- Request bodies are capped at 1 MiB (413 `payload_too_large`); `JWT_SECRET` values starting with `change-me` are rejected when `APP_ENV=prod`.

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
