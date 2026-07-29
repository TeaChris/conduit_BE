# ADR-006: Shared Database Multi-Tenancy

## Status
Accepted

## Date
2026-07-23

## Context
Platform must support multiple tenants. Need to choose isolation strategy.

## Decision
Shared database, shared schema with tenant_id column. Tenant ID propagated via X-Tenant-ID header → context. No row-level security in this phase. Authentication will enforce tenant isolation later.

## Consequences
### Positive
- Simple to implement.
- No database-per-tenant overhead, making it cheaper and easier to manage migrations.
### Negative
- Risk of cross-tenant data access if queries aren't filtered correctly.
### Risks
- Forgetting to add `tenant_id` to a `WHERE` clause. Mitigation: context propagation, future auth middleware, strict code reviews, and potential future adoption of Row-Level Security (RLS).
