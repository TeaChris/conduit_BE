package user

import (
	"github.com/conduit-platform/conduit/backend/internal/platform/errors"
)

// Domain-specific error constructors.
// These use the platform error types and are mapped to HTTP responses
// by the existing httputil.Error() -> errors.ToAPIError() pipeline.
var (
	ErrUserNotFound       = errors.NewNotFound("user not found")
	ErrEmailAlreadyExists = errors.NewConflict("a user with this email already exists")
	ErrInvalidTransition  = errors.NewConflict("invalid status transition")
	ErrAlreadyActive      = errors.NewConflict("user is already active")
)
