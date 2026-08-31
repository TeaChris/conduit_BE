package user

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/conduit-platform/conduit/backend/internal/platform/errors"
	"github.com/conduit-platform/conduit/backend/internal/platform/tenant"
)

var tracer = otel.Tracer("user.service")

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
func (s *Service) CreateUser(ctx context.Context, input CreateUserInput) (result *User, err error) {
	ctx, span := tracer.Start(ctx, "user.service.CreateUser")
	defer span.End()
	start := time.Now()
	defer func() { recordMetrics("create", err, start) }()

	tenantID, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("tenant_id", tenantID.String()))

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

	result, err = s.repo.Create(ctx, user)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetAttributes(attribute.String("user.id", result.ID.String()))
	zerolog.Ctx(ctx).Info().
		Str("user_id", result.ID.String()).
		Str("tenant_id", tenantID.String()).
		Str("email_domain", emailDomain(result.Email)).
		Msg("user created")

	return result, nil
}

// GetUser retrieves a user by ID within the tenant.
func (s *Service) GetUser(ctx context.Context, userID uuid.UUID) (result *User, err error) {
	ctx, span := tracer.Start(ctx, "user.service.GetUser")
	defer span.End()
	start := time.Now()
	defer func() { recordMetrics("get", err, start) }()

	tenantID, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(
		attribute.String("tenant_id", tenantID.String()),
		attribute.String("user.id", userID.String()),
	)

	return s.repo.GetByID(ctx, tenantID, userID)
}

// ListUsers returns a paginated list of users for the tenant.
func (s *Service) ListUsers(ctx context.Context, filter ListFilter) (users []User, total int64, err error) {
	ctx, span := tracer.Start(ctx, "user.service.ListUsers")
	defer span.End()
	start := time.Now()
	defer func() { recordMetrics("list", err, start) }()

	tenantID, err := requireTenantID(ctx)
	if err != nil {
		return nil, 0, err
	}
	span.SetAttributes(attribute.String("tenant_id", tenantID.String()))

	// Apply defaults.
	if filter.PageSize <= 0 || filter.PageSize > 100 {
		filter.PageSize = 20
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}
	span.SetAttributes(attribute.Int("page_size", filter.PageSize))

	return s.repo.List(ctx, tenantID, filter)
}

// UpdateUser updates mutable fields of an existing user.
func (s *Service) UpdateUser(ctx context.Context, userID uuid.UUID, input UpdateUserInput) (result *User, err error) {
	ctx, span := tracer.Start(ctx, "user.service.UpdateUser")
	defer span.End()
	start := time.Now()
	defer func() { recordMetrics("update", err, start) }()

	tenantID, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(
		attribute.String("tenant_id", tenantID.String()),
		attribute.String("user.id", userID.String()),
	)

	// Fetch the existing user to apply partial updates.
	existing, err := s.repo.GetByID(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	// Apply updates.
	if input.Email != nil {
		newEmail := normalizeEmail(*input.Email)
		// Reset email verification when the email actually changes.
		if newEmail != existing.Email {
			existing.Email = newEmail
			existing.EmailVerified = false
			existing.EmailVerifiedAt = nil
		}
	}
	if input.DisplayName != nil {
		existing.DisplayName = strings.TrimSpace(*input.DisplayName)
	}
	if input.Metadata != nil {
		existing.Metadata = input.Metadata
	}

	result, err = s.repo.Update(ctx, existing)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", userID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("user updated")

	return result, nil
}

// DeactivateUser transitions a user from active to deactivated.
func (s *Service) DeactivateUser(ctx context.Context, userID uuid.UUID) (result *User, err error) {
	ctx, span := tracer.Start(ctx, "user.service.DeactivateUser")
	defer span.End()
	start := time.Now()
	defer func() { recordMetrics("deactivate", err, start) }()

	tenantID, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(
		attribute.String("tenant_id", tenantID.String()),
		attribute.String("user.id", userID.String()),
	)

	existing, err := s.repo.GetByID(ctx, tenantID, userID)
	if err != nil {
		return nil, err
	}

	if !existing.CanTransitionTo(StatusDeactivated) {
		return nil, ErrInvalidTransition
	}

	now := time.Now().UTC()
	result, err = s.repo.UpdateStatus(ctx, tenantID, userID, StatusDeactivated, &now)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", userID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("user deactivated")

	return result, nil
}

// ReactivateUser transitions a user from deactivated or suspended back to active.
func (s *Service) ReactivateUser(ctx context.Context, userID uuid.UUID) (result *User, err error) {
	ctx, span := tracer.Start(ctx, "user.service.ReactivateUser")
	defer span.End()
	start := time.Now()
	defer func() { recordMetrics("reactivate", err, start) }()

	tenantID, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(
		attribute.String("tenant_id", tenantID.String()),
		attribute.String("user.id", userID.String()),
	)

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

	result, err = s.repo.UpdateStatus(ctx, tenantID, userID, StatusActive, nil)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", userID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("user reactivated")

	return result, nil
}

// SetEmailVerified marks a user's email as verified.
// This is an internal service method for use by the Auth domain (RFC-0001 FR-7).
// It is NOT exposed as an HTTP endpoint.
func (s *Service) SetEmailVerified(ctx context.Context, userID uuid.UUID) (result *User, err error) {
	ctx, span := tracer.Start(ctx, "user.service.SetEmailVerified")
	defer span.End()
	start := time.Now()
	defer func() { recordMetrics("set_email_verified", err, start) }()

	tenantID, err := requireTenantID(ctx)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(
		attribute.String("tenant_id", tenantID.String()),
		attribute.String("user.id", userID.String()),
	)

	result, err = s.repo.SetEmailVerified(ctx, tenantID, userID)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", userID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("user email verified")

	return result, nil
}

// --- Helpers ---

// requireTenantID extracts the tenant ID from context and returns a parsed UUID.
func requireTenantID(ctx context.Context) (uuid.UUID, error) {
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

// recordMetrics records operation metrics for the user domain.
func recordMetrics(operation string, err error, start time.Time) {
	status := "success"
	if err != nil {
		status = classifyError(err)
	}
	operationsTotal.WithLabelValues(operation, status).Inc()
	operationDuration.WithLabelValues(operation).Observe(time.Since(start).Seconds())
}

// classifyError maps domain errors to metric status labels.
func classifyError(err error) string {
	var domainErr *errors.DomainError
	if errors.As(err, &domainErr) {
		switch domainErr.Code {
		case errors.CodeNotFound:
			return "not_found"
		case errors.CodeConflict:
			return "conflict"
		default:
			return "error"
		}
	}
	return "error"
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
