package auth

import (
	"time"

	"github.com/google/uuid"
)

// Credential represents a user's password credential.
// The PasswordHash field contains an Argon2id PHC string.
// The plaintext password is never stored.
type Credential struct {
	ID           uuid.UUID
	TenantID     uuid.UUID
	UserID       uuid.UUID
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Session represents an authentication session.
// Each successful login creates a new session.
// RefreshTokenHash is the SHA-256 hash of the current valid refresh token.
// FamilyID groups all tokens from a single login event for reuse detection.
type Session struct {
	ID               uuid.UUID
	TenantID         uuid.UUID
	UserID           uuid.UUID
	FamilyID         uuid.UUID
	RefreshTokenHash string
	ExpiresAt        time.Time
	RevokedAt        *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	LastUsedAt       time.Time
	IPAddress        *string
	UserAgent        *string
}

// IsRevoked returns true if the session has been explicitly revoked.
func (s *Session) IsRevoked() bool {
	return s.RevokedAt != nil
}

// IsExpired returns true if the session has passed its expiration time.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsActive returns true if the session is neither revoked nor expired.
func (s *Session) IsActive() bool {
	return !s.IsRevoked() && !s.IsExpired()
}

// PasswordResetToken represents a hashed password reset token.
// The plaintext token is never stored.
type PasswordResetToken struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// IsUsed returns true if the token has been consumed.
func (t *PasswordResetToken) IsUsed() bool {
	return t.UsedAt != nil
}

// IsExpired returns true if the token has passed its expiration time.
func (t *PasswordResetToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}

// EmailVerificationToken represents a hashed email verification token.
// The plaintext token is never stored.
type EmailVerificationToken struct {
	ID        uuid.UUID
	TenantID  uuid.UUID
	UserID    uuid.UUID
	TokenHash string
	ExpiresAt time.Time
	UsedAt    *time.Time
	CreatedAt time.Time
}

// IsUsed returns true if the token has been consumed.
func (t *EmailVerificationToken) IsUsed() bool {
	return t.UsedAt != nil
}

// IsExpired returns true if the token has passed its expiration time.
func (t *EmailVerificationToken) IsExpired() bool {
	return time.Now().After(t.ExpiresAt)
}
