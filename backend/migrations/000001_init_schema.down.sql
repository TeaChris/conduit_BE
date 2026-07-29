-- 000001_init_schema.down.sql
-- Rollback foundation schema.

DROP TRIGGER IF EXISTS set_tenants_updated_at ON tenants;
DROP TABLE IF EXISTS tenants;
DROP FUNCTION IF EXISTS trigger_set_updated_at();
