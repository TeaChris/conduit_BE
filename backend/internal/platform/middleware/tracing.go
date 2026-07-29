package middleware

import (
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// Tracing returns middleware that creates OpenTelemetry spans for each request.
func Tracing(serviceName string) gin.HandlerFunc {
	return otelgin.Middleware(serviceName)
}
