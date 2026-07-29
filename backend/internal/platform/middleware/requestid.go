package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	// RequestIDHeader is the header key for request ID propagation.
	RequestIDHeader = "X-Request-ID"
	// requestIDKey is the context key for the request ID.
	requestIDKey = "request_id"
)

// RequestID extracts or generates a unique request ID for each request.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.New().String()
		}

		c.Set(requestIDKey, id)
		c.Header(RequestIDHeader, id)

		c.Next()
	}
}

// GetRequestID retrieves the request ID from the Gin context.
func GetRequestID(c *gin.Context) string {
	id, _ := c.Get(requestIDKey)
	if s, ok := id.(string); ok {
		return s
	}
	return ""
}
