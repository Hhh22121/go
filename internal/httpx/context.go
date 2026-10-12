package httpx

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const userIDKey = "httpx.user_id"

// SetUserID stores the authenticated user's ID in the Gin context.
func SetUserID(c *gin.Context, id uuid.UUID) {
	c.Set(userIDKey, id)
}

// UserID returns the authenticated user's ID; ok is false when no
// authentication middleware ran for this request.
func UserID(c *gin.Context) (uuid.UUID, bool) {
	v, ok := c.Get(userIDKey)
	if !ok {
		return uuid.Nil, false
	}
	id, ok := v.(uuid.UUID)
	return id, ok && id != uuid.Nil
}
