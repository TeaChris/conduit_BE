# Conduit Notification Platform: Go Coding Standards

## 1. Purpose
This document establishes the official Go coding standards for the Conduit Notification Platform backend. The objective is to maintain a high-quality, predictable, and maintainable codebase. Consistency across our Go modular monolith enables any engineer to jump into a new domain package and understand the architecture, data flow, and error handling immediately.

## 2. Naming Conventions

### Go Naming Rules
- Use `PascalCase` for exported identifiers (functions, types, variables, constants).
- Use `camelCase` for unexported identifiers.
- Prefer concise, descriptive names.
- Avoid repeating the package name in the identifier (e.g., `user.User` is better than `user.UserModel`).

### Package Naming
- Keep package names short, singular, and lowercase.
- Do not use underscores or mixedCaps in package names.
- Package names should reflect their purpose (e.g., `user`, `notification`, `http`).

### File Naming
- Use `snake_case.go` for all file names.
- Example: `repository.go`, `user_service.go`, `http_handler.go`.

### Variable Naming
- Use single-letter or short acronyms for receiver variables (e.g., `u *User`).
- Use descriptive names for long-lived variables and shorter names for short-lived ones (e.g., `i` in loops).
- Return errors as `err`.

### Constant Naming
- Use `PascalCase` for exported constants, `camelCase` for unexported.
- Do not use `ALL_CAPS` or `SNAKE_CASE` (except for specific cases like database column names represented as strings).

### Interface Naming
- Do not prefix interfaces with `I`.
- For single-method interfaces, use the `-er` suffix (e.g., `Reader`, `Writer`, `Validator`).
- For multi-method interfaces, use a descriptive noun (e.g., `Repository`, `Store`).

### Error Variable Naming
- Prefix predefined error variables with `Err` (e.g., `ErrNotFound`, `ErrInvalidInput`).
- Error types should have an `Error` suffix (e.g., `ValidationError`).

### Test Naming
- Use the format `Test<Function>_<Scenario>`.
- Example: `TestCreateUser_DuplicateEmail`, `TestGetUserByID_NotFound`.

## 3. Package Conventions

- **One Package Per Directory**: Each directory should contain exactly one Go package.
- **No Circular Imports**: Architect your code to avoid circular dependencies. Use interfaces or reorganize packages if you encounter one.
- **`internal/` for Domain Logic**: All application-specific code must reside within the `internal/` directory to prevent external imports.
- **`pkg/` for Shared Utilities**: Generic, highly reusable utilities (not tied to business logic) can go in `pkg/`.
- **No Package-Level State**: Avoid global variables (`var`). Configuration and state should be injected as dependencies.

## 4. File Organization

Within an `internal/<domain>` package, organize files logically:
- `model.go`: Domain entities and core types.
- `errors.go`: Domain-specific error definitions and constructors.
- `repository.go`: Database interactions and Repository interfaces.
- `service.go`: Core business logic.
- `handler.go`: HTTP endpoints and request/response mapping.
- `dto.go`: Request and response payload structures.

### Import Ordering
Group imports into three distinct blocks separated by a blank line:
1. Standard library packages.
2. External/third-party packages.
3. Internal project packages.

```go
import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/conduit-platform/conduit/backend/internal/user"
)
```

### File Length
Keep files focused. If a file exceeds 400-500 lines, consider splitting it by logical grouping (e.g., `service.go` into `service_create.go`, `service_get.go`).

## 5. Function Design

- **Context First**: `context.Context` must always be the first parameter in any function that performs I/O or takes significant time.
- **Return Errors**: Always return errors. Never use `panic` for standard error flows.
- **Short Functions**: Aim for functions under 40 lines. Extract complex logic into helper methods.
- **Single Responsibility**: A function should do one thing well.
- **Named Returns**: Use named return values only when they significantly improve documentation or when using `defer` to modify the return error. Avoid naked returns.

```go
// Bad
func calculate(x int) (res int, err error) {
    res = x * 2
    return
}

// Good
func calculate(x int) (int, error) {
    return x * 2, nil
}
```

## 6. Interface Design

- **Small Interfaces**: Interfaces should describe behavior, not data structures. The smaller the interface, the more reusable it is.
- **Accept Interfaces, Return Structs**: Functions should be liberal in what they accept (interfaces) and conservative in what they return (concrete types).
- **Define at Point of Use**: Define interfaces where they are consumed, not where they are implemented, unless it's a standard interface meant for wide reuse.

### Repository Interface Example

```go
// Defined in internal/user/repository.go
type Repository interface {
	GetByID(ctx context.Context, id string) (*User, error)
	Create(ctx context.Context, user *User) error
	Update(ctx context.Context, user *User) error
}

// Struct returns a concrete type, but the service accepts the interface
type postgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *postgresRepository {
	return &postgresRepository{db: db}
}
```

## 7. Error Handling

Conduit uses a strict three-tier error model to handle failures predictably:
1. **DomainError**: Business logic violations (e.g., insufficient balance). Maps to 4xx status codes.
2. **ValidationError**: Input validation failures. Maps to 422 Unprocessable Entity with detailed field lists.
3. **InfraError**: Database, network, or internal failures. Maps to 500 Internal Server Error. The exact error is masked from the client.

### Constructing Errors
Use `errors.go` in your domain to define constructors.

```go
// internal/user/errors.go
var ErrUserNotFound = NewDomainError("NOT_FOUND", "User could not be found")

func NewDomainError(code, message string) *DomainError { /* ... */ return nil }
```

### Contextual Error Wrapping
Wrap external or infra errors with context using `%w`.

```go
if err := r.db.QueryRow(ctx, query, id).Scan(&u); err != nil {
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrUserNotFound
    }
    return nil, fmt.Errorf("repository.GetByID query failed: %w", err)
}
```

### Unique Violations
Map PostgreSQL unique constraint violations (code 23505) to Domain Errors (Conflict).

```go
// Example pseudo-code for mapping pgx error
// if pgErr, ok := err.(*pgconn.PgError); ok && pgErr.Code == "23505" {
//     return ErrEmailAlreadyExists
// }
```

### Anti-Pattern: Log and Return
Never log an error and then return it. This causes duplicate log entries. Let the top-most caller (e.g., the HTTP handler or middleware) log the error.

## 8. Logging Standards

We use `zerolog` for high-performance, structured JSON logging.

### Request-Scoped Logging
Always retrieve the logger from the context to ensure `request_id` and `trace_id` are automatically included.

```go
// Good
logger := zerolog.Ctx(ctx)
logger.Info().Str("user_id", userID).Msg("User logged in successfully")
```

### Log Levels
- **Debug**: Detailed troubleshooting information.
- **Info**: Notable system events (e.g., "User created", "Job started").
- **Warn**: Recoverable errors or unexpected conditions that don't halt execution.
- **Error**: Operation failures requiring investigation (InfraErrors).

### Required Fields
When logging business operations, include:
- `user_id` (if authenticated)
- `tenant_id`
- `operation` (e.g., `user_creation`)

### PII Rules (Personally Identifiable Information)
**Never log**:
- Full email addresses (log `email_domain` instead).
- Passwords, tokens, API keys.
- Connection strings with credentials.
- Financial or raw metadata values.

### Structured Logging Only
Do not use `fmt.Sprintf` to build log messages. Use strongly typed fields.

```go
// Bad
// logger.Info().Msg(fmt.Sprintf("User %s created in tenant %s", userID, tenantID))

// Good
logger.Info().
    Str("user_id", userID).
    Str("tenant_id", tenantID).
    Msg("User created")
```

## 9. Context Usage

- **Always Pass Context**: Pass `context.Context` down the call stack. Do not store it in structs.
- **Cancellation & Deadlines**: Respect context cancellation in long-running operations or database calls.
- **Tenant ID**: Retrieve the tenant ID using our standard helper: `tenantID := tenant.GetID(ctx)`.
- **Logger**: Retrieve the request-scoped logger: `logger := zerolog.Ctx(ctx)`.
- **No Business Data**: Context is for request-scoped metadata (auth, logging, tracing), not for passing business parameters.

## 10. Constants and Configuration

- **Typed Constants**: Create custom types for enumerations and use them as constants.
```go
type Status string
const (
    StatusActive   Status = "active"
    StatusInactive Status = "inactive"
)
```
- **Configuration**: Use `caarlos0/env` to parse environment variables into configuration structs on startup. Do not use `os.Getenv` scattered throughout the codebase.
- **No Magic Strings**: Define constants for reused strings.
- **No Global Mutable State**: Pass configuration down via dependencies.

## 11. Go Idioms

- **Table-Driven Tests**: Use slice-of-structs to define test cases for comprehensive unit testing.
- **Functional Options**: Use the functional options pattern for complex struct initialization where many parameters are optional.
- **Early Returns**: Avoid deep nesting. Handle errors and edge cases early and return.
```go
// Good
if err != nil {
    return err
}
// Proceed with happy path
```
- **Initialization**: Use `var` to declare zero-value variables (`var users []User`). Use `:=` when initializing with a value (`users := make([]User, 0, 10)`).
- **Slices**: Use `make` with capacity when the final size of the slice is known to avoid reallocation.

## 12. Performance Guidelines

- **Avoid Premature Optimization**: Write clean, readable code first. Profile using `pprof` before optimizing.
- **Connection Pooling**: Use `pgxpool` and configure `MaxConns`, `MinConns`, and `MaxConnLifetime` appropriately.
- **Pagination**: Never return unbounded lists from the database. Always use `LIMIT` and `OFFSET`.
- **Timeouts**: Set reasonable timeouts on all network calls and database queries via context.
- **Pre-allocate**: `results := make([]Result, 0, len(items))` when mapping slices.

## 13. Things to Always Do

1. **Always** check for errors.
2. **Always** use `context.Context` as the first argument.
3. **Always** map database errors to Domain Errors or Infra Errors at the Repository boundary.
4. **Always** log structurally using `zerolog.Ctx(ctx)`.
5. **Always** write table-driven tests for core logic.
6. **Always** close resources (HTTP bodies, rows, files) using `defer` immediately after successful acquisition.

## 14. Things to Never Do

1. **Never** log PII or sensitive secrets.
2. **Never** ignore errors using `_` (unless explicitly documented why it's safe).
3. **Never** use `panic` in production business logic.
4. **Never** log an error and then return it.
5. **Never** create circular package dependencies.
6. **Never** bypass the Repository layer to access the database from HTTP handlers.
7. **Never** write generic SQL queries without `pgx` parameterization (SQL injection risk).

## 15. Checklist

- [ ] Does every function take `context.Context`?
- [ ] Are all external/database errors wrapped with context?
- [ ] Are PostgreSQL constraint errors properly mapped to Domain errors?
- [ ] Is logging structured and free of sensitive PII/secrets?
- [ ] Are large lists paginated at the database level?
- [ ] Is there no package-level state (`var`)?
- [ ] Are interfaces defined where they are used?
- [ ] Does the file use the standard import block ordering?
