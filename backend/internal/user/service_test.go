package user

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/conduit-platform/conduit/backend/internal/platform/errors"
	"github.com/conduit-platform/conduit/backend/internal/platform/tenant"
)

// --- Mock Repository ---

type mockRepository struct {
	createFn           func(ctx context.Context, user *User) (*User, error)
	getByIDFn          func(ctx context.Context, tenantID, userID uuid.UUID) (*User, error)
	getByEmailFn       func(ctx context.Context, tenantID uuid.UUID, email string) (*User, error)
	listFn             func(ctx context.Context, tenantID uuid.UUID, filter ListFilter) ([]User, int64, error)
	updateFn           func(ctx context.Context, user *User) (*User, error)
	updateStatusFn     func(ctx context.Context, tenantID, userID uuid.UUID, status Status, deactivatedAt *time.Time) (*User, error)
	setEmailVerifiedFn func(ctx context.Context, tenantID, userID uuid.UUID) (*User, error)
}

func (m *mockRepository) Create(ctx context.Context, user *User) (*User, error) {
	return m.createFn(ctx, user)
}
func (m *mockRepository) GetByID(ctx context.Context, tenantID, userID uuid.UUID) (*User, error) {
	return m.getByIDFn(ctx, tenantID, userID)
}
func (m *mockRepository) GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*User, error) {
	return m.getByEmailFn(ctx, tenantID, email)
}
func (m *mockRepository) List(ctx context.Context, tenantID uuid.UUID, filter ListFilter) ([]User, int64, error) {
	return m.listFn(ctx, tenantID, filter)
}
func (m *mockRepository) Update(ctx context.Context, user *User) (*User, error) {
	return m.updateFn(ctx, user)
}
func (m *mockRepository) UpdateStatus(ctx context.Context, tenantID, userID uuid.UUID, status Status, deactivatedAt *time.Time) (*User, error) {
	return m.updateStatusFn(ctx, tenantID, userID, status, deactivatedAt)
}
func (m *mockRepository) SetEmailVerified(ctx context.Context, tenantID, userID uuid.UUID) (*User, error) {
	return m.setEmailVerifiedFn(ctx, tenantID, userID)
}

// --- Test helpers ---

func testContext(tenantID uuid.UUID) context.Context {
	ctx := context.Background()
	return tenant.SetID(ctx, tenantID.String())
}

func newTestService(repo Repository) *Service {
	logger := zerolog.Nop()
	return NewService(repo, logger)
}

func activeUser(tenantID uuid.UUID) *User {
	now := time.Now().UTC()
	return &User{
		ID:          uuid.New(),
		TenantID:    tenantID,
		Email:       "test@example.com",
		DisplayName: "Test User",
		Status:      StatusActive,
		Metadata:    map[string]any{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// --- Tests ---

func TestCreateUser_Success(t *testing.T) {
	tenantID := uuid.New()
	ctx := testContext(tenantID)

	repo := &mockRepository{
		createFn: func(_ context.Context, user *User) (*User, error) {
			user.ID = uuid.New()
			user.CreatedAt = time.Now().UTC()
			user.UpdatedAt = user.CreatedAt
			return user, nil
		},
	}

	svc := newTestService(repo)
	user, err := svc.CreateUser(ctx, CreateUserInput{
		Email:       "  Test@Example.COM  ",
		DisplayName: "  Jane Doe  ",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Email != "test@example.com" {
		t.Errorf("email not normalized: got %q, want %q", user.Email, "test@example.com")
	}
	if user.DisplayName != "Jane Doe" {
		t.Errorf("display_name not trimmed: got %q, want %q", user.DisplayName, "Jane Doe")
	}
	if user.Status != StatusActive {
		t.Errorf("status: got %q, want %q", user.Status, StatusActive)
	}
}

func TestCreateUser_DuplicateEmail(t *testing.T) {
	tenantID := uuid.New()
	ctx := testContext(tenantID)

	repo := &mockRepository{
		createFn: func(_ context.Context, _ *User) (*User, error) {
			return nil, ErrEmailAlreadyExists
		},
	}

	svc := newTestService(repo)
	_, err := svc.CreateUser(ctx, CreateUserInput{
		Email:       "test@example.com",
		DisplayName: "Test",
	})

	var domainErr *errors.DomainError
	if !errors.As(err, &domainErr) || domainErr.Code != errors.CodeConflict {
		t.Errorf("expected CONFLICT error, got: %v", err)
	}
}

func TestGetUser_NotFound(t *testing.T) {
	tenantID := uuid.New()
	ctx := testContext(tenantID)

	repo := &mockRepository{
		getByIDFn: func(_ context.Context, _, _ uuid.UUID) (*User, error) {
			return nil, ErrUserNotFound
		},
	}

	svc := newTestService(repo)
	_, err := svc.GetUser(ctx, uuid.New())

	var domainErr *errors.DomainError
	if !errors.As(err, &domainErr) || domainErr.Code != errors.CodeNotFound {
		t.Errorf("expected NOT_FOUND error, got: %v", err)
	}
}

func TestDeactivateUser_Success(t *testing.T) {
	tenantID := uuid.New()
	ctx := testContext(tenantID)
	user := activeUser(tenantID)

	repo := &mockRepository{
		getByIDFn: func(_ context.Context, _, _ uuid.UUID) (*User, error) {
			return user, nil
		},
		updateStatusFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, status Status, deactivatedAt *time.Time) (*User, error) {
			user.Status = status
			user.DeactivatedAt = deactivatedAt
			return user, nil
		},
	}

	svc := newTestService(repo)
	result, err := svc.DeactivateUser(ctx, user.ID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusDeactivated {
		t.Errorf("status: got %q, want %q", result.Status, StatusDeactivated)
	}
	if result.DeactivatedAt == nil {
		t.Error("deactivated_at should be set")
	}
}

func TestDeactivateUser_AlreadyDeactivated(t *testing.T) {
	tenantID := uuid.New()
	ctx := testContext(tenantID)
	user := activeUser(tenantID)
	user.Status = StatusDeactivated

	repo := &mockRepository{
		getByIDFn: func(_ context.Context, _, _ uuid.UUID) (*User, error) {
			return user, nil
		},
	}

	svc := newTestService(repo)
	_, err := svc.DeactivateUser(ctx, user.ID)

	var domainErr *errors.DomainError
	if !errors.As(err, &domainErr) || domainErr.Code != errors.CodeConflict {
		t.Errorf("expected CONFLICT error, got: %v", err)
	}
}

func TestReactivateUser_Success(t *testing.T) {
	tenantID := uuid.New()
	ctx := testContext(tenantID)
	user := activeUser(tenantID)
	user.Status = StatusDeactivated
	now := time.Now().UTC()
	user.DeactivatedAt = &now

	repo := &mockRepository{
		getByIDFn: func(_ context.Context, _, _ uuid.UUID) (*User, error) {
			return user, nil
		},
		updateStatusFn: func(_ context.Context, _ uuid.UUID, _ uuid.UUID, status Status, deactivatedAt *time.Time) (*User, error) {
			user.Status = status
			user.DeactivatedAt = deactivatedAt
			return user, nil
		},
	}

	svc := newTestService(repo)
	result, err := svc.ReactivateUser(ctx, user.ID)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != StatusActive {
		t.Errorf("status: got %q, want %q", result.Status, StatusActive)
	}
	if result.DeactivatedAt != nil {
		t.Error("deactivated_at should be nil after reactivation")
	}
}

func TestReactivateUser_AlreadyActive(t *testing.T) {
	tenantID := uuid.New()
	ctx := testContext(tenantID)
	user := activeUser(tenantID)

	repo := &mockRepository{
		getByIDFn: func(_ context.Context, _, _ uuid.UUID) (*User, error) {
			return user, nil
		},
	}

	svc := newTestService(repo)
	_, err := svc.ReactivateUser(ctx, user.ID)

	var domainErr *errors.DomainError
	if !errors.As(err, &domainErr) || domainErr.Code != errors.CodeConflict {
		t.Errorf("expected CONFLICT error, got: %v", err)
	}
}

func TestStatusTransitions(t *testing.T) {
	tests := []struct {
		name   string
		from   Status
		to     Status
		expect bool
	}{
		{"active to deactivated", StatusActive, StatusDeactivated, true},
		{"active to suspended", StatusActive, StatusSuspended, true},
		{"active to active", StatusActive, StatusActive, false},
		{"deactivated to active", StatusDeactivated, StatusActive, true},
		{"deactivated to suspended", StatusDeactivated, StatusSuspended, false},
		{"deactivated to deactivated", StatusDeactivated, StatusDeactivated, false},
		{"suspended to active", StatusSuspended, StatusActive, true},
		{"suspended to deactivated", StatusSuspended, StatusDeactivated, false},
		{"suspended to suspended", StatusSuspended, StatusSuspended, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Status: tt.from}
			got := user.CanTransitionTo(tt.to)
			if got != tt.expect {
				t.Errorf("CanTransitionTo(%q -> %q) = %v, want %v", tt.from, tt.to, got, tt.expect)
			}
		})
	}
}

func TestEmailNormalization(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"User@Example.COM", "user@example.com"},
		{"  user@example.com  ", "user@example.com"},
		{"USER@EXAMPLE.COM", "user@example.com"},
		{"user@example.com", "user@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeEmail(tt.input)
			if got != tt.want {
				t.Errorf("normalizeEmail(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestListUsers_Defaults(t *testing.T) {
	tenantID := uuid.New()
	ctx := testContext(tenantID)

	var capturedFilter ListFilter
	repo := &mockRepository{
		listFn: func(_ context.Context, _ uuid.UUID, filter ListFilter) ([]User, int64, error) {
			capturedFilter = filter
			return []User{}, 0, nil
		},
	}

	svc := newTestService(repo)
	_, _, err := svc.ListUsers(ctx, ListFilter{})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedFilter.PageSize != 20 {
		t.Errorf("default page_size: got %d, want 20", capturedFilter.PageSize)
	}
	if capturedFilter.Page != 1 {
		t.Errorf("default page: got %d, want 1", capturedFilter.Page)
	}
}

func TestCreateUser_NoTenant(t *testing.T) {
	ctx := context.Background() // No tenant set

	svc := newTestService(&mockRepository{})
	_, err := svc.CreateUser(ctx, CreateUserInput{
		Email:       "test@example.com",
		DisplayName: "Test",
	})

	var domainErr *errors.DomainError
	if !errors.As(err, &domainErr) || domainErr.Code != errors.CodeTenantRequired {
		t.Errorf("expected TENANT_REQUIRED error, got: %v", err)
	}
}

func TestUpdateUser_EmailChangeResetsVerification(t *testing.T) {
	tenantID := uuid.New()
	ctx := testContext(tenantID)

	existingUser := activeUser(tenantID)
	existingUser.EmailVerified = true
	now := time.Now().UTC()
	existingUser.EmailVerifiedAt = &now

	var capturedUser *User
	repo := &mockRepository{
		getByIDFn: func(_ context.Context, _, _ uuid.UUID) (*User, error) {
			return existingUser, nil
		},
		updateFn: func(_ context.Context, user *User) (*User, error) {
			capturedUser = user
			return user, nil
		},
	}

	svc := newTestService(repo)
	newEmail := "newemail@example.com"
	_, err := svc.UpdateUser(ctx, existingUser.ID, UpdateUserInput{
		Email: &newEmail,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedUser.EmailVerified != false {
		t.Error("email_verified should be reset to false on email change")
	}
	if capturedUser.EmailVerifiedAt != nil {
		t.Error("email_verified_at should be nil on email change")
	}
	if capturedUser.Email != "newemail@example.com" {
		t.Errorf("email: got %q, want %q", capturedUser.Email, "newemail@example.com")
	}
}

func TestUpdateUser_SameEmailNoReset(t *testing.T) {
	tenantID := uuid.New()
	ctx := testContext(tenantID)

	existingUser := activeUser(tenantID)
	existingUser.EmailVerified = true
	now := time.Now().UTC()
	existingUser.EmailVerifiedAt = &now

	var capturedUser *User
	repo := &mockRepository{
		getByIDFn: func(_ context.Context, _, _ uuid.UUID) (*User, error) {
			return existingUser, nil
		},
		updateFn: func(_ context.Context, user *User) (*User, error) {
			capturedUser = user
			return user, nil
		},
	}

	svc := newTestService(repo)
	sameEmail := "test@example.com" // same as activeUser default
	_, err := svc.UpdateUser(ctx, existingUser.ID, UpdateUserInput{
		Email: &sameEmail,
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedUser.EmailVerified != true {
		t.Error("email_verified should NOT be reset when email doesn't change")
	}
	if capturedUser.EmailVerifiedAt == nil {
		t.Error("email_verified_at should NOT be cleared when email doesn't change")
	}
}
