package auth

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Hhh22121/go/internal/apperr"
	"github.com/Hhh22121/go/internal/httpx"
)

// RequireAuth rejects requests without a valid bearer token and stores the
// caller's ID via httpx.SetUserID.
func RequireAuth(tokens *TokenService) gin.HandlerFunc {
	return func(c *gin.Context) {
		scheme, raw, ok := strings.Cut(c.GetHeader("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || raw == "" {
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
