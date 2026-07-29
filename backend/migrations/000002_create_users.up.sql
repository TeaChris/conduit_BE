-- 000002_create_users.up.sql
-- User domain: user lifecycle management.

CREATE TABLE users (
    id                UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         UUID        NOT NULL REFERENCES tenants(id),
    email             TEXT        NOT NULL,
    display_name      TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'active'
                                  CHECK (status IN ('active', 'deactivated', 'suspended')),
    email_verified    BOOLEAN     NOT NULL DEFAULT false,
    email_verified_at TIMESTAMPTZ,
    metadata          JSONB       NOT NULL DEFAULT '{}',
    deactivated_at    TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Per-tenant email uniqueness (case-insensitive).
CREATE UNIQUE INDEX idx_users_tenant_email ON users (tenant_id, lower(email));

-- Tenant-scoped queries.
CREATE INDEX idx_users_tenant_id ON users (tenant_id);

-- Status filtering within a tenant.
CREATE INDEX idx_users_tenant_status ON users (tenant_id, status);

-- Email verification queries.
CREATE INDEX idx_users_email_verified ON users (tenant_id, email_verified);

-- Auto-update updated_at on row modification.
CREATE TRIGGER set_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();
