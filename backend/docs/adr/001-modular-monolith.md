# ADR-001: Modular Monolith Architecture

## Status
Accepted

## Date
2026-07-23

## Context
Need to ship fast without premature microservices complexity. Team is small. Domains are not yet well understood.

## Decision
Start as modular monolith with clear domain boundaries. Each domain in internal/domain/<name>/ with its own handler, service, repository. Platform infrastructure shared via internal/platform/.

## Consequences
### Positive
- Fast iteration, single deployment, easy debugging.
- Less operational overhead (no distributed tracing required immediately, single CI/CD pipeline).
- Easier refactoring of domain boundaries before they are carved in stone as separate services.
### Negative
- Risk of coupling if boundaries aren't enforced.
### Risks
- Developers might bypass interfaces and access other domains directly or query their database tables. Mitigation: code reviews and strict adherence to architecture guidelines.
