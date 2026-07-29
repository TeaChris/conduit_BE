package errors

import (
	"errors"
	"net/http"
	"testing"
)

func TestDomainError_Error(t *testing.T) {
	err := NewNotFound("user not found")
	expected := "NOT_FOUND: user not found"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestValidationError_Error(t *testing.T) {
	err := NewValidationError("invalid input", FieldError{Field: "email", Message: "must be valid"})
	expected := "VALIDATION_ERROR: invalid input"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

func TestInfraError_Unwrap(t *testing.T) {
	inner := errors.New("db down")
	err := NewInfraError("db.Ping", inner)
	if !errors.Is(err, inner) {
		t.Error("expected errors.Is to match inner error")
	}
	if err.Unwrap() != inner {
		t.Error("expected Unwrap to return inner error")
	}
}

func TestToAPIError_DomainError(t *testing.T) {
	tests := []struct {
		err            *DomainError
		expectedStatus int
	}{
		{NewNotFound("not found"), http.StatusNotFound},
		{NewConflict("conflict"), http.StatusConflict},
		{NewBadRequest("bad request"), http.StatusBadRequest},
		{NewUnauthorized("unauthorized"), http.StatusUnauthorized},
		{NewForbidden("forbidden"), http.StatusForbidden},
		{NewDomainError("UNKNOWN", "unknown"), http.StatusInternalServerError},
	}

	for _, tc := range tests {
		status, apiErr := ToAPIError(tc.err)
		if status != tc.expectedStatus {
			t.Errorf("expected status %d, got %d", tc.expectedStatus, status)
		}
		if apiErr.Code != string(tc.err.Code) {
			t.Errorf("expected code %q, got %q", tc.err.Code, apiErr.Code)
		}
	}
}

func TestToAPIError_ValidationError(t *testing.T) {
	err := NewValidationError("invalid", FieldError{Field: "x", Message: "y"})
	status, apiErr := ToAPIError(err)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("expected status 422, got %d", status)
	}
	if len(apiErr.Details) != 1 || apiErr.Details[0].Field != "x" {
		t.Errorf("expected 1 detail with field 'x', got %+v", apiErr.Details)
	}
}

func TestToAPIError_InfraError(t *testing.T) {
	inner := errors.New("connection reset by peer")
	err := NewInfraError("db.Query", inner)
	status, apiErr := ToAPIError(err)
	if status != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", status)
	}
	if apiErr.Code != "INTERNAL_ERROR" {
		t.Errorf("expected code INTERNAL_ERROR, got %q", apiErr.Code)
	}
}

func TestToAPIError_UnknownError(t *testing.T) {
	err := errors.New("some random error")
	status, apiErr := ToAPIError(err)
	if status != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", status)
	}
	if apiErr.Code != "INTERNAL_ERROR" {
		t.Errorf("expected code INTERNAL_ERROR, got %q", apiErr.Code)
	}
}
