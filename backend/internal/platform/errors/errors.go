package errors

import (
	"errors"
	"fmt"
	"net/http"
)

// Standard library re-exports for convenience.
var (
	As   = errors.As
	Is   = errors.Is
	New  = errors.New
	Wrap = fmt.Errorf
)

// ErrorCode represents a machine-readable error code.
type ErrorCode string

const (
	CodeNotFound       ErrorCode = "NOT_FOUND"
	CodeValidation     ErrorCode = "VALIDATION_ERROR"
	CodeConflict       ErrorCode = "CONFLICT"
	CodeInternal       ErrorCode = "INTERNAL_ERROR"
	CodeUnauthorized   ErrorCode = "UNAUTHORIZED"
	CodeForbidden      ErrorCode = "FORBIDDEN"
	CodeRateLimited    ErrorCode = "RATE_LIMITED"
	CodeBadRequest     ErrorCode = "BAD_REQUEST"
	CodeTenantRequired ErrorCode = "TENANT_REQUIRED"
)

// DomainError represents a business logic error.
type DomainError struct {
	Code    ErrorCode
	Message string
}

func (e *DomainError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// FieldError represents a validation error on a specific field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationError represents input validation failures.
type ValidationError struct {
	Code    ErrorCode
	Message string
	Fields  []FieldError
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// InfraError represents an infrastructure-level failure (DB, cache, network).
// These are never exposed to clients.
type InfraError struct {
	Op  string // Operation that failed, e.g. "database.GetUser"
	Err error  // Underlying error
}

func (e *InfraError) Error() string {
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

func (e *InfraError) Unwrap() error {
	return e.Err
}

// APIError is the error response sent to API clients.
type APIError struct {
	Code    string       `json:"code"`
	Message string       `json:"message"`
	Details []FieldError `json:"details,omitempty"`
}

// --- Constructors ---

func NewDomainError(code ErrorCode, message string) *DomainError {
	return &DomainError{Code: code, Message: message}
}

func NewNotFound(message string) *DomainError {
	return &DomainError{Code: CodeNotFound, Message: message}
}

func NewConflict(message string) *DomainError {
	return &DomainError{Code: CodeConflict, Message: message}
}

func NewBadRequest(message string) *DomainError {
	return &DomainError{Code: CodeBadRequest, Message: message}
}

func NewUnauthorized(message string) *DomainError {
	return &DomainError{Code: CodeUnauthorized, Message: message}
}

func NewForbidden(message string) *DomainError {
	return &DomainError{Code: CodeForbidden, Message: message}
}

func NewValidationError(message string, fields ...FieldError) *ValidationError {
	return &ValidationError{
		Code:    CodeValidation,
		Message: message,
		Fields:  fields,
	}
}

func NewInfraError(op string, err error) *InfraError {
	return &InfraError{Op: op, Err: err}
}

// --- API Error Mapping ---

// codeToStatus maps error codes to HTTP status codes.
var codeToStatus = map[ErrorCode]int{
	CodeNotFound:       http.StatusNotFound,
	CodeValidation:     http.StatusUnprocessableEntity,
	CodeConflict:       http.StatusConflict,
	CodeInternal:       http.StatusInternalServerError,
	CodeUnauthorized:   http.StatusUnauthorized,
	CodeForbidden:      http.StatusForbidden,
	CodeRateLimited:    http.StatusTooManyRequests,
	CodeBadRequest:     http.StatusBadRequest,
	CodeTenantRequired: http.StatusBadRequest,
}

// ToAPIError converts an internal error to an HTTP status code and APIError.
// Infrastructure errors are masked — clients see a generic 500.
func ToAPIError(err error) (int, *APIError) {
	var domainErr *DomainError
	if errors.As(err, &domainErr) {
		status := codeToStatus[domainErr.Code]
		if status == 0 {
			status = http.StatusInternalServerError
		}
		return status, &APIError{
			Code:    string(domainErr.Code),
			Message: domainErr.Message,
		}
	}

	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return http.StatusUnprocessableEntity, &APIError{
			Code:    string(validationErr.Code),
			Message: validationErr.Message,
			Details: validationErr.Fields,
		}
	}

	// InfraError and unknown errors → generic 500.
	return http.StatusInternalServerError, &APIError{
		Code:    string(CodeInternal),
		Message: "An internal error occurred. Please try again later.",
	}
}
