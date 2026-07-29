package user

import (
	"time"

	"github.com/google/uuid"
)

// Status represents the lifecycle state of a user.
type Status string

const (
	StatusActive      Status = "active"
	StatusDeactivated Status = "deactivated"
	StatusSuspended   Status = "suspended"
)

// IsValid returns true if the status is a recognized value.
func (s Status) IsValid() bool {
	switch s {
	case StatusActive, StatusDeactivated, StatusSuspended:
		return true
	default:
		return false
	}
}

// User is the core domain entity for user lifecycle management.
type User struct {
	ID              uuid.UUID
	TenantID        uuid.UUID
	Email           string
	DisplayName     string
	Status          Status
	EmailVerified   bool
	EmailVerifiedAt *time.Time
	Metadata        map[string]any
	DeactivatedAt   *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// IsActive returns true if the user is in active status.
func (u *User) IsActive() bool {
	return u.Status == StatusActive
}

// CanTransitionTo checks if a status transition is valid.
func (u *User) CanTransitionTo(target Status) bool {
	switch u.Status {
	case StatusActive:
		return target == StatusDeactivated || target == StatusSuspended
	case StatusDeactivated:
		return target == StatusActive
	case StatusSuspended:
		return target == StatusActive
	default:
		return false
	}
}

// ListFilter defines the parameters for listing users.
type ListFilter struct {
	Status   *Status
	PageSize int
	Page     int
}

// Offset calculates the SQL offset from page and page size.
func (f ListFilter) Offset() int {
	if f.Page < 1 {
		return 0
	}
	return (f.Page - 1) * f.PageSize
}
