package user

import (
	"math"
	"time"

	"github.com/google/uuid"
)

// --- Request DTOs ---

// CreateUserRequest is the JSON body for POST /api/v1/users.
type CreateUserRequest struct {
	Email       string         `json:"email"`
	DisplayName string         `json:"display_name"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// UpdateUserRequest is the JSON body for PATCH /api/v1/users/:id.
type UpdateUserRequest struct {
	Email       *string        `json:"email,omitempty"`
	DisplayName *string        `json:"display_name,omitempty"`
	Metadata    map[string]any `json:"metadata,omitempty"`
}

// --- Response DTOs ---

// UserResponse is the JSON representation of a user sent to API clients.
type UserResponse struct {
	ID              uuid.UUID      `json:"id"`
	TenantID        uuid.UUID      `json:"tenant_id"`
	Email           string         `json:"email"`
	DisplayName     string         `json:"display_name"`
	Status          string         `json:"status"`
	EmailVerified   bool           `json:"email_verified"`
	EmailVerifiedAt *time.Time     `json:"email_verified_at,omitempty"`
	Metadata        map[string]any `json:"metadata"`
	DeactivatedAt   *time.Time     `json:"deactivated_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// UserListResponse wraps a list of users with pagination metadata.
type UserListResponse struct {
	Users      []UserResponse     `json:"users"`
	Pagination PaginationResponse `json:"pagination"`
}

// PaginationResponse provides pagination metadata.
type PaginationResponse struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// --- Mappers ---

// toResponse converts a domain User to a UserResponse DTO.
func toResponse(u *User) UserResponse {
	return UserResponse{
		ID:              u.ID,
		TenantID:        u.TenantID,
		Email:           u.Email,
		DisplayName:     u.DisplayName,
		Status:          string(u.Status),
		EmailVerified:   u.EmailVerified,
		EmailVerifiedAt: u.EmailVerifiedAt,
		Metadata:        u.Metadata,
		DeactivatedAt:   u.DeactivatedAt,
		CreatedAt:       u.CreatedAt,
		UpdatedAt:       u.UpdatedAt,
	}
}

// toResponseList converts a slice of domain Users to UserResponses.
func toResponseList(users []User) []UserResponse {
	result := make([]UserResponse, len(users))
	for i := range users {
		result[i] = toResponse(&users[i])
	}
	return result
}

// newPagination creates a PaginationResponse from the given parameters.
func newPagination(page, perPage int, total int64) PaginationResponse {
	totalPages := 0
	if perPage > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(perPage)))
	}
	return PaginationResponse{
		Page:       page,
		PerPage:    perPage,
		Total:      total,
		TotalPages: totalPages,
	}
}
