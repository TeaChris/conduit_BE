-- 000003_create_auth_tables.down.sql
-- Rollback authentication tables in reverse order of creation.

DROP TABLE IF EXISTS email_verification_tokens;
DROP TABLE IF EXISTS password_reset_tokens;
DROP TRIGGER IF EXISTS set_auth_sessions_updated_at ON auth_sessions;
DROP TABLE IF EXISTS auth_sessions;
DROP TRIGGER IF EXISTS set_user_credentials_updated_at ON user_credentials;
DROP TABLE IF EXISTS user_credentials;
