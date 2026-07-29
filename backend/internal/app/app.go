package app

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/conduit-platform/conduit/backend/internal/config"
	"github.com/conduit-platform/conduit/backend/internal/health"
	"github.com/conduit-platform/conduit/backend/internal/platform/cache"
	"github.com/conduit-platform/conduit/backend/internal/platform/database"
	"github.com/conduit-platform/conduit/backend/internal/platform/observability"
	"github.com/conduit-platform/conduit/backend/internal/server"
)

// App holds all application dependencies.
type App struct {
	cfg          *config.Config
	logger       zerolog.Logger
	db           *pgxpool.Pool
	redis        *redis.Client
	server       *server.Server
	otelShutdown func(context.Context) error
}

// New creates and wires all application dependencies.
// If any dependency fails to initialize, the application fails fast.
func New(ctx context.Context) (*App, error) {
	// 1. Load configuration.
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	// 2. Set up logger.
	logger := setupLogger(cfg.Log, cfg.Environment)
	logger.Info().
		Str("environment", string(cfg.Environment)).
		Msg("starting conduit")

	// 3. Initialize observability (tracing + metrics).
	otelShutdown, err := observability.Init(ctx, cfg.Observability, string(cfg.Environment), logger)
	if err != nil {
		return nil, fmt.Errorf("initializing observability: %w", err)
	}

	// 4. Connect to database.
	db, err := database.NewPool(ctx, cfg.Database, logger)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	// 5. Connect to Redis.
	redisClient, err := cache.NewClient(ctx, cfg.Redis, logger)
	if err != nil {
		return nil, fmt.Errorf("connecting to redis: %w", err)
	}

	// 6. Create health handler.
	healthHandler := health.NewHandler(db, redisClient)

	// 7. Create HTTP server.
	srv := server.New(cfg, logger, healthHandler)

	return &App{
		cfg:          cfg,
		logger:       logger,
		db:           db,
		redis:        redisClient,
		server:       srv,
		otelShutdown: otelShutdown,
	}, nil
}

// Run starts the application and blocks until a shutdown signal is received.
func (a *App) Run(ctx context.Context) error {
	// Start HTTP server in a goroutine.
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.server.Start()
	}()

	// Wait for interrupt signal or server error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case sig := <-quit:
		a.logger.Info().Str("signal", sig.String()).Msg("shutdown signal received")
	}

	// Graceful shutdown.
	return a.Shutdown(ctx)
}

// Shutdown gracefully tears down all dependencies in reverse order.
func (a *App) Shutdown(ctx context.Context) error {
	shutdownCtx, cancel := context.WithTimeout(ctx, a.cfg.Server.ShutdownTimeout)
	defer cancel()

	a.logger.Info().Msg("starting graceful shutdown")

	// 1. Stop HTTP server (stop accepting new requests, drain existing).
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		a.logger.Error().Err(err).Msg("error shutting down HTTP server")
	}

	// 2. Close Redis.
	if err := a.redis.Close(); err != nil {
		a.logger.Error().Err(err).Msg("error closing redis connection")
	}

	// 3. Close database pool.
	a.db.Close()

	// 4. Flush and shutdown OpenTelemetry.
	if a.otelShutdown != nil {
		if err := a.otelShutdown(shutdownCtx); err != nil {
			a.logger.Error().Err(err).Msg("error shutting down observability")
		}
	}

	a.logger.Info().Msg("shutdown complete")
	return nil
}

// setupLogger configures zerolog based on the log config.
func setupLogger(cfg config.LogConfig, env config.Environment) zerolog.Logger {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(level)

	var logger zerolog.Logger
	if cfg.Pretty || env == config.EnvLocal {
		logger = zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).
			With().
			Timestamp().
			Caller().
			Logger()
	} else {
		logger = zerolog.New(os.Stderr).
			With().
			Timestamp().
			Logger()
	}

	return logger
}
