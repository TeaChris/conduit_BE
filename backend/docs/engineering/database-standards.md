# Database Standards

## 1. Purpose
This document establishes the core standards, conventions, and practices for database design, implementation, and access within the Conduit Notification Platform. Our goal is to ensure consistency, performance, security, and maintainability across our data layer. All engineers modifying schema or writing queries must adhere to these standards.

## 2. Schema Design Philosophy
- **Normalize First**: Always begin with a fully normalized design (3NF). Denormalize only when there is a proven, measured performance justification.
- **Standard Columns**: Every table MUST include `id` (UUID), `created_at` (TIMESTAMPTZ), and `updated_at` (TIMESTAMPTZ).
- **Enforce Constraints in DB**: Use `CHECK` constraints for enum columns to ensure data integrity at the lowest level. Use `UNIQUE` constraints for business keys.
- **Tenant Isolation**: We are a multi-tenant platform. Tenant isolation must be applied at the row level. Almost every table must have a `tenant_id` column.

## 3. Naming Conventions
Consistency in naming reduces cognitive load and prevents errors.
- **Tables**: Plural, `snake_case` (e.g., `users`, `notifications`, `email_templates`).
- **Columns**: `snake_case` (e.g., `created_at`, `tenant_id`, `is_active`).
- **Indexes**: `idx_<table>_<columns>` (e.g., `idx_users_tenant_email`, `idx_notifications_status`).
- **Triggers**: `set_<table>_updated_at`.
- **Functions**: Shared functions like `trigger_set_updated_at()` must be descriptive.
- **Constraints**: Descriptive names such as `chk_users_status`, `fk_users_tenant_id`, `uq_users_tenant_email`.

## 4. Primary Keys
- **UUID v4**: We use UUID v4 for all primary keys, generated via `gen_random_uuid()`.
- **Why UUIDs?**: 
  - Prevents enumeration attacks (e.g., guessing `/api/users/42`).
  - Safe for distributed systems and asynchronous inserts.
  - Avoids sequential guessing.
- **Extensions**: Requires the `pgcrypto` extension to be enabled in PostgreSQL.
- **Immutability**: Primary keys are strictly immutable. Never update an ID.

## 5. Timestamps
- **Data Type**: Always use `TIMESTAMPTZ`. Never use standard `TIMESTAMP` as it lacks timezone awareness.
- **Defaults**: Set `NOT NULL DEFAULT now()`.
- **UTC**: All timestamps are stored and processed in UTC.
- **Auto-Update**: `updated_at` is automatically updated via the shared `trigger_set_updated_at()` trigger function.
- **Optional Events**: Use nullable `TIMESTAMPTZ` for optional events (e.g., `email_verified_at`, `deactivated_at`).

## 6. Foreign Keys
- **Always Use FKs**: Enforce referential integrity in the database.
- **Tenant References**: `tenant_id` must reference `tenants(id)`.
- **Cross-Domain**: Cross-domain foreign keys are acceptable as we are a modular monolith.
- **ON DELETE Behavior**: Default to `RESTRICT`. Never use `CASCADE` without explicit architectural justification, as it can lead to accidental mass data deletion.

## 7. Indexes
- **Foreign Keys**: Index every foreign key to prevent full table scans on joins.
- **Query Driven**: Index columns heavily used in `WHERE`, `ORDER BY`, and `GROUP BY` clauses.
- **Composite Indexes**: When creating composite indexes, place the most selective column first (e.g., `tenant_id` followed by `email`).
- **Unique Indexes**: Use unique indexes to enforce business constraints.
- **Expression Indexes**: Use expression indexes where appropriate, e.g., `CREATE UNIQUE INDEX ... ON lower(email)` for case-insensitive uniqueness.
- **Restraint**: Do not over-index. Every index consumes disk space and incurs write penalties.

## 8. Constraints
- **CHECK Constraints**: Use `CHECK` constraints to enforce valid values for enum-like columns.
- **NOT NULL**: Default to `NOT NULL`. Nullable columns must have specific justification.
- **UNIQUE**: Use `UNIQUE` constraints (or unique indexes) for natural/business keys.
- **DEFAULT**: Provide sensible defaults at the schema level to simplify inserts.

## 9. Soft Deletes
- **Status Column**: Use a `status` column (e.g., `active`, `deactivated`, `suspended`) instead of `deleted_at`.
- **Why?**: Soft deletes via `deleted_at IS NULL` require modifying every single query to exclude "deleted" rows, which is error-prone. A `status` column enforces explicit state management.
- **Referential Integrity**: Deactivated records maintain their foreign key relationships safely.
- **Audit**: Use a `deactivated_at` timestamp to record when the state change occurred. Clear this if the record is reactivated.

## 10. JSONB Usage
- **Purpose**: Use `JSONB` strictly for extensible metadata or schemaless attributes that do not require relational integrity.
- **Defaults**: Default to `'{}'::jsonb NOT NULL`.
- **Anti-Patterns**: 
  - Do not use `JSONB` for relational data.
  - Avoid deep querying into nested `JSONB` structures on hot paths, as it performs poorly even with GIN indexes.

## 11. Migrations
- **Numbering**: Sequential numbering, e.g., `000001_initial.up.sql`, `000002_add_users.up.sql`.
- **Pairs**: Always provide both `.up.sql` and `.down.sql` migrations.
- **Compatibility**: Migrations must be backward-compatible. Deployments happen while the app is running.
- **Separation**: Never modify existing data (DML) in the same migration that changes the schema (DDL).
- **Testing**: Test all migrations locally and in CI against production-like data volumes.

**Example Migration:**
```sql
-- 000002_create_users_table.up.sql
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    email VARCHAR(255) NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'deactivated', 'suspended')),
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deactivated_at TIMESTAMPTZ,
    UNIQUE (tenant_id, email)
);

CREATE INDEX idx_users_tenant_id ON users(tenant_id);

CREATE TRIGGER set_users_updated_at
BEFORE UPDATE ON users
FOR EACH ROW
EXECUTE FUNCTION trigger_set_updated_at();

-- 000002_create_users_table.down.sql
DROP TABLE IF EXISTS users;
```

## 12. sqlc Usage
- **Location**: All queries reside in `sqlc/queries/<domain>.sql`.
- **Generated Code**: Code is generated into `internal/platform/database/sqlcdb/`.
- **Parameters**: Always use parameterized queries to prevent SQL injection.
- **Naming**: `-- name: <Action><Entity> :<type>` (e.g., `-- name: GetUser :one`, `-- name: ListUsers :many`).
- **Optionals**: Use `sqlc.narg()` for optional filter parameters.

**Example Query:**
```sql
-- name: GetUserByEmail :one
SELECT id, tenant_id, email, status, metadata, created_at, updated_at
FROM users
WHERE tenant_id = $1 AND email = $2
LIMIT 1;

-- name: UpdateUserStatus :exec
UPDATE users
SET status = $2, deactivated_at = sqlc.narg('deactivated_at')
WHERE id = $1 AND tenant_id = $3;
```

## 13. Transactions
- **Helper**: Use the provided `database.WithTx()` helper for executing logic within a transaction.
- **Duration**: Keep transactions as short as possible to prevent lock contention.
- **External Calls**: NEVER hold a database transaction open while making external network/API calls.
- **Read-Only**: Read-only queries generally do not need explicit transactions.

## 14. Performance
- **Connection Pooling**: Use `pgxpool`. Our defaults are `min_conns=2`, `max_conns=10` per instance.
- **Profiling**: Use `EXPLAIN ANALYZE` for any query taking > 100ms.
- **Pagination**: Paginate all list queries. Hard limit of 100 items per page.
- **Technique**: Use `LIMIT`/`OFFSET` for v1 functionality, but plan to migrate to cursor-based pagination for high-volume endpoints.
- **Projection**: Avoid `SELECT *` in production. Always list columns explicitly to minimize payload size and improve index utilization.

## 15. Multi-Tenancy
- **Requirement**: Every domain table MUST have a `tenant_id` column.
- **Scoping**: Every single query must be scoped by `tenant_id`. `WHERE id = $1 AND tenant_id = $2`.
- **Isolation**: Never return data across tenant boundaries under any circumstances.
- **Constraints**: Unique constraints are per-tenant. Ensure composite keys include `tenant_id` (e.g., `UNIQUE (tenant_id, business_key)`).

## 16. Anti-Patterns
- Using `deleted_at` for soft deletes.
- Cascading deletes.
- Missing `tenant_id` in queries.
- Business logic in stored procedures (keep logic in Go).
- String concatenation for SQL queries (always use sqlc).
- Updating primary keys.

## 17. Checklist
Before submitting a PR with database changes:
- [ ] Migration includes both `up` and `down` files.
- [ ] Table has `id`, `created_at`, `updated_at`.
- [ ] Table has `tenant_id` and is referenced correctly.
- [ ] Appropriate indexes are added for FKs and common lookups.
- [ ] Unique constraints are defined correctly (per tenant).
- [ ] `updated_at` trigger is attached.
- [ ] sqlc queries are tested via integration tests.
