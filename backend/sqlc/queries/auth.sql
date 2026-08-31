-- ============================================================
-- Authentication Domain: SQLC Queries
-- Implements persistence operations required by RFC-0002.
-- ============================================================

-- ============================================================
-- user_credentials
-- ============================================================

-- name: CreateCredential :one
-- Creates a password credential for a user.
-- The password_hash is an Argon2id PHC string produced by the application layer.
INSERT INTO user_credentials (tenant_id, user_id, password_hash)
VALUES (@tenant_id, @user_id, @password_hash)
RETURNING *;

-- name: GetCredentialByUserID :one
-- Retrieves the password credential for a user within a tenant.
-- Used during login to obtain the hash for password verification.
SELECT * FROM user_credentials
WHERE tenant_id = @tenant_id AND user_id = @user_id;

-- name: UpdatePasswordHash :one
-- Updates the password hash for an existing credential.
-- Used during password change and password reset flows.
UPDATE user_credentials
SET password_hash = @password_hash,
    updated_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id
RETURNING *;

-- ============================================================
-- auth_sessions
-- ============================================================

-- name: CreateSession :one
-- Creates a new authentication session after successful login.
-- family_id is set once at login and remains constant for the session lifetime.
-- refresh_token_hash is the SHA-256 hash of the refresh token.
INSERT INTO auth_sessions (
    tenant_id, user_id, family_id, refresh_token_hash,
    expires_at, ip_address, user_agent
)
VALUES (
    @tenant_id, @user_id, @family_id, @refresh_token_hash,
    @expires_at, @ip_address, @user_agent
)
RETURNING *;

-- name: GetSessionByID :one
-- Retrieves a session by its primary key.
-- Used when the JWT sid claim needs to be resolved to a session.
SELECT * FROM auth_sessions
WHERE id = @id AND tenant_id = @tenant_id;

-- name: GetActiveSessionByFamilyID :one
-- Retrieves the active (non-revoked) session for a token family.
-- Uses SELECT ... FOR UPDATE to acquire a row-level lock.
--
-- CONCURRENCY: This is the critical query for refresh-token rotation safety.
-- The FOR UPDATE lock serializes concurrent refresh attempts:
--   1. Request A acquires the lock, verifies the hash, rotates the token, commits.
--   2. Request B blocks on FOR UPDATE until A commits.
--   3. Request B then sees the updated hash, which doesn't match its token.
--   4. Request B detects reuse and revokes the session.
--
-- The WHERE clause ensures only active, non-expired sessions are returned.
SELECT * FROM auth_sessions
WHERE family_id = @family_id
  AND revoked_at IS NULL
  AND expires_at > now()
FOR UPDATE;

-- name: RotateRefreshToken :one
-- Atomically updates the refresh token hash and last_used_at timestamp.
-- Called after successfully validating the current refresh token.
-- The caller MUST hold the row lock acquired by GetActiveSessionByFamilyID.
UPDATE auth_sessions
SET refresh_token_hash = @refresh_token_hash,
    last_used_at = now(),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id
RETURNING *;

-- name: RevokeSession :exec
-- Revokes a single session by setting revoked_at.
-- Idempotent: revoking an already-revoked session is a no-op.
UPDATE auth_sessions
SET revoked_at = now(),
    updated_at = now()
WHERE id = @id AND tenant_id = @tenant_id AND revoked_at IS NULL;

-- name: RevokeAllUserSessions :exec
-- Revokes all active sessions for a user within a tenant.
-- Used during: logout-all, password reset, password change (other sessions).
UPDATE auth_sessions
SET revoked_at = now(),
    updated_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id AND revoked_at IS NULL;

-- name: RevokeOtherUserSessions :exec
-- Revokes all active sessions for a user EXCEPT the specified session.
-- Used during password change: keep the current session, revoke all others.
UPDATE auth_sessions
SET revoked_at = now(),
    updated_at = now()
WHERE tenant_id = @tenant_id
  AND user_id = @user_id
  AND id != @session_id
  AND revoked_at IS NULL;

-- name: DeleteExpiredSessions :exec
-- Removes sessions that have been expired or revoked for longer than the
-- specified retention period. Used by cleanup jobs.
DELETE FROM auth_sessions
WHERE (expires_at < @before AND revoked_at IS NOT NULL)
   OR (expires_at < @before AND expires_at < now());

-- ============================================================
-- password_reset_tokens
-- ============================================================

-- name: CreatePasswordResetToken :one
-- Creates a new password reset token.
-- The token_hash is the SHA-256 hash of the plaintext token.
-- The plaintext token is NEVER stored in the database.
INSERT INTO password_reset_tokens (tenant_id, user_id, token_hash, expires_at)
VALUES (@tenant_id, @user_id, @token_hash, @expires_at)
RETURNING *;

-- name: GetPasswordResetTokenByHash :one
-- Retrieves an unconsumed, non-expired reset token by its hash.
-- Returns pgx.ErrNoRows if the token does not exist, is expired, or is already used.
SELECT * FROM password_reset_tokens
WHERE token_hash = @token_hash
  AND used_at IS NULL
  AND expires_at > now();

-- name: ConsumePasswordResetToken :exec
-- Marks a reset token as consumed by setting used_at.
-- The WHERE clause with used_at IS NULL ensures single-use:
-- under concurrent requests, only the first UPDATE succeeds (affects 1 row),
-- subsequent attempts affect 0 rows, which the application layer detects.
UPDATE password_reset_tokens
SET used_at = now()
WHERE id = @id AND used_at IS NULL;

-- name: InvalidatePasswordResetTokensForUser :exec
-- Invalidates all unconsumed reset tokens for a user.
-- Called when a new reset token is generated (supersedes previous tokens).
UPDATE password_reset_tokens
SET used_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id AND used_at IS NULL;

-- ============================================================
-- email_verification_tokens
-- ============================================================

-- name: CreateEmailVerificationToken :one
-- Creates a new email verification token.
-- The token_hash is the SHA-256 hash of the plaintext token.
INSERT INTO email_verification_tokens (tenant_id, user_id, token_hash, expires_at)
VALUES (@tenant_id, @user_id, @token_hash, @expires_at)
RETURNING *;

-- name: GetEmailVerificationTokenByHash :one
-- Retrieves an unconsumed, non-expired verification token by its hash.
SELECT * FROM email_verification_tokens
WHERE token_hash = @token_hash
  AND used_at IS NULL
  AND expires_at > now();

-- name: ConsumeEmailVerificationToken :exec
-- Marks a verification token as consumed.
-- Single-use enforcement via used_at IS NULL (same pattern as password reset).
UPDATE email_verification_tokens
SET used_at = now()
WHERE id = @id AND used_at IS NULL;

-- name: InvalidateEmailVerificationTokensForUser :exec
-- Invalidates all unconsumed verification tokens for a user.
-- Called when a new verification token is generated (resend flow).
UPDATE email_verification_tokens
SET used_at = now()
WHERE tenant_id = @tenant_id AND user_id = @user_id AND used_at IS NULL;
