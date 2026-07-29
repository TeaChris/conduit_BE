package user

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/conduit-platform/conduit/backend/internal/platform/errors"
	"github.com/conduit-platform/conduit/backend/internal/platform/tenant"
)

// Service implements the business logic for user lifecycle management.
type Service struct {
	repo   Repository
	logger zerolog.Logger
}

// NewService creates a new user Service.
func NewService(repo Repository, logger zerolog.Logger) *Service {
	return &Service{
		repo:   repo,
		logger: logger,
	}
}

// CreateUser creates a new user within the tenant.
func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) (*User, error) {
	tenantID, err := s.requireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	user := &User{
		TenantID:    tenantID,
		Email:       normalizeEmail(input.Email),
		DisplayName: strings.TrimSpace(input.DisplayName),
		Status:      StatusActive,
		Metadata:    input.Metadata,
	}
	if user.Metadata == nil {
		user.Metadata = make(map[string]any)
	}

	created, err := s.repo.Create(ctx, user)
	if err != nil {
		return nil, err
	}

	s.logger.Info().
		Str("user_id", created.ID.String()).
		Str("tenant_id", tenantID.String()).
		Str("email_domain", emailDomain(created.Email)).
		Msg("user created")

	return created, nil
}

// GetUser retrieves a user by ID within the tenant.
func (s *Service) GetUser(ctx context.Context, userID uuid.UUID) (*User, error) {
	tenantID, err := s.requireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	return s.repo.GetByID(ctx, tenantID, userID)
}

// ListUsers returns a paginated list of users for the tenant.
func (s *Service) ListUsers(ctx context.Context, filter ListFilter) ([]User, int64, error) {
	tenantID, err := s.requireTenantID(ctx)
	if err != nil {
		return nil, 0, err
	}

	// Apply defaults.
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}

	return s.repo.List(ctx, tenantID, filter)
}

// UpdateUser updates mutable fields of an existing user.
func (s *Service) UpdateUser(ctx context.Context, userID uuid.UUID, input UpdateUserInput) (*User, error) {
	tenantID, err := s.requireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	// Fetch the existing user to apply partial updates.
	existing, err := s.repo.GetByID(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	// Apply updates.
	if input.Email != nil {
		existing.Email = normalizeEmail(*input.Email)
	}
	if input.DisplayName != nil {
		existing.DisplayName = strings.TrimSpace(*input.DisplayName)
	}
	if input.Metadata != nil {
		existing.Metadata = input.Metadata
	}

	updated, err := s.repo.Update(ctx, existing)
	if err != nil {
		return nil, err
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("user updated")

	return updated, nil
}

// DeactivateUser transitions a user from active to deactivated.
func (s *Service) DeactivateUser(ctx context.Context, userID uuid.UUID) (*User, error) {
	tenantID, err := s.requireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByID(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	if !existing.CanTransitionTo(StatusDeactivated) {
		return nil, ErrInvalidTransition
	}

	now := time.Now().UTC()
	updated, err := s.repo.UpdateStatus(ctx, tenantID, userID, StatusDeactivated, &now)
	if err != nil {
		return nil, err
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("user deactivated")

	return updated, nil
}

// ReactivateUser transitions a user from deactivated or suspended back to active.
func (s *Service) ReactivateUser(ctx context.Context, userID uuid.UUID) (*User, error) {
	tenantID, err := s.requireTenantID(ctx)
	if err != nil {
		return nil, err
	}

	existing, err := s.repo.GetByID(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	if existing.Status == StatusActive {
		return nil, ErrAlreadyActive
	}

	if !existing.CanTransitionTo(StatusActive) {
		return nil, ErrInvalidTransition
	}

	updated, err := s.repo.UpdateStatus(ctx, tenantID, userID, StatusActive, nil)
	if err != nil {
		return nil, err
	}

	s.logger.Info().
		Str("user_id", userID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("user reactivated")

	return updated, nil
}

// --- Helpers ---

// requireTenantID extracts the tenant ID from context and returns a parsed UUID.
func (s *Service) requireTenantID(ctx context.Context) (uuid.UUID, error) {
	tenantIDStr, ok := tenant.GetID(ctx)
	if !ok {
		return uuid.Nil, errors.NewDomainError(errors.CodeTenantRequired, "tenant ID is required")
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return uuid.Nil, errors.NewBadRequest("invalid tenant ID format")
	}

	return tenantID, nil
}

// normalizeEmail lowercases and trims an email address.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// emailDomain extracts the domain part of an email for safe logging.
func emailDomain(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return ""
}

// --- Input types ---

// CreateUserInput holds the validated input for creating a user.
type CreateUserInput struct {
	Email       string
	DisplayName string
	Metadata    map[string]any
}

// UpdateUserInput holds the validated input for updating a user.
// All fields are optional (pointer types).
type UpdateUserInput struct {
	Email       *string
	DisplayName *string
	Metadata    map[string]any
}
