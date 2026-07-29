# ADR-004: Typed Error Hierarchy

## Status
Accepted

## Date
2026-07-23

## Context
Need to distinguish domain errors, validation errors, and infrastructure errors. Must never expose internal details to API clients.

## Decision
Three error types: DomainError (business logic), ValidationError (input validation with field details), InfraError (infrastructure failures, masked from clients). ToAPIError() maps internal → HTTP response.

## Consequences
### Positive
- Machine-searchable error codes for clients.
- Safe error responses that don't leak database or internal stack traces.
- Clear separation of concerns between layers.
- All errors are cleanly categorized.
### Negative
- Requires discipline to wrap and return the correct error types throughout the application.
### Risks
- Developers might default to generic errors. Mitigation: provide robust helper functions for error creation and enforce usage during code review.
