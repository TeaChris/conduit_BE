package app

import (
	"crypto/ed25519"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/conduit-platform/conduit/backend/internal/auth"
	"github.com/conduit-platform/conduit/backend/internal/config"
	authjwt "github.com/conduit-platform/conduit/backend/internal/infrastructure/auth/jwt"
	"github.com/conduit-platform/conduit/backend/internal/infrastructure/auth/password"
	"github.com/conduit-platform/conduit/backend/internal/infrastructure/auth/token"
	"github.com/conduit-platform/conduit/backend/internal/user"
)

// authDeps holds the constructed authentication components.
// The service handles business logic; the jwtValidator will be used
// by HTTP middleware in a future phase.
type authDeps struct {
	service      *auth.Service
	jwtValidator *authjwt.EdDSAValidator
}

// buildAuth constructs the complete authentication subsystem from configuration.
// It validates cryptographic configuration, loads signing keys, constructs
// infrastructure implementations, and wires them into auth.Service.
//
// This is the composition root for all authentication concerns. If any step
// fails, the application should not start.
func buildAuth(
	cfg config.AuthConfig,
	pool *pgxpool.Pool,
	userSvc *user.Service,
	logger zerolog.Logger,
) (*authDeps, error) {
	// 1. Validate required configuration fields.
	if err := validateAuthConfig(cfg); err != nil {
		return nil, err
	}

	// 2. Load and validate the Ed25519 signing key.
	signingKey, err := loadSigningKey(cfg.JWT)
	if err != nil {
		return nil, err
	}

	// 3. Build cryptographic infrastructure.
	hasher, err := buildPasswordHasher(cfg.Password)
	if err != nil {
		return nil, fmt.Errorf("password hasher: %w", err)
	}

	tokenGen, err := buildTokenGenerator()
	if err != nil {
		return nil, fmt.Errorf("token generator: %w", err)
	}

	issuer, err := buildJWTIssuer(signingKey, cfg.JWT)
	if err != nil {
		return nil, fmt.Errorf("jwt issuer: %w", err)
	}

	validator, err := buildJWTValidator(signingKey, cfg.JWT)
	if err != nil {
		return nil, fmt.Errorf("jwt validator: %w", err)
	}

	// 4. Build persistence infrastructure (reuses the existing pool).
	authRepo := auth.NewPostgresRepository(pool)
	txManager := auth.NewPostgresTxManager(pool)

	// 5. Build user domain adapter.
	userProvider := NewUserProviderAdapter(userSvc)

	// 6. Build auth service config — derive AccessTokenLifetimeSecs from JWT TTL.
	svcCfg := auth.ServiceConfig{
		SessionLifetime:         30 * 24 * time.Hour,
		AccessTokenLifetimeSecs: int(cfg.JWT.AccessTokenLifetime.Seconds()),
		ResetTokenExpiry:        30 * time.Minute,
		VerificationTokenExpiry: 24 * time.Hour,
	}

	// 7. Construct the auth service.
	authLogger := logger.With().Str("component", "auth").Logger()
	svc, err := auth.NewService(
		authRepo,
		userProvider,
		hasher,
		issuer,
		tokenGen,
		txManager,
		svcCfg,
		authLogger,
	)
	if err != nil {
		return nil, fmt.Errorf("auth service: %w", err)
	}

	return &authDeps{
		service:      svc,
		jwtValidator: validator,
	}, nil
}

// validateAuthConfig checks that all required authentication configuration
// fields are present. This runs before any cryptographic operations.
// Errors reference field names, never secret values.
func validateAuthConfig(cfg config.AuthConfig) error {
	if cfg.JWT.ActiveKID == "" {
		return fmt.Errorf("AUTH_JWT_ACTIVE_KID is required")
	}
	if cfg.JWT.PrivateKey == "" {
		return fmt.Errorf("AUTH_JWT_PRIVATE_KEY is required")
	}
	if cfg.JWT.Issuer == "" {
		return fmt.Errorf("AUTH_JWT_ISSUER is required")
	}
	if cfg.JWT.Audience == "" {
		return fmt.Errorf("AUTH_JWT_AUDIENCE is required")
	}
	return nil
}

// loadSigningKey loads and validates the Ed25519 signing key from JWT config.
// It parses the PEM-encoded private key, extracts the public key, verifies
// key pair consistency, and associates the key with the configured kid.
//
// Security: errors must not include the private key or PEM data.
func loadSigningKey(cfg config.JWTConfig) (*authjwt.SigningKey, error) {
	// Replace literal \n with actual newlines (env vars often escape newlines).
	pemStr := strings.ReplaceAll(cfg.PrivateKey, `\n`, "\n")

	// Parse PEM-encoded Ed25519 private key.
	privateKey, err := authjwt.ParseEd25519PrivateKeyPEM([]byte(pemStr))
	if err != nil {
		// Do NOT include the PEM data in the error.
		return nil, fmt.Errorf("jwt signing key is invalid: failed to parse Ed25519 PEM")
	}

	// Extract the public key from the private key.
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("jwt signing key is invalid: cannot extract public key")
	}

	// Verify key pair consistency by signing and verifying a test message.
	testMessage := []byte("conduit-key-pair-validation")
	signature := ed25519.Sign(privateKey, testMessage)
	if !ed25519.Verify(publicKey, testMessage, signature) {
		return nil, fmt.Errorf("jwt signing key is invalid: key pair verification failed")
	}

	return &authjwt.SigningKey{
		ID:         cfg.ActiveKID,
		PrivateKey: privateKey,
		PublicKey:  publicKey,
	}, nil
}

// buildPasswordHasher constructs the Argon2id password hasher from configuration.
func buildPasswordHasher(cfg config.PasswordConfig) (auth.PasswordHasher, error) {
	params := password.Params{
		Memory:      cfg.Memory,
		Iterations:  cfg.Iterations,
		Parallelism: cfg.Parallelism,
		SaltLength:  cfg.SaltLength,
		KeyLength:   cfg.KeyLength,
	}
	return password.NewArgon2idHasher(params)
}

// buildTokenGenerator constructs the secure token generator.
// Uses 32 bytes (256 bits) of entropy per token per TDR-0001.
func buildTokenGenerator() (auth.TokenGenerator, error) {
	return token.NewSecureGenerator(32)
}

// buildJWTIssuer constructs the EdDSA JWT issuer from the signing key and config.
// The issuer and audience values come from a single config source to prevent
// configuration drift between issuance and validation.
func buildJWTIssuer(key *authjwt.SigningKey, cfg config.JWTConfig) (auth.TokenIssuer, error) {
	issuerCfg := authjwt.IssuerConfig{
		Issuer:              cfg.Issuer,
		Audience:            cfg.Audience,
		AccessTokenLifetime: cfg.AccessTokenLifetime,
	}
	return authjwt.NewEdDSAIssuer(key, issuerCfg)
}

// buildJWTValidator constructs the EdDSA JWT validator.
// The resolver is initialized with the active signing key's public key.
// During key rotation, additional public keys can be added to the resolver
// without restarting the application.
func buildJWTValidator(key *authjwt.SigningKey, cfg config.JWTConfig) (*authjwt.EdDSAValidator, error) {
	resolver := authjwt.NewStaticKeyResolver(*key)
	validatorCfg := authjwt.ValidatorConfig{
		Issuer:    cfg.Issuer,
		Audience:  cfg.Audience,
		ClockSkew: cfg.ClockSkew,
	}
	return authjwt.NewEdDSAValidator(resolver, validatorCfg)
}
