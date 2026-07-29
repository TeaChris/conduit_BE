package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/conduit-platform/conduit/backend/internal/config"
	"github.com/conduit-platform/conduit/backend/internal/health"
	"github.com/conduit-platform/conduit/backend/internal/platform/middleware"
	"github.com/conduit-platform/conduit/backend/internal/platform/observability"
	"github.com/conduit-platform/conduit/backend/internal/user"
)

// Server wraps the HTTP server and its dependencies.
type Server struct {
	httpServer *http.Server
	logger     zerolog.Logger
}

// New creates and configures the HTTP server with all middleware and routes.
func New(cfg *config.Config, logger zerolog.Logger, healthHandler *health.Handler, userHandler *user.Handler) *Server {
	// Use release mode — we control logging through zerolog.
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()

	// --- Global middleware (applied to ALL routes including health) ---
	router.Use(
		middleware.Recovery(logger),
		middleware.RequestID(),
	)

	// --- Health endpoints (minimal middleware) ---
	router.GET("/health", healthHandler.Health)
	router.GET("/ready", healthHandler.Ready)
	router.GET("/live", healthHandler.Live)

	// --- Prometheus metrics endpoint ---
	router.GET("/metrics", gin.WrapH(observability.MetricsHandler()))

	// --- API middleware (applied to /api routes) ---
	api := router.Group("/api")
	api.Use(
		middleware.Tracing(cfg.Observability.ServiceName),
		middleware.Logging(logger),
		middleware.Security(),
		middleware.CORS(cfg.Security),
		middleware.RateLimiter(cfg.Security.RateLimitRate, cfg.Security.RateLimitBurst),
	)

	// --- Versioned API group ---
	v1 := api.Group("/v1")
	v1.Use(middleware.TenantID())

	// --- Domain routes ---
	user.RegisterRoutes(v1, userHandler)

	httpServer := &http.Server{
		Addr:         cfg.Server.Addr(),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	return &Server{
		httpServer: httpServer,
		logger:     logger,
	}
}

// Start begins listening for HTTP requests. This blocks until the server stops.
func (s *Server) Start() error {
	s.logger.Info().
		Str("addr", s.httpServer.Addr).
		Msg("starting HTTP server")

	if err := s.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("http server error: %w", err)
	}
	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info().Msg("shutting down HTTP server")
	return s.httpServer.Shutdown(ctx)
}
