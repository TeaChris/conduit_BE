# Conduit Notification Platform: API Guidelines

## 1. Purpose
This document outlines the API design standards for the Conduit Notification Platform. Our goal is to provide a consistent, intuitive, and predictable RESTful interface for all internal clients and external integrations. Adherence to these guidelines ensures backwards compatibility, robust error handling, and a seamless developer experience.

## 2. REST Principles

- **Resources as Nouns**: Endpoints should represent entities (nouns), not actions (verbs).
- **Pluralization**: Always use plural nouns for collections (e.g., `/users`, not `/user`).
- **Nesting**: Use nested resources to represent parent-child relationships, but keep it shallow (maximum one level of nesting is preferred).
- **Stateless**: Every request must contain all the information necessary to fulfill it. The server should not rely on stored session context.

## 3. URL Design

- **Base Path**: `/api/v1/<resource>`
- **Naming format**: Use lowercase, kebab-case for URL segments (e.g., `/api/v1/notification-templates`).

### Examples for CRUD Operations
- **List**: `GET /api/v1/users`
- **Create**: `POST /api/v1/users`
- **Read**: `GET /api/v1/users/:id`
- **Update**: `PATCH /api/v1/users/:id`
- **Delete**: `DELETE /api/v1/users/:id`

### Action Endpoints
If a state change doesn't map cleanly to CRUD, use an action endpoint appended to a specific resource.
- **Format**: `POST /api/v1/<resource>/:id/<action>`
- **Example**: `POST /api/v1/users/:id/deactivate`
- **Rule**: Verbs are only permitted as action suffixes on resource instances.

## 4. HTTP Methods

| Method | Use Case | Idempotent |
|--------|----------|------------|
| **GET** | Retrieve a resource or a list of resources. Never alters state. | Yes |
| **POST** | Create a new resource or execute a specific action. | No |
| **PATCH** | Partially update an existing resource. | Yes |
| **DELETE** | Remove a resource. | Yes |

*Note: We do not use `PUT` for updates. All updates should use `PATCH` to allow partial modifications without requiring the client to send the full resource state.*

## 5. Status Codes

We restrict the set of standard HTTP status codes to maintain predictability.

| Code | Status | Description |
|------|--------|-------------|
| **200** | OK | Request succeeded (GET, PATCH, DELETE action). |
| **201** | Created | Resource was successfully created (POST). |
| **204** | No Content | Request succeeded, but no body is returned (often used for DELETE). |
| **400** | Bad Request | Malformed syntax or missing mandatory headers. |
| **401** | Unauthorized | Missing or invalid authentication credentials. |
| **403** | Forbidden | Authenticated, but lacks permission to access the resource. |
| **404** | Not Found | The requested resource does not exist. |
| **409** | Conflict | State conflict (e.g., duplicate unique field like email). |
| **422** | Unprocessable Entity | Validation failed on the request payload. |
| **429** | Too Many Requests | Rate limit exceeded. |
| **500** | Internal Server Error | Generic server failure. Masked from the client. |

## 6. Request/Response Format

All API endpoints must communicate exclusively using JSON.
- **Header**: `Content-Type: application/json` must be set on all responses.

### Success Response
Successful single-resource responses must wrap the payload in a `"data"` key.
```json
{
  "data": {
    "id": "usr_12345",
    "email": "user@example.com",
    "status": "active"
  }
}
```

### Success List Response
When returning collections, include standard pagination metadata alongside the data array.
```json
{
  "data": {
    "users": [
      {
        "id": "usr_12345",
        "email": "user@example.com"
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

### Error Response
Errors must adhere to a strict structure allowing programmatic handling.
```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "The request payload is invalid.",
    "details": [
      {
        "field": "email",
        "message": "is required and must be a valid email format"
      }
    ]
  }
}
```

## 7. Pagination

All endpoints returning lists must be paginated to prevent memory exhaustion and slow response times.
- **Type**: Offset-based pagination using query parameters.
- **Parameters**: `?page=<number>&per_page=<number>`
- **Defaults**: `page=1`, `per_page=20`
- **Maximum**: `per_page` cannot exceed 100.
- **Future Note**: For high-volume transaction tables (e.g., audit logs, event streams), we will transition to cursor-based pagination.

## 8. Filtering and Sorting

### Filtering
Pass filtering options via query parameters.
- `GET /api/v1/users?status=active&tenant_id=tnt_123`

### Sorting
Use `sort` and `order` query parameters.
- `GET /api/v1/users?sort=created_at&order=desc`
- **Default**: `created_at DESC` (newest first).

## 9. Versioning

- **Strategy**: URL path versioning (`/api/v1/`, `/api/v2/`). We do not use header-based versioning.
- **Policy**: Introduce a new version (e.g., `v2`) only when making backwards-incompatible (breaking) changes. Additive changes (new fields, new endpoints) go into the current version.

## 10. Authentication & Authorization

- **Current**: Multi-tenancy is enforced via the `X-Tenant-ID` header. Handlers must extract this and inject it into the context.
- **Future**: Authentication will be handled via a Bearer token in the `Authorization` header. Server-to-server communication will utilize `X-API-Key`.
- Handlers should assume the context contains valid tenant/user data if the request passes the auth middleware.

## 11. Validation

- **Layering**: Input validation (schema, required fields, formatting) happens in the HTTP handler layer. Business rule validation (state transitions, permissions) happens in the service layer.
- **Status Code**: Validation failures return `422 Unprocessable Entity`.
- **Field Error Format**:
```json
{
  "field": "email",
  "message": "is required"
}
```
- **Path and Query Params**: Explicitly validate path variables (e.g., ensuring they are valid UUIDs/ULIDs) and query parameters (numeric boundaries, valid enums).

## 12. Idempotency

- **Concept**: Idempotency ensures that making multiple identical requests has the same effect as making a single request.
- **Naturally Idempotent**: `GET`, `PATCH`, `DELETE`.
- **Future**: `POST` requests, which are not naturally idempotent, will support an `Idempotency-Key` header to safely retry creation operations without risking duplication.

## 13. Rate Limiting

- **Implementation**: Enforced globally via middleware using Redis.
- **Headers Returned**:
  - `X-RateLimit-Limit`: Total allowed requests per window.
  - `X-RateLimit-Remaining`: Remaining requests in the current window.
  - `X-RateLimit-Reset`: Unix timestamp when the limit resets.
- **Exceeding Limits**: Return `429 Too Many Requests`.

## 14. Error Response Reference

| Code | HTTP | Description | Example Trigger |
|------|------|-------------|-----------------|
| `BAD_REQUEST` | 400 | Malformed request or JSON. | Invalid JSON syntax in payload. |
| `UNAUTHORIZED` | 401 | Missing or invalid auth. | Expired token. |
| `FORBIDDEN` | 403 | Lack of permissions. | User trying to delete a resource they don't own. |
| `NOT_FOUND` | 404 | Resource does not exist. | Querying an ID that is not in the database. |
| `CONFLICT` | 409 | Resource state conflict. | Creating a user with an email that already exists. |
| `VALIDATION_ERROR` | 422 | Payload failed rules check. | Missing required field `name`. |
| `RATE_LIMITED` | 429 | Rate limit exceeded. | Burst of 1000 requests in 1 second. |
| `INTERNAL_ERROR` | 500 | Unexpected backend failure. | Database connection lost. |
| `TENANT_REQUIRED` | 400 | Missing tenant context. | Omitting the `X-Tenant-ID` header. |

## 15. Breaking Changes Policy

**What constitutes a breaking change?**
- Removing an endpoint.
- Removing a field from a response payload.
- Changing the data type of a field.
- Adding a required field to a request payload without a default.

**Process**:
- Breaking changes require a new API version path (e.g., `/v2/`).
- The older version must remain supported for a minimum of 6 months (deprecation period).
- Deprecated endpoints must log a warning or increment a metric to track usage before final removal.

## 16. OpenAPI

- **Documentation First**: Every endpoint must be documented in `docs/openapi.yaml`.
- **Sync**: The OpenAPI specification must perfectly reflect the implementation.
- **Reusability**: Use `$ref` to define reusable components (like the Error Response model, Pagination model, and common entities).

## 17. Anti-Patterns

- **Nesting too deeply**: e.g., `/api/v1/users/:id/posts/:post_id/comments/:comment_id`. Just use `/api/v1/comments/:comment_id`.
- **Returning raw internal errors**: Never expose SQL queries, stack traces, or internal framework errors to the client.
- **200 OK for Errors**: Never return a 200 HTTP status code with an error payload inside the body. Use appropriate HTTP status codes.

## 18. Checklist

- [ ] Does the URL use plural nouns?
- [ ] Is the HTTP method correct for the operation?
- [ ] Does the successful response use the `{"data": ...}` wrapper?
- [ ] Are lists paginated?
- [ ] Do validation errors return `422` with field-level details?
- [ ] Are date/times returned in standard ISO-8601 format (RFC3339)?
- [ ] Is the endpoint fully documented in `openapi.yaml`?
- [ ] Are unique constraints mapped to `409 Conflict` instead of `500`?
