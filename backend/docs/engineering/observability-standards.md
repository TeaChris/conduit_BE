# Conduit Engineering Standards: Observability

## 1. Purpose
This document establishes the observability standards and best practices for the Conduit Notification Platform. A highly observable system allows engineering teams to understand its internal state, troubleshoot issues rapidly, and ensure reliability and performance. This guide covers logging, metrics, tracing, and health checks across our Go services.

## 2. Observability Philosophy
Our approach to observability is built on the following principles:

- **Three Pillars**: We utilize Logs (for specific events), Metrics (for aggregate behavior), and Traces (for request flows).
- **Observe Everything, Alert Selectively**: Instrument all critical paths, but only configure alerts for actionable, user-impacting symptoms.
- **Correlation IDs Connect the Dots**: Every log, metric, and trace must be linked via `request_id` and `trace_id`.
- **Observability is Not Optional**: Code is not complete until it is fully instrumented.

## 3. Logging
Logging provides an event-by-event record of application behavior.

- **Framework**: `zerolog` is our standard logging library.
- **Format**: We use JSON logging in production for structured ingestion, and a pretty console formatter for local development.
- **Request-Scoped Loggers**: Always extract the logger from the context using `zerolog.Ctx(ctx)`. The logging middleware automatically attaches `request_id` and `trace_id` to this logger.

### Log Levels
- **Debug**: Detailed development information (e.g., intermediate variable states). Disabled in production.
- **Info**: Meaningful business events (e.g., "User created", "Notification sent", "Request completed").
- **Warn**: Expected failures or non-critical issues (e.g., 4xx HTTP responses, client validation errors).
- **Error**: Unexpected failures requiring investigation (e.g., 5xx HTTP responses, database connection failures, panics).

### Structured Logging
Always use structured key-value pairs. Never format variables into the message string itself.

```go
// GOOD
logger.Info().
    Str("user_id", userID).
    Str("tenant_id", tenantID).
    Str("email_domain", "example.com").
    Msg("user created successfully")

// BAD
logger.Info().Msgf("User %s created for tenant %s", userID, tenantID)
```

### PII Rules
- **Never Log**: Emails, passwords, session tokens, API keys, connection strings, metadata payloads, or request/response bodies containing PII.
- **Always Log**: `user_id`, `tenant_id`, `email_domain` (the part after the `@`), operation name, duration, and correlation IDs.

### Error Logging Example
```go
// For 5xx Server Errors (Unexpected)
logger.Error().
    Err(err).
    Str("operation", "CreateNotification").
    Msg("failed to save notification to database")

// For 4xx Client Errors (Expected)
logger.Warn().
    Str("operation", "CreateNotification").
    Str("reason", "invalid_payload").
    Msg("client provided invalid data")
```

## 4. Metrics
Metrics provide a macroscopic view of system health and performance.

- **Framework**: Prometheus Go client.
- **Namespace**: All metrics must be prefixed with `conduit_`.
- **Registration**: Register metrics in an `init()` function within a domain-specific metrics file (e.g., `metrics.go` in the domain package).
- **Endpoint**: Exposed at `GET /metrics` for Prometheus scraping.

### Metric Types & Patterns
- **Counters**: For tracking totals (e.g., `requests_total`). Use for cumulative values.
- **Histograms**: For tracking distributions and percentiles (e.g., `request_duration_seconds`).
- **Gauges**: For values that go up and down (e.g., `active_connections`).

### Standard Domain Metrics Pattern
For every major domain operation, define a counter and a histogram:
- `conduit_<domain>_operations_total` (Labels: `operation`, `status`)
- `conduit_<domain>_operation_duration_seconds` (Labels: `operation`)

### Code Example
```go
var (
    operationsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "conduit_notifications_operations_total",
            Help: "Total number of notification operations",
        },
        []string{"operation", "status"}, // status = "success" or "error"
    )

    operationDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "conduit_notifications_operation_duration_seconds",
            Help:    "Duration of notification operations",
            Buckets: prometheus.DefBuckets,
        },
        []string{"operation"},
    )
)

func TrackOperation(operation string, start time.Time, err error) {
    status := "success"
    if err != nil {
        status = "error"
    }
    operationsTotal.WithLabelValues(operation, status).Inc()
    operationDuration.WithLabelValues(operation).Observe(time.Since(start).Seconds())
}
```

## 5. Tracing
Distributed tracing tracks requests as they flow through various services and components.

- **Framework**: OpenTelemetry (OTel) with OTLP gRPC exporter.
- **HTTP Spans**: The `otelgin` middleware automatically creates spans for all incoming HTTP requests.
- **Service Spans**: Create child spans manually in the service/domain layer for complex business logic or external calls (DB, Redis).
- **Propagation**: We use standard W3C Trace Context headers (`traceparent`).
- **Sample Rate**: Configurable via `ObservabilityConfig.SampleRate` (default 1.0 local, 0.1 production).

### Span Naming & Attributes
- Format: `<domain>.service.<Operation>` (e.g., `users.service.CreateUser`).
- Attach contextual attributes like `tenant_id` and `user_id` (but never PII).

### Code Example
```go
import "go.opentelemetry.io/otel/trace"

func (s *UserService) CreateUser(ctx context.Context, req CreateUserRequest) error {
    ctx, span := s.tracer.Start(ctx, "users.service.CreateUser",
        trace.WithAttributes(
            attribute.String("tenant_id", req.TenantID),
        ),
    )
    defer span.End()

    // perform work...
    if err != nil {
        span.RecordError(err)
        span.SetStatus(codes.Error, err.Error())
        return err
    }
    
    return nil
}
```

## 6. Correlation IDs
Correlation IDs tie logs, traces, and metrics together for a single request.

- **Request ID**: Driven by the `X-Request-ID` header. If missing from the client, the middleware generates a new UUID.
- **Trace ID**: Automatically extracted from the OTel span context.
- **Usage**: Both are automatically injected into the `zerolog` context by middleware.
- **Response**: Both IDs must be returned in the HTTP response headers to assist client debugging.

## 7. Health Checks
Robust health checks are required for Kubernetes orchestration. These endpoints do not require authentication.

- **GET /health**: Liveness probe. Verifies the HTTP server is running. Always returns 200 OK.
- **GET /ready**: Readiness probe. Verifies external dependencies (PostgreSQL, Redis) are reachable via simple pings. Returns 200 OK or 503 Service Unavailable.
- **GET /live**: Basic liveness check to ensure the application isn't deadlocked. Usually maps directly to `/health` or performs basic internal state checks.

## 8. Alerting Principles
Alerts page on-call engineers; they must be high-signal.

- **Symptom-Based**: Alert on symptoms that affect users (e.g., "High Error Rate", "High Latency"), not on causes (e.g., "High CPU"). High CPU is only a problem if it causes latency or errors.
- **Actionable**: Every alert must require an action and link to a specific runbook.
- **Thresholds**: Tune thresholds to avoid alert fatigue.
- **Standard Alerts**:
  - HTTP 5xx Error Rate > 1% for 5 minutes.
  - P99 HTTP Latency > 500ms for 5 minutes.
  - Readiness check failing for > 30 seconds.
  - Database connection pool utilization > 90%.

## 9. Dashboards
Dashboards should be structured hierarchically.

- **Service Overview (RED)**: Rate (requests/sec), Errors (error rate), Duration (P50, P95, P99 latency).
- **Business/Domain**: User signups, notifications sent, job completion rates.
- **Database**: Connection pool active/idle, query duration percentiles, transaction rollbacks.
- **Infrastructure**: Goroutine count, heap memory usage, CPU throttling.

## 10. Performance Metrics
Always monitor performance characteristics:

- **Latency**: Track P50 (median), P95, and P99 percentiles. Averages are misleading.
- **Volume**: Track total request volume and volume by endpoint/tenant.
- **Database**: Monitor query execution times via `pgx` tracing hooks.

## 11. Incident Response
Observability is the primary tool during an incident.

- **Tracing Issues**: Use the client's `X-Request-ID` to query the logging backend and immediately isolate the failing request flow.
- **Runbooks**: Ensure every configured alert has an associated runbook link in the alert description.
- **Post-Mortems**: Hold blameless post-mortems for any P1/P2 incidents, focusing on how to improve metrics, logs, and alerts to catch the issue faster next time.

## 12. Anti-Patterns
Avoid these observability mistakes:

- ❌ Using `fmt.Sprintf` or string concatenation in log messages instead of structured fields.
- ❌ Logging sensitive PII or credentials.
- ❌ Creating custom, non-standard metric namespaces (always use `conduit_`).
- ❌ Alerting on non-actionable infrastructure metrics (e.g., CPU spiking to 80% with no impact).
- ❌ Creating spans for every single function call (too much overhead; stick to HTTP, DB, and major domain boundaries).

## 13. Checklist
Before merging any PR, ensure:
- [ ] New endpoints have appropriate log coverage (info/warn/error).
- [ ] No PII is accidentally logged.
- [ ] Structured logging is used with appropriate keys (`tenant_id`, `user_id`).
- [ ] Domain metrics (Counters, Histograms) are added for new business logic.
- [ ] OTel spans are created for complex service-layer workflows.
- [ ] Health checks (`/ready`) are updated if new dependencies are introduced.
