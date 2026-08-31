-- 000003_create_auth_tables.up.sql
-- Authentication domain: credentials, sessions, and token storage.
-- Implements RFC-0002: Authentication Architecture & Design.
-- TDR-0001: Argon2id for password hashing, EdDSA/Ed25519 for JWT signing.

-- ============================================================
-- 1. user_credentials
-- ============================================================
-- Stores password hashes (Argon2id PHC strings).
-- One credential per user per tenant.
-- The database stores only the hash — never plaintext passwords.

CREATE TABLE user_credentials (
    id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     UUID        NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id       UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One password credential per user within a tenant.
CREATE UNIQUE INDEX idx_user_credentials_tenant_user
    ON user_credentials (tenant_id, user_id);

CREATE TRIGGER set_user_credentials_updated_at
    BEFORE UPDATE ON user_credentials
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================
-- 2. auth_sessions
-- ============================================================
-- Stores authentication sessions and refresh token state.
-- Supports multiple concurrent sessions per user (different devices).
-- refresh_token_hash is the SHA-256 hash of the current valid refresh token.
-- family_id groups all tokens from a single login event for reuse detection.

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

-- Lookup sessions by user within a tenant (e.g., list active sessions, revoke all).
CREATE INDEX idx_auth_sessions_tenant_user
    ON auth_sessions (tenant_id, user_id);

-- Lookup active sessions by family_id for refresh-token rotation and reuse detection.
-- Partial index: only active (non-revoked) sessions are indexed.
CREATE INDEX idx_auth_sessions_family
    ON auth_sessions (family_id)
    WHERE revoked_at IS NULL;

-- Cleanup: find expired sessions that can be purged.
-- Partial index: only active sessions need expiry checks.
CREATE INDEX idx_auth_sessions_expires
    ON auth_sessions (expires_at)
    WHERE revoked_at IS NULL;

CREATE TRIGGER set_auth_sessions_updated_at
    BEFORE UPDATE ON auth_sessions
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- ============================================================
-- 3. password_reset_tokens
-- ============================================================
-- Stores hashed password reset tokens.
-- Tokens are single-use (used_at enforces consumption).
-- The plaintext token is NEVER stored — only the SHA-256 hash.
-- No updated_at: these are write-once, then mark-used.

CREATE TABLE password_reset_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    token_hash TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Lookup unconsumed tokens by hash for reset validation.
-- Partial index: only unused tokens need to be found.
CREATE INDEX idx_password_reset_tokens_hash
    ON password_reset_tokens (token_hash)
    WHERE used_at IS NULL;

-- Invalidate previous tokens for a user when a new one is created.
-- Partial index: only unused tokens need invalidation.
CREATE INDEX idx_password_reset_tokens_tenant_user
    ON password_reset_tokens (tenant_id, user_id)
    WHERE used_at IS NULL;

-- ============================================================
-- 4. email_verification_tokens
-- ============================================================
-- Stores hashed email verification tokens.
-- Same structure as password_reset_tokens.
-- The plaintext token is NEVER stored — only the SHA-256 hash.

CREATE TABLE email_verification_tokens (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  UUID        NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
    user_id    UUID        NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    token_hash TEXT        NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Lookup unconsumed tokens by hash for verification validation.
CREATE INDEX idx_email_verification_tokens_hash
    ON email_verification_tokens (token_hash)
    WHERE used_at IS NULL;

-- Invalidate previous tokens for a user when a new one is requested.
CREATE INDEX idx_email_verification_tokens_tenant_user
    ON email_verification_tokens (tenant_id, user_id)
    WHERE used_at IS NULL;
