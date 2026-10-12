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
