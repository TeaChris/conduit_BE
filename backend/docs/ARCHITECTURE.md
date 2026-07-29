# Architecture Overview

The Conduit Notification Platform follows a **Modular Monolith** architecture with clear layers based on Clean Architecture principles. It is designed to allow rapid iteration while preventing spaghetti dependencies, setting us up for future Domain-Driven Design (DDD) domain extraction if required.

## 1. Overview

I have chosen a modular monolith approach. This means the entire application runs as a single deployable process, but the codebase is strictly segregated into independent domains that communicate through well-defined interfaces.

## 2. Layer Diagram

```mermaid
graph TD
    cmd[cmd/server (Entry Point)] --> app[internal/app (Bootstrap)]
    app --> server[internal/server (HTTP)]
    server --> middleware[internal/platform/middleware]
    server --> health[internal/health]
    
    %% Future Domains
    server -.-> domainA[internal/domain/users]
    server -.-> domainB[internal/domain/notifications]
    
    domainA -.-> platformDB[internal/platform/database]
    domainB -.-> platformDB
    
    middleware --> platformCore[internal/platform/* (errors, logger)]
    domainA --> platformCore
    domainB --> platformCore
```

## 3. Dependency Flow

The golden rule of this architecture is that **dependencies flow inward**. 

- **Domain Logic**: The core business rules and models reside in the center. They do NOT depend on HTTP frameworks, databases, or external APIs.
- **Infrastructure Adapters**: Code that interacts with the outside world (e.g., PostgreSQL repositories, HTTP handlers) are adapters that implement interfaces defined by the domain.
- **Platform**: `internal/platform/` contains shared infrastructure utilities (database connections, logging, observability) that can be used across all domains.

## 4. Adding a Domain

When adding a new domain, follow this directory structure:

```
internal/domain/<name>/
├── handler.go    # HTTP handlers (adapter - depends on HTTP framework and service)
├── service.go    # Business logic (depends on repository interface)
├── repository.go # Data access interface (pure Go interfaces)
├── postgres.go   # PostgreSQL implementation of repository (adapter)
└── model.go      # Domain types and structs
```

## 5. Service Extraction Path

If a domain becomes large enough to warrant its own microservice, the extraction path is:

1. Ensure the domain already has clear boundaries and communicates with other domains via interfaces, not direct database access.
2. Move the `internal/domain/<name>/` package to a new repository/service.
3. Packages in `internal/platform/` can be extracted into a shared library repository if needed.
4. Replace direct interface calls between the extracted domain and the monolith with REST API or gRPC calls.

## 6. Multi-Tenancy

The platform is designed to be multi-tenant from the ground up.
- The `tenant_id` is extracted from incoming requests (e.g., via an `X-Tenant-ID` header) by a middleware.
- It is placed into the `context.Context`.
- All domain services and repository methods MUST extract the `tenant_id` from the context and enforce it in queries.

## 7. Error Handling Flow

We have a strict hierarchy for error handling:
- **Domain Errors**: Created by business logic (e.g., `ErrInvalidState`).
- **API Errors**: Created by HTTP handlers mapping Domain errors or request validation issues to HTTP concepts.
- **HTTP Responses**: The final JSON response sent to the client.

*Flow*: Domain Service -> Returns Error -> HTTP Handler maps to API Error -> Middleware/Response Helper formats JSON.

## 8. Middleware Stack

The HTTP server executes middleware in the following order:

1. **Recovery**: Catches panics and returns 500.
2. **RequestID**: Injects a unique ID into the context and response headers.
3. **Trace/Observability**: Starts OpenTelemetry spans for the request.
4. **Logger**: Logs incoming requests and outgoing responses with durations and status codes.
5. **Tenant**: Extracts `X-Tenant-ID` and injects it into the context.
6. **Authentication**: (Future) Validates JWTs.
7. **Rate Limiter**: (Future) Prevents abuse per tenant/IP.
