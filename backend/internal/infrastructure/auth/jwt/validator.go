package jwt

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ValidatedClaims holds the parsed and validated claims from a JWT.
// This is returned to the caller (e.g., middleware) after successful validation.
type ValidatedClaims struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	JTI       string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

// ValidatorConfig holds configuration for JWT validation.
type ValidatorConfig struct {
	Issuer    string        // Expected "iss" claim.
	Audience  string        // Expected "aud" claim.
	ClockSkew time.Duration // Tolerance for clock differences. Default: 5s.
}

// DefaultValidatorConfig returns production defaults per security-standards §3.
func DefaultValidatorConfig() ValidatorConfig {
	return ValidatorConfig{
		Issuer:    "conduit",
		Audience:  "conduit-api",
		ClockSkew: 5 * time.Second,
	}
}

// Validation errors. These are intentionally generic to avoid leaking
// cryptographic details to external callers.
var (
	ErrTokenInvalid    = errors.New("jwt: token is invalid")
	ErrTokenExpired    = errors.New("jwt: token has expired")
	ErrTokenMalformed  = errors.New("jwt: token is malformed")
	ErrTokenClaimsMissing = errors.New("jwt: required claims missing")
)

// EdDSAValidator validates JWTs signed with Ed25519/EdDSA.
// It enforces algorithm, signature, issuer, audience, expiration, and
// required claims. It rejects all non-EdDSA algorithms including
// HS256, RS256, ES256, and alg=none.
type EdDSAValidator struct {
	resolver KeyResolver
	cfg      ValidatorConfig
}

// NewEdDSAValidator creates a new JWT validator.
func NewEdDSAValidator(resolver KeyResolver, cfg ValidatorConfig) (*EdDSAValidator, error) {
	if resolver == nil {
		return nil, fmt.Errorf("jwt: key resolver must not be nil")
	}
	if cfg.Issuer == "" {
		return nil, fmt.Errorf("jwt: issuer must not be empty")
	}
	if cfg.Audience == "" {
		return nil, fmt.Errorf("jwt: audience must not be empty")
	}
	if cfg.ClockSkew < 0 {
		return nil, fmt.Errorf("jwt: clock skew must be >= 0")
	}
	return &EdDSAValidator{resolver: resolver, cfg: cfg}, nil
}

// Validate parses and validates a JWT string. It returns the validated claims
// on success, or an error describing the failure.
//
// Validation steps:
//  1. Parse the token safely.
//  2. Require EdDSA algorithm (reject HS256, RS256, ES256, alg=none).
//  3. Resolve the signing key through kid.
//  4. Verify the Ed25519 signature.
//  5. Validate issuer, audience, and expiration.
//  6. Require sub, sid, and jti claims.
func (v *EdDSAValidator) Validate(tokenString string) (*ValidatedClaims, error) {
	// Define the key function that:
	// 1. Enforces EdDSA algorithm only.
	// 2. Resolves the public key by kid from the JWT header.
	keyFunc := func(token *jwt.Token) (interface{}, error) {
		// CRITICAL: Reject any algorithm that is not EdDSA.
		// This prevents algorithm confusion attacks (HS256, RS256, alg=none).
		if _, ok := token.Method.(*jwt.SigningMethodEd25519); !ok {
			return nil, fmt.Errorf("jwt: unexpected signing method %v", token.Header["alg"])
		}

		// Extract kid from header.
		kidRaw, ok := token.Header["kid"]
		if !ok {
			return nil, errors.New("jwt: missing kid header")
		}
		kid, ok := kidRaw.(string)
		if !ok || kid == "" {
			return nil, errors.New("jwt: invalid kid header")
		}

		// Resolve the public key by kid.
		publicKey, err := v.resolver.ResolveKey(kid)
		if err != nil {
			// Do not reveal whether the kid exists or not in external errors.
			return nil, ErrTokenInvalid
		}

		return ed25519.PublicKey(publicKey), nil
	}

	// Parse with validation options.
	token, err := jwt.ParseWithClaims(tokenString, &conduitClaims{}, keyFunc,
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithAudience(v.cfg.Audience),
		jwt.WithLeeway(v.cfg.ClockSkew),
		jwt.WithValidMethods([]string{"EdDSA"}),
	)
	if err != nil {
		// Map library errors to our error types without leaking details.
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		}
		return nil, ErrTokenInvalid
	}

	if !token.Valid {
		return nil, ErrTokenInvalid
	}

	// Extract and validate our custom claims.
	claims, ok := token.Claims.(*conduitClaims)
	if !ok {
		return nil, ErrTokenInvalid
	}

	// Require sub (user ID).
	if claims.Subject == "" {
		return nil, ErrTokenClaimsMissing
	}
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, ErrTokenClaimsMissing
	}

	// Require sid (session ID).
	if claims.SessionID == "" {
		return nil, ErrTokenClaimsMissing
	}
	sessionID, err := uuid.Parse(claims.SessionID)
	if err != nil {
		return nil, ErrTokenClaimsMissing
	}

	// Require jti.
	if claims.ID == "" {
		return nil, ErrTokenClaimsMissing
	}

	// Extract time claims.
	var issuedAt, expiresAt time.Time
	if claims.IssuedAt != nil {
		issuedAt = claims.IssuedAt.Time
	}
	if claims.ExpiresAt != nil {
		expiresAt = claims.ExpiresAt.Time
	}

	return &ValidatedClaims{
		UserID:    userID,
		SessionID: sessionID,
		JTI:       claims.ID,
		IssuedAt:  issuedAt,
		ExpiresAt: expiresAt,
	}, nil
}
