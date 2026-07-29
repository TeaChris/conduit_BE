# ADR-005: OpenTelemetry with Prometheus Metrics

## Status
Accepted

## Date
2026-07-23

## Context
Need production observability from day one. Distributed tracing for debugging. Metrics for monitoring.

## Decision
OpenTelemetry for tracing (OTLP exporter). Prometheus for metrics. Structured JSON logging with zerolog. Every request gets request_id + trace_id.

## Consequences
### Positive
- Vendor-neutral observability stack.
- Standard tooling that works well with modern infrastructure.
- High visibility into system performance and errors.
### Negative
- Slight startup complexity.
- OTel SDK adds some negligible overhead.
### Risks
- High volume logging/tracing can incur costs. Mitigation: use sampling for traces in production.
