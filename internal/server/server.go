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
