package auth

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	platformerrors "github.com/conduit-platform/conduit/backend/internal/platform/errors"
	"github.com/conduit-platform/conduit/backend/internal/platform/tenant"
)

// ---------------------------------------------------------------------------
// Mock implementations (functional struct pattern per testing-standards §4)
// ---------------------------------------------------------------------------

type mockUserProvider struct {
	createUserFn       func(ctx context.Context, email, displayName string) (*UserInfo, error)
	getUserByEmailFn   func(ctx context.Context, email string) (*UserInfo, error)
	getUserFn          func(ctx context.Context, userID uuid.UUID) (*UserInfo, error)
	setEmailVerifiedFn func(ctx context.Context, userID uuid.UUID) error
}

func (m *mockUserProvider) CreateUser(ctx context.Context, email, displayName string) (*UserInfo, error) {
	if m.createUserFn != nil {
		return m.createUserFn(ctx, email, displayName)
	}
	return nil, fmt.Errorf("mockUserProvider.CreateUser not implemented")
}

func (m *mockUserProvider) GetUserByEmail(ctx context.Context, email string) (*UserInfo, error) {
	if m.getUserByEmailFn != nil {
		return m.getUserByEmailFn(ctx, email)
	}
	return nil, fmt.Errorf("mockUserProvider.GetUserByEmail not implemented")
}

func (m *mockUserProvider) GetUser(ctx context.Context, userID uuid.UUID) (*UserInfo, error) {
	if m.getUserFn != nil {
		return m.getUserFn(ctx, userID)
	}
	return nil, fmt.Errorf("mockUserProvider.GetUser not implemented")
}

func (m *mockUserProvider) SetEmailVerified(ctx context.Context, userID uuid.UUID) error {
	if m.setEmailVerifiedFn != nil {
		return m.setEmailVerifiedFn(ctx, userID)
	}
	return fmt.Errorf("mockUserProvider.SetEmailVerified not implemented")
}

type mockPasswordHasher struct {
	hashFn   func(password string) (string, error)
	verifyFn func(password, hash string) (bool, error)
}

func (m *mockPasswordHasher) Hash(password string) (string, error) {
	if m.hashFn != nil {
		return m.hashFn(password)
	}
	return "$argon2id$mock$" + password, nil
}

func (m *mockPasswordHasher) Verify(password, hash string) (bool, error) {
	if m.verifyFn != nil {
		return m.verifyFn(password, hash)
	}
	return hash == "$argon2id$mock$"+password, nil
}

type mockTokenIssuer struct {
	issueFn func(claims TokenClaims) (string, error)
}

func (m *mockTokenIssuer) Issue(claims TokenClaims) (string, error) {
	if m.issueFn != nil {
		return m.issueFn(claims)
	}
	return "mock-jwt-" + claims.SessionID.String(), nil
}

type mockTokenGenerator struct {
	generateFn func() (string, error)
	hashFn     func(token string) string
}

func (m *mockTokenGenerator) Generate() (string, error) {
	if m.generateFn != nil {
		return m.generateFn()
	}
	return "mock-random-token", nil
}

func (m *mockTokenGenerator) Hash(token string) string {
	if m.hashFn != nil {
		return m.hashFn(token)
	}
	return "hash:" + token
}

type mockRepository struct {
	createCredentialFn                      func(ctx context.Context, tenantID, userID uuid.UUID, passwordHash string) (*Credential, error)
	getCredentialByUserIDFn                 func(ctx context.Context, tenantID, userID uuid.UUID) (*Credential, error)
	updatePasswordHashFn                    func(ctx context.Context, tenantID, userID uuid.UUID, passwordHash string) (*Credential, error)
	createSessionFn                         func(ctx context.Context, session *Session) (*Session, error)
	getSessionByIDFn                        func(ctx context.Context, tenantID, sessionID uuid.UUID) (*Session, error)
	getActiveSessionByFamilyIDForUpdateFn   func(ctx context.Context, familyID uuid.UUID) (*Session, error)
	rotateRefreshTokenFn                    func(ctx context.Context, tenantID, sessionID uuid.UUID, newTokenHash string) (*Session, error)
	revokeSessionFn                         func(ctx context.Context, tenantID, sessionID uuid.UUID) error
	revokeAllUserSessionsFn                 func(ctx context.Context, tenantID, userID uuid.UUID) error
	revokeOtherUserSessionsFn               func(ctx context.Context, tenantID, userID, currentSessionID uuid.UUID) error
	createPasswordResetTokenFn              func(ctx context.Context, tenantID, userID uuid.UUID, tokenHash string, expiresAt time.Time) (*PasswordResetToken, error)
	getPasswordResetTokenByHashFn           func(ctx context.Context, tokenHash string) (*PasswordResetToken, error)
	consumePasswordResetTokenFn             func(ctx context.Context, tokenID uuid.UUID) error
	invalidatePasswordResetTokensForUserFn  func(ctx context.Context, tenantID, userID uuid.UUID) error
	createEmailVerificationTokenFn              func(ctx context.Context, tenantID, userID uuid.UUID, tokenHash string, expiresAt time.Time) (*EmailVerificationToken, error)
	getEmailVerificationTokenByHashFn           func(ctx context.Context, tokenHash string) (*EmailVerificationToken, error)
	consumeEmailVerificationTokenFn             func(ctx context.Context, tokenID uuid.UUID) error
	invalidateEmailVerificationTokensForUserFn  func(ctx context.Context, tenantID, userID uuid.UUID) error
}

func (m *mockRepository) CreateCredential(ctx context.Context, tenantID, userID uuid.UUID, passwordHash string) (*Credential, error) {
	if m.createCredentialFn != nil {
		return m.createCredentialFn(ctx, tenantID, userID, passwordHash)
	}
	return &Credential{ID: uuid.New(), TenantID: tenantID, UserID: userID, PasswordHash: passwordHash}, nil
}

func (m *mockRepository) GetCredentialByUserID(ctx context.Context, tenantID, userID uuid.UUID) (*Credential, error) {
	if m.getCredentialByUserIDFn != nil {
		return m.getCredentialByUserIDFn(ctx, tenantID, userID)
	}
	return nil, ErrCredentialNotFound
}

func (m *mockRepository) UpdatePasswordHash(ctx context.Context, tenantID, userID uuid.UUID, passwordHash string) (*Credential, error) {
	if m.updatePasswordHashFn != nil {
		return m.updatePasswordHashFn(ctx, tenantID, userID, passwordHash)
	}
	return &Credential{ID: uuid.New(), TenantID: tenantID, UserID: userID, PasswordHash: passwordHash}, nil
}

func (m *mockRepository) CreateSession(ctx context.Context, session *Session) (*Session, error) {
	if m.createSessionFn != nil {
		return m.createSessionFn(ctx, session)
	}
	session.ID = uuid.New()
	session.CreatedAt = time.Now().UTC()
	session.UpdatedAt = session.CreatedAt
	session.LastUsedAt = session.CreatedAt
	return session, nil
}

func (m *mockRepository) GetSessionByID(ctx context.Context, tenantID, sessionID uuid.UUID) (*Session, error) {
	if m.getSessionByIDFn != nil {
		return m.getSessionByIDFn(ctx, tenantID, sessionID)
	}
	return nil, ErrSessionNotFound
}

func (m *mockRepository) GetActiveSessionByFamilyIDForUpdate(ctx context.Context, familyID uuid.UUID) (*Session, error) {
	if m.getActiveSessionByFamilyIDForUpdateFn != nil {
		return m.getActiveSessionByFamilyIDForUpdateFn(ctx, familyID)
	}
	return nil, ErrSessionNotFound
}

func (m *mockRepository) RotateRefreshToken(ctx context.Context, tenantID, sessionID uuid.UUID, newTokenHash string) (*Session, error) {
	if m.rotateRefreshTokenFn != nil {
		return m.rotateRefreshTokenFn(ctx, tenantID, sessionID, newTokenHash)
	}
	return &Session{ID: sessionID, TenantID: tenantID, RefreshTokenHash: newTokenHash}, nil
}

func (m *mockRepository) RevokeSession(ctx context.Context, tenantID, sessionID uuid.UUID) error {
	if m.revokeSessionFn != nil {
		return m.revokeSessionFn(ctx, tenantID, sessionID)
	}
	return nil
}

func (m *mockRepository) RevokeAllUserSessions(ctx context.Context, tenantID, userID uuid.UUID) error {
	if m.revokeAllUserSessionsFn != nil {
		return m.revokeAllUserSessionsFn(ctx, tenantID, userID)
	}
	return nil
}

func (m *mockRepository) RevokeOtherUserSessions(ctx context.Context, tenantID, userID, currentSessionID uuid.UUID) error {
	if m.revokeOtherUserSessionsFn != nil {
		return m.revokeOtherUserSessionsFn(ctx, tenantID, userID, currentSessionID)
	}
	return nil
}

func (m *mockRepository) CreatePasswordResetToken(ctx context.Context, tenantID, userID uuid.UUID, tokenHash string, expiresAt time.Time) (*PasswordResetToken, error) {
	if m.createPasswordResetTokenFn != nil {
		return m.createPasswordResetTokenFn(ctx, tenantID, userID, tokenHash, expiresAt)
	}
	return &PasswordResetToken{ID: uuid.New(), TenantID: tenantID, UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt}, nil
}

func (m *mockRepository) GetPasswordResetTokenByHash(ctx context.Context, tokenHash string) (*PasswordResetToken, error) {
	if m.getPasswordResetTokenByHashFn != nil {
		return m.getPasswordResetTokenByHashFn(ctx, tokenHash)
	}
	return nil, ErrResetTokenNotFound
}

func (m *mockRepository) ConsumePasswordResetToken(ctx context.Context, tokenID uuid.UUID) error {
	if m.consumePasswordResetTokenFn != nil {
		return m.consumePasswordResetTokenFn(ctx, tokenID)
	}
	return nil
}

func (m *mockRepository) InvalidatePasswordResetTokensForUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	if m.invalidatePasswordResetTokensForUserFn != nil {
		return m.invalidatePasswordResetTokensForUserFn(ctx, tenantID, userID)
	}
	return nil
}

func (m *mockRepository) CreateEmailVerificationToken(ctx context.Context, tenantID, userID uuid.UUID, tokenHash string, expiresAt time.Time) (*EmailVerificationToken, error) {
	if m.createEmailVerificationTokenFn != nil {
		return m.createEmailVerificationTokenFn(ctx, tenantID, userID, tokenHash, expiresAt)
	}
	return &EmailVerificationToken{ID: uuid.New(), TenantID: tenantID, UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt}, nil
}

func (m *mockRepository) GetEmailVerificationTokenByHash(ctx context.Context, tokenHash string) (*EmailVerificationToken, error) {
	if m.getEmailVerificationTokenByHashFn != nil {
		return m.getEmailVerificationTokenByHashFn(ctx, tokenHash)
	}
	return nil, ErrVerificationTokenNotFound
}

func (m *mockRepository) ConsumeEmailVerificationToken(ctx context.Context, tokenID uuid.UUID) error {
	if m.consumeEmailVerificationTokenFn != nil {
		return m.consumeEmailVerificationTokenFn(ctx, tokenID)
	}
	return nil
}

func (m *mockRepository) InvalidateEmailVerificationTokensForUser(ctx context.Context, tenantID, userID uuid.UUID) error {
	if m.invalidateEmailVerificationTokensForUserFn != nil {
		return m.invalidateEmailVerificationTokensForUserFn(ctx, tenantID, userID)
	}
	return nil
}

// mockTxManager executes fn with the provided repo (no real transaction).
type mockTxManager struct {
	repo Repository
}

func (m *mockTxManager) WithTx(ctx context.Context, fn func(txRepo Repository) error) error {
	return fn(m.repo)
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func authTestContext(tenantID uuid.UUID) context.Context {
	return tenant.SetID(context.Background(), tenantID.String())
}

func activeUserInfo(tenantID uuid.UUID) *UserInfo {
	return &UserInfo{
		ID:            uuid.New(),
		TenantID:      tenantID,
		Email:         "test@example.com",
		Status:        "active",
		EmailVerified: true,
	}
}

func newTestService(repo *mockRepository, users *mockUserProvider) *Service {
	hasher := &mockPasswordHasher{}
	tokenIssuer := &mockTokenIssuer{}
	tokenGen := &mockTokenGenerator{}
	tx := &mockTxManager{repo: repo}

	svc, err := NewService(
		repo, users, hasher, tokenIssuer, tokenGen, tx,
		DefaultServiceConfig(), zerolog.Nop(),
	)
	if err != nil {
		panic("newTestService failed: " + err.Error())
	}
	return svc
}

func newTestServiceWithDeps(
	repo *mockRepository,
	users *mockUserProvider,
	hasher *mockPasswordHasher,
	tokenIssuer *mockTokenIssuer,
	tokenGen *mockTokenGenerator,
) *Service {
	tx := &mockTxManager{repo: repo}
	svc, err := NewService(
		repo, users, hasher, tokenIssuer, tokenGen, tx,
		DefaultServiceConfig(), zerolog.Nop(),
	)
	if err != nil {
		panic("newTestServiceWithDeps failed: " + err.Error())
	}
	return svc
}

func isDomainError(err error, code platformerrors.ErrorCode) bool {
	var domainErr *platformerrors.DomainError
	return platformerrors.As(err, &domainErr) && domainErr.Code == code
}

func isValidationError(err error) bool {
	var validationErr *platformerrors.ValidationError
	return platformerrors.As(err, &validationErr)
}

// ---------------------------------------------------------------------------
// Register tests
// ---------------------------------------------------------------------------

func TestRegister_Success(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	createdUserID := uuid.New()

	repo := &mockRepository{}
	users := &mockUserProvider{
		createUserFn: func(_ context.Context, email, displayName string) (*UserInfo, error) {
			return &UserInfo{ID: createdUserID, TenantID: tenantID, Email: email, Status: "active"}, nil
		},
	}

	svc := newTestService(repo, users)
	result, err := svc.Register(ctx, RegisterInput{
		Email:       "user@example.com",
		DisplayName: "Test User",
		Password:    "securepassword12",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.UserID != createdUserID {
		t.Errorf("UserID = %v, want %v", result.UserID, createdUserID)
	}
	if result.VerificationToken == "" {
		t.Error("VerificationToken should not be empty")
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)

	repo := &mockRepository{}
	users := &mockUserProvider{
		createUserFn: func(_ context.Context, _, _ string) (*UserInfo, error) {
			return nil, platformerrors.NewConflict("email already exists")
		},
	}

	svc := newTestService(repo, users)
	_, err := svc.Register(ctx, RegisterInput{
		Email:       "dup@example.com",
		DisplayName: "Test",
		Password:    "securepassword12",
	})

	if !isDomainError(err, platformerrors.CodeConflict) {
		t.Errorf("expected CONFLICT error, got: %v", err)
	}
}

func TestRegister_PasswordTooShort(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)

	svc := newTestService(&mockRepository{}, &mockUserProvider{})
	_, err := svc.Register(ctx, RegisterInput{
		Email:       "user@example.com",
		DisplayName: "Test",
		Password:    "short",
	})

	if !isValidationError(err) {
		t.Errorf("expected ValidationError, got: %v", err)
	}
}

func TestRegister_PasswordHashingCalled(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	hashCalled := false

	repo := &mockRepository{}
	users := &mockUserProvider{
		createUserFn: func(_ context.Context, email, _ string) (*UserInfo, error) {
			return &UserInfo{ID: uuid.New(), TenantID: tenantID, Email: email, Status: "active"}, nil
		},
	}
	hasher := &mockPasswordHasher{
		hashFn: func(password string) (string, error) {
			hashCalled = true
			return "$argon2id$mock$" + password, nil
		},
	}

	svc := newTestServiceWithDeps(repo, users, hasher, &mockTokenIssuer{}, &mockTokenGenerator{})
	_, err := svc.Register(ctx, RegisterInput{
		Email:       "user@example.com",
		DisplayName: "Test",
		Password:    "securepassword12",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hashCalled {
		t.Error("password hasher.Hash was not called")
	}
}

func TestRegister_VerificationTokenCreated(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	tokenCreated := false

	repo := &mockRepository{
		createEmailVerificationTokenFn: func(_ context.Context, _, _ uuid.UUID, _ string, _ time.Time) (*EmailVerificationToken, error) {
			tokenCreated = true
			return &EmailVerificationToken{ID: uuid.New()}, nil
		},
	}
	users := &mockUserProvider{
		createUserFn: func(_ context.Context, email, _ string) (*UserInfo, error) {
			return &UserInfo{ID: uuid.New(), TenantID: tenantID, Email: email, Status: "active"}, nil
		},
	}

	svc := newTestService(repo, users)
	_, err := svc.Register(ctx, RegisterInput{
		Email:       "user@example.com",
		DisplayName: "Test",
		Password:    "securepassword12",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tokenCreated {
		t.Error("verification token was not created")
	}
}

func TestRegister_TransactionFailure(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)

	repo := &mockRepository{
		createCredentialFn: func(_ context.Context, _, _ uuid.UUID, _ string) (*Credential, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	users := &mockUserProvider{
		createUserFn: func(_ context.Context, email, _ string) (*UserInfo, error) {
			return &UserInfo{ID: uuid.New(), TenantID: tenantID, Email: email, Status: "active"}, nil
		},
	}

	svc := newTestService(repo, users)
	_, err := svc.Register(ctx, RegisterInput{
		Email:       "user@example.com",
		DisplayName: "Test",
		Password:    "securepassword12",
	})

	if err == nil {
		t.Fatal("expected error from transaction failure")
	}
}

func TestRegister_InvalidInput(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	svc := newTestService(&mockRepository{}, &mockUserProvider{})

	_, err := svc.Register(ctx, RegisterInput{
		Email:       "",
		DisplayName: "",
		Password:    "securepassword12",
	})

	if !isValidationError(err) {
		t.Errorf("expected ValidationError for missing fields, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Login tests
// ---------------------------------------------------------------------------

func TestLogin_ValidCredentials(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()

	repo := &mockRepository{
		getCredentialByUserIDFn: func(_ context.Context, _, _ uuid.UUID) (*Credential, error) {
			return &Credential{UserID: userID, PasswordHash: "$argon2id$mock$securepassword12"}, nil
		},
	}
	users := &mockUserProvider{
		getUserByEmailFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return &UserInfo{ID: userID, TenantID: tenantID, Email: "user@example.com", Status: "active", EmailVerified: true}, nil
		},
	}

	svc := newTestService(repo, users)
	result, err := svc.Login(ctx, LoginInput{
		Email:    "user@example.com",
		Password: "securepassword12",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken == "" {
		t.Error("AccessToken should not be empty")
	}
	if result.RefreshToken == "" {
		t.Error("RefreshToken should not be empty")
	}
	if result.TokenType != "Bearer" {
		t.Errorf("TokenType = %q, want %q", result.TokenType, "Bearer")
	}
	if result.ExpiresIn != 900 {
		t.Errorf("ExpiresIn = %d, want 900", result.ExpiresIn)
	}
}

func TestLogin_InvalidPassword(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()

	repo := &mockRepository{
		getCredentialByUserIDFn: func(_ context.Context, _, _ uuid.UUID) (*Credential, error) {
			return &Credential{UserID: userID, PasswordHash: "$argon2id$mock$correctpassword1"}, nil
		},
	}
	users := &mockUserProvider{
		getUserByEmailFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return &UserInfo{ID: userID, TenantID: tenantID, Email: "user@example.com", Status: "active", EmailVerified: true}, nil
		},
	}

	svc := newTestService(repo, users)
	_, err := svc.Login(ctx, LoginInput{
		Email:    "user@example.com",
		Password: "wrongpassword123",
	})

	if !isDomainError(err, platformerrors.CodeUnauthorized) {
		t.Errorf("expected UNAUTHORIZED error, got: %v", err)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	dummyVerifyCalled := false

	users := &mockUserProvider{
		getUserByEmailFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return nil, platformerrors.NewNotFound("user not found")
		},
	}
	hasher := &mockPasswordHasher{
		verifyFn: func(_, _ string) (bool, error) {
			dummyVerifyCalled = true
			return false, nil
		},
	}

	svc := newTestServiceWithDeps(&mockRepository{}, users, hasher, &mockTokenIssuer{}, &mockTokenGenerator{})
	_, err := svc.Login(ctx, LoginInput{
		Email:    "unknown@example.com",
		Password: "somepassword1234",
	})

	if !isDomainError(err, platformerrors.CodeUnauthorized) {
		t.Errorf("expected UNAUTHORIZED error, got: %v", err)
	}
	if !dummyVerifyCalled {
		t.Error("dummy hash verification was not called (timing leak)")
	}
}

func TestLogin_UnverifiedEmail(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)

	users := &mockUserProvider{
		getUserByEmailFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return &UserInfo{ID: uuid.New(), TenantID: tenantID, Email: "user@example.com", Status: "active", EmailVerified: false}, nil
		},
	}

	svc := newTestService(&mockRepository{}, users)
	_, err := svc.Login(ctx, LoginInput{
		Email:    "user@example.com",
		Password: "somepassword1234",
	})

	if !isDomainError(err, platformerrors.CodeForbidden) {
		t.Errorf("expected FORBIDDEN error for unverified email, got: %v", err)
	}
}

func TestLogin_SessionCreated(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()
	sessionCreated := false

	repo := &mockRepository{
		getCredentialByUserIDFn: func(_ context.Context, _, _ uuid.UUID) (*Credential, error) {
			return &Credential{UserID: userID, PasswordHash: "$argon2id$mock$securepassword12"}, nil
		},
		createSessionFn: func(_ context.Context, session *Session) (*Session, error) {
			sessionCreated = true
			session.ID = uuid.New()
			return session, nil
		},
	}
	users := &mockUserProvider{
		getUserByEmailFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return &UserInfo{ID: userID, TenantID: tenantID, Email: "user@example.com", Status: "active", EmailVerified: true}, nil
		},
	}

	svc := newTestService(repo, users)
	_, err := svc.Login(ctx, LoginInput{
		Email:    "user@example.com",
		Password: "securepassword12",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sessionCreated {
		t.Error("session was not created")
	}
}

func TestLogin_TokenIssuance(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()
	issueCalled := false

	repo := &mockRepository{
		getCredentialByUserIDFn: func(_ context.Context, _, _ uuid.UUID) (*Credential, error) {
			return &Credential{UserID: userID, PasswordHash: "$argon2id$mock$securepassword12"}, nil
		},
	}
	users := &mockUserProvider{
		getUserByEmailFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return &UserInfo{ID: userID, TenantID: tenantID, Email: "user@example.com", Status: "active", EmailVerified: true}, nil
		},
	}
	tokenIssuer := &mockTokenIssuer{
		issueFn: func(claims TokenClaims) (string, error) {
			issueCalled = true
			if claims.UserID != userID {
				t.Errorf("claims.UserID = %v, want %v", claims.UserID, userID)
			}
			return "test-jwt", nil
		},
	}

	svc := newTestServiceWithDeps(repo, users, &mockPasswordHasher{}, tokenIssuer, &mockTokenGenerator{})
	result, err := svc.Login(ctx, LoginInput{
		Email:    "user@example.com",
		Password: "securepassword12",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !issueCalled {
		t.Error("TokenIssuer.Issue was not called")
	}
	if result.AccessToken != "test-jwt" {
		t.Errorf("AccessToken = %q, want %q", result.AccessToken, "test-jwt")
	}
}

// ---------------------------------------------------------------------------
// RefreshToken tests
// ---------------------------------------------------------------------------

func TestRefreshToken_ValidRefresh(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	sessionID := uuid.New()
	userID := uuid.New()
	familyID := uuid.New()

	// Use a deterministic token generator that returns different values
	// for the initial token vs. the rotated token.
	callCount := 0
	tokenGen := &mockTokenGenerator{
		generateFn: func() (string, error) {
			callCount++
			return fmt.Sprintf("random-token-%d", callCount), nil
		},
	}

	// Build the initial refresh token using callCount=1.
	initialRaw := "random-token-initial"
	refreshToken := composeRefreshToken(familyID, initialRaw)
	tokenHash := tokenGen.Hash(refreshToken)

	repo := &mockRepository{
		getActiveSessionByFamilyIDForUpdateFn: func(_ context.Context, fID uuid.UUID) (*Session, error) {
			return &Session{
				ID: sessionID, TenantID: tenantID, UserID: userID,
				FamilyID: familyID, RefreshTokenHash: tokenHash,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil
		},
		rotateRefreshTokenFn: func(_ context.Context, _, sID uuid.UUID, newHash string) (*Session, error) {
			return &Session{ID: sID, TenantID: tenantID, UserID: userID, FamilyID: familyID, RefreshTokenHash: newHash}, nil
		},
	}
	users := &mockUserProvider{
		getUserFn: func(_ context.Context, _ uuid.UUID) (*UserInfo, error) {
			return &UserInfo{ID: userID, TenantID: tenantID, Status: "active"}, nil
		},
	}

	svc := newTestServiceWithDeps(repo, users, &mockPasswordHasher{}, &mockTokenIssuer{}, tokenGen)
	result, err := svc.RefreshToken(ctx, RefreshInput{RefreshToken: refreshToken})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AccessToken == "" {
		t.Error("AccessToken should not be empty")
	}
	if result.RefreshToken == "" {
		t.Error("new RefreshToken should not be empty")
	}
	if result.RefreshToken == refreshToken {
		t.Error("new RefreshToken should differ from old one")
	}
}

func TestRefreshToken_ExpiredSession(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	familyID := uuid.New()
	refreshToken := composeRefreshToken(familyID, "mock-random-token")
	tokenHash := "hash:" + refreshToken

	repo := &mockRepository{
		getActiveSessionByFamilyIDForUpdateFn: func(_ context.Context, _ uuid.UUID) (*Session, error) {
			return &Session{
				ID: uuid.New(), TenantID: tenantID, UserID: uuid.New(),
				FamilyID: familyID, RefreshTokenHash: tokenHash,
				ExpiresAt: time.Now().Add(-time.Hour), // expired
			}, nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	_, err := svc.RefreshToken(ctx, RefreshInput{RefreshToken: refreshToken})

	if !isDomainError(err, platformerrors.CodeUnauthorized) {
		t.Errorf("expected UNAUTHORIZED error for expired session, got: %v", err)
	}
}

func TestRefreshToken_RevokedSession(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	familyID := uuid.New()
	refreshToken := composeRefreshToken(familyID, "mock-random-token")

	repo := &mockRepository{
		getActiveSessionByFamilyIDForUpdateFn: func(_ context.Context, _ uuid.UUID) (*Session, error) {
			return nil, ErrSessionNotFound // revoked sessions don't appear
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	_, err := svc.RefreshToken(ctx, RefreshInput{RefreshToken: refreshToken})

	if err == nil {
		t.Fatal("expected error for revoked session")
	}
}

func TestRefreshToken_SuccessfulRotation(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()
	familyID := uuid.New()
	refreshToken := composeRefreshToken(familyID, "mock-random-token")
	tokenHash := "hash:" + refreshToken
	rotated := false

	repo := &mockRepository{
		getActiveSessionByFamilyIDForUpdateFn: func(_ context.Context, _ uuid.UUID) (*Session, error) {
			return &Session{
				ID: uuid.New(), TenantID: tenantID, UserID: userID,
				FamilyID: familyID, RefreshTokenHash: tokenHash,
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil
		},
		rotateRefreshTokenFn: func(_ context.Context, _, sID uuid.UUID, newHash string) (*Session, error) {
			rotated = true
			return &Session{ID: sID, TenantID: tenantID, UserID: userID, FamilyID: familyID, RefreshTokenHash: newHash}, nil
		},
	}
	users := &mockUserProvider{
		getUserFn: func(_ context.Context, _ uuid.UUID) (*UserInfo, error) {
			return &UserInfo{ID: userID, TenantID: tenantID, Status: "active"}, nil
		},
	}

	svc := newTestService(repo, users)
	_, err := svc.RefreshToken(ctx, RefreshInput{RefreshToken: refreshToken})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rotated {
		t.Error("RotateRefreshToken was not called")
	}
}

func TestRefreshToken_TokenReuse(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	familyID := uuid.New()
	refreshToken := composeRefreshToken(familyID, "old-token")
	sessionRevoked := false

	repo := &mockRepository{
		getActiveSessionByFamilyIDForUpdateFn: func(_ context.Context, _ uuid.UUID) (*Session, error) {
			return &Session{
				ID: uuid.New(), TenantID: tenantID, UserID: uuid.New(),
				FamilyID: familyID, RefreshTokenHash: "hash:different-token", // mismatch
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil
		},
		revokeSessionFn: func(_ context.Context, _, _ uuid.UUID) error {
			sessionRevoked = true
			return nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	_, err := svc.RefreshToken(ctx, RefreshInput{RefreshToken: refreshToken})

	if err != ErrRefreshTokenReuse {
		t.Errorf("expected ErrRefreshTokenReuse, got: %v", err)
	}
	if !sessionRevoked {
		t.Error("session was not revoked on reuse detection")
	}
}

func TestRefreshToken_TokenFamilyRevocation(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	familyID := uuid.New()
	refreshToken := composeRefreshToken(familyID, "stolen-token")
	revokedSessionID := uuid.Nil

	repo := &mockRepository{
		getActiveSessionByFamilyIDForUpdateFn: func(_ context.Context, _ uuid.UUID) (*Session, error) {
			sessionID := uuid.New()
			return &Session{
				ID: sessionID, TenantID: tenantID, UserID: uuid.New(),
				FamilyID: familyID, RefreshTokenHash: "hash:current-valid-token",
				ExpiresAt: time.Now().Add(24 * time.Hour),
			}, nil
		},
		revokeSessionFn: func(_ context.Context, _, sID uuid.UUID) error {
			revokedSessionID = sID
			return nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	_, err := svc.RefreshToken(ctx, RefreshInput{RefreshToken: refreshToken})

	if err != ErrRefreshTokenReuse {
		t.Fatalf("expected ErrRefreshTokenReuse, got: %v", err)
	}
	if revokedSessionID == uuid.Nil {
		t.Error("session should have been revoked")
	}
}

// ---------------------------------------------------------------------------
// Logout tests
// ---------------------------------------------------------------------------

func TestLogout_CurrentSessionRevoked(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	sessionID := uuid.New()
	revoked := false

	repo := &mockRepository{
		revokeSessionFn: func(_ context.Context, _, sID uuid.UUID) error {
			if sID == sessionID {
				revoked = true
			}
			return nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.Logout(ctx, LogoutInput{SessionID: sessionID})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !revoked {
		t.Error("session was not revoked")
	}
}

func TestLogout_RepeatedLogout(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	sessionID := uuid.New()

	repo := &mockRepository{
		revokeSessionFn: func(_ context.Context, _, _ uuid.UUID) error {
			return nil // idempotent
		},
	}

	svc := newTestService(repo, &mockUserProvider{})

	// First logout
	if err := svc.Logout(ctx, LogoutInput{SessionID: sessionID}); err != nil {
		t.Fatalf("first logout: %v", err)
	}
	// Second logout (idempotent)
	if err := svc.Logout(ctx, LogoutInput{SessionID: sessionID}); err != nil {
		t.Fatalf("second logout: %v", err)
	}
}

// ---------------------------------------------------------------------------
// LogoutAll tests
// ---------------------------------------------------------------------------

func TestLogoutAll_AllUserSessionsRevoked(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()
	revoked := false

	repo := &mockRepository{
		revokeAllUserSessionsFn: func(_ context.Context, tID, uID uuid.UUID) error {
			if tID == tenantID && uID == userID {
				revoked = true
			}
			return nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.LogoutAll(ctx, LogoutAllInput{UserID: userID})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !revoked {
		t.Error("RevokeAllUserSessions was not called")
	}
}

// ---------------------------------------------------------------------------
// ChangePassword tests
// ---------------------------------------------------------------------------

func TestChangePassword_ValidCurrentPassword(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()
	sessionID := uuid.New()

	repo := &mockRepository{
		getCredentialByUserIDFn: func(_ context.Context, _, _ uuid.UUID) (*Credential, error) {
			return &Credential{PasswordHash: "$argon2id$mock$currentpass1234"}, nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.ChangePassword(ctx, ChangePasswordInput{
		UserID:          userID,
		SessionID:       sessionID,
		CurrentPassword: "currentpass1234",
		NewPassword:     "newpassword12345",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestChangePassword_InvalidCurrentPassword(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)

	repo := &mockRepository{
		getCredentialByUserIDFn: func(_ context.Context, _, _ uuid.UUID) (*Credential, error) {
			return &Credential{PasswordHash: "$argon2id$mock$currentpass1234"}, nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.ChangePassword(ctx, ChangePasswordInput{
		UserID:          uuid.New(),
		SessionID:       uuid.New(),
		CurrentPassword: "wrongpassword123",
		NewPassword:     "newpassword12345",
	})

	if !isDomainError(err, platformerrors.CodeUnauthorized) {
		t.Errorf("expected UNAUTHORIZED error, got: %v", err)
	}
}

func TestChangePassword_SuccessfulUpdate(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()
	sessionID := uuid.New()
	passwordUpdated := false
	othersRevoked := false

	repo := &mockRepository{
		getCredentialByUserIDFn: func(_ context.Context, _, _ uuid.UUID) (*Credential, error) {
			return &Credential{PasswordHash: "$argon2id$mock$currentpass1234"}, nil
		},
		updatePasswordHashFn: func(_ context.Context, _, _ uuid.UUID, _ string) (*Credential, error) {
			passwordUpdated = true
			return &Credential{}, nil
		},
		revokeOtherUserSessionsFn: func(_ context.Context, _, _, curSID uuid.UUID) error {
			if curSID == sessionID {
				othersRevoked = true
			}
			return nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.ChangePassword(ctx, ChangePasswordInput{
		UserID:          userID,
		SessionID:       sessionID,
		CurrentPassword: "currentpass1234",
		NewPassword:     "newpassword12345",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passwordUpdated {
		t.Error("password hash was not updated")
	}
	if !othersRevoked {
		t.Error("other sessions were not revoked")
	}
}

func TestChangePassword_SessionsRevoked(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	sessionID := uuid.New()
	revokedCurrentSession := uuid.Nil

	repo := &mockRepository{
		getCredentialByUserIDFn: func(_ context.Context, _, _ uuid.UUID) (*Credential, error) {
			return &Credential{PasswordHash: "$argon2id$mock$currentpass1234"}, nil
		},
		revokeOtherUserSessionsFn: func(_ context.Context, _, _ uuid.UUID, curSID uuid.UUID) error {
			revokedCurrentSession = curSID
			return nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.ChangePassword(ctx, ChangePasswordInput{
		UserID:          uuid.New(),
		SessionID:       sessionID,
		CurrentPassword: "currentpass1234",
		NewPassword:     "newpassword12345",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revokedCurrentSession != sessionID {
		t.Error("RevokeOtherUserSessions should preserve the current session")
	}
}

// ---------------------------------------------------------------------------
// ForgotPassword tests
// ---------------------------------------------------------------------------

func TestForgotPassword_UnknownEmail(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)

	users := &mockUserProvider{
		getUserByEmailFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return nil, platformerrors.NewNotFound("user not found")
		},
	}

	svc := newTestService(&mockRepository{}, users)
	result, err := svc.ForgotPassword(ctx, ForgotPasswordInput{Email: "unknown@example.com"})

	if err != nil {
		t.Fatalf("expected nil error for unknown email (anti-enumeration), got: %v", err)
	}
	if result != nil {
		t.Error("result should be nil for unknown email")
	}
}

func TestForgotPassword_TokenCreation(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()
	tokenCreated := false

	repo := &mockRepository{
		createPasswordResetTokenFn: func(_ context.Context, _, _ uuid.UUID, _ string, _ time.Time) (*PasswordResetToken, error) {
			tokenCreated = true
			return &PasswordResetToken{ID: uuid.New()}, nil
		},
	}
	users := &mockUserProvider{
		getUserByEmailFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return &UserInfo{ID: userID, TenantID: tenantID, Email: "user@example.com", Status: "active"}, nil
		},
	}

	svc := newTestService(repo, users)
	result, err := svc.ForgotPassword(ctx, ForgotPasswordInput{Email: "user@example.com"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tokenCreated {
		t.Error("reset token was not created")
	}
	if result == nil || result.ResetToken == "" {
		t.Error("result should contain a reset token")
	}
}

// ---------------------------------------------------------------------------
// ResetPassword tests
// ---------------------------------------------------------------------------

func TestResetPassword_InvalidToken(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)

	svc := newTestService(&mockRepository{}, &mockUserProvider{})
	err := svc.ResetPassword(ctx, ResetPasswordInput{
		Token:       "nonexistent-token",
		NewPassword: "newpassword12345",
	})

	if !isDomainError(err, platformerrors.CodeBadRequest) {
		t.Errorf("expected BAD_REQUEST error for invalid token, got: %v", err)
	}
}

func TestResetPassword_ExpiredToken(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	tokenID := uuid.New()

	repo := &mockRepository{
		getPasswordResetTokenByHashFn: func(_ context.Context, _ string) (*PasswordResetToken, error) {
			return &PasswordResetToken{
				ID: tokenID, TenantID: tenantID, UserID: uuid.New(),
				ExpiresAt: time.Now().Add(-time.Hour), // expired
			}, nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.ResetPassword(ctx, ResetPasswordInput{
		Token:       "valid-format-token",
		NewPassword: "newpassword12345",
	})

	if err != ErrResetTokenExpired {
		t.Errorf("expected ErrResetTokenExpired, got: %v", err)
	}
}

func TestResetPassword_SuccessfulReset(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()
	tokenID := uuid.New()
	passwordUpdated := false
	tokenConsumed := false
	sessionsRevoked := false

	repo := &mockRepository{
		getPasswordResetTokenByHashFn: func(_ context.Context, _ string) (*PasswordResetToken, error) {
			return &PasswordResetToken{
				ID: tokenID, TenantID: tenantID, UserID: userID,
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
		updatePasswordHashFn: func(_ context.Context, _, _ uuid.UUID, _ string) (*Credential, error) {
			passwordUpdated = true
			return &Credential{}, nil
		},
		consumePasswordResetTokenFn: func(_ context.Context, tID uuid.UUID) error {
			if tID == tokenID {
				tokenConsumed = true
			}
			return nil
		},
		revokeAllUserSessionsFn: func(_ context.Context, _, uID uuid.UUID) error {
			if uID == userID {
				sessionsRevoked = true
			}
			return nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.ResetPassword(ctx, ResetPasswordInput{
		Token:       "valid-reset-token",
		NewPassword: "newpassword12345",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !passwordUpdated {
		t.Error("password was not updated")
	}
	if !tokenConsumed {
		t.Error("reset token was not consumed")
	}
	if !sessionsRevoked {
		t.Error("sessions were not revoked")
	}
}

func TestResetPassword_TokenReuse(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	usedAt := time.Now()

	repo := &mockRepository{
		getPasswordResetTokenByHashFn: func(_ context.Context, _ string) (*PasswordResetToken, error) {
			return &PasswordResetToken{
				ID: uuid.New(), TenantID: tenantID, UserID: uuid.New(),
				ExpiresAt: time.Now().Add(time.Hour),
				UsedAt:    &usedAt, // already used
			}, nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.ResetPassword(ctx, ResetPasswordInput{
		Token:       "used-token",
		NewPassword: "newpassword12345",
	})

	if err != ErrResetTokenAlreadyUsed {
		t.Errorf("expected ErrResetTokenAlreadyUsed, got: %v", err)
	}
}

func TestResetPassword_SessionRevocation(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()
	revokedUserID := uuid.Nil

	repo := &mockRepository{
		getPasswordResetTokenByHashFn: func(_ context.Context, _ string) (*PasswordResetToken, error) {
			return &PasswordResetToken{
				ID: uuid.New(), TenantID: tenantID, UserID: userID,
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
		revokeAllUserSessionsFn: func(_ context.Context, _, uID uuid.UUID) error {
			revokedUserID = uID
			return nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.ResetPassword(ctx, ResetPasswordInput{
		Token:       "valid-token",
		NewPassword: "newpassword12345",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if revokedUserID != userID {
		t.Error("all user sessions should be revoked after password reset")
	}
}

// ---------------------------------------------------------------------------
// VerifyEmail tests
// ---------------------------------------------------------------------------

func TestVerifyEmail_ValidToken(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()
	tokenID := uuid.New()
	emailVerified := false

	repo := &mockRepository{
		getEmailVerificationTokenByHashFn: func(_ context.Context, _ string) (*EmailVerificationToken, error) {
			return &EmailVerificationToken{
				ID: tokenID, TenantID: tenantID, UserID: userID,
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}
	users := &mockUserProvider{
		setEmailVerifiedFn: func(_ context.Context, uID uuid.UUID) error {
			if uID == userID {
				emailVerified = true
			}
			return nil
		},
	}

	svc := newTestService(repo, users)
	err := svc.VerifyEmail(ctx, VerifyEmailInput{Token: "valid-token"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !emailVerified {
		t.Error("SetEmailVerified was not called")
	}
}

func TestVerifyEmail_ExpiredToken(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)

	repo := &mockRepository{
		getEmailVerificationTokenByHashFn: func(_ context.Context, _ string) (*EmailVerificationToken, error) {
			return &EmailVerificationToken{
				ID: uuid.New(), TenantID: tenantID, UserID: uuid.New(),
				ExpiresAt: time.Now().Add(-time.Hour), // expired
			}, nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.VerifyEmail(ctx, VerifyEmailInput{Token: "expired-token"})

	if err != ErrVerificationTokenExpired {
		t.Errorf("expected ErrVerificationTokenExpired, got: %v", err)
	}
}

func TestVerifyEmail_InvalidToken(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)

	svc := newTestService(&mockRepository{}, &mockUserProvider{})
	err := svc.VerifyEmail(ctx, VerifyEmailInput{Token: "nonexistent"})

	if err != ErrVerificationTokenNotFound {
		t.Errorf("expected ErrVerificationTokenNotFound, got: %v", err)
	}
}

func TestVerifyEmail_AlreadyConsumedToken(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	usedAt := time.Now()

	repo := &mockRepository{
		getEmailVerificationTokenByHashFn: func(_ context.Context, _ string) (*EmailVerificationToken, error) {
			return &EmailVerificationToken{
				ID: uuid.New(), TenantID: tenantID, UserID: uuid.New(),
				ExpiresAt: time.Now().Add(time.Hour),
				UsedAt:    &usedAt,
			}, nil
		},
	}

	svc := newTestService(repo, &mockUserProvider{})
	err := svc.VerifyEmail(ctx, VerifyEmailInput{Token: "used-token"})

	if err != ErrVerificationTokenAlreadyUsed {
		t.Errorf("expected ErrVerificationTokenAlreadyUsed, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// ResendVerification tests
// ---------------------------------------------------------------------------

func TestResendVerification_UnknownEmail(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)

	users := &mockUserProvider{
		getUserByEmailFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return nil, platformerrors.NewNotFound("user not found")
		},
	}

	svc := newTestService(&mockRepository{}, users)
	result, err := svc.ResendVerification(ctx, ResendVerificationInput{Email: "unknown@example.com"})

	if err != nil {
		t.Fatalf("expected nil error (anti-enumeration), got: %v", err)
	}
	if result != nil {
		t.Error("result should be nil for unknown email")
	}
}

func TestResendVerification_AlreadyVerified(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)

	users := &mockUserProvider{
		getUserByEmailFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return &UserInfo{ID: uuid.New(), TenantID: tenantID, Email: "user@example.com", Status: "active", EmailVerified: true}, nil
		},
	}

	svc := newTestService(&mockRepository{}, users)
	result, err := svc.ResendVerification(ctx, ResendVerificationInput{Email: "user@example.com"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("result should be nil for already-verified email")
	}
}

func TestResendVerification_NewTokenCreated(t *testing.T) {
	tenantID := uuid.New()
	ctx := authTestContext(tenantID)
	userID := uuid.New()
	invalidated := false
	created := false

	repo := &mockRepository{
		invalidateEmailVerificationTokensForUserFn: func(_ context.Context, _, _ uuid.UUID) error {
			invalidated = true
			return nil
		},
		createEmailVerificationTokenFn: func(_ context.Context, _, _ uuid.UUID, _ string, _ time.Time) (*EmailVerificationToken, error) {
			created = true
			return &EmailVerificationToken{ID: uuid.New()}, nil
		},
	}
	users := &mockUserProvider{
		getUserByEmailFn: func(_ context.Context, _ string) (*UserInfo, error) {
			return &UserInfo{ID: userID, TenantID: tenantID, Email: "user@example.com", Status: "active", EmailVerified: false}, nil
		},
	}

	svc := newTestService(repo, users)
	result, err := svc.ResendVerification(ctx, ResendVerificationInput{Email: "user@example.com"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !invalidated {
		t.Error("old tokens were not invalidated")
	}
	if !created {
		t.Error("new verification token was not created")
	}
	if result == nil || result.VerificationToken == "" {
		t.Error("result should contain a verification token")
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name    string
		pwd     string
		wantErr bool
	}{
		{"valid 12 chars", "abcdefghijkl", false},
		{"valid long", "a-very-secure-and-long-password-that-is-perfectly-fine", false},
		{"too short", "short", true},
		{"exactly 11", "12345678901", true},
		{"exactly 12", "123456789012", false},
		{"too long", string(make([]byte, 129)), true},
		{"exactly 128", string(make([]byte, 128)), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.pwd)
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestParseRefreshToken(t *testing.T) {
	familyID := uuid.New()
	raw := "some-random-token"
	composed := composeRefreshToken(familyID, raw)

	parsedFamily, parsedRaw, err := parseRefreshToken(composed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsedFamily != familyID {
		t.Errorf("familyID = %v, want %v", parsedFamily, familyID)
	}
	if parsedRaw != raw {
		t.Errorf("raw = %q, want %q", parsedRaw, raw)
	}
}

func TestParseRefreshToken_Malformed(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"no dot", "nodothere"},
		{"empty parts", "."},
		{"empty family", ".raw-part"},
		{"invalid uuid", "not-a-uuid.raw-part"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseRefreshToken(tt.token)
			if err == nil {
				t.Error("expected error for malformed token")
			}
		})
	}
}

func TestComposeRefreshToken(t *testing.T) {
	familyID := uuid.New()
	raw := "test-raw-token"
	composed := composeRefreshToken(familyID, raw)

	expected := familyID.String() + "." + raw
	if composed != expected {
		t.Errorf("composed = %q, want %q", composed, expected)
	}
}

func TestNormalizeAuthEmail(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"User@Example.COM", "user@example.com"},
		{"  user@example.com  ", "user@example.com"},
		{"USER@EXAMPLE.COM", "user@example.com"},
	}

	for _, tt := range tests {
		got := normalizeAuthEmail(tt.input)
		if got != tt.want {
			t.Errorf("normalizeAuthEmail(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
