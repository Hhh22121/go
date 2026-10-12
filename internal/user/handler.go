package user

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Hhh22121/go-backend-template/internal/apperr"
	"github.com/Hhh22121/go-backend-template/internal/httpx"
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
