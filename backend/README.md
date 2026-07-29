# Conduit Notification Platform

Multi-tenant notification platform foundation. Built with Go, Gin, PostgreSQL, and Redis. This platform serves as the base for building robust, scalable notification services across different domains.

## Quick Start

Follow these steps to get the platform running locally:

```bash
1. Clone the repo
2. cp .env.example .env
3. make doctor        # Check prerequisites
4. make up            # Start Postgres + Redis
5. make migrate-up    # Apply database migrations
6. make dev           # Start the server with hot reload
7. curl localhost:8080/health
```

## Prerequisites

To build and run this project, you need the following dependencies installed:

- **Go**: Version 1.24+
- **Docker & Docker Compose**: For running local infrastructure (Postgres, Redis)
- **migrate CLI**: For applying database migrations (`golang-migrate/migrate`)
- **sqlc**: For generating type-safe Go code from SQL (`kyleconroy/sqlc`)
- **golangci-lint**: For linting Go code
- **air** *(optional)*: For live-reloading the server during development
- **gofumpt**: For stricter Go formatting

## Project Structure

This repository follows standard Go project layout conventions tailored for a modular monolith.

- `cmd/` - Application entry points (e.g., `cmd/server/main.go`).
- `internal/` - Private application code, not importable by other repositories.
  - `app/` - Application bootstrap and dependency wiring.
  - `config/` - Configuration structures and environment variable parsing.
  - `server/` - HTTP server setup and routing.
  - `platform/` - Cross-cutting infrastructure (database, cache, middleware, errors, observability, tenant).
  - `health/` - Health check endpoints (`/health`, `/ready`, `/live`).
  - `domain/` - Business domains (each domain will have its own folder here in the future).
- `pkg/` - Shared utilities that are fully decoupled from this specific application and could be extracted.
- `migrations/` - Database SQL migrations.
- `sqlc/` - SQL query definitions for `sqlc`.
- `deploy/` - Deployment configs (e.g., Dockerfiles, Kubernetes manifests).
- `docs/` - Documentation, architecture diagrams, and Architecture Decision Records (ADRs).
- `scripts/` - Development and build scripts.
- `tests/` - Integration and API tests.

## Development Commands

We use `make` to automate common development tasks.

| Command | Description |
|---|---|
| `make up` | Starts external dependencies (Postgres, Redis) via Docker Compose. |
| `make down` | Stops and removes external dependencies. |
| `make migrate-up` | Applies all pending database migrations. |
| `make migrate-down` | Reverts the last database migration. |
| `make sqlc` | Generates Go code from SQL queries using `sqlc`. |
| `make dev` | Starts the server with live reloading (requires `air`). |
| `make test` | Runs unit tests. |
| `make test-integration` | Runs integration tests (requires running database/redis). |
| `make lint` | Runs `golangci-lint` on the codebase. |
| `make doctor` | Checks if all required prerequisites are installed. |

## Configuration

The application is configured exclusively via environment variables.

| Variable | Default | Description |
|---|---|---|
| `ENV` | `development` | Application environment (`development`, `staging`, `production`). |
| `PORT` | `8080` | Port for the HTTP server to listen on. |
| `DATABASE_URL` | `postgres://user:pass@localhost:5432/conduit?sslmode=disable` | PostgreSQL connection string. |
| `REDIS_URL` | `redis://localhost:6379/0` | Redis connection string. |
| `LOG_LEVEL` | `info` | Minimum log level to output (`debug`, `info`, `warn`, `error`). |

## API Endpoints

The following platform-level endpoints are available:

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/health` | Basic application health status. |
| `GET` | `/ready` | Checks if all dependencies (DB, Redis) are ready. |
| `GET` | `/live` | Liveness check for orchestration systems (e.g., K8s). |
| `GET` | `/metrics` | Prometheus metrics scrape endpoint. |

## Architecture

For a detailed overview of the system architecture, please see the [Architecture Document](docs/ARCHITECTURE.md).

## Adding a New Domain

To add a new business domain to the platform:

1. Create a new directory under `internal/domain/<name>/`.
2. Define the domain types, service interface, and repository interface.
3. Register the new HTTP routes in `internal/server/server.go`.
4. Add any necessary database migrations to the `migrations/` folder.
5. Add SQL queries to the `sqlc/` folder and run `make sqlc`.

## Testing

- **Unit Tests**: Run with `make test`. These test isolated logic and do not require external dependencies.
- **Integration Tests**: Run with `make test-integration`. These test the interaction with the database and cache and require `make up` to be running.

## License

TBD
