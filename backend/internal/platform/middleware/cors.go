package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/conduit-platform/conduit/backend/internal/config"
)

// CORS returns middleware that handles Cross-Origin Resource Sharing.
func CORS(cfg config.SecurityConfig) gin.HandlerFunc {
	allowedOrigins := strings.Join(cfg.CORSAllowedOrigins, ",")
	allowedMethods := strings.Join(cfg.CORSAllowedMethods, ",")
	allowedHeaders := strings.Join(cfg.CORSAllowedHeaders, ",")
	maxAge := strconv.Itoa(cfg.CORSMaxAge)

	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", allowedOrigins)
		c.Header("Access-Control-Allow-Methods", allowedMethods)
		c.Header("Access-Control-Allow-Headers", allowedHeaders)
		c.Header("Access-Control-Max-Age", maxAge)
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
