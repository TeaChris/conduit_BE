# ADR-007: Testing Strategy

## Status
Accepted

## Date
2026-07-23

## Context
Need reliable tests that don't slow down development.

## Decision
Three test layers: 
1. Unit tests next to code, standard library, no external deps. 
2. Integration tests in tests/integration/, build tag `integration`, require running DB+Redis. 
3. API tests in tests/api/, hit real HTTP endpoints. Mocking via interfaces (sqlc generates interfaces with emit_interface). No test frameworks beyond standard library + testify for assertions in integration tests.

## Consequences
### Positive
- Fast unit tests keep feedback loop tight.
- Realistic integration tests provide confidence in data access layer.
- Clear separation helps new engineers know exactly where to put each test type.
### Negative
- Requires maintaining Docker-compose for integration tests.
### Risks
- Flaky integration tests. Mitigation: ensure test isolation (e.g., using transactions that rollback, or unique test data).
