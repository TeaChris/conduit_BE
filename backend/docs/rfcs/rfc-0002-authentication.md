# RFC-0002 — Authentication Architecture & Design

| Field       | Value                                    |
|-------------|------------------------------------------|
| **Author**  | Platform Engineering                     |
| **Status**  | Draft — Pending TDR Review               |
| **Created** | 2026-08-18                               |
| **Domain**  | `internal/auth/`                         |
| **Depends** | RFC-0001 (User Domain)                   |
| **Reviewers** | Principal Engineers, TDR Committee     |

---

## 1. Status

**Draft** — Awaiting TDR committee review and approval before implementation begins.

---

## 2. Context

The Conduit Notification Platform has a functioning User domain (RFC-0001) that manages user identity and lifecycle. However, the platform currently has no authentication. The only access control is the `X-Tenant-ID` header, which provides tenant isolation but does not verify caller identity.

Before the platform can serve production workloads, it must answer: **"Who are you?"**

This RFC defines the Authentication module — the system responsible for verifying user identity through password-based credentials, issuing and managing access/refresh tokens, and maintaining authentication sessions. It intentionally excludes authorization ("What are you allowed to do?"), which will be addressed in a future RFC.

### Relationship to RFC-0001

RFC-0001 established the User domain, which owns:

- User entity lifecycle (CRUD, status transitions)
- Email normalization and per-tenant uniqueness
- Email verification state (`email_verified`, `email_verified_at`)
- The `SetEmailVerified()` service method (FR-7), explicitly designed for the Auth domain to call

The Authentication domain references the User domain but does not own user profile data. The User domain must not depend on Authentication implementation details.

### Engineering Standards Compliance

This RFC was designed in compliance with:

- `engineering-principles.md` — Simplicity over cleverness, boring technology
- `architecture-principles.md` — Flat domain packages, Clean Architecture, explicit DI
- `coding-standards.md` — Three-tier error model, context-first, no panics
- `api-guidelines.md` — REST, POST for actions, `{"data": ...}` / `{"error": ...}` envelopes
- `database-standards.md` — UUID PKs, TIMESTAMPTZ, tenant FK, status-based lifecycle
- `testing-standards.md` — Functional struct mocks, table-driven tests, no testify
- `security-standards.md` — PII handling, input validation, parameterized queries
- `observability-standards.md` — `conduit_` metrics, OTel spans, request-scoped logging

### Standards Alignment

This RFC's cryptography choices are consistent with the platform engineering standards:

| Standard | Specification | RFC-0002 | TDR |
|---|---|---|---|
| `security-standards.md` §5 — Password Hashing | Argon2id | Argon2id | [TDR-0001](decisions/0001-authentication-cryptography.md) |
| `security-standards.md` §3 — JWT Signing | EdDSA / Ed25519 | EdDSA / Ed25519 | [TDR-0001](decisions/0001-authentication-cryptography.md) |

> **Resolved:** `security-standards.md` was updated to specify Argon2id and EdDSA/Ed25519 per TDR-0001. No conflicts remain between this RFC and the engineering standards.

---

## 3. Goals

| # | Goal | Measure |
|---|---|---|
| G1 | Secure password-based authentication | Argon2id hashing, constant-time comparison, no plaintext storage |
| G2 | Stateless access tokens with short lifetime | JWT (EdDSA), ~15 min expiry, validated without database lookup |
| G3 | Secure session management with refresh tokens | Opaque tokens, hashed storage, 30-day expiry, rotation on use |
| G4 | Detect and respond to refresh token theft | Token family tracking, reuse detection, automatic session revocation |
| G5 | Support password self-service | Change password (authenticated), forgot/reset password (unauthenticated) |
| G6 | Email verification before access | Verification tokens, single-use, hashed storage |
| G7 | Prevent account enumeration | Consistent error messages, timing-safe operations |
| G8 | Rate-limit authentication endpoints | Redis-backed, per-IP and per-email where applicable |
| G9 | Full observability | Metrics, traces, structured logs on every auth operation |
| G10 | Clean domain boundary | Auth owns credentials/sessions/tokens; User owns identity/profile |

---

## 4. Non-Goals

| # | Non-Goal | Rationale |
|---|---|---|
| NG1 | Authorization (RBAC, roles, permissions) | Separate domain, separate RFC |
| NG2 | OAuth2 / OpenID Connect provider | Not needed until external integrations require it |
| NG3 | Social login (Google, GitHub, etc.) | Adds OAuth complexity; deferred until business requirement exists |
| NG4 | API key authentication | Separate mechanism for machine-to-machine; separate RFC |
| NG5 | Multi-factor authentication (MFA/2FA) | High value but significant scope; deferred to follow-up RFC |
| NG6 | Account lockout mechanism | Risk of denial-of-service; use rate limiting instead |
| NG7 | Email sending infrastructure | Auth generates tokens; a future Notification domain sends emails |
| NG8 | Organization-scoped permissions | Belongs to Organization/RBAC domains |
| NG9 | JWT blocklist / immediate revocation | Short-lived tokens (15 min) make this unnecessary for v1 |

---

## 5. Requirements

### Functional Requirements

| # | Requirement | Category |
|---|---|---|
| FR-1 | Register a new user with email + password | Registration |
| FR-2 | Authenticate with email + password, receive access + refresh tokens | Login |
| FR-3 | Exchange a valid refresh token for new access + refresh tokens | Token refresh |
| FR-4 | Revoke the current session | Logout |
| FR-5 | Revoke all sessions for the authenticated user | Logout all |
| FR-6 | Change password while authenticated | Password management |
| FR-7 | Request a password reset token | Password recovery |
| FR-8 | Reset password using a valid reset token | Password recovery |
| FR-9 | Verify email using a valid verification token | Email verification |
| FR-10 | Request a new email verification token | Email verification |
| FR-11 | Middleware to validate JWT on protected routes | Request authentication |
| FR-12 | Rotate refresh tokens on every successful refresh | Token security |
| FR-13 | Detect and revoke session on refresh token reuse | Token security |

### Non-Functional Requirements

| # | Requirement | Target |
|---|---|---|
| NFR-1 | Login latency P99 | < 500ms (Argon2id is intentionally slow) |
| NFR-2 | Token refresh latency P99 | < 50ms |
| NFR-3 | JWT validation latency P99 | < 1ms (no database lookup) |
| NFR-4 | Rate limit: login per IP | 10 requests / minute (configurable) |
| NFR-5 | Rate limit: login per email | 5 requests / minute (configurable) |
| NFR-6 | Rate limit: forgot-password per IP | 5 requests / minute (configurable) |
| NFR-7 | Rate limit: register per IP | 5 requests / minute (configurable) |
| NFR-8 | Password hashing time | 200-500ms (Argon2id with recommended params) |
| NFR-9 | No authentication secret in logs | Passwords, tokens, hashes, Authorization headers |
| NFR-10 | Test coverage | >= 80% service layer, 100% crypto helpers |

---

## 6. Architectural Principles

1. **Authentication ≠ Authorization.** This module answers "Who are you?" — never "What can you do?"
2. **Auth references User, not the reverse.** The `users` table has no knowledge of credentials, sessions, or tokens. Auth tables hold `user_id` foreign keys.
3. **Credentials are not profile data.** Password hashes, sessions, and tokens are owned exclusively by the Auth domain.
4. **Defense in depth.** Every security-critical operation has multiple layers: input validation, rate limiting, constant-time comparison, hashed storage, short-lived tokens.
5. **Fail secure.** Invalid tokens, expired tokens, and unknown users all produce the same generic error. No information leakage.
6. **Database as source of truth for consistency.** Refresh token rotation and reuse detection use PostgreSQL row-level locking — not application-level mutexes.
7. **Boring technology.** PostgreSQL for sessions and tokens, Redis for rate limiting, standard crypto libraries. No Kafka, no service mesh, no distributed locks.

---

## 7. Authentication Architecture

### Module Structure

```
internal/auth/
├── model.go          ← Domain: Credential, Session, token value objects
├── errors.go         ← Domain: Auth-specific error definitions
├── repository.go     ← Infrastructure: Repository interface + PostgreSQL implementation
├── service.go        ← Application: Auth business logic, orchestration
├── handler.go        ← Transport: HTTP handlers, JWT extraction, validation
├── dto.go            ← Transport: Request/Response structs, mappers
├── metrics.go        ← Observability: Prometheus counters and histograms
├── password.go       ← Infrastructure: Argon2id hashing, verification
├── jwt.go            ← Infrastructure: JWT signing, validation, key management
├── token.go          ← Infrastructure: Opaque token generation, hashing
├── ratelimit.go      ← Infrastructure: Redis-backed rate limiting
└── middleware.go     ← Transport: JWT authentication middleware
```

### Dependency Graph

```
┌──────────────────────────────────────────────────────┐
│                    Transport Layer                     │
│  handler.go   dto.go   middleware.go                 │
│  (Gin, HTTP, JSON parsing, JWT extraction)           │
└─────────────────────┬────────────────────────────────┘
                      │ calls
                      ▼
┌──────────────────────────────────────────────────────┐
│                   Application Layer                   │
│  service.go                                          │
│  (Business logic, transaction orchestration)          │
│                                                      │
│  Depends on:                                         │
│    - Auth Repository (interface)                     │
│    - UserProvider (interface — satisfied by User svc) │
│    - PasswordHasher (interface)                      │
│    - TokenGenerator (interface)                      │
│    - JWTIssuer (interface)                           │
│    - RateLimiter (interface)                         │
└─────────────────────┬────────────────────────────────┘
                      │ calls
                      ▼
┌──────────────────────────────────────────────────────┐
│                 Infrastructure Layer                  │
│  repository.go   password.go   jwt.go   token.go    │
│  ratelimit.go                                        │
│  (PostgreSQL/sqlc, Argon2id, Ed25519, crypto/rand,   │
│   Redis)                                             │
└──────────────────────────────────────────────────────┘
```

### Cross-Domain Interface

The Auth domain defines the interface it needs from the User domain. The User domain does not import Auth.

```go
// Defined in internal/auth/service.go
type UserProvider interface {
    CreateUser(ctx context.Context, input CreateUserInput) (*UserInfo, error)
    GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*UserInfo, error)
    GetByID(ctx context.Context, tenantID uuid.UUID, userID uuid.UUID) (*UserInfo, error)
    SetEmailVerified(ctx context.Context, userID uuid.UUID) error
}

// UserInfo contains only the user fields Auth needs.
// This is NOT user.User — it is an Auth-domain type.
type UserInfo struct {
    ID            uuid.UUID
    TenantID      uuid.UUID
    Email         string
    Status        string
    EmailVerified bool
}
```

Wiring in `app.go`:

```go
// Auth domain defines UserProvider interface.
// user.Service satisfies it (duck typing).
// app.go wires them together.
authService := auth.NewService(authRepo, userService, hasher, jwtIssuer, ...)
```

---

## 8. User Domain Boundary

### What Auth Owns

| Concept | Table | Responsibility |
|---|---|---|
| Password credentials | `user_credentials` | Hashed password storage and verification |
| Authentication sessions | `auth_sessions` | Session lifecycle, refresh token state, token families |
| Password reset tokens | `password_reset_tokens` | Hashed token storage, single-use enforcement |
| Email verification tokens | `email_verification_tokens` | Hashed token storage, single-use enforcement |

### What Auth Reads from User (via UserProvider)

| Operation | User Method | Purpose |
|---|---|---|
| Registration | `CreateUser()` | Create the user identity |
| Login | `GetByEmail()` | Look up user by email for credential verification |
| Token refresh | `GetByID()` | Verify user is still active |
| Email verification | `SetEmailVerified()` | Mark email as verified after token validation |

### What Auth Does NOT Touch

- `users.display_name` — profile data
- `users.metadata` — tenant-extensible data
- `users.status` transitions (deactivate/reactivate) — User domain operations
- Any future user profile fields

### Invariant

The `users` table has no columns related to authentication. No `password_hash`, no `last_login_at`, no `session_count`. All authentication state lives in Auth-owned tables that reference `users.id` via foreign key.

> **Design decision:** RFC-0001 section 13 mentioned `password_hash` as a future column on the `users` table. This RFC overrides that suggestion. A separate `user_credentials` table provides cleaner domain separation, supports multiple credential types in the future (e.g., API keys, OAuth tokens), and allows the Auth module to be extracted independently.

---

## 9. Credential Architecture

### Password Storage

Passwords are hashed using **Argon2id** and stored in the `user_credentials` table.

```go
// Stored format (PHC string format):
// $argon2id$v=19$m=65536,t=3,p=4$<base64-salt>$<base64-hash>
```

### Argon2id Parameters

| Parameter | Value | Rationale |
|---|---|---|
| Memory | 64 MiB (65536 KiB) | OWASP minimum recommendation for Argon2id |
| Iterations | 3 | Balances security with ~300ms hash time |
| Parallelism | 4 | Matches typical server core count |
| Salt length | 16 bytes | Sufficient entropy for unique salts |
| Key length | 32 bytes | 256-bit hash output |

These parameters are centralized in a configuration struct, not scattered through the codebase:

```go
// internal/auth/password.go
type Argon2Params struct {
    Memory      uint32 // KiB
    Iterations  uint32
    Parallelism uint8
    SaltLength  uint32
    KeyLength   uint32
}

var DefaultArgon2Params = Argon2Params{
    Memory:      64 * 1024, // 64 MiB
    Iterations:  3,
    Parallelism: 4,
    SaltLength:  16,
    KeyLength:   32,
}
```

### Password Hasher Interface

```go
type PasswordHasher interface {
    Hash(password string) (string, error)
    Verify(password, hash string) (bool, error)
}
```

The service layer depends on this interface. The implementation uses `golang.org/x/crypto/argon2`.

### Anti-Enumeration: Dummy Hash

When a login attempt targets a non-existent email, the service must still perform a hash verification against a pre-computed dummy hash. This prevents timing-based account enumeration.

```go
func (s *Service) Login(ctx context.Context, input LoginInput) (*AuthResult, error) {
    user, err := s.users.GetByEmail(ctx, tenantID, input.Email)
    if err != nil {
        // User not found — still run password verification to prevent timing leak.
        _ = s.hasher.Verify(input.Password, s.dummyHash)
        return nil, ErrInvalidCredentials
    }
    // ... verify actual password
}
```

---

## 10. Password Security

### Password Policy

| Rule | Value | Rationale |
|---|---|---|
| Minimum length | 12 characters | NIST SP 800-63B recommendation |
| Maximum length | 128 characters | Prevents Argon2id DoS via extremely long inputs |
| Composition rules | **None** | NIST explicitly discourages arbitrary composition rules (uppercase, symbols, etc.) — they reduce usability without meaningfully improving security |

### Password Validation

```go
func validatePassword(password string) []errors.FieldError {
    var errs []errors.FieldError
    if len(password) < 12 {
        errs = append(errs, errors.FieldError{
            Field: "password", Message: "must be at least 12 characters",
        })
    }
    if len(password) > 128 {
        errs = append(errs, errors.FieldError{
            Field: "password", Message: "must be at most 128 characters",
        })
    }
    return errs
}
```

### What Must Never Happen

1. Passwords stored in plaintext
2. Passwords logged (even at debug level)
3. Passwords returned in API responses
4. Passwords encrypted instead of hashed
5. Passwords compared with `==` instead of constant-time verification
6. Password hashes exposed in error messages

---

## 11. Session Architecture

### Model

Each successful login creates an authentication session. Sessions support:

- Multiple concurrent sessions per user (different devices/browsers)
- Individual session revocation (logout)
- Bulk session revocation (logout all, password reset)
- Token family tracking for reuse detection

```go
type Session struct {
    ID               uuid.UUID
    TenantID         uuid.UUID
    UserID           uuid.UUID
    FamilyID         uuid.UUID   // Token family — set at login, constant for session life
    RefreshTokenHash string      // SHA-256 hash of current valid refresh token
    ExpiresAt        time.Time   // Absolute session expiry (30 days from login)
    RevokedAt        *time.Time  // Non-nil = session is revoked
    CreatedAt        time.Time
    UpdatedAt        time.Time
    LastUsedAt       time.Time   // Updated on each successful refresh
    IPAddress        *string     // Optional — client IP at session creation
    UserAgent        *string     // Optional — User-Agent at session creation
}
```

### Session Lifecycle

```
Login
  │
  ├─ Create session (family_id = new UUID)
  ├─ Store hashed refresh token
  ├─ Issue JWT + refresh token
  │
  ▼
Refresh (repeatable)
  │
  ├─ Validate refresh token against stored hash
  ├─ Rotate: generate new token, update hash
  ├─ Issue new JWT + new refresh token
  │
  ▼
Logout / Expiry / Reuse Detection
  │
  ├─ Set revoked_at = now()
  └─ Session is dead
```

### Data Retention

- Active sessions: retained until expiry or revocation
- Revoked/expired sessions: retained for 90 days for audit, then purged by a background job (future)
- IP address and User-Agent: collected at session creation only; not updated on refresh

> **Design decision:** We collect IP address and User-Agent at session creation to support "active sessions" UIs (showing users where they're logged in). We deliberately do NOT track per-request metadata — that belongs in access logs, not session records.

---

## 12. Access Token Architecture

### JWT Structure

Access tokens are JWTs signed with **Ed25519 (EdDSA)**.

**Header:**

```json
{
  "alg": "EdDSA",
  "typ": "JWT",
  "kid": "key-2026-08"
}
```

**Payload (claims):**

```json
{
  "iss": "conduit",
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "aud": ["conduit-api"],
  "jti": "unique-token-id",
  "sid": "session-uuid",
  "tid": "tenant-uuid",
  "iat": 1724000000,
  "exp": 1724000900
}
```

| Claim | Type | Description |
|---|---|---|
| `iss` | string | Issuer — always `"conduit"` |
| `sub` | string | Subject — the `user_id` (UUID) |
| `aud` | []string | Audience — `["conduit-api"]` |
| `jti` | string | JWT ID — unique per token (UUID) |
| `sid` | string | Session ID — links to `auth_sessions.id` |
| `tid` | string | Tenant ID — the user's tenant |
| `iat` | number | Issued at — Unix timestamp |
| `exp` | number | Expiration — Unix timestamp (iat + 900s = 15 minutes) |

### What Is NOT in the JWT

- Passwords or password hashes
- Refresh tokens
- User display names, emails, or profile data
- Organization memberships or roles
- Notification preferences
- Any large or mutable data

### Access Token Lifetime

**15 minutes.** This is short enough that:

- Compromised tokens have limited blast radius
- No JWT blocklist is needed for v1
- Token refresh is required frequently enough to detect revoked sessions

### Key Management

| Aspect | Design |
|---|---|
| Algorithm | Ed25519 (EdDSA) — deterministic, no nonce |
| Key pair | Generated offline, loaded from environment variable at startup |
| `kid` header | Identifies the signing key; supports key rotation |
| Key rotation | Deploy new key with new `kid`; old key remains valid for verification until all old tokens expire (15 min overlap) |
| Storage | Private key: environment variable (`AUTH_JWT_PRIVATE_KEY`). Never in source control. |
| Key format | PEM-encoded Ed25519 private key |

```go
// internal/auth/jwt.go
type JWTIssuer interface {
    Issue(claims Claims) (string, error)
    Validate(tokenString string) (*Claims, error)
}

type Claims struct {
    UserID    uuid.UUID
    TenantID  uuid.UUID
    SessionID uuid.UUID
    TokenID   uuid.UUID
}
```

### JWT Validation Rules

The `Validate` method **must** enforce all of the following:

1. **Signature** — Verify using the public key identified by `kid`
2. **Algorithm** — Must be `EdDSA`. Reject any other algorithm (prevents algorithm confusion attacks)
3. **Issuer** — Must be `"conduit"`
4. **Audience** — Must include `"conduit-api"`
5. **Expiration** — Must not be expired (with up to 5s clock skew tolerance)
6. **Required claims** — `sub`, `sid`, `tid`, `jti` must all be present and valid UUIDs

If any check fails, return `ErrInvalidToken`. Do not provide specific failure reasons to callers (prevents information leakage).

---

## 13. Refresh Token Architecture

### Design

Refresh tokens are **opaque, cryptographically random** strings. They are never JWTs.

```go
// Token format: 32 bytes of crypto/rand, base64url-encoded
// Example: "dGhpcyBpcyBhIHJlZnJlc2ggdG9rZW4gZXhhbXBsZQ"
// Length: 43 characters (256 bits of entropy)
```

### Storage

Refresh tokens are **never stored in plaintext**. Only a SHA-256 hash is stored in `auth_sessions.refresh_token_hash`.

```go
// internal/auth/token.go
type TokenGenerator interface {
    Generate() (token string, hash string, err error)
    Hash(token string) string
}
```

The generator uses `crypto/rand` for token generation and `crypto/sha256` for hashing.

### Lifetime

**30 days** from session creation. The session has an absolute expiry (`expires_at`). Individual refresh tokens within the session do not have independent expiry — they inherit the session's expiry.

### Client Handling

Refresh tokens should be:

- Stored securely by the client (HttpOnly cookie or secure storage)
- Transmitted only over HTTPS
- Never stored in localStorage (XSS vulnerable)
- Sent only to the refresh endpoint

> **Note:** Cookie-based refresh token delivery is a future enhancement. For v1, tokens are returned in the JSON response body and clients are responsible for secure storage.

---

## 14. Token Rotation & Reuse Detection

### Rotation Model

Every successful token refresh issues a new refresh token and invalidates the previous one.

```
Login
  │
  └─▶ Refresh Token A (hash stored in session)
         │
         └─▶ Refresh (using A)
                │
                ├─ Invalidate A (update hash)
                └─▶ Refresh Token B (new hash stored)
                       │
                       └─▶ Refresh (using B)
                              │
                              ├─ Invalidate B
                              └─▶ Refresh Token C
```

### Token Families

Each session has a `family_id` (UUID) set at login time. The family ID groups all refresh tokens that descend from a single login event. This enables detecting when a token is reused after rotation.

### Reuse Detection

If a refresh request presents a token whose hash **does not match** the session's current `refresh_token_hash`, this indicates one of:

1. **Token theft:** An attacker captured a previous token and is replaying it
2. **Race condition:** Client made concurrent requests with the same token

In either case, the response is the same: **revoke the entire session** (set `revoked_at = now()`). The legitimate user must re-authenticate.

```
Attacker captures Token A
      │
      │  Legitimate user refreshes with Token A → gets Token B
      │
      └─▶ Attacker tries to refresh with Token A
              │
              ├─ Hash(A) ≠ session.refresh_token_hash (which is now Hash(B))
              ├─ REUSE DETECTED
              ├─ Revoke session (family)
              └─ Return error — both attacker and user must re-authenticate
```

### Concurrency Strategy

Two concurrent refresh requests using the same token:

```sql
-- Transaction 1: Acquires row lock
BEGIN;
SELECT * FROM auth_sessions
WHERE family_id = $1 AND revoked_at IS NULL
FOR UPDATE;
-- Verifies hash matches, updates hash, commits

-- Transaction 2: Blocks on FOR UPDATE, then proceeds
-- Sees updated hash, which doesn't match the old token
-- REUSE DETECTED → revokes session
```

**`SELECT ... FOR UPDATE`** serializes concurrent refresh attempts at the database level. No application-level mutex is used.

| Scenario | Outcome |
|---|---|
| First request arrives | Succeeds, rotates token |
| Second concurrent request with same token | Blocks on `FOR UPDATE`, then detects reuse, revokes session |
| Request with revoked session | Rejected immediately (`revoked_at IS NOT NULL`) |
| Request with expired session | Rejected (`expires_at < now()`) |

---

## 15. Registration Flow

### Sequence

```
Client                          Auth Handler       Auth Service       User Service       DB
  │                                 │                   │                   │              │
  │ POST /api/v1/auth/register      │                   │                   │              │
  │ {email, display_name, password} │                   │                   │              │
  │────────────────────────────────▶│                   │                   │              │
  │                                 │ validate input    │                   │              │
  │                                 │──────────────────▶│                   │              │
  │                                 │                   │ BEGIN TX          │              │
  │                                 │                   │─────────────────────────────────▶│
  │                                 │                   │                   │              │
  │                                 │                   │ CreateUser()      │              │
  │                                 │                   │──────────────────▶│              │
  │                                 │                   │                   │─────────────▶│
  │                                 │                   │                   │ INSERT users │
  │                                 │                   │                   │◀─────────────│
  │                                 │                   │◀──────────────────│              │
  │                                 │                   │                   │              │
  │                                 │                   │ Hash password     │              │
  │                                 │                   │ (Argon2id)        │              │
  │                                 │                   │                   │              │
  │                                 │                   │ INSERT user_credentials          │
  │                                 │                   │─────────────────────────────────▶│
  │                                 │                   │                   │              │
  │                                 │                   │ Generate verification token      │
  │                                 │                   │ INSERT email_verification_tokens │
  │                                 │                   │─────────────────────────────────▶│
  │                                 │                   │                   │              │
  │                                 │                   │ COMMIT TX         │              │
  │                                 │                   │─────────────────────────────────▶│
  │                                 │                   │                   │              │
  │                                 │◀──────────────────│                   │              │
  │◀────────────────────────────────│                   │                   │              │
  │  201 Created                    │                   │                   │              │
  │  {user data}                    │                   │                   │              │
  │  (NO tokens — must verify       │                   │                   │              │
  │   email first)                  │                   │                   │              │
```

### Transaction Boundary

Registration is a **single atomic transaction** spanning:

1. User creation (via `UserProvider.CreateUser`)
2. Credential creation (password hash)
3. Email verification token creation

All three succeed or all three roll back.

### Post-Registration

- The user receives a 201 response with their user data (no tokens)
- The user cannot log in until their email is verified
- The verification token is available for the future Notification domain to email to the user
- For development/testing, the verification token can be retrieved through the verify-email endpoint directly

---

## 16. Login Flow

### Sequence

```
Client                          Auth Handler       Auth Service       DB
  │                                 │                   │              │
  │ POST /api/v1/auth/login         │                   │              │
  │ {email, password}               │                   │              │
  │────────────────────────────────▶│                   │              │
  │                                 │ validate input    │              │
  │                                 │──────────────────▶│              │
  │                                 │                   │              │
  │                                 │                   │ GetByEmail() │
  │                                 │                   │─────────────▶│
  │                                 │                   │              │
  │                                 │                   │ If not found:│
  │                                 │                   │  verify dummy│
  │                                 │                   │  hash        │
  │                                 │                   │  return err  │
  │                                 │                   │              │
  │                                 │                   │ Check user   │
  │                                 │                   │ status=active│
  │                                 │                   │              │
  │                                 │                   │ Check email  │
  │                                 │                   │ verified     │
  │                                 │                   │              │
  │                                 │                   │ Get cred     │
  │                                 │                   │─────────────▶│
  │                                 │                   │              │
  │                                 │                   │ Verify pwd   │
  │                                 │                   │ (Argon2id)   │
  │                                 │                   │              │
  │                                 │                   │ Create       │
  │                                 │                   │ session      │
  │                                 │                   │─────────────▶│
  │                                 │                   │              │
  │                                 │                   │ Issue JWT    │
  │                                 │                   │              │
  │                                 │◀──────────────────│              │
  │◀────────────────────────────────│                   │              │
  │  200 OK                         │                   │              │
  │  {access_token, refresh_token,  │                   │              │
  │   token_type, expires_in}       │                   │              │
```

### Login Validation

Before attempting authentication, the service validates:

1. **User exists** — If not, verify against dummy hash and return `ErrInvalidCredentials`
2. **User is active** — If deactivated/suspended, return `ErrInvalidCredentials` (same error — do not reveal account state)
3. **Email is verified** — If not, return `ErrEmailNotVerified` (this is safe to distinguish because the user created the account themselves)
4. **Password matches** — If not, return `ErrInvalidCredentials`

### Anti-Enumeration

The login endpoint returns the same error (`"invalid email or password"`) for:

- Unknown email
- Incorrect password
- Deactivated/suspended account

The only exception is `ErrEmailNotVerified`, which is safe because:
- The user knows they registered (they created the account)
- Distinguishing this case improves UX significantly
- An attacker could detect registration via the register endpoint anyway

---

## 17. Logout Flow

### Single Session Logout

```
POST /api/v1/auth/logout
Authorization: Bearer <jwt>

→ Extract session_id from JWT claims
→ Set auth_sessions.revoked_at = now() WHERE id = session_id
→ 204 No Content
```

### Logout All Sessions

```
POST /api/v1/auth/logout-all
Authorization: Bearer <jwt>

→ Extract user_id from JWT claims
→ UPDATE auth_sessions SET revoked_at = now()
   WHERE user_id = $1 AND tenant_id = $2 AND revoked_at IS NULL
→ 204 No Content
```

Both operations are idempotent. Revoking an already-revoked session is a no-op.

> **Note:** Existing JWTs remain valid until they expire (max 15 minutes). For v1, this is acceptable. If immediate revocation becomes a requirement, a Redis-backed JWT blocklist can be introduced.

---

## 18. Password Change

### Flow

```
POST /api/v1/auth/change-password
Authorization: Bearer <jwt>
{
  "current_password": "...",
  "new_password": "..."
}

→ Verify current_password against stored hash
→ Validate new_password (policy check)
→ Hash new_password with Argon2id
→ BEGIN TX:
    UPDATE user_credentials SET password_hash = $new_hash
    Revoke ALL other sessions (keep current session active)
  COMMIT
→ 204 No Content
```

### Security Properties

- Requires the current password (prevents unauthorized changes from stolen JWTs)
- Revokes all other sessions (limits blast radius if password was compromised)
- Keeps the current session active (user doesn't need to re-login on the device they used to change the password)

---

## 19. Password Reset

### Forgot Password (Request Reset)

```
POST /api/v1/auth/forgot-password
{
  "email": "user@example.com"
}

→ Always returns 200 OK with generic message
→ If user exists:
    - Invalidate any existing unused reset tokens for this user
    - Generate new reset token
    - Store SHA-256(token) with 30-minute expiry
    - (Future: trigger email via Notification domain)
→ If user does not exist:
    - Return same 200 OK response (anti-enumeration)
```

**Response (always the same):**

```json
{
  "data": {
    "message": "If an account with that email exists, a password reset link has been sent."
  }
}
```

### Reset Password (Execute Reset)

```
POST /api/v1/auth/reset-password
{
  "token": "<reset-token>",
  "new_password": "<new-password>"
}

→ Hash the provided token
→ Look up password_reset_tokens WHERE token_hash = SHA-256(token)
→ Validate: not expired, not used
→ BEGIN TX:
    UPDATE user_credentials SET password_hash = $new_hash
    UPDATE password_reset_tokens SET used_at = now()
    Revoke ALL auth_sessions for this user
  COMMIT
→ 200 OK with generic success message
```

### Reset Token Properties

| Property | Value |
|---|---|
| Length | 32 bytes (256 bits), base64url-encoded |
| Storage | SHA-256 hash only — plaintext never stored |
| Expiry | 30 minutes from creation |
| Single-use | `used_at` column prevents replay |
| Superseded | New token invalidates previous unused tokens for the same user |

### Post-Reset Behavior

After a successful password reset:

1. Password credential is updated
2. Reset token is marked as used
3. **All** authentication sessions are revoked
4. User must log in again with the new password

---

## 20. Email Verification

### Verify Email

```
POST /api/v1/auth/verify-email
{
  "token": "<verification-token>"
}

→ Hash the provided token
→ Look up email_verification_tokens WHERE token_hash = SHA-256(token)
→ Validate: not expired, not used
→ BEGIN TX:
    Call UserProvider.SetEmailVerified(user_id)
    UPDATE email_verification_tokens SET used_at = now()
  COMMIT
→ 200 OK
```

### Resend Verification

```
POST /api/v1/auth/resend-verification
{
  "email": "user@example.com"
}

→ Look up user by email in tenant
→ If user exists AND email_verified = false:
    - Invalidate existing unused verification tokens
    - Generate new verification token
    - Store SHA-256(token) with 24-hour expiry
    - (Future: trigger email via Notification domain)
→ Always return 200 OK with generic message (anti-enumeration)
```

### Verification Token Properties

| Property | Value |
|---|---|
| Length | 32 bytes (256 bits), base64url-encoded |
| Storage | SHA-256 hash only |
| Expiry | 24 hours from creation |
| Single-use | `used_at` column prevents replay |
| Superseded | New token invalidates previous unused tokens for the same user |

---

## 21. Authentication Middleware

### Purpose

The authentication middleware validates JWTs on protected routes and sets the authenticated principal in the request context.

### Flow

```
HTTP Request
    │
    ▼
Authorization header present?
    │
    ├─ No  → 401 Unauthorized (UNAUTHORIZED)
    │
    ▼
Extract "Bearer <token>"
    │
    ├─ Malformed → 401 Unauthorized
    │
    ▼
Validate JWT (signature, algorithm, issuer, audience, expiry, claims)
    │
    ├─ Invalid → 401 Unauthorized
    │
    ▼
Extract claims (sub=UserID, sid=SessionID, tid=TenantID)
    │
    ▼
Validate tid matches X-Tenant-ID header
    │
    ├─ Mismatch → 401 Unauthorized
    │
    ▼
Set AuthenticatedPrincipal in context
    │
    ▼
Continue to handler
```

### Authenticated Principal

```go
// internal/auth/middleware.go
type AuthenticatedPrincipal struct {
    UserID    uuid.UUID
    SessionID uuid.UUID
    TenantID  uuid.UUID
}
```

The principal is stored in the request context via a typed key. Downstream handlers and services retrieve it with:

```go
func GetPrincipal(ctx context.Context) (*AuthenticatedPrincipal, bool)
```

### What the Middleware Does NOT Do

- Does not check user status (would require a DB call per request)
- Does not check session revocation (would require a DB call per request)
- Does not enforce any authorization rules
- Does not add user profile data to the context

> **Trade-off:** We accept that a revoked session's JWT remains valid for up to 15 minutes. This avoids a database lookup on every authenticated request. If immediate revocation becomes critical, a Redis-backed session check can be added.

### Route Registration

```go
// In server.go — future state
// Public auth routes (no JWT required):
auth.RegisterPublicRoutes(v1, authHandler)

// Protected routes (JWT required):
protected := v1.Group("")
protected.Use(authMiddleware.Authenticate())
user.RegisterRoutes(protected, userHandler)
auth.RegisterProtectedRoutes(protected, authHandler)
```

---

## 22. API Contract

All endpoints are under `/api/v1/auth/`. All require the `X-Tenant-ID` header (enforced by the existing `TenantID` middleware on the `/api/v1` group).

---

### POST /api/v1/auth/register — Register

**Authentication:** None

**Request:**

```json
{
  "email": "jane@example.com",
  "display_name": "Jane Doe",
  "password": "securepassword123"
}
```

**Validation:**

| Field | Rules |
|---|---|
| `email` | Required, contains `@`, max 320 chars |
| `display_name` | Required, non-empty after trim, max 256 chars |
| `password` | Required, 12-128 chars |

**Response (201 Created):**

```json
{
  "data": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "jane@example.com",
    "display_name": "Jane Doe",
    "email_verified": false,
    "message": "Registration successful. Please verify your email address."
  }
}
```

**Errors:**

| Scenario | Status | Code |
|---|---|---|
| Email already registered in tenant | 409 | `CONFLICT` |
| Invalid email format | 422 | `VALIDATION_ERROR` |
| Password too short/long | 422 | `VALIDATION_ERROR` |
| Missing required field | 422 | `VALIDATION_ERROR` |
| Malformed JSON | 400 | `BAD_REQUEST` |
| Missing X-Tenant-ID | 400 | `TENANT_REQUIRED` |

**Transaction:** Yes — user creation + credential creation + verification token.

---

### POST /api/v1/auth/login — Login

**Authentication:** None

**Request:**

```json
{
  "email": "jane@example.com",
  "password": "securepassword123"
}
```

**Validation:**

| Field | Rules |
|---|---|
| `email` | Required, non-empty |
| `password` | Required, non-empty |

**Response (200 OK):**

```json
{
  "data": {
    "access_token": "eyJhbGciOiJFZERTQSIs...",
    "refresh_token": "dGhpcyBpcyBhIHJlZnJl...",
    "token_type": "Bearer",
    "expires_in": 900
  }
}
```

**Errors:**

| Scenario | Status | Code | Message |
|---|---|---|---|
| Wrong email or password | 401 | `UNAUTHORIZED` | `"invalid email or password"` |
| Email not verified | 403 | `FORBIDDEN` | `"email address not verified"` |
| Malformed JSON | 400 | `BAD_REQUEST` | — |

**Security:**

- Returns identical error for unknown email and wrong password
- Runs dummy hash verification when user not found
- Rate limited per IP and per email

---

### POST /api/v1/auth/refresh — Refresh Token

**Authentication:** None (uses refresh token in body)

**Request:**

```json
{
  "refresh_token": "dGhpcyBpcyBhIHJlZnJl..."
}
```

**Response (200 OK):**

```json
{
  "data": {
    "access_token": "eyJhbGciOiJFZERTQSIs...",
    "refresh_token": "bmV3IHJlZnJlc2ggdG9r...",
    "token_type": "Bearer",
    "expires_in": 900
  }
}
```

**Errors:**

| Scenario | Status | Code |
|---|---|---|
| Invalid/expired/revoked token | 401 | `UNAUTHORIZED` |
| Reuse detected | 401 | `UNAUTHORIZED` (session revoked) |

**Transaction:** Yes — `SELECT ... FOR UPDATE` on session, update token hash.

**Concurrency:** Serialized via `FOR UPDATE` row lock. Second concurrent request detects reuse.

---

### POST /api/v1/auth/logout — Logout

**Authentication:** Required (Bearer JWT)

**Request:** No body.

**Response (204 No Content)**

**Error:** 401 if JWT is invalid/missing.

---

### POST /api/v1/auth/logout-all — Logout All Sessions

**Authentication:** Required (Bearer JWT)

**Request:** No body.

**Response (204 No Content)**

**Error:** 401 if JWT is invalid/missing.

---

### POST /api/v1/auth/change-password — Change Password

**Authentication:** Required (Bearer JWT)

**Request:**

```json
{
  "current_password": "oldpassword123",
  "new_password": "newpassword456"
}
```

**Validation:**

| Field | Rules |
|---|---|
| `current_password` | Required, non-empty |
| `new_password` | Required, 12-128 chars, must differ from current |

**Response (204 No Content)**

**Errors:**

| Scenario | Status | Code |
|---|---|---|
| Current password incorrect | 401 | `UNAUTHORIZED` |
| New password fails policy | 422 | `VALIDATION_ERROR` |
| Same as current password | 422 | `VALIDATION_ERROR` |

**Transaction:** Yes — update credential + revoke other sessions.

---

### POST /api/v1/auth/forgot-password — Request Password Reset

**Authentication:** None

**Request:**

```json
{
  "email": "jane@example.com"
}
```

**Response (200 OK — always, regardless of email existence):**

```json
{
  "data": {
    "message": "If an account with that email exists, a password reset link has been sent."
  }
}
```

**Security:** No information leakage. Always returns 200.

---

### POST /api/v1/auth/reset-password — Execute Password Reset

**Authentication:** None

**Request:**

```json
{
  "token": "base64url-encoded-token",
  "new_password": "newpassword456"
}
```

**Validation:**

| Field | Rules |
|---|---|
| `token` | Required, non-empty |
| `new_password` | Required, 12-128 chars |

**Response (200 OK):**

```json
{
  "data": {
    "message": "Password has been reset. Please log in with your new password."
  }
}
```

**Errors:**

| Scenario | Status | Code |
|---|---|---|
| Invalid/expired/used token | 400 | `BAD_REQUEST` |
| New password fails policy | 422 | `VALIDATION_ERROR` |

**Transaction:** Yes — update credential + invalidate token + revoke all sessions.

---

### POST /api/v1/auth/verify-email — Verify Email

**Authentication:** None

**Request:**

```json
{
  "token": "base64url-encoded-token"
}
```

**Response (200 OK):**

```json
{
  "data": {
    "message": "Email address verified successfully."
  }
}
```

**Errors:**

| Scenario | Status | Code |
|---|---|---|
| Invalid/expired/used token | 400 | `BAD_REQUEST` |

**Transaction:** Yes — mark token used + set email verified on user.

---

### POST /api/v1/auth/resend-verification — Resend Verification Email

**Authentication:** None

**Request:**

```json
{
  "email": "jane@example.com"
}
```

**Response (200 OK — always):**

```json
{
  "data": {
    "message": "If an account with that email exists and requires verification, a new verification email has been sent."
  }
}
```

**Security:** No information leakage. Always returns 200.

---

## 23. Rate Limiting

### Strategy

Authentication endpoints use **Redis-backed rate limiting** with sliding window counters. This is separate from the existing in-memory per-IP token bucket middleware.

### Rate Limit Dimensions

| Endpoint | Per-IP Limit | Per-Email Limit |
|---|---|---|
| `POST /auth/login` | 10/min | 5/min |
| `POST /auth/register` | 5/min | — |
| `POST /auth/refresh` | 30/min | — |
| `POST /auth/forgot-password` | 5/min | 3/min |
| `POST /auth/reset-password` | 5/min | — |
| `POST /auth/resend-verification` | 3/min | 3/min |
| `POST /auth/change-password` | 5/min | — |

All limits are configurable via environment variables.

### Implementation

```go
// internal/auth/ratelimit.go
type RateLimiter interface {
    Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, error)
}

// Redis implementation uses INCR + EXPIRE (sliding window counter)
type RedisRateLimiter struct {
    client *redis.Client
}
```

Redis key format: `ratelimit:auth:<endpoint>:<dimension>:<value>`

Example: `ratelimit:auth:login:ip:192.168.1.1`

### Rate Limit Response

When a limit is exceeded, return:

```
HTTP 429 Too Many Requests
Retry-After: <seconds>

{
  "error": {
    "code": "RATE_LIMITED",
    "message": "Too many requests. Please try again later."
  }
}
```

### Degradation

If Redis is unavailable, rate limiting **fails open** — requests are allowed through. This prevents Redis failures from causing a total authentication outage. The failure is logged at `Error` level and a metric is incremented.

> **Trade-off:** Failing open means a Redis outage temporarily disables rate limiting. This is preferable to failing closed, which would lock all users out of authentication during a Redis failure.

---

## 24. Error Handling

### Auth Domain Errors

```go
// internal/auth/errors.go
var (
    ErrInvalidCredentials  = errors.NewUnauthorized("invalid email or password")
    ErrEmailNotVerified    = errors.NewDomainError(errors.CodeForbidden, "email address not verified")
    ErrInvalidToken        = errors.NewUnauthorized("invalid or expired token")
    ErrTokenExpired        = errors.NewBadRequest("token has expired")
    ErrTokenUsed           = errors.NewBadRequest("token has already been used")
    ErrSessionRevoked      = errors.NewUnauthorized("session has been revoked")
    ErrSessionExpired      = errors.NewUnauthorized("session has expired")
    ErrPasswordTooWeak     = errors.NewValidationError("password does not meet requirements")
    ErrSamePassword        = errors.NewValidationError("new password must be different from current password")
    ErrUserNotActive       = errors.NewUnauthorized("invalid email or password")
)
```

### Error Mapping

| Error | HTTP | Code | Notes |
|---|---|---|---|
| `ErrInvalidCredentials` | 401 | `UNAUTHORIZED` | Same for unknown email, wrong password, inactive user |
| `ErrEmailNotVerified` | 403 | `FORBIDDEN` | Only case where we distinguish from invalid credentials |
| `ErrInvalidToken` | 401 | `UNAUTHORIZED` | JWT validation failures |
| `ErrTokenExpired` | 400 | `BAD_REQUEST` | Reset/verification token expired |
| `ErrTokenUsed` | 400 | `BAD_REQUEST` | Reset/verification token already consumed |
| `ErrSessionRevoked` | 401 | `UNAUTHORIZED` | Refresh token for revoked session |
| `ErrPasswordTooWeak` | 422 | `VALIDATION_ERROR` | Password policy violation |
| Infrastructure failure | 500 | `INTERNAL_ERROR` | Masked — no SQL/stack trace leakage |

All errors flow through the existing `httputil.Error()` → `errors.ToAPIError()` pipeline.

### What Errors Must NOT Reveal

- Whether an email address is registered (except registration 409)
- Whether a token was expired vs never existed vs already used (for security-sensitive endpoints)
- Database error details, SQL queries, or stack traces
- Password hashing parameters or timing information
- Internal token formats or session structure

---

## 25. Security Model

### Threat Matrix

| Threat | Vector | Mitigation |
|---|---|---|
| Credential stuffing | Automated login with leaked credentials | Rate limiting (per-IP + per-email) |
| Brute force | Repeated password guessing | Rate limiting + Argon2id intentional slowness |
| Account enumeration | Distinct errors for unknown/known emails | Generic error messages, dummy hash timing |
| Timing attacks | Faster response for unknown users | Constant-time hash comparison, dummy hash for missing users |
| Token theft | Stolen refresh token | Token rotation, reuse detection, session revocation |
| JWT forgery | Crafted/modified JWT | Ed25519 signature verification, algorithm pinning |
| Algorithm confusion | `alg: none` or `alg: HS256` with public key | Explicit algorithm validation — only EdDSA accepted |
| Cross-tenant access | Token from tenant A used in tenant B | JWT `tid` claim validated against `X-Tenant-ID` header |
| Password in logs | Accidental logging of request bodies | Typed DTOs (never log request body), structured logging |
| Reset token interception | Attacker reads reset email | Short-lived tokens (30 min), single-use, hashed storage |
| Session fixation | Attacker pre-sets session ID | Session ID generated server-side, never accepted from client |
| Mass assignment | Client sets admin fields | Typed request DTOs — only declared fields are parsed |
| Replay attack | Reuse of consumed reset/verification token | `used_at` column prevents replay; single-use enforcement |

### Secret Management

| Secret | Storage | Source | Rotation |
|---|---|---|---|
| JWT private key | Environment variable `AUTH_JWT_PRIVATE_KEY` | Injected at deploy | Via `kid` — deploy new key, old key valid for 15 min |
| JWT public key | Environment variable `AUTH_JWT_PUBLIC_KEY` | Injected at deploy | Same as private key |
| Argon2id parameters | Code constant (configurable) | — | Update code, re-hash on next login |
| Database credentials | Environment variable | `.env` file | Standard rotation |
| Redis credentials | Environment variable | `.env` file | Standard rotation |

### Non-Negotiable Security Rules

1. No plaintext passwords — anywhere, ever
2. No plaintext refresh tokens in database
3. No plaintext reset/verification tokens in database
4. No authentication secrets in logs
5. No JWT private keys in source control
6. No account enumeration via error messages
7. No arbitrary JWT algorithms accepted
8. No unverified JWT acceptance
9. Refresh tokens rotate on every use
10. Refresh token reuse triggers session revocation
11. Password reset revokes all sessions
12. All authentication endpoints are rate limited

---

## 26. Database Design

### Table: `user_credentials`

Stores password hashes. One row per user.

```sql
CREATE TABLE user_credentials (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX idx_user_credentials_tenant_user
    ON user_credentials (tenant_id, user_id);

CREATE TRIGGER set_user_credentials_updated_at
    BEFORE UPDATE ON user_credentials
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();
```

| Column | Type | Nullable | Purpose |
|---|---|---|---|
| `id` | UUID | NOT NULL | Primary key |
| `tenant_id` | UUID FK | NOT NULL | Tenant isolation |
| `user_id` | UUID FK | NOT NULL | References `users.id` |
| `password_hash` | TEXT | NOT NULL | Argon2id PHC string |
| `created_at` | TIMESTAMPTZ | NOT NULL | Record creation time |
| `updated_at` | TIMESTAMPTZ | NOT NULL | Last password change time |

---

### Table: `auth_sessions`

Stores authentication sessions and current refresh token state.

```sql
CREATE TABLE auth_sessions (
    id                 UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id          UUID        NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id            UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    family_id          UUID        NOT NULL,
    refresh_token_hash TEXT        NOT NULL,
    expires_at         TIMESTAMPTZ NOT NULL,
    revoked_at         TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_used_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    ip_address         TEXT,
    user_agent         TEXT
);

CREATE INDEX idx_auth_sessions_tenant_user
    ON auth_sessions (tenant_id, user_id);

CREATE INDEX idx_auth_sessions_family
    ON auth_sessions (family_id)
    WHERE revoked_at IS NULL;

CREATE INDEX idx_auth_sessions_expires
    ON auth_sessions (expires_at)
    WHERE revoked_at IS NULL;

CREATE TRIGGER set_auth_sessions_updated_at
    BEFORE UPDATE ON auth_sessions
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();
```

| Column | Type | Nullable | Purpose |
|---|---|---|---|
| `id` | UUID | NOT NULL | Session identifier, used in JWT `sid` claim |
| `tenant_id` | UUID FK | NOT NULL | Tenant isolation |
| `user_id` | UUID FK | NOT NULL | Session owner |
| `family_id` | UUID | NOT NULL | Token family for reuse detection |
| `refresh_token_hash` | TEXT | NOT NULL | SHA-256 of current valid refresh token |
| `expires_at` | TIMESTAMPTZ | NOT NULL | Absolute session expiry (30 days from creation) |
| `revoked_at` | TIMESTAMPTZ | NULL | Non-null = session is revoked |
| `created_at` | TIMESTAMPTZ | NOT NULL | Session creation time |
| `updated_at` | TIMESTAMPTZ | NOT NULL | Last modification time |
| `last_used_at` | TIMESTAMPTZ | NOT NULL | Last successful refresh time |
| `ip_address` | TEXT | NULL | Client IP at session creation |
| `user_agent` | TEXT | NULL | User-Agent at session creation |

**Partial indexes:** The `WHERE revoked_at IS NULL` filter on `family_id` and `expires_at` indexes ensures we only index active sessions, reducing index size and improving lookup performance.

---

### Table: `password_reset_tokens`

```sql
CREATE TABLE password_reset_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    token_hash TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_password_reset_tokens_hash
    ON password_reset_tokens (token_hash)
    WHERE used_at IS NULL;

CREATE INDEX idx_password_reset_tokens_tenant_user
    ON password_reset_tokens (tenant_id, user_id)
    WHERE used_at IS NULL;
```

| Column | Type | Nullable | Purpose |
|---|---|---|---|
| `id` | UUID | NOT NULL | Primary key |
| `tenant_id` | UUID FK | NOT NULL | Tenant isolation |
| `user_id` | UUID FK | NOT NULL | Token owner |
| `token_hash` | TEXT | NOT NULL | SHA-256 of the token |
| `expires_at` | TIMESTAMPTZ | NOT NULL | Token expiry (30 min from creation) |
| `used_at` | TIMESTAMPTZ | NULL | Non-null = token has been consumed |
| `created_at` | TIMESTAMPTZ | NOT NULL | Token creation time |

No `updated_at` or trigger — these tokens are write-once then mark-used.

---

### Table: `email_verification_tokens`

```sql
CREATE TABLE email_verification_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    token_hash TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_email_verification_tokens_hash
    ON email_verification_tokens (token_hash)
    WHERE used_at IS NULL;

CREATE INDEX idx_email_verification_tokens_tenant_user
    ON email_verification_tokens (tenant_id, user_id)
    WHERE used_at IS NULL;
```

Identical structure to `password_reset_tokens` with different table name and expiry duration (24 hours).

---

### Entity-Relationship Diagram

```
┌──────────┐     ┌──────────────────┐     ┌───────────────────────────┐
│ tenants  │     │     users        │     │   user_credentials        │
│          │◄────│  tenant_id (FK)  │◄────│  user_id (FK)             │
│  id (PK) │     │  id (PK)        │     │  tenant_id (FK)           │
│          │     │  email           │     │  password_hash            │
│          │     │  display_name    │     │  id (PK)                  │
│          │     │  status          │     └───────────────────────────┘
│          │     │  email_verified  │
│          │     └────────┬─────────┘
│          │              │
│          │              │ user_id FK
│          │              │
│          │     ┌────────┴─────────┐
│          │◄────│  auth_sessions   │
│          │     │  tenant_id (FK)  │
│          │     │  user_id (FK)    │
│          │     │  family_id       │
│          │     │  refresh_token_  │
│          │     │    hash          │
│          │     │  id (PK)        │
│          │     └────────┬─────────┘
│          │              │
│          │     ┌────────┴──────────────┐   ┌──────────────────────────────┐
│          │◄────│password_reset_tokens  │   │email_verification_tokens     │
│          │     │  tenant_id (FK)       │   │  tenant_id (FK)              │
│          │     │  user_id (FK)         │   │  user_id (FK)                │
│          │     │  token_hash           │   │  token_hash                  │
│          │     │  expires_at           │   │  expires_at                  │
│          │     └───────────────────────┘   └──────────────────────────────┘
```

All Auth tables reference `users.id` and `tenants.id`. The `users` table has no Auth columns.

---

## 27. Transaction Boundaries

### Registration

```
BEGIN;
  INSERT INTO users (...) → user_id
  INSERT INTO user_credentials (user_id, password_hash, ...)
  INSERT INTO email_verification_tokens (user_id, token_hash, ...)
COMMIT;
```

**Why:** If credential creation fails, the user must not exist. If verification token creation fails, the user must not exist without a way to verify their email.

### Token Refresh

```
BEGIN;
  SELECT * FROM auth_sessions WHERE family_id = $1 AND revoked_at IS NULL FOR UPDATE;
  -- Verify token hash matches
  UPDATE auth_sessions SET refresh_token_hash = $new_hash, last_used_at = now(), updated_at = now();
COMMIT;
```

**Why:** `FOR UPDATE` lock prevents concurrent refresh token rotation from creating a race condition. Ensures exactly one rotation succeeds.

### Password Reset Execution

```
BEGIN;
  UPDATE user_credentials SET password_hash = $new_hash WHERE user_id = $1 AND tenant_id = $2;
  UPDATE password_reset_tokens SET used_at = now() WHERE id = $1;
  UPDATE auth_sessions SET revoked_at = now() WHERE user_id = $1 AND tenant_id = $2 AND revoked_at IS NULL;
COMMIT;
```

**Why:** Password change, token invalidation, and session revocation must be atomic. If any step fails, none should take effect.

### Password Change

```
BEGIN;
  UPDATE user_credentials SET password_hash = $new_hash WHERE user_id = $1 AND tenant_id = $2;
  UPDATE auth_sessions SET revoked_at = now() WHERE user_id = $1 AND tenant_id = $2
    AND id != $current_session_id AND revoked_at IS NULL;
COMMIT;
```

**Why:** Must atomically change the password and revoke other sessions (but keep the current session).

### Email Verification

```
BEGIN;
  UPDATE email_verification_tokens SET used_at = now() WHERE id = $1;
  -- Call UserProvider.SetEmailVerified(user_id) which updates users table
COMMIT;
```

**Why:** Token consumption and email verification state must be atomic.

### Operations That Do NOT Need Transactions

- Login (single session insert)
- Logout (single session update)
- Logout all (single update statement)
- Forgot password (single token insert)
- Resend verification (invalidate old + insert new is acceptable non-atomically; worst case: two valid tokens, both work)

---

## 28. Concurrency

### Concurrent Refresh Token Rotation

The primary concurrency challenge is two requests attempting to refresh the same token simultaneously.

**Scenario:** Client makes two requests with Refresh Token A at the same time (e.g., two browser tabs, or request retry on timeout).

**Resolution via `SELECT ... FOR UPDATE`:**

```
Time    Request 1                              Request 2
─────────────────────────────────────────────────────────────
T1      BEGIN                                  BEGIN
T2      SELECT ... FOR UPDATE                  SELECT ... FOR UPDATE
        (acquires lock)                        (BLOCKS — waiting for lock)
T3      Verify Hash(A) = stored_hash ✓
T4      Generate Token B
T5      UPDATE hash = Hash(B)
T6      COMMIT (releases lock)
T7                                             (lock acquired)
T8                                             Verify Hash(A) = stored_hash
T9                                             Hash(A) ≠ Hash(B) → MISMATCH
T10                                            REUSE DETECTED
T11                                            UPDATE revoked_at = now()
T12                                            COMMIT
T13                                            Return 401
```

**Properties:**

- Exactly one request succeeds
- The second request detects reuse and revokes the session
- No application-level mutex is needed
- PostgreSQL row-level locking provides the guarantee

### Concurrent Registration

Two requests to register the same email in the same tenant:

- The `UNIQUE INDEX (tenant_id, lower(email))` on the `users` table prevents duplicate users
- The second request receives `ErrEmailAlreadyExists` (409 Conflict)
- No additional locking is needed

### Concurrent Password Reset

Two requests to use the same reset token:

- The first request sets `used_at = now()` within its transaction
- The second request finds `used_at IS NOT NULL` and returns `ErrTokenUsed`
- The `WHERE used_at IS NULL` condition in the lookup query provides idempotent safety

---

## 29. Observability

### Logging

All auth operations use the request-scoped logger via `zerolog.Ctx(ctx)`.

| Event | Level | Fields |
|---|---|---|
| Registration successful | Info | `user_id`, `tenant_id`, `email_domain` |
| Login successful | Info | `user_id`, `tenant_id`, `session_id` |
| Login failed (invalid credentials) | Warn | `tenant_id`, `email_domain`, `reason: invalid_credentials` |
| Login failed (email not verified) | Warn | `tenant_id`, `email_domain`, `reason: email_not_verified` |
| Token refresh successful | Info | `user_id`, `tenant_id`, `session_id` |
| Refresh token reuse detected | Error | `user_id`, `tenant_id`, `session_id`, `family_id` |
| Session revoked | Info | `user_id`, `tenant_id`, `session_id` |
| All sessions revoked | Info | `user_id`, `tenant_id`, `sessions_revoked: N` |
| Password changed | Info | `user_id`, `tenant_id` |
| Password reset requested | Info | `tenant_id`, `email_domain` |
| Password reset executed | Info | `user_id`, `tenant_id` |
| Email verified | Info | `user_id`, `tenant_id` |
| Rate limit exceeded | Warn | `ip`, `endpoint`, `limit_key` |
| JWT validation failed | Warn | `reason`, `tenant_id` |

**NEVER log:** passwords, password hashes, access tokens, refresh tokens, reset tokens, verification tokens, Authorization header values, token hashes.

### Metrics

```go
// internal/auth/metrics.go
var (
    authOperationsTotal = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Namespace: "conduit",
            Subsystem: "auth",
            Name:      "operations_total",
            Help:      "Total authentication operations.",
        },
        []string{"operation", "status"},
    )

    authOperationDuration = promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Namespace: "conduit",
            Subsystem: "auth",
            Name:      "operation_duration_seconds",
            Help:      "Authentication operation duration.",
            Buckets:   prometheus.DefBuckets,
        },
        []string{"operation"},
    )

    refreshTokenReuse = promauto.NewCounter(
        prometheus.CounterOpts{
            Namespace: "conduit",
            Subsystem: "auth",
            Name:      "refresh_token_reuse_total",
            Help:      "Number of refresh token reuse detections.",
        },
    )

    activeSessionsGauge = promauto.NewGauge(
        prometheus.GaugeOpts{
            Namespace: "conduit",
            Subsystem: "auth",
            Name:      "active_sessions",
            Help:      "Approximate number of active sessions.",
        },
    )
)
```

**Operations:** `register`, `login`, `refresh`, `logout`, `logout_all`, `change_password`, `forgot_password`, `reset_password`, `verify_email`, `resend_verification`

**Status labels:** `success`, `invalid_credentials`, `rate_limited`, `error`

### Tracing

| Span Name | Attributes |
|---|---|
| `auth.service.Register` | `tenant_id` |
| `auth.service.Login` | `tenant_id` |
| `auth.service.RefreshToken` | `tenant_id`, `session_id` |
| `auth.service.Logout` | `tenant_id`, `session_id` |
| `auth.service.LogoutAll` | `tenant_id` |
| `auth.service.ChangePassword` | `tenant_id` |
| `auth.service.ForgotPassword` | `tenant_id` |
| `auth.service.ResetPassword` | `tenant_id` |
| `auth.service.VerifyEmail` | `tenant_id` |
| `auth.service.ResendVerification` | `tenant_id` |

Never include tokens, passwords, or email addresses in span attributes.

### Alerting Recommendations

| Alert | Condition | Severity |
|---|---|---|
| Refresh token reuse spike | `conduit_auth_refresh_token_reuse_total` increases by > 10 in 5 min | Critical |
| Login failure rate | `conduit_auth_operations_total{operation="login",status="invalid_credentials"}` > 50% of total login attempts for 5 min | Warning |
| Auth error rate | `conduit_auth_operations_total{status="error"}` > 1% for 5 min | Critical |
| Password reset spike | `conduit_auth_operations_total{operation="forgot_password"}` > 100 in 5 min | Warning |

---

## 30. Testing Strategy

### Unit Tests — `internal/auth/service_test.go`

**Coverage target: >= 80%**

Mock boundaries: `Repository`, `UserProvider`, `PasswordHasher`, `TokenGenerator`, `JWTIssuer`, `RateLimiter`

| Test | Category |
|---|---|
| `TestRegister_Success` | Happy path — user + credential + verification token created |
| `TestRegister_DuplicateEmail` | Returns 409 CONFLICT |
| `TestRegister_WeakPassword` | Returns 422 VALIDATION_ERROR |
| `TestLogin_Success` | Returns access + refresh tokens |
| `TestLogin_InvalidEmail` | Runs dummy hash, returns generic error |
| `TestLogin_WrongPassword` | Returns same generic error as unknown email |
| `TestLogin_UserDeactivated` | Returns same generic error |
| `TestLogin_EmailNotVerified` | Returns specific "email not verified" error |
| `TestRefresh_Success` | Issues new tokens, updates hash |
| `TestRefresh_ExpiredSession` | Returns unauthorized |
| `TestRefresh_RevokedSession` | Returns unauthorized |
| `TestRefresh_InvalidToken` | Returns unauthorized |
| `TestRefresh_ReuseDetected` | Revokes session, returns unauthorized |
| `TestLogout_Success` | Session revoked |
| `TestLogoutAll_Success` | All sessions revoked |
| `TestChangePassword_Success` | Credential updated, other sessions revoked |
| `TestChangePassword_WrongCurrent` | Returns unauthorized |
| `TestChangePassword_SamePassword` | Returns validation error |
| `TestForgotPassword_ExistingUser` | Token created, returns generic message |
| `TestForgotPassword_UnknownEmail` | No token created, returns same generic message |
| `TestResetPassword_Success` | Credential updated, token consumed, sessions revoked |
| `TestResetPassword_ExpiredToken` | Returns bad request |
| `TestResetPassword_UsedToken` | Returns bad request |
| `TestVerifyEmail_Success` | Token consumed, email verified |
| `TestVerifyEmail_ExpiredToken` | Returns bad request |
| `TestResendVerification_Success` | Old token invalidated, new token created |
| `TestResendVerification_AlreadyVerified` | No-op, returns generic message |

### Crypto Unit Tests — `internal/auth/password_test.go`, `internal/auth/token_test.go`

**Coverage target: 100%**

| Test | Category |
|---|---|
| `TestArgon2id_HashAndVerify` | Round-trip: hash then verify succeeds |
| `TestArgon2id_WrongPassword` | Verification fails for wrong password |
| `TestArgon2id_DifferentSalts` | Same password produces different hashes |
| `TestArgon2id_PHCFormat` | Output matches PHC string format |
| `TestTokenGenerate_Uniqueness` | 1000 tokens are all unique |
| `TestTokenGenerate_Length` | Token has expected entropy |
| `TestTokenHash_Deterministic` | Same token always produces same hash |
| `TestTokenHash_DifferentTokens` | Different tokens produce different hashes |

### JWT Unit Tests — `internal/auth/jwt_test.go`

| Test | Category |
|---|---|
| `TestJWT_IssueAndValidate` | Round-trip: issue then validate succeeds |
| `TestJWT_ExpiredToken` | Validation fails for expired token |
| `TestJWT_WrongIssuer` | Validation fails |
| `TestJWT_WrongAudience` | Validation fails |
| `TestJWT_WrongAlgorithm` | Validation fails (algorithm pinning) |
| `TestJWT_TamperedPayload` | Signature verification fails |
| `TestJWT_MissingClaims` | Validation fails for missing required claims |
| `TestJWT_KeyRotation` | Old key validates old tokens, new key validates new tokens |

### Handler Tests — `internal/auth/handler_test.go`

| Test | Category |
|---|---|
| `TestRegisterHandler_InvalidJSON` | 400 BAD_REQUEST |
| `TestRegisterHandler_MissingEmail` | 422 VALIDATION_ERROR |
| `TestRegisterHandler_MissingPassword` | 422 VALIDATION_ERROR |
| `TestRegisterHandler_ShortPassword` | 422 VALIDATION_ERROR |
| `TestLoginHandler_InvalidJSON` | 400 BAD_REQUEST |
| `TestLoginHandler_MissingFields` | 422 VALIDATION_ERROR |
| `TestRefreshHandler_MissingToken` | 400 BAD_REQUEST |
| `TestLogoutHandler_NoAuth` | 401 UNAUTHORIZED |
| `TestChangePasswordHandler_MissingFields` | 422 VALIDATION_ERROR |

### Middleware Tests — `internal/auth/middleware_test.go`

| Test | Category |
|---|---|
| `TestMiddleware_ValidToken` | Sets principal in context, continues |
| `TestMiddleware_MissingAuthHeader` | 401 |
| `TestMiddleware_MalformedBearer` | 401 |
| `TestMiddleware_ExpiredToken` | 401 |
| `TestMiddleware_WrongAlgorithm` | 401 |
| `TestMiddleware_TenantMismatch` | 401 (JWT tid ≠ X-Tenant-ID) |

### Integration Tests — `tests/integration/auth_test.go`

**Build tag:** `//go:build integration`

| Test | Category |
|---|---|
| `TestAuth_RegisterAndLogin` | Full registration → verify email → login flow |
| `TestAuth_DuplicateRegistration` | Same email same tenant → 409 |
| `TestAuth_LoginUnverifiedEmail` | Registration → login without verify → 403 |
| `TestAuth_LoginWrongPassword` | Generic 401 response |
| `TestAuth_LoginUnknownEmail` | Same generic 401 response |
| `TestAuth_RefreshToken` | Login → refresh → verify new tokens work |
| `TestAuth_RefreshTokenRotation` | Old token invalid after rotation |
| `TestAuth_RefreshTokenReuse` | Use old token → session revoked |
| `TestAuth_ConcurrentRefresh` | Two goroutines, same token → one succeeds, session revoked |
| `TestAuth_Logout` | Logout → refresh with old token fails |
| `TestAuth_LogoutAll` | Multiple sessions → logout all → all fail |
| `TestAuth_ChangePassword` | Change → old sessions revoked → login with new password |
| `TestAuth_ForgotResetPassword` | Forgot → reset → login with new password |
| `TestAuth_ResetTokenExpiry` | Expired token → 400 |
| `TestAuth_ResetTokenReuse` | Used token → 400 |
| `TestAuth_EmailVerification` | Token → verify → email_verified = true |
| `TestAuth_RateLimiting` | Exceed login limit → 429 |
| `TestAuth_AccountEnumerationProtection` | Same timing for known vs unknown emails |
| `TestAuth_TenantIsolation` | User in tenant A cannot auth in tenant B |

### Security Tests

| Test | Category |
|---|---|
| `TestSecurity_NoPasswordInResponse` | No `password` or `password_hash` in any response |
| `TestSecurity_NoTokenInLogs` | Verify structured logging excludes secrets |
| `TestSecurity_AlgorithmPinning` | JWT with `alg: HS256` rejected |
| `TestSecurity_ErrorLeakage` | 500 errors return generic message, no SQL/stack trace |

---

## 31. Future Extraction Strategy

### Current State: Modular Monolith

The Auth domain runs in the same process as the User domain and shares the same PostgreSQL database and Redis instance. Cross-domain communication uses direct Go function calls via interfaces.

### Extraction Boundary

If the Auth domain needs to become an independent service:

1. **Database:** Auth-owned tables (`user_credentials`, `auth_sessions`, `password_reset_tokens`, `email_verification_tokens`) move to a dedicated database. Foreign keys to `users` are replaced with eventual consistency checks.

2. **UserProvider interface:** Replace the direct function call implementation with an HTTP/gRPC client that calls the User service API.

3. **Transaction boundaries:** Cross-domain transactions (registration) become sagas or choreography. Registration would: (a) call User service to create user, (b) create credentials locally, (c) if credential creation fails, call User service to delete user (compensating action).

4. **JWT validation:** The public key can be distributed to all services. JWT validation remains stateless — no service-to-service call needed.

5. **Rate limiting:** Redis-backed rate limiting already works across multiple instances — no change needed.

### What Makes Extraction Possible

| Design Decision | Why It Helps |
|---|---|
| Auth owns its own tables | No shared schema to split |
| UserProvider interface | Swap local calls for network calls |
| JWT is self-contained | No service call needed for validation |
| Redis rate limiting | Already works multi-instance |
| No auth state in User domain | User has no `password_hash` column to migrate |

### What Would NOT Be Done Now

- Kafka for auth events
- gRPC between Auth and User
- Separate Auth database
- Service mesh
- API gateway for token validation
- Distributed transactions

The current monolith architecture is correct. This section documents the extraction path — it does not implement it.

---

## 32. Implementation Plan

### Phase 1: Foundation (PR-1)

**Database + Domain Model + Crypto Infrastructure**

| File | Content |
|---|---|
| `migrations/000003_create_auth_tables.up.sql` | All 4 auth tables |
| `migrations/000003_create_auth_tables.down.sql` | Drop all 4 auth tables |
| `sqlc/queries/auth.sql` | All auth domain queries |
| `internal/auth/model.go` | Session, Credential, token types |
| `internal/auth/errors.go` | Auth domain errors |
| `internal/auth/password.go` | Argon2id hasher |
| `internal/auth/password_test.go` | Crypto unit tests |
| `internal/auth/token.go` | Token generator + SHA-256 hasher |
| `internal/auth/token_test.go` | Token unit tests |
| `internal/auth/jwt.go` | Ed25519 JWT issuer + validator |
| `internal/auth/jwt_test.go` | JWT unit tests |

**New dependency:** `golang.org/x/crypto` (for argon2), `github.com/golang-jwt/jwt/v5` (for JWT).

### Phase 2: Repository (PR-2)

| File | Content |
|---|---|
| `internal/auth/repository.go` | Repository interface + PostgreSQL implementation |

### Phase 3: Service + Observability (PR-3)

| File | Content |
|---|---|
| `internal/auth/service.go` | All auth business logic |
| `internal/auth/metrics.go` | Prometheus metrics |
| `internal/auth/ratelimit.go` | Redis rate limiter |
| `internal/auth/service_test.go` | Service unit tests |

### Phase 4: Transport + Middleware (PR-4)

| File | Content |
|---|---|
| `internal/auth/handler.go` | HTTP handlers for all 10 endpoints |
| `internal/auth/dto.go` | Request/Response DTOs |
| `internal/auth/middleware.go` | JWT authentication middleware |
| `internal/auth/handler_test.go` | Handler tests |
| `internal/auth/middleware_test.go` | Middleware tests |

### Phase 5: Wiring + Integration (PR-5)

| File | Content |
|---|---|
| `internal/config/config.go` | Add `AuthConfig` (JWT keys, token lifetimes, Argon2 params) |
| `internal/app/app.go` | Wire auth domain dependencies |
| `internal/server/server.go` | Register auth routes, apply auth middleware |
| `tests/integration/auth_test.go` | Integration test suite |

---

## 33. Milestones

| # | Milestone | Deliverable | Estimate |
|---|---|---|---|
| M1 | Auth foundation | Database schema, crypto primitives, JWT infrastructure (PR-1) | 2 days |
| M2 | Repository layer | sqlc queries, repository implementation (PR-2) | 1 day |
| M3 | Service layer | Business logic, rate limiting, observability (PR-3) | 3 days |
| M4 | Transport layer | HTTP handlers, middleware, DTOs (PR-4) | 2 days |
| M5 | Integration | Wiring, config, integration tests, E2E verification (PR-5) | 2 days |
| M6 | Security review | Peer review focusing on crypto, timing, enumeration (cross-cutting) | 1 day |

**Total estimated effort:** ~11 engineering days

---

## 34. Risks & Trade-offs

### Risks

| Risk | Probability | Impact | Mitigation |
|---|---|---|---|
| Argon2id DoS via many concurrent login attempts | Medium | High (CPU exhaustion) | Rate limiting per IP/email; max concurrent hash goroutines |
| JWT private key compromise | Low | Critical | Short token lifetime (15 min), key rotation via kid, env-only storage |
| Clock skew causes premature JWT rejection | Low | Medium | 5-second tolerance in JWT validation |
| Redis failure disables rate limiting | Medium | Medium | Fail open with logging; monitor Redis health |
| Cross-domain transaction failure during registration | Low | Medium | Atomic transaction; if any step fails, all roll back |

### Trade-offs

| Decision | Trade-off | Why We Accept It |
|---|---|---|
| No JWT blocklist | Revoked session's JWT valid for up to 15 min | Short token lifetime makes this acceptable; avoids Redis lookup per request |
| Argon2id over bcrypt | Requires `golang.org/x/crypto` dependency | PHC winner, better GPU resistance, industry standard |
| Separate `user_credentials` table | Extra JOIN for login (credential lookup) | Cleaner domain boundary; supports future multi-credential types |
| Opaque refresh tokens over JWT | Requires database lookup on refresh | Enables revocation and reuse detection; refresh happens infrequently |
| SHA-256 for token hashing (not Argon2id) | Faster to brute force than Argon2id | Tokens have 256 bits of entropy — brute force is infeasible regardless of hash speed |
| Fail-open rate limiting | Temporary window without protection during Redis outage | Preferable to locking all users out of auth |
| No MFA in v1 | Lower security ceiling | Significant scope increase; planned for follow-up RFC |

---

## 35. Decisions

| # | Decision | Rationale | Reversibility |
|---|---|---|---|
| D1 | Argon2id over bcrypt | PHC winner, memory-hard, OWASP recommended | Low — password hashes are algorithm-specific. Migration requires re-hash on login. |
| D2 | EdDSA over RS256 | Smaller keys/signatures, faster, no nonce reuse risk | Medium — requires reissuing all JWTs (15 min window) |
| D3 | Separate `user_credentials` table | Clean domain boundary, extensible for future auth methods | Low — migration to column on `users` would be destructive |
| D4 | Opaque refresh tokens (not JWT) | Enables server-side revocation and reuse detection | Low — client change required to switch token format |
| D5 | 15-minute access token lifetime | Balances security (short blast radius) and UX (reasonable refresh frequency) | High — configurable at any time |
| D6 | 30-day refresh token lifetime | Supports "remember me" UX without explicit cookie management | High — configurable at any time |
| D7 | No JWT blocklist in v1 | Avoids per-request Redis lookup; 15-min max exposure | High — can add later with Redis |
| D8 | Redis-backed rate limiting | Consistent across multiple instances, avoids in-memory growth | Medium — could switch to in-memory with trade-offs |
| D9 | Token family-based reuse detection | Enables identifying token theft vs legitimate race conditions | Low — fundamental to security model |
| D10 | No account lockout | Prevents lockout-based DoS | High — can add with careful anti-DoS design |

---

## 36. Acceptance Criteria

Before approving this RFC, verify:

| # | Criterion | Status |
|---|---|---|
| 1 | All 10 API endpoints are defined with request/response contracts | ☐ |
| 2 | Authentication does not implement authorization | ☐ |
| 3 | Authentication does not own user profile data | ☐ |
| 4 | Password hashing uses Argon2id with configurable parameters | ☐ |
| 5 | JWT uses EdDSA with key rotation support | ☐ |
| 6 | Refresh tokens are opaque, hashed, and rotated | ☐ |
| 7 | Reuse detection revokes entire session | ☐ |
| 8 | Concurrency handled via `SELECT ... FOR UPDATE` | ☐ |
| 9 | Account enumeration prevented on login, forgot-password, resend-verification | ☐ |
| 10 | Rate limiting is Redis-backed and configurable | ☐ |
| 11 | Transaction boundaries are explicitly defined | ☐ |
| 12 | Database schema follows repository conventions (UUIDs, TIMESTAMPTZ, tenant FK, indexes) | ☐ |
| 13 | Error model uses existing three-tier hierarchy (DomainError, ValidationError, InfraError) | ☐ |
| 14 | Observability covers metrics, traces, and structured logging | ☐ |
| 15 | No authentication secret can appear in logs | ☐ |
| 16 | Testing strategy covers unit, integration, security, and concurrency | ☐ |
| 17 | API contract follows existing `{"data": ...}` / `{"error": ...}` envelope | ☐ |
| 18 | Implementation can be broken into independently reviewable PRs | ☐ |
| 19 | No unnecessary infrastructure introduced | ☐ |
| 20 | Future extraction path documented without premature implementation | ☐ |
| 21 | `security-standards.md` update for Argon2id and EdDSA approved by TDR | ☑ ([TDR-0001](decisions/0001-authentication-cryptography.md)) |

---

> **Decision requested:** Approve this RFC to proceed with implementation per the PR breakdown in section 32.
