package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/conduit-platform/conduit/backend/internal/auth"
	"github.com/conduit-platform/conduit/backend/internal/user"
)

// userProviderAdapter adapts user.Service to satisfy auth.UserProvider.
// It maps user.User → auth.UserInfo and bridges the interface gap between
// the Auth and User domains without creating a direct dependency.
//
// This adapter lives in the wiring layer (app package) per architecture-principles §8:
// "Share via interfaces… app.go wires them together."
type userProviderAdapter struct {
	svc *user.Service
}

// NewUserProviderAdapter creates a new adapter wrapping user.Service.
func NewUserProviderAdapter(svc *user.Service) auth.UserProvider {
	return &userProviderAdapter{svc: svc}
}

func (a *userProviderAdapter) CreateUser(ctx context.Context, email, displayName string) (*auth.UserInfo, error) {
	u, err := a.svc.CreateUser(ctx, user.CreateUserInput{
		Email:       email,
		DisplayName: displayName,
	})
	if err != nil {
		return nil, err
	}
	return toUserInfo(u), nil
}

func (a *userProviderAdapter) GetUserByEmail(ctx context.Context, email string) (*auth.UserInfo, error) {
	u, err := a.svc.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, err
	}
	return toUserInfo(u), nil
}

func (a *userProviderAdapter) GetUser(ctx context.Context, userID uuid.UUID) (*auth.UserInfo, error) {
	u, err := a.svc.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return toUserInfo(u), nil
}

func (a *userProviderAdapter) SetEmailVerified(ctx context.Context, userID uuid.UUID) error {
	_, err := a.svc.SetEmailVerified(ctx, userID)
	return err
}

// toUserInfo maps a user.User to the auth.UserInfo subset.
func toUserInfo(u *user.User) *auth.UserInfo {
	return &auth.UserInfo{
		ID:            u.ID,
		TenantID:      u.TenantID,
		Email:         u.Email,
		Status:        string(u.Status),
		EmailVerified: u.EmailVerified,
	}
}
