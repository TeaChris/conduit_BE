# RFC-0001: User Domain

| Field | Value |
|---|---|
| **Author** | Platform Engineering |
| **Status** | Draft — Pending TDR Review |
| **Created** | 2026-07-30 |
| **Domain** | `internal/user/` |
| **Reviewers** | Principal Engineers, TDR Committee |

---

## 1. Executive Summary

This RFC defines the engineering design for the User domain of the Conduit Notification Platform. The User domain is the first business domain and establishes the architectural pattern that every future domain will follow.

The User domain owns user lifecycle management: creation, retrieval, update, deactivation, reactivation, and email verification state. It does NOT own authentication, authorization, organizations, or notification preferences.

The design prioritizes simplicity, tenant isolation, future compatibility, and operational excellence. Every decision is aligned with the engineering standards documented in `docs/engineering/`.

**Key design decisions:**

1. **Flat package structure** — `internal/user/` with file-per-layer, no sub-packages
2. **Three lifecycle states** — `active`, `deactivated`, `suspended`
3. **Per-tenant email uniqueness** — case-insensitive, enforced at the database level
4. **No hard deletes** — deactivation serves as soft delete via status
5. **No username** — email is the identity; usernames add complexity without value for B2B SaaS
6. **No locale/timezone yet** — deferred until notification delivery requires them (YAGNI)
7. **Repository interface** — the only interface in the domain, enabling unit testing without a database

---

## 2. Background

Conduit is a multi-tenant Notification Platform targeting enterprise customers. The platform foundation (config, database, cache, middleware, observability, health checks) has been completed. The User domain is the first business domain and will serve as the reference implementation for all future domains.

The platform will eventually support Authentication, Organizations, RBAC, API Keys, Notification Templates, Delivery, Analytics, and Billing. All of these depend on a stable User entity.

### Existing Foundation

| Component | Technology | Location |
|---|---|---|
| HTTP Framework | Gin | `internal/server/` |
| Database | PostgreSQL 16 via pgx/v5 | `internal/platform/database/` |
| Cache | Redis 7 | `internal/platform/cache/` |
| Logging | Zerolog | `internal/platform/middleware/` |
| Tracing | OpenTelemetry + OTLP | `internal/platform/observability/` |
| Metrics | Prometheus | `internal/platform/observability/` |
| Config | caarlos0/env/v11 | `internal/config/` |
| Errors | DomainError/ValidationError/InfraError | `internal/platform/errors/` |
| Multi-tenancy | X-Tenant-ID header middleware | `internal/platform/middleware/` |

### Engineering Standards

This RFC was designed in compliance with:

- `engineering-principles.md` — Simplicity over cleverness, boring technology
- `architecture-principles.md` — Flat domain packages, Clean Architecture, explicit DI
- `coding-standards.md` — Three-tier error model, context-first, no panics
- `api-guidelines.md` — REST, PATCH for updates, `{"data": ...}` envelopes
- `database-standards.md` — UUID PKs, TIMESTAMPTZ, tenant FK, status-based soft delete
- `testing-standards.md` — Functional struct mocks, table-driven tests, 80%+ coverage
- `security-standards.md` — PII handling, input validation, parameterized queries
- `observability-standards.md` — `conduit_` metrics, OTel spans, request-scoped logging

---

## 3. Problem Statement

The platform has no concept of a user. Without a User entity:

- No future domain can identify who is performing an action
- Authentication has no identity to authenticate against
- Organizations have no members to organize
- RBAC has no subjects to assign roles to
- Audit logs have no actors to record
- Billing has no accounts to charge

The User domain must provide a stable, well-tested, tenant-scoped user entity that future domains can build upon without requiring schema redesign.

---

## 4. Goals

| # | Goal | Measure |
|---|---|---|
| G1 | Provide complete user lifecycle management | All CRUD + status transitions implemented |
| G2 | Establish the domain implementation pattern | Every future domain follows the same structure |
| G3 | Support future authentication without schema changes | `password_hash`, `last_login_at` can be added via migration |
| G4 | Support future organizations without schema changes | `org_memberships` table can FK to `users.id` |
| G5 | Scale to millions of users per tenant | Indexed queries, pagination, no N+1 |
| G6 | Full observability | Metrics, traces, structured logs on every operation |
| G7 | Testable in isolation | 80%+ service coverage, integration tests, handler tests |

---

## 5. Non-Goals

| # | Non-Goal | Rationale |
|---|---|---|
| NG1 | Authentication (JWT, sessions, login, logout) | Separate domain, separate RFC |
| NG2 | Organizations and membership | Separate domain |
| NG3 | RBAC (roles, permissions) | Separate domain |
| NG4 | Password handling (hash, reset, verification emails) | Part of Authentication domain |
| NG5 | Notification preferences | Separate domain (NotificationPreferences) |
| NG6 | Profile pictures / avatars | Requires blob storage; deferred |
| NG7 | Username field | Email is the identity for B2B SaaS |
| NG8 | Locale / timezone | Deferred until notification delivery requires scheduling |
| NG9 | Kafka events | Event publishing deferred until second domain needs it |
| NG10 | Redis caching | No read-heavy access pattern yet; premature optimization |

---

## 6. Assumptions

| # | Assumption | Justification |
|---|---|---|
| A1 | Users are human operators of the platform, not end-user notification recipients | Recipients will be a separate entity in the Notification domain |
| A2 | One email per user per tenant | A person can have accounts in multiple tenants with the same email |
| A3 | Tenants already exist when users are created | The `tenants` table and FK constraint enforce this |
| A4 | No self-registration in this phase | User creation is an API call (admin or future auth flow) |
| A5 | The platform will add authentication before going to production | The User schema is designed to support `password_hash` via migration |
| A6 | Email changes require re-verification | Changing email resets `email_verified` to false |
| A7 | Suspension is an admin/system action, distinct from user-initiated deactivation | Different reactivation flows and audit implications |
| A8 | No cross-tenant user queries | Users are strictly scoped to their tenant |

---

## 7. Functional Requirements

### FR-1: User Creation

| Aspect | Requirement |
|---|---|
| Input | email (required), display_name (required), metadata (optional) |
| Email normalization | Lowercase, trim whitespace |
| Email validation | Non-empty, contains `@`, max 320 characters |
| Display name validation | Non-empty after trim, max 256 characters |
| Initial status | `active` |
| Initial email_verified | `false` |
| Uniqueness | Email must be unique within the tenant (case-insensitive) |
| Output | Created user with generated UUID and timestamps |

### FR-2: User Retrieval

| Aspect | Requirement |
|---|---|
| By ID | UUID path parameter, tenant-scoped |
| By Email | Used internally (service layer), tenant-scoped |
| Not found | Returns `NOT_FOUND` domain error |

### FR-3: User Listing

| Aspect | Requirement |
|---|---|
| Pagination | Offset-based: `page` (default 1), `per_page` (default 20, max 100) |
| Filtering | Optional `status` filter (active, deactivated, suspended) |
| Sorting | `created_at DESC` (fixed for v1) |
| Scoping | Always filtered by `tenant_id` |
| Response | Users array + pagination metadata |

### FR-4: User Update

| Aspect | Requirement |
|---|---|
| Mutable fields | email, display_name, metadata |
| Partial update | PATCH semantics — only provided fields are updated |
| Email change | Resets `email_verified` to false, clears `email_verified_at` |
| Validation | Same rules as creation, plus at least one field required |
| Uniqueness | Email uniqueness re-validated on change |

### FR-5: User Deactivation

| Aspect | Requirement |
|---|---|
| Transition | `active` → `deactivated` only |
| Side effects | Sets `deactivated_at` to current UTC time |
| Invalid transitions | Returns `CONFLICT` domain error |

### FR-6: User Reactivation

| Aspect | Requirement |
|---|---|
| Transition | `deactivated` → `active`, `suspended` → `active` |
| Side effects | Clears `deactivated_at` to NULL |
| Already active | Returns `CONFLICT` domain error |

### FR-7: Email Verification

| Aspect | Requirement |
|---|---|
| Operation | Sets `email_verified` to true, `email_verified_at` to now() |
| Exposed | Service method only — no HTTP endpoint in this phase |
| Future | Authentication domain will call this after email confirmation flow |

---

## 8. Non-Functional Requirements

| # | Requirement | Target |
|---|---|---|
| NFR-1 | API latency P99 | < 100ms for single-user operations |
| NFR-2 | List query P99 | < 200ms for 100 results |
| NFR-3 | Availability | 99.9% (inherited from platform) |
| NFR-4 | Test coverage | >= 80% service layer, 100% model methods |
| NFR-5 | Concurrent users per tenant | Support 1M+ without query degradation |
| NFR-6 | Observability | Every operation emits logs, metrics, and trace spans |
| NFR-7 | Security | No PII in logs, parameterized queries, tenant isolation |
| NFR-8 | Zero downtime deploys | Backward-compatible migrations |

---

## 9. Domain Model

### Entity: User

```go
type User struct {
    ID              uuid.UUID       // Immutable. Generated by PostgreSQL.
    TenantID        uuid.UUID       // Immutable. Set at creation.
    Email           string          // Mutable. Normalized (lowercase, trimmed).
    DisplayName     string          // Mutable. Trimmed.
    Status          Status          // Managed via state machine.
    EmailVerified   bool            // Reset to false on email change.
    EmailVerifiedAt *time.Time      // Set when verified, cleared on email change.
    Metadata        map[string]any  // Tenant-extensible key-value storage.
    DeactivatedAt   *time.Time      // Set on deactivation, cleared on reactivation.
    CreatedAt       time.Time       // Immutable. Set by PostgreSQL.
    UpdatedAt       time.Time       // Auto-managed by trigger.
}
```

### Value Object: Status

```go
type Status string

const (
    StatusActive      Status = "active"
    StatusDeactivated Status = "deactivated"
    StatusSuspended   Status = "suspended"
)
```

### Domain Methods

| Method | Purpose | Layer |
|---|---|---|
| `Status.IsValid()` | Validates status string is a known value | Domain |
| `User.IsActive()` | Checks if user is in active state | Domain |
| `User.CanTransitionTo(target)` | Validates state machine transitions | Domain |
| `ListFilter.Offset()` | Calculates SQL offset from page/page_size | Domain |

### Design Decisions

**Why no Username?** This is a B2B SaaS platform. Users are identified by email in all authentication flows (login, password reset, invitations). Adding a username field would require uniqueness enforcement, additional validation, and display logic — all without clear business value. If usernames are needed in the future, they can be added as a nullable column without breaking changes.

**Why no Locale/Timezone?** While a notification platform will eventually need timezone for delivery scheduling, the User domain is not responsible for notification delivery. Adding these now would be speculative. When the Notification Delivery domain is built, timezone can be added via a simple `ALTER TABLE users ADD COLUMN timezone TEXT` migration, or as a field on a NotificationPreferences entity (more appropriate since timezone preference is about delivery, not identity).

**Why `map[string]any` for Metadata?** Tenants need to attach custom data to users (internal IDs, department codes, tags). JSONB provides this flexibility without schema changes. The metadata is NOT queried in hot paths — it's read when fetching a user and written when creating/updating.

---

## 10. Domain Boundaries

### What the User Domain Owns

- User entity lifecycle (CRUD + status transitions)
- Email normalization and uniqueness enforcement
- Email verification state tracking
- Profile information (display_name, metadata)
- User-specific domain errors
- User-specific metrics and tracing

### What the User Domain Does NOT Own

| Concern | Owner |
|---|---|
| Password storage and verification | Authentication domain |
| Login sessions and tokens | Authentication domain |
| Organization membership | Organization domain |
| Roles and permissions | RBAC domain |
| API key generation | API Keys domain |
| Notification preferences | NotificationPreferences domain |
| Audit log records | Audit domain |
| Billing accounts | Billing domain |

### Interaction Points with Future Domains

```
+-------------------+     FK: user_id
|  Authentication   |-----------------------+
|  (credentials)    |                       |
+-------------------+                       v
                                       +----------+
+-------------------+    FK: user_id   |          |
|  Organizations    |------------------|  USERS   |
|  (memberships)    |                  |          |
+-------------------+                  +----------+
                                            ^
+-------------------+    FK: user_id        |
|      RBAC         |----------------------+
|   (user_roles)    |
+-------------------+

+-------------------+    FK: user_id
|   Audit Logs      |-----------------------+
|  (actor_id)       |                       |
+-------------------+                       v
                                       +----------+
+-------------------+    FK: user_id   |          |
|    Billing        |------------------|  USERS   |
| (subscriptions)   |                  |          |
+-------------------+                  +----------+
```

All future domains reference `users.id` via foreign key. No schema changes to the `users` table are required.

---

## 11. Business Rules

### BR-1: Email Uniqueness

**Rule:** A tenant cannot have two users with the same email address.

**Enforcement:** Database unique index on `(tenant_id, lower(email))`.

**Rationale:** Email is the primary identifier for login, invitations, and password reset. Duplicate emails within a tenant would create ambiguity in all authentication flows.

**Cross-tenant:** The same email CAN exist in different tenants. This supports consultants and contractors who work across multiple organizations.

### BR-2: Email Normalization

**Rule:** All emails are lowercased and whitespace-trimmed before storage.

**Enforcement:** Service layer `normalizeEmail()` function.

**Rationale:** Prevents `User@Example.COM` and `user@example.com` from being treated as different users. We do NOT strip dots or plus-suffixes — that is provider-specific logic (Gmail) that would break other email providers.

### BR-3: Email Mutability

**Rule:** Users can change their email address.

**Side effects:**
- `email_verified` resets to `false`
- `email_verified_at` resets to `NULL`
- Uniqueness is re-validated

**Rationale:** Users change companies, migrate to new email providers, or correct typos. Blocking email changes forces admin intervention and creates support burden.

> **Gap in current implementation:** The existing `UpdateUser` service method does NOT reset `email_verified` when the email changes. This must be fixed.

### BR-4: Status Transitions

**Rule:** Only specific status transitions are permitted.

```
         +---- deactivate ----+
         |                    v
     +--------+         +--------------+
     | active |         | deactivated  |
     +--------+         +--------------+
         |                    |
         |                    | reactivate
         |                    |
         |   suspend     +----+
         |               |
         v               v
     +-----------+   +--------+
     | suspended |-->| active |
     +-----------+   +--------+
         reactivate
```

| From | To | Allowed | Trigger |
|---|---|---|---|
| `active` | `deactivated` | Yes | User or admin action |
| `active` | `suspended` | Yes | Admin or system action |
| `deactivated` | `active` | Yes | Reactivation |
| `suspended` | `active` | Yes | Admin unsuspend |
| `deactivated` | `suspended` | No | Already inactive |
| `suspended` | `deactivated` | No | Admin must unsuspend first |
| Any | Same | No | No-op transitions disallowed |

**Enforcement:** Domain method `User.CanTransitionTo()` + service layer validation.

### BR-5: No Hard Deletes

**Rule:** Users are never hard-deleted from the database.

**Rationale:**
1. Future domains (notifications, audit logs, billing) will hold foreign keys to `users.id`
2. GDPR/CCPA compliance requires data retention for legal periods
3. Accidental deactivation is reversible
4. Analytics require historical user data

**GDPR compliance:** A future data purge job can anonymize PII (overwrite email, display_name) on users who have been deactivated longer than the retention period, while preserving the row for referential integrity.

### BR-6: Display Name Rules

| Rule | Value | Rationale |
|---|---|---|
| Required | Yes | Users need a visible name in notification platform UIs |
| Min length | 1 (after trim) | No empty strings |
| Max length | 256 | Accommodates any human name, including CJK |
| Unicode | Allowed | International platform |
| Trimmed | Yes | No leading/trailing whitespace stored |
| Format restriction | None | Names are culturally complex; regex validation is harmful |

---

## 12. State Model

### Status Lifecycle

```
                    +-----------+
      POST /users   |           |
    --------------->|  active   |<----------------------------+
                    |           |                              |
                    +-----+-----+                              |
                          |                                    |
              +-----------+-----------+                        |
              |                       |                        |
              v                       v                        |
     +--------------+         +-----------+                    |
     | deactivated  |         | suspended |                    |
     |              |         |           |                    |
     |deactivated_at|         |           |                    |
     |   = now()    |         |           |                    |
     +------+-------+         +-----+-----+                    |
            |                       |                          |
            |    POST               |    POST                  |
            |  /reactivate          |  /reactivate             |
            |                       |                          |
            +-----------+-----------+                          |
                        |                                      |
                        |   deactivated_at = NULL              |
                        +--------------------------------------+
```

### Invariants

1. `deactivated_at` is NOT NULL if and only if `status = 'deactivated'`
2. `email_verified_at` is NOT NULL if and only if `email_verified = true`
3. `created_at` is immutable
4. `id` is immutable
5. `tenant_id` is immutable

---

## 13. Database Design

### Schema: `users` table

```sql
CREATE TABLE users (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID        NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    email             TEXT        NOT NULL,
    display_name      TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'active'
                                  CHECK (status IN ('active', 'deactivated', 'suspended')),
    email_verified    BOOLEAN     NOT NULL DEFAULT false,
    email_verified_at TIMESTAMPTZ,
    metadata          JSONB       NOT NULL DEFAULT '{}',
    deactivated_at    TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

### Column Justification

| Column | Type | Nullable | Justification |
|---|---|---|---|
| `id` | UUID | NOT NULL | Immutable identity. `gen_random_uuid()` prevents enumeration. |
| `tenant_id` | UUID FK | NOT NULL | Row-level multi-tenancy. Every query scoped by this. |
| `email` | TEXT | NOT NULL | Primary contact and future login identifier. |
| `display_name` | TEXT | NOT NULL | Required for UI rendering and notification sender names. |
| `status` | TEXT + CHECK | NOT NULL | Lifecycle state machine. CHECK prevents invalid values. |
| `email_verified` | BOOLEAN | NOT NULL | Tracks whether the user has confirmed their email. |
| `email_verified_at` | TIMESTAMPTZ | NULL | When verification occurred. NULL = not yet verified. |
| `metadata` | JSONB | NOT NULL | Tenant-extensible data. Default `'{}'` prevents NULL checks. |
| `deactivated_at` | TIMESTAMPTZ | NULL | When deactivation occurred. NULL = currently active or suspended. |
| `created_at` | TIMESTAMPTZ | NOT NULL | Audit trail. Immutable. Set by `DEFAULT now()`. |
| `updated_at` | TIMESTAMPTZ | NOT NULL | Change tracking. Auto-managed by `trigger_set_updated_at()`. |

### Indexes

| Name | Definition | Purpose |
|---|---|---|
| `users_pkey` | `PRIMARY KEY (id)` | Primary lookup |
| `idx_users_tenant_email` | `UNIQUE (tenant_id, lower(email))` | Per-tenant email uniqueness, case-insensitive |
| `idx_users_tenant_id` | `(tenant_id)` | FK index + tenant-scoped listing |
| `idx_users_tenant_status` | `(tenant_id, status)` | Status-filtered listing |
| `idx_users_email_verified` | `(tenant_id, email_verified)` | Future: find unverified users for reminder flows |

### Future-Proof Columns (NOT added now)

These columns will be added via migration when their domains are built. No schema change to existing columns is required.

| Column | When | Migration |
|---|---|---|
| `password_hash TEXT` | Authentication domain | `ALTER TABLE users ADD COLUMN password_hash TEXT` |
| `last_login_at TIMESTAMPTZ` | Authentication domain | `ALTER TABLE users ADD COLUMN last_login_at TIMESTAMPTZ` |
| `timezone TEXT` | Notification Delivery domain | `ALTER TABLE users ADD COLUMN timezone TEXT DEFAULT 'UTC'` |
| `locale TEXT` | Internationalization | `ALTER TABLE users ADD COLUMN locale TEXT DEFAULT 'en'` |
| `avatar_url TEXT` | UI enhancements | `ALTER TABLE users ADD COLUMN avatar_url TEXT` |

---

## 14. API Design

All endpoints require the `X-Tenant-ID` header (enforced by middleware).

### POST /api/v1/users — Create User

**Request:**
```json
{
  "email": "jane@example.com",
  "display_name": "Jane Doe",
  "metadata": {"department": "engineering"}
}
```

**Response (201 Created):**
```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "tenant_id": "660e8400-e29b-41d4-a716-446655440000",
    "email": "jane@example.com",
    "display_name": "Jane Doe",
    "status": "active",
    "email_verified": false,
    "metadata": {"department": "engineering"},
    "created_at": "2026-07-30T08:00:00Z",
    "updated_at": "2026-07-30T08:00:00Z"
  }
}
```

**Error Responses:**

| Scenario | Status | Code |
|---|---|---|
| Missing email | 422 | `VALIDATION_ERROR` |
| Invalid email format | 422 | `VALIDATION_ERROR` |
| Email already exists in tenant | 409 | `CONFLICT` |
| Invalid JSON body | 400 | `BAD_REQUEST` |
| Missing X-Tenant-ID | 400 | `TENANT_REQUIRED` |

---

### GET /api/v1/users/:id — Get User

**Response (200 OK):**
```json
{
  "data": {
    "id": "550e8400-...",
    "tenant_id": "660e8400-...",
    "email": "jane@example.com",
    "display_name": "Jane Doe",
    "status": "active",
    "email_verified": false,
    "metadata": {},
    "created_at": "2026-07-30T08:00:00Z",
    "updated_at": "2026-07-30T08:00:00Z"
  }
}
```

**Error Responses:**

| Scenario | Status | Code |
|---|---|---|
| Invalid UUID | 400 | `BAD_REQUEST` |
| User not found in tenant | 404 | `NOT_FOUND` |

---

### GET /api/v1/users — List Users

**Query Parameters:**

| Param | Type | Default | Max | Description |
|---|---|---|---|---|
| `page` | int | 1 | — | Page number |
| `per_page` | int | 20 | 100 | Items per page |
| `status` | string | — | — | Filter: active, deactivated, suspended |

**Response (200 OK):**
```json
{
  "data": {
    "users": [
      {
        "id": "550e8400-...",
        "email": "jane@example.com",
        "display_name": "Jane Doe",
        "status": "active",
        "email_verified": false,
        "metadata": {},
        "created_at": "2026-07-30T08:00:00Z",
        "updated_at": "2026-07-30T08:00:00Z"
      }
    ],
    "pagination": {
      "page": 1,
      "per_page": 20,
      "total": 150,
      "total_pages": 8
    }
  }
}
```

---

### PATCH /api/v1/users/:id — Update User

**Request (all fields optional, at least one required):**
```json
{
  "email": "new.email@example.com",
  "display_name": "Jane Smith",
  "metadata": {"department": "product"}
}
```

**Response (200 OK):** Updated user object in `{"data": {...}}` envelope.

**Business rule:** If `email` is changed, `email_verified` resets to `false` and `email_verified_at` resets to `NULL`.

**Error Responses:**

| Scenario | Status | Code |
|---|---|---|
| No fields provided | 400 | `BAD_REQUEST` |
| Invalid email format | 422 | `VALIDATION_ERROR` |
| Email taken by another user in tenant | 409 | `CONFLICT` |
| User not found | 404 | `NOT_FOUND` |

---

### POST /api/v1/users/:id/deactivate — Deactivate User

**Request:** No body.

**Response (200 OK):** User with `status: "deactivated"` and `deactivated_at` set.

**Error Responses:**

| Scenario | Status | Code |
|---|---|---|
| User not active (invalid transition) | 409 | `CONFLICT` |
| User not found | 404 | `NOT_FOUND` |

---

### POST /api/v1/users/:id/reactivate — Reactivate User

**Request:** No body.

**Response (200 OK):** User with `status: "active"` and `deactivated_at: null`.

**Error Responses:**

| Scenario | Status | Code |
|---|---|---|
| User already active | 409 | `CONFLICT` |
| User not found | 404 | `NOT_FOUND` |

---

## 15. Validation Rules

### Handler-Layer Validation (Input)

| Field | Create | Update | Rule |
|---|---|---|---|
| `email` | Required | Optional | Non-empty, contains `@`, max 320 chars |
| `display_name` | Required | Optional | Non-empty after trim, max 256 chars |
| `metadata` | Optional | Optional | Valid JSON object (enforced by Go unmarshaling) |
| Path `:id` | — | Required | Valid UUID format |
| Query `page` | — | — | Positive integer, default 1 |
| Query `per_page` | — | — | Positive integer, 1-100, default 20 |
| Query `status` | — | — | Must be a valid Status value |

### Service-Layer Validation (Business Rules)

| Rule | Enforcement |
|---|---|
| Email uniqueness per tenant | Repository -> PostgreSQL unique index |
| Status transition validity | `User.CanTransitionTo()` domain method |
| Tenant existence | PostgreSQL FK constraint on `tenant_id` |
| Email normalization | `normalizeEmail()` before storage |

---

## 16. Error Model

### Domain Errors

```go
var (
    ErrUserNotFound       = errors.NewNotFound("user not found")
    ErrEmailAlreadyExists = errors.NewConflict("a user with this email already exists")
    ErrInvalidTransition  = errors.NewConflict("invalid status transition")
    ErrAlreadyActive      = errors.NewConflict("user is already active")
)
```

### Error Mapping

| Error | Type | HTTP Status | Code | Details |
|---|---|---|---|---|
| User not found | `DomainError` | 404 | `NOT_FOUND` | — |
| Email taken | `DomainError` | 409 | `CONFLICT` | — |
| Invalid transition | `DomainError` | 409 | `CONFLICT` | — |
| Already active | `DomainError` | 409 | `CONFLICT` | — |
| Invalid input | `ValidationError` | 422 | `VALIDATION_ERROR` | Field-level details |
| Bad UUID / body | `DomainError` | 400 | `BAD_REQUEST` | — |
| Missing tenant | `DomainError` | 400 | `TENANT_REQUIRED` | — |
| DB failure | `InfraError` | 500 | `INTERNAL_ERROR` | Masked, logged internally |

All errors flow through `httputil.Error()` -> `errors.ToAPIError()`. No custom error mapping needed.

---

## 17. Security Considerations

### Threat Model

| Threat | Vector | Mitigation |
|---|---|---|
| **Email enumeration** | POST /users returns 409 for existing emails | Accept for now; future: idempotent create returns 201 |
| **Tenant data leakage** | Missing tenant_id in query | All queries include `AND tenant_id = $N`; enforced by repository |
| **SQL injection** | Malicious input in email/display_name | Parameterized queries via pgx (`$1`, `$2`); no string concatenation |
| **Mass assignment** | Client sends unexpected fields | DTOs define exactly which fields are mutable; no struct binding to domain model |
| **PII exposure in logs** | Full email in log output | Only `email_domain` (part after @) is logged |
| **UUID guessing** | Sequential IDs allow enumeration | UUID v4 (random) makes guessing infeasible |

### Data Classification

| Field | Classification | Log? | Index? |
|---|---|---|---|
| `id` | Internal identifier | Yes | Yes |
| `tenant_id` | Internal identifier | Yes | Yes |
| `email` | **PII** | No (domain only) | Yes (hashed via lower()) |
| `display_name` | **PII** | No | No |
| `metadata` | **Potentially PII** | No | No |
| `status` | Non-sensitive | Yes | Yes |
| `email_verified` | Non-sensitive | Yes | Yes |
| `created_at` | Non-sensitive | Yes | No |

### Future Authentication Compatibility

The User schema is designed so that the Authentication domain can:

1. Add `password_hash TEXT` column via migration
2. Create an `auth_credentials` table with `user_id FK -> users(id)`
3. Verify `email_verified = true` before allowing login
4. Check `status = 'active'` before issuing tokens
5. Use `GetByEmail()` repository method for login lookups

No changes to the User domain code are required for any of these.

---

## 18. Observability

### Logging

All service methods must use the request-scoped logger via `zerolog.Ctx(ctx)`:

| Event | Level | Fields |
|---|---|---|
| User created | `Info` | `user_id`, `tenant_id`, `email_domain` |
| User updated | `Info` | `user_id`, `tenant_id` |
| User deactivated | `Info` | `user_id`, `tenant_id` |
| User reactivated | `Info` | `user_id`, `tenant_id` |
| Email conflict | `Warn` | `tenant_id` (NOT the email) |
| Repository failure | `Error` | `op`, `err`, `tenant_id` |

> **Gap in current implementation:** The service currently holds a static `zerolog.Logger` field. Per `observability-standards.md` section 3, it should extract the request-scoped logger from context via `zerolog.Ctx(ctx)` to include `request_id` and `trace_id` automatically.

### Metrics

Per `observability-standards.md` section 4, the User domain must register:

```go
// internal/user/metrics.go

var (
    operationsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "conduit",
            Subsystem: "users",
            Name:      "operations_total",
            Help:      "Total user domain operations.",
        },
        []string{"operation", "status"},
    )

    operationDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "conduit",
            Subsystem: "users",
            Name:      "operation_duration_seconds",
            Help:      "User domain operation duration in seconds.",
            Buckets:   prometheus.DefBuckets,
        },
        []string{"operation"},
    )
)
```

**Operations:** `create`, `get`, `list`, `update`, `deactivate`, `reactivate`

**Status labels:** `success`, `not_found`, `conflict`, `error`

> **Gap in current implementation:** No domain metrics exist. A `metrics.go` file must be created.

### Tracing

Per `observability-standards.md` section 5, service methods must create child spans:

| Span Name | Attributes |
|---|---|
| `user.service.CreateUser` | `tenant_id` |
| `user.service.GetUser` | `tenant_id`, `user.id` |
| `user.service.ListUsers` | `tenant_id`, `page_size` |
| `user.service.UpdateUser` | `tenant_id`, `user.id` |
| `user.service.DeactivateUser` | `tenant_id`, `user.id` |
| `user.service.ReactivateUser` | `tenant_id`, `user.id` |

> **Gap in current implementation:** No OTel spans in the service layer.

### Health Considerations

The User domain has no independent health endpoint. It relies on the platform-level `/ready` check (database connectivity). If the database is down, all User operations will fail and the readiness check will report `not_ready`.

---

## 19. Performance Considerations

### Query Performance

| Operation | Expected Performance | Index Used |
|---|---|---|
| Get by ID | O(1) via PK | `users_pkey` |
| Get by email | O(1) via unique index | `idx_users_tenant_email` |
| List (filtered) | O(page_size) after index seek | `idx_users_tenant_status` |
| Count | O(N) worst case | `idx_users_tenant_status` |
| Create | O(1) + index maintenance | All indexes |
| Update | O(1) + index maintenance | `users_pkey` |

### Pagination

- Offset-based pagination is O(offset + limit) in PostgreSQL
- For v1 with max 100 per page, this is acceptable up to millions of users
- If `total` count becomes expensive at scale, it can be made optional or cached
- Cursor-based pagination should be introduced when notification delivery needs to iterate all users

### Connection Pooling

User domain shares the platform connection pool (`pgxpool`, min 2, max 10). No domain-specific pool tuning is needed at this scale.

---

## 20. Scalability Considerations

### Horizontal

| Scale Factor | Concern | Mitigation |
|---|---|---|
| Users per tenant | Query performance at 1M+ | Indexed queries, pagination capped at 100 |
| Number of tenants | Table size | All queries scoped by tenant_id (index-first) |
| Write throughput | Email unique index contention | Unique index is per-tenant (partitioned by tenant_id) |
| Read throughput | Repeated fetches of same user | Future: Redis caching when access pattern demands it |

### Vertical

- Single PostgreSQL instance handles millions of users easily
- Read replicas can be introduced behind the Repository interface without service changes
- Table partitioning by `tenant_id` is available if needed (transparent to application)

### Service Extraction

If the User domain needs to become a standalone service:

1. The `Repository` interface provides a clean seam for replacing PostgreSQL with gRPC calls
2. All domain logic is in `internal/user/` — copy the directory
3. The only external dependencies are `platform/errors`, `platform/tenant`, and `pgxpool`
4. Replace `PostgresRepository` with a gRPC client implementing the same `Repository` interface

---

## 21. Testing Strategy

### Unit Tests — `internal/user/service_test.go`

**Coverage target: >= 80%**

Test the service layer with a functional struct mock repository:

| Test | Category | What It Validates |
|---|---|---|
| `TestCreateUser_Success` | Happy path | Email normalization, display_name trim, status=active |
| `TestCreateUser_DuplicateEmail` | Error path | Returns `CONFLICT` |
| `TestCreateUser_NoTenant` | Error path | Returns `TENANT_REQUIRED` |
| `TestGetUser_NotFound` | Error path | Returns `NOT_FOUND` |
| `TestUpdateUser_EmailChange` | Business rule | Resets email_verified (**new test needed**) |
| `TestDeactivateUser_Success` | State transition | active -> deactivated, sets deactivated_at |
| `TestDeactivateUser_AlreadyDeactivated` | Invalid transition | Returns `CONFLICT` |
| `TestReactivateUser_Success` | State transition | deactivated -> active, clears deactivated_at |
| `TestReactivateUser_AlreadyActive` | Invalid transition | Returns `CONFLICT` |
| `TestStatusTransitions` | Domain rule | 9-case table-driven: all valid/invalid combinations |
| `TestEmailNormalization` | Domain rule | Uppercase, whitespace, mixed case |
| `TestListUsers_Defaults` | Defaults | page=1, per_page=20 |

> **Gap in current implementation:** Missing test for email change resetting `email_verified`.

### Model Tests — `internal/user/model_test.go` (new)

| Test | What It Validates |
|---|---|
| `TestStatus_IsValid` | Valid and invalid status strings |
| `TestUser_IsActive` | Returns true only for active status |
| `TestUser_CanTransitionTo` | All 9 state transition combinations |
| `TestListFilter_Offset` | Correct SQL offset calculation |

### Integration Tests — `tests/integration/user_test.go` (new)

**Build tag:** `//go:build integration`

| Test | What It Validates |
|---|---|
| `TestCreateAndGetUser` | Round-trip create + get against real DB |
| `TestEmailUniquenessPerTenant` | Same email in same tenant -> unique violation |
| `TestEmailUniquenessCrossTenant` | Same email in different tenants -> both succeed |
| `TestListUsersWithPagination` | Insert N users, verify pagination |
| `TestDeactivateAndReactivateUser` | Full lifecycle against real DB |
| `TestUpdateUserEmail` | Email change + uniqueness re-validation |
| `TestTenantIsolation` | User from tenant A invisible to tenant B |

> **Gap in current implementation:** No integration tests exist.

### Handler Tests — `internal/user/handler_test.go` (new)

| Test | What It Validates |
|---|---|
| `TestCreateHandler_ValidationErrors` | Missing email, invalid email -> 422 |
| `TestCreateHandler_InvalidJSON` | Malformed body -> 400 |
| `TestGetHandler_InvalidUUID` | Bad path param -> 400 |
| `TestListHandler_InvalidStatus` | Unknown status filter -> 400 |
| `TestUpdateHandler_NoFields` | Empty PATCH body -> 400 |

> **Gap in current implementation:** No handler tests exist.

---

## 22. Open Questions

| # | Question | Impact | Recommendation |
|---|---|---|---|
| OQ-1 | Should email enumeration be prevented on POST /users? | Security | Defer to Auth domain. Accept 409 for now; Auth can implement idempotent create. |
| OQ-2 | Should we add a `suspended_at` timestamp alongside `deactivated_at`? | Audit completeness | Defer. Suspension audit belongs in the Audit domain. |
| OQ-3 | Should metadata have a size limit? | Storage cost | Add `CHECK (pg_column_size(metadata) < 65536)` in a future migration if abused. |
| OQ-4 | Should the list endpoint support email search? | Discoverability | Defer. Add `?email=` filter when admin UI requires it. |
| OQ-5 | Should we expose SetEmailVerified as an API endpoint? | Verification flow | No. Auth domain will call the service method directly within the monolith. |
| OQ-6 | Should the email change trigger an email to the old address? | Security notification | Yes, but this belongs in the Notification domain, not User. |

---

## 23. Risks

### Architectural Risks

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| User domain becomes a "god object" with auth, org, billing logic | Medium | High | Strict domain boundaries enforced by code review. Document which domain owns each concern. |
| Email uniqueness constraint blocks future multi-login (e.g., social login) | Low | Medium | Social login creates auth_credentials, not new users. One user, multiple auth methods. |
| Offset pagination becomes slow at 10M+ users | Low | Medium | Switch to cursor-based when needed. API contract supports adding `next_cursor` field. |

### Operational Risks

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| Missing metrics delays incident detection | High (current gap) | Medium | PR-2 adds domain metrics |
| Missing tracing makes debugging cross-domain flows hard | High (current gap) | Medium | PR-2 adds OTel spans |
| No integration tests means SQL bugs ship to production | Medium (current gap) | High | PR-4 adds integration tests |

### Business Risks

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| Email change without re-verification creates security hole | High (current bug) | High | PR-3 fixes email_verified reset on email change |

---

## 24. Alternatives Considered

### Alternative 1: Separate `user_profiles` table

**Description:** Store mutable profile fields (display_name, metadata) in a separate table with FK to users.

**Rejected because:**
- Adds a JOIN to every user query
- No independent lifecycle — profile is always fetched with user
- Over-normalization for 2 fields
- Violates "simplicity over cleverness" principle

### Alternative 2: UUID with type prefix (e.g., `usr_550e8400...`)

**Description:** Prefix UUIDs with entity type for readability (Stripe-style).

**Deferred because:**
- Requires custom type, custom serialization, custom pgx scanning
- PostgreSQL UUID type doesn't support prefixes natively
- Adds complexity to every domain
- Can be introduced as a display format later without changing storage

### Alternative 3: Event sourcing for user state

**Description:** Store state transitions as events rather than mutable rows.

**Rejected because:**
- Massive complexity for a CRUD entity with 3 states
- Violates "boring technology" and "avoid unnecessary abstractions" principles
- Event sourcing is warranted for domains with complex workflows (notification delivery), not user profiles

### Alternative 4: Cursor-based pagination from day one

**Description:** Use `created_at` + `id` cursor instead of page/offset.

**Deferred because:**
- Offset is simpler and well-understood
- Admin UIs need "go to page 5" functionality
- Cursor-based will be added when notification delivery iterates all users
- API contract is forward-compatible (add `next_cursor` field)

---

## 25. Trade-offs

| Decision | Trade-off | Why We Accept It |
|---|---|---|
| Flat package (no sub-packages) | Less enforcement of layer boundaries | Go doesn't enforce intra-package deps. File naming provides sufficient layering. Fewer import paths. |
| Status-based soft delete (no `deleted_at`) | Can't distinguish "never deactivated" from "reactivated" | `updated_at` changes on every status change. Full status history belongs in Audit domain. |
| Inline SQL (not sqlc-generated) | Queries aren't validated at compile time | SQL is simple CRUD; pgx parameterization prevents injection. sqlc adds build tooling dependency. Trade-off documented for future evaluation. |
| Per-tenant email uniqueness | Same person can't have one global account | This is by design — multi-tenancy means tenants are isolated. Global accounts are a product decision for a future Identity domain. |
| No locale/timezone | Notification delivery can't schedule by timezone | YAGNI — delivery domain doesn't exist yet. Adding a column later is trivial. |
| Repository interface (only interface) | Testing seam adds indirection | Justified: enables 80%+ service coverage without a database. Service interface not needed until a second consumer exists. |

---

## 26. Future Evolution

### Phase 2: Authentication Domain

- Adds `password_hash` column to `users` or creates `auth_credentials` table
- Calls `User.GetByEmail()` for login
- Calls `User.SetEmailVerified()` after email confirmation
- Checks `User.IsActive()` before issuing tokens

### Phase 3: Organization Domain

- Creates `organizations` and `org_memberships` tables
- `org_memberships` has FK to `users.id`
- User domain unchanged

### Phase 4: RBAC Domain

- Creates `roles` and `user_roles` tables
- `user_roles` has FK to `users.id`
- User domain unchanged

### Phase 5: Notification Preferences

- Creates `notification_preferences` table with FK to `users.id`
- May add `timezone` column to `users` table
- User domain unchanged except possible migration

### Phase 6: Domain Events

- User domain publishes events: `user.created`, `user.updated`, `user.deactivated`, `user.reactivated`
- Service accepts an optional `EventPublisher` dependency
- Events consumed by Audit, Analytics, Notification domains

---

## 27. Pull Request Breakdown

### PR-1: Database and Domain Foundation

**Objective:** Schema, domain model, errors, repository interface + implementation.

**Files:**
- `migrations/000002_create_users.up.sql`
- `migrations/000002_create_users.down.sql`
- `sqlc/queries/user.sql`
- `internal/user/model.go`
- `internal/user/errors.go`
- `internal/user/repository.go`

**Testing:** `go build ./...` passes

**Review checklist:**
- [ ] Schema matches section 13 exactly
- [ ] All indexes present
- [ ] FK to tenants with ON DELETE RESTRICT
- [ ] CHECK constraint on status
- [ ] Repository returns typed domain errors
- [ ] `pgx.ErrNoRows` mapped to `ErrUserNotFound`
- [ ] Unique violation 23505 mapped to `ErrEmailAlreadyExists`

---

### PR-2: Service Layer + Observability

**Objective:** Business logic with metrics, tracing, and request-scoped logging.

**Files:**
- `internal/user/service.go`
- `internal/user/metrics.go` (**new**)

**Testing:** Service unit tests pass

**Review checklist:**
- [ ] Logger extracted from context: `zerolog.Ctx(ctx)`
- [ ] OTel spans on every service method
- [ ] Prometheus metrics registered (operations_total, operation_duration)
- [ ] Email normalization applied
- [ ] Status transitions validated
- [ ] Email change resets email_verified
- [ ] No PII in logs or span attributes

---

### PR-3: Transport Layer (DTOs + Handlers + Routes)

**Objective:** HTTP endpoints, validation, route registration.

**Files:**
- `internal/user/dto.go`
- `internal/user/handler.go`

**Testing:** Handler tests pass

**Review checklist:**
- [ ] All 6 endpoints registered
- [ ] Input validation at handler layer
- [ ] Errors mapped via `httputil.Error()`
- [ ] Response format matches `{"data": ...}` envelope
- [ ] Path UUIDs validated
- [ ] Query params have defaults and limits

---

### PR-4: Wiring + Integration Tests

**Objective:** Wire domain into app.go/server.go, add integration tests.

**Files:**
- `internal/app/app.go` (modified)
- `internal/server/server.go` (modified)
- `internal/user/service_test.go` (unit tests)
- `internal/user/model_test.go` (**new**)
- `internal/user/handler_test.go` (**new**)
- `tests/integration/user_test.go` (**new**)

**Testing:** `go test ./internal/user/...` + `go test -tags=integration ./tests/integration/...`

**Review checklist:**
- [ ] DI wiring: repo -> service -> handler -> server
- [ ] 80%+ service test coverage
- [ ] Integration tests cover: create, get, list, update, deactivate, reactivate
- [ ] Tenant isolation tested
- [ ] Email uniqueness tested (same tenant + cross-tenant)
- [ ] Handler tests cover validation errors and status codes

---

## 28. Approval Checklist

Before approving this RFC, the TDR committee should verify:

| # | Criterion | Status |
|---|---|---|
| 1 | Design is simple and avoids unnecessary abstractions | Verified |
| 2 | Domain boundaries are clearly defined | Verified |
| 3 | All business rules are explicit and justified | Verified |
| 4 | State transitions are documented and enforced | Verified |
| 5 | Database schema supports future domains without redesign | Verified |
| 6 | API design follows `api-guidelines.md` | Verified |
| 7 | Error model follows `coding-standards.md` three-tier hierarchy | Verified |
| 8 | Security threats are identified and mitigated | Verified |
| 9 | PII handling follows `security-standards.md` | Verified |
| 10 | Observability follows `observability-standards.md` (logs, metrics, traces) | Verified |
| 11 | Testing strategy covers unit, integration, and handler levels | Verified |
| 12 | Implementation can be broken into independently reviewable PRs | Verified |
| 13 | No speculative design or premature optimization | Verified |
| 14 | Trade-offs are documented with rationale | Verified |
| 15 | Open questions are identified for committee input | Verified |
| 16 | The design aligns with all 8 engineering standard documents | Verified |

---

> **Decision requested:** Approve this RFC to proceed with implementation per the PR breakdown in section 27.
