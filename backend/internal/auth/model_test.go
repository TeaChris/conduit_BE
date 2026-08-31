package auth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

// --- Session tests ---

func TestSession_IsRevoked(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		revokedAt *time.Time
		want      bool
	}{
		{"not revoked", nil, false},
		{"revoked", &now, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{RevokedAt: tt.revokedAt}
			if got := s.IsRevoked(); got != tt.want {
				t.Errorf("IsRevoked() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSession_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"not expired", time.Now().Add(time.Hour), false},
		{"expired", time.Now().Add(-time.Hour), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{ExpiresAt: tt.expiresAt}
			if got := s.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSession_IsActive(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		expiresAt time.Time
		revokedAt *time.Time
		want      bool
	}{
		{"active", time.Now().Add(time.Hour), nil, true},
		{"expired", time.Now().Add(-time.Hour), nil, false},
		{"revoked", time.Now().Add(time.Hour), &now, false},
		{"expired and revoked", time.Now().Add(-time.Hour), &now, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Session{ExpiresAt: tt.expiresAt, RevokedAt: tt.revokedAt}
			if got := s.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- PasswordResetToken tests ---

func TestPasswordResetToken_IsUsed(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		usedAt *time.Time
		want   bool
	}{
		{"not used", nil, false},
		{"used", &now, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := &PasswordResetToken{UsedAt: tt.usedAt}
			if got := tok.IsUsed(); got != tt.want {
				t.Errorf("IsUsed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPasswordResetToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"not expired", time.Now().Add(time.Hour), false},
		{"expired", time.Now().Add(-time.Hour), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := &PasswordResetToken{ExpiresAt: tt.expiresAt}
			if got := tok.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- EmailVerificationToken tests ---

func TestEmailVerificationToken_IsUsed(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name   string
		usedAt *time.Time
		want   bool
	}{
		{"not used", nil, false},
		{"used", &now, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := &EmailVerificationToken{UsedAt: tt.usedAt}
			if got := tok.IsUsed(); got != tt.want {
				t.Errorf("IsUsed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEmailVerificationToken_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{"not expired", time.Now().Add(time.Hour), false},
		{"expired", time.Now().Add(-time.Hour), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tok := &EmailVerificationToken{ExpiresAt: tt.expiresAt}
			if got := tok.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Model struct construction tests ---

func TestCredential_Fields(t *testing.T) {
	id := uuid.New()
	tenantID := uuid.New()
	userID := uuid.New()
	now := time.Now().UTC()

	c := Credential{
		ID:           id,
		TenantID:     tenantID,
		UserID:       userID,
		PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$salt$hash",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if c.ID != id {
		t.Errorf("ID = %v, want %v", c.ID, id)
	}
	if c.TenantID != tenantID {
		t.Errorf("TenantID = %v, want %v", c.TenantID, tenantID)
	}
	if c.UserID != userID {
		t.Errorf("UserID = %v, want %v", c.UserID, userID)
	}
}
