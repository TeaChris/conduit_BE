package observability

import (
	"context"

	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"

	"github.com/conduit-platform/conduit/backend/internal/config"
)

// Init initializes OpenTelemetry tracing and returns a shutdown function.
func Init(ctx context.Context, cfg config.ObservabilityConfig, env string, logger zerolog.Logger) (shutdown func(context.Context) error, err error) {
	if !cfg.TracingEnabled {
		return func(context.Context) error { return nil }, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceNameKey.String(cfg.ServiceName),
			semconv.ServiceVersionKey.String(cfg.ServiceVersion),
			semconv.DeploymentEnvironmentKey.String(env),
		),
	)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to create tracing resource")
		return func(context.Context) error { return nil }, err
	}

	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(cfg.OTLPEndpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		logger.Warn().Err(err).Msg("failed to create OTLP trace exporter")
		return func(context.Context) error { return nil }, err
	}

	bsp := sdktrace.NewBatchSpanProcessor(exporter)
	tracerProvider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(cfg.TracingSampleRate)),
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(bsp),
	)
	otel.SetTracerProvider(tracerProvider)

	logger.Info().Msg("observability initialized")

	return tracerProvider.Shutdown, nil
}
