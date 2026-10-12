package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/Hhh22121/go-backend-template/internal/auth"
	"github.com/Hhh22121/go-backend-template/internal/config"
	"github.com/Hhh22121/go-backend-template/internal/database"
	"github.com/Hhh22121/go-backend-template/internal/server"
	"github.com/Hhh22121/go-backend-template/internal/user"
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
