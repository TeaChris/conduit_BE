-- 000001_init_schema.up.sql
-- Foundation schema for Conduit Notification Platform.

-- Enable UUID generation.
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Trigger function to automatically update updated_at columns.
-- Reusable across all tables that have an updated_at column.
CREATE OR REPLACE FUNCTION trigger_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Tenants table: the foundation of multi-tenancy.
CREATE TABLE tenants (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name       TEXT NOT NULL,
    slug       TEXT NOT NULL UNIQUE,
    status     TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    metadata   JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TRIGGER set_tenants_updated_at
    BEFORE UPDATE ON tenants
    FOR EACH ROW
    EXECUTE FUNCTION trigger_set_updated_at();

-- Index for lookups by slug.
CREATE INDEX idx_tenants_slug ON tenants (slug);

-- Index for filtering by status.
CREATE INDEX idx_tenants_status ON tenants (status);
