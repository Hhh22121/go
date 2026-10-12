package httpx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BindJSON binds and validates the JSON body into dst. On failure it writes a
// 400 validation_error response and returns false.
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		abort(c, http.StatusBadRequest, "validation_error", err.Error())
		return false
	}
	return true
}
