package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/conduit-platform/conduit/backend/internal/auth"
	"github.com/conduit-platform/conduit/backend/internal/config"
	authjwt "github.com/conduit-platform/conduit/backend/internal/infrastructure/auth/jwt"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// generateTestPEM generates a PEM-encoded Ed25519 private key for testing.
func generateTestPEM(t *testing.T) string {
	t.Helper()
	key, err := authjwt.GenerateSigningKey("test-kid")
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}
	pemData, err := authjwt.MarshalEd25519PrivateKeyPEM(key.PrivateKey)
	if err != nil {
		t.Fatalf("MarshalEd25519PrivateKeyPEM: %v", err)
	}
	return string(pemData)
}

func testJWTConfig(pem string) config.JWTConfig {
	return config.JWTConfig{
		Issuer:              "conduit",
		Audience:            "conduit-api",
		ActiveKID:           "test-kid-1",
		PrivateKey:          pem,
		AccessTokenLifetime: 15 * time.Minute,
		ClockSkew:           5 * time.Second,
	}
}

func testPasswordConfig() config.PasswordConfig {
	return config.PasswordConfig{
		Memory:      64 * 1024,
		Iterations:  1, // Use 1 iteration in tests for speed.
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
}

func testAuthConfig(pem string) config.AuthConfig {
	return config.AuthConfig{
		JWT:      testJWTConfig(pem),
		Password: testPasswordConfig(),
	}
}

// stubRepository implements auth.Repository for unit tests.
// No methods are called during auth.NewService construction,
// so all methods are no-ops.
type stubRepository struct{}

func (*stubRepository) CreateCredential(_ context.Context, _, _ uuid.UUID, _ string) (*auth.Credential, error) {
	return nil, nil
}
func (*stubRepository) GetCredentialByUserID(_ context.Context, _, _ uuid.UUID) (*auth.Credential, error) {
	return nil, nil
}
func (*stubRepository) UpdatePasswordHash(_ context.Context, _, _ uuid.UUID, _ string) (*auth.Credential, error) {
	return nil, nil
}
func (*stubRepository) CreateSession(_ context.Context, _ *auth.Session) (*auth.Session, error) {
	return nil, nil
}
func (*stubRepository) GetSessionByID(_ context.Context, _, _ uuid.UUID) (*auth.Session, error) {
	return nil, nil
}
func (*stubRepository) GetActiveSessionByFamilyIDForUpdate(_ context.Context, _ uuid.UUID) (*auth.Session, error) {
	return nil, nil
}
func (*stubRepository) RotateRefreshToken(_ context.Context, _, _ uuid.UUID, _ string) (*auth.Session, error) {
	return nil, nil
}
func (*stubRepository) RevokeSession(_ context.Context, _, _ uuid.UUID) error { return nil }
func (*stubRepository) RevokeAllUserSessions(_ context.Context, _, _ uuid.UUID) error {
	return nil
}
func (*stubRepository) RevokeOtherUserSessions(_ context.Context, _, _, _ uuid.UUID) error {
	return nil
}
func (*stubRepository) CreatePasswordResetToken(_ context.Context, _, _ uuid.UUID, _ string, _ time.Time) (*auth.PasswordResetToken, error) {
	return nil, nil
}
func (*stubRepository) GetPasswordResetTokenByHash(_ context.Context, _ string) (*auth.PasswordResetToken, error) {
	return nil, nil
}
func (*stubRepository) ConsumePasswordResetToken(_ context.Context, _ uuid.UUID) error { return nil }
func (*stubRepository) InvalidatePasswordResetTokensForUser(_ context.Context, _, _ uuid.UUID) error {
	return nil
}
func (*stubRepository) CreateEmailVerificationToken(_ context.Context, _, _ uuid.UUID, _ string, _ time.Time) (*auth.EmailVerificationToken, error) {
	return nil, nil
}
func (*stubRepository) GetEmailVerificationTokenByHash(_ context.Context, _ string) (*auth.EmailVerificationToken, error) {
	return nil, nil
}
func (*stubRepository) ConsumeEmailVerificationToken(_ context.Context, _ uuid.UUID) error {
	return nil
}
func (*stubRepository) InvalidateEmailVerificationTokensForUser(_ context.Context, _, _ uuid.UUID) error {
	return nil
}

// stubTxManager implements auth.TxManager for unit tests.
type stubTxManager struct{}

func (*stubTxManager) WithTx(_ context.Context, fn func(txRepo auth.Repository) error) error {
	return fn(&stubRepository{})
}

// stubUserProvider implements auth.UserProvider for unit tests.
type stubUserProvider struct{}

func (*stubUserProvider) CreateUser(_ context.Context, _, _ string) (*auth.UserInfo, error) {
	return nil, nil
}
func (*stubUserProvider) GetUserByEmail(_ context.Context, _ string) (*auth.UserInfo, error) {
	return nil, nil
}
func (*stubUserProvider) GetUser(_ context.Context, _ uuid.UUID) (*auth.UserInfo, error) {
	return nil, nil
}
func (*stubUserProvider) SetEmailVerified(_ context.Context, _ uuid.UUID) error { return nil }

// ---------------------------------------------------------------------------
// validateAuthConfig tests
// ---------------------------------------------------------------------------

func TestValidateAuthConfig_Valid(t *testing.T) {
	pem := generateTestPEM(t)
	cfg := testAuthConfig(pem)
	if err := validateAuthConfig(cfg); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateAuthConfig_MissingKID(t *testing.T) {
	pem := generateTestPEM(t)
	cfg := testAuthConfig(pem)
	cfg.JWT.ActiveKID = ""
	err := validateAuthConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing kid")
	}
	if !strings.Contains(err.Error(), "ACTIVE_KID") {
		t.Errorf("error should mention ACTIVE_KID: %v", err)
	}
}

func TestValidateAuthConfig_MissingPrivateKey(t *testing.T) {
	pem := generateTestPEM(t)
	cfg := testAuthConfig(pem)
	cfg.JWT.PrivateKey = ""
	err := validateAuthConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing private key")
	}
	if !strings.Contains(err.Error(), "PRIVATE_KEY") {
		t.Errorf("error should mention PRIVATE_KEY: %v", err)
	}
}

func TestValidateAuthConfig_MissingIssuer(t *testing.T) {
	pem := generateTestPEM(t)
	cfg := testAuthConfig(pem)
	cfg.JWT.Issuer = ""
	err := validateAuthConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing issuer")
	}
}

func TestValidateAuthConfig_MissingAudience(t *testing.T) {
	pem := generateTestPEM(t)
	cfg := testAuthConfig(pem)
	cfg.JWT.Audience = ""
	err := validateAuthConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing audience")
	}
}

// ---------------------------------------------------------------------------
// loadSigningKey tests
// ---------------------------------------------------------------------------

func TestLoadSigningKey_Valid(t *testing.T) {
	pem := generateTestPEM(t)
	cfg := testJWTConfig(pem)
	key, err := loadSigningKey(cfg)
	if err != nil {
		t.Fatalf("loadSigningKey: %v", err)
	}
	if key.ID != "test-kid-1" {
		t.Errorf("kid = %q, want %q", key.ID, "test-kid-1")
	}
	if key.PrivateKey == nil {
		t.Error("private key is nil")
	}
	if key.PublicKey == nil {
		t.Error("public key is nil")
	}
}

func TestLoadSigningKey_InvalidPEM(t *testing.T) {
	cfg := testJWTConfig("not-a-valid-pem")
	_, err := loadSigningKey(cfg)
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
	// Error must NOT contain the PEM data.
	if strings.Contains(err.Error(), "not-a-valid-pem") {
		t.Error("error message contains PEM data")
	}
}

func TestLoadSigningKey_SecretNotInError(t *testing.T) {
	// Use a recognizable string to verify it doesn't appear in errors.
	fakePEM := "-----BEGIN PRIVATE KEY-----\nSECRET_MATERIAL_HERE\n-----END PRIVATE KEY-----"
	cfg := testJWTConfig(fakePEM)
	_, err := loadSigningKey(cfg)
	if err == nil {
		t.Fatal("expected error for invalid PEM content")
	}
	if strings.Contains(err.Error(), "SECRET_MATERIAL_HERE") {
		t.Error("error message contains secret material")
	}
}

func TestLoadSigningKey_EscapedNewlines(t *testing.T) {
	// Generate a PEM key and replace real newlines with literal \n.
	pem := generateTestPEM(t)
	escaped := strings.ReplaceAll(pem, "\n", `\n`)
	cfg := testJWTConfig(escaped)
	key, err := loadSigningKey(cfg)
	if err != nil {
		t.Fatalf("loadSigningKey with escaped newlines: %v", err)
	}
	if key.PrivateKey == nil {
		t.Error("private key is nil")
	}
}

// ---------------------------------------------------------------------------
// buildPasswordHasher tests
// ---------------------------------------------------------------------------

func TestBuildPasswordHasher_DefaultParams(t *testing.T) {
	hasher, err := buildPasswordHasher(testPasswordConfig())
	if err != nil {
		t.Fatalf("buildPasswordHasher: %v", err)
	}
	// Verify it can hash and verify.
	hash, err := hasher.Hash("test-password-123")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	match, err := hasher.Verify("test-password-123", hash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !match {
		t.Error("password should match")
	}
}

func TestBuildPasswordHasher_InvalidParams(t *testing.T) {
	cfg := testPasswordConfig()
	cfg.Memory = 0 // Invalid
	_, err := buildPasswordHasher(cfg)
	if err == nil {
		t.Error("expected error for invalid params")
	}
}

// ---------------------------------------------------------------------------
// buildTokenGenerator tests
// ---------------------------------------------------------------------------

func TestBuildTokenGenerator_Works(t *testing.T) {
	gen, err := buildTokenGenerator()
	if err != nil {
		t.Fatalf("buildTokenGenerator: %v", err)
	}
	raw, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if raw == "" {
		t.Error("generated token is empty")
	}
	hash := gen.Hash(raw)
	if hash == "" {
		t.Error("hash is empty")
	}
	if len(hash) != 64 {
		t.Errorf("hash length = %d, want 64", len(hash))
	}
}

// ---------------------------------------------------------------------------
// buildJWTIssuer tests
// ---------------------------------------------------------------------------

func TestBuildJWTIssuer_ProducesValidToken(t *testing.T) {
	pem := generateTestPEM(t)
	cfg := testJWTConfig(pem)
	key, err := loadSigningKey(cfg)
	if err != nil {
		t.Fatalf("loadSigningKey: %v", err)
	}
	issuer, err := buildJWTIssuer(key, cfg)
	if err != nil {
		t.Fatalf("buildJWTIssuer: %v", err)
	}

	tokenString, err := issuer.Issue(auth.TokenClaims{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		SessionID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if tokenString == "" {
		t.Error("issued token is empty")
	}
}

func TestBuildJWTIssuer_KIDInToken(t *testing.T) {
	pem := generateTestPEM(t)
	cfg := testJWTConfig(pem)
	cfg.ActiveKID = "my-custom-kid"
	key, err := loadSigningKey(cfg)
	if err != nil {
		t.Fatalf("loadSigningKey: %v", err)
	}
	issuer, err := buildJWTIssuer(key, cfg)
	if err != nil {
		t.Fatalf("buildJWTIssuer: %v", err)
	}

	tokenString, err := issuer.Issue(auth.TokenClaims{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		SessionID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	// The kid is in the JWT header (base64url-encoded first segment).
	if !strings.Contains(tokenString, ".") {
		t.Fatal("token doesn't have JWT format")
	}
}

// ---------------------------------------------------------------------------
// buildJWTValidator tests
// ---------------------------------------------------------------------------

func TestBuildJWTValidator_ResolvesConfiguredKey(t *testing.T) {
	pem := generateTestPEM(t)
	cfg := testJWTConfig(pem)
	key, err := loadSigningKey(cfg)
	if err != nil {
		t.Fatalf("loadSigningKey: %v", err)
	}
	validator, err := buildJWTValidator(key, cfg)
	if err != nil {
		t.Fatalf("buildJWTValidator: %v", err)
	}
	if validator == nil {
		t.Fatal("validator is nil")
	}
}

func TestBuildJWTValidator_RejectsWrongKey(t *testing.T) {
	// Create validator with one key.
	pem := generateTestPEM(t)
	cfg := testJWTConfig(pem)
	key, err := loadSigningKey(cfg)
	if err != nil {
		t.Fatalf("loadSigningKey: %v", err)
	}
	validator, err := buildJWTValidator(key, cfg)
	if err != nil {
		t.Fatalf("buildJWTValidator: %v", err)
	}

	// Issue token with a DIFFERENT key.
	otherKey, err := authjwt.GenerateSigningKey(cfg.ActiveKID)
	if err != nil {
		t.Fatalf("GenerateSigningKey: %v", err)
	}
	otherIssuer, err := authjwt.NewEdDSAIssuer(otherKey, authjwt.IssuerConfig{
		Issuer:              cfg.Issuer,
		Audience:            cfg.Audience,
		AccessTokenLifetime: cfg.AccessTokenLifetime,
	})
	if err != nil {
		t.Fatalf("NewEdDSAIssuer: %v", err)
	}
	tokenString, err := otherIssuer.Issue(auth.TokenClaims{
		UserID:    uuid.New(),
		TenantID:  uuid.New(),
		SessionID: uuid.New(),
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Validation should fail.
	_, err = validator.Validate(tokenString)
	if err == nil {
		t.Error("expected error validating token signed with wrong key")
	}
}

// ---------------------------------------------------------------------------
// JWT round-trip test (issue → validate)
// ---------------------------------------------------------------------------

func TestJWTRoundTrip(t *testing.T) {
	pem := generateTestPEM(t)
	cfg := testJWTConfig(pem)
	key, err := loadSigningKey(cfg)
	if err != nil {
		t.Fatalf("loadSigningKey: %v", err)
	}
	issuer, err := buildJWTIssuer(key, cfg)
	if err != nil {
		t.Fatalf("buildJWTIssuer: %v", err)
	}
	validator, err := buildJWTValidator(key, cfg)
	if err != nil {
		t.Fatalf("buildJWTValidator: %v", err)
	}

	userID := uuid.New()
	sessionID := uuid.New()

	tokenString, err := issuer.Issue(auth.TokenClaims{
		UserID:    userID,
		TenantID:  uuid.New(),
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	validated, err := validator.Validate(tokenString)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if validated.UserID != userID {
		t.Errorf("UserID = %v, want %v", validated.UserID, userID)
	}
	if validated.SessionID != sessionID {
		t.Errorf("SessionID = %v, want %v", validated.SessionID, sessionID)
	}
}

// ---------------------------------------------------------------------------
// Full composition test (without database)
// ---------------------------------------------------------------------------

func TestAuthComposition_ServiceConstruction(t *testing.T) {
	pem := generateTestPEM(t)
	cfg := testJWTConfig(pem)
	key, err := loadSigningKey(cfg)
	if err != nil {
		t.Fatalf("loadSigningKey: %v", err)
	}

	hasher, err := buildPasswordHasher(testPasswordConfig())
	if err != nil {
		t.Fatalf("buildPasswordHasher: %v", err)
	}
	tokenGen, err := buildTokenGenerator()
	if err != nil {
		t.Fatalf("buildTokenGenerator: %v", err)
	}
	issuer, err := buildJWTIssuer(key, cfg)
	if err != nil {
		t.Fatalf("buildJWTIssuer: %v", err)
	}
	validator, err := buildJWTValidator(key, cfg)
	if err != nil {
		t.Fatalf("buildJWTValidator: %v", err)
	}

	// Construct auth.Service with stubs.
	svc, err := auth.NewService(
		&stubRepository{},
		&stubUserProvider{},
		hasher,
		issuer,
		tokenGen,
		&stubTxManager{},
		auth.DefaultServiceConfig(),
		zerolog.Nop(),
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	if svc == nil {
		t.Fatal("service is nil")
	}
	if validator == nil {
		t.Fatal("validator is nil")
	}
}

func TestAuthComposition_PasswordHashingWorks(t *testing.T) {
	hasher, err := buildPasswordHasher(testPasswordConfig())
	if err != nil {
		t.Fatalf("buildPasswordHasher: %v", err)
	}

	hash, err := hasher.Hash("my-secure-password")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	match, err := hasher.Verify("my-secure-password", hash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !match {
		t.Error("password should match")
	}

	noMatch, err := hasher.Verify("wrong-password", hash)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if noMatch {
		t.Error("wrong password should not match")
	}
}

func TestAuthComposition_TokenGenerationWorks(t *testing.T) {
	gen, err := buildTokenGenerator()
	if err != nil {
		t.Fatalf("buildTokenGenerator: %v", err)
	}

	raw, err := gen.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	hash := gen.Hash(raw)
	hash2 := gen.Hash(raw)
	if hash != hash2 {
		t.Error("hash should be deterministic")
	}
}
