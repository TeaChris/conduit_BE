# Conduit Engineering Standards: Security

## 1. Purpose
This document defines the security standards, practices, and architectural principles for the Conduit Notification Platform. By following these guidelines, engineering teams ensure the confidentiality, integrity, and availability of user data while maintaining a robust defense against common vulnerabilities. These standards apply to all code contributions, architectural designs, and deployment configurations.

## 2. Security Philosophy
Our approach to security is guided by the following core principles:

- **Defense in Depth**: Implement multiple layers of security controls. If one layer fails, subsequent layers should prevent a total compromise. Never rely on a single control (e.g., UI validation alone).
- **Least Privilege**: Components, services, and users should only have the minimum permissions necessary to perform their required functions.
- **Secure by Default**: The default configuration of any system or component must be secure. Security features should not be opt-in.
- **Never Trust Client Input**: All data originating from outside the system boundary (including internal microservices) is considered untrusted and must be validated, sanitized, and type-checked before processing.
- **Fail Securely**: When a system or process fails, it must fail into a secure state. Errors should not leak sensitive information or bypass security checks.

## 3. Authentication (Current & Future)
Authentication is the process of verifying identity.

### Current State
- The system currently utilizes the `X-Tenant-ID` header for isolating tenant environments.
- This is *not* a true authentication mechanism but serves as a boundary marker for tenant isolation during early development.

### Future State
- **JWT-Based Authentication**: For user-facing applications, we implement JSON Web Tokens (JWT).
  - Tokens must be signed using **EdDSA with Ed25519** keys. This is the platform standard per [TDR-0001](../rfcs/decisions/0001-authentication-cryptography.md).
  - JWT verification must explicitly validate that the `alg` header is `EdDSA`. Tokens using any other algorithm (including `none`, `HS256`, `RS256`) must be rejected to prevent algorithm confusion attacks.
  - JWTs must include a `kid` (Key ID) header to support key rotation without downtime.
  - Tokens must have a short expiration time (e.g., 15 minutes). Short-lived tokens limit the blast radius of a compromised token and reduce the need for server-side revocation infrastructure.
  - Refresh tokens will be used for session extension and must be stored securely. Refresh tokens are opaque (not JWTs) and must be stored as hashes in the database — never in plaintext.
- **API Key Authentication**: For programmatic access (machine-to-machine).
  - API keys will be hashed before storage (e.g., using SHA-256).
  - Keys will be prefixed to allow easy identification (e.g., `cdt_live_xxxx...`).

### Token Validation Rules
- Always validate the token signature using the public key identified by the `kid` header.
- Always verify the signing algorithm is `EdDSA`. Reject all other algorithms.
- Always check the `exp` (expiration) and `nbf` (not before) claims.
- Validate the `iss` (issuer) and `aud` (audience) claims.
- Validate required custom claims (`sub`, `sid`, `tid`) are present and contain valid UUIDs.
- Allow a small clock skew tolerance (e.g., 5 seconds) for expiration checks.
- Session management principles dictate that tokens should be easily revocable via a fast lookup (e.g., Redis blocklist) if the short token lifetime proves insufficient.

## 4. Authorization
Authorization determines what an authenticated identity is allowed to do.

### Current State
- **Tenant-Scoped Data Access**: All data queries must include a strict `tenant_id` filter. Cross-tenant data access is strictly prohibited.

### Future State
- **Role-Based Access Control (RBAC)**: We will implement RBAC where users are assigned roles within a tenant context (e.g., Admin, Editor, Viewer).

### Key Rules
- **Always verify tenant ownership at the query level**: Do not fetch a record by its ID and then check the tenant in code. Include the `tenant_id` in the SQL `WHERE` clause.
- **Defense in Depth**: Never rely solely on middleware for authorization. Handlers and service layers must explicitly authorize the requested action against the specific resource.

## 5. Password Handling
When implementing local user authentication, passwords must be handled with the utmost care.

- **Hashing Algorithm**: Use **Argon2id** for all new password credentials. This is the platform standard per [TDR-0001](../rfcs/decisions/0001-authentication-cryptography.md). Argon2id is the winner of the Password Hashing Competition and provides memory-hardness that resists GPU/ASIC-accelerated cracking.
  - Initial parameters: Memory 64 MiB, Iterations 3, Parallelism 4, Salt 16 bytes, Key length 32 bytes.
  - Parameters must be centralized in a single configuration struct and must not be scattered throughout application code.
  - Output must use the PHC string format (`$argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>`), which self-describes the algorithm and parameters for future upgradability.
- **Never Store Plaintext**: Plaintext passwords must never be written to disk, databases, or memory caches.
- **Never Log Passwords**: Ensure logging configurations explicitly strip or redact password fields from request bodies. Passwords must never appear in structured log fields, error messages, or API responses.
- **Constant-Time Comparison**: Use the Argon2id library's built-in verification (which performs constant-time comparison) or an explicit constant-time comparison function to prevent timing attacks.
- **Anti-Enumeration**: When a login attempt targets a non-existent email, perform a dummy hash verification to prevent timing-based account enumeration.
- **Password Length**: Enforce a minimum of 12 characters and a maximum of 128 characters. Do not enforce arbitrary composition rules (uppercase, symbols, etc.) per NIST SP 800-63B guidance.

## 6. Secret Management
Secrets include API keys, database credentials, TLS certificates, and encryption keys.

- **Environment Variables**: All secrets must be injected into the application via environment variables.
- **Never Hardcode Secrets**: Source code must not contain any secrets.
- **Never Commit Secrets**: Ensure `.gitignore` prevents committing `.env` files.
- **.env.example**: Provide a `.env.example` file in the repository with placeholder values to guide developers on required configuration.
- **Rotation**: Design systems to support regular rotation of secrets without downtime.

## 7. Input Validation
Validating input is the first line of defense against injection and manipulation attacks.

- **Handler Layer Validation**: Perform all input validation immediately at the handler layer.
- **Types, Lengths, Formats**: Validate that input matches expected data types, string lengths, and specific formats.
- **Sanitization**: Sanitize input before storing or rendering it, particularly if it will be displayed in a UI (to prevent XSS).
- **Email Normalization**: Always normalize emails by converting them to lowercase and trimming whitespace before validation and storage.
- **UUID Validation**: Ensure any path parameters or query parameters representing UUIDs are validated as legitimate UUID formats before executing database queries.
- **JSON Binding**: Use Gin's `ShouldBindJSON` coupled with `go-playground/validator` tags on struct fields to automatically enforce validation rules.

## 8. SQL Injection Prevention
SQL injection is entirely preventable through proper query construction.

- **Parameterized Queries**: Always use parameterized queries provided by `pgx` and `sqlc`. Positional parameters (e.g., `$1`, `$2`) ensure the database treats input as data, not executable code.
- **sqlc Usage**: Rely on `sqlc` to generate safe database access code from raw SQL schemas and queries.
- **NEVER Concatenate SQL Strings**: Do not build SQL queries by concatenating strings.
- **NEVER use `fmt.Sprintf` for Queries**: Constructing SQL with `fmt.Sprintf` is strictly prohibited.

## 9. PII Handling
Personally Identifiable Information (PII) must be carefully managed to comply with privacy regulations (GDPR, CCPA).

- **What is PII**: Email addresses, display names, metadata payloads, IP addresses, and any data that can identify an individual.
- **Logging Rules**: Never log full email addresses. Use the email domain (the part after the `@`) for debugging purposes. Never log metadata payloads or full request/response bodies containing PII.
- **Storage**: PII must be encrypted at rest (handled by the database layer/cloud provider).
- **Access**: PII access is strictly tenant-scoped. Any risk of cross-tenant data leaks must be treated as a critical severity incident.
- **Retention & Deletion**: Support soft deletes (deactivation) for immediate action, followed by a scheduled hard purge policy to comply with data subject deletion requests.

## 10. OWASP Top 10 Coverage
Our architecture must defend against the OWASP Top 10 web application security risks:

- **A01:2021-Broken Access Control**: Enforced via mandatory `tenant_id` query scoping and future RBAC.
- **A02:2021-Cryptographic Failures**: Enforced by requiring TLS for all traffic, using strong password hashing (Argon2id), JWT signing with EdDSA/Ed25519, and encrypting data at rest.
- **A03:2021-Injection**: Prevented entirely via `sqlc` and `pgx` parameterized queries.
- **A04:2021-Insecure Design**: Mitigated through defense in depth, threat modeling, and secure by default principles.
- **A05:2021-Security Misconfiguration**: Managed via IaC, disabling debug features in prod, and strict CORS policies.
- **A06:2021-Vulnerable and Outdated Components**: Addressed by dependency pinning, `go mod verify`, and automated dependency updates.
- **A07:2021-Identification and Authentication Failures**: Prevented by standardized token validation, secure password hashing, and rate limiting.
- **A08:2021-Software and Data Integrity Failures**: CI/CD pipeline enforces signed commits and verifies go modules.
- **A09:2021-Security Logging and Monitoring Failures**: Mandated structured logging, correlation IDs, and robust alerting.
- **A10:2021-Server-Side Request Forgery (SSRF)**: Ensure any outbound webhooks or API calls use explicit allow-lists or rigorous URL validation.

## 11. Rate Limiting
Rate limiting protects the platform from abuse, DoS attacks, and brute-force attempts.

- **Global Rate Limiting**: Implemented via middleware to protect overall service health.
- **Per-Tenant Rate Limiting (Future)**: Limit the number of API calls a specific tenant can make based on their tier.
- **Per-Endpoint Rate Limiting (Future)**: Apply stricter limits on sensitive endpoints (e.g., login, password reset).
- **Response**: Always return a `429 Too Many Requests` status code with a standard `Retry-After` header indicating when the client may retry.

## 12. Enumeration Attacks
Prevent attackers from gathering information about the system state or users.

- **Information Leakage**: Do not reveal whether an email exists during account creation or password reset. Use generic messages like "If an account exists, an email has been sent."
- **Consistent Errors**: Ensure authentication failure messages are identical whether the username is invalid or the password is incorrect (e.g., "Invalid credentials").
- **Rate Limiting**: Apply aggressive rate limiting to login and token generation endpoints.
- **Monitoring**: Log and alert on suspicious patterns of repeated failures from specific IPs.

## 13. Dependency Security
Third-party dependencies introduce external risk to our codebase.

- **Verification**: Always run `go mod verify` in CI to ensure the integrity of downloaded modules.
- **Automated Updates**: Use Dependabot (or similar tools) to monitor for vulnerabilities in dependencies.
- **Pin Versions**: Pin dependency versions in `go.mod`.
- **Review Upgrades**: Major version upgrades must be manually reviewed for security implications.
- **No Wildcard Imports**: Avoid wildcard imports to ensure explicit control over the namespace.

## 14. Audit Logging (Future)
Audit logs provide a forensic trail of actions taken within the system.

- **Scope**: Log all state-changing operations (CREATE, UPDATE, DELETE).
- **Content**: Include WHO (user ID/tenant ID), WHAT (resource type and ID), WHEN (timestamp), and FROM WHERE (IP/User-Agent).
- **Immutability**: Audit logs must be append-only and immutable.
- **Separation**: Store audit logs separately from standard application debug/info logs.

## 15. API Security
Secure the outer edge of our application APIs.

- **HTTPS Only**: All production traffic must occur over TLS (HTTPS). HTTP traffic should be rejected or redirected.
- **Security Headers**: Ensure middleware injects headers like `Strict-Transport-Security`, `X-Content-Type-Options: nosniff`, and `X-Frame-Options: DENY`.
- **CORS**: Configure Cross-Origin Resource Sharing explicitly. Never use wildcard `*` for origins in production.
- **Content-Type Validation**: Enforce `Content-Type: application/json` for API requests to prevent unexpected parsing behaviors.
- **Request Size Limits**: Enforce maximum request body sizes to prevent memory exhaustion attacks.

## 16. Anti-Patterns
Avoid these dangerous practices:

- ❌ Returning internal database errors or stack traces to the client.
- ❌ Logging full request/response bodies blindly (risks PII exposure).
- ❌ Trusting the `X-Forwarded-For` header without verifying the proxy topology.
- ❌ Checking authorization in the UI but forgetting to check it in the backend API handler.
- ❌ Using custom cryptography instead of standard, audited libraries.

## 17. Checklist
Before merging any PR, ensure:
- [ ] No secrets are hardcoded or checked in.
- [ ] All database queries use parameterized inputs (sqlc/pgx).
- [ ] Handlers validate all inputs explicitly.
- [ ] Service/Repo layers enforce `tenant_id` scoping.
- [ ] No PII (emails, names, passwords, API keys) is being logged.
- [ ] Dependencies have been checked for known vulnerabilities.
