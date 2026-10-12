package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// BindJSON binds and validates the JSON body into dst. On failure it writes a
// 400 validation_error response (413 payload_too_large if the body exceeds the server limit) and returns false.
func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			abort(c, http.StatusRequestEntityTooLarge, "payload_too_large", "request body too large")
			return false
		}
		abort(c, http.StatusBadRequest, "validation_error", err.Error())
		return false
	}
	return true
}
