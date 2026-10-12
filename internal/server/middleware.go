package server

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Hhh22121/go/internal/httpx"
)

const requestIDHeader = "X-Request-ID"

// maxBodyBytes caps request bodies (1 MiB).
const maxBodyBytes = 1 << 20

// recovery logs panics with a stack trace and responds with the generic
// internal-error envelope.
func recovery(log *slog.Logger) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(io.Discard, func(c *gin.Context, rec any) {
		log.ErrorContext(c.Request.Context(), "panic recovered",
			"panic", fmt.Sprint(rec),
			"stack", string(debug.Stack()),
			"request_id", c.GetString(httpx.RequestIDKey))
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": gin.H{
			"code": "internal_error", "message": "internal server error",
		}})
	})
}

// bodyLimit makes reads past limit bytes fail with *http.MaxBytesError.
func bodyLimit(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
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
