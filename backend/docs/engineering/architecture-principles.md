# Architecture Principles

## 1. Purpose
This document defines the architectural boundaries and conventions for the Conduit Notification Platform. It provides a blueprint for how we structure our Go codebase, how domains communicate, and how we ensure the system remains maintainable as we scale.

## 2. Modular Monolith Philosophy
We are building a **Modular Monolith**. 

*   **Why Monolith First?** Microservices introduce network boundaries, distributed tracing complexity, complex transaction management, and deployment overhead. We want the velocity of a single deployable unit while maintaining the discipline of isolated domains.
*   **Domain Boundaries:** The application is split into business domains (e.g., `user`, `notification`, `billing`). Each domain operates as if it were a microservice, but they run in the same process.
*   **Module Communication Rules:** Domains cannot reach into the data store of another domain. They must communicate via well-defined Go interfaces (public service methods) or, in the future, domain events.
*   **When to Extract a Service:** We extract a service only when there is a divergent scaling requirement (e.g., the worker processing notifications requires vastly different CPU/Memory profiles than the HTTP API) or an isolated security perimeter requirement.
*   **How to Extract a Service:** Because we enforce strict domain boundaries in the monolith, extracting a service should theoretically only require wrapping the existing domain service interface in a transport layer (e.g., gRPC or HTTP) and deploying it separately.

## 3. Clean Architecture
We adhere to a simplified Clean Architecture model. Dependencies must always point INWARD toward the Domain layer.

```text
+-------------------------------------------------------+
|  Transport / Delivery Layer (HTTP/Gin, CLI, Workers)  |
|  (Depends on Application)                             |
+-------------------------------------------------------+
                           |
                           v
+-------------------------------------------------------+
|  Application Layer (Services, Orchestration)          |
|  (Depends on Domain and Interfaces of Infrastructure) |
+-------------------------------------------------------+
                           |
                           v
+-------------------------------------------------------+
|  Domain Layer (Entities, Rules, Errors)               |
|  (Depends on NOTHING)                                 |
+-------------------------------------------------------+

+-------------------------------------------------------+
|  Infrastructure Layer (PostgreSQL, Redis, External)   |
|  (Depends on Domain, implements Application interfaces)|
+-------------------------------------------------------+
```

*   **Domain Layer:** Core business models and domain errors. No external libraries.
*   **Application Layer:** Use cases and business orchestration. Depends on repositories via interfaces.
*   **Infrastructure Layer:** Specific implementations (pgx, redis, external APIs).
*   **Transport Layer:** HTTP handlers (Gin), parsing JSON, validating input, writing responses.

## 4. Domain-Driven Design Boundaries
*   **Bounded Contexts:** Each `internal/<domain>` package represents a bounded context.
*   **Aggregates:** Data mutations must happen through Aggregate Roots. Do not save child entities directly to the database bypassing the parent entity's invariants.
*   **Domain Events (Future):** When state changes in one domain that another domain cares about, we will emit an internal event (and eventually Kafka messages) rather than tightly coupling the services.
*   **Ubiquitous Language:** Class names, variables, and database tables must match the language used by Product and Business stakeholders.
*   **Anti-Corruption Layers:** When integrating with external systems (e.g., Twilio, SendGrid), wrap their SDKs in our own interfaces. Do not let their data models leak into our domain.

## 5. Package Organization
We use a **flat package structure** per domain to avoid deep nesting and artificial separation of related concepts.

```
internal/<domain>/
    model.go        ← Domain layer: Core structs (e.g., User, Notification)
    errors.go       ← Domain layer: DomainError definitions
    repository.go   ← Infrastructure: Repository interface and PostgreSQL/pgx implementation
    service.go      ← Application layer: Business logic, orchestrates repos
    handler.go      ← Transport layer: Gin HTTP handlers, parses requests
    dto.go          ← Transport layer: Request/Response structs, mapping logic
```

*   **File Naming Rules:** Stick to the standard names above. If a file gets too large, split by entity (e.g., `service_user.go`, `service_preferences.go`), but keep them in the same flat package.
*   **What Goes Where:** Never put HTTP knowledge (status codes, JSON tags) in `model.go`. Never put SQL queries in `service.go`.

## 6. Dependency Injection
We do not use reflection-based DI frameworks (e.g., Wire, Dig). We use **explicit constructor injection**.

*   Wiring happens strictly in `cmd/server/app.go`.
*   Every component must define its dependencies via interfaces (except for pure data structs).

**Example:**
```go
// internal/notification/service.go
type Repository interface {
    Create(ctx context.Context, n *Notification) error
}

type Service struct {
    repo   Repository
    logger *zerolog.Logger
}

func NewService(repo Repository, logger *zerolog.Logger) *Service {
    return &Service{
        repo:   repo,
        logger: logger,
    }
}
```

## 7. Layer Responsibilities

| Layer | Allowed Imports | Responsibilities | Forbidden |
| :--- | :--- | :--- | :--- |
| **Domain** | Standard library only | Structs, business invariants, domain errors (`errors.go`) | JSON tags, SQL, HTTP concepts, 3rd party libs |
| **Application** | Domain, Platform | Business orchestration, transaction boundaries, caching logic | SQL queries, HTTP Request/Response reading |
| **Infrastructure** | Domain, pgx, Redis | Implementing repo interfaces, writing SQL (sqlc), API calls | Enforcing core business rules |
| **Transport** | Domain, Application, Gin | Parsing JSON, checking auth, calling Service, writing JSON | Direct database access, business logic |

## 8. Cross-Domain Communication
*   **Direct function calls:** Because we are a monolith, domains can call each other directly via exported Service interfaces.
*   **Never import another domain's internal types:** Do not import `internal/user/model.go` into `internal/notification/repository.go`.
*   **Share via interfaces:** If `notification` needs user data, it defines an interface `UserProvider` in its own package. The `user.Service` implements it. `app.go` wires them together.
*   **Platform packages:** Use `internal/platform/` for truly cross-cutting concerns (e.g., `logger`, `telemetry`, `database`).

## 9. Service Extraction Strategy
*   **When extraction is justified:** Different scaling profiles, strictly different security compliance requirements, or team sizes exceeding 20 engineers on a single domain.
*   **How to prepare:** Ensure the domain has zero direct database queries to other domains. Ensure its Service interface is well-defined.
*   **Steps to extract:**
    1. Define a network contract (gRPC/OpenAPI) that mirrors the Service interface.
    2. Create a new deployment repository/pipeline.
    3. Swap the DI wiring in `app.go` of the monolith to inject an HTTP/gRPC client implementation of the interface instead of the local implementation.

## 10. Platform Packages
*   **Purpose:** The `internal/platform/` directory is for infrastructural code that is not specific to any business domain.
*   **What belongs there:** `logger` (zerolog setup), `database` (pgx pool initialization), `telemetry` (OpenTelemetry and Prometheus setup), `config` (env var parsing via caarlos0/env/v11).
*   **What does NOT belong there:** Anything related to Users, Notifications, Billing, or any business logic.

## 11. Future Considerations
*   **Event-Driven Architecture:** As the monolith grows, we will introduce an internal event bus (and later Kafka) to reduce synchronous coupling between domains.
*   **CQRS:** Command Query Responsibility Segregation will be introduced only for high-read-volume domains (e.g., retrieving notification history) where a separate read model is justified.
*   **Microservice Boundaries:** The boundaries defined by our `internal/<domain>` packages will serve as the exact boundaries if we ever transition to microservices.

## 12. Anti-Patterns
*   **Entity Leaking:** Returning database models (with `sql.NullString` or `pgtype`) directly to the HTTP handler to be serialized. Always map Domain models to DTOs.
*   **Framework Coupling:** Passing `*gin.Context` into the Application (Service) layer. The Service layer must only know about standard `context.Context`.
*   **Mocking Libraries:** Using external mocking frameworks like Testify/Mock. Use standard table-driven tests and functional structs for mocking (e.g., `type MockRepo struct { CreateFunc func(...) }`).
*   **Bypassing the Tenant ID:** Failing to extract the Tenant ID from the context (set by the `X-Tenant-ID` header middleware) in the repository layer. Every query must be tenant-scoped.

## 13. Checklist
- [ ] Is all DI explicit in `app.go`?
- [ ] Are domains strictly isolated from each other's databases?
- [ ] Are all standard responses formatted as `{"data": ...}` or `{"error": ...}`?
- [ ] Are errors correctly typed as `DomainError`, `ValidationError`, or `InfraError`?
- [ ] Is `*gin.Context` strictly contained within `handler.go`?
- [ ] Does the infrastructure layer rely on interfaces defined by the application layer?
- [ ] Are SQL queries written securely with parameterized inputs via `pgx` or `sqlc`?
