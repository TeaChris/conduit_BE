# TDR-0001 — Authentication Cryptography Standards

## Status

**Accepted**

## Date

2026-08-19

## Approvers

Platform Engineering — Technical Design Review Committee

## Related Documents

- [RFC-0002 — Authentication Architecture & Design](../rfc-0002-authentication.md)
- [Security Standards](../../engineering/security-standards.md)
- [Engineering Principles](../../engineering/engineering-principles.md)
- [Architecture Principles](../../engineering/architecture-principles.md)

---

## Context

The Conduit Notification Platform is a new reusable notification platform currently in early development. No authentication system has been implemented. No user credentials exist in any environment. No JWTs have been issued.

RFC-0002 defines the authentication architecture for the platform, which requires two foundational technology decisions:

1. **Password hashing algorithm** — how the platform stores and verifies user passwords
2. **JWT signing algorithm** — how the platform signs and verifies access tokens

The existing `security-standards.md` was written during the platform foundation phase as forward-looking guidance. It specified:

- **bcrypt** with a minimum cost factor of 12 for password hashing (§5)
- **RS256** as an example strong JWT signing algorithm (§3)

RFC-0002 proposes:

- **Argon2id** for password hashing
- **EdDSA using Ed25519** for JWT signing

Both bcrypt and RS256 were reasonable initial recommendations. This TDR evaluates whether Argon2id and EdDSA are better choices for a platform being built from scratch in 2026 with no existing credentials or tokens to migrate.

---

## Decision

### Password Hashing: Argon2id

**Argon2id is the standard password hashing algorithm for all new credentials on the Conduit platform.**

Initial parameters (per RFC-0002):

| Parameter | Value |
|---|---|
| Memory | 64 MiB (65536 KiB) |
| Iterations | 3 |
| Parallelism | 4 |
| Salt length | 16 bytes |
| Key length | 32 bytes |

These parameters are based on the OWASP 2023 recommendation for Argon2id and target a hash computation time of 200–500ms on server hardware.

**Operational requirements:**

- Parameters must be centralized in a single configuration struct (`Argon2Params`) and must not be scattered throughout application code.
- The password hashing implementation must expose a `PasswordHasher` interface to allow the service layer to remain independent of the specific algorithm.
- Output format must use the PHC string format (`$argon2id$v=19$m=65536,t=3,p=4$<salt>$<hash>`), which self-describes the algorithm and parameters used — enabling future parameter upgrades without a flag day.
- Parameters may be tuned operationally (e.g., increasing memory or iterations) without a code change to the hashing interface.

### JWT Signing: EdDSA / Ed25519

**EdDSA using Ed25519 is the standard JWT signing algorithm for authentication access tokens on the Conduit platform.**

**Operational requirements:**

- JWTs must include a `kid` (Key ID) header to support key rotation.
- Private signing keys must be injected via environment variable (`AUTH_JWT_PRIVATE_KEY`). Private keys must never be committed to source control.
- JWT verification must explicitly validate that the token's `alg` header is `EdDSA`. Tokens using any other algorithm (including `none`, `HS256`, `RS256`) must be rejected. This prevents algorithm confusion attacks.
- Key rotation is performed by deploying a new key pair with a new `kid`. The previous public key remains valid for verification until all tokens signed with the old key have expired (maximum 15 minutes, per RFC-0002's access token lifetime).

---

## Alternatives Considered

### bcrypt

bcrypt was the original recommendation in `security-standards.md`. It is a well-established password hashing algorithm with a strong track record:

**Why bcrypt was selected originally:**

- Mature, battle-tested algorithm (published 1999)
- Available in Go's standard extended library (`golang.org/x/crypto/bcrypt`)
- Widely understood by engineers
- Configurable cost factor allows increasing computational difficulty over time
- No known practical attacks against properly configured bcrypt

**Why Argon2id is preferred for this new platform:**

- Argon2id is the winner of the 2015 Password Hashing Competition (PHC), specifically designed to address the limitations of prior algorithms
- Argon2id is **memory-hard**: it requires 64 MiB of RAM per hash (configurable), which makes GPU and ASIC-based cracking attacks dramatically more expensive. bcrypt uses only ~4 KiB of memory, offering minimal resistance to GPU-based attacks
- Argon2id combines the best properties of Argon2i (side-channel resistance) and Argon2d (GPU resistance)
- Argon2id is the current OWASP recommendation for new applications (2023)
- The PHC string format is self-describing, enabling smooth parameter upgrades
- Since no existing credentials use bcrypt, there is no migration cost

**bcrypt is not broken or insecure.** It remains a sound choice for existing systems. For a new platform with no legacy constraints, Argon2id provides stronger guarantees against modern hardware-accelerated attacks.

### RS256

RS256 (RSASSA-PKCS1-v1_5 with SHA-256) was mentioned as an example strong algorithm in `security-standards.md`. It is a widely used JWT signing algorithm:

**Why RS256 was selected originally:**

- Widely supported across languages, frameworks, and cloud identity providers
- Well-understood asymmetric signature scheme
- Strong security track record for JWT signing
- Supported by all major JWT libraries

**Why EdDSA / Ed25519 is preferred for this platform:**

- **Smaller signatures**: Ed25519 produces 64-byte signatures vs 256 bytes for RS256, reducing JWT size
- **Smaller keys**: Ed25519 uses 32-byte private keys and 32-byte public keys vs 2048-bit (256-byte) RSA keys
- **Faster verification**: Ed25519 verification is significantly faster than RSA, which matters for per-request JWT validation
- **Deterministic signing**: EdDSA does not use a random nonce during signing. This eliminates an entire class of implementation vulnerabilities (nonce reuse/bias attacks that have historically affected ECDSA and RSA-PSS)
- **No padding oracle surface**: RSA PKCS#1 v1.5 has a history of padding oracle attacks (Bleichenbacher's attack). While these do not directly affect JWT verification, they are part of RSA's broader attack surface
- **Modern standard**: Ed25519 is specified in RFC 8032 and EdDSA for JOSE in RFC 8037. It is supported by `github.com/golang-jwt/jwt/v5`

**RS256 is not broken or insecure.** It remains a valid choice, particularly for systems that must interoperate with legacy identity providers or infrastructure that does not support EdDSA. For a new platform with no existing tokens or integrations, EdDSA provides better performance characteristics and a smaller attack surface.

---

## Compatibility Considerations

### Existing Credentials

**No existing password credentials use bcrypt (or any algorithm).** The platform has not implemented authentication. No `user_credentials` table exists. No password hashes are stored in any environment.

**Migration compatibility is not required.**

### Existing Tokens

**No existing JWTs use RS256 (or any algorithm).** The platform has not implemented JWT-based authentication. No JWTs have been issued. No signing keys exist.

**Migration compatibility is not required.**

### Existing Code

A repository-wide search confirmed:

| Search term | Go source files | `go.mod` | Documentation |
|---|---|---|---|
| `bcrypt` | 0 matches | Not present | `security-standards.md` only |
| `RS256` | 0 matches | Not present | `security-standards.md` only |
| `argon2` | 0 matches | Not present | `rfc-0002-authentication.md` only |
| `EdDSA` / `Ed25519` | 0 matches | Not present | `rfc-0002-authentication.md` only |

No Go code, configuration, tests, or dependencies reference any of these algorithms. The only references are in documentation.

### External Integrations

The platform has no external authentication integrations. No OAuth2 providers, SAML providers, or external JWT consumers depend on the signing algorithm.

### Conclusion

There is no existing implementation, data, or integration that depends on bcrypt or RS256. Adopting Argon2id and EdDSA requires only updating the engineering standards documentation to reflect the new authoritative choices.

---

## Consequences

### Positive

1. **Stronger password security**: Argon2id's memory-hardness provides substantially better resistance to GPU/ASIC-accelerated cracking compared to bcrypt's CPU-only cost model.
2. **Smaller JWTs**: EdDSA's 64-byte signatures reduce token size, which benefits every authenticated HTTP request (tokens travel in the `Authorization` header).
3. **Faster token verification**: EdDSA verification is faster than RSA, reducing per-request overhead for the authentication middleware.
4. **Deterministic signing**: Eliminates nonce-related vulnerabilities entirely.
5. **Modern standards alignment**: Both Argon2id and EdDSA represent current industry best practices (OWASP 2023, RFC 8032/8037).
6. **Self-describing hash format**: Argon2id's PHC string format encodes algorithm and parameters in the hash itself, simplifying future parameter upgrades.
7. **No migration burden**: Starting fresh means no credential re-hashing or token reissuance is needed.

### Negative

1. **Less familiarity**: Some engineers may be less familiar with Argon2id and EdDSA compared to bcrypt and RSA. This is mitigated by clear documentation and centralized implementations behind interfaces.
2. **External interop**: If the platform later needs to interoperate with legacy identity providers that only support RS256, a multi-algorithm JWT verification strategy would need to be added. This is a future concern — not a current constraint.
3. **Dependency**: Argon2id requires `golang.org/x/crypto/argon2`. EdDSA JWT support requires `github.com/golang-jwt/jwt/v5`. Both are well-maintained, widely used libraries.

### Operational Considerations

1. **Memory usage**: Each concurrent Argon2id hash operation consumes 64 MiB of RAM. Under heavy load (e.g., mass login), this can be significant. RFC-0002 specifies rate limiting (10 login requests/min per IP) and recommends limiting concurrent hash goroutines to prevent memory exhaustion.
2. **Key management**: Ed25519 key pairs must be generated offline and distributed via environment variables. Key rotation requires deploying a new key with a new `kid`. The operational procedure must be documented before production deployment.
3. **Monitoring**: Argon2id hash duration should be monitored to ensure parameters remain appropriate as server hardware changes. If hash time drops below 200ms, parameters should be increased.

### Key Management Considerations

1. Ed25519 private keys are 32 bytes (64 bytes in the common "seed + public" format). They must be stored as PEM-encoded environment variables.
2. Key rotation uses the `kid` JWT header. During rotation:
   - Deploy the new key pair (new `kid`)
   - The application loads both old and new public keys for verification
   - Old tokens expire naturally (within 15 minutes)
   - After the overlap window, the old key can be removed
3. Private keys must never appear in source control, logs, error messages, or API responses.

---

## Security Considerations

### Password Hashing

- All passwords are hashed with Argon2id before storage. Plaintext passwords are never written to disk, database, cache, or log.
- The PHC string format preserves algorithm and parameter metadata in the hash itself, enabling the system to verify hashes created with older parameters while hashing new passwords with current parameters.

### Password Verification

- Password verification uses Argon2id's built-in constant-time comparison. This prevents timing attacks that could reveal whether a password prefix is correct.
- When a login attempt targets a non-existent email, the system performs a dummy hash verification against a pre-computed hash to prevent timing-based account enumeration.

### JWT Signing

- JWTs are signed with Ed25519 using the private key loaded from the `AUTH_JWT_PRIVATE_KEY` environment variable.
- The signing operation is deterministic — the same claims always produce the same signature. This eliminates nonce-related vulnerabilities.

### JWT Verification

- Verification explicitly checks that the `alg` header is `EdDSA`. Any other algorithm (including `none`, `HS256`, `RS256`) causes immediate rejection.
- The `kid` header is used to select the correct public key from the loaded key set.
- Standard claims (`iss`, `aud`, `exp`, `sub`, `sid`, `tid`) are validated on every request.
- Clock skew tolerance is limited to 5 seconds.

### Key Rotation

- Key rotation is performed by deploying a new Ed25519 key pair with a new `kid`.
- The application maintains a set of valid public keys (current + previous), keyed by `kid`.
- Old keys are removed after the access token lifetime (15 minutes) has elapsed.
- No service downtime is required for key rotation.

### Secret Management

- JWT private keys: environment variable, never in source control
- Argon2id parameters: centralized code constant (configurable), not a secret
- Password hashes: stored in `user_credentials` table, never logged or returned in API responses
- Refresh tokens: stored as SHA-256 hashes, plaintext never persisted
- Reset/verification tokens: stored as SHA-256 hashes, plaintext never persisted

---

## Implementation Impact

### Engineering Standards Changes

| Document | Section | Change Required |
|---|---|---|
| `security-standards.md` §3 | Authentication (Future State) | Replace RS256 recommendation with EdDSA/Ed25519 |
| `security-standards.md` §5 | Password Handling | Replace bcrypt requirement with Argon2id |
| `security-standards.md` §10 | OWASP A02 | Update hashing reference from bcrypt to Argon2id |

### Implementation Components (per RFC-0002)

| Component | File | Description |
|---|---|---|
| Password hasher | `internal/auth/password.go` | Argon2id implementation behind `PasswordHasher` interface |
| JWT issuer | `internal/auth/jwt.go` | Ed25519 signing/verification behind `JWTIssuer` interface |
| Configuration | `internal/config/config.go` | `AuthConfig` with Argon2id params, JWT key paths, token lifetimes |

### New Dependencies

| Package | Version | Purpose |
|---|---|---|
| `golang.org/x/crypto` | latest | Argon2id (`argon2.IDKey`) |
| `github.com/golang-jwt/jwt/v5` | v5.x | JWT signing/validation with EdDSA support |

### No Changes Required

- No existing Go code uses bcrypt or RS256
- No database migration for existing credentials
- No token reissuance
- No changes to the User domain
- No changes to existing middleware
- No changes to existing API contracts
