package server

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/Hhh22121/go-backend-template/internal/httpx"
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
