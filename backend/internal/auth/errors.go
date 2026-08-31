package auth

import (
	"github.com/conduit-platform/conduit/backend/internal/platform/errors"
)

// Domain-specific errors for the authentication module.
// These use the platform error types and are mapped to HTTP responses
// by the existing httputil.Error() → errors.ToAPIError() pipeline.
var (
	ErrCredentialNotFound         = errors.NewNotFound("credential not found")
	ErrCredentialAlreadyExists    = errors.NewConflict("credential already exists for this user")
	ErrSessionNotFound            = errors.NewNotFound("session not found")
	ErrSessionRevoked             = errors.NewUnauthorized("session has been revoked")
	ErrSessionExpired             = errors.NewUnauthorized("session has expired")
	ErrResetTokenNotFound         = errors.NewBadRequest("invalid or expired reset token")
	ErrResetTokenAlreadyUsed      = errors.NewBadRequest("reset token has already been used")
	ErrVerificationTokenNotFound  = errors.NewBadRequest("invalid or expired verification token")
	ErrVerificationTokenAlreadyUsed = errors.NewBadRequest("verification token has already been used")
)
