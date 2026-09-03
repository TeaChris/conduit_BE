package auth

import (
	"github.com/conduit-platform/conduit/backend/internal/platform/errors"
)

// --- Persistence errors ---
// These are returned by the Repository implementation.
var (
	ErrCredentialNotFound           = errors.NewNotFound("credential not found")
	ErrCredentialAlreadyExists      = errors.NewConflict("credential already exists for this user")
	ErrSessionNotFound              = errors.NewNotFound("session not found")
	ErrSessionRevoked               = errors.NewUnauthorized("session has been revoked")
	ErrSessionExpired               = errors.NewUnauthorized("session has expired")
	ErrResetTokenNotFound           = errors.NewBadRequest("invalid or expired reset token")
	ErrResetTokenAlreadyUsed        = errors.NewBadRequest("reset token has already been used")
	ErrVerificationTokenNotFound    = errors.NewBadRequest("invalid or expired verification token")
	ErrVerificationTokenAlreadyUsed = errors.NewBadRequest("verification token has already been used")
)

// --- Application/service errors ---
// These are returned by the Service layer for authentication use cases.
var (
	// ErrInvalidCredentials is returned for unknown email, wrong password, or
	// inactive account. The message is intentionally generic to prevent
	// account enumeration (RFC-0002 §16).
	ErrInvalidCredentials = errors.NewUnauthorized("invalid email or password")

	// ErrEmailNotVerified is safe to distinguish per RFC-0002 §16 because the
	// user created the account themselves.
	ErrEmailNotVerified = errors.NewDomainError(errors.CodeForbidden, "email not verified")

	// ErrEmailAlreadyRegistered is returned when registration is attempted
	// with an email that already exists.
	ErrEmailAlreadyRegistered = errors.NewConflict("email already registered")

	// ErrRefreshTokenReuse is returned when a previously-rotated refresh token
	// is presented, indicating potential token theft (RFC-0002 §14).
	ErrRefreshTokenReuse = errors.NewUnauthorized("session compromised: please log in again")

	// ErrRefreshTokenInvalid is returned for malformed or unrecognized refresh tokens.
	ErrRefreshTokenInvalid = errors.NewUnauthorized("invalid refresh token")

	// ErrResetTokenExpired is returned when a password reset token has expired.
	ErrResetTokenExpired = errors.NewBadRequest("reset token has expired")

	// ErrVerificationTokenExpired is returned when an email verification token has expired.
	ErrVerificationTokenExpired = errors.NewBadRequest("verification token has expired")
)
