package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	platformerrors "github.com/conduit-platform/conduit/backend/internal/platform/errors"
	"github.com/conduit-platform/conduit/backend/internal/platform/tenant"
)

var authTracer = otel.Tracer("auth.service")

// ---------------------------------------------------------------------------
// Interfaces (ports) — owned by this consumer per coding-standards §6
// ---------------------------------------------------------------------------

// UserProvider defines what the Auth domain needs from the User domain.
// The user.Service satisfies this via duck typing; app.go wires them via an adapter.
type UserProvider interface {
	// CreateUser creates a new user identity. Returns the created user info.
	CreateUser(ctx context.Context, email, displayName string) (*UserInfo, error)
	// GetUserByEmail looks up a user by email within the tenant (from context).
	GetUserByEmail(ctx context.Context, email string) (*UserInfo, error)
	// GetUser looks up a user by ID within the tenant (from context).
	GetUser(ctx context.Context, userID uuid.UUID) (*UserInfo, error)
	// SetEmailVerified marks a user's email as verified.
	SetEmailVerified(ctx context.Context, userID uuid.UUID) error
}

// UserInfo is Auth's view of a user. This is NOT user.User — it is an
// Auth-domain type containing only the fields Auth needs.
type UserInfo struct {
	ID            uuid.UUID
	TenantID      uuid.UUID
	Email         string
	Status        string
	EmailVerified bool
}

// PasswordHasher abstracts password hashing. The infrastructure layer
// implements this with Argon2id per TDR-0001.
type PasswordHasher interface {
	Hash(password string) (string, error)
	Verify(password, hash string) (bool, error)
}

// TokenIssuer abstracts JWT access token issuance. The infrastructure
// layer implements this with Ed25519/EdDSA per TDR-0001.
type TokenIssuer interface {
	Issue(claims TokenClaims) (string, error)
}

// TokenClaims are the minimal claims passed to the JWT issuer.
// The issuer adds iss, aud, iat, exp, jti, and kid.
type TokenClaims struct {
	UserID    uuid.UUID
	TenantID  uuid.UUID
	SessionID uuid.UUID
}

// TokenGenerator abstracts opaque token generation and hashing.
// Used for refresh tokens, password reset tokens, and verification tokens.
// The infrastructure layer uses crypto/rand + crypto/sha256.
type TokenGenerator interface {
	// Generate creates a cryptographically random token (base64url-encoded)
	// and returns the raw token string.
	Generate() (raw string, err error)
	// Hash returns the SHA-256 hex digest of the given token string.
	Hash(token string) string
}

// TxManager manages database transactions for the auth service.
// The implementation (PostgresTxManager in repository.go) wraps
// database.WithTx and creates a transaction-scoped Repository.
type TxManager interface {
	WithTx(ctx context.Context, fn func(txRepo Repository) error) error
}

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

// ServiceConfig holds configurable durations for the auth service.
type ServiceConfig struct {
	SessionLifetime         time.Duration // Absolute session expiry. Default: 30 days.
	AccessTokenLifetimeSecs int           // Reported in AuthResult.ExpiresIn. Default: 900 (15 min).
	ResetTokenExpiry        time.Duration // Password reset token lifetime. Default: 30 minutes.
	VerificationTokenExpiry time.Duration // Email verification token lifetime. Default: 24 hours.
}

// DefaultServiceConfig returns production-ready defaults per RFC-0002.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		SessionLifetime:         30 * 24 * time.Hour,
		AccessTokenLifetimeSecs: 900,
		ResetTokenExpiry:        30 * time.Minute,
		VerificationTokenExpiry: 24 * time.Hour,
	}
}

// ---------------------------------------------------------------------------
// Input / Output types
// ---------------------------------------------------------------------------

// RegisterInput is the input for user registration.
type RegisterInput struct {
	Email       string
	DisplayName string
	Password    string
}

// RegisterResult is returned on successful registration.
// VerificationToken is the plaintext token for the notification layer
// to include in the verification email. It must NEVER be returned to
// API clients directly.
type RegisterResult struct {
	UserID            uuid.UUID
	Email             string
	VerificationToken string
}

// LoginInput is the input for authentication.
type LoginInput struct {
	Email     string
	Password  string
	IPAddress *string
	UserAgent *string
}

// AuthResult is returned on successful login or token refresh.
type AuthResult struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// RefreshInput is the input for token refresh.
type RefreshInput struct {
	RefreshToken string
}

// LogoutInput is the input for single-session logout.
type LogoutInput struct {
	SessionID uuid.UUID
}

// LogoutAllInput is the input for revoking all sessions.
type LogoutAllInput struct {
	UserID uuid.UUID
}

// ChangePasswordInput is the input for authenticated password change.
type ChangePasswordInput struct {
	UserID          uuid.UUID
	SessionID       uuid.UUID
	CurrentPassword string
	NewPassword     string
}

// ForgotPasswordInput is the input for the forgot-password flow.
type ForgotPasswordInput struct {
	Email string
}

// ForgotPasswordResult carries the plaintext reset token for the
// notification layer. Nil when the email doesn't exist (anti-enumeration).
type ForgotPasswordResult struct {
	ResetToken string
	UserID     uuid.UUID
	Email      string
}

// ResetPasswordInput is the input for password reset.
type ResetPasswordInput struct {
	Token       string
	NewPassword string
}

// VerifyEmailInput is the input for email verification.
type VerifyEmailInput struct {
	Token string
}

// ResendVerificationInput is the input for requesting a new verification token.
type ResendVerificationInput struct {
	Email string
}

// ResendVerificationResult carries the plaintext verification token for
// the notification layer. Nil when the user doesn't exist or email is
// already verified (anti-enumeration).
type ResendVerificationResult struct {
	VerificationToken string
	UserID            uuid.UUID
	Email             string
}

// ---------------------------------------------------------------------------
// Service
// ---------------------------------------------------------------------------

// Service implements the authentication business logic defined by RFC-0002.
// It orchestrates the 10 authentication use cases using injected interfaces
// for persistence, user management, cryptography, and transactions.
//
// The service MUST NOT depend on: Gin, HTTP, PostgreSQL, pgx, sqlc, Redis,
// JWT libraries, or Argon2 implementations. Those are infrastructure concerns
// accessed through the interfaces above.
type Service struct {
	repo      Repository     // auth persistence
	users     UserProvider   // user domain
	hasher    PasswordHasher // password hashing (Argon2id)
	tokens    TokenIssuer    // JWT access token issuance
	tokenGen  TokenGenerator // opaque token generation + hashing
	tx        TxManager      // database transactions
	cfg       ServiceConfig
	logger    zerolog.Logger
	dummyHash string // pre-computed hash for anti-enumeration timing safety
}

// NewService creates a new authentication Service.
// It pre-computes a dummy password hash for anti-enumeration (RFC-0002 §9).
func NewService(
	repo Repository,
	users UserProvider,
	hasher PasswordHasher,
	tokens TokenIssuer,
	tokenGen TokenGenerator,
	tx TxManager,
	cfg ServiceConfig,
	logger zerolog.Logger,
) (*Service, error) {
	// Pre-compute a dummy hash to make login timing constant regardless
	// of whether the user exists. This prevents timing-based enumeration.
	dummyHash, err := hasher.Hash("dummy-password-for-timing-safety")
	if err != nil {
		return nil, fmt.Errorf("auth.NewService: generating dummy hash: %w", err)
	}

	return &Service{
		repo:      repo,
		users:     users,
		hasher:    hasher,
		tokens:    tokens,
		tokenGen:  tokenGen,
		tx:        tx,
		cfg:       cfg,
		logger:    logger,
		dummyHash: dummyHash,
	}, nil
}

// ---------------------------------------------------------------------------
// Use Case 1: Register
// ---------------------------------------------------------------------------

// Register creates a new user account with password credentials and generates
// an email verification token.
//
// Transaction: Credential + verification token creation is atomic.
// User creation happens first via the User domain (non-transactional).
// If credential creation fails, an orphan user row exists but cannot
// authenticate (no credentials). Registration can be retried.
func (s *Service) Register(ctx context.Context, input RegisterInput) (result *RegisterResult, err error) {
	ctx, span := authTracer.Start(ctx, "auth.service.Register")
	defer span.End()
	start := time.Now()
	defer func() { recordAuthMetrics("register", err, start) }()

	tenantID, err := requireAuthTenantID(ctx)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("tenant_id", tenantID.String()))

	// 1. Validate input.
	if fieldErrors := validateRegisterInput(input); len(fieldErrors) > 0 {
		return nil, platformerrors.NewValidationError("invalid registration input", fieldErrors...)
	}

	// 2. Validate password policy.
	if validationErr := validatePassword(input.Password); validationErr != nil {
		return nil, validationErr
	}

	// 3. Hash password.
	passwordHash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return nil, fmt.Errorf("auth.Register: hashing password: %w", err)
	}

	// 4. Generate verification token.
	verificationRaw, err := s.tokenGen.Generate()
	if err != nil {
		return nil, fmt.Errorf("auth.Register: generating verification token: %w", err)
	}
	verificationHash := s.tokenGen.Hash(verificationRaw)

	// 5. Create user via User domain.
	email := normalizeAuthEmail(input.Email)
	userInfo, err := s.users.CreateUser(ctx, email, strings.TrimSpace(input.DisplayName))
	if err != nil {
		// Map conflict to registration-specific error.
		var domainErr *platformerrors.DomainError
		if platformerrors.As(err, &domainErr) && domainErr.Code == platformerrors.CodeConflict {
			return nil, ErrEmailAlreadyRegistered
		}
		return nil, err
	}

	// 6. Create credential + verification token atomically.
	expiresAt := time.Now().Add(s.cfg.VerificationTokenExpiry)
	err = s.tx.WithTx(ctx, func(txRepo Repository) error {
		if _, credErr := txRepo.CreateCredential(ctx, tenantID, userInfo.ID, passwordHash); credErr != nil {
			return credErr
		}
		if _, tokErr := txRepo.CreateEmailVerificationToken(ctx, tenantID, userInfo.ID, verificationHash, expiresAt); tokErr != nil {
			return tokErr
		}
		return nil
	})
	if err != nil {
		span.SetStatus(codes.Error, "registration failed")
		return nil, err
	}

	span.SetAttributes(attribute.String("user.id", userInfo.ID.String()))
	zerolog.Ctx(ctx).Info().
		Str("user_id", userInfo.ID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("user registered")

	return &RegisterResult{
		UserID:            userInfo.ID,
		Email:             userInfo.Email,
		VerificationToken: verificationRaw,
	}, nil
}

// ---------------------------------------------------------------------------
// Use Case 2: Login
// ---------------------------------------------------------------------------

// Login authenticates a user with email and password, creates a session,
// and returns access + refresh tokens.
//
// Anti-enumeration: Unknown email, wrong password, and inactive accounts
// all produce the same ErrInvalidCredentials error (RFC-0002 §16).
// A dummy hash verification runs on unknown emails to prevent timing leaks.
func (s *Service) Login(ctx context.Context, input LoginInput) (result *AuthResult, err error) {
	ctx, span := authTracer.Start(ctx, "auth.service.Login")
	defer span.End()
	start := time.Now()
	defer func() { recordAuthMetrics("login", err, start) }()

	tenantID, err := requireAuthTenantID(ctx)
	if err != nil {
		return nil, err
	}
	span.SetAttributes(attribute.String("tenant_id", tenantID.String()))

	// 1. Validate input.
	if fieldErrors := validateLoginInput(input); len(fieldErrors) > 0 {
		return nil, platformerrors.NewValidationError("invalid login input", fieldErrors...)
	}

	// 2. Find user by email.
	email := normalizeAuthEmail(input.Email)
	userInfo, err := s.users.GetUserByEmail(ctx, email)
	if err != nil {
		var domainErr *platformerrors.DomainError
		if platformerrors.As(err, &domainErr) && domainErr.Code == platformerrors.CodeNotFound {
			// User not found — run dummy hash to prevent timing leak.
			_, _ = s.hasher.Verify(input.Password, s.dummyHash)
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// 3. Check user is active (same error to prevent enumeration).
	if userInfo.Status != "active" {
		_, _ = s.hasher.Verify(input.Password, s.dummyHash)
		return nil, ErrInvalidCredentials
	}

	// 4. Check email verification (safe to distinguish per RFC-0002 §16).
	if !userInfo.EmailVerified {
		return nil, ErrEmailNotVerified
	}

	// 5. Get credential and verify password.
	cred, err := s.repo.GetCredentialByUserID(ctx, tenantID, userInfo.ID)
	if err != nil {
		// Credential not found for an existing user shouldn't happen,
		// but handle it without leaking information.
		_, _ = s.hasher.Verify(input.Password, s.dummyHash)
		return nil, ErrInvalidCredentials
	}

	match, err := s.hasher.Verify(input.Password, cred.PasswordHash)
	if err != nil {
		return nil, fmt.Errorf("auth.Login: verifying password: %w", err)
	}
	if !match {
		return nil, ErrInvalidCredentials
	}

	// 6. Create session + tokens.
	familyID := uuid.New()
	refreshRaw, err := s.tokenGen.Generate()
	if err != nil {
		return nil, fmt.Errorf("auth.Login: generating refresh token: %w", err)
	}
	fullRefreshToken := composeRefreshToken(familyID, refreshRaw)
	tokenHash := s.tokenGen.Hash(fullRefreshToken)

	session := &Session{
		TenantID:         tenantID,
		UserID:           userInfo.ID,
		FamilyID:         familyID,
		RefreshTokenHash: tokenHash,
		ExpiresAt:        time.Now().Add(s.cfg.SessionLifetime),
		IPAddress:        input.IPAddress,
		UserAgent:        input.UserAgent,
	}

	created, err := s.repo.CreateSession(ctx, session)
	if err != nil {
		return nil, err
	}

	// 7. Issue JWT access token.
	accessToken, err := s.tokens.Issue(TokenClaims{
		UserID:    userInfo.ID,
		TenantID:  tenantID,
		SessionID: created.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("auth.Login: issuing access token: %w", err)
	}

	span.SetAttributes(attribute.String("user.id", userInfo.ID.String()))
	zerolog.Ctx(ctx).Info().
		Str("user_id", userInfo.ID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("user logged in")

	return &AuthResult{
		AccessToken:  accessToken,
		RefreshToken: fullRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.cfg.AccessTokenLifetimeSecs,
	}, nil
}

// ---------------------------------------------------------------------------
// Use Case 3: RefreshToken
// ---------------------------------------------------------------------------

// RefreshToken exchanges a valid refresh token for new access + refresh tokens.
// It implements the full rotation and reuse detection flow per RFC-0002 §14.
//
// Transaction: The rotation uses SELECT ... FOR UPDATE to serialize
// concurrent refresh attempts at the database level.
func (s *Service) RefreshToken(ctx context.Context, input RefreshInput) (result *AuthResult, err error) {
	ctx, span := authTracer.Start(ctx, "auth.service.RefreshToken")
	defer span.End()
	start := time.Now()
	defer func() { recordAuthMetrics("refresh", err, start) }()

	tenantID, err := requireAuthTenantID(ctx)
	if err != nil {
		return nil, err
	}

	// 1. Parse refresh token to extract familyID.
	familyID, _, parseErr := parseRefreshToken(input.RefreshToken)
	if parseErr != nil {
		return nil, ErrRefreshTokenInvalid
	}

	incomingHash := s.tokenGen.Hash(input.RefreshToken)

	// 2. Execute rotation within a transaction (FOR UPDATE lock).
	var rotatedSession *Session
	var newRefreshToken string
	var reuseDetected bool

	txErr := s.tx.WithTx(ctx, func(txRepo Repository) error {
		session, getErr := txRepo.GetActiveSessionByFamilyIDForUpdate(ctx, familyID)
		if getErr != nil {
			return getErr
		}

		// 3. Compare hashes — detect reuse.
		if session.RefreshTokenHash != incomingHash {
			// Reuse detected — revoke the session and commit.
			// The error MUST NOT be discarded: if revocation fails,
			// the tx must roll back so the compromised session is not
			// left active.
			if revokeErr := txRepo.RevokeSession(ctx, tenantID, session.ID); revokeErr != nil {
				return fmt.Errorf("auth.RefreshToken: revoking compromised session: %w", revokeErr)
			}
			reuseDetected = true
			return nil // Return nil so the revocation commits.
		}

		// 4. Check session state.
		if session.IsExpired() {
			return ErrSessionExpired
		}

		// 5. Generate new refresh token and rotate.
		raw, genErr := s.tokenGen.Generate()
		if genErr != nil {
			return fmt.Errorf("auth.RefreshToken: generating token: %w", genErr)
		}
		newRefreshToken = composeRefreshToken(familyID, raw)
		newHash := s.tokenGen.Hash(newRefreshToken)

		var rotErr error
		rotatedSession, rotErr = txRepo.RotateRefreshToken(ctx, tenantID, session.ID, newHash)
		return rotErr
	})

	if txErr != nil {
		return nil, txErr
	}
	if reuseDetected {
		zerolog.Ctx(ctx).Warn().
			Str("tenant_id", tenantID.String()).
			Msg("refresh token reuse detected — session revoked")
		return nil, ErrRefreshTokenReuse
	}

	// 6. Verify user is still active.
	userInfo, err := s.users.GetUser(ctx, rotatedSession.UserID)
	if err != nil {
		return nil, ErrInvalidCredentials
	}
	if userInfo.Status != "active" {
		return nil, ErrInvalidCredentials
	}

	// 7. Issue new JWT.
	accessToken, err := s.tokens.Issue(TokenClaims{
		UserID:    rotatedSession.UserID,
		TenantID:  tenantID,
		SessionID: rotatedSession.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("auth.RefreshToken: issuing access token: %w", err)
	}

	return &AuthResult{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    s.cfg.AccessTokenLifetimeSecs,
	}, nil
}

// ---------------------------------------------------------------------------
// Use Case 4: Logout
// ---------------------------------------------------------------------------

// Logout revokes the current authentication session. It is idempotent —
// revoking an already-revoked session is a no-op.
func (s *Service) Logout(ctx context.Context, input LogoutInput) (err error) {
	ctx, span := authTracer.Start(ctx, "auth.service.Logout")
	defer span.End()
	start := time.Now()
	defer func() { recordAuthMetrics("logout", err, start) }()

	tenantID, err := requireAuthTenantID(ctx)
	if err != nil {
		return err
	}

	if err := s.repo.RevokeSession(ctx, tenantID, input.SessionID); err != nil {
		return err
	}

	zerolog.Ctx(ctx).Info().
		Str("tenant_id", tenantID.String()).
		Msg("session revoked")

	return nil
}

// ---------------------------------------------------------------------------
// Use Case 5: LogoutAll
// ---------------------------------------------------------------------------

// LogoutAll revokes all active sessions belonging to the user.
// It does not affect sessions of other users. It is idempotent.
func (s *Service) LogoutAll(ctx context.Context, input LogoutAllInput) (err error) {
	ctx, span := authTracer.Start(ctx, "auth.service.LogoutAll")
	defer span.End()
	start := time.Now()
	defer func() { recordAuthMetrics("logout_all", err, start) }()

	tenantID, err := requireAuthTenantID(ctx)
	if err != nil {
		return err
	}

	if err := s.repo.RevokeAllUserSessions(ctx, tenantID, input.UserID); err != nil {
		return err
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", input.UserID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("all sessions revoked")

	return nil
}

// ---------------------------------------------------------------------------
// Use Case 6: ChangePassword
// ---------------------------------------------------------------------------

// ChangePassword updates the authenticated user's password. It verifies
// the current password, validates the new password, updates credentials,
// and revokes all other sessions (keeping the current session active).
//
// Transaction: Credential update + session revocation is atomic.
func (s *Service) ChangePassword(ctx context.Context, input ChangePasswordInput) (err error) {
	ctx, span := authTracer.Start(ctx, "auth.service.ChangePassword")
	defer span.End()
	start := time.Now()
	defer func() { recordAuthMetrics("change_password", err, start) }()

	tenantID, err := requireAuthTenantID(ctx)
	if err != nil {
		return err
	}

	// 1. Validate new password policy.
	if validationErr := validatePassword(input.NewPassword); validationErr != nil {
		return validationErr
	}

	// 2. Get current credential and verify current password.
	cred, err := s.repo.GetCredentialByUserID(ctx, tenantID, input.UserID)
	if err != nil {
		return ErrInvalidCredentials
	}

	match, err := s.hasher.Verify(input.CurrentPassword, cred.PasswordHash)
	if err != nil {
		return fmt.Errorf("auth.ChangePassword: verifying current password: %w", err)
	}
	if !match {
		return ErrInvalidCredentials
	}

	// 3. Hash new password.
	newHash, err := s.hasher.Hash(input.NewPassword)
	if err != nil {
		return fmt.Errorf("auth.ChangePassword: hashing new password: %w", err)
	}

	// 4. Update credential + revoke other sessions atomically.
	err = s.tx.WithTx(ctx, func(txRepo Repository) error {
		if _, updateErr := txRepo.UpdatePasswordHash(ctx, tenantID, input.UserID, newHash); updateErr != nil {
			return updateErr
		}
		return txRepo.RevokeOtherUserSessions(ctx, tenantID, input.UserID, input.SessionID)
	})
	if err != nil {
		return err
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", input.UserID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("password changed")

	return nil
}

// ---------------------------------------------------------------------------
// Use Case 7: ForgotPassword
// ---------------------------------------------------------------------------

// ForgotPassword generates a password reset token if the account exists.
// It always succeeds (returns nil error) regardless of whether the email
// exists, to prevent account enumeration.
//
// The result is nil when the email is unknown. The handler MUST return
// the same generic response in both cases.
func (s *Service) ForgotPassword(ctx context.Context, input ForgotPasswordInput) (result *ForgotPasswordResult, err error) {
	ctx, span := authTracer.Start(ctx, "auth.service.ForgotPassword")
	defer span.End()
	start := time.Now()
	defer func() { recordAuthMetrics("forgot_password", err, start) }()

	tenantID, err := requireAuthTenantID(ctx)
	if err != nil {
		return nil, err
	}

	// 1. Look up user by email.
	email := normalizeAuthEmail(input.Email)
	userInfo, err := s.users.GetUserByEmail(ctx, email)
	if err != nil {
		var domainErr *platformerrors.DomainError
		if platformerrors.As(err, &domainErr) && domainErr.Code == platformerrors.CodeNotFound {
			// User not found — return nil result (anti-enumeration).
			return nil, nil
		}
		return nil, err
	}

	// 2. Invalidate existing reset tokens for this user.
	_ = s.repo.InvalidatePasswordResetTokensForUser(ctx, tenantID, userInfo.ID)

	// 3. Generate reset token.
	raw, err := s.tokenGen.Generate()
	if err != nil {
		return nil, fmt.Errorf("auth.ForgotPassword: generating reset token: %w", err)
	}
	hash := s.tokenGen.Hash(raw)
	expiresAt := time.Now().Add(s.cfg.ResetTokenExpiry)

	if _, err := s.repo.CreatePasswordResetToken(ctx, tenantID, userInfo.ID, hash, expiresAt); err != nil {
		return nil, err
	}

	zerolog.Ctx(ctx).Info().
		Str("tenant_id", tenantID.String()).
		Msg("password reset token created")

	return &ForgotPasswordResult{
		ResetToken: raw,
		UserID:     userInfo.ID,
		Email:      userInfo.Email,
	}, nil
}

// ---------------------------------------------------------------------------
// Use Case 8: ResetPassword
// ---------------------------------------------------------------------------

// ResetPassword consumes a valid reset token, updates the password, and
// revokes all sessions, forcing re-authentication.
//
// Transaction: Credential update + token consumption + session revocation.
func (s *Service) ResetPassword(ctx context.Context, input ResetPasswordInput) (err error) {
	ctx, span := authTracer.Start(ctx, "auth.service.ResetPassword")
	defer span.End()
	start := time.Now()
	defer func() { recordAuthMetrics("reset_password", err, start) }()

	tenantID, err := requireAuthTenantID(ctx)
	if err != nil {
		return err
	}

	// 1. Validate new password policy.
	if validationErr := validatePassword(input.NewPassword); validationErr != nil {
		return validationErr
	}

	// 2. Look up token by hash.
	tokenHash := s.tokenGen.Hash(input.Token)
	resetToken, err := s.repo.GetPasswordResetTokenByHash(ctx, tokenHash)
	if err != nil {
		return ErrResetTokenNotFound
	}

	// 3. Validate token state.
	if resetToken.IsUsed() {
		return ErrResetTokenAlreadyUsed
	}
	if resetToken.IsExpired() {
		return ErrResetTokenExpired
	}

	// 4. Hash new password.
	newHash, err := s.hasher.Hash(input.NewPassword)
	if err != nil {
		return fmt.Errorf("auth.ResetPassword: hashing new password: %w", err)
	}

	// 5. Update credential + consume token + revoke sessions atomically.
	err = s.tx.WithTx(ctx, func(txRepo Repository) error {
		if _, updateErr := txRepo.UpdatePasswordHash(ctx, tenantID, resetToken.UserID, newHash); updateErr != nil {
			return updateErr
		}
		if consumeErr := txRepo.ConsumePasswordResetToken(ctx, resetToken.ID); consumeErr != nil {
			return consumeErr
		}
		return txRepo.RevokeAllUserSessions(ctx, tenantID, resetToken.UserID)
	})
	if err != nil {
		return err
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", resetToken.UserID.String()).
		Str("tenant_id", tenantID.String()).
		Msg("password reset completed")

	return nil
}

// ---------------------------------------------------------------------------
// Use Case 9: VerifyEmail
// ---------------------------------------------------------------------------

// VerifyEmail consumes a valid verification token and marks the user's
// email as verified.
//
// Order of operations: validate token → set email verified → consume token.
// If SetEmailVerified fails, the token remains unconsumed and can be retried.
// If token consumption fails after verification, the email is verified but
// the token is unconsumed (harmless — it expires naturally).
func (s *Service) VerifyEmail(ctx context.Context, input VerifyEmailInput) (err error) {
	ctx, span := authTracer.Start(ctx, "auth.service.VerifyEmail")
	defer span.End()
	start := time.Now()
	defer func() { recordAuthMetrics("verify_email", err, start) }()

	_, err = requireAuthTenantID(ctx)
	if err != nil {
		return err
	}

	// 1. Look up token by hash.
	tokenHash := s.tokenGen.Hash(input.Token)
	verifyToken, err := s.repo.GetEmailVerificationTokenByHash(ctx, tokenHash)
	if err != nil {
		return ErrVerificationTokenNotFound
	}

	// 2. Validate token state.
	if verifyToken.IsUsed() {
		return ErrVerificationTokenAlreadyUsed
	}
	if verifyToken.IsExpired() {
		return ErrVerificationTokenExpired
	}

	// 3. Mark email as verified via User domain.
	if err := s.users.SetEmailVerified(ctx, verifyToken.UserID); err != nil {
		return err
	}

	// 4. Consume the verification token.
	if err := s.repo.ConsumeEmailVerificationToken(ctx, verifyToken.ID); err != nil {
		// Email is already verified — token consumption failure is non-critical.
		zerolog.Ctx(ctx).Warn().
			Err(err).
			Str("user_id", verifyToken.UserID.String()).
			Msg("failed to consume verification token after email verified")
	}

	zerolog.Ctx(ctx).Info().
		Str("user_id", verifyToken.UserID.String()).
		Msg("email verified")

	return nil
}

// ---------------------------------------------------------------------------
// Use Case 10: ResendVerification
// ---------------------------------------------------------------------------

// ResendVerification invalidates existing verification tokens and creates
// a new one. It always succeeds (nil error) regardless of whether the
// email exists or is already verified, to prevent enumeration.
//
// The result is nil when the user doesn't exist or email is already verified.
func (s *Service) ResendVerification(ctx context.Context, input ResendVerificationInput) (result *ResendVerificationResult, err error) {
	ctx, span := authTracer.Start(ctx, "auth.service.ResendVerification")
	defer span.End()
	start := time.Now()
	defer func() { recordAuthMetrics("resend_verification", err, start) }()

	tenantID, err := requireAuthTenantID(ctx)
	if err != nil {
		return nil, err
	}

	// 1. Look up user by email.
	email := normalizeAuthEmail(input.Email)
	userInfo, err := s.users.GetUserByEmail(ctx, email)
	if err != nil {
		var domainErr *platformerrors.DomainError
		if platformerrors.As(err, &domainErr) && domainErr.Code == platformerrors.CodeNotFound {
			return nil, nil // anti-enumeration
		}
		return nil, err
	}

	// 2. If already verified, return nil (no action needed).
	if userInfo.EmailVerified {
		return nil, nil
	}

	// 3. Invalidate existing verification tokens.
	_ = s.repo.InvalidateEmailVerificationTokensForUser(ctx, tenantID, userInfo.ID)

	// 4. Generate new verification token.
	raw, err := s.tokenGen.Generate()
	if err != nil {
		return nil, fmt.Errorf("auth.ResendVerification: generating token: %w", err)
	}
	hash := s.tokenGen.Hash(raw)
	expiresAt := time.Now().Add(s.cfg.VerificationTokenExpiry)

	if _, err := s.repo.CreateEmailVerificationToken(ctx, tenantID, userInfo.ID, hash, expiresAt); err != nil {
		return nil, err
	}

	zerolog.Ctx(ctx).Info().
		Str("tenant_id", tenantID.String()).
		Msg("verification token resent")

	return &ResendVerificationResult{
		VerificationToken: raw,
		UserID:            userInfo.ID,
		Email:             userInfo.Email,
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// requireAuthTenantID extracts the tenant ID from context and returns a
// parsed UUID. Same logic as user.requireTenantID but in the auth package
// (unexported helpers are not shared across packages).
func requireAuthTenantID(ctx context.Context) (uuid.UUID, error) {
	tenantIDStr, ok := tenant.GetID(ctx)
	if !ok {
		return uuid.Nil, platformerrors.NewDomainError(platformerrors.CodeTenantRequired, "tenant ID is required")
	}

	tenantID, err := uuid.Parse(tenantIDStr)
	if err != nil {
		return uuid.Nil, platformerrors.NewBadRequest("invalid tenant ID format")
	}

	return tenantID, nil
}

// normalizeAuthEmail lowercases and trims an email address.
func normalizeAuthEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// validatePassword checks the password against the NIST SP 800-63B policy
// defined in RFC-0002 §10: min 12 chars, max 128 chars, no composition rules.
func validatePassword(password string) *platformerrors.ValidationError {
	var fieldErrors []platformerrors.FieldError
	if len(password) < 12 {
		fieldErrors = append(fieldErrors, platformerrors.FieldError{
			Field: "password", Message: "must be at least 12 characters",
		})
	}
	if len(password) > 128 {
		fieldErrors = append(fieldErrors, platformerrors.FieldError{
			Field: "password", Message: "must be at most 128 characters",
		})
	}
	if len(fieldErrors) > 0 {
		return platformerrors.NewValidationError("password does not meet requirements", fieldErrors...)
	}
	return nil
}

// validateRegisterInput validates non-password registration fields.
func validateRegisterInput(input RegisterInput) []platformerrors.FieldError {
	var errs []platformerrors.FieldError

	email := strings.TrimSpace(input.Email)
	if email == "" {
		errs = append(errs, platformerrors.FieldError{Field: "email", Message: "is required"})
	} else if !strings.Contains(email, "@") {
		errs = append(errs, platformerrors.FieldError{Field: "email", Message: "must be a valid email address"})
	} else if len(email) > 320 {
		errs = append(errs, platformerrors.FieldError{Field: "email", Message: "must be at most 320 characters"})
	}

	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		errs = append(errs, platformerrors.FieldError{Field: "display_name", Message: "is required"})
	} else if len(displayName) > 256 {
		errs = append(errs, platformerrors.FieldError{Field: "display_name", Message: "must be at most 256 characters"})
	}

	return errs
}

// validateLoginInput validates login fields.
func validateLoginInput(input LoginInput) []platformerrors.FieldError {
	var errs []platformerrors.FieldError

	if strings.TrimSpace(input.Email) == "" {
		errs = append(errs, platformerrors.FieldError{Field: "email", Message: "is required"})
	}
	if input.Password == "" {
		errs = append(errs, platformerrors.FieldError{Field: "password", Message: "is required"})
	}

	return errs
}

// composeRefreshToken creates a refresh token in the format "familyID.raw".
// The familyID prefix allows the service to identify the session's token
// family without a separate database lookup.
func composeRefreshToken(familyID uuid.UUID, raw string) string {
	return familyID.String() + "." + raw
}

// parseRefreshToken splits a composed refresh token into familyID and raw part.
func parseRefreshToken(token string) (familyID uuid.UUID, raw string, err error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return uuid.Nil, "", fmt.Errorf("malformed refresh token")
	}

	familyID, err = uuid.Parse(parts[0])
	if err != nil {
		return uuid.Nil, "", fmt.Errorf("invalid family ID in refresh token")
	}

	return familyID, parts[1], nil
}

// recordAuthMetrics records operation metrics for the auth domain.
func recordAuthMetrics(operation string, err error, start time.Time) {
	status := "success"
	if err != nil {
		status = classifyAuthError(err)
	}
	operationsTotal.WithLabelValues(operation, status).Inc()
	operationDuration.WithLabelValues(operation).Observe(time.Since(start).Seconds())
}

// classifyAuthError maps domain errors to metric status labels.
func classifyAuthError(err error) string {
	var domainErr *platformerrors.DomainError
	if platformerrors.As(err, &domainErr) {
		switch domainErr.Code {
		case platformerrors.CodeNotFound:
			return "not_found"
		case platformerrors.CodeConflict:
			return "conflict"
		case platformerrors.CodeUnauthorized:
			return "unauthorized"
		case platformerrors.CodeForbidden:
			return "forbidden"
		default:
			return "error"
		}
	}
	return "error"
}
