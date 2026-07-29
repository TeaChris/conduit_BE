# ADR-002: Environment Variable Configuration

## Status
Accepted

## Date
2026-07-23

## Context
Need config that works across local/dev/staging/prod. Must fail fast. Must never log secrets.

## Decision
Use environment variables only (12-factor). github.com/caarlos0/env for struct-tag parsing. Validate at startup. No config files, no Viper.

## Consequences
### Positive
- Simple, portable, works natively with Docker and Kubernetes.
- Explicit over implicit configuration.
- Fail-fast mechanism ensures the app doesn't start with missing required config.
### Negative
- No hot reload of config without restarting the process.
### Risks
- Large number of environment variables could become unwieldy. Mitigation: group variables logically in structs.
