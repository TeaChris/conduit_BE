package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// Recovery returns middleware that recovers from panics and logs the error.
func Recovery(logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logger.Error().
					Interface("panic", r).
					Str("stack", string(debug.Stack())).
					Str("path", c.Request.URL.Path).
					Str("method", c.Request.Method).
					Msg("recovered from panic")

				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"error": gin.H{
						"code":    "INTERNAL_ERROR",
						"message": "An internal error occurred. Please try again later.",
					},
				})
			}
		}()
		c.Next()
	}
}
