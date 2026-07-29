package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel/trace"
)

// Logging returns middleware that logs each request using zerolog.
func Logging(logger zerolog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		if raw != "" {
			path = path + "?" + raw
		}

		// Attach logger to request context for downstream use.
		requestID := GetRequestID(c)
		spanCtx := trace.SpanContextFromContext(c.Request.Context())
		traceID := ""
		if spanCtx.HasTraceID() {
			traceID = spanCtx.TraceID().String()
		}

		requestLogger := logger.With().
			Str("request_id", requestID).
			Str("trace_id", traceID).
			Logger()

		// Store logger in request context for use by handlers.
		ctx := requestLogger.WithContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		duration := time.Since(start)
		status := c.Writer.Status()

		event := requestLogger.Info()
		if status >= 500 {
			event = requestLogger.Error()
		} else if status >= 400 {
			event = requestLogger.Warn()
		}

		event.
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", status).
			Dur("duration", duration).
			Str("client_ip", c.ClientIP()).
			Int("body_size", c.Writer.Size()).
			Msg("request completed")
	}
}
