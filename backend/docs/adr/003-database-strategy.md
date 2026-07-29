# ADR-003: PostgreSQL with pgx and sqlc

## Status
Accepted

## Date
2026-07-23

## Context
Need type-safe database access without ORM overhead. Need migrations.

## Decision
pgx for connection pooling and driver. sqlc for type-safe query generation from SQL. golang-migrate for schema migrations. No GORM, no active record.

## Consequences
### Positive
- SQL is explicit and reviewable.
- Generated code is predictable and performant.
- No magic behavior from ORMs.
- pgx offers superior performance and features over standard lib database/sql.
### Negative
- Trade-off: must write SQL by hand (this is considered a feature, not a bug, for better control).
- More boilerplate for simple CRUD compared to ORMs.
### Risks
- Developers unfamiliar with SQL might struggle initially. Mitigation: team training and code reviews.
